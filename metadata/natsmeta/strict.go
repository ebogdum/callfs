package natsmeta

import (
	"context"
	"fmt"
)

type barrierRound struct {
	done chan struct{}
	err  error
}

func (s *Store) SetStrictReads(strict bool) {
	s.strictReads.Store(strict)
}

func (s *Store) strictBarrier(ctx context.Context) error {
	if !s.strictReads.Load() {
		return nil
	}
	s.barrierMu.Lock()
	if nil == s.nextRound {
		s.nextRound = &barrierRound{done: make(chan struct{})}
	}
	round := s.nextRound
	if !s.roundRunning {
		s.roundRunning = true
		s.nextRound = nil
		go s.runBarrierRounds(round)
	}
	s.barrierMu.Unlock()
	select {
	case <-round.done:
		return round.err
	case <-ctx.Done():
		return fmt.Errorf("natsmeta: waiting for read barrier: %w", ctx.Err())
	case <-s.ctx.Done():
		return ErrClosed
	}
}

func (s *Store) runBarrierRounds(round *barrierRound) {
	for nil != round {
		ctx, cancel := context.WithTimeout(s.ctx, s.applyTimeout)
		round.err = s.barrier(ctx)
		cancel()
		close(round.done)
		s.barrierMu.Lock()
		round = s.nextRound
		s.nextRound = nil
		if nil == round {
			s.roundRunning = false
		}
		s.barrierMu.Unlock()
	}
}
