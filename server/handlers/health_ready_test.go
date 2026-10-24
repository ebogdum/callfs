package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ebogdum/callfs/v2/health"
)

type readyStubChecker struct {
	status     health.Status
	components []health.Component
	calls      int
}

func (s *readyStubChecker) Report(_ context.Context, _ bool) health.Report {
	s.calls++
	return health.Report{Status: s.status, Components: s.components}
}

type readyStubClock struct {
	now time.Time
}

func (c *readyStubClock) Now() time.Time {
	return c.now
}

func serveReady(t *testing.T, handler http.HandlerFunc) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

func TestHealthReadyStatusMapping(t *testing.T) {
	cases := []struct {
		name   string
		status health.Status
		code   int
		body   string
	}{
		{name: "ok", status: health.StatusOK, code: http.StatusOK, body: `{"status":"ok"}`},
		{name: "warn is degraded", status: health.StatusWarn, code: http.StatusOK, body: `{"status":"degraded"}`},
		{name: "down", status: health.StatusDown, code: http.StatusServiceUnavailable, body: `{"status":"down"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checker := &readyStubChecker{status: tc.status}
			clock := &readyStubClock{now: time.Unix(1000, 0)}
			code, body := serveReady(t, healthReadyHandler(checker, clock.Now))
			if tc.code != code || tc.body != body {
				t.Fatalf("got %d %s, want %d %s", code, body, tc.code, tc.body)
			}
		})
	}
}

func TestHealthReadyCachesForTwoSeconds(t *testing.T) {
	checker := &readyStubChecker{status: health.StatusOK}
	clock := &readyStubClock{now: time.Unix(1000, 0)}
	handler := healthReadyHandler(checker, clock.Now)

	serveReady(t, handler)
	serveReady(t, handler)
	if 1 != checker.calls {
		t.Fatalf("calls within TTL = %d, want 1", checker.calls)
	}

	clock.now = clock.now.Add(healthReadyCacheTTL)
	serveReady(t, handler)
	if 2 != checker.calls {
		t.Fatalf("calls after TTL = %d, want 2", checker.calls)
	}
}

func TestHealthReadyNilCheckerIsDown(t *testing.T) {
	clock := &readyStubClock{now: time.Unix(1000, 0)}
	code, body := serveReady(t, healthReadyHandler(nil, clock.Now))
	if http.StatusServiceUnavailable != code || `{"status":"down"}` != body {
		t.Fatalf("got %d %s, want 503 down", code, body)
	}
}

func TestHealthReadyListsPeersWithoutFailing(t *testing.T) {
	checker := &readyStubChecker{
		status: health.StatusOK,
		components: []health.Component{
			{Name: "locks", Status: health.StatusOK},
			{Name: "storage.internal_proxy", Status: health.StatusDown, Detail: "peer down", Informational: true},
		},
	}
	clock := &readyStubClock{now: time.Unix(1000, 0)}
	code, body := serveReady(t, healthReadyHandler(checker, clock.Now))
	want := `{"status":"ok","peers":[{"name":"storage.internal_proxy","status":"down","detail":"peer down"}]}`
	if http.StatusOK != code || want != body {
		t.Fatalf("got %d %s, want 200 %s", code, body, want)
	}
}
