package locks

import (
	"context"
	"sync"
)

// LocalManager provides in-process lock management for local/single-node deployments.
type LocalManager struct {
	mu    sync.Mutex
	locks map[string]struct{}
}

// NewLocalManager creates a new in-memory lock manager.
func NewLocalManager() *LocalManager {
	return &LocalManager{
		locks: make(map[string]struct{}),
	}
}

// Acquire acquires a lock if it is currently free.
func (m *LocalManager) Acquire(ctx context.Context, key string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.locks[key]; exists {
		return false, nil
	}

	m.locks[key] = struct{}{}
	return true, nil
}

// Release releases a previously acquired lock.
func (m *LocalManager) Release(ctx context.Context, key string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.locks, key)
	return nil
}

// Close clears all local locks.
func (m *LocalManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.locks = make(map[string]struct{})
	return nil
}
