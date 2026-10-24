package natsbytes

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/backends"
	"github.com/ebogdum/callfs/v2/cluster"
	"github.com/ebogdum/callfs/v2/metadata"
)

const (
	defaultControlTimeout     = 30 * time.Second
	defaultControlConcurrency = 64
	defaultStreamConcurrency  = 32
	subscriptionPendingMsgs   = 65536
	subscriptionPendingBytes  = 256 << 20
	flushTimeout              = 5 * time.Second
)

type Hooks struct {
	Move   func(ctx context.Context, req MoveRequest) error
	Delete func(ctx context.Context, path string, recursive bool) error
}

type ServerOptions struct {
	Hooks
	ChunkSize          int
	Window             int
	IdleTimeout        time.Duration
	ControlTimeout     time.Duration
	ControlConcurrency int
	StreamConcurrency  int
}

func (o ServerOptions) withDefaults() ServerOptions {
	if 1 > o.ChunkSize {
		o.ChunkSize = DefaultChunkSize
	}
	o.Window = clampWindow(o.Window, maxWindow)
	if 0 >= o.IdleTimeout {
		o.IdleTimeout = DefaultIdleTimeout
	}
	if 0 >= o.ControlTimeout {
		o.ControlTimeout = defaultControlTimeout
	}
	if 1 > o.ControlConcurrency {
		o.ControlConcurrency = defaultControlConcurrency
	}
	if 1 > o.StreamConcurrency {
		o.StreamConcurrency = defaultStreamConcurrency
	}
	return o
}

type flowSink interface {
	deliver(m *nats.Msg)
}

type pager interface {
	ListDirectoryPage(ctx context.Context, path string, offset, limit int) ([]*metadata.Metadata, error)
}

type Server struct {
	nc         *nats.Conn
	self       string
	prefix     string
	storage    backends.Storage
	opts       ServerOptions
	logger     *zap.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	sub        *nats.Subscription
	controlSem chan struct{}
	streamSem  chan struct{}
	wg         sync.WaitGroup

	mu     sync.Mutex
	closed bool
	sinks  map[string]flowSink
}

func NewServer(nc *nats.Conn, self string, storage backends.Storage, opts ServerOptions, logger *zap.Logger) (*Server, error) {
	if nil == nc {
		return nil, errors.New("natsbytes: nats connection is required")
	}
	if "" == self {
		return nil, errors.New("natsbytes: instance id is required")
	}
	if nil == storage {
		return nil, errors.New("natsbytes: local storage is required")
	}
	if nil == logger {
		logger = zap.NewNop()
	}
	opts = opts.withDefaults()
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		nc:         nc,
		self:       self,
		prefix:     NodePrefix(self),
		storage:    storage,
		opts:       opts,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		controlSem: make(chan struct{}, opts.ControlConcurrency),
		streamSem:  make(chan struct{}, opts.StreamConcurrency),
		sinks:      make(map[string]flowSink),
	}
	sub, err := nc.Subscribe(ServeSubject(self), s.handle)
	if nil != err {
		cancel()
		return nil, fmt.Errorf("natsbytes: subscribe %s: %w", ServeSubject(self), err)
	}
	if err := sub.SetPendingLimits(subscriptionPendingMsgs, subscriptionPendingBytes); nil != err {
		_ = sub.Unsubscribe()
		cancel()
		return nil, fmt.Errorf("natsbytes: set pending limits: %w", err)
	}
	if err := nc.FlushTimeout(flushTimeout); nil != err {
		_ = sub.Unsubscribe()
		cancel()
		return nil, fmt.Errorf("natsbytes: flush subscription: %w", err)
	}
	s.sub = sub
	return s, nil
}

func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	err := s.sub.Unsubscribe()
	s.cancel()
	s.wg.Wait()
	if nil != err && !errors.Is(err, nats.ErrConnectionClosed) && !errors.Is(err, nats.ErrBadSubscription) {
		return fmt.Errorf("natsbytes: unsubscribe: %w", err)
	}
	return nil
}

func (s *Server) addSink(sink flowSink) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); nil != err {
		return "", fmt.Errorf("generate stream id: %w", err)
	}
	id := hex.EncodeToString(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", cluster.ErrClosed
	}
	s.sinks[id] = sink
	return id, nil
}

func (s *Server) removeSink(id string) {
	s.mu.Lock()
	delete(s.sinks, id)
	s.mu.Unlock()
}

