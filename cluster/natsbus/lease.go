package natsbus

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/ebogdum/callfs/v2/cluster"
)

const leaseRetryInterval = time.Second

type Lease struct {
	js   jetstream.JetStream
	self string
}

var _ cluster.Lease = (*Lease)(nil)

func NewLease(js jetstream.JetStream, self string) *Lease {
	return &Lease{js: js, self: self}
}

func (l *Lease) Hold(ctx context.Context, name string, ttl time.Duration) (<-chan struct{}, func(), error) {
	if !cluster.ValidToken(name) {
		return nil, nil, cluster.ErrInvalidName
	}
	if 0 >= ttl {
		return nil, nil, cluster.ErrInvalidTTL
	}
	kv, err := l.js.KeyValue(ctx, cluster.BucketLeases)
	if nil != err {
		return nil, nil, fmt.Errorf("open lease bucket: %w", err)
	}
	key := cluster.LeaseKey(name)
	rev, err := l.acquire(ctx, kv, key, ttl)
	if nil != err {
		return nil, nil, err
	}
	held := make(chan struct{})
	stop := make(chan struct{})
	finished := make(chan struct{})
	go l.keepAlive(ctx, kv, key, ttl, rev, held, stop, finished)
	var once sync.Once
	release := func() {
		once.Do(func() {
			close(stop)
		})
		<-finished
	}
	return held, release, nil
}

func (l *Lease) Holder(ctx context.Context, name string) (string, error) {
	if !cluster.ValidToken(name) {
		return "", cluster.ErrInvalidName
	}
	kv, err := l.js.KeyValue(ctx, cluster.BucketLeases)
	if nil != err {
		return "", fmt.Errorf("open lease bucket: %w", err)
	}
	entry, err := kv.Get(ctx, cluster.LeaseKey(name))
	if nil != err {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("read lease %s: %w", name, err)
	}
	return string(entry.Value()), nil
}

func (l *Lease) acquire(ctx context.Context, kv jetstream.KeyValue, key string, ttl time.Duration) (uint64, error) {
	for {
		rev, err := kv.Create(ctx, key, []byte(l.self), jetstream.KeyTTL(ttl))
		if nil == err {
			return rev, nil
		}
		if !errors.Is(err, jetstream.ErrKeyExists) {
			return 0, fmt.Errorf("acquire lease %s: %w", key, err)
		}
		if err := l.awaitChange(ctx, kv, key); nil != err {
			return 0, err
		}
	}
}

func (l *Lease) awaitChange(ctx context.Context, kv jetstream.KeyValue, key string) error {
	watcher, err := kv.Watch(ctx, key)
	if nil != err {
		return fmt.Errorf("watch lease %s: %w", key, err)
	}
	defer watcher.Stop()
	timer := time.NewTimer(leaseRetryInterval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return cluster.ToError(ctx.Err())
		case <-timer.C:
			return nil
		case entry, open := <-watcher.Updates():
			if !open {
				return fmt.Errorf("watch lease %s: channel closed", key)
			}
			if nil != entry && jetstream.KeyValuePut != entry.Operation() {
				return nil
			}
		}
	}
}

func (l *Lease) keepAlive(ctx context.Context, kv jetstream.KeyValue, key string, ttl time.Duration, rev uint64, held chan<- struct{}, stop <-chan struct{}, finished chan<- struct{}) {
	defer close(finished)
	defer close(held)
	interval := ttl / 3
	if 0 >= interval {
		interval = time.Nanosecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	subject := "$KV." + cluster.BucketLeases + "." + key
	for {
		select {
		case <-stop:
			_ = kv.Delete(context.Background(), key, jetstream.LastRevision(rev))
			return
		case <-ctx.Done():
			_ = kv.Delete(context.Background(), key, jetstream.LastRevision(rev))
			return
		case <-ticker.C:
			ack, err := l.js.Publish(ctx, subject, []byte(l.self), jetstream.WithExpectLastSequencePerSubject(rev), jetstream.WithMsgTTL(ttl))
			if nil != err {
				return
			}
			rev = ack.Sequence
		}
	}
}
