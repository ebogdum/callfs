package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

const (
	rateLimiterCleanupInterval = 5 * time.Minute
	rateLimiterEntryTTL        = 10 * time.Minute
	rateLimiterMaxEntries      = 100_000
	// rateLimiterEvictBatch is how many entries a single eviction frees once the
	// map is full, so the O(n) scan is amortized rather than paid per request.
	rateLimiterEvictBatch = 1_000
)

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// perIPRateLimiter tracks per-IP rate limiters with TTL-based eviction.
type perIPRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*limiterEntry
	rate     rate.Limit
	burst    int
	stopChan chan struct{}
}

func newPerIPRateLimiter(r rate.Limit, burst int) *perIPRateLimiter {
	p := &perIPRateLimiter{
		limiters: make(map[string]*limiterEntry),
		rate:     r,
		burst:    burst,
		stopChan: make(chan struct{}),
	}
	go p.cleanupLoop()
	return p
}

func (p *perIPRateLimiter) getLimiter(ip string) *rate.Limiter {
	p.mu.Lock()
	defer p.mu.Unlock()

	if entry, exists := p.limiters[ip]; exists {
		entry.lastSeen = time.Now()
		return entry.limiter
	}

	// Enforce max entries cap to prevent unbounded growth
	if len(p.limiters) >= rateLimiterMaxEntries {
		p.evictBatch()
	}

	limiter := rate.NewLimiter(p.rate, p.burst)
	p.limiters[ip] = &limiterEntry{
		limiter:  limiter,
		lastSeen: time.Now(),
	}
	return limiter
}

// evictBatch frees capacity when the limiter map is full (caller must hold lock).
//
// It first drops entries that are already past their TTL, and only if that frees
// nothing does it evict the oldest rateLimiterEvictBatch entries by lastSeen.
//
// Evicting a single entry per call would make every request from a new IP pay a
// full O(rateLimiterMaxEntries) scan while holding the mutex, once the map is
// full. An attacker rotating source addresses (trivial over IPv6) could hold the
// map at the cap and serialize all request handling behind that scan. Freeing a
// batch amortizes the scan over rateLimiterEvictBatch insertions instead.
func (p *perIPRateLimiter) evictBatch() {
	cutoff := time.Now().Add(-rateLimiterEntryTTL)
	evicted := 0
	for ip, entry := range p.limiters {
		if entry.lastSeen.Before(cutoff) {
			delete(p.limiters, ip)
			evicted++
		}
	}
	if evicted > 0 {
		return
	}

	// Nothing was stale, so every entry is live: drop the batch with the oldest
	// lastSeen. A partial selection sort over the batch size keeps this to a
	// single pass rather than sorting all rateLimiterMaxEntries entries.
	oldest := make([]limiterAge, 0, rateLimiterEvictBatch)
	for ip, entry := range p.limiters {
		if len(oldest) < rateLimiterEvictBatch {
			oldest = append(oldest, limiterAge{ip: ip, lastSeen: entry.lastSeen})
			continue
		}
		newestIdx := 0
		for i := 1; i < len(oldest); i++ {
			if oldest[i].lastSeen.After(oldest[newestIdx].lastSeen) {
				newestIdx = i
			}
		}
		if entry.lastSeen.Before(oldest[newestIdx].lastSeen) {
			oldest[newestIdx] = limiterAge{ip: ip, lastSeen: entry.lastSeen}
		}
	}
	for _, c := range oldest {
		delete(p.limiters, c.ip)
	}
}

// limiterAge pairs an IP with its last-seen time for batch eviction.
type limiterAge struct {
	ip       string
	lastSeen time.Time
}

// cleanupLoop periodically removes stale entries.
func (p *perIPRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rateLimiterCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			cutoff := time.Now().Add(-rateLimiterEntryTTL)
			for ip, entry := range p.limiters {
				if entry.lastSeen.Before(cutoff) {
					delete(p.limiters, ip)
				}
			}
			p.mu.Unlock()
		case <-p.stopChan:
			return
		}
	}
}

// stopPerIPLimiter stops the background cleanup goroutine. Exposed for graceful shutdown.
func (p *perIPRateLimiter) stop() {
	select {
	case <-p.stopChan:
		// Already stopped
	default:
		close(p.stopChan)
	}
}

// RateLimiterHandle wraps a per-IP rate limiter so its background goroutine can be stopped.
type RateLimiterHandle struct {
	perIP *perIPRateLimiter
}

// Stop stops the background cleanup goroutine. Call during graceful shutdown.
func (h *RateLimiterHandle) Stop() {
	h.perIP.stop()
}

// V1RateLimitMiddleware creates a middleware that applies per-IP rate limiting.
// The returned RateLimiterHandle must be stopped on shutdown to avoid goroutine leaks.
func V1RateLimitMiddleware(limiter *rate.Limiter, logger *zap.Logger) (*RateLimiterHandle, func(http.Handler) http.Handler) {
	perIP := newPerIPRateLimiter(limiter.Limit(), limiter.Burst())

	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if ip == "" {
				ip = r.RemoteAddr
			}

			if !perIP.getLimiter(ip).Allow() {
				logger.Warn("Request rate limited",
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.String("remote_addr", r.RemoteAddr))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				if _, err := w.Write([]byte(`{"code":"RATE_LIMIT_EXCEEDED","message":"Rate limit exceeded"}`)); err != nil {
					logger.Error("Failed to write rate limit error response", zap.Error(err))
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	}

	return &RateLimiterHandle{perIP: perIP}, mw
}