func (s *Server) flowSubject(id string) string {
	return s.prefix + flowToken + "." + id
}

func (s *Server) handle(m *nats.Msg) {
	suffix := strings.TrimPrefix(m.Subject, s.prefix)
	op, rest, _ := strings.Cut(suffix, ".")
	if flowToken == op {
		s.mu.Lock()
		sink := s.sinks[rest]
		s.mu.Unlock()
		if nil != sink {
			sink.deliver(m)
		}
		return
	}
	onBehalf := m.Header.Get(cluster.HeaderOnBehalfOf)
	if "" != onBehalf && !cluster.ValidOnBehalfOf(onBehalf) {
		s.replyError(m, cluster.ErrInvalidOnBehalf)
		return
	}
	switch op {
	case OpStat, OpList, OpDelete, OpDeleteTree, OpMkdir, OpMove:
		s.dispatch(s.controlSem, m, func() { s.serveControl(op, onBehalf, m) })
	case OpOpen:
		s.dispatch(s.streamSem, m, func() { s.serveOpen(onBehalf, m) })
	case OpCreate, OpUpdate:
		s.dispatch(s.streamSem, m, func() { s.serveWrite(op, onBehalf, m) })
	default:
		s.replyError(m, BadRequest("unknown operation "+op))
	}
}

func (s *Server) dispatch(sem chan struct{}, m *nats.Msg, run func()) {
	select {
	case sem <- struct{}{}:
	default:
		s.replyError(m, cluster.ErrBusy)
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		<-sem
		s.replyError(m, cluster.ErrClosed)
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer func() { <-sem }()
		run()
	}()
}

func (s *Server) requestContext(m *nats.Msg, onBehalf string) (context.Context, context.CancelFunc) {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	deadline, err := cluster.ParseDeadline(m.Header.Get(cluster.HeaderDeadline))
	if nil != err || deadline.IsZero() {
		ctx, cancel = context.WithTimeout(s.ctx, s.opts.ControlTimeout)
	} else {
		ctx, cancel = context.WithDeadline(s.ctx, deadline)
	}
	if "" != onBehalf {
		ctx = WithOnBehalfOf(ctx, onBehalf)
	}
	return ctx, cancel
}

func (s *Server) serveControl(op, onBehalf string, m *nats.Msg) {
	ctx, cancel := s.requestContext(m, onBehalf)
	defer cancel()
	result, err := s.runControl(ctx, op, m.Data)
	s.replyControl(m, result, err)
	s.logOutcome(op, m.Header.Get(cluster.HeaderFrom), onBehalf, err)
}

func (s *Server) logOutcome(op, from, onBehalf string, err error) {
	if nil == err {
		s.logger.Debug("node fs request served", zap.String("op", op), zap.String("from", from), zap.String("on_behalf_of", onBehalf))
		return
	}
	if http.StatusInternalServerError <= toWire(err).Status {
		s.logger.Warn("node fs request failed", zap.String("op", op), zap.String("from", from), zap.String("on_behalf_of", onBehalf), zap.Error(err))
		return
	}
	s.logger.Debug("node fs request refused", zap.String("op", op), zap.String("from", from), zap.String("on_behalf_of", onBehalf), zap.Error(err))
}

func (s *Server) runControl(ctx context.Context, op string, raw []byte) (any, error) {
	switch op {
	case OpStat:
		var req PathRequest
		if err := decodeRequest(raw, &req); nil != err {
			return nil, err
		}
		rel := relative(req.Path)
		md, err := s.storage.Stat(ctx, rel)
		if nil != err {
			return nil, err
		}
		return toFileInfo(md, absolute(rel)), nil
	case OpList:
		var req ListRequest
		if err := decodeRequest(raw, &req); nil != err {
			return nil, err
		}
		return s.list(ctx, req)
	case OpDelete, OpDeleteTree:
		var req DeleteRequest
		if err := decodeRequest(raw, &req); nil != err {
			return nil, err
		}
		return nil, s.remove(ctx, req.Path, OpDeleteTree == op || req.Recursive)
	case OpMkdir:
		var req PathRequest
		if err := decodeRequest(raw, &req); nil != err {
			return nil, err
		}
		return nil, s.storage.CreateDirectory(ctx, relative(req.Path))
	case OpMove:
		var req MoveRequest
		if err := decodeRequest(raw, &req); nil != err {
			return nil, err
		}
		return nil, s.move(ctx, req)
	}
	return nil, BadRequest("unknown operation " + op)
}

