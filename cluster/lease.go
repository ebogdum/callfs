package cluster

import (
	"context"
	"time"
)

const (
	singletonRetryDelay = time.Second
)

type Lease interface {
	Hold(ctx context.Context, name string, ttl time.Duration) (held <-chan struct{}, release func(), err error)
	Holder(ctx context.Context, name string) (string, error)
}

func Singleton(ctx context.Context, lease Lease, name string, ttl time.Duration, run func(ctx context.Context) error, report func(error)) {
	for {
		if nil != ctx.Err() {
			return
		}
		held, release, err := lease.Hold(ctx, name, ttl)
		if nil != err {
			if nil != ctx.Err() {
				return
			}
			report(err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(singletonRetryDelay):
			}
			continue
		}
		runCtx, cancel := context.WithCancel(ctx)
		go func() {
			select {
			case <-held:
			case <-runCtx.Done():
			}
			cancel()
		}()
		runErr := run(runCtx)
		cancel()
		release()
		if nil != ctx.Err() {
			return
		}
		if nil != runErr {
			report(runErr)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(singletonRetryDelay):
		}
	}
}
