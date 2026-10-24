package fsm

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ebogdum/callfs/metadata"
)

func newTestFSM() *Machine {
	return New()
}

func TestFSMApplyIsDeterministicAcrossReplicas(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	seed := func(f *Machine) {
		applyCmd(t, f, Command{Op: "create_metadata", Metadata: &metadata.Metadata{
			Path: "/dir", Name: "dir", Type: "directory", Owner: "api-user-0",
			CreatedAt: base, UpdatedAt: base,
		}})
		applyCmd(t, f, Command{Op: "create_metadata", Metadata: &metadata.Metadata{
			Path: "/dir/child.txt", Name: "child.txt", Type: "file", Owner: "api-user-0",
			CreatedAt: base, UpdatedAt: base,
		}})
		applyCmd(t, f, Command{Op: "create_link", Link: &metadata.SingleUseLink{
			Token: "tok", FilePath: "/dir/child.txt", Status: "active",
			ExpiresAt: base.Add(time.Hour), CreatedAt: base,
		}})
	}

	stamp := base.Add(30 * time.Minute)
	used := base.Add(31 * time.Minute)
	ip := "203.0.113.7"
	cmds := []Command{
		{Op: "rename_metadata", Path: "/dir", NewPath: "/renamed", Timestamp: stamp},
		{Op: "update_link", Token: "tok", Status: "used", UsedAt: &used, UsedByIP: &ip, Timestamp: stamp},
	}

	replicaA := newTestFSM()
	seed(replicaA)
	for _, c := range cmds {
		applyCmd(t, replicaA, c)
	}

	time.Sleep(5 * time.Millisecond)
	replicaB := newTestFSM()
	seed(replicaB)
	for _, c := range cmds {
		applyCmd(t, replicaB, c)
	}

	stateA, err := json.Marshal(replicaA.state)
	if nil != err {
		t.Fatalf("marshal replica A state: %v", err)
	}
	stateB, err := json.Marshal(replicaB.state)
	if nil != err {
		t.Fatalf("marshal replica B state: %v", err)
	}
	if string(stateA) != string(stateB) {
		t.Fatalf("FSM diverged between replicas applying identical log entries.\nA: %s\nB: %s", stateA, stateB)
	}

	renamed := replicaA.state.MetadataByPath["/renamed"]
	if nil == renamed {
		t.Fatal("expected /renamed to exist after rename_metadata")
	}
	if !renamed.UpdatedAt.Equal(stamp) {
		t.Errorf("renamed dir UpdatedAt = %v, want leader stamp %v", renamed.UpdatedAt, stamp)
	}
	child := replicaA.state.MetadataByPath["/renamed/child.txt"]
	if nil == child {
		t.Fatal("expected subtree entry /renamed/child.txt after rename_metadata")
	}
	if !child.UpdatedAt.Equal(stamp) {
		t.Errorf("renamed child UpdatedAt = %v, want leader stamp %v", child.UpdatedAt, stamp)
	}
	link := replicaA.state.LinksByToken["tok"]
	if nil == link {
		t.Fatal("expected link tok to exist")
	}
	if !link.UpdatedAt.Equal(stamp) {
		t.Errorf("link UpdatedAt = %v, want leader stamp %v", link.UpdatedAt, stamp)
	}
}

func TestFSMReplayPreservesLegacyTimestamps(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := newTestFSM()

	applyCmd(t, f, Command{Op: "create_metadata", Metadata: &metadata.Metadata{
		Path: "/old", Name: "old", Type: "file", Owner: "api-user-0",
		CreatedAt: base, UpdatedAt: base,
	}})
	applyCmd(t, f, Command{Op: "create_link", Link: &metadata.SingleUseLink{
		Token: "legacy", FilePath: "/old", Status: "active",
		ExpiresAt: base.Add(time.Hour), CreatedAt: base, UpdatedAt: base,
	}})

	applyCmd(t, f, Command{Op: "rename_metadata", Path: "/old", NewPath: "/new"})
	applyCmd(t, f, Command{Op: "update_link", Token: "legacy", Status: "used"})

	renamed := f.state.MetadataByPath["/new"]
	if nil == renamed {
		t.Fatal("expected /new to exist after legacy rename")
	}
	if !renamed.UpdatedAt.Equal(base) {
		t.Errorf("legacy replay rewrote UpdatedAt to %v, want preserved %v", renamed.UpdatedAt, base)
	}
	link := f.state.LinksByToken["legacy"]
	if nil == link {
		t.Fatal("expected legacy link to exist")
	}
	if "used" != link.Status {
		t.Errorf("legacy link status = %q, want \"used\"", link.Status)
	}
	if !link.UpdatedAt.Equal(base) {
		t.Errorf("legacy replay rewrote link UpdatedAt to %v, want preserved %v", link.UpdatedAt, base)
	}
}

func TestUpdateLinkIsSingleUse(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := newTestFSM()

	applyCmd(t, f, Command{Op: "create_link", Link: &metadata.SingleUseLink{
		Token: "once", FilePath: "/f.txt", Status: "active",
		ExpiresAt: base.Add(time.Hour), CreatedAt: base,
	}})

	first := applyCmd(t, f, Command{Op: "update_link", Token: "once", Status: "used", Timestamp: base})
	if "" != first.Err {
		t.Fatalf("first consume failed: %s", first.Err)
	}

	second := applyCmd(t, f, Command{Op: "update_link", Token: "once", Status: "used", Timestamp: base})
	if "not_found" != second.Err {
		t.Errorf("second consume err = %q, want \"not_found\" (link must be single-use)", second.Err)
	}
}
