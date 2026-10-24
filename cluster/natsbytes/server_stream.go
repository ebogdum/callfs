package natsbytes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/cluster"
)

type sendSink struct {
	credits chan int
	cancel  context.CancelFunc
}

func (k *sendSink) deliver(m *nats.Msg) {
	switch m.Header.Get(HeaderFrame) {
	case frameCredit:
		if n, ok := atoi(m.Header.Get(HeaderCredit)); ok && 0 < n {
			select {
			case k.credits <- n:
			default:
			}
		}
	case frameCancel:
		k.cancel()
	}
}

func (s *Server) streamContext(m *nats.Msg, onBehalf string) (context.Context, context.CancelFunc, func()) {
	ctx, cancel := context.WithCancel(s.ctx)
	if "" != onBehalf {
		ctx = WithOnBehalfOf(ctx, onBehalf)
	}
	deadline, err := cluster.ParseDeadline(m.Header.Get(cluster.HeaderDeadline))
	if nil != err || deadline.IsZero() {
		return ctx, cancel, func() {}
	}
	timer := time.AfterFunc(time.Until(deadline), cancel)
	return ctx, cancel, func() { timer.Stop() }
}

func (s *Server) serveOpen(onBehalf string, m *nats.Msg) {
	var req OpenRequest
	if err := decodeRequest(m.Data, &req); nil != err {
		s.replyError(m, err)
		return
	}
	if "" == m.Reply {
		return
	}
	if 0 > req.Offset {
		s.replyError(m, BadRequest("offset must not be negative"))
		return
	}
	ctx, cancel, endSetup := s.streamContext(m, onBehalf)
	defer cancel()
	defer endSetup()
	rel := relative(req.Path)
	md, err := s.storage.Stat(ctx, rel)
	if nil != err {
		s.replyError(m, err)
		return
	}
	if "directory" == md.Type {
		s.replyError(m, BadRequest("path is a directory"))
		return
	}
	remaining := max(md.Size-req.Offset, 0)
	if 0 <= req.Length {
		remaining = min(remaining, req.Length)
	}
	var reader io.ReadCloser
	switch {
	case 0 == req.Length:
		reader = io.NopCloser(strings.NewReader(""))
	case 0 == req.Offset && 0 > req.Length:
		reader, err = s.storage.Open(ctx, rel)
	default:
		reader, err = s.storage.OpenRange(ctx, rel, req.Offset, req.Length)
	}
	if nil != err {
		s.replyError(m, err)
		return
	}
	sink := &sendSink{credits: make(chan int, maxWindow*4), cancel: cancel}
	id, err := s.addSink(sink)
	if nil != err {
		_ = reader.Close()
		s.replyError(m, err)
		return
	}
	defer s.removeSink(id)
	head := s.frameMsg(m.Reply, frameHead)
	head.Header.Set(HeaderSize, strconv.FormatInt(remaining, 10))
	head.Header.Set(HeaderFlow, s.flowSubject(id))
	if err := s.nc.PublishMsg(head); nil != err {
		_ = reader.Close()
		s.logger.Warn("node fs stream head failed", zap.String("path", rel), zap.Error(err))
		return
	}
	endSetup()
	sent, pumpErr := s.pump(ctx, m.Reply, reader, sink, clampWindow(req.Window, s.opts.Window))
	closeErr := reader.Close()
	if nil == pumpErr && nil != closeErr {
		pumpErr = closeErr
	}
	s.logStream(OpOpen, rel, m.Header.Get(cluster.HeaderFrom), onBehalf, sent, pumpErr)
}

func (s *Server) logStream(op, rel, from, onBehalf string, bytes int64, err error) {
	if nil == err {
		s.logger.Debug("node fs stream served", zap.String("op", op), zap.String("path", rel), zap.String("from", from), zap.String("on_behalf_of", onBehalf), zap.Int64("bytes", bytes))
		return
	}
	s.logger.Warn("node fs stream ended early", zap.String("op", op), zap.String("path", rel), zap.String("from", from), zap.String("on_behalf_of", onBehalf), zap.Int64("bytes", bytes), zap.Error(err))
}

