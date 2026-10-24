package admin

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ebogdum/callfs/v2/telemetry"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const telemetryTestMetric = "callfs_telemetry_test_requests_total"

type stubTransfers struct {
	items []Transfer
}

func (s stubTransfers) ActiveTransfers() []Transfer {
	return s.items
}

func newTelemetryTestAPI(t *testing.T) (*API, *telemetry.Hub, time.Time) {
	t.Helper()
	hub, err := telemetry.NewHub(telemetry.Config{
		InstanceID:   "test-instance",
		LogEntries:   100,
		AuditEntries: 100,
		EventEntries: 100,
	})
	if nil != err {
		t.Fatalf("new hub: %v", err)
	}
	registry := prometheus.NewRegistry()
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: telemetryTestMetric,
		Help: "Test requests.",
	}, []string{"method"})
	if err := registry.Register(counter); nil != err {
		t.Fatalf("register counter: %v", err)
	}
	sampler, err := telemetry.NewSampler(telemetry.SamplerConfig{
		Gatherer:  registry,
		Interval:  time.Second,
		Retention: time.Hour,
	})
	if nil != err {
		t.Fatalf("new sampler: %v", err)
	}
	start := time.Now().UTC().Truncate(time.Second)
	counter.WithLabelValues("GET").Add(3)
	if err := sampler.Sample(start); nil != err {
		t.Fatalf("first sample: %v", err)
	}
	counter.WithLabelValues("GET").Add(5)
	if err := sampler.Sample(start.Add(10 * time.Second)); nil != err {
		t.Fatalf("second sample: %v", err)
	}
	return &API{Deps: Deps{Telemetry: hub, Sampler: sampler}}, hub, start
}

func serveTelemetryTest(handler http.HandlerFunc, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func TestMetricsReturnsNamedFamilies(t *testing.T) {
	api, _, _ := newTelemetryTestAPI(t)
	recorder := serveTelemetryTest(api.Metrics, "/v1/admin/metrics?names="+telemetryTestMetric)
	if http.StatusOK != recorder.Code {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Families []struct {
			Name    string `json:"name"`
			Emitted bool   `json:"emitted"`
		} `json:"families"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); nil != err {
		t.Fatalf("decode: %v", err)
	}
	if 1 != len(body.Families) || telemetryTestMetric != body.Families[0].Name || !body.Families[0].Emitted {
		t.Fatalf("unexpected families: %s", recorder.Body.String())
	}
}

func TestMetricSeriesReturnsRequestedMeasure(t *testing.T) {
	api, _, start := newTelemetryTestAPI(t)
	target := fmt.Sprintf("/v1/admin/metrics/series?name=%s&measure=total&from=%d&to=%d&step_s=60",
		telemetryTestMetric, start.Add(-time.Minute).Unix(), start.Add(time.Minute).Unix())
	recorder := serveTelemetryTest(api.MetricSeries, target)
	if http.StatusOK != recorder.Code {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Name    string `json:"name"`
		Measure string `json:"measure"`
		Series  []struct {
			Labels map[string]string `json:"labels"`
		} `json:"series"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); nil != err {
		t.Fatalf("decode: %v", err)
	}
	if telemetryTestMetric != body.Name || "total" != body.Measure || 0 == len(body.Series) {
		t.Fatalf("unexpected series: %s", recorder.Body.String())
	}
}

func TestMetricSeriesRejectsInvalidQueries(t *testing.T) {
	api, _, _ := newTelemetryTestAPI(t)
	cases := []struct {
		name   string
		query  string
		status int
	}{
		{name: "missing name", query: "", status: http.StatusBadRequest},
		{name: "unknown family", query: "name=callfs_telemetry_missing_total", status: http.StatusNotFound},
		{name: "measure not for kind", query: "name=" + telemetryTestMetric + "&measure=p95", status: http.StatusBadRequest},
		{name: "zero step", query: "name=" + telemetryTestMetric + "&step_s=0", status: http.StatusBadRequest},
		{name: "filter without value", query: "name=" + telemetryTestMetric + "&filter=method", status: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := serveTelemetryTest(api.MetricSeries, "/v1/admin/metrics/series?"+tc.query)
			if tc.status != recorder.Code {
				t.Fatalf("status %d, want %d: %s", recorder.Code, tc.status, recorder.Body.String())
			}
		})
	}
}

