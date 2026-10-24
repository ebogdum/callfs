package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/locks"
	"github.com/ebogdum/callfs/v2/server/handlers"
	"github.com/ebogdum/callfs/v2/server/middleware"
)

type runtimeLockInspector struct {
	held     []locks.HeldLock
	stats    locks.LockStats
	released []string
}

func (f *runtimeLockInspector) Snapshot(context.Context) ([]locks.HeldLock, error) {
	return f.held, nil
}

func (f *runtimeLockInspector) Stats() locks.LockStats {
	return f.stats
}

func (f *runtimeLockInspector) ForceRelease(_ context.Context, key string) (bool, error) {
	f.released = append(f.released, key)
	return true, nil
}

func runtimeRequestWithKey(method, target, body, key string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("key", key)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
}

func decodeRuntimeBody(t *testing.T, rec *httptest.ResponseRecorder, status int, dst any) {
	t.Helper()
	if status != rec.Code {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, status, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), dst); nil != err {
		t.Fatalf("decode body: %v", err)
	}
}

func TestRuntimeLocksFiltersByPath(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	inspector := &runtimeLockInspector{held: []locks.HeldLock{
		{Key: "path:/a.txt", Holder: "a1b2", AcquiredAt: now, ExpiresAt: now.Add(time.Minute)},
		{Key: "path:/b.txt", Holder: "c3d4", AcquiredAt: now, ExpiresAt: now.Add(time.Minute)},
	}}
	api := &API{Deps: Deps{Locks: inspector, Config: &config.AppConfig{}, Logger: zap.NewNop()}}

	rec := httptest.NewRecorder()
	api.Locks(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/locks?path=/a.txt", nil))

	var body locksResponse
	decodeRuntimeBody(t, rec, http.StatusOK, &body)
	if 1 != len(body.Held) || "/a.txt" != body.Held[0].Path || "a1b2" != body.Held[0].OwnerHint {
		t.Fatalf("held = %+v, want only /a.txt", body.Held)
	}
}

func TestRuntimeLocksRejectsBadLimit(t *testing.T) {
	api := &API{Deps: Deps{Locks: &runtimeLockInspector{}, Config: &config.AppConfig{}, Logger: zap.NewNop()}}

	rec := httptest.NewRecorder()
	api.Locks(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/locks?limit=5000", nil))

	if http.StatusBadRequest != rec.Code {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestRuntimeForceReleaseLockRequiresReason(t *testing.T) {
	inspector := &runtimeLockInspector{}
	api := &API{Deps: Deps{Locks: inspector, Config: &config.AppConfig{}, Logger: zap.NewNop()}}

	rec := httptest.NewRecorder()
	api.ForceReleaseLock(rec, runtimeRequestWithKey(http.MethodDelete, "/v1/admin/locks/path:%2Fa.txt", `{"reason":"  "}`, "path:/a.txt"))

	if http.StatusUnprocessableEntity != rec.Code {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if 0 != len(inspector.released) {
		t.Fatalf("released = %v, want none", inspector.released)
	}
}

func TestRuntimeForceReleaseLockReleases(t *testing.T) {
	inspector := &runtimeLockInspector{}
	api := &API{Deps: Deps{Locks: inspector, Config: &config.AppConfig{}, Logger: zap.NewNop()}}

	rec := httptest.NewRecorder()
	api.ForceReleaseLock(rec, runtimeRequestWithKey(http.MethodDelete, "/v1/admin/locks/path:%2Fa.txt", `{"reason":"stuck upload"}`, "path:/a.txt"))

	var body forceReleaseResponse
	decodeRuntimeBody(t, rec, http.StatusOK, &body)
	if !body.Released || 1 != len(inspector.released) || "path:/a.txt" != inspector.released[0] {
		t.Fatalf("released body = %+v, calls = %v", body, inspector.released)
	}
}

func TestRuntimeInvalidateCacheRejectsPrefixWithAll(t *testing.T) {
	api := &API{Deps: Deps{Logger: zap.NewNop()}}

	rec := httptest.NewRecorder()
	api.InvalidateCache(rec, httptest.NewRequest(http.MethodPost, "/v1/admin/cache/invalidate", strings.NewReader(`{"prefix":"/a","all":true}`)))

	if http.StatusUnprocessableEntity != rec.Code {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestRuntimeInvalidateCacheRejectsRelativePrefix(t *testing.T) {
	problems := validateCacheInvalidate(cacheInvalidateRequest{Prefix: "a/../b"})

	if 0 == len(problems) || "prefix" != problems[0].Path {
		t.Fatalf("problems = %+v, want prefix problem", problems)
	}
}

func TestRuntimeRateLimitsReportsLimitsAndRejections(t *testing.T) {
	registry := middleware.NewRateLimitRegistry(zap.NewNop())
	defer registry.Stop()
	if err := registry.SetLimits(middleware.RateLimitDownload, 10, 20); nil != err {
		t.Fatalf("SetLimits: %v", err)
	}
	api := &API{Deps: Deps{RateLimits: registry, Logger: zap.NewNop()}}

	rec := httptest.NewRecorder()
	api.RateLimits(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/ratelimits", nil))

	var body rateLimitsResponse
	decodeRuntimeBody(t, rec, http.StatusOK, &body)
	if 1 != len(body.Limits) || middleware.RateLimitDownload != body.Limits[0].Endpoint {
		t.Fatalf("limits = %+v, want one download limit", body.Limits)
	}
	if _, ok := body.RejectionsTotal[middleware.RateLimitSession]; !ok {
		t.Fatalf("rejections_total = %+v, want session key", body.RejectionsTotal)
	}
}

func TestRuntimeRateLimitsFeatureDisabledWhenMissing(t *testing.T) {
	api := &API{Deps: Deps{Logger: zap.NewNop()}}

	rec := httptest.NewRecorder()
	api.RateLimits(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/ratelimits", nil))

	var body handlers.ErrorResponse
	decodeRuntimeBody(t, rec, http.StatusNotFound, &body)
	if "FEATURE_DISABLED" != body.Code {
		t.Fatalf("code = %q, want FEATURE_DISABLED", body.Code)
	}
}

func TestRuntimeErasurePlanRequiresInstances(t *testing.T) {
	problems := validateErasurePlan(erasurePlanRequest{DataShards: 4, ParityShards: 2})

	if 0 == len(problems) || "instances" != problems[0].Path {
		t.Fatalf("problems = %+v, want instances problem", problems)
	}
}
