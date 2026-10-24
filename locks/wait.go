package locks

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	mrand "math/rand/v2"
	"sync/atomic"
	"time"

	"github.com/ebogdum/callfs/metrics"
)

const (
	lockPollInitialDelay = 10 * time.Millisecond
	lockPollMaxDelay     = 200 * time.Millisecond
	holderHintBytes      = 4
)

var (
	lockOperations        = metrics.LockOperationsTotal
	lockOperationDuration = metrics.LockOperationDuration
	activeLocks           = metrics.ActiveLocks
)

type lockCounters struct {
	acquired    atomic.Uint64
	timeouts    atomic.Uint64
	cancelled   atomic.Uint64
	errors      atomic.Uint64
	lostLeases  atomic.Uint64
	renewErrors atomic.Uint64
}

func (c *lockCounters) Stats() LockStats {
	return LockStats{
		Acquired:    c.acquired.Load(),
		Timeouts:    c.timeouts.Load(),
		Cancelled:   c.cancelled.Load(),
		Errors:      c.errors.Load(),
		LostLeases:  c.lostLeases.Load(),
		RenewErrors: c.renewErrors.Load(),
	}
}

func (c *lockCounters) recordAcquire(started time.Time, outcome string) {
	lockOperations.WithLabelValues("acquire", outcome).Inc()
	lockOperationDuration.WithLabelValues("acquire").Observe(time.Since(started).Seconds())
	switch outcome {
	case "acquired":
		c.acquired.Add(1)
		activeLocks.Inc()
	case "timeout":
		c.timeouts.Add(1)
	case "cancelled":
		c.cancelled.Add(1)
	default:
		c.errors.Add(1)
	}
}

func (c *lockCounters) recordLost() {
	c.lostLeases.Add(1)
	lockOperations.WithLabelValues("lease", "lost").Inc()
}

func (c *lockCounters) recordRenewError() {
	c.renewErrors.Add(1)
	lockOperations.WithLabelValues("renew", "error").Inc()
}

func waitForLock(ctx context.Context, timeout time.Duration, stats *lockCounters, try func() (Lease, error), released func() <-chan struct{}) (Lease, error) {
	started := time.Now()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	delay := lockPollInitialDelay
	for {
		var signal <-chan struct{}
		if released != nil {
			signal = released()
		}

		lease, err := try()
		if err != nil {
			stats.recordAcquire(started, "error")
			return nil, err
		}
		if lease != nil {
			stats.recordAcquire(started, "acquired")
			return lease, nil
		}

		wait := time.NewTimer(jitter(delay))
		select {
		case <-ctx.Done():
			wait.Stop()
			stats.recordAcquire(started, "cancelled")
			return nil, fmt.Errorf("%w: %w", ErrLockHeld, ctx.Err())
		case <-deadline.C:
			wait.Stop()
			stats.recordAcquire(started, "timeout")
			return nil, ErrLockHeld
		case <-signal:
			wait.Stop()
			continue
		case <-wait.C:
		}

		delay = min(delay*2, lockPollMaxDelay)
	}
}

func jitter(d time.Duration) time.Duration {
	half := d / 2
	return half + mrand.N(half+1)
}

func generateOwnerToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate lock owner token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func holderHint(ownerToken string) string {
	sum := sha256.Sum256([]byte(ownerToken))
	return hex.EncodeToString(sum[:holderHintBytes])
}
