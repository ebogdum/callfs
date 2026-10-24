package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/core"
	"github.com/ebogdum/callfs/v2/health"
	"github.com/ebogdum/callfs/v2/jobs"
	"github.com/ebogdum/callfs/v2/locks"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/server/admin"
	authMiddleware "github.com/ebogdum/callfs/v2/server/middleware"
	"github.com/ebogdum/callfs/v2/telemetry"
)

const (
	routerTestAdminKey  = "admin-key-0123456789abcdef0123456789abcdef"
	routerTestMemberKey = "member-key-0123456789abcdef0123456789abcdef"
	routerTestSecret    = "internal-secret-0123456789abcdef0123456789"
	routerTestCookie    = "callfs_session"
	routerTestCSRF      = "X-CallFS-CSRF"
	chiNotFoundBody     = "404 page not found\n"
)

type routerTestStore struct {
	metadata.Store
}

type routerTestErasureStore struct {
	metadata.ErasureMetadataStore
}

func (routerTestErasureStore) ListErasureInfos(context.Context, string, string, int) ([]*metadata.ErasureFileInfo, string, error) {
	return nil, "", nil
}

type routerTestLocks struct{}

func (routerTestLocks) Snapshot(context.Context) ([]locks.HeldLock, error) {
	return nil, nil
}

func (routerTestLocks) Stats() locks.LockStats {
	return locks.LockStats{}
}

func (routerTestLocks) ForceRelease(context.Context, string) (bool, error) {
	return false, nil
}

type routerTestPublisher struct{}

func (routerTestPublisher) Publish(string, any) {}

type routerTestRoute struct {
	method string
	path   string
}

type routerFixture struct {
	router       http.Handler
	adminCookie  *http.Cookie
	memberCookie *http.Cookie
	adminCSRF    string
}

func newRouterFixture(t *testing.T) routerFixture {
	t.Helper()
	logger := zap.NewNop()
	authenticator := auth.NewAPIKeyAuthenticator(
		[]string{routerTestAdminKey, routerTestMemberKey},
		map[string]string{"root": routerTestAdminKey, "alice": routerTestMemberKey},
		routerTestSecret,
	)
	authorizer := auth.NewUnixAuthorizer(routerTestStore{}, authenticator)
	admins := auth.NewAdminSet([]string{"root"})
	sessionKey, err := auth.DeriveSessionKey(routerTestSecret)
	if nil != err {
		t.Fatalf("derive session key: %v", err)
	}
	sessions := auth.NewSessionManager(sessionKey, time.Hour)
	adminValue, adminCSRF, _, err := sessions.Issue("root", auth.Fingerprint(routerTestAdminKey))
	if nil != err {
		t.Fatalf("issue admin session: %v", err)
	}
	memberValue, _, _, err := sessions.Issue("alice", auth.Fingerprint(routerTestMemberKey))
	if nil != err {
		t.Fatalf("issue member session: %v", err)
	}
	hub, err := telemetry.NewHub(telemetry.Config{InstanceID: "test", LogEntries: 100, AuditEntries: 100, EventEntries: 100})
	if nil != err {
		t.Fatalf("create telemetry hub: %v", err)
	}
	runner := jobs.NewRunner(routerTestPublisher{}, logger)
	trash := core.NewTrash(nil, authorizer, admins, runner, time.Hour, logger)
	uploads, err := core.NewUploads(nil, authorizer, t.TempDir(), "localfs", logger)
	if nil != err {
		t.Fatalf("create uploads: %v", err)
	}
	registry := authMiddleware.NewRateLimitRegistry(logger)
	for _, endpoint := range []string{authMiddleware.RateLimitDownload, authMiddleware.RateLimitLinkGenerate, authMiddleware.RateLimitSession} {
		if err := registry.SetLimits(endpoint, 100, 100); nil != err {
			t.Fatalf("set %s rate limit: %v", endpoint, err)
		}
	}
	api := admin.NewAPI(admin.Deps{
		Store:         routerTestStore{},
		Authenticator: authenticator,
		Admins:        admins,
		RateLimits:    registry,
		Locks:         routerTestLocks{},
		Telemetry:     hub,
		Jobs:          runner,
		Trash:         trash,
		Uploads:       uploads,
		Health:        health.NewChecker(nil),
		Logger:        logger,
	})
	router, resources := NewRouterWithOptions(RouterOptions{
		Authenticator: authenticator,
		Authorizer:    authorizer,
		ServerConfig:  &config.ServerConfig{},
		BackendConfig: &config.BackendConfig{},
		Logger:        logger,
		Config:        &config.AppConfig{},
		Store:         routerTestStore{},
		ErasureStore:  routerTestErasureStore{},
		Locks:         routerTestLocks{},
		Sessions:      sessions,
		Admins:        admins,
		RateLimits:    registry,
		Telemetry:     hub,
		Jobs:          runner,
		Trash:         trash,
		Uploads:       uploads,
		Health:        health.NewChecker(nil),
		Admin:         api,
	})
	t.Cleanup(resources.Stop)
	return routerFixture{
		router:       router,
		adminCookie:  &http.Cookie{Name: routerTestCookie, Value: adminValue},
		memberCookie: &http.Cookie{Name: routerTestCookie, Value: memberValue},
		adminCSRF:    adminCSRF,
	}
}

