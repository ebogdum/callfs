package erasure

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/backends/localfs"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/metadata"
)

type memErasureStore struct {
	infos map[string]*metadata.ErasureFileInfo
}

func (s *memErasureStore) CreateErasureInfo(_ context.Context, filePath string, info *metadata.ErasureFileInfo) error {
	s.infos[filePath] = info
	return nil
}

func (s *memErasureStore) GetErasureInfo(_ context.Context, filePath string) (*metadata.ErasureFileInfo, error) {
	info, ok := s.infos[filePath]
	if !ok {
		return nil, metadata.ErrNotFound
	}
	return info, nil
}

func (s *memErasureStore) DeleteErasureInfo(_ context.Context, filePath string) error {
	delete(s.infos, filePath)
	return nil
}

func TestIdenticalFilesDoNotShareShards(t *testing.T) {
	ctx := context.Background()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	backend, err := localfs.NewLocalFSAdapter(root)
	if err != nil {
		t.Fatal(err)
	}
	store := &memErasureStore{infos: map[string]*metadata.ErasureFileInfo{}}
	cfg := &config.ErasureConfig{DataShards: 4, ParityShards: 2, ShardBackend: "localfs", ShardPath: ".callfs/shards"}
	manager := NewManager(store, backend, cfg, "local", nil, "", zap.NewNop())

	data := []byte("identical content stored under two paths")
	first, err := manager.StoreFile(ctx, "a.bin", data, int64(len(data)), nil)
	if err != nil {
		t.Fatalf("store a.bin: %v", err)
	}
	second, err := manager.StoreFile(ctx, "b.bin", data, int64(len(data)), nil)
	if err != nil {
		t.Fatalf("store b.bin: %v", err)
	}
	if err := manager.CommitErasureInfo(ctx, first); err != nil {
		t.Fatalf("commit a.bin: %v", err)
	}
	if err := manager.CommitErasureInfo(ctx, second); err != nil {
		t.Fatalf("commit b.bin: %v", err)
	}

	for i := range first.Shards {
		if first.Shards[i].Path == second.Shards[i].Path {
			t.Fatalf("shard %d shares path %s between identical files", i, first.Shards[i].Path)
		}
	}

	if err := manager.DeleteFile(ctx, "a.bin"); err != nil {
		t.Fatalf("delete a.bin: %v", err)
	}

	got, err := manager.RetrieveFile(ctx, "b.bin")
	if err != nil {
		t.Fatalf("retrieve b.bin after deleting a.bin: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("retrieved data does not match after deleting identical file")
	}
}
