package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, zap.NewNop())
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func mkFile(t *testing.T, s *SQLiteStore, path string) {
	t.Helper()
	if err := s.Create(context.Background(), &metadata.Metadata{
		Name: filepath.Base(path), Path: path, Type: "file", Mode: "0644",
		Owner: "u1", BackendType: "localfs",
	}); err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
}

func mkDir(t *testing.T, s *SQLiteStore, path string) {
	t.Helper()
	if err := s.Create(context.Background(), &metadata.Metadata{
		Name: filepath.Base(path), Path: path, Type: "directory", Mode: "0755",
		Owner: "u1", BackendType: "localfs",
	}); err != nil {
		t.Fatalf("create dir %s: %v", path, err)
	}
}

func TestRenameFile(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkFile(t, s, "/a.txt")

	if err := s.Rename(ctx, "/a.txt", "/b.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	if _, err := s.Get(ctx, "/a.txt"); err != metadata.ErrNotFound {
		t.Fatalf("old path should be gone, got %v", err)
	}
	md, err := s.Get(ctx, "/b.txt")
	if err != nil {
		t.Fatalf("new path get: %v", err)
	}
	if md.Name != "b.txt" {
		t.Fatalf("expected name b.txt, got %q", md.Name)
	}
}

func TestRenameMissingSource(t *testing.T) {
	s := newTestStore(t)
	if err := s.Rename(context.Background(), "/nope.txt", "/x.txt"); err != metadata.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRenameDestinationExists(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkFile(t, s, "/a.txt")
	mkFile(t, s, "/b.txt")
	if err := s.Rename(ctx, "/a.txt", "/b.txt"); err != metadata.ErrAlreadyExists {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestRenameDirectorySubtree(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkDir(t, s, "/old")
	mkDir(t, s, "/old/sub")
	mkFile(t, s, "/old/f1.txt")
	mkFile(t, s, "/old/sub/f2.txt")

	if err := s.Rename(ctx, "/old", "/new"); err != nil {
		t.Fatalf("rename dir: %v", err)
	}

	for _, p := range []string{"/old", "/old/sub", "/old/f1.txt", "/old/sub/f2.txt"} {
		if _, err := s.Get(ctx, p); err != metadata.ErrNotFound {
			t.Fatalf("old path %s should be gone, got %v", p, err)
		}
	}
	for _, p := range []string{"/new", "/new/sub", "/new/f1.txt", "/new/sub/f2.txt"} {
		if _, err := s.Get(ctx, p); err != nil {
			t.Fatalf("new path %s should exist, got %v", p, err)
		}
	}
}

func TestRenameDirectoryDoesNotTouchSiblings(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkDir(t, s, "/old")
	mkFile(t, s, "/old/f.txt")
	mkDir(t, s, "/older") // shares a prefix but is not a child
	mkFile(t, s, "/older/keep.txt")

	if err := s.Rename(ctx, "/old", "/new"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := s.Get(ctx, "/older/keep.txt"); err != nil {
		t.Fatalf("sibling /older/keep.txt must be untouched, got %v", err)
	}
}
