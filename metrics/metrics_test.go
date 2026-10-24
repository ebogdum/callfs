package metrics_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ebogdum/callfs/v2/backends"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/metrics"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type stubStore struct {
	metadata.Store
	err error
}

func (s stubStore) Get(_ context.Context, _ string) (*metadata.Metadata, error) {
	return nil, s.err
}

type stubStorage struct {
	backends.Storage
}

func (stubStorage) Stat(_ context.Context, _ string) (*metadata.Metadata, error) {
	return nil, nil
}

func counterValue(t *testing.T, counter prometheus.Counter) float64 {
	t.Helper()
	var metric dto.Metric
	if err := counter.Write(&metric); err != nil {
		t.Fatalf("reading counter: %v", err)
	}
	return metric.GetCounter().GetValue()
}

func TestMetricsNormalizeMethod(t *testing.T) {
	cases := map[string]string{
		"GET":      "GET",
		"HEAD":     "HEAD",
		"POST":     "POST",
		"PUT":      "PUT",
		"PATCH":    "PATCH",
		"DELETE":   "DELETE",
		"OPTIONS":  "OPTIONS",
		"PROPFIND": "OTHER",
		"get":      "OTHER",
		"":         "OTHER",
	}
	for method, want := range cases {
		if got := metrics.NormalizeMethod(method); want != got {
			t.Fatalf("NormalizeMethod(%q) = %q, want %q", method, got, want)
		}
	}
}

func TestInstrumentMetadataCountsQueries(t *testing.T) {
	counter := metrics.MetadataDBQueriesTotal.WithLabelValues("get")
	before := counterValue(t, counter)

	store := metadata.Instrument(stubStore{err: metadata.ErrNotFound})
	_, err := store.Get(context.Background(), "/a")
	if !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("Get error = %v, want %v", err, metadata.ErrNotFound)
	}

	if after := counterValue(t, counter); 1 != after-before {
		t.Fatalf("query count delta = %v, want 1", after-before)
	}
}

func TestInstrumentBackendCountsOps(t *testing.T) {
	counter := metrics.BackendOpsTotal.WithLabelValues("stubtype", "stat")
	before := counterValue(t, counter)

	storage := backends.Instrument("stubtype", stubStorage{})
	if _, err := storage.Stat(context.Background(), "/a"); nil != err {
		t.Fatalf("Stat error = %v", err)
	}

	if after := counterValue(t, counter); 1 != after-before {
		t.Fatalf("op count delta = %v, want 1", after-before)
	}
}
