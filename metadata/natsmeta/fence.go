package natsmeta

import (
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/metadata/fsm"
)

type FenceSource func(path string) (uint64, bool)

func (s *Store) SetFenceSource(source FenceSource) {
	if nil == source {
		s.fences.Store(nil)
		return
	}
	s.fences.Store(&source)
}

func (s *Store) fenceFor(md *metadata.Metadata) *fsm.Fence {
	source := s.fences.Load()
	if nil == source || nil == md {
		return nil
	}
	token, held := (*source)(md.Path)
	if !held {
		return nil
	}
	return &fsm.Fence{Key: md.Path, Token: token}
}
