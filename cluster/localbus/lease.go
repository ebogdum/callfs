package localbus

import (
	"context"
	"sync"
	"time"

	"github.com/ebogdum/callfs/cluster"
)

func (l *Local) Hold(ctx context.Context, name string, ttl time.Duration) (<-chan struct{}, func(), error) {
	if !cluster.ValidToken(name) {
		return nil, nil, cluster.ErrInvalidName
	}
	if 0 >= ttl {
		return nil, nil, cluster.ErrInvalidTTL
	}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil, nil, cluster.ErrClosed
	}
	slot := l.leases[name]
	if nil == slot {
		slot = make(chan struct{}, 1)
		l.leases[name] = slot
	}
	l.mu.Unlock()
	select {
	case slot <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, cluster.ToError(ctx.Err())
	case <-l.ctx.Done():
		return nil, nil, cluster.ErrClosed
	}
	held := make(chan struct{})
	released := make(chan struct{})
	var once sync.Once
	release := func() {
		once.Do(func() {
			close(held)
			<-slot
			close(released)
		})
	}
	go func() {
		select {
		case <-ctx.Done():
			release()
		case <-l.ctx.Done():
			release()
		case <-released:
		}
	}()
	return held, release, nil
}
