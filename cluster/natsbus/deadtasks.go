package natsbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/ebogdum/callfs/v2/cluster"
)

const (
	deadSummaryMax    = 200
	deadStepTimeout   = 15 * time.Second
	deadUnknownReason = "unknown"
)

type DeadTasks struct {
	js jetstream.JetStream
}

var _ cluster.DeadTasks = (*DeadTasks)(nil)

func NewDeadTasks(js jetstream.JetStream) *DeadTasks {
	return &DeadTasks{js: js}
}

func (d *DeadTasks) List(ctx context.Context, limit int, before uint64) ([]cluster.DeadTask, bool, error) {
	stepCtx, cancel := context.WithTimeout(ctx, deadStepTimeout)
	defer cancel()
	stream, err := d.js.Stream(stepCtx, cluster.StreamDLQ)
	if nil != err {
		return nil, false, fmt.Errorf("dead task stream lookup: %w", err)
	}
	info, err := stream.Info(stepCtx)
	if nil != err {
		return nil, false, fmt.Errorf("dead task stream info: %w", err)
	}
	start := info.State.LastSeq
	if 0 < before && before-1 < start {
		start = before - 1
	}
	items := make([]cluster.DeadTask, 0, limit)
	for seq := start; 0 < seq && info.State.FirstSeq <= seq; seq-- {
		raw, err := stream.GetMsg(stepCtx, seq)
		if nil != err {
			if errors.Is(err, jetstream.ErrMsgNotFound) {
				continue
			}
			return nil, false, fmt.Errorf("get dead task %d: %w", seq, err)
		}
		if limit == len(items) {
			return items, true, nil
		}
		items = append(items, deadTaskOf(raw))
	}
	return items, false, nil
}

func (d *DeadTasks) Retry(ctx context.Context, id uint64) error {
	stepCtx, cancel := context.WithTimeout(ctx, deadStepTimeout)
	defer cancel()
	stream, err := d.js.Stream(stepCtx, cluster.StreamDLQ)
	if nil != err {
		return fmt.Errorf("dead task stream lookup: %w", err)
	}
	raw, err := stream.GetMsg(stepCtx, id)
	if nil != err {
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			return cluster.ErrDeadTaskNotFound
		}
		return fmt.Errorf("get dead task %d: %w", id, err)
	}
	pool := deadPool(raw.Subject)
	if "" == pool {
		return fmt.Errorf("unexpected dead task subject %q", raw.Subject)
	}
	subject := raw.Header.Get(headerDLQSubject)
	if "" == subject {
		subject = cluster.TaskSubject(pool, "")
	}
	out := nats.NewMsg(subject)
	out.Data = raw.Data
	for k, vals := range raw.Header {
		for _, v := range vals {
			out.Header.Add(k, v)
		}
	}
	out.Header.Del(headerDLQError)
	out.Header.Del(headerDLQSubject)
	out.Header.Set(headerMsgID, fmt.Sprintf("%s:retry-%d-%d", pool, id, time.Now().UnixNano()))
	out.Header.Set(headerEnqueuedAt, time.Now().UTC().Format(time.RFC3339Nano))
	if _, err := d.js.PublishMsg(stepCtx, out); nil != err {
		return fmt.Errorf("requeue dead task %d: %w", id, err)
	}
	if err := stream.DeleteMsg(stepCtx, id); nil != err {
		return fmt.Errorf("delete dead task %d: %w", id, err)
	}
	return nil
}

func (d *DeadTasks) Drop(ctx context.Context, id uint64) error {
	stepCtx, cancel := context.WithTimeout(ctx, deadStepTimeout)
	defer cancel()
	stream, err := d.js.Stream(stepCtx, cluster.StreamDLQ)
	if nil != err {
		return fmt.Errorf("dead task stream lookup: %w", err)
	}
	if err := stream.DeleteMsg(stepCtx, id); nil != err {
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			return cluster.ErrDeadTaskNotFound
		}
		return fmt.Errorf("delete dead task %d: %w", id, err)
	}
	return nil
}

func deadPool(subject string) string {
	parts := strings.Split(subject, ".")
	if 3 != len(parts) {
		return ""
	}
	return parts[2]
}

func deadTaskOf(raw *jetstream.RawStreamMsg) cluster.DeadTask {
	reason := raw.Header.Get(headerDLQError)
	if "" == reason {
		reason = deadUnknownReason
	}
	return cluster.DeadTask{
		ID:       raw.Sequence,
		Pool:     deadPool(raw.Subject),
		Reason:   reason,
		FailedAt: raw.Time,
		Summary:  deadSummary(raw.Data),
	}
}

func deadSummary(data []byte) string {
	var task struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(data, &task); nil == err && "" != task.Path {
		return task.Path
	}
	text := string(data)
	if deadSummaryMax < len(text) {
		return text[:deadSummaryMax]
	}
	return text
}
