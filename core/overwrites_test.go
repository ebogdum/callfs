package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/backends/localfs"
	"github.com/ebogdum/callfs/v2/backends/noop"
	"github.com/ebogdum/callfs/v2/locks"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/metadata/sqlite"
)

type failingUpdateStore struct {
	metadata.Store
	fail atomic.Bool
}

func (s *failingUpdateStore) UpdateIfMatch(ctx context.Context, md *metadata.Metadata, expectedETag string) error {
	if s.fail.Load() {
		return errors.New("index unavailable")
	}
	return s.Store.UpdateIfMatch(ctx, md, expectedETag)
}

type overwriteFixture struct {
	root  string
	local *localfs.LocalFSAdapter
	e     *Engine
}

func newOverwriteFixture(t *testing.T, root string, store metadata.Store) *overwriteFixture {
	t.Helper()
	local, err := localfs.NewLocalFSAdapter(root)
	if nil != err {
		t.Fatalf("localfs: %v", err)
	}
	e := NewEngine(store, local, nil, noop.NewNoopAdapter(), nil,
		locks.NewLocalManager(), "node-1", map[string]string{}, false, "", false, zap.NewNop())
	if err := e.EnsureRootDirectory(context.Background()); nil != err {
		t.Fatalf("root: %v", err)
	}
	t.Cleanup(func() {
		e.Close()
		_ = local.Close()
	})
	return &overwriteFixture{root: root, local: local, e: e}
}

func newOverwriteStore(t *testing.T) metadata.Store {
	t.Helper()
	store, err := sqlite.NewSQLiteStore(filepath.Join(resolvedTempDir(t), "meta.db"), zap.NewNop())
	if nil != err {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func putContent(t *testing.T, e *Engine, path, content string) (*metadata.Metadata, error) {
	t.Helper()
	md := &metadata.Metadata{Name: filepath.Base(path), Type: "file", Mode: "0644", Owner: "u1", BackendType: "localfs"}
	committed, _, err := e.PutFile(context.Background(), path, strings.NewReader(content), int64(len(content)), md, "", false)
	return committed, err
}

func mustPut(t *testing.T, e *Engine, path, content string) *metadata.Metadata {
	t.Helper()
	committed, err := putContent(t, e, path, content)
	if nil != err {
		t.Fatalf("put %s: %v", path, err)
	}
	return committed
}

func diskContent(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(path, "/"))))
	if nil != err {
		t.Fatalf("read disk %s: %v", path, err)
	}
	return string(data)
}

