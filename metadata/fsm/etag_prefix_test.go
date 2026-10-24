package fsm

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ebogdum/callfs/v2/metadata"
)

func TestFSMUpdateIfMatchAppliesOnlyOnMatchingETag(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := newTestFSM()

	applyCmd(t, f, Command{Op: "create_metadata", Metadata: &metadata.Metadata{
		Path: "/doc.txt", Name: "doc.txt", Type: "file", Owner: "api-user-0", ETag: "v1",
		CreatedAt: base, UpdatedAt: base,
	}})

	updated := &metadata.Metadata{Path: "/doc.txt", Name: "doc.txt", Type: "file", Owner: "api-user-0", ETag: "v2", CreatedAt: base, UpdatedAt: base}

	stale := applyCmd(t, f, Command{Op: "update_metadata_if_match", Metadata: updated, ExpectedETag: "stale"})
	if "precondition_failed" != stale.Err {
		t.Fatalf("stale update err = %q, want \"precondition_failed\"", stale.Err)
	}
	if got := f.state.MetadataByPath["/doc.txt"].ETag; "v1" != got {
		t.Fatalf("stale update changed ETag to %q", got)
	}

	match := applyCmd(t, f, Command{Op: "update_metadata_if_match", Metadata: updated, ExpectedETag: "v1"})
	if "" != match.Err {
		t.Fatalf("matching update failed: %s", match.Err)
	}
	if got := f.state.MetadataByPath["/doc.txt"].ETag; "v2" != got {
		t.Errorf("ETag after matching update = %q, want \"v2\"", got)
	}

	missing := applyCmd(t, f, Command{Op: "update_metadata_if_match", Metadata: &metadata.Metadata{Path: "/nope", Type: "file"}, ExpectedETag: ""})
	if "not_found" != missing.Err {
		t.Errorf("missing update err = %q, want \"not_found\"", missing.Err)
	}
}

func TestFSMDeleteRemovesErasureInfo(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := newTestFSM()

	applyCmd(t, f, Command{Op: "create_metadata", Metadata: &metadata.Metadata{
		Path: "/coded.bin", Name: "coded.bin", Type: "file", Owner: "api-user-0", ErasureCoded: true,
		CreatedAt: base, UpdatedAt: base,
	}})
	applyCmd(t, f, Command{Op: "create_erasure_info", Path: "/coded.bin", ErasureInfo: &metadata.ErasureFileInfo{
		FilePath: "/coded.bin", OriginalSize: 10, DataShards: 2, ParityShards: 1, ShardSize: 5,
	}})

	if res := applyCmd(t, f, Command{Op: "delete_metadata", Path: "/coded.bin"}); "" != res.Err {
		t.Fatalf("delete failed: %s", res.Err)
	}
	if _, ok := f.state.ErasureByPath["/coded.bin"]; ok {
		t.Error("erasure info survived delete_metadata")
	}
}

func TestFSMFilesByPrefixPagesInPathOrder(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := newTestFSM()

	create := func(path, typ string) {
		applyCmd(t, f, Command{Op: "create_metadata", Metadata: &metadata.Metadata{
			Path: path, Name: pathBase(path), Type: typ, Owner: "api-user-0", CreatedAt: base, UpdatedAt: base,
		}})
	}
	create("/logs", "directory")
	create("/logs/b.txt", "file")
	create("/logs/a.txt", "file")
	create("/logs/c.txt", "file")
	create("/logs/sub", "directory")
	create("/logs/sub/d.txt", "file")
	create("/other.txt", "file")

	f.mu.RLock()
	defer f.mu.RUnlock()

	first := f.filesByPrefix("/logs/", "", 2)
	if 2 != len(first) || "/logs/a.txt" != first[0].Path || "/logs/b.txt" != first[1].Path {
		t.Fatalf("first page = %v, want [/logs/a.txt /logs/b.txt]", paths(first))
	}

	second := f.filesByPrefix("/logs/", first[1].Path, 2)
	if 2 != len(second) || "/logs/c.txt" != second[0].Path || "/logs/sub/d.txt" != second[1].Path {
		t.Fatalf("second page = %v, want [/logs/c.txt /logs/sub/d.txt]", paths(second))
	}

	rest := f.filesByPrefix("/logs/", second[1].Path, 2)
	if 0 != len(rest) {
		t.Fatalf("page past end = %v, want empty", paths(rest))
	}
}

