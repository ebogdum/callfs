package natsbus

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ebogdum/callfs/v2/cluster"
	"github.com/nats-io/nats.go/jetstream"
)

const presenceKeyPrefix = "n."

var _ cluster.Presence = (*Presence)(nil)

type Presence struct {
	kv       jetstream.KeyValue
	key      string
	ttl      time.Duration
	interval time.Duration
	cancel   context.CancelFunc
	watcher  jetstream.KeyWatcher
	wg       sync.WaitGroup

	mu        sync.RWMutex
	self      cluster.Member
	members   map[string]cluster.Member
	watchers  map[uint64]func(cluster.Event)
	nextWatch uint64
	loopErr   error
	closed    bool
}

func NewPresence(ctx context.Context, js jetstream.JetStream, self cluster.Member, ttl time.Duration) (*Presence, error) {
	if "" == self.ID {
		return nil, errors.New("presence: self id is empty")
	}
	interval := ttl / 3
	if 0 >= interval {
		return nil, fmt.Errorf("presence: ttl %s is too small", ttl)
	}
	kv, err := js.KeyValue(ctx, cluster.BucketNodes)
	if nil != err {
		return nil, fmt.Errorf("presence: open bucket %s: %w", cluster.BucketNodes, err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	watcher, err := kv.WatchAll(runCtx)
	if nil != err {
		cancel()
		return nil, fmt.Errorf("presence: watch bucket %s: %w", cluster.BucketNodes, err)
	}
	p := &Presence{
		kv:       kv,
		key:      cluster.PresenceKey(self.ID),
		ttl:      ttl,
		interval: interval,
		cancel:   cancel,
		watcher:  watcher,
		self:     self,
		members:  make(map[string]cluster.Member),
		watchers: make(map[uint64]func(cluster.Event)),
	}
	if beatErr := p.beat(runCtx); nil != beatErr {
		return nil, errors.Join(beatErr, p.halt())
	}
	p.wg.Add(3)
	go p.heartbeatLoop(runCtx)
	go p.watchLoop(runCtx, watcher.Updates())
	go p.expiryLoop(runCtx)
	return p, nil
}

func (p *Presence) Self() cluster.Member {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.self
}

func (p *Presence) Members() []cluster.Member {
	p.mu.RLock()
	list := make([]cluster.Member, 0, len(p.members)+1)
	list = append(list, p.self)
	for _, member := range p.members {
		list = append(list, member)
	}
	p.mu.RUnlock()
	slices.SortFunc(list, func(a, b cluster.Member) int {
		return strings.Compare(a.ID, b.ID)
	})
	return list
}

func (p *Presence) Alive(id string) bool {
	if id == p.self.ID {
		return true
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.members[id]
	return ok
}

func (p *Presence) Watch(fn func(cluster.Event)) func() {
	if nil == fn {
		return func() {}
	}
	p.mu.Lock()
	p.nextWatch++
	id := p.nextWatch
	p.watchers[id] = fn
	p.mu.Unlock()
	return func() {
		p.mu.Lock()
		delete(p.watchers, id)
		p.mu.Unlock()
	}
}

func (p *Presence) Close(ctx context.Context) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	var errs []error
	p.cancel()
	if stopErr := p.watcher.Stop(); nil != stopErr {
		errs = append(errs, fmt.Errorf("presence: stop watch: %w", stopErr))
	}
	p.wg.Wait()
	if deleteErr := p.kv.Delete(ctx, p.key); nil != deleteErr {
		errs = append(errs, fmt.Errorf("presence: delete %s: %w", p.key, deleteErr))
	}
	p.mu.RLock()
	loopErr := p.loopErr
	p.mu.RUnlock()
	if nil != loopErr {
		errs = append(errs, loopErr)
	}
	return errors.Join(errs...)
}

func (p *Presence) halt() error {
	p.cancel()
	stopErr := p.watcher.Stop()
	if nil == stopErr {
		return nil
	}
	return fmt.Errorf("presence: stop watch: %w", stopErr)
}

func (p *Presence) beat(ctx context.Context) error {
	p.mu.Lock()
	p.self.Seen = time.Now().UTC()
	member := p.self
	p.mu.Unlock()
	raw, err := cluster.Encode(member)
	if nil != err {
		return fmt.Errorf("presence: encode self: %w", err)
	}
	if _, err = p.kv.Put(ctx, p.key, raw); nil != err {
		return fmt.Errorf("presence: put %s: %w", p.key, err)
	}
	return nil
}

func (p *Presence) heartbeatLoop(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if beatErr := p.beat(ctx); nil != beatErr && nil == ctx.Err() {
				p.recordErr(beatErr)
			}
		}
	}
}

