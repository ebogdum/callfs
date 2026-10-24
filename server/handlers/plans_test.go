package handlers

import (
	"encoding/json"
	"testing"

	"github.com/ebogdum/callfs/v2/core"
)

func TestFileModeNormalization(t *testing.T) {
	valid := map[string]string{"640": "0640", "0640": "0640", "0777": "0777"}
	for raw, want := range valid {
		got, ok := normalizeFileMode(raw)
		if !ok || got != want {
			t.Fatalf("normalizeFileMode(%q) = %q, %v; want %q, true", raw, got, ok, want)
		}
	}

	for _, raw := range []string{"", "0", "08", "0800", "12345", "64a", "1640", "00640", "rwx"} {
		if _, ok := normalizeFileMode(raw); ok {
			t.Fatalf("normalizeFileMode(%q) accepted an invalid mode", raw)
		}
	}
}

func TestPrefixBoundary(t *testing.T) {
	cases := []struct {
		path, prefix string
		want         bool
	}{
		{"/dir/a.txt", "/dir", true},
		{"/dir/sub/b", "/dir/", true},
		{"/dir", "/dir", false},
		{"/dirty.txt", "/dir", false},
	}
	for _, c := range cases {
		if got := core.WithinPrefixBoundary(c.path, c.prefix); got != c.want {
			t.Fatalf("WithinPrefixBoundary(%q, %q) = %v, want %v", c.path, c.prefix, got, c.want)
		}
	}
}

func TestDeletePlanJSONShape(t *testing.T) {
	buf, err := json.Marshal(core.DeletePlan{RemoteInstances: []string{}})
	if err != nil {
		t.Fatalf("marshal delete plan: %v", err)
	}
	want := `{"files":0,"directories":0,"bytes":0,"erasure_files":0,"remote_instances":[]}`
	if string(buf) != want {
		t.Fatalf("delete plan JSON = %s, want %s", buf, want)
	}
}
