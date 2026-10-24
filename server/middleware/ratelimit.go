package middleware

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"github.com/ebogdum/callfs/v2/metrics"
)

const (
	rateLimiterCleanupInterval = 5 * time.Minute
	rateLimiterEntryTTL        = 10 * time.Minute
	rateLimiterMaxEntries      = 100_000
	rateLimiterCleanupChunk    = 1_000
	rateLimiterEvictSample     = 16
	rateLimitRejectionRingSize = 1024
	rateLimitTopClientsLimit   = 10

	RateLimitDownload     = "download"
	RateLimitLinkGenerate = "link_generate"
	RateLimitSession      = "session"
)

var (
	ErrUnknownRateLimitEndpoint = errors.New("unknown rate limit endpoint")
	ErrInvalidRateLimit         = errors.New("rate limit must be above zero and burst at least 1")
	ErrRateLimitNotConfigured   = errors.New("rate limit endpoint is not configured")
)

var rateLimitEndpointNames = []string{RateLimitDownload, RateLimitLinkGenerate, RateLimitSession}

type limiterEntry struct {
	limiter       *rate.Limiter
	lastSeenNanos atomic.Int64
}

func newLimiterEntry(limiter *rate.Limiter, seen time.Time) *limiterEntry {
	entry := &limiterEntry{limiter: limiter}
	entry.lastSeenNanos.Store(seen.UnixNano())
	return entry
}

func (e *limiterEntry) lastSeen() int64 {
	return e.lastSeenNanos.Load()
}

func (e *limiterEntry) touch(now int64) {
	e.lastSeenNanos.Store(now)
}

type perIPRateLimiter struct {
	mu        sync.RWMutex
	limiters  map[string]*limiterEntry
	rate      rate.Limit
	burst     int
	evictions atomic.Uint64
	stopChan  chan struct{}
	stopOnce  sync.Once
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

func rateLimitKey(ip string) string {
	parsed := net.ParseIP(ip)
	if nil == parsed || nil != parsed.To4() {
		return ip
	}
	return parsed.Mask(net.CIDRMask(64, 128)).String()
}

func (p *perIPRateLimiter) getLimiter(ip string) *rate.Limiter {
	key := rateLimitKey(ip)
	now := time.Now().UnixNano()

	p.mu.RLock()
	if entry, exists := p.limiters[key]; exists {
		entry.touch(now)
		p.mu.RUnlock()
		return entry.limiter
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if entry, exists := p.limiters[key]; exists {
		entry.touch(now)
		return entry.limiter
	}

	if rateLimiterMaxEntries <= len(p.limiters) {
		p.evictStale()
		if rateLimiterMaxEntries <= len(p.limiters) {
			p.evictLeastRecentSampled()
		}
	}

	limiter := rate.NewLimiter(p.rate, p.burst)
	p.limiters[key] = newLimiterEntry(limiter, time.Unix(0, now))
	return limiter
}

func (p *perIPRateLimiter) evictStale() {
	cutoff := time.Now().Add(-rateLimiterEntryTTL).UnixNano()
	for key, entry := range p.limiters {
		if entry.lastSeen() < cutoff {
			delete(p.limiters, key)
			p.evictions.Add(1)
		}
	}
}

func (p *perIPRateLimiter) evictLeastRecentSampled() {
	var oldestKey string
	var oldestSeen int64
	sampled := 0
	for key, entry := range p.limiters {
		seen := entry.lastSeen()
		if 0 == sampled || seen < oldestSeen {
			oldestKey = key
			oldestSeen = seen
		}
		sampled++
		if rateLimiterEvictSample <= sampled {
			break
		}
	}
	if 0 < sampled {
		delete(p.limiters, oldestKey)
		p.evictions.Add(1)
	}
}

func (p *perIPRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rateLimiterCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.removeStale()
		case <-p.stopChan:
			return
		}
	}
}

func (p *perIPRateLimiter) removeStale() {
	cutoff := time.Now().Add(-rateLimiterEntryTTL).UnixNano()

	p.mu.RLock()
	stale := make([]string, 0)
	for ip, entry := range p.limiters {
		if entry.lastSeen() < cutoff {
			stale = append(stale, ip)
		}
	}
	p.mu.RUnlock()

	for start := 0; start < len(stale); start += rateLimiterCleanupChunk {
		end := min(start+rateLimiterCleanupChunk, len(stale))
		p.mu.Lock()
		for _, ip := range stale[start:end] {
			if entry, ok := p.limiters[ip]; ok && entry.lastSeen() < cutoff {
				delete(p.limiters, ip)
			}
		}
		p.mu.Unlock()
	}
}

