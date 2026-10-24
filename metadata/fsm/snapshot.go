package fsm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"time"
)

const maxSnapshotRestoreBytes = 4 << 30

type Snapshot struct {
	state state
}

func (m *Machine) Capture() *Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	staged := make(map[string]*stagedBatch, len(m.state.StagedBatches))
	for id, batch := range m.state.StagedBatches {
		copied := *batch
		copied.Ops = batch.Ops[:len(batch.Ops):len(batch.Ops)]
		staged[id] = &copied
	}
	return &Snapshot{state: state{
		MetadataByPath: maps.Clone(m.state.MetadataByPath),
		LinksByToken:   maps.Clone(m.state.LinksByToken),
		ErasureByPath:  maps.Clone(m.state.ErasureByPath),
		LastAt:         m.state.LastAt,
		Fences:         maps.Clone(m.state.Fences),
		StagedBatches:  staged,
	}}
}

func (s *Snapshot) Write(w io.Writer) ([32]byte, error) {
	hasher := sha256.New()
	var sum [32]byte
	if err := json.NewEncoder(io.MultiWriter(w, hasher)).Encode(s.state); nil != err {
		return sum, fmt.Errorf("encode metadata snapshot: %w", err)
	}
	copy(sum[:], hasher.Sum(nil))
	return sum, nil
}

func (m *Machine) WriteSnapshot(w io.Writer) ([32]byte, error) {
	return m.Capture().Write(w)
}

func (m *Machine) StateHash() [32]byte {
	sum, err := m.Capture().Write(io.Discard)
	if nil != err {
		return [32]byte{}
	}
	return sum
}

func (m *Machine) Restore(r io.Reader) error {
	var restored state
	if err := json.NewDecoder(io.LimitReader(r, maxSnapshotRestoreBytes)).Decode(&restored); nil != err {
		return fmt.Errorf("failed to decode metadata snapshot: %w", err)
	}
	restored.fillMissing()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = restored
	m.rebuildChildIndex()
	m.rebuildLinkIndex()
	return nil
}

func (m *Machine) LastAt() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.LastAt
}
