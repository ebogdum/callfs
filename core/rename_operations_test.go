package core

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/backends/localfs"
	"github.com/ebogdum/callfs/backends/noop"
	"github.com/ebogdum/callfs/locks"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/metadata/sqlite"
)

// resolvedTempDir returns a temp directory with symlinks resolved, so that
// SafeJoin's EvalSymlinks check matches the root (macOS /var -> /private/var).
func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}
	return dir
}

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	dbPath := filepath.Join(resolvedTempDir(t), "meta.db")
	store, err := sqlite.NewSQLiteStore(dbPath, zap.NewNop())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	localBackend, err := localfs.NewLocalFSAdapter(resolvedTempDir(t))
	if err != nil {
		t.Fatalf("localfs: %v", err)
	}
	// A second localfs adapter stands in as the "s3" backend so cross-backend
	// relocation can be exercised without a live object store.
	s3Backend, err := localfs.NewLocalFSAdapter(resolvedTempDir(t))
	if err != nil {
		t.Fatalf("s3 stand-in: %v", err)
	}

	e := NewEngine(store, localBackend, s3Backend, noop.NewNoopAdapter(), nil,
		locks.NewLocalManager(), "node-1", map[string]string{}, false, "", false, zap.NewNop())
	if err := e.EnsureRootDirectory(context.Background()); err != nil {
		t.Fatalf("root: %v", err)
	}
	t.Cleanup(e.Close)
	return e
}

func createFile(t *testing.T, e *Engine, path, content string) {
	t.Helper()
	md := &metadata.Metadata{
		Name: filepath.Base(path), Type: "file", Mode: "0644",
		Owner: "u1", BackendType: "localfs",
	}
	if err := e.CreateFile(context.Background(), path, strings.NewReader(content), int64(len(content)), md); err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
}

func readFile(t *testing.T, e *Engine, path string) string {
	t.Helper()
	rc, err := e.GetFile(context.Background(), path)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	return string(data)
}

func TestEngineRenameFile(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()
	createFile(t, e, "/dir/a.txt", "hello")

	if err := e.Rename(ctx, "/dir/a.txt", "b.txt"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := e.GetMetadata(ctx, "/dir/a.txt"); err != metadata.ErrNotFound {
		t.Fatalf("old gone? got %v", err)
	}
	if got := readFile(t, e, "/dir/b.txt"); got != "hello" {
		t.Fatalf("content = %q", got)
	}
}

func TestEngineRenameRejectsSlash(t *testing.T) {
	e := newTestEngine(t)
	createFile(t, e, "/a.txt", "x")
	err := e.Rename(context.Background(), "/a.txt", "sub/b.txt")
	if _, ok := err.(*ErrInvalidRename); !ok {
		t.Fatalf("expected ErrInvalidRename, got %v", err)
	}
}

func TestEngineMoveFileToFolder(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()
	createFile(t, e, "/a.txt", "data")

	if err := e.Move(ctx, "/a.txt", "/sub/a.txt", MoveOptions{CreateParents: true}); err != nil {
		t.Fatalf("move: %v", err)
	}
	if got := readFile(t, e, "/sub/a.txt"); got != "data" {
		t.Fatalf("content = %q", got)
	}
}

func TestEngineMoveMissingParentFails(t *testing.T) {
	e := newTestEngine(t)
	createFile(t, e, "/a.txt", "x")
	err := e.Move(context.Background(), "/a.txt", "/nope/a.txt", MoveOptions{})
	if _, ok := err.(*ErrInvalidRename); !ok {
		t.Fatalf("expected ErrInvalidRename for missing parent, got %v", err)
	}
}

func TestEngineMoveNoClobber(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()
	createFile(t, e, "/a.txt", "aaa")
	createFile(t, e, "/b.txt", "bbb")

	if err := e.Move(ctx, "/a.txt", "/b.txt", MoveOptions{}); err != metadata.ErrAlreadyExists {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
	if err := e.Move(ctx, "/a.txt", "/b.txt", MoveOptions{Overwrite: true}); err != nil {
		t.Fatalf("overwrite move: %v", err)
	}
	if got := readFile(t, e, "/b.txt"); got != "aaa" {
		t.Fatalf("after overwrite content = %q", got)
	}
}

func TestEngineMoveDirectory(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()
	createFile(t, e, "/old/sub/f.txt", "deep")

	if err := e.Move(ctx, "/old", "/new", MoveOptions{}); err != nil {
		t.Fatalf("move dir: %v", err)
	}
	if got := readFile(t, e, "/new/sub/f.txt"); got != "deep" {
		t.Fatalf("content = %q", got)
	}
	if _, err := e.GetMetadata(ctx, "/old/sub/f.txt"); err != metadata.ErrNotFound {
		t.Fatalf("old subtree should be gone, got %v", err)
	}
}

func TestEngineMoveIntoItself(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()
	createFile(t, e, "/d/f.txt", "x")
	err := e.Move(ctx, "/d", "/d/sub", MoveOptions{CreateParents: true})
	if _, ok := err.(*ErrInvalidRename); !ok {
		t.Fatalf("expected ErrInvalidRename, got %v", err)
	}
}

func TestEngineCrossBackendMove(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()
	createFile(t, e, "/a.txt", "payload")

	// Relocate to the "s3" backend (a second localfs adapter under the hood).
	if err := e.Move(ctx, "/a.txt", "/a.txt", MoveOptions{Backend: "s3"}); err != nil {
		t.Fatalf("cross-backend move: %v", err)
	}
	md, err := e.GetMetadata(ctx, "/a.txt")
	if err != nil {
		t.Fatalf("get after relocation: %v", err)
	}
	if md.BackendType != "s3" {
		t.Fatalf("expected backend s3, got %q", md.BackendType)
	}
	if got := readFile(t, e, "/a.txt"); got != "payload" {
		t.Fatalf("content after relocation = %q", got)
	}
}

func TestEngineErasureRetierRejected(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()
	createFile(t, e, "/a.txt", "x")
	err := e.Move(ctx, "/a.txt", "/b.txt", MoveOptions{Backend: "erasure"})
	if _, ok := err.(*ErrUnsupportedMove); !ok {
		t.Fatalf("expected ErrUnsupportedMove, got %v", err)
	}
}
