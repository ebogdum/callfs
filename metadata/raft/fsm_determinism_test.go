package raft

import (
	"encoding/json"
	"testing"
	"time"

	hashiraft "github.com/hashicorp/raft"

	"github.com/ebogdum/callfs/metadata"
)

// newTestFSM builds an FSM through the same constructor NewRaftStore uses, so
// tests can never diverge from production initialization.
func newTestFSM() *fsm {
	return newFSM()
}

func applyCmd(t *testing.T, f *fsm, cmd Command) CommandResult {
	t.Helper()
	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("marshal command: %v", err)
	}
	res, ok := f.Apply(&hashiraft.Log{Data: data}).(CommandResult)
	if !ok {
		t.Fatalf("Apply returned unexpected type for op %q", cmd.Op)
	}
	return res
}

// TestFSMApplyIsDeterministicAcrossReplicas is the regression guard for the
// wall-clock reads that used to live inside Apply. Every replica applies the
// same log entry independently, at different wall-clock instants, so any
// time.Now() inside Apply makes replicas diverge. Applying an identical
// command to two FSMs at different instants must produce identical state.
func TestFSMApplyIsDeterministicAcrossReplicas(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	seed := func(f *fsm) {
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

	// One command set, stamped once by the leader, as applyAsLeader does.
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

	// Replica B applies the very same entries at a measurably later instant,
	// standing in for a follower that lags the leader or replays after restart.
	time.Sleep(5 * time.Millisecond)
	replicaB := newTestFSM()
	seed(replicaB)
	for _, c := range cmds {
		applyCmd(t, replicaB, c)
	}

	stateA, err := json.Marshal(replicaA.state)
	if err != nil {
		t.Fatalf("marshal replica A state: %v", err)
	}
	stateB, err := json.Marshal(replicaB.state)
	if err != nil {
		t.Fatalf("marshal replica B state: %v", err)
	}
	if string(stateA) != string(stateB) {
		t.Fatalf("FSM diverged between replicas applying identical log entries.\nA: %s\nB: %s", stateA, stateB)
	}

	// The applied timestamp must be the leader's stamp, not the apply-time clock.
	renamed := replicaA.state.MetadataByPath["/renamed"]
	if renamed == nil {
		t.Fatal("expected /renamed to exist after rename_metadata")
	}
	if !renamed.UpdatedAt.Equal(stamp) {
		t.Errorf("renamed dir UpdatedAt = %v, want leader stamp %v", renamed.UpdatedAt, stamp)
	}
	child := replicaA.state.MetadataByPath["/renamed/child.txt"]
	if child == nil {
		t.Fatal("expected subtree entry /renamed/child.txt after rename_metadata")
	}
	if !child.UpdatedAt.Equal(stamp) {
		t.Errorf("renamed child UpdatedAt = %v, want leader stamp %v", child.UpdatedAt, stamp)
	}
	link := replicaA.state.LinksByToken["tok"]
	if link == nil {
		t.Fatal("expected link tok to exist")
	}
	if !link.UpdatedAt.Equal(stamp) {
		t.Errorf("link UpdatedAt = %v, want leader stamp %v", link.UpdatedAt, stamp)
	}
}

// TestFSMReplayPreservesLegacyTimestamps covers Raft logs written before
// Command.Timestamp existed. Those entries unmarshal with a zero timestamp;
// replaying them must not rewrite UpdatedAt to the replay instant, which would
// make every renamed file appear modified at the moment the node restarted.
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

	// Legacy entries: no Timestamp field on the wire.
	applyCmd(t, f, Command{Op: "rename_metadata", Path: "/old", NewPath: "/new"})
	applyCmd(t, f, Command{Op: "update_link", Token: "legacy", Status: "used"})

	renamed := f.state.MetadataByPath["/new"]
	if renamed == nil {
		t.Fatal("expected /new to exist after legacy rename")
	}
	if !renamed.UpdatedAt.Equal(base) {
		t.Errorf("legacy replay rewrote UpdatedAt to %v, want preserved %v", renamed.UpdatedAt, base)
	}
	link := f.state.LinksByToken["legacy"]
	if link == nil {
		t.Fatal("expected legacy link to exist")
	}
	if link.Status != "used" {
		t.Errorf("legacy link status = %q, want \"used\"", link.Status)
	}
	if !link.UpdatedAt.Equal(base) {
		t.Errorf("legacy replay rewrote link UpdatedAt to %v, want preserved %v", link.UpdatedAt, base)
	}
}

// TestUpdateLinkIsSingleUse guards the CAS that makes a download link
// single-use: the second consume of the same token must be rejected.
func TestUpdateLinkIsSingleUse(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := newTestFSM()

	applyCmd(t, f, Command{Op: "create_link", Link: &metadata.SingleUseLink{
		Token: "once", FilePath: "/f.txt", Status: "active",
		ExpiresAt: base.Add(time.Hour), CreatedAt: base,
	}})

	first := applyCmd(t, f, Command{Op: "update_link", Token: "once", Status: "used", Timestamp: base})
	if first.Err != "" {
		t.Fatalf("first consume failed: %s", first.Err)
	}

	second := applyCmd(t, f, Command{Op: "update_link", Token: "once", Status: "used", Timestamp: base})
	if second.Err != "not_found" {
		t.Errorf("second consume err = %q, want \"not_found\" (link must be single-use)", second.Err)
	}
}
