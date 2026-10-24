package natsbus

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ebogdum/callfs/cluster"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

const defaultGatherWindow = 2 * time.Second

type service struct {
	handler cluster.Handler
	sem     chan struct{}
}

type Bus struct {
	nc     *nats.Conn
	self   string
	logger *zap.Logger
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	closed   bool
	services map[string]struct{}
	subs     map[*nats.Subscription]struct{}
}

var _ cluster.Bus = (*Bus)(nil)

func NewBus(nc *nats.Conn, self string, logger *zap.Logger) *Bus {
	if nil == logger {
		logger = zap.NewNop()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Bus{
		nc:       nc,
		self:     self,
		logger:   logger,
		ctx:      ctx,
		cancel:   cancel,
		services: make(map[string]struct{}),
		subs:     make(map[*nats.Subscription]struct{}),
	}
}

func (b *Bus) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	subs := make([]*nats.Subscription, 0, len(b.subs))
	for sub := range b.subs {
		subs = append(subs, sub)
	}
	b.subs = make(map[*nats.Subscription]struct{})
	b.mu.Unlock()
	b.cancel()
	var errs []error
	for _, sub := range subs {
		if err := sub.Unsubscribe(); nil != err && !errors.Is(err, nats.ErrConnectionClosed) && !errors.Is(err, nats.ErrBadSubscription) {
			errs = append(errs, fmt.Errorf("unsubscribe %s: %w", sub.Subject, err))
		}
	}
	return errors.Join(errs...)
}

func (b *Bus) isClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

func (b *Bus) track(sub *nats.Subscription) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return cluster.ErrClosed
	}
	b.subs[sub] = struct{}{}
	return nil
}

func (b *Bus) untrack(sub *nats.Subscription) {
	b.mu.Lock()
	delete(b.subs, sub)
	b.mu.Unlock()
}

func (b *Bus) Call(ctx context.Context, node, svc, method string, req, resp any, opt ...cluster.CallOpt) error {
	if !cluster.ValidToken(svc) || !cluster.ValidToken(method) {
		return cluster.ErrInvalidName
	}
	if "" == node {
		node = b.self
	}
	return b.call(ctx, cluster.NodeRPCSubject(node, svc, method), req, resp, cluster.ApplyCallOpts(cluster.DefaultCallTimeout, opt...))
}

func (b *Bus) CallAny(ctx context.Context, svc, method string, req, resp any) error {
	if !cluster.ValidToken(svc) || !cluster.ValidToken(method) {
		return cluster.ErrInvalidName
	}
	return b.call(ctx, cluster.ServiceSubject(svc, method), req, resp, cluster.ApplyCallOpts(cluster.DefaultCallTimeout))
}

func (b *Bus) call(ctx context.Context, subject string, req, resp any, co cluster.CallOptions) error {
	if b.isClosed() {
		return cluster.ErrClosed
	}
	if "" != co.OnBehalfOf && !cluster.ValidOnBehalfOf(co.OnBehalfOf) {
		return cluster.ErrInvalidOnBehalf
	}
	body, err := cluster.Encode(req)
	if nil != err {
		return cluster.NewError(http.StatusBadRequest, "encode", err.Error())
	}
	deadline := cluster.CallDeadline(ctx, co.Timeout)
	callCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	for attempt := 0; ; attempt++ {
		raw, callErr := b.attempt(callCtx, subject, body, deadline, co)
		if nil == callErr {
			if decodeErr := cluster.Decode(raw, resp); nil != decodeErr {
				return cluster.NewError(http.StatusInternalServerError, "decode", decodeErr.Error())
			}
			return nil
		}
		if attempt >= co.Retries || nil != callCtx.Err() || !retryable(callErr) {
			return callErr
		}
		select {
		case <-callCtx.Done():
			return cluster.ToError(callCtx.Err())
		case <-b.ctx.Done():
			return cluster.ErrClosed
		case <-time.After(cluster.CallRetryDelay(attempt)):
		}
	}
}

func retryable(err error) bool {
	status := cluster.StatusOf(err)
	return http.StatusServiceUnavailable == status || http.StatusGatewayTimeout == status
}

func (b *Bus) attempt(ctx context.Context, subject string, body []byte, deadline time.Time, co cluster.CallOptions) ([]byte, error) {
	msg := &nats.Msg{Subject: subject, Data: body, Header: nats.Header{}}
	msg.Header.Set(cluster.HeaderFrom, b.self)
	if "" != co.OnBehalfOf {
		msg.Header.Set(cluster.HeaderOnBehalfOf, co.OnBehalfOf)
	}
	if formatted := cluster.FormatDeadline(deadline); "" != formatted {
		msg.Header.Set(cluster.HeaderDeadline, formatted)
	}
	if "" != co.IdempotencyKey {
		msg.Header.Set(cluster.HeaderIdempotencyKey, co.IdempotencyKey)
	}
	reply, err := b.nc.RequestMsgWithContext(ctx, msg)
	if nil != err {
		return nil, mapTransportError(err)
	}
	return replyResult(reply)
}