func (p *perIPRateLimiter) settings() (rate.Limit, int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.rate, p.burst
}

func (p *perIPRateLimiter) setSettings(r rate.Limit, burst int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rate = r
	p.burst = burst
	for _, entry := range p.limiters {
		entry.limiter.SetLimit(r)
		entry.limiter.SetBurst(burst)
	}
}

func (p *perIPRateLimiter) trackedClients() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.limiters)
}

func (p *perIPRateLimiter) stop() {
	p.stopOnce.Do(func() {
		close(p.stopChan)
	})
}

type RateLimiterHandle struct {
	perIP *perIPRateLimiter
}

func (h *RateLimiterHandle) Stop() {
	h.perIP.stop()
}

func clientIP(remoteAddr string) string {
	ip, _, _ := net.SplitHostPort(remoteAddr)
	if "" == ip {
		return remoteAddr
	}
	return ip
}

func retryAfterSeconds(limit rate.Limit) int {
	if 0 >= limit {
		return 1
	}
	return max(1, int(math.Ceil(1/float64(limit))))
}

func writeRateLimited(w http.ResponseWriter, limit rate.Limit, logger *zap.Logger) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(limit)))
	w.WriteHeader(http.StatusTooManyRequests)
	if _, err := w.Write([]byte(`{"code":"RATE_LIMIT_EXCEEDED","message":"Rate limit exceeded"}`)); nil != err {
		logger.Error("Failed to write rate limit error response", zap.Error(err))
	}
}

type ClusterCounters interface {
	Allow(ctx context.Context, endpoint, client string, rps float64, burst int) (bool, error)
	ReportFailure(op string, err error)
}

type clusterDecision func(ctx context.Context, client string) (allowed, decided bool)

func limitPerIP(perIP *perIPRateLimiter, onReject func(client string), cluster clusterDecision, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r.RemoteAddr)
			limiter := perIP.getLimiter(ip)
			permitted := false
			decided := false
			if nil != cluster {
				permitted, decided = cluster(r.Context(), ip)
			}
			if !decided {
				permitted = limiter.Allow()
			}
			if permitted {
				next.ServeHTTP(w, r)
				return
			}
			if nil != onReject {
				onReject(ip)
			}
			writeRateLimited(w, limiter.Limit(), logger)
			logger.Warn("Request rate limited",
				zap.String("method", r.Method),
				zap.String("path", RedactDownloadPath(r.URL.Path)),
				zap.String("remote_addr", r.RemoteAddr))
		})
	}
}

func V1RateLimitMiddleware(limiter *rate.Limiter, logger *zap.Logger) (*RateLimiterHandle, func(http.Handler) http.Handler) {
	perIP := newPerIPRateLimiter(limiter.Limit(), limiter.Burst())
	return &RateLimiterHandle{perIP: perIP}, limitPerIP(perIP, nil, nil, logger)
}

func RedactDownloadPath(path string) string {
	if strings.HasPrefix(path, "/download/") {
		return "/download/{token}"
	}
	return path
}

type endpointRateLimit struct {
	perIP      *perIPRateLimiter
	rejections atomic.Uint64
}

type RateLimitRegistry struct {
	mu         sync.RWMutex
	endpoints  map[string]*endpointRateLimit
	rejections *rejectionRing
	logger     *zap.Logger
	counterMu  sync.RWMutex
	counters   ClusterCounters
}

type RateLimitSetting struct {
	Endpoint string
	RPS      float64
	Burst    int
}

type RateLimitTopClient struct {
	Client     string
	Endpoint   string
	Rejections int
	LastAt     time.Time
}

type RateLimitStats struct {
	Limits          []RateLimitSetting
	TrackedClients  int
	MaxEntries      int
	TTLSeconds      int
	RejectionsTotal map[string]uint64
	EvictionsTotal  uint64
	TopClients      []RateLimitTopClient
}

func NewRateLimitRegistry(logger *zap.Logger) *RateLimitRegistry {
	return &RateLimitRegistry{
		endpoints:  make(map[string]*endpointRateLimit),
		rejections: &rejectionRing{},
		logger:     logger,
	}
}

func (r *RateLimitRegistry) SetCounters(counters ClusterCounters) {
	r.counterMu.Lock()
	defer r.counterMu.Unlock()
	r.counters = counters
}

func (r *RateLimitRegistry) clusterCounters() ClusterCounters {
	r.counterMu.RLock()
	defer r.counterMu.RUnlock()
	return r.counters
}

