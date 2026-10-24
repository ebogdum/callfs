package natsbytes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/ebogdum/callfs/v2/cluster"
)

const noRespondersStatus = "503"

func (c *Client) Open(ctx context.Context, instanceID, path string) (io.ReadCloser, error) {
	return c.openStream(ctx, instanceID, path, 0, -1)
}

func (c *Client) OpenRange(ctx context.Context, instanceID, path string, offset, length int64) (io.ReadCloser, error) {
	return c.openStream(ctx, instanceID, path, offset, length)
}

func (c *Client) Create(ctx context.Context, instanceID, path string, reader io.Reader, size int64, ifMatch string, ifNoneMatch bool) (string, error) {
	return c.write(ctx, instanceID, OpCreate, path, reader, size, ifMatch, ifNoneMatch)
}

func (c *Client) Update(ctx context.Context, instanceID, path string, reader io.Reader, size int64, ifMatch string, ifNoneMatch bool) (string, error) {
	return c.write(ctx, instanceID, OpUpdate, path, reader, size, ifMatch, ifNoneMatch)
}

func (c *Client) subscribeInbox() (string, chan *nats.Msg, *nats.Subscription, error) {
	inbox := c.nc.NewRespInbox()
	ch := make(chan *nats.Msg, c.window+8)
	sub, err := c.nc.ChanSubscribe(inbox, ch)
	if nil != err {
		return "", nil, nil, transportError(err)
	}
	return inbox, ch, sub, nil
}

func releaseSub(sub *nats.Subscription) error {
	err := sub.Unsubscribe()
	if nil != err && !errors.Is(err, nats.ErrConnectionClosed) && !errors.Is(err, nats.ErrBadSubscription) {
		return err
	}
	return nil
}

func (c *Client) awaitFrame(ctx context.Context, ch chan *nats.Msg, wait time.Duration) (*nats.Msg, error) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case m := <-ch:
		if 0 == len(m.Data) && noRespondersStatus == m.Header.Get("Status") && "" == m.Header.Get(HeaderFrame) {
			return nil, cluster.ErrNoResponders
		}
		return m, nil
	case <-ctx.Done():
		return nil, cluster.ToError(ctx.Err())
	case <-timer.C:
		return nil, cluster.ErrTimeout
	}
}

func (c *Client) publishFrame(subject, frame string, set func(h nats.Header), data []byte) error {
	msg := &nats.Msg{Subject: subject, Data: data, Header: nats.Header{}}
	msg.Header.Set(cluster.HeaderFrom, c.self)
	msg.Header.Set(HeaderFrame, frame)
	if nil != set {
		set(msg.Header)
	}
	if err := c.nc.PublishMsg(msg); nil != err {
		return transportError(err)
	}
	return nil
}

func (c *Client) openStream(ctx context.Context, instanceID, path string, offset, length int64) (io.ReadCloser, error) {
	body, err := json.Marshal(OpenRequest{Path: path, Offset: offset, Length: length, Window: c.window})
	if nil != err {
		return nil, fmt.Errorf("natsbytes: encode open request: %w", err)
	}
	msg, err := c.request(ctx, instanceID, OpOpen, body)
	if nil != err {
		return nil, err
	}
	inbox, ch, sub, err := c.subscribeInbox()
	if nil != err {
		return nil, err
	}
	msg.Reply = inbox
	if err := c.nc.PublishMsg(msg); nil != err {
		_ = releaseSub(sub)
		return nil, transportError(err)
	}
	head, err := c.awaitFrame(ctx, ch, c.timeout)
	if nil != err {
		_ = releaseSub(sub)
		return nil, err
	}
	if err := frameError(head); nil != err {
		_ = releaseSub(sub)
		return nil, err
	}
	flow := head.Header.Get(HeaderFlow)
	if frameHead != head.Header.Get(HeaderFrame) || "" == flow {
		_ = releaseSub(sub)
		return nil, ErrStreamBroken
	}
	r := &streamReader{
		c:    c,
		ctx:  ctx,
		sub:  sub,
		ch:   ch,
		flow: flow,
		hash: sha256.New(),
		done: make(chan struct{}),
	}
	r.mu.Lock()
	r.stop = context.AfterFunc(ctx, func() { _ = r.Close() })
	r.mu.Unlock()
	return r, nil
}

