package erasure

import (
	"testing"
)

func assertAssignment(t *testing.T, assignments []string, idx int, expected string) {
	t.Helper()
	if assignments[idx] != expected {
		t.Errorf("shard %d: expected %s, got %s", idx, expected, assignments[idx])
	}
}

func assertAllOnNode(t *testing.T, assignments []string, node string) {
	t.Helper()
	for i, a := range assignments {
		if a != node {
			t.Errorf("shard %d should be on %s, got %s", i, node, a)
		}
	}
}

func TestRoundRobinPlacement(t *testing.T) {
	p := &RoundRobinPlacement{}

	t.Run("basic distribution", func(t *testing.T) {
		assignments := p.AssignShards(6, "node1", []string{"node1", "node2", "node3"})
		if len(assignments) != 6 {
			t.Fatalf("expected 6 assignments, got %d", len(assignments))
		}

		assertAssignment(t, assignments, 0, "node1")
		assertAssignment(t, assignments, 1, "node2")
		assertAssignment(t, assignments, 2, "node3")
		assertAssignment(t, assignments, 3, "node1")
	})

	t.Run("single node", func(t *testing.T) {
		assignments := p.AssignShards(4, "node1", []string{"node1"})
		assertAllOnNode(t, assignments, "node1")
	})

	t.Run("no available instances", func(t *testing.T) {
		assignments := p.AssignShards(3, "node1", nil)
		assertAllOnNode(t, assignments, "node1")
	})

	t.Run("more shards than nodes", func(t *testing.T) {
		assignments := p.AssignShards(8, "a", []string{"a", "b"})
		if len(assignments) != 8 {
			t.Fatalf("expected 8 assignments, got %d", len(assignments))
		}
		for i, a := range assignments {
			expected := "a"
			if i%2 == 1 {
				expected = "b"
			}
			assertAssignment(t, assignments, i, expected)
			_ = a
		}
	})

	t.Run("current instance not in available list", func(t *testing.T) {
		assignments := p.AssignShards(4, "nodeX", []string{"node1", "node2"})
		assertAssignment(t, assignments, 0, "nodeX")
		assertAssignment(t, assignments, 1, "node1")
		assertAssignment(t, assignments, 2, "node2")
	})
}
