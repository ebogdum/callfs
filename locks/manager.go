package locks

import (
	"context"
	"errors"
	"time"
)

var ErrLockHeld = errors.New("lock is held by another owner")

type Lease interface {
	Release(ctx context.Context) error
}

type Manager interface {
	Acquire(ctx context.Context, key string) (Lease, error)

	Close() error
}

type HeldLock struct {
	Key        string
	Backend    string
	Holder     string
	AcquiredAt time.Time
	ExpiresAt  time.Time
}

type LockStats struct {
	Acquired    uint64
	Timeouts    uint64
	Cancelled   uint64
	Errors      uint64
	LostLeases  uint64
	RenewErrors uint64
	WaitP50Ms   *int64
	WaitP95Ms   *int64
}

type Inspector interface {
	Snapshot(ctx context.Context) ([]HeldLock, error)
	Stats() LockStats
	ForceRelease(ctx context.Context, key string) (bool, error)
}

var (
	_ Inspector = (*LocalManager)(nil)
	_ Inspector = (*RedisManager)(nil)
)
