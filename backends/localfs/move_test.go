package localfs

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ebogdum/callfs/metadata"
)

func newTestAdapter(t *testing.T) (*LocalFSAdapter, string) {
	t.Helper()
	// Resolve symlinks so SafeJoin's EvalSymlinks check matches the root
	// (macOS /var -> /private/var, etc.).
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	a, err := NewLocalFSAdapter(root)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	return a, root
}

func TestMoveFile(t *testing.T) {
	a, _ := newTestAdapter(t)
	ctx := context.Background()
	if err := a.Create(ctx, "a.txt", bytes.NewReader([]byte("hello")), 5); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := a.Move(ctx, "a.txt", "sub/b.txt"); err != nil {
		t.Fatalf("move: %v", err)
	}

	if _, err := a.Open(ctx, "a.txt"); err != metadata.ErrNotFound {
		t.Fatalf("source should be gone, got %v", err)
	}
	rc, err := a.Open(ctx, "sub/b.txt")
	if err != nil {
		t.Fatalf("open dest: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "hello" {
		t.Fatalf("expected hello, got %q", data)
	}
}

func TestMoveDirectorySubtree(t *testing.T) {
	a, root := newTestAdapter(t)
	ctx := context.Background()
	if err := os.MkdirAll(filepath.Join(root, "old", "sub"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "old", "sub", "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := a.Move(ctx, "old", "new"); err != nil {
		t.Fatalf("move dir: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "old")); !os.IsNotExist(err) {
		t.Fatalf("old dir should be gone, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "new", "sub", "f.txt")); err != nil {
		t.Fatalf("moved file should exist: %v", err)
	}
}

func TestMoveMissingSource(t *testing.T) {
	a, _ := newTestAdapter(t)
	if err := a.Move(context.Background(), "nope.txt", "x.txt"); err != metadata.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
