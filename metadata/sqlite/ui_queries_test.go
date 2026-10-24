package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ebogdum/callfs/metadata"
)

func TestListSingleUseLinksPagesByOwner(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	mkFile(t, s, "/a.txt")
	specs := []struct {
		token, owner string
		expires      time.Time
	}{
		{"stale", "alice", now.Add(-time.Minute)},
		{"a1", "alice", now.Add(time.Hour)},
		{"b1", "bob", now.Add(time.Hour)},
		{"a2", "alice", now.Add(time.Hour)},
		{"a3", "alice", now.Add(time.Hour)},
	}
	for _, spec := range specs {
		if err := s.CreateSingleUseLink(ctx, &metadata.SingleUseLink{
			Token: spec.token, FilePath: "/a.txt", Owner: spec.owner, Status: "active", ExpiresAt: spec.expires, HMACSignature: "sig",
		}); nil != err {
			t.Fatalf("create %s: %v", spec.token, err)
		}
	}

	page, next, err := s.ListSingleUseLinks(ctx, metadata.LinkFilter{Status: "active", CreatedBy: "alice", Now: now, Limit: 2})
	if nil != err {
		t.Fatalf("first page: %v", err)
	}
	if 2 != len(page) || "" == next {
		t.Fatalf("first page: len=%d next=%q", len(page), next)
	}

	rest, next, err := s.ListSingleUseLinks(ctx, metadata.LinkFilter{Status: "active", CreatedBy: "alice", Now: now, Limit: 2, Cursor: next})
	if nil != err {
		t.Fatalf("second page: %v", err)
	}
	if 1 != len(rest) || "" != next || "alice" != rest[0].Owner {
		t.Fatalf("second page: len=%d next=%q", len(rest), next)
	}

	expired, _, err := s.ListSingleUseLinks(ctx, metadata.LinkFilter{Status: "expired", Now: now, Limit: 10})
	if nil != err {
		t.Fatalf("expired list: %v", err)
	}
	if 1 != len(expired) || "alice" != expired[0].Owner {
		t.Fatalf("expired list: %+v", expired)
	}

	if _, _, err := s.ListSingleUseLinks(ctx, metadata.LinkFilter{Status: "bogus", Limit: 10}); !errors.Is(err, metadata.ErrInvalidQuery) {
		t.Fatalf("unknown status: %v", err)
	}
	if _, _, err := s.ListSingleUseLinks(ctx, metadata.LinkFilter{Cursor: "x", Limit: 10}); !errors.Is(err, metadata.ErrInvalidQuery) {
		t.Fatalf("bad cursor: %v", err)
	}
}

func TestGetAndRevokeSingleUseLink(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkFile(t, s, "/a.txt")
	link := &metadata.SingleUseLink{
		Token: "t", FilePath: "/a.txt", Owner: "alice", Status: "active", ExpiresAt: time.Now().UTC().Add(time.Hour), HMACSignature: "sig",
	}
	if err := s.CreateSingleUseLink(ctx, link); nil != err {
		t.Fatalf("create: %v", err)
	}

	if err := s.RevokeSingleUseLink(ctx, link.ID); nil != err {
		t.Fatalf("revoke: %v", err)
	}
	got, err := s.GetSingleUseLinkByID(ctx, link.ID)
	if nil != err || "revoked" != got.Status || "alice" != got.Owner {
		t.Fatalf("get after revoke: link=%+v err=%v", got, err)
	}
	if err := s.RevokeSingleUseLink(ctx, link.ID); !errors.Is(err, metadata.ErrPrecondition) {
		t.Fatalf("revoke twice: %v", err)
	}
	if err := s.RevokeSingleUseLink(ctx, link.ID+100); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("revoke missing: %v", err)
	}
	if _, err := s.GetSingleUseLinkByID(ctx, link.ID+100); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}
}

func TestOwnerStatsAndEmptyCounts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkFile(t, s, "/a.txt")
	if err := s.Create(ctx, &metadata.Metadata{
		Name: "b.bin", Path: "/b.bin", Type: "file", Mode: "0644", Owner: "u2", Size: 10, BackendType: "localfs",
	}); nil != err {
		t.Fatalf("create b.bin: %v", err)
	}
	mkDir(t, s, "/d")
	if err := s.Create(ctx, &metadata.Metadata{
		Name: "e", Path: "/e", Type: "directory", Mode: "0755", Owner: "", BackendType: "localfs",
	}); nil != err {
		t.Fatalf("create e: %v", err)
	}

	stats, next, err := s.OwnerStats(ctx, "", 10)
	if nil != err {
		t.Fatalf("owner stats: %v", err)
	}
	if 2 != len(stats) || "u1" != stats[0].Owner || 1 != stats[0].Files || 0 != stats[0].Bytes ||
		"u2" != stats[1].Owner || 1 != stats[1].Files || 10 != stats[1].Bytes || "" != next {
		t.Fatalf("owner stats: %+v next=%q", stats, next)
	}
	after, _, err := s.OwnerStats(ctx, "u1", 10)
	if nil != err || 1 != len(after) || "u2" != after[0].Owner {
		t.Fatalf("owner stats after cursor: %+v err=%v", after, err)
	}

	etags, err := s.CountEmptyETags(ctx)
	if nil != err || 2 != etags {
		t.Fatalf("empty etags: %d err=%v", etags, err)
	}
	owners, err := s.CountEmptyOwners(ctx)
	if nil != err || 1 != owners {
		t.Fatalf("empty owners: %d err=%v", owners, err)
	}
}

func TestSearchMetadataByName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkFile(t, s, "/app.log")
	mkFile(t, s, "/notes.txt")

	result, err := s.SearchMetadata(ctx, metadata.SearchQuery{Q: "*.log", Limit: 10})
	if nil != err {
		t.Fatalf("search: %v", err)
	}
	if 1 != len(result.Items) || "/app.log" != result.Items[0].Path {
		t.Fatalf("search items: %+v", result.Items)
	}
}

func TestListErasureInfosPages(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, p := range []string{"/a.bin", "/b.bin", "/c.bin"} {
		info := &metadata.ErasureFileInfo{
			FilePath: p, OriginalSize: 4, DataShards: 1, ParityShards: 1, ShardSize: 4,
			Shards: []metadata.ErasureShardInfo{{Index: 0, InstanceID: "i1", BackendType: "localfs", Path: p + ".0", Size: 4, Checksum: "c"}},
		}
		if err := s.CreateErasureInfo(ctx, p, info); nil != err {
			t.Fatalf("create %s: %v", p, err)
		}
	}

	infos, next, err := s.ListErasureInfos(ctx, "/", "", 2)
	if nil != err {
		t.Fatalf("first page: %v", err)
	}
	if 2 != len(infos) || "/b.bin" != next || 1 != len(infos[0].Shards) || "/a.bin" != infos[0].FilePath {
		t.Fatalf("first page: len=%d next=%q", len(infos), next)
	}

	rest, next, err := s.ListErasureInfos(ctx, "/", next, 2)
	if nil != err {
		t.Fatalf("second page: %v", err)
	}
	if 1 != len(rest) || "" != next || "/c.bin" != rest[0].FilePath || 1 != len(rest[0].Shards) {
		t.Fatalf("second page: len=%d next=%q", len(rest), next)
	}
}
