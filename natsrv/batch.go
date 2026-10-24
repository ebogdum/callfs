package natsrv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nuid"
)

const MaxAtomicBatch = 1000

var ErrBatchTooLarge = errors.New("nats: atomic batch exceeds 1000 messages")

type PubAck struct {
	Stream    string `json:"stream"`
	Sequence  uint64 `json:"seq"`
	Duplicate bool   `json:"duplicate,omitempty"`
	Error     *struct {
		Code        int    `json:"code"`
		ErrCode     int    `json:"err_code"`
		Description string `json:"description"`
	} `json:"error,omitempty"`
}

func PublishAtomic(ctx context.Context, nc *nats.Conn, subject string, payloads [][]byte) (PubAck, error) {
	if 0 == len(payloads) {
		return PubAck{}, errors.New("nats: empty atomic batch")
	}
	if MaxAtomicBatch < len(payloads) {
		return PubAck{}, ErrBatchTooLarge
	}

	id := nuid.Next()
	last := len(payloads) - 1
	for i, p := range payloads {
		msg := nats.NewMsg(subject)
		msg.Data = p
		msg.Header.Set("Nats-Batch-Id", id)
		msg.Header.Set("Nats-Batch-Sequence", strconv.Itoa(i+1))

		if last > i {
			if err := nc.PublishMsg(msg); nil != err {
				return PubAck{}, fmt.Errorf("nats: publish atomic batch message %d: %w", i+1, err)
			}
			continue
		}

		msg.Header.Set("Nats-Batch-Commit", "1")
		resp, err := nc.RequestMsgWithContext(ctx, msg)
		if nil != err {
			return PubAck{}, fmt.Errorf("nats: commit atomic batch: %w", err)
		}

		var ack PubAck
		if err := json.Unmarshal(resp.Data, &ack); nil != err {
			return PubAck{}, fmt.Errorf("nats: decode atomic batch ack: %w", err)
		}
		if nil != ack.Error {
			return ack, fmt.Errorf("nats: atomic batch rejected: %s (%d)", ack.Error.Description, ack.Error.ErrCode)
		}
		return ack, nil
	}

	return PubAck{}, errors.New("nats: atomic batch not committed")
}
