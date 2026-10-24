package telemetry

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func newTestHub(t *testing.T) *Hub {
	t.Helper()
	hub, err := NewHub(Config{InstanceID: "node-1", LogEntries: 100, AuditEntries: 100, EventEntries: 100})
	if nil != err {
		t.Fatalf("NewHub: %v", err)
	}
	return hub
}

func stringPointer(value string) *string {
	return &value
}

func TestRingKeepsNewestEntriesNewestFirst(t *testing.T) {
	entries, err := newRing[int](3)
	if nil != err {
		t.Fatalf("newRing: %v", err)
	}
	for value := 1; value <= 5; value++ {
		entries.add(value)
	}
	var seen []int
	entries.scan(func(value int) bool {
		seen = append(seen, value)
		return true
	})
	if 3 != len(seen) || 5 != seen[0] || 4 != seen[1] || 3 != seen[2] {
		t.Fatalf("scan order = %v, want [5 4 3]", seen)
	}
}

func TestLogRingQueryFiltersAndPaginates(t *testing.T) {
	hub := newTestHub(t)
	logger := zap.New(hub.Logs.Core())
	logger.Info("first", zap.String("request_id", "r1"))
	logger.Warn("second")
	logger.Error("third boom")

	warnings, err := hub.Logs.Query(LogQuery{MinLevel: "warn"})
	if nil != err {
		t.Fatalf("min level query: %v", err)
	}
	if 2 != len(warnings.Items) || "third boom" != warnings.Items[0].Message {
		t.Fatalf("min level items = %+v", warnings.Items)
	}

	text, err := hub.Logs.Query(LogQuery{Text: "BOOM"})
	if nil != err || 1 != len(text.Items) {
		t.Fatalf("text query = %+v, err %v", text.Items, err)
	}

	byRequest, err := hub.Logs.Query(LogQuery{RequestID: "r1"})
	if nil != err || 1 != len(byRequest.Items) || "first" != byRequest.Items[0].Message {
		t.Fatalf("request id query = %+v, err %v", byRequest.Items, err)
	}

	page, err := hub.Logs.Query(LogQuery{Limit: 1})
	if nil != err || 1 != len(page.Items) || nil == page.NextCursor {
		t.Fatalf("first page = %+v, err %v", page, err)
	}
	next, err := hub.Logs.Query(LogQuery{Limit: 1, Cursor: *page.NextCursor})
	if nil != err || 1 != len(next.Items) || "second" != next.Items[0].Message {
		t.Fatalf("second page = %+v, err %v", next.Items, err)
	}

	if _, err := hub.Logs.Query(LogQuery{MinLevel: "loud"}); nil == err {
		t.Fatal("invalid level accepted")
	}
}

func TestClassifyOp(t *testing.T) {
	cases := []struct {
		method string
		target string
		route  string
		op     string
	}{
		{http.MethodGet, "/v1/files/a", "/v1/files/*", OpRead},
		{http.MethodGet, "/v1/directories/a", "/v1/directories/*", OpList},
		{http.MethodDelete, "/v1/files/a?trash=true", "/v1/files/*", OpTrash},
		{http.MethodDelete, "/v1/files/a", "/v1/files/*", OpDelete},
		{http.MethodPut, "/v1/files/a", "/v1/files/*", OpUpload},
		{http.MethodPatch, "/v1/files/a", "/v1/files/*", OpRename},
		{http.MethodPost, "/v1/links/generate", "/v1/links/generate", OpLinkCreate},
		{http.MethodGet, "/download/token", "/download/{token}", OpLinkOpen},
		{http.MethodPost, "/v1/trash/3/restore", "/v1/trash/{id}/restore", OpRestore},
		{http.MethodGet, "/v1/admin/status", "/v1/admin/status", OpAdmin},
		{http.MethodPost, "/v1/session", "/v1/session", OpSession},
	}
	for _, tc := range cases {
		request := httptest.NewRequest(tc.method, tc.target, nil)
		if got := classifyOp(request, tc.route); tc.op != got {
			t.Errorf("classifyOp(%s %s) = %q, want %q", tc.method, tc.target, got, tc.op)
		}
	}
}

