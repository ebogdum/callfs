package localbus

import (
	"context"
	"sync"
	"time"

	"github.com/ebogdum/callfs/cluster"
)

const (
	defaultSelfID      = "local"
	subscriptionBuffer = 1024
	replyCacheTTL      = 2 * time.Minute
	replyCacheMax      = 1024
	dedupPruneAt       = 4096
)

type Options struct {
	Self           cluster.Member
	TaskBackoff    []time.Duration
	TaskMaxDeliver int
	TaskMaxPending int
	OnDeadLetter   func(cluster.DeadLetter)
}

type Local struct {
	opts   Options
	self   cluster.Member
	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.Mutex
	closed    bool
	services  map[string]*service
	topics    map[string]map[uint64]*subscription
	nextSub   uint64
	watchers  map[uint64]func(cluster.Event)
	nextWatch uint64
	leases    map[string]chan struct{}
	pools     map[string]*pool
	dedup     map[string]time.Time
	timers    map[*time.Timer]struct{}
	replies   map[string]reply
}

var (
	_ cluster.Bus      = (*Local)(nil)
	_ cluster.Presence = (*Local)(nil)
	_ cluster.Lease    = (*Local)(nil)
	_ cluster.Queue    = (*Local)(nil)
)

func New(opts Options) *Local {
	if "" == opts.Self.ID {
		opts.Self.ID = defaultSelfID
	}
	if "" == opts.Self.ServerName {
		opts.Self.ServerName = opts.Self.ID
	}
	if opts.Self.StartedAt.IsZero() {
		opts.Self.StartedAt = time.Now()
	}
	if nil == opts.TaskBackoff {
		opts.TaskBackoff = cluster.DefaultTaskBackoff()
	}
	if 1 > opts.TaskMaxDeliver {
		opts.TaskMaxDeliver = cluster.DefaultTaskMaxDeliver
	}
	if 1 > opts.TaskMaxPending {
		opts.TaskMaxPending = cluster.DefaultTaskMaxPending
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Local{
		opts:     opts,
		self:     opts.Self,
		ctx:      ctx,
		cancel:   cancel,
		services: make(map[string]*service),
		topics:   make(map[string]map[uint64]*subscription),
		watchers: make(map[uint64]func(cluster.Event)),
		leases:   make(map[string]chan struct{}),
		pools:    make(map[string]*pool),
		dedup:    make(map[string]time.Time),
		timers:   make(map[*time.Timer]struct{}),
		replies:  make(map[string]reply),
	}
}

func (l *Local) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	for timer := range l.timers {
		timer.Stop()
	}
	l.timers = make(map[*time.Timer]struct{})
	for _, p := range l.pools {
		p.cond.Broadcast()
	}
	l.mu.Unlock()
	l.cancel()
	return nil
}

func (l *Local) Self() cluster.Member {
	member := l.self
	member.Seen = time.Now()
	return member
}

func (l *Local) Members() []cluster.Member {
	return []cluster.Member{l.Self()}
}

func (l *Local) Alive(id string) bool {
	return l.self.ID == id
}

func (l *Local) Watch(fn func(cluster.Event)) func() {
	if nil == fn {
		return func() {}
	}
	l.mu.Lock()
	l.nextWatch++
	id := l.nextWatch
	l.watchers[id] = fn
	l.mu.Unlock()
	return func() {
		l.mu.Lock()
		delete(l.watchers, id)
		l.mu.Unlock()
	}
}
