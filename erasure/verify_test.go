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

func TestPlanToleratesInstanceFailuresWithinParity(t *testing.T) {
	manager := NewManager(nil, nil, &config.ErasureConfig{}, "local", nil, "", zap.NewNop())

	cases := []struct {
		name      string
		instances []string
		tolerated int
		faultOK   bool
	}{
		{name: "two servers exceed spare pieces", instances: []string{"local", "peer"}, tolerated: 0, faultOK: false},
		{name: "three servers survive one server loss", instances: []string{"local", "peer", "other"}, tolerated: 1, faultOK: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := manager.Plan(4, 2, tc.instances)
			if 6 != plan.TotalShards || 6 != len(plan.Assignments) {
				t.Fatalf("total shards = %d, assignments = %d, want 6", plan.TotalShards, len(plan.Assignments))
			}
			if 1.5 != plan.OverheadRatio {
				t.Fatalf("overhead ratio = %v, want 1.5", plan.OverheadRatio)
			}
			if tc.tolerated != plan.ToleratedInstanceFailures {
				t.Fatalf("tolerated instance failures = %d, want %d", plan.ToleratedInstanceFailures, tc.tolerated)
			}
			if tc.faultOK != plan.FaultTolerant {
				t.Fatalf("fault tolerant = %v, want %v", plan.FaultTolerant, tc.faultOK)
			}
		})
	}
}

func TestVerifyFlagsCorruptAndMissingShards(t *testing.T) {
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

	data := []byte("split file that loses two pieces")
	info, err := manager.StoreFile(ctx, "split.bin", data, int64(len(data)), nil)
	if err != nil {
		t.Fatalf("store split.bin: %v", err)
	}
	if err := manager.CommitErasureInfo(ctx, info); err != nil {
		t.Fatalf("commit split.bin: %v", err)
	}

	corrupt := info.Shards[0]
	junk := bytes.Repeat([]byte{0xAB}, int(corrupt.Size))
	if err := backend.Delete(ctx, corrupt.Path); err != nil {
		t.Fatalf("remove shard 0: %v", err)
	}
	if err := backend.Create(ctx, corrupt.Path, bytes.NewReader(junk), int64(len(junk))); err != nil {
		t.Fatalf("overwrite shard 0: %v", err)
	}
	if err := backend.Delete(ctx, info.Shards[1].Path); err != nil {
		t.Fatalf("delete shard 1: %v", err)
	}

	report, err := manager.Verify(ctx, "split.bin")
	if err != nil {
		t.Fatalf("verify split.bin: %v", err)
	}
	if !report.Readable || 4 != report.Healthy || 4 != report.Required {
		t.Fatalf("report readable=%v healthy=%d required=%d, want true 4 4", report.Readable, report.Healthy, report.Required)
	}
	if ShardCorrupt != report.Shards[0].State || nil == report.Shards[0].Error {
		t.Fatalf("shard 0 = %+v, want corrupt with error", report.Shards[0])
	}
	if ShardMissing != report.Shards[1].State {
		t.Fatalf("shard 1 state = %s, want missing", report.Shards[1].State)
	}
	if ShardOK != report.Shards[2].State || nil != report.Shards[2].Error {
		t.Fatalf("shard 2 = %+v, want ok without error", report.Shards[2])
	}

	health, ok := manager.LastHealth("split.bin")
	if !ok || HealthCritical != health.Health || 4 != health.HealthyShards || 6 != health.ShardsTotal {
		t.Fatalf("last health = %+v ok=%v, want critical 4 of 6", health, ok)
	}
}
