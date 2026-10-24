package localbus

import (
	"context"
	"sync"
	"time"

	"github.com/ebogdum/callfs/v2/cluster"
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
	go l.keepLease(ctx, ttl, release, released)
	return held, release, nil
}

func (l *Local) keepLease(ctx context.Context, ttl time.Duration, release func(), released <-chan struct{}) {
	interval := max(ttl/3, time.Millisecond)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	renewed := time.Now()
	for {
		select {
		case <-ctx.Done():
			release()
			return
		case <-l.ctx.Done():
			release()
			return
		case <-released:
			return
		case now := <-ticker.C:
			if ttl < now.Sub(renewed) {
				release()
				return
			}
			renewed = now
		}
	}
}

func (l *Local) Holder(_ context.Context, name string) (string, error) {
	if !cluster.ValidToken(name) {
		return "", cluster.ErrInvalidName
	}
	l.mu.Lock()
	slot := l.leases[name]
	l.mu.Unlock()
	if nil == slot || 0 == len(slot) {
		return "", nil
	}
	return l.self.ID, nil
}
