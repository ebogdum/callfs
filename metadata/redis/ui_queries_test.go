package redis

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

func TestCursorRoundTrip(t *testing.T) {
	for _, value := range []string{"alice", "/files/a b.txt", "0"} {
		decoded, err := decodeCursor(encodeCursor(value))
		if nil != err {
			t.Fatalf("decodeCursor(encodeCursor(%q)) error: %v", value, err)
		}
		if value != decoded {
			t.Fatalf("round trip = %q, want %q", decoded, value)
		}
	}
}

func TestDecodeCursorEmptyAndInvalid(t *testing.T) {
	decoded, err := decodeCursor("")
	if nil != err || "" != decoded {
		t.Fatalf("decodeCursor(\"\") = %q, %v; want empty, nil", decoded, err)
	}
	for _, cursor := range []string{"!!!", "YWJj"} {
		if _, err := decodeCursor(cursor); !errors.Is(err, metadata.ErrInvalidQuery) {
			t.Fatalf("decodeCursor(%q) error = %v, want ErrInvalidQuery", cursor, err)
		}
	}
}

func TestDecodeLinkCursor(t *testing.T) {
	id, err := decodeLinkCursor(encodeCursor("42"))
	if nil != err || 42 != id {
		t.Fatalf("decodeLinkCursor = %d, %v; want 42, nil", id, err)
	}
	id, err = decodeLinkCursor("")
	if nil != err || 0 != id {
		t.Fatalf("decodeLinkCursor(\"\") = %d, %v; want 0, nil", id, err)
	}
	for _, value := range []string{"abc", "0", "-3"} {
		if _, err := decodeLinkCursor(encodeCursor(value)); !errors.Is(err, metadata.ErrInvalidQuery) {
			t.Fatalf("decodeLinkCursor(%q) error = %v, want ErrInvalidQuery", value, err)
		}
	}
}

func TestLinkStatusAt(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)
	cases := []struct {
		name   string
		status string
		expiry time.Time
		want   string
	}{
		{"active unexpired", "active", future, "active"},
		{"active expired", "active", past, "expired"},
		{"used expired", "used", past, "used"},
		{"revoked expired", "revoked", past, "revoked"},
	}
	for _, tc := range cases {
		link := &metadata.SingleUseLink{Status: tc.status, ExpiresAt: tc.expiry}
		if got := linkStatusAt(link, now); tc.want != got {
			t.Fatalf("%s: linkStatusAt = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestLinkMatches(t *testing.T) {
	link := &metadata.SingleUseLink{ID: 7, Status: "active", FilePath: "/a.txt", Owner: "alice"}
	cases := []struct {
		name   string
		filter metadata.LinkFilter
		want   bool
	}{
		{"empty filter", metadata.LinkFilter{}, true},
		{"status match", metadata.LinkFilter{Status: "active"}, true},
		{"status mismatch", metadata.LinkFilter{Status: "used"}, false},
		{"path match", metadata.LinkFilter{FilePath: "/a.txt"}, true},
		{"path mismatch", metadata.LinkFilter{FilePath: "/b.txt"}, false},
		{"creator match", metadata.LinkFilter{CreatedBy: "alice"}, true},
		{"creator mismatch", metadata.LinkFilter{CreatedBy: "bob"}, false},
	}
	for _, tc := range cases {
		if got := linkMatches(link, tc.filter); tc.want != got {
			t.Fatalf("%s: linkMatches = %t, want %t", tc.name, got, tc.want)
		}
	}
}

func TestDecodeLinkSkipsCorruptRecord(t *testing.T) {
	store := &RedisStore{logger: zap.NewNop()}
	if _, ok := store.decodeLink("callfs:sul:x", "{not json"); ok {
		t.Fatal("decodeLink accepted corrupt JSON")
	}
	link, ok := store.decodeLink("callfs:sul:y", `{"id":3,"status":"used","file_path":"/a"}`)
	if !ok || 3 != link.ID || "used" != link.Status || "/a" != link.FilePath {
		t.Fatalf("decodeLink = %+v, %t; want id 3 status used path /a", link, ok)
	}
}
