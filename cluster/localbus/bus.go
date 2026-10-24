package localbus

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ebogdum/callfs/v2/cluster"
)

type service struct {
	handler cluster.Handler
	sem     chan struct{}
}

type reply struct {
	raw []byte
	at  time.Time
}

type subscription struct {
	ch   chan message
	done chan struct{}
}

type message struct {
	from string
	raw  []byte
}

type outcome struct {
	raw []byte
	err error
}

func (l *Local) Serve(name string, h cluster.Handler, opt ...cluster.ServeOpt) error {
	if !cluster.ValidToken(name) {
		return cluster.ErrInvalidName
	}
	if nil == h {
		return cluster.ErrInvalidHandler
	}
	options := cluster.ApplyServeOpts(opt...)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return cluster.ErrClosed
	}
	if _, exists := l.services[name]; exists {
		return cluster.ErrAlreadyServing
	}
	l.services[name] = &service{handler: h, sem: make(chan struct{}, options.Concurrency)}
	return nil
}

func (l *Local) Call(ctx context.Context, node, name, method string, req, resp any, opt ...cluster.CallOpt) error {
	if "" != node && l.self.ID != node {
		return cluster.ErrNoResponders
	}
	return l.call(ctx, name, method, req, resp, opt...)
}

func (l *Local) CallAny(ctx context.Context, name, method string, req, resp any) error {
	return l.call(ctx, name, method, req, resp)
}

func (l *Local) call(ctx context.Context, name, method string, req, resp any, opt ...cluster.CallOpt) error {
	co := cluster.ApplyCallOpts(cluster.DefaultCallTimeout, opt...)
	raw, err := l.dispatch(ctx, name, method, req, co)
	if nil != err {
		return cluster.ToError(err)
	}
	if decodeErr := cluster.Decode(raw, resp); nil != decodeErr {
		return cluster.NewError(http.StatusInternalServerError, "decode", decodeErr.Error())
	}
	return nil
}

func (l *Local) Gather(ctx context.Context, name, method string, req any, each func(node string, raw []byte, err error)) error {
	if !cluster.ValidToken(name) || !cluster.ValidToken(method) {
		return cluster.ErrInvalidName
	}
	l.mu.Lock()
	closed := l.closed
	_, serving := l.services[name]
	l.mu.Unlock()
	if closed {
		return cluster.ErrClosed
	}
	if !serving {
		return nil
	}
	co := cluster.ApplyCallOpts(cluster.DefaultCallTimeout)
	raw, err := l.dispatch(ctx, name, method, req, co)
	if nil == each {
		return nil
	}
	if nil != err {
		each(l.self.ID, nil, cluster.ToError(err))
		return nil
	}
	each(l.self.ID, raw, nil)
	return nil
}

func (l *Local) dispatch(ctx context.Context, name, method string, req any, co cluster.CallOptions) ([]byte, error) {
	if !cluster.ValidToken(name) || !cluster.ValidToken(method) {
		return nil, cluster.ErrInvalidName
	}
	if "" != co.OnBehalfOf && !cluster.ValidOnBehalfOf(co.OnBehalfOf) {
		return nil, cluster.ErrInvalidOnBehalf
	}
	body, err := cluster.Encode(req)
	if nil != err {
		return nil, cluster.NewError(http.StatusBadRequest, "encode", err.Error())
	}
	deadline := cluster.CallDeadline(ctx, co.Timeout)
	callCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	cacheKey := ""
	if "" != co.IdempotencyKey {
		cacheKey = name + "\x00" + method + "\x00" + co.IdempotencyKey
		if raw, ok := l.cachedReply(cacheKey); ok {
			return raw, nil
		}
	}
	for attempt := 0; ; attempt++ {
		raw, callErr := l.attempt(callCtx, name, method, body, deadline, co)
		if nil == callErr {
			l.storeReply(cacheKey, raw)
			return raw, nil
		}
		if attempt >= co.Retries || !retryable(callErr) {
			return nil, callErr
		}
		select {
		case <-callCtx.Done():
			return nil, cluster.ToError(callCtx.Err())
		case <-time.After(cluster.CallRetryDelay(attempt)):
		}
	}
}

func retryable(err error) bool {
	return http.StatusServiceUnavailable == cluster.StatusOf(err)
}

