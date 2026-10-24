package natsmeta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/nats-io/nuid"

	"github.com/ebogdum/callfs/v2/metadata/fsm"
)

const (
	commitMaxBatch  = 1024
	commitQueueSize = 4096
)

type commitItem struct {
	nonce string
	data  []byte
	errs  chan error
}

func (s *Store) startCommitters() error {
	origin, err := json.Marshal(s.self)
	if nil != err {
		return fmt.Errorf("natsmeta: marshal origin: %w", err)
	}
	s.groupTail = []byte(`],"v":` + strconv.Itoa(fsm.Version) + `,"origin":` + string(origin) + `,"nonce":"`)
	s.commits = make(chan *commitItem, commitQueueSize)
	go s.runCommitter()
	return nil
}

func (s *Store) enqueueCommit(ctx context.Context, nonce string, data []byte) error {
	item := &commitItem{nonce: nonce, data: data, errs: make(chan error, 1)}
	select {
	case s.commits <- item:
	case <-ctx.Done():
		return fmt.Errorf("natsmeta: queue command: %w", ctx.Err())
	case <-s.ctx.Done():
		return ErrClosed
	}
	select {
	case err := <-item.errs:
		return err
	case <-ctx.Done():
		return fmt.Errorf("natsmeta: publish command: %w", ctx.Err())
	case <-s.ctx.Done():
		return ErrClosed
	}
}

func (s *Store) runCommitter() {
	var carry *commitItem
	for {
		first := carry
		if nil == first {
			select {
			case <-s.ctx.Done():
				return
			case first = <-s.commits:
			}
		}
		var batch []*commitItem
		batch, carry = s.collectCommits(first)
		s.publishCommits(batch)
	}
}

func (s *Store) collectCommits(first *commitItem) ([]*commitItem, *commitItem) {
	batch := []*commitItem{first}
	size := len(first.data)
	limit := s.maxMsgSize - envelopeMargin
	for commitMaxBatch > len(batch) {
		select {
		case next := <-s.commits:
			if limit < size+len(next.data)+1 {
				return batch, next
			}
			batch = append(batch, next)
			size += len(next.data) + 1
		default:
			return batch, nil
		}
	}
	return batch, nil
}

func (s *Store) publishCommits(batch []*commitItem) {
	data, nonce := batch[0].data, batch[0].nonce
	if 1 < len(batch) {
		nonce = nuid.Next()
		data = s.encodeGroup(batch, nonce)
	}
	ctx, cancel := context.WithTimeout(s.ctx, s.applyTimeout)
	err := s.publishWithRetry(ctx, data, nonce)
	cancel()
	for _, item := range batch {
		item.errs <- err
	}
}

func (s *Store) encodeGroup(batch []*commitItem, nonce string) []byte {
	size := 32 + len(s.groupTail) + len(nonce)
	for _, item := range batch {
		size += len(item.data) + 1
	}
	var buf bytes.Buffer
	buf.Grow(size)
	buf.WriteString(`{"op":"group","ops":[`)
	for i, item := range batch {
		if 0 < i {
			buf.WriteByte(',')
		}
		buf.Write(item.data)
	}
	buf.Write(s.groupTail)
	buf.WriteString(nonce)
	buf.WriteString(`"}`)
	return buf.Bytes()
}
