package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type State string

const (
	StateQueued     State = "queued"
	StateRunning    State = "running"
	StateCancelling State = "cancelling"
	StateSucceeded  State = "succeeded"
	StatePartial    State = "partial"
	StateFailed     State = "failed"
	StateCancelled  State = "cancelled"
)

const maxFailedItems = 1000

var (
	ErrNotFound        = errors.New("job not found")
	ErrCursorNotFound  = errors.New("cursor does not match any job")
	ErrInvalidState    = errors.New("job state does not allow this action")
	ErrNothingToRetry  = errors.New("job has no failed items to retry")
	ErrWorkerNotFound  = errors.New("worker not found")
	ErrWorkerBusy      = errors.New("worker is already running")
	ErrWorkerElsewhere = fmt.Errorf("%w on another server", ErrWorkerBusy)
	ErrAlreadyStarted  = errors.New("runner already started")
)

type JobFunc func(ctx context.Context, p *Progress) (any, error)

type Counts struct {
	Done   int `json:"done"`
	Total  int `json:"total"`
	Failed int `json:"failed"`
}

type FailedItem struct {
	Path   string `json:"path"`
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type Job struct {
	ID         string       `json:"id"`
	Kind       string       `json:"kind"`
	Node       string       `json:"node,omitempty"`
	State      State        `json:"state"`
	CreatedBy  string       `json:"created_by"`
	RetryOf    string       `json:"retry_of,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
	StartedAt  *time.Time   `json:"started_at"`
	FinishedAt *time.Time   `json:"finished_at"`
	Progress   Counts       `json:"progress"`
	Failed     []FailedItem `json:"failed_items"`
	Result     any          `json:"result"`
	Error      *string      `json:"error"`
}

func (s State) Terminal() bool {
	switch s {
	case StateSucceeded, StatePartial, StateFailed, StateCancelled:
		return true
	}
	return false
}

type Progress struct {
	runner *Runner
	rec    *record
}

func (p *Progress) SetTotal(total int) {
	p.runner.mutate(p.rec, true, func(rec *record) {
		rec.job.Progress.Total = total
	})
}

func (p *Progress) Advance(done int) {
	p.runner.mutate(p.rec, true, func(rec *record) {
		rec.job.Progress.Done += done
	})
}

func (p *Progress) Fail(item FailedItem) {
	p.runner.mutate(p.rec, true, func(rec *record) {
		rec.failedCount++
		if len(rec.job.Failed) < maxFailedItems {
			rec.job.Failed = append(rec.job.Failed, item)
		}
	})
}

func (p *Progress) Retrying() []FailedItem {
	p.runner.mu.Lock()
	defer p.runner.mu.Unlock()
	return append([]FailedItem{}, p.rec.retrying...)
}

type record struct {
	job         Job
	fn          JobFunc
	cancel      context.CancelFunc
	cancelReq   bool
	failedCount int
	retrying    []FailedItem
	lastPublish time.Time
}

func (rec *record) snapshot() Job {
	job := rec.job
	job.Failed = append([]FailedItem{}, rec.job.Failed...)
	job.Progress.Failed = rec.failedCount
	return job
}
