package erasure

// PlacementStrategy assigns shards to instances.
type PlacementStrategy interface {
	AssignShards(totalShards int, currentInstanceID string, availableInstances []string) []string
}

// RoundRobinPlacement distributes shards round-robin across available instances.
// Shard 0 is always placed on the current instance.
type RoundRobinPlacement struct{}

// AssignShards returns a slice of instance IDs, one per shard.
// The current instance gets shard 0, then round-robin across all instances.
func (p *RoundRobinPlacement) AssignShards(totalShards int, currentInstanceID string, availableInstances []string) []string {
	if len(availableInstances) == 0 {
		assignments := make([]string, totalShards)
		for i := range assignments {
			assignments[i] = currentInstanceID
		}
		return assignments
	}

	// Build ordered list with current instance first, then others
	ordered := make([]string, 0, len(availableInstances))
	ordered = append(ordered, currentInstanceID)
	for _, inst := range availableInstances {
		if inst != currentInstanceID {
			ordered = append(ordered, inst)
		}
	}

	assignments := make([]string, totalShards)
	for i := range assignments {
		assignments[i] = ordered[i%len(ordered)]
	}
	return assignments
}