func decodeRequest(raw []byte, v any) error {
	if err := json.Unmarshal(raw, v); nil != err {
		return BadRequest("invalid request body: " + err.Error())
	}
	return nil
}

func (s *Server) list(ctx context.Context, req ListRequest) (ListReply, error) {
	if 0 > req.Offset {
		return ListReply{}, BadRequest("offset must not be negative")
	}
	limit := req.Limit
	if 1 > limit || maxListPage < limit {
		limit = maxListPage
	}
	rel := relative(req.Path)
	entries, more, err := s.listPage(ctx, rel, req.Offset, limit)
	if nil != err {
		return ListReply{}, err
	}
	for {
		reply := ListReply{Entries: make([]FileInfo, 0, len(entries))}
		for _, entry := range entries {
			reply.Entries = append(reply.Entries, toFileInfo(entry, absolute(path.Join(rel, entry.Name))))
		}
		if more {
			reply.Next = req.Offset + len(entries)
		}
		encoded, err := json.Marshal(reply)
		if nil != err {
			return ListReply{}, fmt.Errorf("encode listing: %w", err)
		}
		if maxReplyBytes >= len(encoded) || 1 >= len(entries) {
			return reply, nil
		}
		entries = entries[:len(entries)/2]
		more = true
	}
}

func (s *Server) listPage(ctx context.Context, rel string, offset, limit int) ([]*metadata.Metadata, bool, error) {
	if paged, ok := s.storage.(pager); ok {
		items, err := paged.ListDirectoryPage(ctx, rel, offset, limit+1)
		if nil != err {
			return nil, false, err
		}
		if limit < len(items) {
			return items[:limit], true, nil
		}
		return items, false, nil
	}
	items, err := s.storage.ListDirectory(ctx, rel)
	if nil != err {
		return nil, false, err
	}
	if len(items) <= offset {
		return nil, false, nil
	}
	items = items[offset:]
	if limit < len(items) {
		return items[:limit], true, nil
	}
	return items, false, nil
}

func (s *Server) remove(ctx context.Context, target string, recursive bool) error {
	if nil != s.opts.Delete {
		return s.opts.Delete(ctx, absolute(target), recursive)
	}
	rel := relative(target)
	if !recursive {
		return s.storage.Delete(ctx, rel)
	}
	if "/" == rel {
		return BadRequest("the root directory cannot be deleted")
	}
	return s.removeTree(ctx, rel)
}

func (s *Server) removeTree(ctx context.Context, rel string) error {
	if err := ctx.Err(); nil != err {
		return err
	}
	md, err := s.storage.Stat(ctx, rel)
	if nil != err {
		return err
	}
	if "directory" == md.Type {
		children, err := s.storage.ListDirectory(ctx, rel)
		if nil != err {
			return err
		}
		for _, child := range children {
			if err := s.removeTree(ctx, path.Join(rel, child.Name)); nil != err {
				return err
			}
		}
	}
	return s.storage.Delete(ctx, rel)
}

func (s *Server) move(ctx context.Context, req MoveRequest) error {
	if "" == strings.Trim(req.Source, "/") || "" == strings.Trim(req.Destination, "/") {
		return BadRequest("source and destination are required")
	}
	if nil != s.opts.Move {
		req.Source = absolute(req.Source)
		req.Destination = absolute(req.Destination)
		return s.opts.Move(ctx, req)
	}
	if "" != req.Backend || "" != req.Instance {
		return BadRequest("relocation to another backend or instance is not supported here")
	}
	src := relative(req.Source)
	dst := relative(req.Destination)
	if src == dst {
		return BadRequest("destination is the same as the source")
	}
	if strings.HasPrefix(dst, src+"/") {
		return BadRequest("cannot move a path into itself")
	}
	if !req.Overwrite {
		_, err := s.storage.Stat(ctx, dst)
		if nil == err {
			return metadata.ErrAlreadyExists
		}
		if !errors.Is(err, metadata.ErrNotFound) {
			return err
		}
	}
	if req.CreateParents {
		if err := s.ensureParents(ctx, dst); nil != err {
			return err
		}
	}
	return s.storage.Move(ctx, src, dst)
}

