package erasure

import (
	"testing"
)

func TestRoundRobinPlacement(t *testing.T) {
	p := &RoundRobinPlacement{}

	t.Run("basic distribution", func(t *testing.T) {
		assignments := p.AssignShards(6, "node1", []string{"node1", "node2", "node3"})
		if len(assignments) != 6 {
			t.Fatalf("expected 6 assignments, got %d", len(assignments))
		}

		// Shard 0 should be on current instance
		if assignments[0] != "node1" {
			t.Errorf("shard 0 should be on node1, got %s", assignments[0])
		}

		// Should round-robin
		if assignments[1] != "node2" {
			t.Errorf("shard 1 should be on node2, got %s", assignments[1])
		}
		if assignments[2] != "node3" {
			t.Errorf("shard 2 should be on node3, got %s", assignments[2])
		}
		if assignments[3] != "node1" {
			t.Errorf("shard 3 should wrap to node1, got %s", assignments[3])
		}
	})

	t.Run("single node", func(t *testing.T) {
		assignments := p.AssignShards(4, "node1", []string{"node1"})
		for i, a := range assignments {
			if a != "node1" {
				t.Errorf("shard %d should be on node1, got %s", i, a)
			}
		}
	})

	t.Run("no available instances", func(t *testing.T) {
		assignments := p.AssignShards(3, "node1", nil)
		for i, a := range assignments {
			if a != "node1" {
				t.Errorf("shard %d should be on node1 (fallback), got %s", i, a)
			}
		}
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
			if a != expected {
				t.Errorf("shard %d: expected %s, got %s", i, expected, a)
			}
		}
	})

	t.Run("current instance not in available list", func(t *testing.T) {
		assignments := p.AssignShards(4, "nodeX", []string{"node1", "node2"})
		// nodeX should be first, then round-robin
		if assignments[0] != "nodeX" {
			t.Errorf("shard 0 should be on nodeX, got %s", assignments[0])
		}
		if assignments[1] != "node1" {
			t.Errorf("shard 1 should be on node1, got %s", assignments[1])
		}
		if assignments[2] != "node2" {
			t.Errorf("shard 2 should be on node2, got %s", assignments[2])
		}
	})
}
