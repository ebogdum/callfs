package handlers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/ebogdum/callfs/v2/health"
)

const (
	healthReadyCacheTTL     = 2 * time.Second
	healthReadyProbeTimeout = 10 * time.Second
)

type healthChecker interface {
	Report(ctx context.Context, fresh bool) health.Report
}

type healthReadyBody struct {
	Status string `json:"status"`
}

type healthReadyCache struct {
	mu        sync.Mutex
	checker   healthChecker
	now       func() time.Time
	filled    bool
	checkedAt time.Time
	code      int
	status    string
}

func HealthReady(checker healthChecker) http.HandlerFunc {
	return healthReadyHandler(checker, time.Now)
}

func healthReadyHandler(checker healthChecker, now func() time.Time) http.HandlerFunc {
	cache := &healthReadyCache{checker: checker, now: now}
	return func(w http.ResponseWriter, _ *http.Request) {
		code, status := cache.result()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		SendJSONResponse(w, healthReadyBody{Status: status})
	}
}

func (c *healthReadyCache) result() (int, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	current := c.now()
	if c.filled && healthReadyCacheTTL > current.Sub(c.checkedAt) {
		return c.code, c.status
	}
	c.code, c.status = healthReadyStatus(c.probe())
	c.checkedAt = current
	c.filled = true
	return c.code, c.status
}

func (c *healthReadyCache) probe() health.Report {
	if nil == c.checker {
		return health.Report{Status: health.StatusDown}
	}
	ctx, cancel := context.WithTimeout(context.Background(), healthReadyProbeTimeout)
	defer cancel()
	return c.checker.Report(ctx, false)
}

func healthReadyStatus(report health.Report) (int, string) {
	switch report.Status {
	case health.StatusDown:
		return http.StatusServiceUnavailable, "down"
	case health.StatusWarn:
		return http.StatusOK, "degraded"
	default:
		return http.StatusOK, "ok"
	}
}
