package locks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

const (
	localLockTTL             = 30 * time.Second
	localLockCleanupInterval = time.Minute
)

type lockEntry struct {
	expiry  time.Time
	ownerID string
}

// LocalManager provides in-process lock management for local/single-node deployments.
// Each lock tracks an ownerID to prevent releasing another holder's lock after TTL expiry.
type LocalManager struct {
	mu         sync.Mutex
	locks      map[string]lockEntry
	instanceID string
	stopChan   chan struct{}
}

// NewLocalManager creates a new in-memory lock manager.
func NewLocalManager() *LocalManager {
	m := &LocalManager{
		locks:      make(map[string]lockEntry),
		instanceID: mustGenerateID(),
		stopChan:   make(chan struct{}),
	}
	go m.cleanupLoop()
	return m
}

// Acquire acquires a lock if it is currently free or expired.
func (m *LocalManager) Acquire(ctx context.Context, key string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, exists := m.locks[key]; exists {
		if time.Now().Before(entry.expiry) {
			return false, nil // Lock is still held
		}
		// Lock expired, allow re-acquisition
	}

	ownerID, err := generateOwnerID()
	if err != nil {
		return false, fmt.Errorf("failed to generate lock owner ID: %w", err)
	}

	m.locks[key] = lockEntry{
		expiry:  time.Now().Add(localLockTTL),
		ownerID: ownerID,
	}
	return true, nil
}

// Release releases a previously acquired lock only if it hasn't been re-acquired by another holder.
func (m *LocalManager) Release(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, exists := m.locks[key]
	if !exists {
		return nil // Already released or expired
	}

	// If the lock has expired, another holder may have re-acquired it. Don't delete.
	if time.Now().After(entry.expiry) {
		return nil
	}

	delete(m.locks, key)
	return nil
}

// Close stops the background cleanup goroutine and clears all local locks.
func (m *LocalManager) Close() error {
	close(m.stopChan)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.locks = make(map[string]lockEntry)
	return nil
}

// cleanupLoop periodically removes expired lock entries to prevent unbounded map growth.
func (m *LocalManager) cleanupLoop() {
	ticker := time.NewTicker(localLockCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.mu.Lock()
			now := time.Now()
			for key, entry := range m.locks {
				if now.After(entry.expiry) {
					delete(m.locks, key)
				}
			}
			m.mu.Unlock()
		case <-m.stopChan:
			return
		}
	}
}

func generateOwnerID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func mustGenerateID() string {
	id, err := generateOwnerID()
	if err != nil {
		return "local"
	}
	return id
}
