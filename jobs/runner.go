package jobs

import (
	"context"
	"crypto/rand"
	"slices"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/metrics"
)

const (
	historyLimit     = 500
	progressInterval = 500 * time.Millisecond
)

type Publisher interface {
	Publish(eventType string, payload any)
}

type Filter struct {
	Kind      string
	CreatedBy string
	States    []State
}

type Runner struct {
	mu          sync.Mutex
	publisher   Publisher
	logger      *zap.Logger
	records     map[string]*record
	order       []string
	workers     map[string]*worker
	workerOrder []string
	baseCtx     context.Context
	cluster     *Cluster
	self        string
}

func NewRunner(publisher Publisher, logger *zap.Logger) *Runner {
	if nil == logger {
		logger = zap.NewNop()
	}
	return &Runner{
		publisher: publisher,
		logger:    logger,
		records:   make(map[string]*record),
		workers:   make(map[string]*worker),
	}
}

func (r *Runner) Submit(kind, createdBy string, fn JobFunc) Job {
	return r.submit(kind, createdBy, "", nil, fn)
}

func (r *Runner) submit(kind, createdBy, retryOf string, retrying []FailedItem, fn JobFunc) Job {
	ctx, cancel := context.WithCancel(context.Background())
	rec := &record{
		job: Job{
			ID:        rand.Text(),
			Kind:      kind,
			Node:      r.self,
			State:     StateQueued,
			CreatedBy: createdBy,
			RetryOf:   retryOf,
			CreatedAt: time.Now().UTC(),
			Failed:    []FailedItem{},
		},
		fn:       fn,
		cancel:   cancel,
		retrying: retrying,
	}
	r.mu.Lock()
	r.records[rec.job.ID] = rec
	r.order = append(r.order, rec.job.ID)
	r.evictLocked()
	snapshot := rec.snapshot()
	r.mu.Unlock()
	r.publish("job.queued", snapshot)
	go r.run(ctx, rec)
	return snapshot
}

func (r *Runner) run(ctx context.Context, rec *record) {
	defer rec.cancel()
	started := time.Now().UTC()
	r.mu.Lock()
	if !rec.cancelReq {
		rec.job.State = StateRunning
	}
	rec.job.StartedAt = &started
	snapshot := rec.snapshot()
	r.mu.Unlock()
	r.publish("job.running", snapshot)
	r.logger.Info("job started", zap.String("job_id", rec.job.ID), zap.String("kind", rec.job.Kind))
	result, err := rec.fn(ctx, &Progress{runner: r, rec: rec})
	r.finish(rec, started, result, err, nil != ctx.Err())
}

func (r *Runner) finish(rec *record, started time.Time, result any, runErr error, cancelled bool) {
	finished := time.Now().UTC()
	var message *string
	r.mu.Lock()
	switch {
	case cancelled:
		rec.job.State = StateCancelled
	case nil != runErr:
		rec.job.State = StateFailed
		text := runErr.Error()
		message = &text
	case 0 < rec.failedCount:
		rec.job.State = StatePartial
	default:
		rec.job.State = StateSucceeded
	}
	rec.job.Result = result
	rec.job.Error = message
	rec.job.FinishedAt = &finished
	snapshot := rec.snapshot()
	state := rec.job.State
	kind := rec.job.Kind
	r.mu.Unlock()
	metrics.JobsTotal.WithLabelValues(kind, string(state)).Inc()
	r.publish("job."+string(state), snapshot)
	r.logger.Info("job finished",
		zap.String("job_id", rec.job.ID),
		zap.String("kind", rec.job.Kind),
		zap.String("state", string(state)),
		zap.Duration("duration", finished.Sub(started)),
		zap.Error(runErr),
	)
}

func (r *Runner) mutate(rec *record, throttled bool, apply func(rec *record)) {
	r.mu.Lock()
	apply(rec)
	now := time.Now()
	publish := !throttled || progressInterval <= now.Sub(rec.lastPublish)
	if publish {
		rec.lastPublish = now
	}
	snapshot := rec.snapshot()
	r.mu.Unlock()
	if publish {
		r.publish("job.progress", snapshot)
	}
}

func (r *Runner) publish(eventType string, job Job) {
	if nil != r.publisher {
		r.publisher.Publish(eventType, job)
	}
}

func (r *Runner) Get(id string) (Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.records[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	return rec.snapshot(), nil
}

func (f Filter) matches(rec *record) bool {
	if "" != f.Kind && f.Kind != rec.job.Kind {
		return false
	}
	if "" != f.CreatedBy && f.CreatedBy != rec.job.CreatedBy {
		return false
	}
	if 0 < len(f.States) && !slices.Contains(f.States, rec.job.State) {
		return false
	}
	return true
}

func (r *Runner) List(filter Filter) []Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]Job, 0, len(r.order))
	for i := len(r.order) - 1; 0 <= i; i-- {
		rec := r.records[r.order[i]]
		if !filter.matches(rec) {
			continue
		}
		items = append(items, rec.snapshot())
	}
	return items
}

func (r *Runner) ListPage(filter Filter, cursorID string, limit int) ([]Job, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]Job, 0, min(limit, len(r.order)))
	seeking := "" != cursorID
	for i := len(r.order) - 1; 0 <= i; i-- {
		rec := r.records[r.order[i]]
		if !filter.matches(rec) {
			continue
		}
		if seeking {
			if cursorID == rec.job.ID {
				seeking = false
			}
			continue
		}
		if limit <= len(items) {
			return items, true, nil
		}
		items = append(items, rec.snapshot())
	}
	if seeking {
		return nil, false, ErrCursorNotFound
	}
	return items, false, nil
}

func (r *Runner) Cancel(id string) (Job, error) {
	r.mu.Lock()
	rec, ok := r.records[id]
	if !ok {
		r.mu.Unlock()
		return Job{}, ErrNotFound
	}
	if rec.job.State.Terminal() {
		r.mu.Unlock()
		return Job{}, ErrInvalidState
	}
	if rec.cancelReq {
		snapshot := rec.snapshot()
		r.mu.Unlock()
		return snapshot, nil
	}
	rec.cancelReq = true
	rec.job.State = StateCancelling
	snapshot := rec.snapshot()
	r.mu.Unlock()
	rec.cancel()
	r.publish("job.cancelling", snapshot)
	return snapshot, nil
}

func (r *Runner) RetryFailed(id, actor string) (Job, error) {
	r.mu.Lock()
	rec, ok := r.records[id]
	if !ok {
		r.mu.Unlock()
		return Job{}, ErrNotFound
	}
	if !rec.job.State.Terminal() {
		r.mu.Unlock()
		return Job{}, ErrInvalidState
	}
	if 0 == len(rec.job.Failed) {
		r.mu.Unlock()
		return Job{}, ErrNothingToRetry
	}
	kind := rec.job.Kind
	failed := append([]FailedItem{}, rec.job.Failed...)
	fn := rec.fn
	r.mu.Unlock()
	return r.submit(kind, actor, id, failed, fn), nil
}

func (r *Runner) evictLocked() {
	for i := 0; historyLimit < len(r.order) && i < len(r.order); {
		id := r.order[i]
		if !r.records[id].job.State.Terminal() {
			i++
			continue
		}
		delete(r.records, id)
		r.order = append(r.order[:i], r.order[i+1:]...)
	}
}