func TestAuditMiddlewareRecordsRouteUserAndBytes(t *testing.T) {
	hub := newTestHub(t)
	router := chi.NewRouter()
	router.Use(AuditMiddleware(hub))
	router.Get("/v1/files/*", func(w http.ResponseWriter, r *http.Request) {
		SetAuditUser(r.Context(), "alice")
		_, _ = w.Write([]byte("hello"))
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/files/docs/a.txt", nil))

	page, err := hub.Audit.Query(AuditQuery{})
	if nil != err || 1 != len(page.Items) {
		t.Fatalf("audit page = %+v, err %v", page.Items, err)
	}
	record := page.Items[0]
	if "/v1/files/*" != record.Route || "docs/a.txt" != record.Path || OpRead != record.Op {
		t.Errorf("route/path/op = %q %q %q", record.Route, record.Path, record.Op)
	}
	if "alice" != record.UserID || http.StatusOK != record.Status || 5 != record.BytesOut || "node-1" != record.InstanceID {
		t.Errorf("user/status/bytes/instance = %q %d %d %q", record.UserID, record.Status, record.BytesOut, record.InstanceID)
	}

	failed, err := hub.Audit.Query(AuditQuery{Status: ">=400"})
	if nil != err || 0 != len(failed.Items) {
		t.Fatalf("status filter items = %+v, err %v", failed.Items, err)
	}
}

func TestEventBusPublishValidatesAndQueries(t *testing.T) {
	bus, err := NewEventBus("node-1", 10)
	if nil != err {
		t.Fatalf("NewEventBus: %v", err)
	}
	if err := bus.Publish(Event{Type: "", Level: LevelInfo}); nil == err {
		t.Fatal("empty type accepted")
	}
	if err := bus.Publish(Event{Type: "file.created", Level: "debug"}); nil == err {
		t.Fatal("unknown level accepted")
	}
	publish := []Event{
		{Type: "file.created", Level: LevelInfo, Actor: stringPointer("alice")},
		{Type: "link.created", Level: LevelInfo, Actor: stringPointer("bob")},
		{Type: "file.deleted", Level: LevelWarn, Actor: stringPointer("alice")},
	}
	for _, event := range publish {
		if err := bus.Publish(event); nil != err {
			t.Fatalf("Publish: %v", err)
		}
	}
	byType, err := bus.Query(EventQuery{Types: []string{"file.created", "file.deleted"}})
	if nil != err || 2 != len(byType) || "file.created" != byType[0].Type || "file.deleted" != byType[1].Type {
		t.Fatalf("type query = %+v, err %v", byType, err)
	}
	byActor, err := bus.Query(EventQuery{Actor: "alice"})
	if nil != err || 2 != len(byActor) {
		t.Fatalf("actor query = %+v, err %v", byActor, err)
	}
	if _, err := bus.Query(EventQuery{Limit: 5000}); nil == err {
		t.Fatal("limit above maximum accepted")
	}
}

func TestEventSubscribeReplaysAndDropsOldest(t *testing.T) {
	bus, err := NewEventBus("node-1", 1000)
	if nil != err {
		t.Fatalf("NewEventBus: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := bus.Publish(Event{Type: "file.created", Level: LevelInfo}); nil != err {
			t.Fatalf("Publish: %v", err)
		}
	}
	sub := bus.Subscribe(nil, 1)
	if 2 != len(sub.Replay) || 2 != sub.Replay[0].ID || 3 != sub.Replay[1].ID {
		t.Fatalf("replay = %+v", sub.Replay)
	}
	for i := 0; i < 300; i++ {
		if err := bus.Publish(Event{Type: "file.created", Level: LevelInfo}); nil != err {
			t.Fatalf("Publish: %v", err)
		}
	}
	first := <-sub.C
	if 48 != first.ID {
		t.Fatalf("first buffered id = %d, want 48", first.ID)
	}
	last := first
	for i := 1; i < subscriberBuffer; i++ {
		last = <-sub.C
	}
	if 303 != last.ID {
		t.Fatalf("last buffered id = %d, want 303", last.ID)
	}
	sub.Close()
	if _, open := <-sub.C; open {
		t.Fatal("channel open after Close")
	}
}

func TestServeSSEReplaysLastEventIDAndStreamsLive(t *testing.T) {
	hub := newTestHub(t)
	for i := 0; i < 2; i++ {
		if err := hub.Events.Publish(Event{Type: "file.created", Level: LevelInfo}); nil != err {
			t.Fatalf("Publish: %v", err)
		}
	}
	errs := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sub := hub.Events.Subscribe(nil, ParseLastEventID(r))
		errs <- ServeSSE(w, r, sub)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if nil != err {
		t.Fatalf("NewRequest: %v", err)
	}
	request.Header.Set("Last-Event-ID", "1")
	response, err := server.Client().Do(request)
	if nil != err {
		t.Fatalf("Do: %v", err)
	}
	defer response.Body.Close()

	if err := hub.Events.Publish(Event{Type: "file.created", Level: LevelInfo}); nil != err {
		t.Fatalf("Publish: %v", err)
	}
	var ids []string
	scanner := bufio.NewScanner(response.Body)
	for len(ids) < 2 && scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "id: ") {
			ids = append(ids, strings.TrimPrefix(scanner.Text(), "id: "))
		}
	}
	if 2 != len(ids) || "2" != ids[0] || "3" != ids[1] {
		t.Fatalf("streamed ids = %v, want [2 3]", ids)
	}
	cancel()
	select {
	case err := <-errs:
		if nil != err {
			t.Fatalf("ServeSSE: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ServeSSE did not return after client disconnect")
	}
}

func TestTelemetryConcurrentAccess(t *testing.T) {
	hub := newTestHub(t)
	logger := zap.New(hub.Logs.Core())
	var group sync.WaitGroup
	for writer := 0; writer < 4; writer++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for i := 0; i < 200; i++ {
				if err := hub.Events.Publish(Event{Type: "file.created", Level: LevelInfo}); nil != err {
					t.Errorf("Publish: %v", err)
				}
				logger.Info("tick")
				hub.Audit.Add(AuditRecord{Method: http.MethodGet, Route: "/v1/files/*", Path: "a", Op: OpRead, Status: http.StatusOK})
			}
		}()
	}
	for reader := 0; reader < 4; reader++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for i := 0; i < 200; i++ {
				if _, err := hub.Logs.Query(LogQuery{Limit: 50}); nil != err {
					t.Errorf("log query: %v", err)
				}
				if _, err := hub.Audit.Query(AuditQuery{Limit: 50}); nil != err {
					t.Errorf("audit query: %v", err)
				}
				if _, err := hub.Events.Query(EventQuery{Limit: 50}); nil != err {
					t.Errorf("event query: %v", err)
				}
				sub, err := hub.Logs.Subscribe(LogQuery{}, 0)
				if nil != err {
					t.Errorf("log subscribe: %v", err)
					continue
				}
				sub.Close()
			}
		}()
	}
	group.Wait()
}
