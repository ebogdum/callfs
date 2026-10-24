package cluster

import (
	"context"
	"math/rand/v2"
	"time"
)

const (
	DefaultCallTimeout      = 10 * time.Second
	DefaultCallRetries      = 3
	DefaultServeConcurrency = 64
	callRetryBase           = 50 * time.Millisecond
	callRetryMax            = 2 * time.Second
)

type Bus interface {
	Call(ctx context.Context, node, service, method string, req, resp any, opt ...CallOpt) error
	CallAny(ctx context.Context, service, method string, req, resp any) error
	Gather(ctx context.Context, service, method string, req any, each func(node string, raw []byte, err error)) error
	Broadcast(ctx context.Context, topic string, msg any) error
	Subscribe(topic string, h func(ctx context.Context, from string, raw []byte)) (func(), error)
	Serve(service string, h Handler, opt ...ServeOpt) error
}

type Request struct {
	From           string
	OnBehalfOf     string
	Service        string
	Method         string
	IdempotencyKey string
	Deadline       time.Time
	Body           []byte
}

func (r *Request) Decode(v any) error {
	return Decode(r.Body, v)
}

type Handler func(ctx context.Context, req *Request) (any, error)

type CallOptions struct {
	OnBehalfOf     string
	Timeout        time.Duration
	IdempotencyKey string
	Retries        int
}

type CallOpt func(*CallOptions)

func OnBehalfOf(user string) CallOpt {
	return func(o *CallOptions) {
		o.OnBehalfOf = user
	}
}

func WithTimeout(d time.Duration) CallOpt {
	return func(o *CallOptions) {
		o.Timeout = d
	}
}

func WithIdempotencyKey(key string) CallOpt {
	return func(o *CallOptions) {
		o.IdempotencyKey = key
	}
}

func Idempotent() CallOpt {
	return func(o *CallOptions) {
		o.Retries = DefaultCallRetries
	}
}

func ApplyCallOpts(defaultTimeout time.Duration, opts ...CallOpt) CallOptions {
	out := CallOptions{Timeout: defaultTimeout}
	for _, opt := range opts {
		if nil != opt {
			opt(&out)
		}
	}
	if 0 >= out.Timeout {
		out.Timeout = DefaultCallTimeout
	}
	if 0 > out.Retries {
		out.Retries = 0
	}
	return out
}

func CallDeadline(ctx context.Context, timeout time.Duration) time.Time {
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		return ctxDeadline
	}
	return deadline
}

func CallRetryDelay(attempt int) time.Duration {
	delay := callRetryBase
	for i := 0; i < attempt && delay < callRetryMax; i++ {
		delay *= 2
	}
	delay = min(delay, callRetryMax)
	half := int64(delay / 2)
	return time.Duration(half + rand.Int64N(half+1))
}

type ServeOptions struct {
	Concurrency int
	QueueGroup  bool
}

type ServeOpt func(*ServeOptions)

func WithConcurrency(n int) ServeOpt {
	return func(o *ServeOptions) {
		o.Concurrency = n
	}
}

func WithQueueGroup() ServeOpt {
	return func(o *ServeOptions) {
		o.QueueGroup = true
	}
}

func ApplyServeOpts(opts ...ServeOpt) ServeOptions {
	out := ServeOptions{Concurrency: DefaultServeConcurrency}
	for _, opt := range opts {
		if nil != opt {
			opt(&out)
		}
	}
	if 1 > out.Concurrency {
		out.Concurrency = 1
	}
	return out
}
