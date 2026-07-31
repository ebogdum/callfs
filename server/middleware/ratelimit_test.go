package middleware

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestEvictBatchDropsStaleEntriesFirst checks that entries past their TTL are
// reclaimed before any live entry is touched.
func TestEvictBatchDropsStaleEntriesFirst(t *testing.T) {
	p := &perIPRateLimiter{
		limiters: make(map[string]*limiterEntry),
		rate:     rate.Limit(10),
		burst:    5,
		stopChan: make(chan struct{}),
	}

	stale := time.Now().Add(-2 * rateLimiterEntryTTL)
	live := time.Now()
	for i := 0; i < 100; i++ {
		p.limiters[fmt.Sprintf("stale-%d", i)] = &limiterEntry{limiter: rate.NewLimiter(p.rate, p.burst), lastSeen: stale}
	}
	for i := 0; i < 50; i++ {
		p.limiters[fmt.Sprintf("live-%d", i)] = &limiterEntry{limiter: rate.NewLimiter(p.rate, p.burst), lastSeen: live}
	}

	p.evictBatch()

	if len(p.limiters) != 50 {
		t.Errorf("after evictBatch got %d entries, want 50 (all stale dropped, all live kept)", len(p.limiters))
	}
	for i := 0; i < 50; i++ {
		if _, ok := p.limiters[fmt.Sprintf("live-%d", i)]; !ok {
			t.Fatalf("evictBatch dropped live entry live-%d while stale entries existed", i)
		}
	}
}

// TestEvictBatchFreesBatchWhenAllLive is the regression guard for the eviction
// cost. When nothing is stale, a single eviction must free a whole batch, not
// one entry — otherwise every request from a new IP pays a full scan of the map
// under the mutex once it is full, which an IP-rotating client can hold open.
func TestEvictBatchFreesBatchWhenAllLive(t *testing.T) {
	p := &perIPRateLimiter{
		limiters: make(map[string]*limiterEntry),
		rate:     rate.Limit(10),
		burst:    5,
		stopChan: make(chan struct{}),
	}

	now := time.Now()
	const total = 5_000
	for i := 0; i < total; i++ {
		// Older index => older lastSeen, all still well inside the TTL.
		p.limiters[fmt.Sprintf("ip-%d", i)] = &limiterEntry{
			limiter:  rate.NewLimiter(p.rate, p.burst),
			lastSeen: now.Add(time.Duration(i) * time.Millisecond),
		}
	}

	p.evictBatch()

	want := total - rateLimiterEvictBatch
	if len(p.limiters) != want {
		t.Errorf("after evictBatch got %d entries, want %d (a full batch must be freed)", len(p.limiters), want)
	}

	// The evicted entries must be the oldest ones.
	for i := 0; i < rateLimiterEvictBatch; i++ {
		if _, ok := p.limiters[fmt.Sprintf("ip-%d", i)]; ok {
			t.Fatalf("evictBatch kept ip-%d, which is among the oldest %d entries", i, rateLimiterEvictBatch)
		}
	}
	for i := rateLimiterEvictBatch; i < total; i++ {
		if _, ok := p.limiters[fmt.Sprintf("ip-%d", i)]; !ok {
			t.Fatalf("evictBatch dropped ip-%d, which is newer than the batch it should have evicted", i)
		}
	}
}

// TestGetLimiterStaysUnderCap checks the map never exceeds its configured cap
// and that repeated distinct IPs keep working.
func TestGetLimiterStaysUnderCap(t *testing.T) {
	p := &perIPRateLimiter{
		limiters: make(map[string]*limiterEntry),
		rate:     rate.Limit(10),
		burst:    5,
		stopChan: make(chan struct{}),
	}

	now := time.Now()
	for i := 0; i < rateLimiterMaxEntries; i++ {
		p.limiters[fmt.Sprintf("ip-%d", i)] = &limiterEntry{
			limiter:  rate.NewLimiter(p.rate, p.burst),
			lastSeen: now.Add(time.Duration(i) * time.Microsecond),
		}
	}

	for i := 0; i < 10; i++ {
		if l := p.getLimiter(fmt.Sprintf("fresh-%d", i)); l == nil {
			t.Fatalf("getLimiter returned nil for fresh-%d", i)
		}
	}
	if len(p.limiters) > rateLimiterMaxEntries {
		t.Errorf("limiter map grew to %d, above cap %d", len(p.limiters), rateLimiterMaxEntries)
	}
}

// TestGetLimiterReusesEntryPerIP confirms an IP keeps the same limiter across
// calls, so its budget is actually enforced rather than reset each request.
func TestGetLimiterReusesEntryPerIP(t *testing.T) {
	p := newPerIPRateLimiter(rate.Limit(1), 1)
	defer p.stop()

	first := p.getLimiter("198.51.100.4")
	second := p.getLimiter("198.51.100.4")
	if first != second {
		t.Error("getLimiter returned a different limiter for the same IP; per-IP budget would never be enforced")
	}
	other := p.getLimiter("198.51.100.5")
	if other == first {
		t.Error("getLimiter shared one limiter across distinct IPs")
	}

	if !first.Allow() {
		t.Error("first request from an IP should be allowed by the burst")
	}
	if first.Allow() {
		t.Error("second immediate request should be rate limited with rate=1 burst=1")
	}
	if !other.Allow() {
		t.Error("a different IP must have its own independent budget")
	}
}
