package natsbus

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/cluster"
)

const (
	headerMsgID      = "Nats-Msg-Id"
	headerEnqueuedAt = "Callfs-Enqueued-At"
	taskAckWait      = 2 * time.Minute
	setupTimeout     = 15 * time.Second
)

type Queue struct {
	js         jetstream.JetStream
	nc         *nats.Conn
	self       string
	maxDeliver int
	backoff    []time.Duration
	logger     *zap.Logger
}

var _ cluster.Queue = (*Queue)(nil)

func NewQueue(js jetstream.JetStream, nc *nats.Conn, self string, maxDeliver int, backoff []time.Duration, logger *zap.Logger) *Queue {
	if nil == logger {
		logger = zap.NewNop()
	}
	return &Queue{js: js, nc: nc, self: self, maxDeliver: maxDeliver, backoff: backoff, logger: logger}
}

func (q *Queue) Enqueue(ctx context.Context, pool, affinity string, id string, body any) error {
	if !cluster.ValidToken(pool) {
		return cluster.ErrInvalidName
	}
	if "" == id {
		return cluster.ErrInvalidTask
	}
	raw, err := cluster.Encode(body)
	if nil != err {
		return cluster.NewError(400, "encode", err.Error())
	}
	msg := nats.NewMsg(cluster.TaskSubject(pool, affinity))
	msg.Data = raw
	msg.Header.Set(headerMsgID, pool+":"+id)
	msg.Header.Set(headerEnqueuedAt, time.Now().UTC().Format(time.RFC3339Nano))
	if _, err := q.js.PublishMsg(ctx, msg); nil != err {
		return fmt.Errorf("enqueue task %s/%s: %w", pool, id, err)
	}
	return nil
}

func (q *Queue) Consume(pool string, n int, h cluster.TaskHandler) func() {
	if !cluster.ValidToken(pool) || nil == h {
		return func() {}
	}
	if 1 > n {
		n = 1
	}
	setupCtx, cancelSetup := context.WithTimeout(context.Background(), setupTimeout)
	defer cancelSetup()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var iters []jetstream.MessagesContext

	specs := []struct {
		name    string
		subject string
		node    bool
	}{
		{cluster.TaskConsumerAny(pool), cluster.TaskAnySubject(pool), false},
		{cluster.TaskConsumerNode(pool, q.self), cluster.TaskNodeSubject(pool, q.self), true},
	}
	for _, spec := range specs {
		cons, err := q.js.CreateOrUpdateConsumer(setupCtx, cluster.StreamTasks, jetstream.ConsumerConfig{
			Durable:       spec.name,
			FilterSubject: spec.subject,
			AckPolicy:     jetstream.AckExplicitPolicy,
			AckWait:       taskAckWait,
			MaxDeliver:    q.maxDeliver,
			BackOff:       q.backoff,
			MaxAckPending: 2 * n,
		})
		if nil != err {
			q.logger.Error("task consumer setup failed", zap.String("consumer", spec.name), zap.Error(err))
			continue
		}
		iter, err := cons.Messages(jetstream.PullMaxMessages(n))
		if nil != err {
			q.logger.Error("task consumer start failed", zap.String("consumer", spec.name), zap.Error(err))
			continue
		}
		iters = append(iters, iter)
		affinity := ""
		if spec.node {
			affinity = q.self
		}
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				q.work(ctx, iter, pool, affinity, h)
			}()
		}
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			for _, iter := range iters {
				iter.Stop()
			}
			wg.Wait()
		})
	}
}

func (q *Queue) work(ctx context.Context, iter jetstream.MessagesContext, pool, affinity string, h cluster.TaskHandler) {
	for {
		msg, err := iter.Next()
		if nil != err {
			if errors.Is(err, jetstream.ErrMsgIteratorClosed) || nil != ctx.Err() {
				return
			}
			q.logger.Warn("task fetch failed", zap.String("pool", pool), zap.Error(err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		q.run(ctx, msg, pool, affinity, h)
	}
}

func (q *Queue) run(ctx context.Context, msg jetstream.Msg, pool, affinity string, h cluster.TaskHandler) {
	attempt := 1
	if meta, err := msg.Metadata(); nil == err {
		attempt = int(meta.NumDelivered)
	}
	header := msg.Headers()
	id := strings.TrimPrefix(header.Get(headerMsgID), pool+":")
	enqueuedAt, _ := time.Parse(time.RFC3339Nano, header.Get(headerEnqueuedAt))
	task := cluster.NewTask(pool, id, affinity, attempt, enqueuedAt, msg.Data(), msg.InProgress)
	err := invoke(ctx, h, task)
	switch {
	case nil == err:
		if ackErr := msg.Ack(); nil != ackErr {
			q.logger.Warn("task ack failed", zap.String("pool", pool), zap.String("id", id), zap.Error(ackErr))
		}
	case cluster.IsPermanent(err) || q.permanentStatus(err):
		if termErr := msg.Term(); nil != termErr {
			q.logger.Warn("task term failed", zap.String("pool", pool), zap.String("id", id), zap.Error(termErr))
		}
	default:
		if nakErr := msg.Nak(); nil != nakErr {
			q.logger.Warn("task nak failed", zap.String("pool", pool), zap.String("id", id), zap.Error(nakErr))
		}
	}
}

func (q *Queue) permanentStatus(err error) bool {
	status := cluster.StatusOf(err)
	return 400 <= status && 499 >= status
}

func invoke(ctx context.Context, h cluster.TaskHandler, t *cluster.Task) (err error) {
	defer func() {
		if recovered := recover(); nil != recovered {
			err = fmt.Errorf("task handler panic: %v", recovered)
		}
	}()
	return h(ctx, t)
}
