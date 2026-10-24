package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type recordingPublisher struct {
	mu     sync.Mutex
	events []string
}

func (p *recordingPublisher) Publish(eventType string, _ any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, eventType)
}

func (p *recordingPublisher) has(eventType string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, event := range p.events {
		if eventType == event {
			return true
		}
	}
	return false
}

func waitForState(t *testing.T, r *Runner, id string, want State) Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		job, err := r.Get(id)
		if nil != err {
			t.Fatalf("get %s: %v", id, err)
		}
		if want == job.State {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s state %s, want %s", id, job.State, want)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestSubmitSucceeds(t *testing.T) {
	pub := &recordingPublisher{}
	r := NewRunner(pub, nil)
	job := r.Submit("link_cleanup", "alice", func(ctx context.Context, p *Progress) (any, error) {
		return "ok", nil
	})
	done := waitForState(t, r, job.ID, StateSucceeded)
	if "ok" != done.Result {
		t.Fatalf("result = %v, want ok", done.Result)
	}
	if nil == done.FinishedAt {
		t.Fatal("finished_at not set")
	}
	deadline := time.Now().Add(5 * time.Second)
	for !pub.has("job.succeeded") {
		if time.Now().After(deadline) {
			t.Fatalf("events = %v, want job.succeeded", pub.events)
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !pub.has("job.queued") {
		t.Fatalf("events = %v, want job.queued", pub.events)
	}
}

func TestSubmitPartialWhenItemsFail(t *testing.T) {
	r := NewRunner(nil, nil)
	job := r.Submit("erasure_repair", "alice", func(ctx context.Context, p *Progress) (any, error) {
		p.SetTotal(2)
		p.Advance(1)
		p.Fail(FailedItem{Path: "/a.bin", Code: "IO", Reason: "disk full"})
		return nil, nil
	})
	done := waitForState(t, r, job.ID, StatePartial)
	if 1 != done.Progress.Failed || 1 != len(done.Failed) {
		t.Fatalf("failed = %d items = %d, want 1 and 1", done.Progress.Failed, len(done.Failed))
	}
	if "/a.bin" != done.Failed[0].Path {
		t.Fatalf("failed path = %s, want /a.bin", done.Failed[0].Path)
	}
}

func TestSubmitFailedWithError(t *testing.T) {
	r := NewRunner(nil, nil)
	job := r.Submit("trash_cleanup", "alice", func(ctx context.Context, p *Progress) (any, error) {
		return nil, errors.New("store unavailable")
	})
	done := waitForState(t, r, job.ID, StateFailed)
	if nil == done.Error || "store unavailable" != *done.Error {
		t.Fatalf("error = %v, want store unavailable", done.Error)
	}
}

func TestCancelStopsRunningJob(t *testing.T) {
	r := NewRunner(nil, nil)
	started := make(chan struct{})
	job := r.Submit("usage_refresh", "alice", func(ctx context.Context, p *Progress) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	<-started
	if _, err := r.Cancel(job.ID); nil != err {
		t.Fatalf("cancel: %v", err)
	}
	waitForState(t, r, job.ID, StateCancelled)
	if _, err := r.Cancel(job.ID); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("cancel finished job err = %v, want ErrInvalidState", err)
	}
}

func TestRetryFailedPassesFailedItems(t *testing.T) {
	r := NewRunner(nil, nil)
	retried := make(chan []FailedItem, 1)
	original := r.Submit("erasure_repair", "alice", func(ctx context.Context, p *Progress) (any, error) {
		if 0 == len(p.Retrying()) {
			p.Fail(FailedItem{Path: "/a.bin", Code: "IO", Reason: "timeout"})
			return nil, nil
		}
		retried <- p.Retrying()
		return nil, nil
	})
	waitForState(t, r, original.ID, StatePartial)
	retry, err := r.RetryFailed(original.ID, "alice")
	if nil != err {
		t.Fatalf("retry: %v", err)
	}
	if original.ID != retry.RetryOf {
		t.Fatalf("retry_of = %s, want %s", retry.RetryOf, original.ID)
	}
	items := <-retried
	if 1 != len(items) || "/a.bin" != items[0].Path {
		t.Fatalf("retried items = %v, want /a.bin", items)
	}
	waitForState(t, r, retry.ID, StateSucceeded)
}

func TestRetryFailedWithoutFailedItems(t *testing.T) {
	r := NewRunner(nil, nil)
	job := r.Submit("metrics_sampler", "alice", func(ctx context.Context, p *Progress) (any, error) {
		return nil, nil
	})
	waitForState(t, r, job.ID, StateSucceeded)
	if _, err := r.RetryFailed(job.ID, "alice"); !errors.Is(err, ErrNothingToRetry) {
		t.Fatalf("retry err = %v, want ErrNothingToRetry", err)
	}
}

func TestListFiltersByState(t *testing.T) {
	r := NewRunner(nil, nil)
	job := r.Submit("link_cleanup", "alice", func(ctx context.Context, p *Progress) (any, error) {
		return nil, nil
	})
	waitForState(t, r, job.ID, StateSucceeded)
	r.Submit("link_cleanup", "bob", func(ctx context.Context, p *Progress) (any, error) {
		return nil, errors.New("boom")
	})
	items := r.List(Filter{State: StateSucceeded, CreatedBy: "alice"})
	if 1 != len(items) || job.ID != items[0].ID {
		t.Fatalf("list = %v, want only alice succeeded job", items)
	}
}

func TestRegisterWorkerValidation(t *testing.T) {
	r := NewRunner(nil, nil)
	noop := func(ctx context.Context) (any, error) { return nil, nil }
	if err := r.RegisterWorker("", time.Minute, noop); nil == err {
		t.Fatal("empty name accepted")
	}
	if err := r.RegisterWorker("link_cleanup", time.Millisecond, noop); nil == err {
		t.Fatal("interval below 1s accepted")
	}
	if err := r.RegisterWorker("link_cleanup", time.Minute, nil); nil == err {
		t.Fatal("nil function accepted")
	}
	if err := r.RegisterWorker("link_cleanup", time.Minute, noop); nil != err {
		t.Fatalf("register: %v", err)
	}
	if err := r.RegisterWorker("link_cleanup", time.Minute, noop); nil == err {
		t.Fatal("duplicate name accepted")
	}
}

func TestRunNowRecordsWorkerStatus(t *testing.T) {
	r := NewRunner(nil, nil)
	if err := r.RegisterWorker("trash_cleanup", time.Hour, func(ctx context.Context) (any, error) {
		return "removed 3", nil
	}); nil != err {
		t.Fatalf("register: %v", err)
	}
	job, err := r.RunNow("trash_cleanup", "alice")
	if nil != err {
		t.Fatalf("run now: %v", err)
	}
	waitForState(t, r, job.ID, StateSucceeded)
	status := r.Workers()
	if 1 != len(status) || "trash_cleanup" != status[0].Name {
		t.Fatalf("workers = %v, want trash_cleanup", status)
	}
	if status[0].Running || nil == status[0].LastRunAt || "removed 3" != status[0].LastResult {
		t.Fatalf("status = %+v, want finished run with result", status[0])
	}
	if nil != status[0].LastError {
		t.Fatalf("last error = %s, want none", *status[0].LastError)
	}
}

func TestRunNowBusyAndUnknown(t *testing.T) {
	r := NewRunner(nil, nil)
	release := make(chan struct{})
	if err := r.RegisterWorker("upload_cleanup", time.Hour, func(ctx context.Context) (any, error) {
		<-release
		return nil, nil
	}); nil != err {
		t.Fatalf("register: %v", err)
	}
	if _, err := r.RunNow("missing", "alice"); !errors.Is(err, ErrWorkerNotFound) {
		t.Fatalf("unknown worker err = %v, want ErrWorkerNotFound", err)
	}
	first, err := r.RunNow("upload_cleanup", "alice")
	if nil != err {
		t.Fatalf("first run: %v", err)
	}
	if _, err := r.RunNow("upload_cleanup", "alice"); !errors.Is(err, ErrWorkerBusy) {
		t.Fatalf("second run err = %v, want ErrWorkerBusy", err)
	}
	close(release)
	waitForState(t, r, first.ID, StateSucceeded)
}