type streamReader struct {
	c    *Client
	ctx  context.Context
	sub  *nats.Subscription
	ch   chan *nats.Msg
	flow string
	hash hash.Hash

	cur   []byte
	seq   uint64
	total int64
	err   error

	finished  atomic.Bool
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
	mu        sync.Mutex
	stop      func() bool
}

func (r *streamReader) Read(p []byte) (int, error) {
	for {
		if nil != r.err {
			return 0, r.err
		}
		select {
		case <-r.done:
			return 0, r.closedError()
		default:
		}
		if 0 < len(r.cur) {
			n := copy(p, r.cur)
			r.cur = r.cur[n:]
			return n, nil
		}
		if 0 == len(p) {
			return 0, nil
		}
		if err := r.next(); nil != err {
			r.err = err
		}
	}
}

func (r *streamReader) closedError() error {
	if err := r.ctx.Err(); nil != err {
		return cluster.ToError(err)
	}
	return io.ErrClosedPipe
}

func (r *streamReader) next() error {
	select {
	case <-r.done:
		return r.closedError()
	default:
	}
	timer := time.NewTimer(r.c.idle)
	defer timer.Stop()
	select {
	case m := <-r.ch:
		return r.accept(m)
	case <-r.done:
		return r.closedError()
	case <-timer.C:
		_ = r.Close()
		return cluster.ErrTimeout
	}
}

func (r *streamReader) accept(m *nats.Msg) error {
	if err := frameError(m); nil != err {
		r.finished.Store(true)
		_ = r.Close()
		return err
	}
	switch m.Header.Get(HeaderFrame) {
	case frameData:
		seq, ok := atoi64(m.Header.Get(HeaderSeq))
		if !ok || uint64(seq) != r.seq {
			_ = r.Close()
			return fmt.Errorf("%w: expected chunk %d", ErrStreamBroken, r.seq)
		}
		r.seq++
		r.hash.Write(m.Data)
		r.total += int64(len(m.Data))
		r.cur = m.Data
		if err := r.c.publishFrame(r.flow, frameCredit, func(h nats.Header) { h.Set(HeaderCredit, "1") }, nil); nil != err {
			_ = r.Close()
			return err
		}
		return nil
	case frameTrailer:
		total, ok := atoi64(m.Header.Get(HeaderBytes))
		if !ok || total != r.total {
			_ = r.Close()
			return fmt.Errorf("%w: received %d bytes, sender reported %d", io.ErrUnexpectedEOF, r.total, total)
		}
		if m.Header.Get(HeaderSHA256) != hex.EncodeToString(r.hash.Sum(nil)) {
			_ = r.Close()
			return ErrChecksum
		}
		r.finished.Store(true)
		_ = r.Close()
		return io.EOF
	}
	_ = r.Close()
	return ErrStreamBroken
}

func (r *streamReader) Close() error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		stop := r.stop
		r.mu.Unlock()
		if nil != stop {
			stop()
		}
		var cancelErr error
		if !r.finished.Load() {
			cancelErr = r.c.publishFrame(r.flow, frameCancel, nil, nil)
		}
		close(r.done)
		r.closeErr = errors.Join(cancelErr, releaseSub(r.sub))
	})
	return r.closeErr
}