func (r *RateLimitRegistry) SetLimits(endpoint string, rps float64, burst int) error {
	if !slices.Contains(rateLimitEndpointNames, endpoint) {
		return fmt.Errorf("%w: %s", ErrUnknownRateLimitEndpoint, endpoint)
	}
	if !(0 < rps) || math.IsInf(rps, 1) || 1 > burst {
		return ErrInvalidRateLimit
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if entry, configured := r.endpoints[endpoint]; configured {
		entry.perIP.setSettings(rate.Limit(rps), burst)
		return nil
	}
	r.endpoints[endpoint] = &endpointRateLimit{perIP: newPerIPRateLimiter(rate.Limit(rps), burst)}
	return nil
}

func (r *RateLimitRegistry) Middleware(endpoint string) (func(http.Handler) http.Handler, error) {
	r.mu.RLock()
	entry, configured := r.endpoints[endpoint]
	r.mu.RUnlock()
	if !configured {
		return nil, fmt.Errorf("%w: %s", ErrRateLimitNotConfigured, endpoint)
	}

	onReject := func(client string) {
		entry.rejections.Add(1)
		metrics.RateLimitRejectionsTotal.WithLabelValues(endpoint).Inc()
		r.rejections.add(rejectionEvent{client: client, endpoint: endpoint, at: time.Now()})
	}
	cluster := func(ctx context.Context, client string) (bool, bool) {
		counters := r.clusterCounters()
		if nil == counters {
			return false, false
		}
		rps, burst := entry.perIP.settings()
		allowed, err := counters.Allow(ctx, endpoint, rateLimitKey(client), float64(rps), burst)
		if nil != err {
			counters.ReportFailure("rate_limit."+endpoint, err)
			return false, false
		}
		return allowed, true
	}
	return limitPerIP(entry.perIP, onReject, cluster, r.logger), nil
}

func (r *RateLimitRegistry) Stats() RateLimitStats {
	stats := RateLimitStats{
		RejectionsTotal: make(map[string]uint64),
		MaxEntries:      rateLimiterMaxEntries,
		TTLSeconds:      int(rateLimiterEntryTTL / time.Second),
	}

	r.mu.RLock()
	for _, name := range rateLimitEndpointNames {
		entry, configured := r.endpoints[name]
		if !configured {
			continue
		}
		rps, burst := entry.perIP.settings()
		stats.Limits = append(stats.Limits, RateLimitSetting{Endpoint: name, RPS: float64(rps), Burst: burst})
		stats.TrackedClients += entry.perIP.trackedClients()
		stats.EvictionsTotal += entry.perIP.evictions.Load()
		stats.RejectionsTotal[name] = entry.rejections.Load()
	}
	r.mu.RUnlock()

	stats.TopClients = r.rejections.topClients(rateLimitTopClientsLimit)
	return stats
}

func (r *RateLimitRegistry) Stop() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, entry := range r.endpoints {
		entry.perIP.stop()
	}
}

type rejectionEvent struct {
	client   string
	endpoint string
	at       time.Time
}

type rejectionRing struct {
	mu     sync.Mutex
	events []rejectionEvent
	next   int
}

func (g *rejectionRing) add(event rejectionEvent) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if rateLimitRejectionRingSize > len(g.events) {
		g.events = append(g.events, event)
		return
	}
	g.events[g.next] = event
	g.next = (g.next + 1) % rateLimitRejectionRingSize
}

func (g *rejectionRing) topClients(limit int) []RateLimitTopClient {
	type clientKey struct {
		client   string
		endpoint string
	}

	counts := make(map[clientKey]*RateLimitTopClient)
	g.mu.Lock()
	for _, event := range g.events {
		key := clientKey{client: event.client, endpoint: event.endpoint}
		top, exists := counts[key]
		if !exists {
			top = &RateLimitTopClient{Client: event.client, Endpoint: event.endpoint}
			counts[key] = top
		}
		top.Rejections++
		if top.LastAt.Before(event.at) {
			top.LastAt = event.at
		}
	}
	g.mu.Unlock()

	result := make([]RateLimitTopClient, 0, len(counts))
	for _, top := range counts {
		result = append(result, *top)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Rejections != b.Rejections {
			return a.Rejections > b.Rejections
		}
		if !a.LastAt.Equal(b.LastAt) {
			return a.LastAt.After(b.LastAt)
		}
		if a.Client != b.Client {
			return a.Client < b.Client
		}
		return a.Endpoint < b.Endpoint
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}
