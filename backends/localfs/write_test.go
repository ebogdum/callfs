package localfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ebogdum/callfs/v2/metadata"
)

func newModeAdapter(t *testing.T, mode SyncMode) (*LocalFSAdapter, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if nil != err {
		t.Fatalf("eval symlinks: %v", err)
	}
	a, err := NewLocalFSAdapter(root, WithSyncMode(mode))
	if nil != err {
		t.Fatalf("new adapter: %v", err)
	}
	t.Cleanup(func() { a.Close() })
	return a, root
}

func readAll(t *testing.T, a *LocalFSAdapter, p string) string {
	t.Helper()
	rc, err := a.Open(context.Background(), p)
	if nil != err {
		t.Fatalf("open %s: %v", p, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if nil != err {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(data)
}

func TestRenameExclusiveRefusesExistingDestination(t *testing.T) {
	a, root := newModeAdapter(t, SyncFull)
	if err := os.WriteFile(filepath.Join(root, "src"), []byte("new"), 0644); nil != err {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dst"), []byte("old"), 0644); nil != err {
		t.Fatal(err)
	}
	err := renameExclusive(a.root, ".", "src", "dst")
	if errors.Is(err, errExclusiveRenameUnsupported) {
		t.Skip("exclusive rename unsupported on this platform or filesystem")
	}
	if !errors.Is(err, fs.ErrExist) {
		t.Fatalf("expected ErrExist, got %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "dst"))
	if "old" != string(data) {
		t.Fatalf("destination was overwritten: %q", data)
	}
	if _, statErr := os.Stat(filepath.Join(root, "src")); nil != statErr {
		t.Fatalf("source must remain after refusal: %v", statErr)
	}
}

func TestRenameExclusiveMovesIntoFreeName(t *testing.T) {
	a, root := newModeAdapter(t, SyncFull)
	if err := os.MkdirAll(filepath.Join(root, "d"), 0755); nil != err {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "d", "src"), []byte("x"), 0644); nil != err {
		t.Fatal(err)
	}
	err := renameExclusive(a.root, "d", "src", "dst")
	if errors.Is(err, errExclusiveRenameUnsupported) {
		t.Skip("exclusive rename unsupported on this platform or filesystem")
	}
	if nil != err {
		t.Fatalf("rename: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "d", "dst")); nil != statErr {
		t.Fatalf("destination missing: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, "d", "src")); !os.IsNotExist(statErr) {
		t.Fatalf("source should be gone: %v", statErr)
	}
}

func TestCreateExistingReturnsAlreadyExistsAndKeepsContent(t *testing.T) {
	for _, mode := range []SyncMode{SyncFull, SyncBarrier, SyncNone} {
		t.Run(string(mode), func(t *testing.T) {
			a, root := newModeAdapter(t, mode)
			ctx := context.Background()
			if err := a.Create(ctx, "/a/b.txt", strings.NewReader("first"), 5); nil != err {
				t.Fatalf("create: %v", err)
			}
			err := a.Create(ctx, "/a/b.txt", strings.NewReader("second"), 6)
			if !errors.Is(err, metadata.ErrAlreadyExists) {
				t.Fatalf("expected ErrAlreadyExists, got %v", err)
			}
			if "first" != readAll(t, a, "/a/b.txt") {
				t.Fatal("existing content was changed")
			}
			entries, _ := os.ReadDir(filepath.Join(root, "a"))
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), tempFilePrefix) {
					t.Fatalf("temp file left behind: %s", entry.Name())
				}
			}
		})
	}
}

func TestCreateAppliesFinalPermissions(t *testing.T) {
	a, root := newModeAdapter(t, SyncFull)
	if err := a.Create(context.Background(), "/perm.txt", strings.NewReader("x"), 1); nil != err {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "perm.txt"))
	if nil != err {
		t.Fatal(err)
	}
	if 0o600 != info.Mode().Perm() {
		t.Fatalf("file must be private to its owner (0600), got %v", info.Mode().Perm())
	}
}

func TestUpdateReplacesContentInEveryMode(t *testing.T) {
	for _, mode := range []SyncMode{SyncFull, SyncBarrier, SyncNone} {
		t.Run(string(mode), func(t *testing.T) {
			a, _ := newModeAdapter(t, mode)
			ctx := context.Background()
			if err := a.Create(ctx, "/u.txt", strings.NewReader("one"), 3); nil != err {
				t.Fatal(err)
			}
			if err := a.Update(ctx, "/u.txt", bytes.NewReader([]byte("two-two")), 7); nil != err {
				t.Fatal(err)
			}
			if "two-two" != readAll(t, a, "/u.txt") {
				t.Fatal("update did not replace content")
			}
		})
	}
}

func TestConcurrentCreatesSameDirectory(t *testing.T) {
	a, _ := newModeAdapter(t, SyncFull)
	ctx := context.Background()
	const writers = 64
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := fmt.Sprintf("payload-%d", i)
			errs[i] = a.Create(ctx, fmt.Sprintf("/shared/f%d", i), strings.NewReader(body), int64(len(body)))
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if nil != err {
			t.Fatalf("writer %d: %v", i, err)
		}
		if want := fmt.Sprintf("payload-%d", i); want != readAll(t, a, fmt.Sprintf("/shared/f%d", i)) {
			t.Fatalf("writer %d content mismatch", i)
		}
	}
}

func TestConcurrentCreatesSameNameSingleWinner(t *testing.T) {
	a, _ := newModeAdapter(t, SyncFull)
	ctx := context.Background()
	const writers = 16
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = a.Create(ctx, "/race.txt", strings.NewReader(fmt.Sprintf("w%d", i)), 2)
		}()
	}
	wg.Wait()
	winners := 0
	for _, err := range errs {
		switch {
		case nil == err:
			winners++
		case errors.Is(err, metadata.ErrAlreadyExists):
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if 1 != winners {
		t.Fatalf("expected exactly one winner, got %d", winners)
	}
}

func TestParseSyncMode(t *testing.T) {
	cases := map[string]SyncMode{"": SyncFull, "full": SyncFull, " Barrier ": SyncBarrier, "NONE": SyncNone}
	for in, want := range cases {
		got, err := ParseSyncMode(in)
		if nil != err || want != got {
			t.Fatalf("ParseSyncMode(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseSyncMode("always"); nil == err {
		t.Fatal("expected error for unknown mode")
	}
}

func TestNewAdapterRejectsUnknownSyncMode(t *testing.T) {
	if _, err := NewLocalFSAdapter(t.TempDir(), WithSyncMode(SyncMode("bogus"))); nil == err {
		t.Fatal("expected error")
	}
}