func (c *Client) write(ctx context.Context, instanceID, op, path string, reader io.Reader, size int64, ifMatch string, ifNoneMatch bool) (string, error) {
	body, err := json.Marshal(WriteRequest{Path: path, Size: size, IfMatch: ifMatch, IfNoneMatch: ifNoneMatch, Window: c.window})
	if nil != err {
		return "", fmt.Errorf("natsbytes: encode %s request: %w", op, err)
	}
	msg, err := c.request(ctx, instanceID, op, body)
	if nil != err {
		return "", err
	}
	inbox, ch, sub, err := c.subscribeInbox()
	if nil != err {
		return "", err
	}
	defer func() { _ = releaseSub(sub) }()
	msg.Reply = inbox
	if err := c.nc.PublishMsg(msg); nil != err {
		return "", transportError(err)
	}
	accept, err := c.awaitFrame(ctx, ch, c.timeout)
	if nil != err {
		return "", err
	}
	if err := frameError(accept); nil != err {
		return "", err
	}
	flow := accept.Header.Get(HeaderFlow)
	credits, ok := atoi(accept.Header.Get(HeaderCredit))
	if frameAccept != accept.Header.Get(HeaderFrame) || "" == flow || !ok || 1 > credits {
		return "", ErrStreamBroken
	}
	sender := &uploadSender{c: c, ctx: ctx, ch: ch, flow: flow, credits: credits}
	return sender.run(reader, size)
}

type uploadSender struct {
	c       *Client
	ctx     context.Context
	ch      chan *nats.Msg
	flow    string
	credits int
}

func (u *uploadSender) abort(err error) (string, error) {
	if cancelErr := u.c.publishFrame(u.flow, frameCancel, nil, nil); nil != cancelErr {
		return "", errors.Join(err, cancelErr)
	}
	return "", err
}

func (u *uploadSender) apply(m *nats.Msg) (bool, string, error) {
	switch m.Header.Get(HeaderFrame) {
	case frameCredit:
		if n, ok := atoi(m.Header.Get(HeaderCredit)); ok && 0 < n {
			u.credits += n
		}
		return false, "", nil
	case frameResult:
		if err := frameError(m); nil != err {
			return true, "", err
		}
		return true, m.Header.Get(HeaderETag), nil
	}
	return true, "", ErrStreamBroken
}

func (u *uploadSender) drain() (bool, string, error) {
	for {
		select {
		case m := <-u.ch:
			if done, etag, err := u.apply(m); done {
				return true, etag, err
			}
		default:
			return false, "", nil
		}
	}
}

func (u *uploadSender) run(reader io.Reader, size int64) (string, error) {
	buf := make([]byte, u.c.chunk)
	sum := sha256.New()
	var (
		seq   uint64
		total int64
	)
	for {
		n, readErr := io.ReadFull(reader, buf)
		if err := u.ctx.Err(); nil != err {
			return u.abort(cluster.ToError(err))
		}
		if 0 < n {
			if done, _, err := u.drain(); done {
				if nil == err {
					return "", ErrStreamBroken
				}
				return "", err
			}
			for 0 >= u.credits {
				m, err := u.c.awaitFrame(u.ctx, u.ch, u.c.idle)
				if nil != err {
					return u.abort(err)
				}
				if done, _, err := u.apply(m); done {
					if nil == err {
						return "", ErrStreamBroken
					}
					return "", err
				}
			}
			chunkSeq := seq
			if err := u.c.publishFrame(u.flow, frameData, func(h nats.Header) { h.Set(HeaderSeq, strconv.FormatUint(chunkSeq, 10)) }, buf[:n]); nil != err {
				return "", err
			}
			sum.Write(buf[:n])
			total += int64(n)
			seq++
			u.credits--
			if 0 < size && size < total {
				return u.abort(fmt.Errorf("natsbytes: reader produced more than the declared %d bytes", size))
			}
		}
		if nil == readErr {
			continue
		}
		if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
			break
		}
		return u.abort(readErr)
	}
	if 0 < size && size != total {
		return u.abort(fmt.Errorf("natsbytes: reader produced %d of the declared %d bytes: %w", total, size, io.ErrUnexpectedEOF))
	}
	trailerErr := u.c.publishFrame(u.flow, frameTrailer, func(h nats.Header) {
		h.Set(HeaderSHA256, hex.EncodeToString(sum.Sum(nil)))
		h.Set(HeaderBytes, strconv.FormatInt(total, 10))
	}, nil)
	if nil != trailerErr {
		return "", trailerErr
	}
	for {
		m, err := u.c.awaitFrame(u.ctx, u.ch, u.c.idle)
		if nil != err {
			return u.abort(err)
		}
		if done, etag, err := u.apply(m); done {
			return etag, err
		}
	}
}