func TestLogsFilterByLevel(t *testing.T) {
	api, hub, _ := newTelemetryTestAPI(t)
	logger := zap.New(hub.Tee(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(io.Discard), zapcore.InfoLevel)))
	logger.Info("upload started")
	logger.Warn("upload slow")

	recorder := serveTelemetryTest(api.Logs, "/v1/admin/logs?level=warn")
	if http.StatusOK != recorder.Code {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Items []struct {
			Message string `json:"message"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); nil != err {
		t.Fatalf("decode: %v", err)
	}
	if 1 != len(body.Items) || "upload slow" != body.Items[0].Message {
		t.Fatalf("unexpected logs: %s", recorder.Body.String())
	}

	if bad := serveTelemetryTest(api.Logs, "/v1/admin/logs?level=loud"); http.StatusBadRequest != bad.Code {
		t.Fatalf("invalid level status %d, want 400", bad.Code)
	}
}

func TestAuditFiltersAndExportsCSV(t *testing.T) {
	api, hub, _ := newTelemetryTestAPI(t)
	now := time.Now().UTC()
	hub.Audit.Add(telemetry.AuditRecord{
		At: now, RequestID: "req-1", UserID: "alice", Method: http.MethodGet,
		Route: "GET /v1/files/*", Path: "/team/a.txt", Op: "read", Status: 200,
		BytesOut: 12, DurationMS: 3, InstanceID: "test-instance", Client: "10.0.0.1",
	})
	hub.Audit.Add(telemetry.AuditRecord{
		At: now, RequestID: "req-2", UserID: "bob", Method: http.MethodDelete,
		Route: "DELETE /v1/files/*", Path: "/team/b.txt", Op: "delete", Status: 500,
		DurationMS: 7, InstanceID: "test-instance", Client: "10.0.0.2",
	})

	recorder := serveTelemetryTest(api.Audit, "/v1/admin/audit?user=alice")
	if http.StatusOK != recorder.Code {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Items []struct {
			UserID string `json:"user_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); nil != err {
		t.Fatalf("decode: %v", err)
	}
	if 1 != len(body.Items) || "alice" != body.Items[0].UserID {
		t.Fatalf("unexpected audit items: %s", recorder.Body.String())
	}

	csvTarget := "/v1/admin/audit?" + url.Values{"status": {">=400"}, "format": {"csv"}}.Encode()
	csvRecorder := serveTelemetryTest(api.Audit, csvTarget)
	if http.StatusOK != csvRecorder.Code {
		t.Fatalf("csv status %d: %s", csvRecorder.Code, csvRecorder.Body.String())
	}
	rows, err := csv.NewReader(strings.NewReader(csvRecorder.Body.String())).ReadAll()
	if nil != err {
		t.Fatalf("parse csv: %v", err)
	}
	if 2 != len(rows) || "at" != rows[0][0] || "bob" != rows[1][2] || "500" != rows[1][7] {
		t.Fatalf("unexpected csv rows: %v", rows)
	}

	if bad := serveTelemetryTest(api.Audit, "/v1/admin/audit?op=bogus"); http.StatusBadRequest != bad.Code {
		t.Fatalf("invalid op status %d, want 400", bad.Code)
	}
	if bad := serveTelemetryTest(api.Audit, "/v1/admin/audit?format=xml"); http.StatusBadRequest != bad.Code {
		t.Fatalf("invalid format status %d, want 400", bad.Code)
	}
}

func TestEventHistoryFiltersByType(t *testing.T) {
	api, hub, _ := newTelemetryTestAPI(t)
	if err := hub.Events.Publish(telemetry.Event{Type: "file.created", Level: "info", Message: "created"}); nil != err {
		t.Fatalf("publish file event: %v", err)
	}
	if err := hub.Events.Publish(telemetry.Event{Type: "config.changed", Level: "info", Message: "changed"}); nil != err {
		t.Fatalf("publish config event: %v", err)
	}
	recorder := serveTelemetryTest(api.EventHistory, "/v1/admin/events/history?types=file.created")
	if http.StatusOK != recorder.Code {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Items []struct {
			Type string `json:"type"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); nil != err {
		t.Fatalf("decode: %v", err)
	}
	if 1 != len(body.Items) || "file.created" != body.Items[0].Type {
		t.Fatalf("unexpected events: %s", recorder.Body.String())
	}
}

func TestTransfersListsActiveSource(t *testing.T) {
	now := time.Now().UTC()
	total := int64(100)
	api := &API{Deps: Deps{Transfers: stubTransfers{items: []Transfer{
		{ID: "t1", UserID: "alice", Kind: "ws_upload", Path: "/team/a.bin", BytesDone: 5, BytesTotal: &total, StartedAt: now, Client: "10.0.0.1"},
	}}}}
	recorder := serveTelemetryTest(api.Transfers, "/v1/admin/transfers")
	if http.StatusOK != recorder.Code {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); nil != err {
		t.Fatalf("decode: %v", err)
	}
	if 1 != len(body.Items) || "t1" != body.Items[0].ID {
		t.Fatalf("unexpected transfers: %s", recorder.Body.String())
	}

	empty := &API{}
	emptyRecorder := serveTelemetryTest(empty.Transfers, "/v1/admin/transfers")
	if !strings.Contains(emptyRecorder.Body.String(), `"items":[]`) {
		t.Fatalf("expected empty items list: %s", emptyRecorder.Body.String())
	}
}

func TestEventAndLogStreamsOpenSSE(t *testing.T) {
	api, _, _ := newTelemetryTestAPI(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	streams := map[string]http.HandlerFunc{
		"logs":   api.LogStream,
		"events": api.EventStream,
	}
	for name, handler := range streams {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/v1/admin/stream", nil).WithContext(ctx)
			recorder := httptest.NewRecorder()
			handler(recorder, request)
			if http.StatusOK != recorder.Code || "text/event-stream" != recorder.Header().Get("Content-Type") {
				t.Fatalf("status %d content-type %q", recorder.Code, recorder.Header().Get("Content-Type"))
			}
		})
	}
}
