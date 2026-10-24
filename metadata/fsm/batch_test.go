package fsm

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/ebogdum/callfs/metadata"
)

func applyCmd(t *testing.T, f *Machine, cmd Command) Result {
	t.Helper()
	data, err := json.Marshal(cmd)
	if nil != err {
		t.Fatalf("marshal command: %v", err)
	}
	return f.ApplyBytes(data, time.Time{})
}

func seedBatchFSM(t *testing.T, f *Machine, base time.Time) {
	t.Helper()
	for _, md := range []*metadata.Metadata{
		{Path: "/dir", Name: "dir", Type: "directory", Owner: "api-user-0", CreatedAt: base, UpdatedAt: base},
		{Path: "/dir/a.txt", Name: "a.txt", Type: "file", Owner: "api-user-0", ETag: "etag-a", CreatedAt: base, UpdatedAt: base},
		{Path: "/dir/sub", Name: "sub", Type: "directory", Owner: "api-user-0", CreatedAt: base, UpdatedAt: base},
		{Path: "/dir/sub/b.txt", Name: "b.txt", Type: "file", Owner: "api-user-0", ErasureCoded: true, CreatedAt: base, UpdatedAt: base},
	} {
		if res := applyCmd(t, f, Command{Op: "create_metadata", Metadata: md}); "" != res.Err {
			t.Fatalf("seed %s: %s", md.Path, res.Err)
		}
	}
	info := &metadata.ErasureFileInfo{FilePath: "/dir/sub/b.txt", DataShards: 2, ParityShards: 1}
	if res := applyCmd(t, f, Command{Op: "create_erasure_info", Path: "/dir/sub/b.txt", ErasureInfo: info}); "" != res.Err {
		t.Fatalf("seed erasure: %s", res.Err)
	}
}

func fsmFingerprint(t *testing.T, f *Machine) string {
	t.Helper()
	index := make([]string, 0)
	for parent, children := range f.childIndex {
		for child := range children {
			index = append(index, parent+">"+child)
		}
	}
	sort.Strings(index)
	data, err := json.Marshal(struct {
		State state
		Index []string
	}{State: f.state, Index: index})
	if nil != err {
		t.Fatalf("marshal fsm: %v", err)
	}
	return string(data)
}

func TestFSMApplyBatchFailureLeavesStateUntouched(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := []struct {
		name string
		ops  []Command
		want string
	}{
		{
			name: "missing delete after rename",
			ops: []Command{
				{Op: "create_metadata", Metadata: &metadata.Metadata{Path: "/new.txt", Name: "new.txt", Type: "file"}},
				{Op: "rename_metadata", Path: "/dir", NewPath: "/moved"},
				{Op: "delete_metadata", Path: "/dir/a.txt"},
			},
			want: "not_found",
		},
		{
			name: "create collides with renamed subtree",
			ops: []Command{
				{Op: "rename_metadata", Path: "/dir", NewPath: "/moved"},
				{Op: "create_metadata", Metadata: &metadata.Metadata{Path: "/moved/sub/b.txt", Name: "b.txt", Type: "file"}},
			},
			want: "already_exists",
		},
		{
			name: "etag mismatch after delete",
			ops: []Command{
				{Op: "delete_metadata", Path: "/dir/sub/b.txt"},
				{Op: "update_metadata_if_match", Metadata: &metadata.Metadata{Path: "/dir/a.txt", Name: "a.txt", Type: "file", ETag: "next"}, ExpectedETag: "stale"},
			},
			want: "precondition_failed",
		},
		{
			name: "erasure info moved by rename",
			ops: []Command{
				{Op: "rename_metadata", Path: "/dir", NewPath: "/moved"},
				{Op: "create_erasure_info", Path: "/moved/sub/b.txt", ErasureInfo: &metadata.ErasureFileInfo{FilePath: "/moved/sub/b.txt"}},
			},
			want: "already_exists",
		},
		{
			name: "nested batch rejected",
			ops: []Command{
				{Op: "delete_metadata", Path: "/dir/a.txt"},
				{Op: "apply_batch"},
			},
			want: "unknown_operation",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := New()
			seedBatchFSM(t, f, base)
			before := fsmFingerprint(t, f)
			res := applyCmd(t, f, Command{Op: "apply_batch", Ops: tc.ops, Timestamp: base.Add(time.Hour)})
			if tc.want != res.Err {
				t.Fatalf("batch err = %q, want %q", res.Err, tc.want)
			}
			if after := fsmFingerprint(t, f); before != after {
				t.Fatalf("failed batch mutated state.\nbefore: %s\nafter:  %s", before, after)
			}
		})
	}
}

