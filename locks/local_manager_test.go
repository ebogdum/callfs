package locks

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLocalManagerAcquireWaitsForRelease(t *testing.T) {
	m := NewLocalManager()
	defer m.Close()

	first, err := m.Acquire(context.Background(), "path:/a")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = first.Release(context.Background())
	}()

	second, err := m.Acquire(context.Background(), "path:/a")
	if err != nil {
		t.Fatalf("waiting acquire: %v", err)
	}
	if err := second.Release(context.Background()); err != nil {
		t.Fatalf("second release: %v", err)
	}
}

func TestLocalManagerAcquireTimesOut(t *testing.T) {
	m := NewLocalManager()
	defer m.Close()
	m.SetWaitTimeout(50 * time.Millisecond)

	if _, err := m.Acquire(context.Background(), "path:/a"); err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	_, err := m.Acquire(context.Background(), "path:/a")
	if !errors.Is(err, ErrLockHeld) {
		t.Fatalf("expected ErrLockHeld, got %v", err)
	}
}

func TestLocalManagerAcquireStopsWhenContextEnds(t *testing.T) {
	m := NewLocalManager()
	defer m.Close()

	if _, err := m.Acquire(context.Background(), "path:/a"); err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := m.Acquire(ctx, "path:/a")
	if !errors.Is(err, ErrLockHeld) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected ErrLockHeld wrapping DeadlineExceeded, got %v", err)
	}
}

func TestLocalManagerStaleLeaseDoesNotReleaseNewOwner(t *testing.T) {
	m := NewLocalManager()
	defer m.Close()
	m.SetWaitTimeout(50 * time.Millisecond)

	stale, err := m.Acquire(context.Background(), "path:/a")
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	shard := m.shardFor("path:/a")
	shard.mu.Lock()
	entry := shard.locks["path:/a"]
	entry.expiry = time.Now().Add(-time.Second)
	shard.locks["path:/a"] = entry
	shard.mu.Unlock()

	current, err := m.Acquire(context.Background(), "path:/a")
	if err != nil {
		t.Fatalf("re-acquire after expiry: %v", err)
	}

	if err := stale.Release(context.Background()); err != nil {
		t.Fatalf("stale release: %v", err)
	}

	if _, err := m.Acquire(context.Background(), "path:/a"); !errors.Is(err, ErrLockHeld) {
		t.Fatalf("expected ErrLockHeld after stale release, got %v", err)
	}

	if err := current.Release(context.Background()); err != nil {
		t.Fatalf("current release: %v", err)
	}
	if err := current.Release(context.Background()); err != nil {
		t.Fatalf("double release: %v", err)
	}
}
