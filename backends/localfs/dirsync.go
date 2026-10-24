package localfs

import (
	"sync"
	"time"
)

const (
	groupCommitWindow   = 2 * time.Millisecond
	groupCommitMaxBatch = 128
)

type syncBatch struct {
	waiters  int
	released bool
	ready    chan struct{}
	done     chan struct{}
	err      error
}

type dirSyncState struct {
	pending int
	running *syncBatch
	next    *syncBatch
}

type dirSyncer struct {
	mode     SyncMode
	window   time.Duration
	maxBatch int
	syncFn   func(dirRel string) error
	mu       sync.Mutex
	dirs     map[string]*dirSyncState
}

func newDirSyncer(mode SyncMode, window time.Duration, maxBatch int, syncFn func(dirRel string) error) *dirSyncer {
	if 1 > maxBatch {
		maxBatch = 1
	}
	return &dirSyncer{
		mode:     mode,
		window:   window,
		maxBatch: maxBatch,
		syncFn:   syncFn,
		dirs:     make(map[string]*dirSyncState),
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

func (s *dirSyncer) begin(dirRel string) {
	if SyncNone == s.mode {
		return
	}
	s.mu.Lock()
	s.state(dirRel).pending++
	s.mu.Unlock()
}

func (s *dirSyncer) cancel(dirRel string) {
	if SyncNone == s.mode {
		return
	}
	s.mu.Lock()
	st := s.state(dirRel)
	st.pending--
	s.releaseIfReady(st)
	s.forgetIfIdle(dirRel, st)
	s.mu.Unlock()
}

func (s *dirSyncer) commit(dirRel string) error {
	if SyncNone == s.mode {
		return nil
	}
	s.mu.Lock()
	st := s.state(dirRel)
	st.pending--
	batch, leader, previous := s.enqueue(st)
	s.mu.Unlock()
	return s.await(dirRel, st, batch, leader, previous)
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
		st.next.waiters++
		s.releaseIfReady(st)
		return st.next, false, nil
	}
	batch := &syncBatch{waiters: 1, ready: make(chan struct{}), done: make(chan struct{})}
	st.next = batch
	s.releaseIfReady(st)
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

func (s *dirSyncer) releaseIfReady(st *dirSyncState) {
	batch := st.next
	if nil == batch || batch.released {
		return
	}
	if 0 >= st.pending || s.maxBatch <= batch.waiters {
		batch.released = true
		close(batch.ready)
	}
}

func (s *dirSyncer) forgetIfIdle(dirRel string, st *dirSyncState) {
	if 0 == st.pending && nil == st.next && nil == st.running {
		delete(s.dirs, dirRel)
	}
}
