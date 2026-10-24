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
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/cluster"
)

const (
	advisoryMaxDeliveries = "$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES."
	advisoryTerminated    = "$JS.EVENT.ADVISORY.CONSUMER.MSG_TERMINATED."
	headerDLQError        = "Callfs-Error"
	dlqStepTimeout        = 15 * time.Second
)

type advisory struct {
	StreamSeq uint64 `json:"stream_seq"`
}

func StartDLQ(ctx context.Context, js jetstream.JetStream, nc *nats.Conn, logger *zap.Logger) (func(), error) {
	if nil == logger {
		logger = zap.NewNop()
	}
	stream, err := js.Stream(ctx, cluster.StreamTasks)
	if nil != err {
		return nil, fmt.Errorf("dlq stream lookup: %w", err)
	}
	handle := func(reason string) nats.MsgHandler {
		return func(m *nats.Msg) {
			var adv advisory
			if err := json.Unmarshal(m.Data, &adv); nil != err {
				logger.Warn("dlq advisory decode failed", zap.Error(err))
				return
			}
			if 0 == adv.StreamSeq {
				return
			}
			if err := moveToDLQ(ctx, js, stream, adv.StreamSeq, reason); nil != err {
				logger.Error("dlq move failed", zap.Uint64("stream_seq", adv.StreamSeq), zap.Error(err))
			}
		}
	}
	subs := make([]*nats.Subscription, 0, 2)
	stop := func() {
		for _, sub := range subs {
			if err := sub.Unsubscribe(); nil != err {
				logger.Debug("dlq unsubscribe failed", zap.Error(err))
			}
		}
	}
	for _, spec := range []struct{ subject, reason string }{
		{advisoryMaxDeliveries + cluster.StreamTasks + ".*", "max deliveries exceeded"},
		{advisoryTerminated + cluster.StreamTasks + ".*", "terminated by handler"},
	} {
		sub, err := nc.Subscribe(spec.subject, handle(spec.reason))
		if nil != err {
			stop()
			return nil, fmt.Errorf("dlq subscribe %s: %w", spec.subject, err)
		}
		subs = append(subs, sub)
	}
	return stop, nil
}

func moveToDLQ(ctx context.Context, js jetstream.JetStream, stream jetstream.Stream, seq uint64, reason string) error {
	stepCtx, cancel := context.WithTimeout(ctx, dlqStepTimeout)
	defer cancel()
	raw, err := stream.GetMsg(stepCtx, seq)
	if nil != err {
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			return nil
		}
		return fmt.Errorf("get message %d: %w", seq, err)
	}
	parts := strings.Split(raw.Subject, ".")
	if 3 >= len(parts) {
		return fmt.Errorf("unexpected task subject %q", raw.Subject)
	}
	pool := parts[2]
	out := nats.NewMsg(cluster.DLQSubject(pool))
	out.Data = raw.Data
	for k, vals := range raw.Header {
		for _, v := range vals {
			out.Header.Add(k, v)
		}
	}
	out.Header.Set(headerDLQError, reason)
	out.Header.Set(headerDLQSubject, raw.Subject)
	if _, err := js.PublishMsg(stepCtx, out); nil != err {
		return fmt.Errorf("publish dlq %s: %w", out.Subject, err)
	}
	if err := stream.DeleteMsg(stepCtx, seq); nil != err {
		return fmt.Errorf("delete message %d: %w", seq, err)
	}
	return nil
}

const headerDLQSubject = "Callfs-Task-Subject"
