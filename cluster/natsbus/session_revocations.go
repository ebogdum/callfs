package natsbus

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ebogdum/callfs/v2/cluster"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	sessionRevocationPrefix  = "session.revoked."
	sessionRevocationTimeout = 5 * time.Second
)

type SessionRevocations struct {
	kv      jetstream.KeyValue
	cancel  context.CancelFunc
	watcher jetstream.KeyWatcher
	wg      sync.WaitGroup
	apply   func(id string, expiresAt time.Time)
}

func NewSessionRevocations(ctx context.Context, js jetstream.JetStream, apply func(id string, expiresAt time.Time)) (*SessionRevocations, error) {
	kv, err := js.KeyValue(ctx, cluster.BucketConfig)
	if nil != err {
		return nil, fmt.Errorf("session revocations: open bucket %s: %w", cluster.BucketConfig, err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	watcher, err := kv.Watch(runCtx, sessionRevocationPrefix+">")
	if nil != err {
		cancel()
		return nil, fmt.Errorf("session revocations: watch bucket %s: %w", cluster.BucketConfig, err)
	}
	r := &SessionRevocations{kv: kv, cancel: cancel, watcher: watcher, apply: apply}
	r.wg.Add(1)
	go r.watchLoop(runCtx)
	return r, nil
}

func (r *SessionRevocations) Publish(id string, expiresAt time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), sessionRevocationTimeout)
	defer cancel()
	if _, err := r.kv.Put(ctx, sessionRevocationPrefix+id, []byte(strconv.FormatInt(expiresAt.Unix(), 10))); nil != err {
		return err
	}
	return nil
}

func (r *SessionRevocations) Close() error {
	r.cancel()
	err := r.watcher.Stop()
	r.wg.Wait()
	return err
}

func (r *SessionRevocations) watchLoop(ctx context.Context) {
	defer r.wg.Done()
	updates := r.watcher.Updates()
	for {
		select {
		case <-ctx.Done():
			return
		case entry, open := <-updates:
			if !open {
				return
			}
			if nil == entry || jetstream.KeyValuePut != entry.Operation() {
				continue
			}
			seconds, parseErr := strconv.ParseInt(string(entry.Value()), 10, 64)
			if nil != parseErr {
				continue
			}
			r.apply(entry.Key()[len(sessionRevocationPrefix):], time.Unix(seconds, 0).UTC())
		}
	}
}
