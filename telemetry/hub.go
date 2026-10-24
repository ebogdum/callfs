package telemetry

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"go.uber.org/zap/zapcore"
)

var (
	ErrInvalidQuery    = errors.New("invalid telemetry query")
	ErrInvalidCapacity = errors.New("telemetry capacity must be at least 1")
	ErrInvalidEvent    = errors.New("invalid telemetry event")
)

const (
	DefaultPageLimit           = 200
	MaxPageLimit               = 1000
	subscriberBuffer           = 256
	replayLimit                = MaxPageLimit
	replicationFailureCapacity = 1000
)

type Config struct {
	InstanceID   string
	LogEntries   int
	AuditEntries int
	EventEntries int
}

type Hub struct {
	Logs         *LogRing
	Audit        *AuditRing
	Events       *EventBus
	AuthFailures *AuthFailureRing
	Replication  *ReplicationFailureRing
}

func NewHub(cfg Config) (*Hub, error) {
	logs, err := NewLogRing(cfg.LogEntries)
	if nil != err {
		return nil, err
	}
	audit, err := NewAuditRing(cfg.InstanceID, cfg.AuditEntries)
	if nil != err {
		return nil, err
	}
	events, err := NewEventBus(cfg.InstanceID, cfg.EventEntries)
	if nil != err {
		return nil, err
	}
	authFailures, err := NewAuthFailureRing(cfg.AuditEntries)
	if nil != err {
		return nil, err
	}
	replication, err := NewReplicationFailureRing(replicationFailureCapacity)
	if nil != err {
		return nil, err
	}
	return &Hub{
		Logs:         logs,
		Audit:        audit,
		Events:       events,
		AuthFailures: authFailures,
		Replication:  replication,
	}, nil
}

func (h *Hub) Tee(next zapcore.Core) zapcore.Core {
	return zapcore.NewTee(next, h.Logs.CoreAt(next))
}

type ring[T any] struct {
	mu       sync.RWMutex
	buf      []T
	capacity int
	next     int
}

func newRing[T any](capacity int) (*ring[T], error) {
	if 1 > capacity {
		return nil, ErrInvalidCapacity
	}
	return &ring[T]{capacity: capacity}, nil
}

func (r *ring[T]) add(item T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) < r.capacity {
		r.buf = append(r.buf, item)
		r.next = len(r.buf) % r.capacity
		return
	}
	r.buf[r.next] = item
	r.next = (r.next + 1) % r.capacity
}

func (r *ring[T]) size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.buf)
}

func (r *ring[T]) scan(visit func(T) bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := len(r.buf)
	for i := 0; i < n; i++ {
		if !visit(r.buf[(r.next+n-1-i)%n]) {
			return
		}
	}
}

type Message struct {
	ID   uint64
	Name string
	Data any
}

type Subscription struct {
	Replay  []Message
	C       <-chan Message
	ch      chan Message
	accept  func(Message) bool
	release func()
	once    sync.Once
}

func newSubscription(accept func(Message) bool, replay []Message) *Subscription {
	ch := make(chan Message, subscriberBuffer)
	return &Subscription{Replay: replay, C: ch, ch: ch, accept: accept}
}

func (s *Subscription) Close() {
	s.once.Do(s.release)
}

func detachFunc(mu *sync.Mutex, subs map[*Subscription]struct{}, sub *Subscription) func() {
	return func() {
		mu.Lock()
		defer mu.Unlock()
		delete(subs, sub)
		close(sub.ch)
	}
}

func fanout(subs map[*Subscription]struct{}, message Message) {
	for sub := range subs {
		if !sub.accept(message) {
			continue
		}
		deliver(sub.ch, message)
	}
}

func deliver(ch chan Message, message Message) {
	select {
	case ch <- message:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- message:
	default:
	}
}

func reverseMessages(items []Message) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func pageLimit(limit int) (int, error) {
	if 0 == limit {
		return DefaultPageLimit, nil
	}
	if 0 > limit || MaxPageLimit < limit {
		return 0, fmt.Errorf("%w: limit", ErrInvalidQuery)
	}
	return limit, nil
}

func encodeCursor(id uint64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatUint(id, 10)))
}

func parseCursor(cursor string) (uint64, error) {
	if "" == cursor {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if nil != err {
		return 0, fmt.Errorf("%w: cursor", ErrInvalidQuery)
	}
	id, err := strconv.ParseUint(string(raw), 10, 64)
	if nil != err {
		return 0, fmt.Errorf("%w: cursor", ErrInvalidQuery)
	}
	return id, nil
}
