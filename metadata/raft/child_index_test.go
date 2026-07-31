package raft

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ebogdum/callfs/metadata"
)

func seedFile(t *testing.T, f *fsm, path string) {
	t.Helper()
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	applyCmd(t, f, Command{Op: "create_metadata", Metadata: &metadata.Metadata{
		Path: path, Name: pathBaseRaft(path), Type: "file", Owner: "alice",
		CreatedAt: base, UpdatedAt: base,
	}})
}

func seedDir(t *testing.T, f *fsm, path string) {
	t.Helper()
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	applyCmd(t, f, Command{Op: "create_metadata", Metadata: &metadata.Metadata{
		Path: path, Name: pathBaseRaft(path), Type: "directory", Owner: "alice",
		CreatedAt: base, UpdatedAt: base,
	}})
}

// childPaths returns the sorted direct children of parent, as the index sees them.
func childPaths(f *fsm, parent string) []string {
	out := make([]string, 0)
	for _, md := range f.childrenOf(parent) {
		out = append(out, md.Path)
	}
	sort.Strings(out)
	return out
}

// bruteForceChildren computes direct children by the full scan the index replaced.
// It is the oracle the index is checked against.
func bruteForceChildren(f *fsm, parent string) []string {
	out := make([]string, 0)
	for path := range f.state.MetadataByPath {
		if path == "/" {
			continue
		}
		if pathDir(path) == parent {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

func assertIndexMatchesScan(t *testing.T, f *fsm, parents ...string) {
	t.Helper()
	for _, parent := range parents {
		got := childPaths(f, parent)
		want := bruteForceChildren(f, parent)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("children of %q: index gave %v, full scan gave %v", parent, got, want)
		}
	}
}

// TestChildIndexMatchesFullScan checks the index agrees with the exhaustive scan
// it replaced, across creates, deletes and renames.
func TestChildIndexMatchesFullScan(t *testing.T) {
	f := newTestFSM()

	seedDir(t, f, "/dir")
	seedDir(t, f, "/dir/sub")
	seedFile(t, f, "/dir/a.txt")
	seedFile(t, f, "/dir/b.txt")
	seedFile(t, f, "/dir/sub/c.txt")
	seedFile(t, f, "/root.txt")

	assertIndexMatchesScan(t, f, "/", "/dir", "/dir/sub")

	if got := childPaths(f, "/dir"); len(got) != 3 {
		t.Errorf("children of /dir = %v, want 3 entries", got)
	}

	// Delete removes exactly one child from its parent.
	applyCmd(t, f, Command{Op: "delete_metadata", Path: "/dir/a.txt"})
	assertIndexMatchesScan(t, f, "/", "/dir", "/dir/sub")
	for _, p := range childPaths(f, "/dir") {
		if p == "/dir/a.txt" {
			t.Error("deleted path still present in child index")
		}
	}

	// Renaming a directory must move the whole subtree in the index.
	stamp := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	applyCmd(t, f, Command{Op: "rename_metadata", Path: "/dir", NewPath: "/moved", Timestamp: stamp})
	assertIndexMatchesScan(t, f, "/", "/dir", "/dir/sub", "/moved", "/moved/sub")

	if got := childPaths(f, "/dir"); len(got) != 0 {
		t.Errorf("old parent /dir still has children %v after rename", got)
	}
	if got := childPaths(f, "/moved/sub"); len(got) != 1 || got[0] != "/moved/sub/c.txt" {
		t.Errorf("children of /moved/sub = %v, want [/moved/sub/c.txt]", got)
	}
}

// TestChildIndexRebuiltOnRestore is the guard that matters most for correctness:
// the index is not part of the snapshot, so Restore must rebuild it. If it did
// not, a node that restored from a snapshot would report every directory as
// empty while its metadata was fully intact.
func TestChildIndexRebuiltOnRestore(t *testing.T) {
	source := newTestFSM()
	seedDir(t, source, "/dir")
	seedFile(t, source, "/dir/a.txt")
	seedFile(t, source, "/dir/b.txt")
	seedFile(t, source, "/root.txt")

	encoded, err := json.Marshal(source.state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}

	target := newTestFSM()
	if err := target.Restore(io.NopCloser(strings.NewReader(string(encoded)))); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	assertIndexMatchesScan(t, target, "/", "/dir")
	if got := childPaths(target, "/dir"); len(got) != 2 {
		t.Errorf("after restore, children of /dir = %v, want 2 entries", got)
	}
	if got := childPaths(target, "/"); len(got) != 2 {
		t.Errorf("after restore, children of / = %v, want /dir and /root.txt", got)
	}
}

// TestChildIndexReleasesEmptyParents confirms the index does not leak an entry
// per directory that once had children, which would reintroduce unbounded growth
// on a create/delete-heavy workload.
func TestChildIndexReleasesEmptyParents(t *testing.T) {
	f := newTestFSM()
	seedDir(t, f, "/dir")

	for i := 0; i < 50; i++ {
		path := fmt.Sprintf("/dir/f%d.txt", i)
		seedFile(t, f, path)
		applyCmd(t, f, Command{Op: "delete_metadata", Path: path})
	}

	if set, ok := f.childIndex["/dir"]; ok {
		t.Errorf("child index kept an empty entry for /dir with %d members", len(set))
	}
	assertIndexMatchesScan(t, f, "/", "/dir")
}

// TestListChildrenExcludesRootAndGrandchildren checks the index returns direct
// children only, matching the previous pathDir-based filter exactly.
func TestListChildrenExcludesRootAndGrandchildren(t *testing.T) {
	f := newTestFSM()
	seedDir(t, f, "/a")
	seedDir(t, f, "/a/b")
	seedFile(t, f, "/a/b/deep.txt")
	seedFile(t, f, "/a/shallow.txt")

	got := childPaths(f, "/a")
	if len(got) != 2 || got[0] != "/a/b" || got[1] != "/a/shallow.txt" {
		t.Errorf("children of /a = %v, want [/a/b /a/shallow.txt] (direct children only)", got)
	}
}
