package middleware

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestEvictStaleDropsStaleEntriesKeepsLive(t *testing.T) {
	p := &perIPRateLimiter{
		limiters: make(map[string]*limiterEntry),
		rate:     rate.Limit(10),
		burst:    5,
		stopChan: make(chan struct{}),
	}

	stale := time.Now().Add(-2 * rateLimiterEntryTTL)
	live := time.Now()
	for i := 0; i < 100; i++ {
		p.limiters[fmt.Sprintf("stale-%d", i)] = newLimiterEntry(rate.NewLimiter(p.rate, p.burst), stale)
	}
	for i := 0; i < 50; i++ {
		p.limiters[fmt.Sprintf("live-%d", i)] = newLimiterEntry(rate.NewLimiter(p.rate, p.burst), live)
	}

	p.evictStale()

	if 50 != len(p.limiters) {
		t.Errorf("after evictStale got %d entries, want 50 (all stale dropped, all live kept)", len(p.limiters))
	}
	for i := 0; i < 50; i++ {
		if _, ok := p.limiters[fmt.Sprintf("live-%d", i)]; !ok {
			t.Fatalf("evictStale dropped live entry live-%d", i)
		}
	}
}

func TestEvictStaleKeepsAllLiveEntries(t *testing.T) {
	p := &perIPRateLimiter{
		limiters: make(map[string]*limiterEntry),
		rate:     rate.Limit(10),
		burst:    5,
		stopChan: make(chan struct{}),
	}

	now := time.Now()
	const total = 5_000
	for i := 0; i < total; i++ {
		p.limiters[fmt.Sprintf("ip-%d", i)] = newLimiterEntry(rate.NewLimiter(p.rate, p.burst), now.Add(time.Duration(i)*time.Millisecond))
	}

	p.evictStale()

	if total != len(p.limiters) {
		t.Errorf("after evictStale got %d entries, want %d (no live entry may be evicted)", len(p.limiters), total)
	}
	for i := 0; i < total; i++ {
		if _, ok := p.limiters[fmt.Sprintf("ip-%d", i)]; !ok {
			t.Fatalf("evictStale dropped live entry ip-%d", i)
		}
	}
}

func TestGetLimiterStaysUnderCap(t *testing.T) {
	p := &perIPRateLimiter{
		limiters: make(map[string]*limiterEntry),
		rate:     rate.Limit(10),
		burst:    5,
		stopChan: make(chan struct{}),
	}

	now := time.Now()
	for i := 0; i < rateLimiterMaxEntries; i++ {
		p.limiters[fmt.Sprintf("ip-%d", i)] = newLimiterEntry(rate.NewLimiter(p.rate, p.burst), now.Add(time.Duration(i)*time.Microsecond))
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
