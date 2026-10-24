package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/ebogdum/callfs/v2/metadata"
)

func batchFileMeta(path string) *metadata.Metadata {
	return &metadata.Metadata{
		Name: pathBase(path), Path: path, Type: "file", Mode: "0644",
		Owner: "u1", BackendType: "localfs", ETag: "e1",
	}
}

func TestApplyBatchSuccess(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	info := &metadata.ErasureFileInfo{
		DataShards: 2, ParityShards: 1, ShardSize: 10, OriginalSize: 20,
		Shards: []metadata.ErasureShardInfo{
			{Index: 0, InstanceID: "i1", BackendType: "localfs", Path: "/s0", Size: 10, Checksum: "c0"},
		},
	}
	dirMd := &metadata.Metadata{
		Name: "d", Path: "/d", Type: "directory", Mode: "0755",
		Owner: "u1", BackendType: "localfs",
	}
	ops := []metadata.Op{
		{Kind: metadata.OpCreate, Metadata: dirMd},
		{Kind: metadata.OpCreate, Metadata: batchFileMeta("/d/a.txt")},
		{Kind: metadata.OpRename, Path: "/d/a.txt", NewPath: "/d/b.txt"},
		{Kind: metadata.OpCreateErasureInfo, Path: "/d/b.txt", ErasureInfo: info},
		{Kind: metadata.OpCreateErasureInfo, Path: "/d/gone.txt", ErasureInfo: info},
		{Kind: metadata.OpDeleteErasureInfo, Path: "/d/gone.txt"},
	}
	if err := s.ApplyBatch(ctx, ops); nil != err {
		t.Fatalf("apply batch: %v", err)
	}

	if _, err := s.Get(ctx, "/d"); nil != err {
		t.Fatalf("dir should exist: %v", err)
	}
	if _, err := s.Get(ctx, "/d/a.txt"); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("old path should be gone, got %v", err)
	}
	if _, err := s.Get(ctx, "/d/b.txt"); nil != err {
		t.Fatalf("renamed file should exist: %v", err)
	}
	got, err := s.GetErasureInfo(ctx, "/d/b.txt")
	if nil != err {
		t.Fatalf("erasure info should exist: %v", err)
	}
	if 1 != len(got.Shards) {
		t.Fatalf("expected 1 shard, got %d", len(got.Shards))
	}
	if _, err := s.GetErasureInfo(ctx, "/d/gone.txt"); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("deleted erasure info should be gone, got %v", err)
	}
}

func TestApplyBatchRollsBackOnCreateConflict(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkFile(t, s, "/exists.txt")

	ops := []metadata.Op{
		{Kind: metadata.OpCreate, Metadata: batchFileMeta("/n1.txt")},
		{Kind: metadata.OpCreate, Metadata: batchFileMeta("/n2.txt")},
		{Kind: metadata.OpCreate, Metadata: batchFileMeta("/exists.txt")},
	}
	err := s.ApplyBatch(ctx, ops)
	if !errors.Is(err, metadata.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	for _, p := range []string{"/n1.txt", "/n2.txt"} {
		if _, err := s.Get(ctx, p); !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("%s should be rolled back, got %v", p, err)
		}
	}
	if _, err := s.Get(ctx, "/exists.txt"); nil != err {
		t.Fatalf("existing row should be unchanged: %v", err)
	}
}

func TestApplyBatchUpdateIfMatchPreconditionRollsBack(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkFile(t, s, "/keep.txt")
	target := batchFileMeta("/target.txt")
	if err := s.Create(ctx, target); nil != err {
		t.Fatalf("create target: %v", err)
	}

	update := batchFileMeta("/target.txt")
	update.ETag = "e2"
	ops := []metadata.Op{
		{Kind: metadata.OpDelete, Path: "/keep.txt"},
		{Kind: metadata.OpUpdateIfMatch, Metadata: update, ExpectedETag: "wrong"},
	}
	err := s.ApplyBatch(ctx, ops)
	if !errors.Is(err, metadata.ErrPrecondition) {
		t.Fatalf("expected ErrPrecondition, got %v", err)
	}

	if _, err := s.Get(ctx, "/keep.txt"); nil != err {
		t.Fatalf("delete should be rolled back: %v", err)
	}
	got, err := s.Get(ctx, "/target.txt")
	if nil != err {
		t.Fatalf("target get: %v", err)
	}
	if "e1" != got.ETag {
		t.Fatalf("target etag should be unchanged, got %q", got.ETag)
	}
}
