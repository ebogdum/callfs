package localbus

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ebogdum/callfs/v2/cluster"
)

type pool struct {
	cond    *sync.Cond
	ready   []*item
	pending int
}

type item struct {
	pool       string
	id         string
	affinity   string
	body       []byte
	enqueuedAt time.Time
	attempt    int
}

type consumer struct {
	ctx     context.Context
	cancel  context.CancelFunc
	stopped bool
}

func (l *Local) poolFor(name string) *pool {
	p := l.pools[name]
	if nil == p {
		p = &pool{cond: sync.NewCond(&l.mu)}
		l.pools[name] = p
	}
	return p
}

func (l *Local) Enqueue(ctx context.Context, poolName, affinity, id string, body any) error {
	if !cluster.ValidToken(poolName) {
		return cluster.ErrInvalidName
	}
	if "" == id {
		return cluster.ErrInvalidTask
	}
	if "" != affinity && l.self.ID != affinity {
		return cluster.ErrNoResponders
	}
	if err := ctx.Err(); nil != err {
		return cluster.ToError(err)
	}
	raw, err := cluster.Encode(body)
	if nil != err {
		return cluster.NewError(http.StatusBadRequest, "encode", err.Error())
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return cluster.ErrClosed
	}
	dedupKey := poolName + "\x00" + id
	if at, seen := l.dedup[dedupKey]; seen && cluster.TaskDedupWindow > now.Sub(at) {
		return nil
	}
	p := l.poolFor(poolName)
	if l.opts.TaskMaxPending <= p.pending {
		return cluster.ErrQueueFull
	}
	if dedupPruneAt <= len(l.dedup) {
		for k, at := range l.dedup {
			if cluster.TaskDedupWindow <= now.Sub(at) {
				delete(l.dedup, k)
			}
		}
	}
	l.dedup[dedupKey] = now
	p.pending++
	p.ready = append(p.ready, &item{pool: poolName, id: id, affinity: affinity, body: raw, enqueuedAt: now})
	p.cond.Broadcast()
	return nil
}

func (l *Local) Consume(poolName string, n int, h cluster.TaskHandler) func() {
	if !cluster.ValidToken(poolName) || nil == h {
		return func() {}
	}
	if 1 > n {
		n = 1
	}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return func() {}
	}
	p := l.poolFor(poolName)
	ctx, cancel := context.WithCancel(l.ctx)
	c := &consumer{ctx: ctx, cancel: cancel}
	l.mu.Unlock()
	for i := 0; i < n; i++ {
		go l.work(p, c, h)
	}
	return func() {
		l.mu.Lock()
		c.stopped = true
		p.cond.Broadcast()
		l.mu.Unlock()
		cancel()
	}
}

func (l *Local) work(p *pool, c *consumer, h cluster.TaskHandler) {
	for {
		l.mu.Lock()
		for 0 == len(p.ready) && !c.stopped && !l.closed {
			p.cond.Wait()
		}
		if c.stopped || l.closed {
			l.mu.Unlock()
			return
		}
		it := p.ready[0]
		p.ready[0] = nil
		p.ready = p.ready[1:]
		l.mu.Unlock()
		l.run(p, c, h, it)
	}
}

func (l *Local) run(p *pool, c *consumer, h cluster.TaskHandler, it *item) {
	attempt := it.attempt + 1
	task := cluster.NewTask(it.pool, it.id, it.affinity, attempt, it.enqueuedAt, it.body, c.ctx.Err)
	err := runTask(c.ctx, h, task)
	switch {
	case nil == err:
		l.finish(p)
	case nil != c.ctx.Err() && !cluster.IsPermanent(err):
		l.requeue(p, it, 0)
	case cluster.IsPermanent(err) || l.opts.TaskMaxDeliver <= attempt:
		l.finish(p)
		if nil != l.opts.OnDeadLetter {
			l.opts.OnDeadLetter(cluster.DeadLetter{
				Pool:       it.pool,
				ID:         it.id,
				Affinity:   it.affinity,
				Attempts:   attempt,
				Error:      err.Error(),
				EnqueuedAt: it.enqueuedAt,
				FailedAt:   time.Now(),
				Body:       it.body,
			})
		}
	default:
		it.attempt = attempt
		l.requeue(p, it, cluster.TaskRetryDelay(l.opts.TaskBackoff, attempt))
	}
}

func runTask(ctx context.Context, h cluster.TaskHandler, t *cluster.Task) (err error) {
	defer func() {
		if recovered := recover(); nil != recovered {
			err = fmt.Errorf("task handler panic: %v", recovered)
		}
	}()
	return h(ctx, t)
}

func (l *Local) finish(p *pool) {
	l.mu.Lock()
	p.pending--
	l.mu.Unlock()
}

func (l *Local) requeue(p *pool, it *item, delay time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	if 0 >= delay {
		p.ready = append(p.ready, it)
		p.cond.Broadcast()
		return
	}
	var timer *time.Timer
	timer = time.AfterFunc(delay, func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		delete(l.timers, timer)
		if l.closed {
			return
		}
		p.ready = append(p.ready, it)
		p.cond.Broadcast()
	})
	l.timers[timer] = struct{}{}
}