func mapTransportError(err error) error {
	switch {
	case errors.Is(err, nats.ErrNoResponders):
		return cluster.ErrNoResponders
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, nats.ErrTimeout):
		return cluster.ErrTimeout
	case errors.Is(err, context.Canceled):
		return cluster.ErrCanceled
	case errors.Is(err, nats.ErrConnectionClosed):
		return cluster.ErrClosed
	}
	return cluster.NewError(http.StatusServiceUnavailable, "transport", err.Error())
}

func replyResult(reply *nats.Msg) ([]byte, error) {
	status, err := cluster.ParseStatus(reply.Header.Get(cluster.HeaderStatus))
	if nil != err {
		return nil, cluster.NewError(http.StatusBadGateway, "bad_reply", err.Error())
	}
	if 200 > status || 299 < status {
		if decoded := cluster.DecodeError(status, reply.Header.Get(cluster.HeaderErrorCode), reply.Data); nil != decoded {
			return nil, decoded
		}
		return nil, cluster.NewError(status, "unexpected_status", http.StatusText(status))
	}
	return reply.Data, nil
}

func (b *Bus) Gather(ctx context.Context, svc, method string, req any, each func(node string, raw []byte, err error)) error {
	if !cluster.ValidToken(svc) || !cluster.ValidToken(method) {
		return cluster.ErrInvalidName
	}
	if b.isClosed() {
		return cluster.ErrClosed
	}
	body, err := cluster.Encode(req)
	if nil != err {
		return cluster.NewError(http.StatusBadRequest, "encode", err.Error())
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultGatherWindow)
		defer cancel()
	}
	deadline, _ := ctx.Deadline()
	inbox := nats.NewInbox()
	sub, err := b.nc.SubscribeSync(inbox)
	if nil != err {
		return mapTransportError(err)
	}
	defer func() { _ = sub.Unsubscribe() }()
	msg := &nats.Msg{Subject: cluster.GatherSubject(svc, method), Reply: inbox, Data: body, Header: nats.Header{}}
	msg.Header.Set(cluster.HeaderFrom, b.self)
	msg.Header.Set(cluster.HeaderDeadline, cluster.FormatDeadline(deadline))
	if err := b.nc.PublishMsg(msg); nil != err {
		return mapTransportError(err)
	}
	for {
		reply, err := sub.NextMsgWithContext(ctx)
		if nil != err {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, nats.ErrTimeout) {
				return nil
			}
			if errors.Is(err, context.Canceled) {
				return cluster.ErrCanceled
			}
			return mapTransportError(err)
		}
		if nil == each {
			continue
		}
		raw, replyErr := replyResult(reply)
		each(reply.Header.Get(cluster.HeaderFrom), raw, replyErr)
	}
}

func (b *Bus) Broadcast(ctx context.Context, topic string, msg any) error {
	if !cluster.ValidTopic(topic) {
		return cluster.ErrInvalidName
	}
	if err := ctx.Err(); nil != err {
		return cluster.ToError(err)
	}
	if b.isClosed() {
		return cluster.ErrClosed
	}
	raw, err := cluster.Encode(msg)
	if nil != err {
		return cluster.NewError(http.StatusBadRequest, "encode", err.Error())
	}
	out := &nats.Msg{Subject: cluster.BroadcastSubject(topic), Data: raw, Header: nats.Header{}}
	out.Header.Set(cluster.HeaderFrom, b.self)
	if err := b.nc.PublishMsg(out); nil != err {
		return mapTransportError(err)
	}
	return nil
}

func (b *Bus) Subscribe(topic string, h func(ctx context.Context, from string, raw []byte)) (func(), error) {
	if !cluster.ValidTopic(topic) {
		return nil, cluster.ErrInvalidName
	}
	if nil == h {
		return nil, cluster.ErrInvalidHandler
	}
	sub, err := b.nc.Subscribe(cluster.BroadcastSubject(topic), func(m *nats.Msg) {
		h(b.ctx, m.Header.Get(cluster.HeaderFrom), m.Data)
	})
	if nil != err {
		return nil, mapTransportError(err)
	}
	if trackErr := b.track(sub); nil != trackErr {
		_ = sub.Unsubscribe()
		return nil, trackErr
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			b.untrack(sub)
			_ = sub.Unsubscribe()
		})
	}, nil
}

