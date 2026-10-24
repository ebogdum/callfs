package metadata

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type scanStore struct {
	Store
	children map[string][]*Metadata
	files    []*Metadata
}

func (s *scanStore) ListChildren(_ context.Context, parentPath string) ([]*Metadata, error) {
	return s.children[parentPath], nil
}

func (s *scanStore) ListByPrefix(_ context.Context, prefix, startAfter string, limit int) ([]*Metadata, error) {
	var out []*Metadata
	for _, md := range s.files {
		if !strings.HasPrefix(md.Path, prefix) {
			continue
		}
		if "" != startAfter && md.Path <= startAfter {
			continue
		}
		out = append(out, md)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func newScanFixture() *scanStore {
	dir := &Metadata{Name: "a", Path: "/a", Type: "directory", Owner: "alice"}
	files := []*Metadata{
		{Name: "1.log", Path: "/a/1.log", Type: "file", Size: 10, Owner: "alice", BackendType: "localfs"},
		{Name: "2.log", Path: "/a/2.log", Type: "file", Size: 20, Owner: "alice", BackendType: "localfs"},
		{Name: "3.log", Path: "/a/3.log", Type: "file", Size: 30, Owner: "bob", BackendType: "localfs"},
		{Name: "4.log", Path: "/a/4.log", Type: "file", Size: 40, Owner: "alice", BackendType: "localfs"},
	}
	return &scanStore{
		children: map[string][]*Metadata{
			"/":  {dir},
			"/a": files,
		},
		files: files,
	}
}

func TestSearchByScanFiltersAndPaginates(t *testing.T) {
	store := newScanFixture()
	ctx := context.Background()

	first, err := SearchByScan(ctx, store, SearchQuery{Q: "*.log", Owner: "alice", Limit: 2})
	if nil != err {
		t.Fatalf("first page: %v", err)
	}
	if 2 != len(first.Items) || "/a/1.log" != first.Items[0].Path || "/a/2.log" != first.Items[1].Path {
		t.Fatalf("first page items: %+v", first.Items)
	}
	if "/a/2.log" != first.NextCursor {
		t.Fatalf("next cursor: %q", first.NextCursor)
	}
	if 3 != first.Facets["owner"]["alice"] {
		t.Fatalf("owner facet alice: %d", first.Facets["owner"]["alice"])
	}

	second, err := SearchByScan(ctx, store, SearchQuery{Q: "*.log", Owner: "alice", Limit: 2, Cursor: first.NextCursor})
	if nil != err {
		t.Fatalf("second page: %v", err)
	}
	if 1 != len(second.Items) || "/a/4.log" != second.Items[0].Path || "" != second.NextCursor {
		t.Fatalf("second page: items=%d cursor=%q", len(second.Items), second.NextCursor)
	}
}

func TestSearchByScanRejectsInvalidQuery(t *testing.T) {
	store := newScanFixture()
	ctx := context.Background()

	if _, err := SearchByScan(ctx, store, SearchQuery{Limit: 0}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("zero limit: %v", err)
	}
	if _, err := SearchByScan(ctx, store, SearchQuery{Type: "link", Limit: 10}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("unknown type: %v", err)
	}
}
