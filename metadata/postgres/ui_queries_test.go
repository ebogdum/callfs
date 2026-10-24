//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/metadata"
)

const uiTestDSNEnv = "CALLFS_TEST_POSTGRES_DSN"

func newUITestStore(t *testing.T) *PostgresStore {
	t.Helper()
	dsn := os.Getenv(uiTestDSNEnv)
	if "" == dsn {
		t.Skipf("%s not set", uiTestDSNEnv)
	}
	store, err := NewPostgresStoreWithPool(dsn, 25, 25, 5*time.Minute, zap.NewNop())
	if nil != err {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); nil != err {
			t.Errorf("close store: %v", err)
		}
	})
	return store
}

func TestUIQueries(t *testing.T) {
	store := newUITestStore(t)
	ctx := context.Background()

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	dir := "/ui-test-" + suffix
	owner := "ui-owner-" + suffix
	now := time.Now().UTC()

	t.Cleanup(func() {
		cleanups := []struct {
			query string
			arg   string
		}{
			{`DELETE FROM single_use_links WHERE owner = $1`, owner},
			{`DELETE FROM inodes WHERE path LIKE $1`, dir + "/%"},
			{`DELETE FROM erasure_shards WHERE file_path LIKE $1`, dir + "/%"},
			{`DELETE FROM erasure_profiles WHERE file_path LIKE $1`, dir + "/%"},
		}
		for _, cleanup := range cleanups {
			if _, err := store.db.ExecContext(ctx, cleanup.query, cleanup.arg); nil != err {
				t.Errorf("cleanup %q: %v", cleanup.query, err)
			}
		}
	})

	for _, md := range []*metadata.Metadata{
		{Name: "a.txt", Path: dir + "/a.txt", Type: "file", Mode: "0644", Owner: owner, ETag: "etag-a", BackendType: "localfs"},
		{Name: "b.txt", Path: dir + "/b.txt", Type: "file", Mode: "0644", Owner: "", ETag: "", BackendType: "localfs"},
	} {
		md.ATime, md.MTime, md.CTime = now, now, now
		if err := store.Create(ctx, md); nil != err {
			t.Fatalf("create %s: %v", md.Path, err)
		}
	}

	var linkIDs []int64
	for i := 0; i < 2; i++ {
		link := &metadata.SingleUseLink{
			Token:         fmt.Sprintf("ui-token-%s-%d", suffix, i),
			FilePath:      dir + "/a.txt",
			Owner:         owner,
			Status:        "active",
			ExpiresAt:     now.Add(time.Hour),
			CreatedAt:     now,
			HMACSignature: "sig",
		}
		if err := store.CreateSingleUseLink(ctx, link); nil != err {
			t.Fatalf("create link: %v", err)
		}
		stored, err := store.GetSingleUseLink(ctx, link.Token)
		if nil != err {
			t.Fatalf("get link: %v", err)
		}
		linkIDs = append(linkIDs, stored.ID)
	}

	if err := store.RevokeSingleUseLink(ctx, linkIDs[1]); nil != err {
		t.Fatalf("revoke: %v", err)
	}
	if err := store.RevokeSingleUseLink(ctx, linkIDs[1]); !errors.Is(err, metadata.ErrPrecondition) {
		t.Fatalf("revoke twice: want ErrPrecondition, got %v", err)
	}
	if err := store.RevokeSingleUseLink(ctx, 0); !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("revoke missing: want ErrNotFound, got %v", err)
	}

	revoked, err := store.GetSingleUseLinkByID(ctx, linkIDs[1])
	if nil != err {
		t.Fatalf("get by id: %v", err)
	}
	if "revoked" != revoked.Status || owner != revoked.Owner {
		t.Fatalf("revoked link: status=%q owner=%q", revoked.Status, revoked.Owner)
	}

	firstPage, next, err := store.ListSingleUseLinks(ctx, metadata.LinkFilter{CreatedBy: owner, Limit: 1, Now: now})
	if nil != err || 1 != len(firstPage) || "" == next {
		t.Fatalf("first page: len=%d next=%q err=%v", len(firstPage), next, err)
	}
	secondPage, next, err := store.ListSingleUseLinks(ctx, metadata.LinkFilter{CreatedBy: owner, Limit: 1, Cursor: next, Now: now})
	if nil != err || 1 != len(secondPage) || "" != next {
		t.Fatalf("second page: len=%d next=%q err=%v", len(secondPage), next, err)
	}
	if firstPage[0].ID == secondPage[0].ID {
		t.Fatalf("pages repeat link %d", firstPage[0].ID)
	}

	activeOnly, _, err := store.ListSingleUseLinks(ctx, metadata.LinkFilter{CreatedBy: owner, Status: "active", Limit: 10, Now: now})
	if nil != err || 1 != len(activeOnly) {
		t.Fatalf("active filter: len=%d err=%v", len(activeOnly), err)
	}
	if _, _, err := store.ListSingleUseLinks(ctx, metadata.LinkFilter{Cursor: "abc", Limit: 10}); !errors.Is(err, metadata.ErrInvalidQuery) {
		t.Fatalf("bad cursor: want ErrInvalidQuery, got %v", err)
	}

	stats, _, err := store.OwnerStats(ctx, "", 500)
	if nil != err {
		t.Fatalf("owner stats: %v", err)
	}
	found := false
	for _, stat := range stats {
		if owner == stat.Owner {
			found = 1 == stat.Files && 0 == stat.Bytes
		}
		if "" == stat.Owner {
			t.Fatalf("owner stats include empty owner")
		}
	}
	if !found {
		t.Fatalf("owner %s missing from stats", owner)
	}

	emptyOwners, err := store.CountEmptyOwners(ctx)
	if nil != err || 1 > emptyOwners {
		t.Fatalf("count empty owners: n=%d err=%v", emptyOwners, err)
	}
	emptyETags, err := store.CountEmptyETags(ctx)
	if nil != err || 1 > emptyETags {
		t.Fatalf("count empty etags: n=%d err=%v", emptyETags, err)
	}

	result, err := store.SearchMetadata(ctx, metadata.SearchQuery{Path: dir, Limit: 10})
	if nil != err || 2 != len(result.Items) {
		t.Fatalf("search: items=%d err=%v", len(result.Items), err)
	}

	erasureInfo := &metadata.ErasureFileInfo{
		OriginalSize: 10,
		DataShards:   2,
		ParityShards: 1,
		ShardSize:    5,
		Shards: []metadata.ErasureShardInfo{
			{Index: 0, InstanceID: "n1", BackendType: "localfs", Path: "/s0", Size: 5, Checksum: "c0"},
			{Index: 1, InstanceID: "n2", BackendType: "localfs", Path: "/s1", Size: 5, Checksum: "c1"},
		},
	}
	erasurePath := dir + "/big.bin"
	if err := store.CreateErasureInfo(ctx, erasurePath, erasureInfo); nil != err {
		t.Fatalf("create erasure info: %v", err)
	}
	infos, nextErasure, err := store.ListErasureInfos(ctx, dir, "", 10)
	if nil != err || 1 != len(infos) || "" != nextErasure {
		t.Fatalf("list erasure: len=%d next=%q err=%v", len(infos), nextErasure, err)
	}
	if erasurePath != infos[0].FilePath || 2 != len(infos[0].Shards) || 10 != infos[0].OriginalSize {
		t.Fatalf("erasure info: path=%q shards=%d original=%d", infos[0].FilePath, len(infos[0].Shards), infos[0].OriginalSize)
	}
}
