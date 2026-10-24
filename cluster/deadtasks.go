package cluster

import (
	"context"
	"errors"
	"time"
)

var ErrDeadTaskNotFound = errors.New("dead task not found")

type DeadTask struct {
	ID       uint64    `json:"id"`
	Pool     string    `json:"pool"`
	Reason   string    `json:"reason"`
	FailedAt time.Time `json:"failed_at"`
	Summary  string    `json:"summary"`
}

type DeadTasks interface {
	List(ctx context.Context, limit int, before uint64) ([]DeadTask, bool, error)
	Retry(ctx context.Context, id uint64) error
	Drop(ctx context.Context, id uint64) error
}
