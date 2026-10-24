package sqlite

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ebogdum/callfs/metadata"
)

func listedPaths(mds []*metadata.Metadata) []string {
	paths := make([]string, 0, len(mds))
	for _, md := range mds {
		paths = append(paths, md.Path)
	}
	return paths
}

func TestUpdateIfMatchMatch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkFile(t, s, "/a.txt")

	md, err := s.Get(ctx, "/a.txt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	md.ETag = "v1"
	if err := s.UpdateIfMatch(ctx, md, ""); err != nil {
		t.Fatalf("update with matching etag: %v", err)
	}

	got, err := s.Get(ctx, "/a.txt")
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.ETag != "v1" {
		t.Fatalf("expected etag v1, got %q", got.ETag)
	}
}

func TestUpdateIfMatchMismatch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkFile(t, s, "/a.txt")

	md, err := s.Get(ctx, "/a.txt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	md.ETag = "v2"
	if err := s.UpdateIfMatch(ctx, md, "stale"); err != metadata.ErrPrecondition {
		t.Fatalf("expected ErrPrecondition, got %v", err)
	}

	got, err := s.Get(ctx, "/a.txt")
	if err != nil {
		t.Fatalf("get after mismatch: %v", err)
	}
	if got.ETag != "" {
		t.Fatalf("etag must not change on mismatch, got %q", got.ETag)
	}
}

func TestUpdateIfMatchMissing(t *testing.T) {
	s := newTestStore(t)
	md := &metadata.Metadata{Path: "/nope.txt", ETag: "v1"}
	if err := s.UpdateIfMatch(context.Background(), md, "v1"); err != metadata.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListByPrefixOrderingStartAfterLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkFile(t, s, "/p/c.txt")
	mkFile(t, s, "/p/a.txt")
	mkFile(t, s, "/p/b.txt")
	mkDir(t, s, "/p/dir")
	mkFile(t, s, "/q/z.txt")

	all, err := s.ListByPrefix(ctx, "/p/", "", 10)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if want := []string{"/p/a.txt", "/p/b.txt", "/p/c.txt"}; !reflect.DeepEqual(listedPaths(all), want) {
		t.Fatalf("expected %v, got %v", want, listedPaths(all))
	}

	after, err := s.ListByPrefix(ctx, "/p/", "/p/a.txt", 10)
	if err != nil {
		t.Fatalf("list after: %v", err)
	}
	if want := []string{"/p/b.txt", "/p/c.txt"}; !reflect.DeepEqual(listedPaths(after), want) {
		t.Fatalf("startAfter: expected %v, got %v", want, listedPaths(after))
	}

	limited, err := s.ListByPrefix(ctx, "/p/", "", 2)
	if err != nil {
		t.Fatalf("list limited: %v", err)
	}
	if want := []string{"/p/a.txt", "/p/b.txt"}; !reflect.DeepEqual(listedPaths(limited), want) {
		t.Fatalf("limit: expected %v, got %v", want, listedPaths(limited))
	}
}

func TestListByPrefixCaseSensitive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkFile(t, s, "/p/a.txt")
	mkFile(t, s, "/P/x.txt")

	upper, err := s.ListByPrefix(ctx, "/P/", "", 10)
	if err != nil {
		t.Fatalf("list upper: %v", err)
	}
	if want := []string{"/P/x.txt"}; !reflect.DeepEqual(listedPaths(upper), want) {
		t.Fatalf("expected %v, got %v", want, listedPaths(upper))
	}
}