func serveRouterRequest(handler http.Handler, method, path string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(method, path, nil).WithContext(ctx)
	if nil != cookie {
		req.AddCookie(cookie)
	}
	if "" != csrf {
		req.Header.Set(routerTestCSRF, csrf)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

var routerTestAuthenticatedRoutes = []routerTestRoute{
	{http.MethodGet, "/v1/me"},
	{http.MethodGet, "/v1/me/events"},
	{http.MethodDelete, "/v1/session"},
	{http.MethodGet, "/v1/capabilities"},
	{http.MethodGet, "/v1/stat/a.txt"},
	{http.MethodGet, "/v1/search"},
	{http.MethodGet, "/v1/usage"},
	{http.MethodGet, "/v1/permissions/check"},
	{http.MethodGet, "/v1/directories/a"},
	{http.MethodGet, "/v1/objects"},
	{http.MethodDelete, "/v1/objects"},
	{http.MethodGet, "/v1/links"},
	{http.MethodGet, "/v1/links/stats"},
	{http.MethodGet, "/v1/links/1"},
	{http.MethodDelete, "/v1/links/1"},
	{http.MethodPost, "/v1/links/generate"},
	{http.MethodGet, "/v1/erasure/files"},
	{http.MethodGet, "/v1/erasure/files/a.txt"},
	{http.MethodGet, "/v1/erasure/manifest/a.txt"},
	{http.MethodPost, "/v1/erasure/verify/a.txt"},
	{http.MethodPost, "/v1/erasure/repair/a.txt"},
	{http.MethodGet, "/v1/trash"},
	{http.MethodDelete, "/v1/trash"},
	{http.MethodPost, "/v1/trash/1/restore"},
	{http.MethodDelete, "/v1/trash/1"},
	{http.MethodPost, "/v1/batch"},
	{http.MethodPost, "/v1/uploads"},
	{http.MethodPut, "/v1/uploads/1"},
	{http.MethodGet, "/v1/uploads/1"},
	{http.MethodPost, "/v1/uploads/1/complete"},
	{http.MethodDelete, "/v1/uploads/1"},
	{http.MethodGet, "/v1/jobs"},
	{http.MethodGet, "/v1/jobs/1"},
	{http.MethodPost, "/v1/jobs/1/cancel"},
	{http.MethodPost, "/v1/jobs/1/retry-failed"},
	{http.MethodGet, "/v1/admin/status"},
	{http.MethodPost, "/v1/admin/restart"},
	{http.MethodGet, "/v1/admin/migrations"},
	{http.MethodGet, "/v1/admin/health"},
	{http.MethodGet, "/v1/admin/findings"},
	{http.MethodPost, "/v1/admin/findings/tls.plain_http/mute"},
	{http.MethodDelete, "/v1/admin/findings/tls.plain_http/mute"},
	{http.MethodGet, "/v1/admin/tls"},
	{http.MethodGet, "/v1/admin/config"},
	{http.MethodGet, "/v1/admin/config/schema"},
	{http.MethodPost, "/v1/admin/config/validate"},
	{http.MethodPut, "/v1/admin/config"},
	{http.MethodPatch, "/v1/admin/config/runtime"},
	{http.MethodGet, "/v1/admin/config/export"},
	{http.MethodGet, "/v1/admin/instances"},
	{http.MethodPut, "/v1/admin/instances/i1/endpoint"},
	{http.MethodDelete, "/v1/admin/instances/i1/endpoint"},
	{http.MethodPost, "/v1/admin/instances/i1/probe"},
	{http.MethodGet, "/v1/admin/backends"},
	{http.MethodPost, "/v1/admin/backends/localfs/probe"},
	{http.MethodGet, "/v1/admin/erasure"},
	{http.MethodPost, "/v1/admin/erasure/plan"},
	{http.MethodPost, "/v1/admin/erasure/verify-all"},
	{http.MethodGet, "/v1/admin/replication"},
	{http.MethodPost, "/v1/admin/replication/retry"},
	{http.MethodGet, "/v1/admin/staging"},
	{http.MethodDelete, "/v1/admin/staging"},
	{http.MethodGet, "/v1/admin/hygiene"},
	{http.MethodPost, "/v1/admin/retag"},
	{http.MethodGet, "/v1/admin/locks"},
	{http.MethodDelete, "/v1/admin/locks/k1"},
	{http.MethodGet, "/v1/admin/cache"},
	{http.MethodPost, "/v1/admin/cache/invalidate"},
	{http.MethodGet, "/v1/admin/ratelimits"},
	{http.MethodGet, "/v1/admin/users"},
	{http.MethodPost, "/v1/admin/users"},
	{http.MethodPost, "/v1/admin/users/migrate-positional"},
	{http.MethodPatch, "/v1/admin/users/u1"},
	{http.MethodDelete, "/v1/admin/users/u1"},
	{http.MethodPost, "/v1/admin/users/u1/rotate"},
	{http.MethodGet, "/v1/admin/owners"},
	{http.MethodPost, "/v1/admin/owners/reassign"},
	{http.MethodGet, "/v1/admin/auth/failures"},
	{http.MethodGet, "/v1/admin/metrics"},
	{http.MethodGet, "/v1/admin/metrics/series"},
	{http.MethodGet, "/v1/admin/logs"},
	{http.MethodGet, "/v1/admin/logs/stream"},
	{http.MethodGet, "/v1/admin/audit"},
	{http.MethodGet, "/v1/admin/events"},
	{http.MethodGet, "/v1/admin/events/history"},
	{http.MethodGet, "/v1/admin/transfers"},
	{http.MethodGet, "/v1/admin/jobs"},
	{http.MethodGet, "/v1/admin/jobs/1"},
	{http.MethodPost, "/v1/admin/jobs/1/cancel"},
	{http.MethodPost, "/v1/admin/jobs/1/retry-failed"},
	{http.MethodGet, "/v1/admin/workers"},
	{http.MethodPost, "/v1/admin/workers/usage_refresh/run"},
}

var routerTestPublicRoutes = []routerTestRoute{
	{http.MethodPost, "/v1/session"},
	{http.MethodGet, "/health/ready"},
}

func TestRouterRegistersAuthenticatedRoutes(t *testing.T) {
	fx := newRouterFixture(t)
	for i, route := range routerTestAuthenticatedRoutes {
		req := httptest.NewRequest(route.method, route.path, nil)
		req.RemoteAddr = fmt.Sprintf("10.%d.%d.1:4000", i/256, i%256)
		rec := httptest.NewRecorder()
		fx.router.ServeHTTP(rec, req)
		if http.StatusUnauthorized != rec.Code {
			t.Errorf("%s %s without credentials: status %d, want %d (route missing or unprotected)", route.method, route.path, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestRouterDispatchesGetRoutesWithAdminSession(t *testing.T) {
	fx := newRouterFixture(t)
	for _, route := range routerTestAuthenticatedRoutes {
		if http.MethodGet != route.method {
			continue
		}
		rec := serveRouterRequest(fx.router, route.method, route.path, fx.adminCookie, "")
		if chiNotFoundBody == rec.Body.String() {
			t.Errorf("GET %s with admin session: route not registered", route.path)
		}
		if http.StatusUnauthorized == rec.Code || http.StatusForbidden == rec.Code {
			t.Errorf("GET %s with admin session: status %d", route.path, rec.Code)
		}
	}
}

func TestRouterServesPublicRoutes(t *testing.T) {
	fx := newRouterFixture(t)
	for _, route := range routerTestPublicRoutes {
		rec := serveRouterRequest(fx.router, route.method, route.path, nil, "")
		if chiNotFoundBody == rec.Body.String() {
			t.Errorf("%s %s: route not registered", route.method, route.path)
		}
	}
}

func TestNonGetRouteRequiresCSRFToken(t *testing.T) {
	fx := newRouterFixture(t)
	rec := serveRouterRequest(fx.router, http.MethodPost, "/v1/jobs/1/cancel", fx.adminCookie, "")
	if http.StatusForbidden != rec.Code {
		t.Fatalf("POST without CSRF token: status %d, want %d", rec.Code, http.StatusForbidden)
	}
	rec = serveRouterRequest(fx.router, http.MethodPost, "/v1/jobs/1/cancel", fx.adminCookie, fx.adminCSRF)
	if http.StatusForbidden == rec.Code {
		t.Fatalf("POST with CSRF token: status %d, want the handler response", rec.Code)
	}
}

func TestAdminRoutesRequireAdmin(t *testing.T) {
	fx := newRouterFixture(t)
	rec := serveRouterRequest(fx.router, http.MethodGet, "/v1/admin/status", fx.memberCookie, "")
	if http.StatusForbidden != rec.Code {
		t.Fatalf("GET /v1/admin/status with member session: status %d, want %d", rec.Code, http.StatusForbidden)
	}
}
