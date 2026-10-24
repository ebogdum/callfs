package localfs

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDirSyncerSeparateDirectoriesSyncSeparately(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	s := newDirSyncer(SyncFull, time.Millisecond, func(dir string) error {
		mu.Lock()
		seen[dir]++
		mu.Unlock()
		return nil
	})
	for _, dir := range []string{"a", "b"} {
		if err := s.sync(dir); nil != err {
			t.Fatal(err)
		}
	}
	if 1 != seen["a"] || 1 != seen["b"] {
		t.Fatalf("unexpected sync counts: %v", seen)
	}
}

func TestDirSyncerNoneModeNeverSyncs(t *testing.T) {
	var calls atomic.Int64
	s := newDirSyncer(SyncNone, time.Millisecond, func(string) error {
		calls.Add(1)
		return nil
	})
	if err := s.sync("d"); nil != err {
		t.Fatal(err)
	}
	if 0 != calls.Load() {
		t.Fatalf("none mode synced %d times", calls.Load())
	}
}
