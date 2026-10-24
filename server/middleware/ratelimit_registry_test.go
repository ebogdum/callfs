package middleware

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func newRateLimitRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/download/abc", nil)
	req.RemoteAddr = "192.0.2.1:4000"
	return req
}

func TestRateLimitRegistrySetLimitsRejectsUnknownEndpoint(t *testing.T) {
	registry := NewRateLimitRegistry(zap.NewNop())
	defer registry.Stop()

	if err := registry.SetLimits("bogus", 1, 1); !errors.Is(err, ErrUnknownRateLimitEndpoint) {
		t.Fatalf("got %v, want ErrUnknownRateLimitEndpoint", err)
	}
}

func TestRateLimitRegistrySetLimitsRejectsInvalidValues(t *testing.T) {
	registry := NewRateLimitRegistry(zap.NewNop())
	defer registry.Stop()

	cases := []struct {
		name  string
		rps   float64
		burst int
	}{
		{"zero rps", 0, 1},
		{"negative rps", -1, 1},
		{"nan rps", math.NaN(), 1},
		{"infinite rps", math.Inf(1), 1},
		{"zero burst", 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := registry.SetLimits(RateLimitSession, tc.rps, tc.burst); !errors.Is(err, ErrInvalidRateLimit) {
				t.Fatalf("got %v, want ErrInvalidRateLimit", err)
			}
		})
	}
}

func TestRateLimitRegistryMiddlewareRequiresConfiguredEndpoint(t *testing.T) {
	registry := NewRateLimitRegistry(zap.NewNop())
	defer registry.Stop()

	if _, err := registry.Middleware(RateLimitSession); !errors.Is(err, ErrRateLimitNotConfigured) {
		t.Fatalf("got %v, want ErrRateLimitNotConfigured", err)
	}
}

func TestRateLimitRegistryRejectsWithRetryAfterAndCounts(t *testing.T) {
	registry := NewRateLimitRegistry(zap.NewNop())
	defer registry.Stop()

	if err := registry.SetLimits(RateLimitDownload, 0.5, 1); nil != err {
		t.Fatal(err)
	}
	mw, err := registry.Middleware(RateLimitDownload)
	if nil != err {
		t.Fatal(err)
	}
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, newRateLimitRequest())
	if http.StatusOK != first.Code {
		t.Fatalf("first request got %d, want 200", first.Code)
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, newRateLimitRequest())
	if http.StatusTooManyRequests != second.Code {
		t.Fatalf("second request got %d, want 429", second.Code)
	}
	if "2" != second.Header().Get("Retry-After") {
		t.Fatalf("got Retry-After %q, want \"2\"", second.Header().Get("Retry-After"))
	}

	stats := registry.Stats()
	if 1 != stats.RejectionsTotal[RateLimitDownload] {
		t.Fatalf("got rejections %d, want 1", stats.RejectionsTotal[RateLimitDownload])
	}
	if 1 != stats.TrackedClients {
		t.Fatalf("got tracked clients %d, want 1", stats.TrackedClients)
	}
	if 1 != len(stats.TopClients) {
		t.Fatalf("got %d top clients, want 1", len(stats.TopClients))
	}
	top := stats.TopClients[0]
	if "192.0.2.1" != top.Client || RateLimitDownload != top.Endpoint || 1 != top.Rejections {
		t.Fatalf("got top client %+v, want 192.0.2.1 download with 1 rejection", top)
	}
}

func TestRateLimitRegistrySetLimitsUpdatesExistingLimiter(t *testing.T) {
	registry := NewRateLimitRegistry(zap.NewNop())
	defer registry.Stop()

	if err := registry.SetLimits(RateLimitSession, 1, 1); nil != err {
		t.Fatal(err)
	}
	if err := registry.SetLimits(RateLimitSession, 10, 3); nil != err {
		t.Fatal(err)
	}

	stats := registry.Stats()
	if 1 != len(stats.Limits) {
		t.Fatalf("got %d limits, want 1", len(stats.Limits))
	}
	setting := stats.Limits[0]
	if RateLimitSession != setting.Endpoint || 10 != setting.RPS || 3 != setting.Burst {
		t.Fatalf("got limit %+v, want session 10 rps burst 3", setting)
	}
}

type stubClusterCounters struct {
	allowed  bool
	err      error
	calls    int
	failures int
}

func (s *stubClusterCounters) Allow(_ context.Context, _, _ string, _ float64, _ int) (bool, error) {
	s.calls++
	return s.allowed, s.err
}

func (s *stubClusterCounters) ReportFailure(string, error) {
	s.failures++
}

func serveRateLimited(t *testing.T, registry *RateLimitRegistry) int {
	t.Helper()
	mw, err := registry.Middleware(RateLimitDownload)
	if nil != err {
		t.Fatalf("middleware: %v", err)
	}
	recorder := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })).ServeHTTP(recorder, newRateLimitRequest())
	return recorder.Code
}

func TestRateLimitRegistryUsesClusterCounters(t *testing.T) {
	registry := NewRateLimitRegistry(zap.NewNop())
	defer registry.Stop()
	if err := registry.SetLimits(RateLimitDownload, 100, 100); nil != err {
		t.Fatalf("set limits: %v", err)
	}
	counters := &stubClusterCounters{allowed: false}
	registry.SetCounters(counters)

	if code := serveRateLimited(t, registry); http.StatusTooManyRequests != code {
		t.Fatalf("cluster rejection: got %d, want 429", code)
	}
	counters.allowed = true
	if code := serveRateLimited(t, registry); http.StatusOK != code {
		t.Fatalf("cluster allowance: got %d, want 200", code)
	}
	if 2 != counters.calls {
		t.Fatalf("counter calls = %d, want 2", counters.calls)
	}
}

func TestRateLimitRegistryFallsBackToLocalOnCounterError(t *testing.T) {
	registry := NewRateLimitRegistry(zap.NewNop())
	defer registry.Stop()
	if err := registry.SetLimits(RateLimitDownload, 100, 100); nil != err {
		t.Fatalf("set limits: %v", err)
	}
	counters := &stubClusterCounters{err: errors.New("nats down")}
	registry.SetCounters(counters)

	if code := serveRateLimited(t, registry); http.StatusOK != code {
		t.Fatalf("fail open: got %d, want 200", code)
	}
	if 1 != counters.failures {
		t.Fatalf("failures reported = %d, want 1", counters.failures)
	}
}
