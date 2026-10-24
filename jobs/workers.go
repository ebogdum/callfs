package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

type WorkerFunc func(ctx context.Context) (any, error)

type worker struct {
	name         string
	interval     time.Duration
	fn           WorkerFunc
	running      bool
	lastRunAt    *time.Time
	lastDuration time.Duration
	lastResult   any
	lastError    string
	nextRunAt    *time.Time
}

type WorkerStatus struct {
	Name           string     `json:"name"`
	IntervalS      int64      `json:"interval_s"`
	Running        bool       `json:"running"`
	LastRunAt      *time.Time `json:"last_run_at"`
	LastDurationMs int64      `json:"last_duration_ms"`
	LastResult     any        `json:"last_result"`
	LastError      *string    `json:"last_error"`
	NextRunAt      *time.Time `json:"next_run_at"`
}

func (w *worker) status() WorkerStatus {
	status := WorkerStatus{
		Name:           w.name,
		IntervalS:      int64(w.interval / time.Second),
		Running:        w.running,
		LastRunAt:      w.lastRunAt,
		LastDurationMs: w.lastDuration.Milliseconds(),
		LastResult:     w.lastResult,
		NextRunAt:      w.nextRunAt,
	}
	if "" != w.lastError {
		message := w.lastError
		status.LastError = &message
	}
	return status
}

func (r *Runner) RegisterWorker(name string, interval time.Duration, fn WorkerFunc) error {
	if "" == name {
		return errors.New("worker name is required")
	}
	if time.Second > interval {
		return fmt.Errorf("worker %s interval must be at least 1s", name)
	}
	if nil == fn {
		return fmt.Errorf("worker %s has no function", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.workers[name]; exists {
		return fmt.Errorf("worker %s is already registered", name)
	}
	w := &worker{name: name, interval: interval, fn: fn}
	r.workers[name] = w
	r.workerOrder = append(r.workerOrder, name)
	if nil != r.baseCtx {
		next := time.Now().Add(interval)
		w.nextRunAt = &next
		go r.schedule(r.baseCtx, w)
	}
	return nil
}

func (r *Runner) Start(ctx context.Context) error {
	if nil == ctx {
		return errors.New("start context is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if nil != r.baseCtx {
		return ErrAlreadyStarted
	}
	r.baseCtx = ctx
	for _, name := range r.workerOrder {
		w := r.workers[name]
		next := time.Now().Add(w.interval)
		w.nextRunAt = &next
		go r.schedule(ctx, w)
	}
	return nil
}

func (r *Runner) schedule(ctx context.Context, w *worker) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			next := time.Now().Add(w.interval)
			r.mu.Lock()
			w.nextRunAt = &next
			claimed := !w.running
			if claimed {
				w.running = true
			}
			r.mu.Unlock()
			if claimed {
				r.execute(ctx, w)
			}
		}
	}
}

func (r *Runner) RunNow(name, actor string) (Job, error) {
	r.mu.Lock()
	w, ok := r.workers[name]
	if !ok {
		r.mu.Unlock()
		return Job{}, ErrWorkerNotFound
	}
	if w.running {
		r.mu.Unlock()
		return Job{}, ErrWorkerBusy
	}
	w.running = true
	r.mu.Unlock()
	return r.submit(name, actor, "", nil, func(ctx context.Context, _ *Progress) (any, error) {
		return r.execute(ctx, w)
	}), nil
}

func (r *Runner) Workers() []WorkerStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]WorkerStatus, 0, len(r.workerOrder))
	for _, name := range r.workerOrder {
		items = append(items, r.workers[name].status())
	}
	return items
}

func (r *Runner) execute(ctx context.Context, w *worker) (any, error) {
	started := time.Now().UTC()
	r.logger.Info("worker started", zap.String("worker", w.name))
	result, err := w.fn(ctx)
	duration := time.Since(started)
	r.mu.Lock()
	w.running = false
	w.lastRunAt = &started
	w.lastDuration = duration
	w.lastResult = result
	w.lastError = ""
	if nil != err {
		w.lastError = err.Error()
	}
	r.mu.Unlock()
	if nil != err {
		r.logger.Error("worker failed", zap.String("worker", w.name), zap.Duration("duration", duration), zap.Error(err))
	} else {
		r.logger.Info("worker finished", zap.String("worker", w.name), zap.Duration("duration", duration))
	}
	return result, err
}