func (l *Local) attempt(ctx context.Context, name, method string, body []byte, deadline time.Time, co cluster.CallOptions) ([]byte, error) {
	l.mu.Lock()
	closed := l.closed
	svc := l.services[name]
	l.mu.Unlock()
	if closed {
		return nil, cluster.ErrClosed
	}
	if nil == svc {
		return nil, cluster.ErrNoResponders
	}
	select {
	case svc.sem <- struct{}{}:
	default:
		return nil, cluster.ErrBusy
	}
	request := &cluster.Request{
		From:           l.self.ID,
		OnBehalfOf:     co.OnBehalfOf,
		Service:        name,
		Method:         method,
		IdempotencyKey: co.IdempotencyKey,
		Deadline:       deadline,
		Body:           body,
	}
	done := make(chan outcome, 1)
	go func() {
		defer func() { <-svc.sem }()
		done <- runHandler(ctx, svc.handler, request)
	}()
	select {
	case out := <-done:
		return out.raw, out.err
	case <-ctx.Done():
		return nil, cluster.ToError(ctx.Err())
	case <-l.ctx.Done():
		return nil, cluster.ErrClosed
	}
}

func runHandler(ctx context.Context, h cluster.Handler, req *cluster.Request) (out outcome) {
	defer func() {
		if recovered := recover(); nil != recovered {
			out = outcome{err: cluster.NewError(http.StatusInternalServerError, "internal", fmt.Sprint(recovered))}
		}
	}()
	result, err := h(ctx, req)
	if nil != err {
		return outcome{err: cluster.ToError(err)}
	}
	raw, encodeErr := cluster.Encode(result)
	if nil != encodeErr {
		return outcome{err: cluster.NewError(http.StatusInternalServerError, "encode", encodeErr.Error())}
	}
	return outcome{raw: raw}
}

func (l *Local) cachedReply(key string) ([]byte, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.replies[key]
	if !ok {
		return nil, false
	}
	if replyCacheTTL < time.Since(entry.at) {
		delete(l.replies, key)
		return nil, false
	}
	return entry.raw, true
}

func (l *Local) storeReply(key string, raw []byte) {
	if "" == key {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if replyCacheMax <= len(l.replies) {
		for k, entry := range l.replies {
			if replyCacheTTL < time.Since(entry.at) {
				delete(l.replies, k)
			}
		}
		if replyCacheMax <= len(l.replies) {
			for k := range l.replies {
				delete(l.replies, k)
				break
			}
		}
	}
	l.replies[key] = reply{raw: raw, at: time.Now()}
}

func (l *Local) Broadcast(ctx context.Context, topic string, msg any) error {
	if !cluster.ValidTopic(topic) {
		return cluster.ErrInvalidName
	}
	if err := ctx.Err(); nil != err {
		return cluster.ToError(err)
	}
	raw, err := cluster.Encode(msg)
	if nil != err {
		return cluster.NewError(http.StatusBadRequest, "encode", err.Error())
	}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return cluster.ErrClosed
	}
	subs := make([]*subscription, 0, len(l.topics[topic]))
	for _, sub := range l.topics[topic] {
		subs = append(subs, sub)
	}
	l.mu.Unlock()
	for _, sub := range subs {
		select {
		case sub.ch <- message{from: l.self.ID, raw: raw}:
		default:
		}
	}
	return nil
}

func (l *Local) Subscribe(topic string, h func(ctx context.Context, from string, raw []byte)) (func(), error) {
	if !cluster.ValidTopic(topic) {
		return nil, cluster.ErrInvalidName
	}
	if nil == h {
		return nil, cluster.ErrInvalidHandler
	}
	sub := &subscription{ch: make(chan message, subscriptionBuffer), done: make(chan struct{})}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil, cluster.ErrClosed
	}
	l.nextSub++
	id := l.nextSub
	if nil == l.topics[topic] {
		l.topics[topic] = make(map[uint64]*subscription)
	}
	l.topics[topic][id] = sub
	l.mu.Unlock()
	go func() {
		for {
			select {
			case msg := <-sub.ch:
				h(l.ctx, msg.from, msg.raw)
			case <-sub.done:
				return
			case <-l.ctx.Done():
				return
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(sub.done)
			l.mu.Lock()
			delete(l.topics[topic], id)
			if 0 == len(l.topics[topic]) {
				delete(l.topics, topic)
			}
			l.mu.Unlock()
		})
	}, nil
}
