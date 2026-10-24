package localfs

import (
	"sync"
	"time"
)

const groupCommitWindow = 2 * time.Millisecond

type syncBatch struct {
	ready chan struct{}
	done  chan struct{}
	err   error
}

type dirSyncState struct {
	running *syncBatch
	next    *syncBatch
}

type dirSyncer struct {
	mode   SyncMode
	window time.Duration
	syncFn func(dirRel string) error
	mu     sync.Mutex
	dirs   map[string]*dirSyncState
}

func newDirSyncer(mode SyncMode, window time.Duration, syncFn func(dirRel string) error) *dirSyncer {
	return &dirSyncer{
		mode:   mode,
		window: window,
		syncFn: syncFn,
		dirs:   make(map[string]*dirSyncState),
	}
}

func (s *dirSyncer) state(dirRel string) *dirSyncState {
	st := s.dirs[dirRel]
	if nil == st {
		st = &dirSyncState{}
		s.dirs[dirRel] = st
	}
	return st
}

func (s *dirSyncer) sync(dirRel string) error {
	if SyncNone == s.mode {
		return nil
	}
	s.mu.Lock()
	st := s.state(dirRel)
	batch, leader, previous := s.enqueue(st)
	s.mu.Unlock()
	return s.await(dirRel, st, batch, leader, previous)
}

func (s *dirSyncer) enqueue(st *dirSyncState) (*syncBatch, bool, *syncBatch) {
	if nil != st.next {
		return st.next, false, nil
	}
	batch := &syncBatch{ready: make(chan struct{}), done: make(chan struct{})}
	close(batch.ready)
	st.next = batch
	return batch, true, st.running
}

func (s *dirSyncer) await(dirRel string, st *dirSyncState, batch *syncBatch, leader bool, previous *syncBatch) error {
	if !leader {
		<-batch.done
		return batch.err
	}
	timer := time.NewTimer(s.window)
	if nil != previous {
		<-previous.done
	}
	select {
	case <-batch.ready:
	case <-timer.C:
	}
	timer.Stop()

	s.mu.Lock()
	st.next = nil
	st.running = batch
	s.mu.Unlock()

	batch.err = s.syncFn(dirRel)

	s.mu.Lock()
	if batch == st.running {
		st.running = nil
	}
	s.forgetIfIdle(dirRel, st)
	s.mu.Unlock()
	close(batch.done)
	return batch.err
}

func (s *dirSyncer) forgetIfIdle(dirRel string, st *dirSyncState) {
	if nil == st.next && nil == st.running {
		delete(s.dirs, dirRel)
	}
}