func TestRenameDoesNotTouchCaseVariantSibling(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkDir(t, s, "/dir")
	mkFile(t, s, "/dir/f.txt")
	mkDir(t, s, "/DIR")
	mkFile(t, s, "/DIR/keep.txt")

	if err := s.Rename(ctx, "/dir", "/new"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := s.Get(ctx, "/DIR/keep.txt"); err != nil {
		t.Fatalf("case-variant sibling must be untouched, got %v", err)
	}
	if _, err := s.Get(ctx, "/new/f.txt"); err != nil {
		t.Fatalf("renamed child: %v", err)
	}
}

func TestHasChildren(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkDir(t, s, "/empty")
	mkDir(t, s, "/d")
	mkFile(t, s, "/d/f.txt")

	if has, err := s.HasChildren(ctx, "/empty"); nil != err || has {
		t.Fatalf("empty dir: has=%v err=%v", has, err)
	}
	if has, err := s.HasChildren(ctx, "/d"); nil != err || !has {
		t.Fatalf("dir with child: has=%v err=%v", has, err)
	}
}

func TestListDescendantsDepthAndOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mkDir(t, s, "/r")
	mkFile(t, s, "/r/a.txt")
	mkDir(t, s, "/r/sub")
	mkFile(t, s, "/r/sub/b.txt")
	mkDir(t, s, "/r/sub/deep")
	mkFile(t, s, "/r/sub/deep/c.txt")
	mkFile(t, s, "/rx.txt")

	cases := []struct {
		dir   string
		depth int
		want  []string
	}{
		{"/r", 0, []string{"/r/a.txt", "/r/sub", "/r/sub/b.txt", "/r/sub/deep", "/r/sub/deep/c.txt"}},
		{"/r", 1, []string{"/r/a.txt", "/r/sub"}},
		{"/r", 2, []string{"/r/a.txt", "/r/sub", "/r/sub/b.txt", "/r/sub/deep"}},
		{"/", 1, []string{"/r", "/rx.txt"}},
	}
	for _, tc := range cases {
		got, err := s.ListDescendants(ctx, tc.dir, tc.depth, 0)
		if nil != err {
			t.Fatalf("list %s depth %d: %v", tc.dir, tc.depth, err)
		}
		if !reflect.DeepEqual(listedPaths(got), tc.want) {
			t.Fatalf("list %s depth %d: expected %v, got %v", tc.dir, tc.depth, tc.want, listedPaths(got))
		}
	}
}

func TestConsumeSingleUseLink(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	mkFile(t, s, "/a.txt")
	for token, expiresAt := range map[string]time.Time{"live": now.Add(time.Hour), "stale": now.Add(-time.Minute)} {
		if err := s.CreateSingleUseLink(ctx, &metadata.SingleUseLink{
			Token: token, FilePath: "/a.txt", Status: "active", ExpiresAt: expiresAt, HMACSignature: "sig",
		}); nil != err {
			t.Fatalf("create %s: %v", token, err)
		}
	}

	link, consumed, err := s.ConsumeSingleUseLink(ctx, "live", "10.0.0.1", now)
	if nil != err {
		t.Fatalf("first consume: %v", err)
	}
	if !consumed || "used" != link.Status || nil == link.UsedByIP || "10.0.0.1" != *link.UsedByIP {
		t.Fatalf("first consume: consumed=%v link=%+v", consumed, link)
	}

	link, consumed, err = s.ConsumeSingleUseLink(ctx, "live", "10.0.0.2", now)
	if nil != err {
		t.Fatalf("second consume: %v", err)
	}
	if consumed || "used" != link.Status || nil == link.UsedByIP || "10.0.0.1" != *link.UsedByIP {
		t.Fatalf("second consume must not re-consume: consumed=%v link=%+v", consumed, link)
	}

	link, consumed, err = s.ConsumeSingleUseLink(ctx, "stale", "10.0.0.1", now)
	if nil != err {
		t.Fatalf("expired consume: %v", err)
	}
	if consumed || "active" != link.Status {
		t.Fatalf("expired link must not be consumed: consumed=%v status=%q", consumed, link.Status)
	}

	_, consumed, err = s.ConsumeSingleUseLink(ctx, "missing", "10.0.0.1", now)
	if !errors.Is(err, metadata.ErrNotFound) || consumed {
		t.Fatalf("missing token: consumed=%v err=%v", consumed, err)
	}
}
