package fsm

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ebogdum/callfs/v2/metadata"
)

func mustApply(t *testing.T, f *Machine, cmd Command) {
	t.Helper()
	data, err := json.Marshal(cmd)
	if nil != err {
		t.Fatalf("marshal %s: %v", cmd.Op, err)
	}
	if err := f.ApplyBytes(data, time.Time{}).AsError(); nil != err {
		t.Fatalf("apply %s: %v", cmd.Op, err)
	}
}

func seedFile(t *testing.T, f *Machine, path string) {
	t.Helper()
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	mustApply(t, f, Command{Op: "create_metadata", Metadata: &metadata.Metadata{
		Path: path, Name: pathBase(path), Type: "file", Owner: "alice",
		CreatedAt: base, UpdatedAt: base,
	}})
}

func seedDir(t *testing.T, f *Machine, path string) {
	t.Helper()
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	mustApply(t, f, Command{Op: "create_metadata", Metadata: &metadata.Metadata{
		Path: path, Name: pathBase(path), Type: "directory", Owner: "alice",
		CreatedAt: base, UpdatedAt: base,
	}})
}

func childPaths(f *Machine, parent string) []string {
	out := make([]string, 0)
	for _, md := range f.childrenOf(parent) {
		out = append(out, md.Path)
	}
	sort.Strings(out)
	return out
}

func bruteForceChildren(f *Machine, parent string) []string {
	out := make([]string, 0)
	for path := range f.state.MetadataByPath {
		if "/" == path {
			continue
		}
		if parent == pathDir(path) {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

func assertIndexMatchesScan(t *testing.T, f *Machine, parents ...string) {
	t.Helper()
	for _, parent := range parents {
		got := childPaths(f, parent)
		want := bruteForceChildren(f, parent)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("children of %q: index gave %v, full scan gave %v", parent, got, want)
		}
	}
}

func TestChildIndexMatchesFullScan(t *testing.T) {
	f := New()

	seedDir(t, f, "/dir")
	seedDir(t, f, "/dir/sub")
	seedFile(t, f, "/dir/a.txt")
	seedFile(t, f, "/dir/b.txt")
	seedFile(t, f, "/dir/sub/c.txt")
	seedFile(t, f, "/root.txt")

	assertIndexMatchesScan(t, f, "/", "/dir", "/dir/sub")

	if got := childPaths(f, "/dir"); 3 != len(got) {
		t.Errorf("children of /dir = %v, want 3 entries", got)
	}

	mustApply(t, f, Command{Op: "delete_metadata", Path: "/dir/a.txt"})
	assertIndexMatchesScan(t, f, "/", "/dir", "/dir/sub")
	for _, p := range childPaths(f, "/dir") {
		if "/dir/a.txt" == p {
			t.Error("deleted path still present in child index")
		}
	}

	stamp := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	mustApply(t, f, Command{Op: "rename_metadata", Path: "/dir", NewPath: "/moved", Timestamp: stamp})
	assertIndexMatchesScan(t, f, "/", "/dir", "/dir/sub", "/moved", "/moved/sub")

	if got := childPaths(f, "/dir"); 0 != len(got) {
		t.Errorf("old parent /dir still has children %v after rename", got)
	}
	if got := childPaths(f, "/moved/sub"); 1 != len(got) || "/moved/sub/c.txt" != got[0] {
		t.Errorf("children of /moved/sub = %v, want [/moved/sub/c.txt]", got)
	}
}

func TestChildIndexRebuiltOnRestore(t *testing.T) {
	source := New()
	seedDir(t, source, "/dir")
	seedFile(t, source, "/dir/a.txt")
	seedFile(t, source, "/dir/b.txt")
	seedFile(t, source, "/root.txt")

	encoded, err := json.Marshal(source.state)
	if nil != err {
		t.Fatalf("marshal state: %v", err)
	}

	target := New()
	if err := target.Restore(io.NopCloser(strings.NewReader(string(encoded)))); nil != err {
		t.Fatalf("Restore: %v", err)
	}

	assertIndexMatchesScan(t, target, "/", "/dir")
	if got := childPaths(target, "/dir"); 2 != len(got) {
		t.Errorf("after restore, children of /dir = %v, want 2 entries", got)
	}
	if got := childPaths(target, "/"); 2 != len(got) {
		t.Errorf("after restore, children of / = %v, want /dir and /root.txt", got)
	}
}

func TestChildIndexReleasesEmptyParents(t *testing.T) {
	f := New()
	seedDir(t, f, "/dir")

	for i := 0; 50 > i; i++ {
		path := fmt.Sprintf("/dir/f%d.txt", i)
		seedFile(t, f, path)
		mustApply(t, f, Command{Op: "delete_metadata", Path: path})
	}

	if set, ok := f.childIndex["/dir"]; ok {
		t.Errorf("child index kept an empty entry for /dir with %d members", len(set))
	}
	assertIndexMatchesScan(t, f, "/", "/dir")
}

func TestListChildrenExcludesRootAndGrandchildren(t *testing.T) {
	f := New()
	seedDir(t, f, "/a")
	seedDir(t, f, "/a/b")
	seedFile(t, f, "/a/b/deep.txt")
	seedFile(t, f, "/a/shallow.txt")

	got := childPaths(f, "/a")
	if 2 != len(got) || "/a/b" != got[0] || "/a/shallow.txt" != got[1] {
		t.Errorf("children of /a = %v, want [/a/b /a/shallow.txt] (direct children only)", got)
	}
}
