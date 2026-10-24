package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ebogdum/callfs/v2/cluster"
)

type fakeDeadTasks struct {
	items   []cluster.DeadTask
	retried []uint64
	dropped []uint64
}

func (f *fakeDeadTasks) List(_ context.Context, limit int, before uint64) ([]cluster.DeadTask, bool, error) {
	out := []cluster.DeadTask{}
	for _, item := range f.items {
		if 0 < before && before <= item.ID {
			continue
		}
		if limit == len(out) {
			return out, true, nil
		}
		out = append(out, item)
	}
	return out, false, nil
}

func (f *fakeDeadTasks) Retry(_ context.Context, id uint64) error {
	for _, item := range f.items {
		if id == item.ID {
			f.retried = append(f.retried, id)
			return nil
		}
	}
	return cluster.ErrDeadTaskNotFound
}

func (f *fakeDeadTasks) Drop(_ context.Context, id uint64) error {
	for _, item := range f.items {
		if id == item.ID {
			f.dropped = append(f.dropped, id)
			return nil
		}
	}
	return cluster.ErrDeadTaskNotFound
}

func newDeadTasksHandler(source cluster.DeadTasks) http.Handler {
	api := &API{Deps: Deps{DeadTasks: source}}
	router := chi.NewRouter()
	router.Get("/tasks/dead", api.DeadTasks)
	router.Post("/tasks/dead/{id}/retry", api.RetryDeadTask)
	router.Delete("/tasks/dead/{id}", api.DropDeadTask)
	return router
}

func doDeadTasksRequest(handler http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestDeadTasksListPages(t *testing.T) {
	fake := &fakeDeadTasks{items: []cluster.DeadTask{
		{ID: 9, Pool: "replicate", Reason: "max deliveries exceeded", FailedAt: time.Unix(9, 0), Summary: "/a"},
		{ID: 7, Pool: "replicate", Reason: "terminated by handler", FailedAt: time.Unix(7, 0), Summary: "/b"},
		{ID: 3, Pool: "replicate", Reason: "max deliveries exceeded", FailedAt: time.Unix(3, 0), Summary: "/c"},
	}}
	handler := newDeadTasksHandler(fake)

	rec := doDeadTasksRequest(handler, http.MethodGet, "/tasks/dead?limit=2")
	if http.StatusOK != rec.Code {
		t.Fatalf("status %d", rec.Code)
	}
	var page deadTasksPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); nil != err {
		t.Fatal(err)
	}
	if 2 != len(page.Items) || nil == page.NextBefore || 7 != *page.NextBefore {
		t.Fatalf("unexpected first page %+v", page)
	}

	rec = doDeadTasksRequest(handler, http.MethodGet, "/tasks/dead?limit=2&before=7")
	page = deadTasksPage{}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); nil != err {
		t.Fatal(err)
	}
	if 1 != len(page.Items) || 3 != page.Items[0].ID || nil != page.NextBefore {
		t.Fatalf("unexpected second page %+v", page)
	}
}

func TestDeadTasksListRejectsBadQuery(t *testing.T) {
	handler := newDeadTasksHandler(&fakeDeadTasks{})
	for _, target := range []string{"/tasks/dead?limit=0", "/tasks/dead?limit=x", "/tasks/dead?before=-1"} {
		if rec := doDeadTasksRequest(handler, http.MethodGet, target); http.StatusBadRequest != rec.Code {
			t.Fatalf("%s: status %d", target, rec.Code)
		}
	}
}

func TestDeadTasksRetryAndDrop(t *testing.T) {
	fake := &fakeDeadTasks{items: []cluster.DeadTask{{ID: 5}, {ID: 6}}}
	handler := newDeadTasksHandler(fake)

	if rec := doDeadTasksRequest(handler, http.MethodPost, "/tasks/dead/5/retry"); http.StatusNoContent != rec.Code {
		t.Fatalf("retry status %d", rec.Code)
	}
	if rec := doDeadTasksRequest(handler, http.MethodDelete, "/tasks/dead/6"); http.StatusNoContent != rec.Code {
		t.Fatalf("drop status %d", rec.Code)
	}
	if 1 != len(fake.retried) || 5 != fake.retried[0] || 1 != len(fake.dropped) || 6 != fake.dropped[0] {
		t.Fatalf("unexpected calls %+v", fake)
	}
	if rec := doDeadTasksRequest(handler, http.MethodDelete, "/tasks/dead/99"); http.StatusNotFound != rec.Code {
		t.Fatalf("missing status %d", rec.Code)
	}
	if rec := doDeadTasksRequest(handler, http.MethodPost, "/tasks/dead/abc/retry"); http.StatusBadRequest != rec.Code {
		t.Fatalf("bad id status %d", rec.Code)
	}
}

func TestDeadTasksWithoutNATS(t *testing.T) {
	handler := newDeadTasksHandler(nil)

	rec := doDeadTasksRequest(handler, http.MethodGet, "/tasks/dead")
	if http.StatusOK != rec.Code {
		t.Fatalf("list status %d", rec.Code)
	}
	var page deadTasksPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); nil != err {
		t.Fatal(err)
	}
	if nil == page.Items || 0 != len(page.Items) {
		t.Fatalf("expected empty list, got %+v", page)
	}
	if rec := doDeadTasksRequest(handler, http.MethodPost, "/tasks/dead/1/retry"); http.StatusConflict != rec.Code {
		t.Fatalf("retry status %d", rec.Code)
	}
	if rec := doDeadTasksRequest(handler, http.MethodDelete, "/tasks/dead/1"); http.StatusConflict != rec.Code {
		t.Fatalf("drop status %d", rec.Code)
	}
}
