package erasure

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/backends/localfs"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/metadata"
)

func TestValidateShardPath(t *testing.T) {
	for _, valid := range []string{".callfs/shards", "shards", "a/b/c"} {
		if got, err := ValidateShardPath(valid); nil != err || valid != got {
			t.Fatalf("%q = %q, %v", valid, got, err)
		}
	}
	for _, invalid := range []string{"", "/abs/shards", "../shards", "a/../b", "a//b", "a/b/", "./a", ".", "a\\b"} {
		if _, err := ValidateShardPath(invalid); nil == err {
			t.Fatalf("%q accepted", invalid)
		}
	}
}

func TestAutoSplitThreshold(t *testing.T) {
	cases := []struct {
		name      string
		threshold config.ByteSize
		size      int64
		want      bool
	}{
		{"disabled", 0, 1 << 30, false},
		{"below", 100, 99, false},
		{"equal", 100, 100, true},
		{"above", 100, 101, true},
		{"unknown size", 100, -1, false},
	}
	for _, tc := range cases {
		manager := NewManager(nil, nil, &config.ErasureConfig{MinFileSize: tc.threshold}, "local", nil, "", zap.NewNop())
		if got := manager.AutoSplit(tc.size); tc.want != got {
			t.Fatalf("%s: AutoSplit(%d) = %v", tc.name, tc.size, got)
		}
	}
}

func TestShardsUseConfiguredPath(t *testing.T) {
	ctx := context.Background()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if nil != err {
		t.Fatal(err)
	}
	backend, err := localfs.NewLocalFSAdapter(root)
	if nil != err {
		t.Fatal(err)
	}
	store := &memErasureStore{infos: map[string]*metadata.ErasureFileInfo{}}
	cfg := &config.ErasureConfig{DataShards: 4, ParityShards: 2, ShardBackend: "localfs", ShardPath: "custom/pieces"}
	manager := NewManager(store, backend, cfg, "local", nil, "", zap.NewNop())

	data := []byte("shard path usage check")
	info, err := manager.StoreFile(ctx, "p.bin", data, int64(len(data)), nil)
	if nil != err {
		t.Fatalf("store: %v", err)
	}
	if err := manager.CommitErasureInfo(ctx, info); nil != err {
		t.Fatalf("commit: %v", err)
	}
	for _, shard := range info.Shards {
		if !strings.HasPrefix(shard.Path, "custom/pieces/") {
			t.Fatalf("shard path %q outside configured folder", shard.Path)
		}
	}
	got, err := manager.RetrieveFile(ctx, "p.bin")
	if nil != err || string(data) != string(got) {
		t.Fatalf("retrieve = %q, %v", got, err)
	}
	if prefix := manager.extractShardPrefix(info.Shards[0].Path); strings.Contains(prefix, "custom") || "" == prefix {
		t.Fatalf("prefix = %q", prefix)
	}
	if found, err := manager.DeleteFileShards(ctx, "p.bin"); nil != err || !found {
		t.Fatalf("delete = %v, %v", found, err)
	}
	if _, err := backend.Stat(ctx, info.Shards[0].Path); nil == err {
		t.Fatal("shard still present after delete")
	}
}

func TestStoreRejectsInvalidShardPath(t *testing.T) {
	cfg := &config.ErasureConfig{DataShards: 4, ParityShards: 2, ShardBackend: "localfs", ShardPath: "../escape"}
	manager := NewManager(&memErasureStore{infos: map[string]*metadata.ErasureFileInfo{}}, nil, cfg, "local", nil, "", zap.NewNop())
	data := []byte("data")
	if _, err := manager.StoreFile(context.Background(), "x.bin", data, int64(len(data)), nil); nil == err {
		t.Fatal("invalid shard path accepted")
	}
}