func (p *Presence) watchLoop(ctx context.Context, updates <-chan jetstream.KeyValueEntry) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case entry, open := <-updates:
			if !open {
				if nil == ctx.Err() {
					p.recordErr(errors.New("presence: watch closed unexpectedly"))
				}
				return
			}
			if nil == entry {
				continue
			}
			p.apply(entry)
		}
	}
}

func (p *Presence) expiryLoop(ctx context.Context) {
	defer p.wg.Done()
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.expire()
		}
	}
}

func (p *Presence) apply(entry jetstream.KeyValueEntry) {
	switch entry.Operation() {
	case jetstream.KeyValueDelete, jetstream.KeyValuePurge:
		id, ok := p.memberIDFromKey(entry.Key())
		if !ok {
			p.recordErr(fmt.Errorf("presence: invalid key %q", entry.Key()))
			return
		}
		p.remove(id)
	default:
		var member cluster.Member
		if decodeErr := cluster.Decode(entry.Value(), &member); nil != decodeErr {
			p.recordErr(fmt.Errorf("presence: decode %q: %w", entry.Key(), decodeErr))
			return
		}
		p.upsert(member)
	}
}

func (p *Presence) memberIDFromKey(key string) (string, bool) {
	token, found := strings.CutPrefix(key, presenceKeyPrefix)
	if !found {
		return "", false
	}
	return cluster.ParseToken(token)
}

func (p *Presence) upsert(member cluster.Member) {
	if member.ID == p.self.ID {
		return
	}
	if p.stale(member) {
		p.remove(member.ID)
		return
	}
	p.mu.Lock()
	_, existed := p.members[member.ID]
	p.members[member.ID] = member
	watchers := p.watcherList()
	p.mu.Unlock()
	kind := cluster.MemberUp
	if existed {
		kind = cluster.MemberUpdated
	}
	emit(cluster.Event{Kind: kind, Member: member}, watchers)
}

func (p *Presence) remove(id string) {
	if id == p.self.ID {
		return
	}
	p.mu.Lock()
	member, existed := p.members[id]
	delete(p.members, id)
	watchers := p.watcherList()
	p.mu.Unlock()
	if existed {
		emit(cluster.Event{Kind: cluster.MemberDown, Member: member}, watchers)
	}
}

func (p *Presence) expire() {
	p.mu.Lock()
	var downs []cluster.Member
	for id, member := range p.members {
		if p.stale(member) {
			delete(p.members, id)
			downs = append(downs, member)
		}
	}
	watchers := p.watcherList()
	p.mu.Unlock()
	for _, member := range downs {
		emit(cluster.Event{Kind: cluster.MemberDown, Member: member}, watchers)
	}
}

func (p *Presence) stale(member cluster.Member) bool {
	return p.ttl < time.Since(member.Seen)
}

func (p *Presence) watcherList() []func(cluster.Event) {
	list := make([]func(cluster.Event), 0, len(p.watchers))
	for _, fn := range p.watchers {
		list = append(list, fn)
	}
	return list
}

func (p *Presence) recordErr(err error) {
	p.mu.Lock()
	p.loopErr = err
	p.mu.Unlock()
}

func emit(event cluster.Event, watchers []func(cluster.Event)) {
	for _, fn := range watchers {
		fn(event)
	}
}
