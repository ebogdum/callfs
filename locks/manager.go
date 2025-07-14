package locks

import (
	"context"
)

// Manager defines the interface for distributed locking operations
type Manager interface {
	// Acquire attempts to acquire a distributed lock for the given key
	// Returns true if the lock was acquired, false if it was already held by another process
	Acquire(ctx context.Context, key string) (bool, error)

	// Release releases a previously acquired lock for the given key
	// Only the process that acquired the lock can release it
	Release(ctx context.Context, key string) error

	// Close closes the lock manager and releases any resources
	Close() error
}
