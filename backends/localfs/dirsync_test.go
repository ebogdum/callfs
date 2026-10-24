package localfs

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDirSyncerBatchesConcurrentCommits(t *testing.T) {
	var calls atomic.Int64
	s := newDirSyncer(SyncFull, 50*time.Millisecond, 1000, func(string) error {
		calls.Add(1)
		return nil
	})
	const writers = 32
	for range writers {
		s.begin("d")
	}
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = s.commit("d")
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if nil != err {
			t.Fatalf("waiter %d: %v", i, err)
		}
	}
	if 1 != calls.Load() {
		t.Fatalf("expected one shared sync, got %d", calls.Load())
	}
	if 0 != len(s.dirs) {
		t.Fatalf("state leaked: %d dirs", len(s.dirs))
	}
}

func TestDirSyncerSharesErrorWithAllWaiters(t *testing.T) {
	boom := errors.New("boom")
	s := newDirSyncer(SyncFull, 50*time.Millisecond, 1000, func(string) error { return boom })
	const writers = 8
	for range writers {
		s.begin("d")
	}
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = s.commit("d")
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if !errors.Is(err, boom) {
			t.Fatalf("waiter %d got %v", i, err)
		}
	}
}

func TestDirSyncerCancelReleasesWaiters(t *testing.T) {
	var calls atomic.Int64
	s := newDirSyncer(SyncFull, time.Second, 1000, func(string) error {
		calls.Add(1)
		return nil
	})
	s.begin("d")
	s.begin("d")
	done := make(chan error, 1)
	go func() { done <- s.commit("d") }()
	time.Sleep(20 * time.Millisecond)
	s.cancel("d")
	select {
	case err := <-done:
		if nil != err {
			t.Fatal(err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("commit not released after the other writer cancelled")
	}
	if 1 != calls.Load() {
		t.Fatalf("expected one sync, got %d", calls.Load())
	}
}

func TestDirSyncerMaxBatchReleasesEarly(t *testing.T) {
	var calls atomic.Int64
	s := newDirSyncer(SyncFull, 5*time.Second, 2, func(string) error {
		calls.Add(1)
		return nil
	})
	for range 3 {
		s.begin("d")
	}
	var wg sync.WaitGroup
	start := time.Now()
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.commit("d")
		}()
	}
	wg.Wait()
	if time.Second < time.Since(start) {
		t.Fatal("batch did not release at max size")
	}
	s.cancel("d")
}

func TestDirSyncerSeparateDirectoriesSyncSeparately(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	s := newDirSyncer(SyncFull, time.Millisecond, 8, func(dir string) error {
		mu.Lock()
		seen[dir]++
		mu.Unlock()
		return nil
	})
	for _, dir := range []string{"a", "b"} {
		s.begin(dir)
		if err := s.commit(dir); nil != err {
			t.Fatal(err)
		}
	}
	if 1 != seen["a"] || 1 != seen["b"] {
		t.Fatalf("unexpected sync counts: %v", seen)
	}
}

func TestDirSyncerNoneModeNeverSyncs(t *testing.T) {
	var calls atomic.Int64
	s := newDirSyncer(SyncNone, time.Millisecond, 8, func(string) error {
		calls.Add(1)
		return nil
	})
	s.begin("d")
	if err := s.commit("d"); nil != err {
		t.Fatal(err)
	}
	if err := s.sync("d"); nil != err {
		t.Fatal(err)
	}
	if 0 != calls.Load() {
		t.Fatalf("none mode synced %d times", calls.Load())
	}
}
