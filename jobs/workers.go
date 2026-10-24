package jobs

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/cluster"
)

const (
	ScopeNode            = "node"
	ScopeCluster         = "cluster"
	DefaultLeaseTTL      = 30 * time.Second
	jobsService          = "jobs"
	jobsRunNowMethod     = "run_now"
	jobsServeConcurrency = 4
)

type WorkerFunc func(ctx context.Context) (any, error)

type Cluster struct {
	Self     string
	Lease    cluster.Lease
	Bus      cluster.Bus
	LeaseTTL time.Duration
}

type runNowRequest struct {
	Name  string `json:"name"`
	Actor string `json:"actor"`
}

type worker struct {
	name         string
	interval     time.Duration
	scope        string
	fn           WorkerFunc
	running      bool
	holding      bool
	lastKey      string
	lastJob      Job
	lastRunAt    *time.Time
	lastDuration time.Duration
	lastResult   any
	lastError    string
	nextRunAt    *time.Time
}

type WorkerStatus struct {
	Name           string     `json:"name"`
	IntervalS      int64      `json:"interval_s"`
	Scope          string     `json:"scope"`
	LeaseHeld      bool       `json:"lease_held,omitempty"`
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
		Scope:          w.scope,
		LeaseHeld:      w.holding,
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
	return r.register(name, interval, ScopeNode, fn)
}

func (r *Runner) RegisterClusterWorker(name string, interval time.Duration, fn WorkerFunc) error {
	if !cluster.ValidToken(name) {
		return fmt.Errorf("worker %s name must use only letters, digits, - and _", name)
	}
	return r.register(name, interval, ScopeCluster, fn)
}

func (r *Runner) UseCluster(c Cluster) error {
	if nil == c.Lease {
		return errors.New("cluster lease is required")
	}
	if 0 >= c.LeaseTTL {
		c.LeaseTTL = DefaultLeaseTTL
	}
	r.mu.Lock()
	if nil != r.baseCtx {
		r.mu.Unlock()
		return ErrAlreadyStarted
	}
	r.cluster = &c
	r.self = c.Self
	r.mu.Unlock()
	if nil == c.Bus {
		return nil
	}
	if err := c.Bus.Serve(jobsService, r.serve, cluster.WithConcurrency(jobsServeConcurrency)); nil != err {
		return fmt.Errorf("serve jobs over the cluster bus: %w", err)
	}
	return nil
}

func (r *Runner) register(name string, interval time.Duration, scope string, fn WorkerFunc) error {
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
	w := &worker{name: name, interval: interval, scope: scope, fn: fn}
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
	if ScopeCluster != w.scope || nil == r.cluster {
		r.tick(ctx, w)
		return
	}
	lease := r.cluster.Lease
	cluster.Singleton(ctx, lease, w.name, r.cluster.LeaseTTL, func(heldCtx context.Context) error {
		r.setHolding(w, true)
		defer r.setHolding(w, false)
		r.logger.Info("worker lease acquired", zap.String("worker", w.name))
		r.tick(heldCtx, w)
		r.logger.Info("worker lease released", zap.String("worker", w.name))
		return nil
	}, func(err error) {
		r.logger.Warn("worker lease failed", zap.String("worker", w.name), zap.Error(err))
	})
}

func (r *Runner) setHolding(w *worker, holding bool) {
	r.mu.Lock()
	w.holding = holding
	r.mu.Unlock()
}

func (r *Runner) tick(ctx context.Context, w *worker) {
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
	if ScopeCluster == w.scope && nil != r.cluster && !w.holding {
		r.mu.Unlock()
		return r.runRemote(name, actor)
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

func (r *Runner) runRemote(name, actor string) (Job, error) {
	if nil == r.cluster.Bus {
		return Job{}, ErrWorkerElsewhere
	}
	ctx, cancel := context.WithTimeout(context.Background(), cluster.DefaultCallTimeout)
	defer cancel()
	holder, err := r.cluster.Lease.Holder(ctx, name)
	if nil != err {
		return Job{}, fmt.Errorf("find the server running worker %s: %w", name, err)
	}
	if "" == holder || r.self == holder {
		return Job{}, ErrWorkerElsewhere
	}
	var job Job
	callErr := r.cluster.Bus.Call(ctx, holder, jobsService, jobsRunNowMethod, runNowRequest{Name: name, Actor: actor}, &job,
		cluster.WithIdempotencyKey(rand.Text()), cluster.Idempotent())
	if nil == callErr {
		return job, nil
	}
	switch cluster.StatusOf(callErr) {
	case http.StatusNotFound:
		return Job{}, fmt.Errorf("%w on server %s", ErrWorkerNotFound, holder)
	case http.StatusConflict:
		return Job{}, fmt.Errorf("%w on server %s", ErrWorkerBusy, holder)
	}
	return Job{}, fmt.Errorf("run worker %s on server %s: %w", name, holder, callErr)
}

func (r *Runner) serve(_ context.Context, req *cluster.Request) (any, error) {
	if jobsRunNowMethod != req.Method {
		return nil, cluster.NewError(http.StatusNotFound, "unknown_method", "unknown jobs method "+req.Method)
	}
	var body runNowRequest
	if err := req.Decode(&body); nil != err {
		return nil, cluster.NewError(http.StatusBadRequest, "decode", err.Error())
	}
	r.mu.Lock()
	w, ok := r.workers[body.Name]
	if ok && "" != req.IdempotencyKey && req.IdempotencyKey == w.lastKey {
		job := w.lastJob
		r.mu.Unlock()
		return job, nil
	}
	held := ok && (ScopeCluster != w.scope || w.holding)
	r.mu.Unlock()
	if !ok {
		return nil, cluster.NewError(http.StatusNotFound, "worker_not_found", ErrWorkerNotFound.Error())
	}
	if !held {
		return nil, cluster.NewError(http.StatusConflict, "worker_elsewhere", ErrWorkerElsewhere.Error())
	}
	job, err := r.RunNow(body.Name, body.Actor)
	if errors.Is(err, ErrWorkerBusy) {
		return nil, cluster.NewError(http.StatusConflict, "worker_busy", err.Error())
	}
	if nil != err {
		return nil, err
	}
	if "" != req.IdempotencyKey {
		r.mu.Lock()
		w.lastKey = req.IdempotencyKey
		w.lastJob = job
		r.mu.Unlock()
	}
	return job, nil
}
