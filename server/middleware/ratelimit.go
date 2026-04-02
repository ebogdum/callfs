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
		p.evictOldest()
	}

	limiter := rate.NewLimiter(p.rate, p.burst)
	p.limiters[ip] = &limiterEntry{
		limiter:  limiter,
		lastSeen: time.Now(),
	}
	return limiter
}

// evictOldest removes the oldest entry (caller must hold lock).
func (p *perIPRateLimiter) evictOldest() {
	var oldestIP string
	var oldestTime time.Time
	first := true
	for ip, entry := range p.limiters {
		if first || entry.lastSeen.Before(oldestTime) {
			oldestIP = ip
			oldestTime = entry.lastSeen
			first = false
		}
	}
	if !first {
		delete(p.limiters, oldestIP)
	}
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

// V1RateLimitMiddleware creates a middleware that applies per-IP rate limiting.
func V1RateLimitMiddleware(limiter *rate.Limiter, logger *zap.Logger) func(http.Handler) http.Handler {
	perIP := newPerIPRateLimiter(limiter.Limit(), limiter.Burst())

	return func(next http.Handler) http.Handler {
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
}
