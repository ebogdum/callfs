package admin

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/locks"
	"github.com/ebogdum/callfs/v2/server/handlers"
	"github.com/ebogdum/callfs/v2/server/middleware"
)

const (
	lockPathKeyPrefix      = "path:"
	lockReasonMaxBytes     = 500
	lockBackendRedis       = "redis"
	lockBackendNATS        = "nats"
	lockBackendLocal       = "local"
	lockDefaultListLimit   = 200
	lockMaxListLimit       = 1000
	cacheInvalidateMaxSize = adminPathMaxBytes
	lockLeaseTTL           = 30 * time.Second
)

type lockStatsResponse struct {
	Acquired    uint64 `json:"acquired"`
	Timeouts    uint64 `json:"timeouts"`
	Cancelled   uint64 `json:"cancelled"`
	Errors      uint64 `json:"errors"`
	Lost        uint64 `json:"lost"`
	RenewErrors uint64 `json:"renew_errors"`
	WaitP50Ms   *int64 `json:"wait_p50_ms"`
	WaitP95Ms   *int64 `json:"wait_p95_ms"`
}

type heldLockResponse struct {
	Key        string     `json:"key"`
	Path       string     `json:"path"`
	OwnerHint  string     `json:"owner_hint"`
	AcquiredAt *time.Time `json:"acquired_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

type locksResponse struct {
	Type          string             `json:"type"`
	WaitTimeoutMs int64              `json:"wait_timeout_ms"`
	TTLMs         int64              `json:"ttl_ms"`
	Held          []heldLockResponse `json:"held"`
	Stats         lockStatsResponse  `json:"stats"`
}

type forceReleaseRequest struct {
	Reason string `json:"reason"`
}

type forceReleaseResponse struct {
	Released bool `json:"released"`
}

type cacheResponse struct {
	Enabled        bool       `json:"enabled"`
	TTLMs          int64      `json:"ttl_ms"`
	Entries        int        `json:"entries"`
	MaxEntries     int        `json:"max_entries"`
	Hits           uint64     `json:"hits"`
	Misses         uint64     `json:"misses"`
	Evictions      uint64     `json:"evictions"`
	ExpiredCleaned uint64     `json:"expired_cleaned"`
	LastCleanupAt  *time.Time `json:"last_cleanup_at"`
}

type cacheInvalidateRequest struct {
	Prefix string `json:"prefix"`
	All    bool   `json:"all"`
}

type cacheInvalidateResponse struct {
	Invalidated int `json:"invalidated"`
}

type rateLimitSettingResponse struct {
	Endpoint string  `json:"endpoint"`
	RPS      float64 `json:"rps"`
	Burst    int     `json:"burst"`
}

type rateLimitClientResponse struct {
	Client     string     `json:"client"`
	Endpoint   string     `json:"endpoint"`
	Rejections int        `json:"rejections"`
	LastAt     *time.Time `json:"last_at"`
}

type rateLimitsResponse struct {
	Limits          []rateLimitSettingResponse `json:"limits"`
	TrackedClients  int                        `json:"tracked_clients"`
	MaxEntries      int                        `json:"max_entries"`
	TTLSeconds      int                        `json:"ttl_s"`
	RejectionsTotal map[string]uint64          `json:"rejections_total"`
	EvictionsTotal  uint64                     `json:"evictions_total"`
	TopClients      []rateLimitClientResponse  `json:"top_clients"`
}

func (a *API) Locks(w http.ResponseWriter, r *http.Request) {
	logger := a.Deps.Logger
	if nil == a.Deps.Locks {
		writeFeatureDisabled(w, "locks")
		return
	}
	limit, err := parseRuntimeLimit(r.URL.Query().Get("limit"))
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusBadRequest)
		return
	}
	held, err := a.Deps.Locks.Snapshot(r.Context())
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	stats := a.Deps.Locks.Stats()
	handlers.SendJSONResponse(w, locksResponse{
		Type:          lockBackendName(a.Deps.Config.DLM.Type),
		WaitTimeoutMs: a.Deps.Config.DLM.WaitTimeout.Milliseconds(),
		TTLMs:         lockLeaseTTL.Milliseconds(),
		Held:          heldLockItems(held, r.URL.Query().Get("path"), limit),
		Stats: lockStatsResponse{
			Acquired:    stats.Acquired,
			Timeouts:    stats.Timeouts,
			Cancelled:   stats.Cancelled,
			Errors:      stats.Errors,
			Lost:        stats.LostLeases,
			RenewErrors: stats.RenewErrors,
			WaitP50Ms:   stats.WaitP50Ms,
			WaitP95Ms:   stats.WaitP95Ms,
		},
	})
}

func (a *API) ForceReleaseLock(w http.ResponseWriter, r *http.Request) {
	logger := a.Deps.Logger
	if nil == a.Deps.Locks {
		writeFeatureDisabled(w, "locks")
		return
	}
	key := chi.URLParam(r, "key")
	if "" == key {
		handlers.SendErrorResponse(w, logger, errors.New("lock key is required"), http.StatusBadRequest)
		return
	}
	var req forceReleaseRequest
	if err := decodeAdminBody(w, r, &req); nil != err && !errors.Is(err, io.EOF) {
		respondBodyError(w, logger, err)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if "" == reason || lockReasonMaxBytes < len(reason) {
		writeFieldProblems(w, []fieldProblem{{Path: "reason", Message: "reason is required and must be at most 500 characters"}})
		return
	}
	released, err := a.Deps.Locks.ForceRelease(r.Context(), key)
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	handlers.SendJSONResponse(w, forceReleaseResponse{Released: released})
	userID, _ := middleware.GetUserID(r.Context())
	logger.Warn("Edit lock force released",
		zap.String("lock_key", key),
		zap.String("reason", reason),
		zap.String("user_id", userID),
		zap.Bool("released", released))
}

func (a *API) Cache(w http.ResponseWriter, _ *http.Request) {
	stats := a.Deps.Engine.MetadataCache().Stats()
	handlers.SendJSONResponse(w, cacheResponse{
		Enabled:        stats.Enabled,
		TTLMs:          stats.TTL.Milliseconds(),
		Entries:        stats.Entries,
		MaxEntries:     stats.MaxEntries,
		Hits:           stats.Hits,
		Misses:         stats.Misses,
		Evictions:      stats.Evictions,
		ExpiredCleaned: stats.ExpiredCleaned,
		LastCleanupAt:  utcPtr(stats.LastCleanupAt),
	})
}

func (a *API) InvalidateCache(w http.ResponseWriter, r *http.Request) {
	logger := a.Deps.Logger
	var req cacheInvalidateRequest
	if err := decodeAdminBody(w, r, &req); nil != err {
		respondBodyError(w, logger, err)
		return
	}
	if problems := validateCacheInvalidate(req); 0 < len(problems) {
		writeFieldProblems(w, problems)
		return
	}
	cache := a.Deps.Engine.MetadataCache()
	var invalidated int
	if req.All {
		invalidated = cache.Clear()
	} else {
		invalidated = cache.InvalidatePrefix(req.Prefix)
	}
	handlers.SendJSONResponse(w, cacheInvalidateResponse{Invalidated: invalidated})
	logger.Info("Metadata cache invalidated",
		zap.String("prefix", req.Prefix),
		zap.Bool("all", req.All),
		zap.Int("invalidated", invalidated))
}

func (a *API) RateLimits(w http.ResponseWriter, _ *http.Request) {
	if nil == a.Deps.RateLimits {
		writeFeatureDisabled(w, "rate_limits")
		return
	}
	stats := a.Deps.RateLimits.Stats()
	rejections := map[string]uint64{
		middleware.RateLimitDownload:     0,
		middleware.RateLimitLinkGenerate: 0,
		middleware.RateLimitSession:      0,
	}
	for endpoint, count := range stats.RejectionsTotal {
		rejections[endpoint] = count
	}
	limits := make([]rateLimitSettingResponse, 0, len(stats.Limits))
	for _, limit := range stats.Limits {
		limits = append(limits, rateLimitSettingResponse{Endpoint: limit.Endpoint, RPS: limit.RPS, Burst: limit.Burst})
	}
	clients := make([]rateLimitClientResponse, 0, len(stats.TopClients))
	for _, client := range stats.TopClients {
		clients = append(clients, rateLimitClientResponse{
			Client:     client.Client,
			Endpoint:   client.Endpoint,
			Rejections: client.Rejections,
			LastAt:     utcPtr(client.LastAt),
		})
	}
	handlers.SendJSONResponse(w, rateLimitsResponse{
		Limits:          limits,
		TrackedClients:  stats.TrackedClients,
		MaxEntries:      stats.MaxEntries,
		TTLSeconds:      stats.TTLSeconds,
		RejectionsTotal: rejections,
		EvictionsTotal:  stats.EvictionsTotal,
		TopClients:      clients,
	})
}

func heldLockItems(held []locks.HeldLock, pathFilter string, limit int) []heldLockResponse {
	items := make([]heldLockResponse, 0, len(held))
	for _, lock := range held {
		path := ""
		if strings.HasPrefix(lock.Key, lockPathKeyPrefix) {
			path = strings.TrimPrefix(lock.Key, lockPathKeyPrefix)
		}
		if "" != pathFilter && path != pathFilter {
			continue
		}
		items = append(items, heldLockResponse{
			Key:        lock.Key,
			Path:       path,
			OwnerHint:  lock.Holder,
			AcquiredAt: utcPtr(lock.AcquiredAt),
			ExpiresAt:  utcPtr(lock.ExpiresAt),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	if limit < len(items) {
		items = items[:limit]
	}
	return items
}

func lockBackendName(dlmType string) string {
	switch strings.ToLower(strings.TrimSpace(dlmType)) {
	case lockBackendRedis:
		return lockBackendRedis
	case lockBackendNATS:
		return lockBackendNATS
	}
	return lockBackendLocal
}

func parseRuntimeLimit(raw string) (int, error) {
	if "" == raw {
		return lockDefaultListLimit, nil
	}
	value, err := strconv.Atoi(raw)
	if nil != err || 1 > value || lockMaxListLimit < value {
		return 0, fmt.Errorf("limit must be between 1 and %d", lockMaxListLimit)
	}
	return value, nil
}

func validateCacheInvalidate(req cacheInvalidateRequest) []fieldProblem {
	var problems []fieldProblem
	switch {
	case req.All && "" != req.Prefix:
		problems = append(problems, fieldProblem{Path: "all", Message: "set either prefix or all, not both"})
	case !req.All && "" == req.Prefix:
		problems = append(problems, fieldProblem{Path: "prefix", Message: "set prefix, or set all to true"})
	}
	if "" != req.Prefix && (cacheInvalidateMaxSize < len(req.Prefix) || !validAdminPath(req.Prefix)) {
		problems = append(problems, fieldProblem{Path: "prefix", Message: "prefix must start with / and must not contain .."})
	}
	return problems
}

func utcPtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}