func TestFSMApplyBatchAppliesEveryOpAndIsDeterministic(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	stamp := base.Add(time.Hour)
	batch := Command{Op: "apply_batch", Timestamp: stamp, Ops: []Command{
		{Op: "create_metadata", Metadata: &metadata.Metadata{Path: "/trash", Name: "trash", Type: "directory", CreatedAt: base, UpdatedAt: base}},
		{Op: "rename_metadata", Path: "/dir", NewPath: "/trash/dir"},
		{Op: "create_metadata", Metadata: &metadata.Metadata{Path: "/trash/rec.json", Name: "rec.json", Type: "file", CreatedAt: base, UpdatedAt: base}},
		{Op: "delete_erasure_info", Path: "/trash/dir/sub/b.txt"},
		{Op: "delete_metadata", Path: "/trash/dir/sub/b.txt"},
		{Op: "update_metadata_if_match", Metadata: &metadata.Metadata{Path: "/trash/dir/a.txt", Name: "a.txt", Type: "file", ETag: "etag-b", UpdatedAt: base}, ExpectedETag: "etag-a"},
	}}

	replicaA := New()
	seedBatchFSM(t, replicaA, base)
	if res := applyCmd(t, replicaA, batch); "" != res.Err {
		t.Fatalf("batch failed: %s", res.Err)
	}
	time.Sleep(5 * time.Millisecond)
	replicaB := New()
	seedBatchFSM(t, replicaB, base)
	if res := applyCmd(t, replicaB, batch); "" != res.Err {
		t.Fatalf("batch failed on replica B: %s", res.Err)
	}
	if fsmFingerprint(t, replicaA) != fsmFingerprint(t, replicaB) {
		t.Fatal("replicas diverged after applying the same batch")
	}

	st := replicaA.state
	for _, gone := range []string{"/dir", "/dir/a.txt", "/dir/sub", "/dir/sub/b.txt", "/trash/dir/sub/b.txt"} {
		if _, ok := st.MetadataByPath[gone]; ok {
			t.Errorf("%s should not exist", gone)
		}
	}
	for _, kept := range []string{"/trash", "/trash/dir", "/trash/dir/a.txt", "/trash/dir/sub", "/trash/rec.json"} {
		if _, ok := st.MetadataByPath[kept]; !ok {
			t.Errorf("%s should exist", kept)
		}
	}
	if 0 != len(st.ErasureByPath) {
		t.Errorf("erasure info left behind: %v", st.ErasureByPath)
	}
	if moved := st.MetadataByPath["/trash/dir"]; nil == moved || !moved.UpdatedAt.Equal(stamp) {
		t.Errorf("renamed dir must carry the leader stamp %v", stamp)
	}
	if "etag-b" != st.MetadataByPath["/trash/dir/a.txt"].ETag {
		t.Error("update_if_match inside the batch was not applied")
	}
	if replicaA.HasChildren("/dir") {
		t.Error("child index still lists children under /dir")
	}
	if 2 != replicaA.CountChildren("/trash") {
		t.Errorf("child index for /trash = %d, want 2", replicaA.CountChildren("/trash"))
	}
}