func TestFSMETagSurvivesSnapshotRoundTrip(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := newTestFSM()

	applyCmd(t, f, Command{Op: "create_metadata", Metadata: &metadata.Metadata{
		Path: "/tagged.txt", Name: "tagged.txt", Type: "file", Owner: "api-user-0", ETag: "abc123",
		CreatedAt: base, UpdatedAt: base,
	}})

	data, err := json.Marshal(f.state)
	if nil != err {
		t.Fatalf("marshal state: %v", err)
	}
	var restored state
	if err := json.Unmarshal(data, &restored); nil != err {
		t.Fatalf("unmarshal state: %v", err)
	}
	if got := restored.MetadataByPath["/tagged.txt"].ETag; "abc123" != got {
		t.Errorf("restored ETag = %q, want \"abc123\"", got)
	}
}

func paths(entries []*metadata.Metadata) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return out
}

func TestFSMListDescendantsHonorsDepthAndOrder(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := newTestFSM()

	for _, md := range []*metadata.Metadata{
		{Path: "/a", Name: "a", Type: "directory"},
		{Path: "/a/b", Name: "b", Type: "directory"},
		{Path: "/a/b/c", Name: "c", Type: "file"},
		{Path: "/a/d", Name: "d", Type: "file"},
		{Path: "/z", Name: "z", Type: "file"},
	} {
		md.Owner, md.ETag, md.CreatedAt, md.UpdatedAt = "api-user-0", "e", base, base
		applyCmd(t, f, Command{Op: "create_metadata", Metadata: md})
	}

	paths := func(list []*metadata.Metadata) []string {
		out := make([]string, 0, len(list))
		for _, md := range list {
			out = append(out, md.Path)
		}
		return out
	}

	if got := paths(f.descendantsOf("/a", 1, 0)); 2 != len(got) || "/a/b" != got[0] || "/a/d" != got[1] {
		t.Errorf("depth 1 descendants = %v, want [/a/b /a/d]", got)
	}
	if got := paths(f.descendantsOf("/a", 0, 0)); 3 != len(got) || "/a/b" != got[0] || "/a/b/c" != got[1] || "/a/d" != got[2] {
		t.Errorf("unlimited descendants = %v, want [/a/b /a/b/c /a/d]", got)
	}
	if !f.HasChildren("/a") {
		t.Errorf("HasChildren(/a) = false, want true")
	}
	if f.HasChildren("/a/d") {
		t.Errorf("HasChildren(/a/d) = true, want false")
	}
}

func TestFSMConsumeLinkConsumesOnce(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := newTestFSM()

	applyCmd(t, f, Command{Op: "create_link", Link: &metadata.SingleUseLink{
		Token: "live", FilePath: "/x", Status: "active", ExpiresAt: base.Add(time.Hour), CreatedAt: base, UpdatedAt: base,
	}})
	applyCmd(t, f, Command{Op: "create_link", Link: &metadata.SingleUseLink{
		Token: "expired", FilePath: "/x", Status: "active", ExpiresAt: base, CreatedAt: base, UpdatedAt: base,
	}})

	first := applyCmd(t, f, Command{Op: "consume_link", Token: "live", UsedAt: &base, UsedByIP: stringPtr("10.0.0.1")})
	if "" != first.Err || !first.Consumed || nil == first.Link || "used" != first.Link.Status {
		t.Fatalf("first consume = %+v, want consumed used link", first)
	}

	second := applyCmd(t, f, Command{Op: "consume_link", Token: "live", UsedAt: timePtr(base.Add(time.Second)), UsedByIP: stringPtr("10.0.0.2")})
	if "" != second.Err || second.Consumed || nil == second.Link || "used" != second.Link.Status {
		t.Errorf("second consume = %+v, want not consumed", second)
	}

	expired := applyCmd(t, f, Command{Op: "consume_link", Token: "expired", UsedAt: &base, UsedByIP: stringPtr("10.0.0.1")})
	if "" != expired.Err || expired.Consumed || nil == expired.Link || "active" != expired.Link.Status {
		t.Errorf("expired consume = %+v, want not consumed and still active", expired)
	}

	missing := applyCmd(t, f, Command{Op: "consume_link", Token: "nope", UsedAt: &base, UsedByIP: stringPtr("10.0.0.1")})
	if "not_found" != missing.Err {
		t.Errorf("missing consume err = %q, want \"not_found\"", missing.Err)
	}
}

func stringPtr(s string) *string {
	return &s
}

func timePtr(t time.Time) *time.Time {
	return &t
}
