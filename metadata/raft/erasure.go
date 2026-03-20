package raft

import (
	"context"

	"github.com/ebogdum/callfs/metadata"
)

// CreateErasureInfo stores erasure coding metadata via Raft consensus.
func (s *Store) CreateErasureInfo(ctx context.Context, filePath string, info *metadata.ErasureFileInfo) error {
	_, err := s.applyCommand(ctx, Command{
		Op:          "create_erasure_info",
		Path:        filePath,
		ErasureInfo: cloneErasureFileInfo(info),
	})
	return err
}

// GetErasureInfo retrieves erasure coding metadata from in-memory state.
func (s *Store) GetErasureInfo(ctx context.Context, filePath string) (*metadata.ErasureFileInfo, error) {
	s.fsm.mu.RLock()
	defer s.fsm.mu.RUnlock()
	info, ok := s.fsm.state.ErasureByPath[filePath]
	if !ok {
		return nil, metadata.ErrNotFound
	}
	return cloneErasureFileInfo(info), nil
}

// DeleteErasureInfo removes erasure coding metadata via Raft consensus.
func (s *Store) DeleteErasureInfo(ctx context.Context, filePath string) error {
	_, err := s.applyCommand(ctx, Command{
		Op:   "delete_erasure_info",
		Path: filePath,
	})
	return err
}

func cloneErasureFileInfo(in *metadata.ErasureFileInfo) *metadata.ErasureFileInfo {
	if in == nil {
		return nil
	}
	out := *in
	if in.Shards != nil {
		out.Shards = make([]metadata.ErasureShardInfo, len(in.Shards))
		copy(out.Shards, in.Shards)
	}
	return &out
}

func cloneErasureMap(in map[string]*metadata.ErasureFileInfo) map[string]*metadata.ErasureFileInfo {
	out := make(map[string]*metadata.ErasureFileInfo, len(in))
	for k, v := range in {
		out[k] = cloneErasureFileInfo(v)
	}
	return out
}