func (s *Server) ensureParents(ctx context.Context, rel string) error {
	parent := path.Dir(rel)
	if "." == parent || "/" == parent {
		return nil
	}
	built := ""
	for _, segment := range strings.Split(parent, "/") {
		built = path.Join(built, segment)
		err := s.storage.CreateDirectory(ctx, built)
		if nil != err && !errors.Is(err, metadata.ErrAlreadyExists) {
			return err
		}
	}
	return nil
}

func (s *Server) checkPreconditions(ctx context.Context, rel, ifMatch string, ifNoneMatch bool) error {
	if "" == ifMatch && !ifNoneMatch {
		return nil
	}
	_, err := s.storage.Stat(ctx, rel)
	if nil != err && !errors.Is(err, metadata.ErrNotFound) {
		return err
	}
	exists := nil == err
	if ifNoneMatch && exists {
		return metadata.ErrPrecondition
	}
	if "" == ifMatch {
		return nil
	}
	if !exists {
		return metadata.ErrPrecondition
	}
	if "*" == ifMatch {
		return nil
	}
	etag, err := s.localETag(ctx, rel)
	if nil != err {
		return err
	}
	if etag != ifMatch {
		return metadata.ErrPrecondition
	}
	return nil
}

func (s *Server) localETag(ctx context.Context, rel string) (string, error) {
	reader, err := s.storage.Open(ctx, rel)
	if nil != err {
		return "", err
	}
	sum := sha256.New()
	_, copyErr := io.Copy(sum, reader)
	closeErr := reader.Close()
	if nil != copyErr {
		return "", copyErr
	}
	if nil != closeErr {
		return "", closeErr
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func (s *Server) replyControl(m *nats.Msg, result any, err error) {
	if "" == m.Reply {
		return
	}
	if nil != err {
		s.replyError(m, err)
		return
	}
	out := &nats.Msg{Subject: m.Reply, Header: nats.Header{}}
	out.Header.Set(cluster.HeaderFrom, s.self)
	out.Header.Set(cluster.HeaderStatus, cluster.FormatStatus(http.StatusOK))
	if nil != result {
		encoded, encodeErr := json.Marshal(result)
		if nil != encodeErr {
			s.replyError(m, cluster.NewError(http.StatusInternalServerError, "encode", encodeErr.Error()))
			return
		}
		out.Data = encoded
	}
	if pubErr := s.nc.PublishMsg(out); nil != pubErr {
		s.logger.Warn("node fs reply failed", zap.String("subject", m.Subject), zap.Error(pubErr))
	}
}

func (s *Server) replyError(m *nats.Msg, err error) {
	if "" == m.Reply {
		return
	}
	if pubErr := s.nc.PublishMsg(s.errorMsg(m.Reply, frameResult, err)); nil != pubErr {
		s.logger.Warn("node fs error reply failed", zap.String("subject", m.Subject), zap.Error(pubErr))
	}
}

func (s *Server) errorMsg(subject, frame string, err error) *nats.Msg {
	wire := toWire(err)
	status, code, body := cluster.EncodeError(wire)
	out := &nats.Msg{Subject: subject, Data: body, Header: nats.Header{}}
	out.Header.Set(cluster.HeaderFrom, s.self)
	out.Header.Set(HeaderFrame, frame)
	out.Header.Set(cluster.HeaderStatus, cluster.FormatStatus(status))
	out.Header.Set(cluster.HeaderErrorCode, code)
	out.Header.Set(cluster.HeaderError, wire.Message)
	if 0 < wire.RetryAfter {
		out.Header.Set(cluster.HeaderRetryAfter, cluster.FormatRetryAfter(wire.RetryAfter))
	}
	return out
}

func (s *Server) frameMsg(subject, frame string) *nats.Msg {
	out := &nats.Msg{Subject: subject, Header: nats.Header{}}
	out.Header.Set(cluster.HeaderFrom, s.self)
	out.Header.Set(HeaderFrame, frame)
	out.Header.Set(cluster.HeaderStatus, cluster.FormatStatus(http.StatusOK))
	return out
}

func (s *Server) chunkSize() int {
	return chunkLimit(s.nc, s.opts.ChunkSize)
}

func chunkLimit(nc *nats.Conn, wanted int) int {
	payload := int(nc.MaxPayload())
	if 0 >= payload {
		payload = fallbackPayload
	}
	return max(min(wanted, payload-payloadMargin), 1024)
}
