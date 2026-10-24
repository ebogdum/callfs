package locks

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	localLockTTL             = 30 * time.Second
	localLockCleanupInterval = time.Minute
	defaultWaitTimeout       = 30 * time.Second
	localLockShardCount      = 64
	localBackendName         = "local"
)

type lockEntry struct {
	expiry     time.Time
	acquiredAt time.Time
	ownerToken string
}

type lockShard struct {
	mu       sync.Mutex
	locks    map[string]lockEntry
	released chan struct{}
}

func (s *lockShard) releaseLocked(key string) {
	delete(s.locks, key)
	activeLocks.Dec()
	close(s.released)
	s.released = make(chan struct{})
}

type LocalManager struct {
	lockCounters
	shards      [localLockShardCount]lockShard
	waitTimeout atomic.Int64
	stopChan    chan struct{}
	closeOnce   sync.Once
}

type localLease struct {
	shard      *lockShard
	key        string
	ownerToken string
}

func NewLocalManager() *LocalManager {
	m := &LocalManager{
		stopChan: make(chan struct{}),
	}
	m.waitTimeout.Store(int64(defaultWaitTimeout))
	for i := range m.shards {
		m.shards[i].locks = make(map[string]lockEntry)
		m.shards[i].released = make(chan struct{})
	}
	go m.cleanupLoop()
	return m
}

func (m *LocalManager) SetWaitTimeout(d time.Duration) {
	m.waitTimeout.Store(int64(d))
}

func (m *LocalManager) shardFor(key string) *lockShard {
	var h uint32 = 2166136261
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return &m.shards[h%localLockShardCount]
}

func (m *LocalManager) Acquire(ctx context.Context, key string) (Lease, error) {
	ownerToken, err := generateOwnerToken()
	if err != nil {
		return nil, err
	}

	shard := m.shardFor(key)
	return waitForLock(ctx, time.Duration(m.waitTimeout.Load()), &m.lockCounters, func() (Lease, error) {
		return m.tryAcquire(shard, key, ownerToken)
	}, func() <-chan struct{} {
		shard.mu.Lock()
		defer shard.mu.Unlock()
		return shard.released
	})
}

func (m *LocalManager) tryAcquire(shard *lockShard, key, ownerToken string) (Lease, error) {
	shard.mu.Lock()
	defer shard.mu.Unlock()

	now := time.Now()
	if entry, exists := shard.locks[key]; exists {
		if now.Before(entry.expiry) {
			return nil, nil
		}
		m.recordLost()
		activeLocks.Dec()
	}

	shard.locks[key] = lockEntry{
		expiry:     now.Add(localLockTTL),
		acquiredAt: now,
		ownerToken: ownerToken,
	}
	return &localLease{shard: shard, key: key, ownerToken: ownerToken}, nil
}

func (l *localLease) Release(_ context.Context) error {
	shard := l.shard
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if entry, exists := shard.locks[l.key]; exists && entry.ownerToken == l.ownerToken {
		shard.releaseLocked(l.key)
		lockOperations.WithLabelValues("release", "ok").Inc()
	}
	return nil
}

func (m *LocalManager) Snapshot(_ context.Context) ([]HeldLock, error) {
	now := time.Now()
	var held []HeldLock
	for i := range m.shards {
		shard := &m.shards[i]
		shard.mu.Lock()
		for key, entry := range shard.locks {
			if now.Before(entry.expiry) {
				held = append(held, HeldLock{
					Key:        key,
					Backend:    localBackendName,
					Holder:     holderHint(entry.ownerToken),
					AcquiredAt: entry.acquiredAt,
					ExpiresAt:  entry.expiry,
				})
			}
		}
		shard.mu.Unlock()
	}
	sort.Slice(held, func(i, j int) bool {
		return held[i].Key < held[j].Key
	})
	return held, nil
}

func (m *LocalManager) ForceRelease(_ context.Context, key string) (bool, error) {
	shard := m.shardFor(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	entry, exists := shard.locks[key]
	if !exists || !time.Now().Before(entry.expiry) {
		return false, nil
	}
	shard.releaseLocked(key)
	lockOperations.WithLabelValues("force_release", "ok").Inc()
	return true, nil
}

func (m *LocalManager) Close() error {
	m.closeOnce.Do(func() { close(m.stopChan) })
	for i := range m.shards {
		shard := &m.shards[i]
		shard.mu.Lock()
		activeLocks.Sub(float64(len(shard.locks)))
		shard.locks = make(map[string]lockEntry)
		shard.mu.Unlock()
	}
	return nil
}

func (m *LocalManager) cleanupLoop() {
	ticker := time.NewTicker(localLockCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			for i := range m.shards {
				shard := &m.shards[i]
				shard.mu.Lock()
				now := time.Now()
				for key, entry := range shard.locks {
					if now.After(entry.expiry) {
						delete(shard.locks, key)
						m.recordLost()
						activeLocks.Dec()
					}
				}
				shard.mu.Unlock()
			}
		case <-m.stopChan:
			return
		}
	}
}