func hiddenEntries(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if nil != err {
		t.Fatalf("read root: %v", err)
	}
	var names []string
	for _, entry := range entries {
		if localfs.IsOverwriteName(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	return names
}

func waitNoHidden(t *testing.T, root string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for names := hiddenEntries(t, root); 0 < len(names); names = hiddenEntries(t, root) {
		if time.Now().After(deadline) {
			t.Fatalf("hidden overwrite files left: %v", names)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func writeHidden(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); nil != err {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestOverwriteAsideReplacesContent(t *testing.T) {
	f := newOverwriteFixture(t, resolvedTempDir(t), newOverwriteStore(t))
	mustPut(t, f.e, "/f.txt", "old bytes")
	mustPut(t, f.e, "/f.txt", "new bytes")

	if got := diskContent(t, f.root, "/f.txt"); "new bytes" != got {
		t.Fatalf("disk = %q, want new bytes", got)
	}
	if got := readFile(t, f.e, "/f.txt"); "new bytes" != got {
		t.Fatalf("read = %q, want new bytes", got)
	}
	waitNoHidden(t, f.root)
}

func TestOverwriteAsideRestoresOnIndexFailure(t *testing.T) {
	store := &failingUpdateStore{Store: newOverwriteStore(t)}
	f := newOverwriteFixture(t, resolvedTempDir(t), store)
	mustPut(t, f.e, "/f.txt", "old bytes")

	store.fail.Store(true)
	if _, err := putContent(t, f.e, "/f.txt", "new bytes"); nil == err {
		t.Fatal("overwrite succeeded while the index failed")
	}
	store.fail.Store(false)

	if got := diskContent(t, f.root, "/f.txt"); "old bytes" != got {
		t.Fatalf("disk = %q, want old bytes", got)
	}
	if got := readFile(t, f.e, "/f.txt"); "old bytes" != got {
		t.Fatalf("read = %q, want old bytes", got)
	}
	if names := hiddenEntries(t, f.root); 0 != len(names) {
		t.Fatalf("hidden overwrite files left: %v", names)
	}
}

func TestRecoverOverwritesRestoresUncommittedOverwrite(t *testing.T) {
	f := newOverwriteFixture(t, resolvedTempDir(t), newOverwriteStore(t))
	md := mustPut(t, f.e, "/f.txt", "old bytes")
	oldCopy := ".f.txt.callfs-old-" + md.ETag
	if err := os.Rename(filepath.Join(f.root, "f.txt"), filepath.Join(f.root, oldCopy)); nil != err {
		t.Fatalf("rename: %v", err)
	}
	writeHidden(t, f.root, "f.txt", "new bytes")

	if err := f.e.RecoverOverwrites(context.Background()); nil != err {
		t.Fatalf("recover: %v", err)
	}
	if got := diskContent(t, f.root, "/f.txt"); "old bytes" != got {
		t.Fatalf("disk = %q, want old bytes", got)
	}
	if names := hiddenEntries(t, f.root); 0 != len(names) {
		t.Fatalf("hidden overwrite files left: %v", names)
	}
}

func TestRecoverOverwritesDropsCommittedOldCopy(t *testing.T) {
	f := newOverwriteFixture(t, resolvedTempDir(t), newOverwriteStore(t))
	old := mustPut(t, f.e, "/f.txt", "old bytes")
	mustPut(t, f.e, "/f.txt", "new bytes")
	waitNoHidden(t, f.root)
	writeHidden(t, f.root, ".f.txt.callfs-old-"+old.ETag, "old bytes")

	if err := f.e.RecoverOverwrites(context.Background()); nil != err {
		t.Fatalf("recover: %v", err)
	}
	if got := diskContent(t, f.root, "/f.txt"); "new bytes" != got {
		t.Fatalf("disk = %q, want new bytes", got)
	}
	if names := hiddenEntries(t, f.root); 0 != len(names) {
		t.Fatalf("hidden overwrite files left: %v", names)
	}
}

func TestRecoverOverwritesRemovesLeftoverTemps(t *testing.T) {
	f := newOverwriteFixture(t, resolvedTempDir(t), newOverwriteStore(t))
	mustPut(t, f.e, "/f.txt", "bytes")
	writeHidden(t, f.root, ".f.txt.callfs-new-0123456789abcdef", "partial")
	writeHidden(t, f.root, ".gone.txt.callfs-new-fedcba9876543210", "partial")

	if err := f.e.RecoverOverwrites(context.Background()); nil != err {
		t.Fatalf("recover: %v", err)
	}
	if names := hiddenEntries(t, f.root); 0 != len(names) {
		t.Fatalf("hidden overwrite files left: %v", names)
	}
	if got := diskContent(t, f.root, "/f.txt"); "bytes" != got {
		t.Fatalf("disk = %q, want bytes", got)
	}
}

func TestOverwriteHiddenNamesNeverListed(t *testing.T) {
	f := newOverwriteFixture(t, resolvedTempDir(t), newOverwriteStore(t))
	mustPut(t, f.e, "/f.txt", "bytes")
	writeHidden(t, f.root, ".f.txt.callfs-old-abc123", "old")
	writeHidden(t, f.root, ".f.txt.callfs-new-0123456789abcdef", "new")

	children, err := f.local.ListDirectory(context.Background(), "/")
	if nil != err {
		t.Fatalf("list: %v", err)
	}
	for _, child := range children {
		if localfs.IsOverwriteName(child.Name) {
			t.Fatalf("hidden overwrite file listed: %s", child.Name)
		}
	}
	if 0 == len(children) {
		t.Fatal("listing lost the real file")
	}
}
