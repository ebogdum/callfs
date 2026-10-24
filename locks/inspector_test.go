package locks

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

type fakeRedisStore struct {
	mu     sync.Mutex
	values map[string]string
	ttls   map[string]time.Duration
}

func newFakeRedisStore() *fakeRedisStore {
	return &fakeRedisStore{
		values: make(map[string]string),
		ttls:   make(map[string]time.Duration),
	}
}

func (f *fakeRedisStore) hold(key, token string, ttl time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[key] = token
	f.ttls[key] = ttl
}

func (f *fakeRedisStore) Scan(_ context.Context, _ uint64, match string, _ int64) ([]string, uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	prefix := strings.TrimSuffix(match, "*")
	var keys []string
	for key := range f.values {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, 0, nil
}

func (f *fakeRedisStore) Get(_ context.Context, key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	value, ok := f.values[key]
	if !ok {
		return "", redis.Nil
	}
	return value, nil
}

func (f *fakeRedisStore) PTTL(_ context.Context, key string) (time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ttl, ok := f.ttls[key]
	if !ok {
		return -2, nil
	}
	return ttl, nil
}

func (f *fakeRedisStore) Del(_ context.Context, key string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.values[key]; !ok {
		return 0, nil
	}
	delete(f.values, key)
	delete(f.ttls, key)
	return 1, nil
}

func TestLocalInspectorSnapshotListsHeldLocks(t *testing.T) {
	m := NewLocalManager()
	defer m.Close()

	lease, err := m.Acquire(context.Background(), "path:/a")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	held, err := m.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if 1 != len(held) {
		t.Fatalf("snapshot length = %d, want 1", len(held))
	}
	if "path:/a" != held[0].Key {
		t.Fatalf("snapshot key = %q, want path:/a", held[0].Key)
	}
	if "local" != held[0].Backend {
		t.Fatalf("snapshot backend = %q, want local", held[0].Backend)
	}
	if 2*holderHintBytes != len(held[0].Holder) {
		t.Fatalf("holder hint = %q, want %d hex chars", held[0].Holder, 2*holderHintBytes)
	}
	if !held[0].ExpiresAt.After(time.Now()) {
		t.Fatalf("expires at %v is not in the future", held[0].ExpiresAt)
	}

	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("release: %v", err)
	}
	held, err = m.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot after release: %v", err)
	}
	if 0 != len(held) {
		t.Fatalf("snapshot after release length = %d, want 0", len(held))
	}
}

func TestLocalInspectorStatsCountOutcomes(t *testing.T) {
	m := NewLocalManager()
	defer m.Close()
	m.SetWaitTimeout(10 * time.Millisecond)

	lease, err := m.Acquire(context.Background(), "path:/a")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	if _, err := m.Acquire(context.Background(), "path:/a"); !errors.Is(err, ErrLockHeld) {
		t.Fatalf("contended acquire error = %v, want ErrLockHeld", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Acquire(cancelled, "path:/a"); !errors.Is(err, ErrLockHeld) {
		t.Fatalf("cancelled acquire error = %v, want ErrLockHeld", err)
	}

	stats := m.Stats()
	if 1 != stats.Acquired {
		t.Fatalf("acquired = %d, want 1", stats.Acquired)
	}
	if 1 != stats.Timeouts {
		t.Fatalf("timeouts = %d, want 1", stats.Timeouts)
	}
	if 1 != stats.Cancelled {
		t.Fatalf("cancelled = %d, want 1", stats.Cancelled)
	}

	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("release: %v", err)
	}
}

func TestLocalInspectorForceReleaseFreesLock(t *testing.T) {
	m := NewLocalManager()
	defer m.Close()

	lease, err := m.Acquire(context.Background(), "path:/a")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	released, err := m.ForceRelease(context.Background(), "path:/a")
	if err != nil {
		t.Fatalf("force release: %v", err)
	}
	if !released {
		t.Fatal("force release of a held lock reported false")
	}

	released, err = m.ForceRelease(context.Background(), "path:/a")
	if err != nil {
		t.Fatalf("second force release: %v", err)
	}
	if released {
		t.Fatal("second force release reported true")
	}

	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("stale release: %v", err)
	}

	next, err := m.Acquire(context.Background(), "path:/a")
	if err != nil {
		t.Fatalf("acquire after force release: %v", err)
	}
	if err := next.Release(context.Background()); err != nil {
		t.Fatalf("release after force release: %v", err)
	}
}

func TestRedisInspectorSnapshotAndForceRelease(t *testing.T) {
	store := newFakeRedisStore()
	store.hold("callfs:lock:path:/a", "token-a", 20*time.Second)
	store.hold("callfs:lock:path:/b", "token-b", 10*time.Second)
	m := &RedisManager{inspector: store}

	held, err := m.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if 2 != len(held) {
		t.Fatalf("snapshot length = %d, want 2", len(held))
	}
	byKey := make(map[string]HeldLock, len(held))
	for _, lock := range held {
		byKey[lock.Key] = lock
	}
	lockA, ok := byKey["path:/a"]
	if !ok {
		t.Fatal("snapshot missing path:/a")
	}
	if "redis" != lockA.Backend {
		t.Fatalf("backend = %q, want redis", lockA.Backend)
	}
	if lockA.Holder != holderHint("token-a") {
		t.Fatalf("holder = %q, want hint of owner token", lockA.Holder)
	}
	if strings.Contains(lockA.Holder, "token-a") {
		t.Fatalf("holder leaks owner token: %q", lockA.Holder)
	}

	released, err := m.ForceRelease(context.Background(), "path:/a")
	if err != nil {
		t.Fatalf("force release: %v", err)
	}
	if !released {
		t.Fatal("force release of a held lock reported false")
	}

	held, err = m.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot after force release: %v", err)
	}
	if 1 != len(held) {
		t.Fatalf("snapshot after force release length = %d, want 1", len(held))
	}

	released, err = m.ForceRelease(context.Background(), "path:/a")
	if err != nil {
		t.Fatalf("second force release: %v", err)
	}
	if released {
		t.Fatal("second force release reported true")
	}
}
