package admin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/jobs"
	"github.com/ebogdum/callfs/server/middleware"
)

const jobsTestActor = "alice"

type jobsTestAuthenticator struct{}

func (jobsTestAuthenticator) Authenticate(_ context.Context, token string) (string, error) {
	if jobsTestActor != token {
		return "", auth.ErrAuthenticationFailed
	}
	return token, nil
}

func newJobsTestHandler(runner *jobs.Runner) http.Handler {
	api := &API{Deps: Deps{Jobs: runner}}
	router := chi.NewRouter()
	router.Use(middleware.V1AuthMiddleware(jobsTestAuthenticator{}, nil, nil, zap.NewNop(), nil))
	router.Get("/v1/admin/jobs", api.Jobs)
	router.Get("/v1/admin/jobs/{id}", api.Job)
	router.Post("/v1/admin/jobs/{id}/cancel", api.CancelJob)
	router.Post("/v1/admin/jobs/{id}/retry-failed", api.RetryJobFailed)
	router.Get("/v1/admin/workers", api.Workers)
	router.Post("/v1/admin/workers/{name}/run", api.RunWorker)
	return router
}

func doJobsRequest(handler http.Handler, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Authorization", jobsTestActor)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func waitForJobTerminal(t *testing.T, runner *jobs.Runner, id string) jobs.Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		job, err := runner.Get(id)
		if nil != err {
			t.Fatalf("get job %s: %v", id, err)
		}
		if job.State.Terminal() {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s did not finish, state %s", id, job.State)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestJobsListFiltersAndPaginates(t *testing.T) {
	runner := jobs.NewRunner(nil, zap.NewNop())
	noop := func(context.Context, *jobs.Progress) (any, error) { return nil, nil }
	for range 3 {
		runner.Submit("retag", jobsTestActor, noop)
	}
	runner.Submit("erasure_repair", jobsTestActor, noop)
	handler := newJobsTestHandler(runner)

	first := doJobsRequest(handler, http.MethodGet, "/v1/admin/jobs?kind=retag&limit=2")
	if http.StatusOK != first.Code {
		t.Fatalf("first page status %d: %s", first.Code, first.Body.String())
	}
	var firstPage jobsPage
	if err := json.Unmarshal(first.Body.Bytes(), &firstPage); nil != err {
		t.Fatalf("decode first page: %v", err)
	}
	if 2 != len(firstPage.Items) || nil == firstPage.NextCursor {
		t.Fatalf("first page items %d next_cursor %v", len(firstPage.Items), firstPage.NextCursor)
	}

	second := doJobsRequest(handler, http.MethodGet, "/v1/admin/jobs?kind=retag&limit=2&cursor="+*firstPage.NextCursor)
	if http.StatusOK != second.Code {
		t.Fatalf("second page status %d: %s", second.Code, second.Body.String())
	}
	var secondPage jobsPage
	if err := json.Unmarshal(second.Body.Bytes(), &secondPage); nil != err {
		t.Fatalf("decode second page: %v", err)
	}
	if 1 != len(secondPage.Items) || nil != secondPage.NextCursor {
		t.Fatalf("second page items %d next_cursor %v", len(secondPage.Items), secondPage.NextCursor)
	}
	if "retag" != secondPage.Items[0].Kind {
		t.Fatalf("second page kind %q", secondPage.Items[0].Kind)
	}
}

func TestJobsRejectsInvalidQuery(t *testing.T) {
	handler := newJobsTestHandler(jobs.NewRunner(nil, zap.NewNop()))
	targets := []string{
		"/v1/admin/jobs?state=bogus",
		"/v1/admin/jobs?limit=0",
		"/v1/admin/jobs?limit=1001",
		"/v1/admin/jobs?cursor=!!!",
		"/v1/admin/jobs?cursor=" + base64.RawURLEncoding.EncodeToString([]byte("missing")),
	}
	for _, target := range targets {
		rec := doJobsRequest(handler, http.MethodGet, target)
		if http.StatusBadRequest != rec.Code {
			t.Fatalf("%s status %d, want 400", target, rec.Code)
		}
	}
}

func TestJobGetReturnsJobOrNotFound(t *testing.T) {
	runner := jobs.NewRunner(nil, zap.NewNop())
	job := runner.Submit("retag", jobsTestActor, func(context.Context, *jobs.Progress) (any, error) { return nil, nil })
	handler := newJobsTestHandler(runner)

	rec := doJobsRequest(handler, http.MethodGet, "/v1/admin/jobs/"+job.ID)
	if http.StatusOK != rec.Code {
		t.Fatalf("get status %d: %s", rec.Code, rec.Body.String())
	}
	missing := doJobsRequest(handler, http.MethodGet, "/v1/admin/jobs/unknown")
	if http.StatusNotFound != missing.Code {
		t.Fatalf("missing status %d, want 404", missing.Code)
	}
}

func TestJobCancelRequestedAndUnknown(t *testing.T) {
	runner := jobs.NewRunner(nil, zap.NewNop())
	job := runner.Submit("retag", jobsTestActor, func(ctx context.Context, _ *jobs.Progress) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	handler := newJobsTestHandler(runner)

	rec := doJobsRequest(handler, http.MethodPost, "/v1/admin/jobs/"+job.ID+"/cancel")
	if http.StatusOK != rec.Code {
		t.Fatalf("cancel status %d: %s", rec.Code, rec.Body.String())
	}
	var cancelled jobs.Job
	if err := json.Unmarshal(rec.Body.Bytes(), &cancelled); nil != err {
		t.Fatalf("decode cancel: %v", err)
	}
	if jobs.StateCancelling != cancelled.State {
		t.Fatalf("cancel state %s, want cancelling", cancelled.State)
	}
	waitForJobTerminal(t, runner, job.ID)

	missing := doJobsRequest(handler, http.MethodPost, "/v1/admin/jobs/unknown/cancel")
	if http.StatusNotFound != missing.Code {
		t.Fatalf("cancel missing status %d, want 404", missing.Code)
	}
}

func TestJobRetryFailedAcceptedAndConflict(t *testing.T) {
	runner := jobs.NewRunner(nil, zap.NewNop())
	clean := runner.Submit("retag", jobsTestActor, func(context.Context, *jobs.Progress) (any, error) { return nil, nil })
	waitForJobTerminal(t, runner, clean.ID)
	handler := newJobsTestHandler(runner)

	conflict := doJobsRequest(handler, http.MethodPost, "/v1/admin/jobs/"+clean.ID+"/retry-failed")
	if http.StatusConflict != conflict.Code {
		t.Fatalf("retry without failures status %d, want 409", conflict.Code)
	}

	partial := runner.Submit("retag", jobsTestActor, func(_ context.Context, p *jobs.Progress) (any, error) {
		p.Fail(jobs.FailedItem{Path: "/a.txt", Code: "IO", Reason: "disk"})
		return nil, nil
	})
	waitForJobTerminal(t, runner, partial.ID)

	rec := doJobsRequest(handler, http.MethodPost, "/v1/admin/jobs/"+partial.ID+"/retry-failed")
	if http.StatusAccepted != rec.Code {
		t.Fatalf("retry status %d: %s", rec.Code, rec.Body.String())
	}
	var retried jobs.Job
	if err := json.Unmarshal(rec.Body.Bytes(), &retried); nil != err {
		t.Fatalf("decode retry: %v", err)
	}
	if partial.ID != retried.RetryOf {
		t.Fatalf("retry_of %q, want %q", retried.RetryOf, partial.ID)
	}
	waitForJobTerminal(t, runner, retried.ID)
}

func TestWorkersListsRegisteredWorkers(t *testing.T) {
	runner := jobs.NewRunner(nil, zap.NewNop())
	if err := runner.RegisterWorker("trash_cleanup", time.Hour, func(context.Context) (any, error) { return nil, nil }); nil != err {
		t.Fatalf("register worker: %v", err)
	}
	rec := doJobsRequest(newJobsTestHandler(runner), http.MethodGet, "/v1/admin/workers")
	if http.StatusOK != rec.Code {
		t.Fatalf("workers status %d: %s", rec.Code, rec.Body.String())
	}
	var page workersPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); nil != err {
		t.Fatalf("decode workers: %v", err)
	}
	if 1 != len(page.Items) || "trash_cleanup" != page.Items[0].Name || 3600 != page.Items[0].IntervalS {
		t.Fatalf("workers %+v", page.Items)
	}
}

func TestRunWorkerAcceptedNotFoundAndBusy(t *testing.T) {
	runner := jobs.NewRunner(nil, zap.NewNop())
	release := make(chan struct{})
	if err := runner.RegisterWorker("usage_refresh", time.Hour, func(ctx context.Context) (any, error) {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil, nil
	}); nil != err {
		t.Fatalf("register worker: %v", err)
	}
	handler := newJobsTestHandler(runner)

	accepted := doJobsRequest(handler, http.MethodPost, "/v1/admin/workers/usage_refresh/run")
	if http.StatusAccepted != accepted.Code {
		t.Fatalf("run status %d: %s", accepted.Code, accepted.Body.String())
	}
	var body jobAccepted
	if err := json.Unmarshal(accepted.Body.Bytes(), &body); nil != err {
		t.Fatalf("decode run: %v", err)
	}
	if "" == body.JobID {
		t.Fatal("run response has no job_id")
	}

	busy := doJobsRequest(handler, http.MethodPost, "/v1/admin/workers/usage_refresh/run")
	if http.StatusConflict != busy.Code {
		t.Fatalf("busy status %d, want 409", busy.Code)
	}

	missing := doJobsRequest(handler, http.MethodPost, "/v1/admin/workers/unknown/run")
	if http.StatusNotFound != missing.Code {
		t.Fatalf("unknown worker status %d, want 404", missing.Code)
	}

	close(release)
	waitForJobTerminal(t, runner, body.JobID)
}
