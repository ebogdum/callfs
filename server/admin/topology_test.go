package admin

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ebogdum/callfs/v2/core"
)

func TestInstanceEndpointValidation(t *testing.T) {
	for _, raw := range []string{"https://peer-b.internal:8443", "http://10.0.0.2:8080/callfs/"} {
		if _, err := validateInstanceEndpoint(raw); nil != err {
			t.Fatalf("validateInstanceEndpoint(%q) returned %v", raw, err)
		}
	}
	for _, raw := range []string{"", "ftp://peer:21", "https://", "https://user:pass@peer", "https://peer:8443?x=1"} {
		if _, err := validateInstanceEndpoint(raw); nil == err {
			t.Fatalf("validateInstanceEndpoint(%q) accepted an invalid endpoint", raw)
		}
	}
}

func TestInstanceSchemeFromEndpoint(t *testing.T) {
	if "https" != instanceScheme("https://peer:8443") {
		t.Fatal("https endpoint did not report https")
	}
	if "http" != instanceScheme("http://peer:8080") {
		t.Fatal("http endpoint did not report http")
	}
}

func TestInstanceProbeStateKeepsLatest(t *testing.T) {
	state := &instanceState{probes: map[string]instanceProbe{}, overrides: map[string]string{}}
	state.storeProbe("peer-b", instanceProbe{Reachable: true, RTTMs: 12})
	probe, found := state.probe("peer-b")
	if !found || !probe.Reachable || 12 != probe.RTTMs {
		t.Fatalf("stored probe not returned: found=%v probe=%+v", found, probe)
	}
	state.setOverride("peer-b", "")
	if endpoint, set := state.overrideSnapshot()["peer-b"]; !set || "" != endpoint {
		t.Fatal("removal override not recorded as empty endpoint")
	}
}

func TestBackendStatusFromProbe(t *testing.T) {
	failed := &backendProbeRecord{OK: false}
	cases := []struct {
		name    string
		enabled bool
		probe   *backendProbeRecord
		free    uint64
		total   uint64
		want    string
	}{
		{"disabled", false, nil, 0, 0, "disabled"},
		{"down after failed probe", true, failed, 500, 1000, "down"},
		{"warn below ten percent", true, nil, 50, 1000, "warn"},
		{"ok above ten percent", true, nil, 500, 1000, "ok"},
	}
	for _, tc := range cases {
		if got := backendStatus(tc.enabled, tc.probe, tc.free, tc.total); tc.want != got {
			t.Errorf("%s: backendStatus returned %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestStagingScanFindsLeftovers(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".callfs-staging"), 0o700); nil != err {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".callfs-staging", "abc"), []byte("12345"), 0o600); nil != err {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, stagingTempPrefix+"x"), []byte("1"), 0o600); nil != err {
		t.Fatal(err)
	}
	items, err := scanLocalStaging(root, time.Now().UTC())
	if nil != err {
		t.Fatal(err)
	}
	if 2 != len(items) {
		t.Fatalf("scan returned %d items, want 2", len(items))
	}
	var total int64
	kinds := map[string]string{}
	for _, item := range items {
		total += item.Bytes
		kinds[item.Path] = item.Kind
	}
	if 6 != total || "staging" != kinds["/.callfs-staging/abc"] || "tmp" != kinds["/"+stagingTempPrefix+"x"] {
		t.Fatalf("unexpected scan result: total=%d kinds=%v", total, kinds)
	}
	if _, err := stagingTarget(root, "/"); nil == err {
		t.Fatal("stagingTarget accepted the storage root itself")
	}
}

func TestStagingAgeParsing(t *testing.T) {
	if got, err := parseStagingAge(""); nil != err || 86400*time.Second != got {
		t.Fatalf("default age = %v, %v", got, err)
	}
	if got, err := parseStagingAge("5"); nil != err || 5*time.Second != got {
		t.Fatalf("age 5 = %v, %v", got, err)
	}
	for _, raw := range []string{"-1", "abc", "99999999999"} {
		if _, err := parseStagingAge(raw); nil == err {
			t.Fatalf("parseStagingAge(%q) accepted an invalid age", raw)
		}
	}
}

func TestReplicationPathNormalization(t *testing.T) {
	if got, err := normalizeAdminPath("/a/../b"); nil != err || "/b" != got {
		t.Fatalf("normalizeAdminPath returned %q, %v", got, err)
	}
	for _, raw := range []string{"", "relative/path", "/bad\x00name"} {
		if _, err := normalizeAdminPath(raw); nil == err {
			t.Fatalf("normalizeAdminPath(%q) accepted an invalid path", raw)
		}
	}
}

func TestHygieneMigrationDriver(t *testing.T) {
	if "postgres" != migrationDriver("postgres") || "none" != migrationDriver("sqlite") {
		t.Fatal("migrationDriver did not map store types")
	}
}

func TestRetagHashMatchesETag(t *testing.T) {
	digest, err := hashContent(io.NopCloser(strings.NewReader("abc")))
	if nil != err {
		t.Fatal(err)
	}
	if want := core.ETagFor([]byte("abc")); want != digest {
		t.Fatalf("hashContent returned %s, want %s", digest, want)
	}
}
