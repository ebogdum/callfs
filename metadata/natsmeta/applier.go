package natsmeta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ebogdum/callfs/v2/metadata/fsm"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

const applierRetryDelay = 100 * time.Millisecond

func (s *Store) runApplier() {
	defer close(s.done)
	for {
		msg, err := s.iter.Next()
		if nil != err {
			if errors.Is(err, jetstream.ErrMsgIteratorClosed) || nil != s.ctx.Err() {
				return
			}
			s.logger.Warn("metadata applier read failed", zap.Error(err))
			select {
			case <-s.ctx.Done():
				return
			case <-time.After(applierRetryDelay):
			}
			continue
		}
		if !s.applyMessage(msg) {
			return
		}
	}
}

func (s *Store) applyMessage(msg jetstream.Msg) bool {
	meta, err := msg.Metadata()
	if nil != err {
		s.halt(fmt.Errorf("natsmeta: read message metadata: %w", err))
		return false
	}
	seq := meta.Sequence.Stream
	s.applyMu.Lock()
	last := s.appliedSeq.Load()
	if seq <= last {
		s.applyMu.Unlock()
		return true
	}
	if last+1 != seq {
		s.applyMu.Unlock()
		s.halt(fmt.Errorf("natsmeta: stream sequence %d follows applied sequence %d: %w", seq, last, ErrSnapshotGap))
		return false
	}
	data := msg.Data()
	var cmd fsm.Command
	var res fsm.Result
	if jsonErr := json.Unmarshal(data, &cmd); nil != jsonErr {
		res = s.machine.ApplyBytes(data, meta.Timestamp)
	} else {
		res = s.machine.Apply(cmd, meta.Timestamp)
		if "group" == cmd.Op {
			res = s.deliverGroup(cmd, res)
		} else if s.self == cmd.Origin && "" != cmd.Nonce {
			s.deliver(cmd.Nonce, res)
		}
	}

	if res.Halt {
		s.applyMu.Unlock()
		s.halt(fmt.Errorf("natsmeta: halted at stream sequence %d: %w", seq, res.AsError()))
		return false
	}
	s.appliedSeq.Store(seq)
	s.applyMu.Unlock()
	s.broadcast()
	return true
}

func (s *Store) deliverGroup(cmd fsm.Command, res fsm.Result) fsm.Result {
	for i, sub := range res.Results {
		if s.self == cmd.Origin && len(cmd.Ops) > i && "" != cmd.Ops[i].Nonce {
			s.deliver(cmd.Ops[i].Nonce, sub)
		}
		if sub.Halt {
			res.Halt = true
			res.Err = sub.Err
		}
	}
	return res
}

func (s *Store) halt(err error) {
	s.haltMu.Lock()
	if nil == s.halted {
		s.halted = err
	}
	s.haltMu.Unlock()
	s.logger.Error("metadata applier stopped", zap.Error(err))
	s.broadcast()
}

func (s *Store) broadcast() {
	s.progressMu.Lock()
	close(s.progress)
	s.progress = make(chan struct{})
	s.progressMu.Unlock()
}

func (s *Store) WaitApplied(ctx context.Context, seq uint64) error {
	for {
		s.progressMu.Lock()
		changed := s.progress
		s.progressMu.Unlock()

		if seq <= s.appliedSeq.Load() {
			return nil
		}
		if err := s.Ready(); nil != err {
			return err
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return fmt.Errorf("natsmeta: waiting for sequence %d: %w", seq, ctx.Err())
		case <-s.ctx.Done():
			return ErrClosed
		}
	}
}

func (s *Store) catchUp(ctx context.Context) error {
	callCtx, cancel := context.WithTimeout(ctx, s.applyTimeout)
	info, err := s.stream.Info(callCtx)
	cancel()
	if nil != err {
		return fmt.Errorf("natsmeta: read stream info: %w", err)
	}
	target := info.State.LastSeq
	for {
		last := s.appliedSeq.Load()
		if target <= last {
			return nil
		}
		stallCtx, stallCancel := context.WithTimeout(ctx, catchUpStall)
		err := s.WaitApplied(stallCtx, min(target, last+1))
		stallCancel()
		if nil != err {
			return fmt.Errorf("natsmeta: replay stalled at sequence %d of %d: %w", s.appliedSeq.Load(), target, err)
		}
	}
}

const catchUpStall = 30 * time.Second
