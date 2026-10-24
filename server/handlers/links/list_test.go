package links

import (
	"testing"
	"time"

	"github.com/ebogdum/callfs/v2/metadata"
)

func TestParseLinkStatus(t *testing.T) {
	if got, err := parseLinkStatus(""); nil != err || "" != got {
		t.Fatalf("empty status = %q, %v", got, err)
	}
	for _, status := range []string{"active", "used", "expired", "revoked"} {
		if got, err := parseLinkStatus(status); nil != err || status != got {
			t.Fatalf("parseLinkStatus(%q) = %q, %v", status, got, err)
		}
	}
	if _, err := parseLinkStatus("deleted"); nil == err {
		t.Fatal("unknown status accepted")
	}
}

func TestParseLinkLimit(t *testing.T) {
	if got, err := parseLinkLimit(""); nil != err || linkListDefaultLimit != got {
		t.Fatalf("default limit = %d, %v", got, err)
	}
	if got, err := parseLinkLimit("1000"); nil != err || 1000 != got {
		t.Fatalf("limit 1000 = %d, %v", got, err)
	}
	for _, raw := range []string{"0", "-3", "1001", "many"} {
		if _, err := parseLinkLimit(raw); nil == err {
			t.Fatalf("parseLinkLimit(%q) accepted", raw)
		}
	}
}

func TestParseLinkStatsWindow(t *testing.T) {
	if window, duration, err := parseLinkStatsWindow(""); nil != err || "24h" != window || 24*time.Hour != duration {
		t.Fatalf("default window = %q, %v, %v", window, duration, err)
	}
	if window, duration, err := parseLinkStatsWindow("7d"); nil != err || "7d" != window || 7*24*time.Hour != duration {
		t.Fatalf("window 7d = %q, %v, %v", window, duration, err)
	}
	if _, _, err := parseLinkStatsWindow("2h"); nil == err {
		t.Fatal("unknown window accepted")
	}
}

func TestParseLinkID(t *testing.T) {
	if got, err := parseLinkID("42"); nil != err || 42 != got {
		t.Fatalf("parseLinkID(42) = %d, %v", got, err)
	}
	for _, raw := range []string{"0", "-1", "abc", ""} {
		if _, err := parseLinkID(raw); nil == err {
			t.Fatalf("parseLinkID(%q) accepted", raw)
		}
	}
}

func TestEffectiveLinkStatus(t *testing.T) {
	now := time.Now()
	active := metadata.SingleUseLink{Status: "active", ExpiresAt: now.Add(time.Minute)}
	if got := effectiveLinkStatus(active, now); "active" != got {
		t.Fatalf("future active link = %q", got)
	}
	expired := metadata.SingleUseLink{Status: "active", ExpiresAt: now.Add(-time.Minute)}
	if got := effectiveLinkStatus(expired, now); "expired" != got {
		t.Fatalf("past active link = %q", got)
	}
	revoked := metadata.SingleUseLink{Status: "revoked", ExpiresAt: now.Add(-time.Minute)}
	if got := effectiveLinkStatus(revoked, now); "revoked" != got {
		t.Fatalf("revoked link = %q", got)
	}
}
