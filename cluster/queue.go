package cluster

import (
	"context"
	"errors"
	"time"
)

const (
	DefaultTaskMaxDeliver = 8
	DefaultTaskMaxPending = 100000
	TaskDedupWindow       = 10 * time.Minute
)

func DefaultTaskBackoff() []time.Duration {
	return []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute}
}

func TaskRetryDelay(backoff []time.Duration, attempt int) time.Duration {
	if 0 == len(backoff) || 1 > attempt {
		return 0
	}
	return backoff[min(attempt, len(backoff))-1]
}

type Queue interface {
	Enqueue(ctx context.Context, pool, affinity string, id string, body any) error
	Consume(pool string, n int, h TaskHandler) (stop func())
}

type TaskHandler func(ctx context.Context, t *Task) error

type Task struct {
	Pool       string
	ID         string
	Affinity   string
	Attempt    int
	EnqueuedAt time.Time
	Body       []byte
	progress   func() error
}

func NewTask(pool, id, affinity string, attempt int, enqueuedAt time.Time, body []byte, progress func() error) *Task {
	return &Task{
		Pool:       pool,
		ID:         id,
		Affinity:   affinity,
		Attempt:    attempt,
		EnqueuedAt: enqueuedAt,
		Body:       body,
		progress:   progress,
	}
}

func (t *Task) Decode(v any) error {
	return Decode(t.Body, v)
}

func (t *Task) InProgress() error {
	if nil == t.progress {
		return nil
	}
	return t.progress()
}

type DeadLetter struct {
	Pool       string    `json:"pool"`
	ID         string    `json:"id"`
	Affinity   string    `json:"affinity,omitempty"`
	Attempts   int       `json:"attempts"`
	Error      string    `json:"error"`
	EnqueuedAt time.Time `json:"enqueued_at"`
	FailedAt   time.Time `json:"failed_at"`
	Body       []byte    `json:"body,omitempty"`
}

type permanentError struct {
	err error
}

func (e *permanentError) Error() string {
	return e.err.Error()
}

func (e *permanentError) Unwrap() error {
	return e.err
}

func Permanent(err error) error {
	if nil == err {
		return nil
	}
	return &permanentError{err: err}
}

func IsPermanent(err error) bool {
	var target *permanentError
	return errors.As(err, &target)
}