func (b *Bus) Serve(name string, h cluster.Handler, opt ...cluster.ServeOpt) error {
	if !cluster.ValidToken(name) {
		return cluster.ErrInvalidName
	}
	if nil == h {
		return cluster.ErrInvalidHandler
	}
	options := cluster.ApplyServeOpts(opt...)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return cluster.ErrClosed
	}
	if _, exists := b.services[name]; exists {
		b.mu.Unlock()
		return cluster.ErrAlreadyServing
	}
	b.services[name] = struct{}{}
	b.mu.Unlock()

	svc := &service{handler: h, sem: make(chan struct{}, options.Concurrency)}
	dispatch := func(m *nats.Msg) { b.dispatch(name, svc, m) }
	var created []*nats.Subscription
	fail := func(err error) error {
		for _, sub := range created {
			b.untrack(sub)
			_ = sub.Unsubscribe()
		}
		b.mu.Lock()
		delete(b.services, name)
		b.mu.Unlock()
		return err
	}
	add := func(sub *nats.Subscription, err error) error {
		if nil != err {
			return mapTransportError(err)
		}
		created = append(created, sub)
		return b.track(sub)
	}
	if err := add(b.nc.Subscribe(cluster.NodeRPCServiceWildcard(b.self, name), dispatch)); nil != err {
		return fail(err)
	}
	if err := add(b.nc.Subscribe(cluster.GatherWildcard(name), dispatch)); nil != err {
		return fail(err)
	}
	if options.QueueGroup {
		if err := add(b.nc.QueueSubscribe(cluster.ServiceWildcard(name), cluster.ServiceQueueGroup(name), dispatch)); nil != err {
			return fail(err)
		}
	}
	return nil
}

func (b *Bus) dispatch(name string, svc *service, m *nats.Msg) {
	select {
	case svc.sem <- struct{}{}:
	default:
		b.respond(m, nil, cluster.ErrBusy)
		return
	}
	method := m.Subject
	if idx := strings.LastIndexByte(method, '.'); 0 <= idx {
		method = method[idx+1:]
	}
	onBehalf := m.Header.Get(cluster.HeaderOnBehalfOf)
	if "" != onBehalf && !cluster.ValidOnBehalfOf(onBehalf) {
		<-svc.sem
		b.respond(m, nil, cluster.ErrInvalidOnBehalf)
		return
	}
	deadline, parseErr := cluster.ParseDeadline(m.Header.Get(cluster.HeaderDeadline))
	if nil != parseErr {
		deadline = time.Time{}
	}
	request := &cluster.Request{
		From:           m.Header.Get(cluster.HeaderFrom),
		OnBehalfOf:     onBehalf,
		Service:        name,
		Method:         method,
		IdempotencyKey: m.Header.Get(cluster.HeaderIdempotencyKey),
		Deadline:       deadline,
		Body:           m.Data,
	}
	go func() {
		defer func() { <-svc.sem }()
		var (
			ctx    context.Context
			cancel context.CancelFunc
		)
		if deadline.IsZero() {
			ctx, cancel = context.WithTimeout(b.ctx, cluster.DefaultCallTimeout)
		} else {
			ctx, cancel = context.WithDeadline(b.ctx, deadline)
		}
		defer cancel()
		raw, err := b.run(ctx, svc.handler, request)
		b.respond(m, raw, err)
	}()
}

func (b *Bus) run(ctx context.Context, h cluster.Handler, req *cluster.Request) (raw []byte, err error) {
	defer func() {
		if recovered := recover(); nil != recovered {
			raw = nil
			err = cluster.NewError(http.StatusInternalServerError, "internal", fmt.Sprint(recovered))
		}
	}()
	result, handlerErr := h(ctx, req)
	if nil != handlerErr {
		return nil, handlerErr
	}
	encoded, encodeErr := cluster.Encode(result)
	if nil != encodeErr {
		return nil, cluster.NewError(http.StatusInternalServerError, "encode", encodeErr.Error())
	}
	return encoded, nil
}

func (b *Bus) respond(m *nats.Msg, raw []byte, err error) {
	if "" == m.Reply {
		return
	}
	out := &nats.Msg{Subject: m.Reply, Header: nats.Header{}}
	out.Header.Set(cluster.HeaderFrom, b.self)
	if nil == err {
		out.Header.Set(cluster.HeaderStatus, cluster.FormatStatus(http.StatusOK))
		out.Data = raw
	} else {
		status, code, body := cluster.EncodeError(err)
		out.Header.Set(cluster.HeaderStatus, cluster.FormatStatus(status))
		out.Header.Set(cluster.HeaderErrorCode, code)
		out.Header.Set(cluster.HeaderError, cluster.ToError(err).Message)
		if retryAfter := cluster.ToError(err).RetryAfter; 0 < retryAfter {
			out.Header.Set(cluster.HeaderRetryAfter, cluster.FormatRetryAfter(retryAfter))
		}
		out.Data = body
	}
	if pubErr := m.RespondMsg(out); nil != pubErr {
		b.logger.Warn("cluster reply failed", zap.String("subject", m.Subject), zap.Error(pubErr))
	}
}