func (s *Server) pump(ctx context.Context, subject string, reader io.Reader, sink *sendSink, window int) (int64, error) {
	buf := make([]byte, s.chunkSize())
	sum := sha256.New()
	credits := window
	var (
		seq   uint64
		total int64
	)
	fail := func(err error) (int64, error) {
		if pubErr := s.nc.PublishMsg(s.errorMsg(subject, frameResult, err)); nil != pubErr {
			return total, errors.Join(err, pubErr)
		}
		return total, err
	}
	for {
	drain:
		for {
			select {
			case n := <-sink.credits:
				credits += n
			default:
				break drain
			}
		}
		if err := ctx.Err(); nil != err {
			return fail(err)
		}
		for 0 >= credits {
			timer := time.NewTimer(s.opts.IdleTimeout)
			select {
			case n := <-sink.credits:
				credits += n
				timer.Stop()
			case <-ctx.Done():
				timer.Stop()
				return fail(ctx.Err())
			case <-timer.C:
				return fail(cluster.NewError(http.StatusGatewayTimeout, "stalled", "receiver stopped granting credit"))
			}
		}
		n, readErr := io.ReadFull(reader, buf)
		if 0 < n {
			frame := s.frameMsg(subject, frameData)
			frame.Header.Set(HeaderSeq, strconv.FormatUint(seq, 10))
			frame.Data = buf[:n]
			if err := s.nc.PublishMsg(frame); nil != err {
				return total, err
			}
			sum.Write(buf[:n])
			total += int64(n)
			seq++
			credits--
		}
		if nil == readErr {
			continue
		}
		if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
			trailer := s.frameMsg(subject, frameTrailer)
			trailer.Header.Set(HeaderSHA256, hex.EncodeToString(sum.Sum(nil)))
			trailer.Header.Set(HeaderBytes, strconv.FormatInt(total, 10))
			if err := s.nc.PublishMsg(trailer); nil != err {
				return total, err
			}
			return total, nil
		}
		return fail(readErr)
	}
}

type uploadFrame struct {
	kind  string
	seq   uint64
	data  []byte
	sha   string
	bytes int64
}

type uploadSink struct {
	in     chan uploadFrame
	cancel context.CancelFunc
	broken atomic.Bool
}

func (u *uploadSink) deliver(m *nats.Msg) {
	frame := uploadFrame{kind: m.Header.Get(HeaderFrame)}
	switch frame.kind {
	case frameData:
		seq, ok := atoi64(m.Header.Get(HeaderSeq))
		if !ok || 0 > seq {
			u.broken.Store(true)
			u.cancel()
			return
		}
		frame.seq = uint64(seq)
		frame.data = m.Data
	case frameTrailer:
		total, ok := atoi64(m.Header.Get(HeaderBytes))
		if !ok {
			u.broken.Store(true)
			u.cancel()
			return
		}
		frame.sha = m.Header.Get(HeaderSHA256)
		frame.bytes = total
	case frameCancel:
	default:
		return
	}
	select {
	case u.in <- frame:
	default:
		u.broken.Store(true)
		u.cancel()
	}
}

type uploadReader struct {
	s        *Server
	ctx      context.Context
	sink     *uploadSink
	creditTo string
	size     int64
	hash     hash.Hash
	cur      []byte
	consumed int64
	seq      uint64
	done     bool
	err      error
}

func (r *uploadReader) Read(p []byte) (int, error) {
	for {
		if 0 < len(r.cur) {
			n := copy(p, r.cur)
			r.cur = r.cur[n:]
			return n, nil
		}
		if nil != r.err {
			return 0, r.err
		}
		if r.done {
			return 0, io.EOF
		}
		if 0 == len(p) {
			return 0, nil
		}
		if err := r.pull(); nil != err {
			r.cur = nil
			r.err = err
		}
	}
}

func (r *uploadReader) wait() (uploadFrame, error) {
	timer := time.NewTimer(r.s.opts.IdleTimeout)
	defer timer.Stop()
	select {
	case frame := <-r.sink.in:
		return frame, nil
	case <-r.ctx.Done():
		if r.sink.broken.Load() {
			return uploadFrame{}, ErrStreamBroken
		}
		return uploadFrame{}, cluster.ToError(r.ctx.Err())
	case <-timer.C:
		return uploadFrame{}, cluster.NewError(http.StatusGatewayTimeout, "stalled", "sender stopped sending data")
	}
}

