package core

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/backends/localfs"
	"github.com/ebogdum/callfs/v2/backends/noop"
	"github.com/ebogdum/callfs/v2/jobs"
	"github.com/ebogdum/callfs/v2/locks"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/metadata/sqlite"
)

type countingStore struct {
	metadata.Store
	batcher metadata.Batcher
	mu      sync.Mutex
	single  int
	batches int
}

func (s *countingStore) countSingle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.single++
}

func (s *countingStore) counts() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.single, s.batches
}

func (s *countingStore) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.single, s.batches = 0, 0
}

func (s *countingStore) Create(ctx context.Context, md *metadata.Metadata) error {
	s.countSingle()
	return s.Store.Create(ctx, md)
}

func (s *countingStore) Update(ctx context.Context, md *metadata.Metadata) error {
	s.countSingle()
	return s.Store.Update(ctx, md)
}

func (s *countingStore) UpdateIfMatch(ctx context.Context, md *metadata.Metadata, expectedETag string) error {
	s.countSingle()
	return s.Store.UpdateIfMatch(ctx, md, expectedETag)
}

func (s *countingStore) Delete(ctx context.Context, path string) error {
	s.countSingle()
	return s.Store.Delete(ctx, path)
}

func (s *countingStore) Rename(ctx context.Context, oldPath, newPath string) error {
	s.countSingle()
	return s.Store.Rename(ctx, oldPath, newPath)
}

func (s *countingStore) ApplyBatch(ctx context.Context, ops []metadata.Op) error {
	s.mu.Lock()
	s.batches++
	s.mu.Unlock()
	return s.batcher.ApplyBatch(ctx, ops)
}

func newCountingEngine(t *testing.T) (*Engine, *countingStore) {
	t.Helper()
	inner, err := sqlite.NewSQLiteStore(filepath.Join(resolvedTempDir(t), "meta.db"), zap.NewNop())
	if nil != err {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = inner.Close() })
	store := &countingStore{Store: inner, batcher: inner}

	localBackend, err := localfs.NewLocalFSAdapter(resolvedTempDir(t))
	if nil != err {
		t.Fatalf("localfs: %v", err)
	}
	e := NewEngine(store, localBackend, nil, noop.NewNoopAdapter(), nil,
		locks.NewLocalManager(), "node-1", map[string]string{}, false, "", false, zap.NewNop())
	if err := e.EnsureRootDirectory(context.Background()); nil != err {
		t.Fatalf("root: %v", err)
	}
	t.Cleanup(e.Close)
	return e, store
}

func TestDeleteDirectoryRecursiveCommitsOneBatch(t *testing.T) {
	ctx := context.Background()
	engine, store := newCountingEngine(t)
	createFile(t, engine, "/tree/a.txt", "a")
	createFile(t, engine, "/tree/sub/b.txt", "bb")
	createFile(t, engine, "/tree/sub/deep/c.txt", "ccc")
	store.reset()

	files, dirs, err := engine.DeleteDirectoryRecursive(ctx, "/tree")
	if nil != err {
		t.Fatalf("DeleteDirectoryRecursive: %v", err)
	}
	if 3 != files || 3 != dirs {
		t.Fatalf("deleted files=%d dirs=%d, want 3 and 3", files, dirs)
	}
	if single, batches := store.counts(); 0 != single || 1 != batches {
		t.Fatalf("metadata writes: single=%d batches=%d, want 0 and 1", single, batches)
	}
	for _, p := range []string{"/tree", "/tree/a.txt", "/tree/sub", "/tree/sub/deep", "/tree/sub/deep/c.txt"} {
		if _, err := store.Get(ctx, p); !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("%s still present: %v", p, err)
		}
	}
}

func TestMoveToTrashCommitsOneBatch(t *testing.T) {
	ctx := context.Background()
	engine, store := newCountingEngine(t)
	createFile(t, engine, "/docs/report.txt", "hello")
	trash := NewTrash(engine, allowAllAuthorizer{}, auth.NewAdminSet(nil), jobs.NewRunner(discardPublisher{}, zap.NewNop()), time.Hour, zap.NewNop())
	store.reset()

	item, err := trash.MoveToTrash(ctx, "u1", "/docs/report.txt")
	if nil != err {
		t.Fatalf("MoveToTrash: %v", err)
	}
	if single, batches := store.counts(); 0 != single || 1 != batches {
		t.Fatalf("metadata writes: single=%d batches=%d, want 0 and 1", single, batches)
	}
	if _, err := store.Get(ctx, "/docs/report.txt"); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("original still present: %v", err)
	}
	if _, err := store.Get(ctx, recordPath(item.ID)); nil != err {
		t.Fatalf("trash record missing: %v", err)
	}
	if got := readFile(t, engine, childPath(trashItemDir(item.ID), "report.txt")); "hello" != got {
		t.Fatalf("trashed content = %q", got)
	}

	store.reset()
	if err := trash.Delete(ctx, "u1", item.ID); nil != err {
		t.Fatalf("Delete: %v", err)
	}
	if single, batches := store.counts(); 0 != single || 1 != batches {
		t.Fatalf("purge metadata writes: single=%d batches=%d, want 0 and 1", single, batches)
	}
	for _, p := range []string{trashItemDir(item.ID), recordPath(item.ID)} {
		if _, err := store.Get(ctx, p); !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("%s still present after purge: %v", p, err)
		}
	}
}