func (r *uploadReader) pull() error {
	frame, err := r.wait()
	if nil != err {
		return err
	}
	switch frame.kind {
	case frameData:
		if r.seq != frame.seq {
			return fmt.Errorf("%w: expected chunk %d, got %d", ErrStreamBroken, r.seq, frame.seq)
		}
		r.seq++
		r.hash.Write(frame.data)
		r.consumed += int64(len(frame.data))
		if 0 < r.size && r.size < r.consumed {
			return fmt.Errorf("%w: received more than the declared %d bytes", ErrStreamBroken, r.size)
		}
		credit := r.s.frameMsg(r.creditTo, frameCredit)
		credit.Header.Set(HeaderCredit, "1")
		if err := r.s.nc.PublishMsg(credit); nil != err {
			return err
		}
		r.cur = frame.data
		if 0 < r.size && r.size == r.consumed {
			trailer, err := r.wait()
			if nil != err {
				return err
			}
			if frameTrailer != trailer.kind {
				return fmt.Errorf("%w: expected trailer after the final chunk", ErrStreamBroken)
			}
			return r.verify(trailer)
		}
		return nil
	case frameTrailer:
		return r.verify(frame)
	case frameCancel:
		return cluster.ErrCanceled
	}
	return ErrStreamBroken
}

func (r *uploadReader) verify(trailer uploadFrame) error {
	if trailer.bytes != r.consumed {
		return fmt.Errorf("%w: sender reported %d bytes, received %d", ErrStreamBroken, trailer.bytes, r.consumed)
	}
	if trailer.sha != r.sum() {
		return ErrChecksum
	}
	r.done = true
	return nil
}

func (r *uploadReader) sum() string {
	return hex.EncodeToString(r.hash.Sum(nil))
}

func (r *uploadReader) finish() error {
	for !r.done && nil == r.err {
		if err := r.pull(); nil != err {
			r.err = err
		}
		r.cur = nil
	}
	return r.err
}

func (s *Server) serveWrite(op, onBehalf string, m *nats.Msg) {
	var req WriteRequest
	if err := decodeRequest(m.Data, &req); nil != err {
		s.replyError(m, err)
		return
	}
	if "" == m.Reply {
		return
	}
	ctx, cancel, endSetup := s.streamContext(m, onBehalf)
	defer cancel()
	defer endSetup()
	rel := relative(req.Path)
	if "/" == rel {
		s.replyError(m, BadRequest("path must name a file"))
		return
	}
	if err := s.checkPreconditions(ctx, rel, req.IfMatch, req.IfNoneMatch); nil != err {
		s.replyError(m, err)
		return
	}
	window := clampWindow(req.Window, s.opts.Window)
	sink := &uploadSink{in: make(chan uploadFrame, window+4), cancel: cancel}
	id, err := s.addSink(sink)
	if nil != err {
		s.replyError(m, err)
		return
	}
	defer s.removeSink(id)
	accept := s.frameMsg(m.Reply, frameAccept)
	accept.Header.Set(HeaderFlow, s.flowSubject(id))
	accept.Header.Set(HeaderCredit, strconv.Itoa(window))
	if err := s.nc.PublishMsg(accept); nil != err {
		s.logger.Warn("node fs upload accept failed", zap.String("path", rel), zap.Error(err))
		return
	}
	endSetup()
	reader := &uploadReader{s: s, ctx: ctx, sink: sink, creditTo: m.Reply, size: req.Size, hash: sha256.New()}
	if OpCreate == op {
		err = s.storage.Create(ctx, rel, reader, req.Size)
	} else {
		err = s.storage.Update(ctx, rel, reader, req.Size)
	}
	if nil == err {
		err = reader.finish()
	} else if nil != reader.err {
		err = reader.err
	}
	s.logStream(op, rel, m.Header.Get(cluster.HeaderFrom), onBehalf, reader.consumed, err)
	if nil != err {
		if pubErr := s.nc.PublishMsg(s.errorMsg(m.Reply, frameResult, err)); nil != pubErr {
			s.logger.Warn("node fs upload error reply failed", zap.String("path", rel), zap.Error(pubErr))
		}
		return
	}
	result := s.frameMsg(m.Reply, frameResult)
	result.Header.Set(HeaderETag, reader.sum())
	result.Header.Set(HeaderBytes, strconv.FormatInt(reader.consumed, 10))
	if pubErr := s.nc.PublishMsg(result); nil != pubErr {
		s.logger.Warn("node fs upload result failed", zap.String("path", rel), zap.Error(pubErr))
	}
}
