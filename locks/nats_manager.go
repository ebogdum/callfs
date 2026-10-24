package locks

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

const (
	natsBackendName     = "nats"
	natsLockBucket      = "CALLFS_LOCKS"
	natsLockPrefix      = "l."
	natsQueuePrefix     = "q."
	natsKVSubjectPrefix = "$KV."
	natsOpTimeout       = 5 * time.Second
	natsReleaseTimeout  = 10 * time.Second
)

type natsLockValue struct {
	Key        string    `json:"key"`
	Owner      string    `json:"owner"`
	Holder     string    `json:"holder"`
	Node       string    `json:"node"`
	AcquiredAt time.Time `json:"acquired_at"`
}

type NATSManager struct {
	lockCounters
	js          jetstream.JetStream
	locks       jetstream.KeyValue
	nodeID      string
	ttl         time.Duration
	queueStale  time.Duration
	logger      *zap.Logger
	waitTimeout atomic.Int64
	stopChan    chan struct{}
	closeOnce   sync.Once
	watcher     jetstream.KeyWatcher
	held        sync.Map

	mu      sync.Mutex
	waiters map[string]map[chan struct{}]struct{}
	queues  map[string][]natsQueueEntry
	queued  map[string]uint64
	seen    uint64
}

type natsQueueEntry struct {
	order uint64
	key   string
}

type natsTicket struct {
	manager  *NATSManager
	key      string
	value    []byte
	order    uint64
	rev      atomic.Uint64
	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

type natsLease struct {
	manager  *NATSManager
	key      string
	lockKey  string
	value    []byte
	rev      atomic.Uint64
	stopOnce sync.Once
	stop     chan struct{}
	ended    atomic.Bool
}

var (
	_ Manager   = (*NATSManager)(nil)
	_ Inspector = (*NATSManager)(nil)
)

func NewNATSManager(ctx context.Context, js jetstream.JetStream, nodeID string, ttl, queueStale time.Duration, logger *zap.Logger) (*NATSManager, error) {
	if nil == js {
		return nil, errors.New("nats lock manager requires a jetstream handle")
	}
	if nil == logger {
		logger = zap.NewNop()
	}
	locks, err := js.KeyValue(ctx, natsLockBucket)
	if nil != err {
		return nil, fmt.Errorf("failed to open lock bucket %s: %w", natsLockBucket, err)
	}
	watcher, err := locks.WatchAll(context.WithoutCancel(ctx))
	if nil != err {
		return nil, fmt.Errorf("failed to watch lock bucket %s: %w", natsLockBucket, err)
	}
	m := &NATSManager{
		js:         js,
		locks:      locks,
		nodeID:     nodeID,
		ttl:        ttl,
		queueStale: queueStale,
		logger:     logger,
		stopChan:   make(chan struct{}),
		watcher:    watcher,
		waiters:    map[string]map[chan struct{}]struct{}{},
		queues:     map[string][]natsQueueEntry{},
		queued:     map[string]uint64{},
	}
	m.waitTimeout.Store(int64(defaultWaitTimeout))
	go m.dispatchReleases()
	return m, nil
}

func (m *NATSManager) SetWaitTimeout(d time.Duration) {
	m.waitTimeout.Store(int64(d))
}

func natsLockKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return natsLockPrefix + base64.RawURLEncoding.EncodeToString(sum[:])
}

func isWrongSequence(err error) bool {
	if nil == err {
		return false
	}
	if errors.Is(err, jetstream.ErrKeyExists) || errors.Is(err, jetstream.ErrKeyRevisionMismatch) {
		return true
	}
	var apiErr *jetstream.APIError
	if errors.As(err, &apiErr) {
		return jetstream.JSErrCodeStreamWrongLastSequence == apiErr.ErrorCode || jetstream.JSErrCodeStreamWrongLastSequenceConstant == apiErr.ErrorCode
	}
	return false
}

func (m *NATSManager) dispatchReleases() {
	for entry := range m.watcher.Updates() {
		if nil == entry {
			continue
		}
		op := entry.Operation()
		removed := jetstream.KeyValueDelete == op || jetstream.KeyValuePurge == op
		key := entry.Key()
		m.mu.Lock()
		if entry.Revision() > m.seen {
			m.seen = entry.Revision()
		}
		if strings.HasPrefix(key, natsQueuePrefix) {
			hash, ok := queueKeyHash(key)
			if ok && m.applyQueueEvent(hash, key, entry, removed) {
				m.signalLocked(natsLockPrefix + hash)
			}
		} else if removed {
			m.signalLocked(key)
		}
		m.mu.Unlock()
	}
}

func (m *NATSManager) signalLocked(lockKey string) {
	for signal := range m.waiters[lockKey] {
		select {
		case signal <- struct{}{}:
		default:
		}
	}
}

func queueKeyHash(key string) (string, bool) {
	rest := key[len(natsQueuePrefix):]
	idx := strings.IndexByte(rest, '.')
	if 0 >= idx {
		return "", false
	}
	return rest[:idx], true
}

func (m *NATSManager) applyQueueEvent(hash, key string, entry jetstream.KeyValueEntry, removed bool) bool {
	old, exists := m.queued[key]
	if removed {
		if !exists {
			return false
		}
		m.removeQueueLocked(hash, key, old)
		return true
	}
	order := entry.Revision()
	if value := entry.Value(); 0 < len(value) {
		if parsed, err := strconv.ParseUint(string(value), 10, 64); nil == err {
			order = parsed
		}
	}
	if exists {
		if old == order {
			return false
		}
		m.removeQueueLocked(hash, key, old)
	}
	list := m.queues[hash]
	idx := sort.Search(len(list), func(i int) bool { return list[i].order > order || (list[i].order == order && list[i].key >= key) })
	list = append(list, natsQueueEntry{})
	copy(list[idx+1:], list[idx:])
	list[idx] = natsQueueEntry{order: order, key: key}
	m.queues[hash] = list
	m.queued[key] = order
	return true
}

func (m *NATSManager) removeQueueLocked(hash, key string, order uint64) {
	list := m.queues[hash]
	idx := sort.Search(len(list), func(i int) bool { return list[i].order > order || (list[i].order == order && list[i].key >= key) })
	if idx < len(list) && key == list[idx].key {
		list = append(list[:idx], list[idx+1:]...)
	}
	if 0 == len(list) {
		delete(m.queues, hash)
	} else {
		m.queues[hash] = list
	}
	delete(m.queued, key)
}

func (m *NATSManager) queueBusy(hash string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return 0 < len(m.queues[hash])
}

func (m *NATSManager) isQueueHead(hash string, t *natsTicket) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.seen < t.order {
		return false
	}
	list := m.queues[hash]
	return 0 < len(list) && t.key == list[0].key
}

func (m *NATSManager) subscribeRelease(lockKey string) (chan struct{}, func()) {
	signal := make(chan struct{}, 1)
	m.mu.Lock()
	set, ok := m.waiters[lockKey]
	if !ok {
		set = map[chan struct{}]struct{}{}
		m.waiters[lockKey] = set
	}
	set[signal] = struct{}{}
	m.mu.Unlock()
	return signal, func() {
		m.mu.Lock()
		delete(set, signal)
		if 0 == len(set) {
			delete(m.waiters, lockKey)
		}
		m.mu.Unlock()
	}
}

func (m *NATSManager) Acquire(ctx context.Context, key string) (Lease, error) {
	owner, err := generateOwnerToken()
	if nil != err {
		return nil, err
	}
	value, err := json.Marshal(natsLockValue{Key: key, Owner: owner, Holder: m.nodeID + ":" + holderHint(owner), Node: m.nodeID, AcquiredAt: time.Now().UTC()})
	if nil != err {
		return nil, err
	}
	lockKey := natsLockKey(key)
	hash := lockKey[len(natsLockPrefix):]
	signal, unsubscribe := m.subscribeRelease(lockKey)
	defer unsubscribe()
	var ticket *natsTicket
	defer func() {
		if nil == ticket {
			return
		}
		if err := ticket.leave(ctx); nil != err {
			m.logger.Warn("Failed to leave lock queue", zap.String("key", key), zap.Error(err))
		}
	}()
	return waitForLock(ctx, time.Duration(m.waitTimeout.Load()), &m.lockCounters, func() (Lease, error) {
		if nil != ticket {
			if !m.isQueueHead(hash, ticket) {
				return nil, nil
			}
			return m.tryAcquire(ctx, key, lockKey, value)
		}
		if 0 < m.queueStale && m.queueBusy(hash) {
			created, err := m.enqueue(ctx, key, hash, owner)
			if nil != err {
				return nil, err
			}
			ticket = created
			return nil, nil
		}
		lease, err := m.tryAcquire(ctx, key, lockKey, value)
		if nil != err || nil != lease || 0 >= m.queueStale {
			return lease, err
		}
		created, err := m.enqueue(ctx, key, hash, owner)
		if nil != err {
			return nil, err
		}
		ticket = created
		return nil, nil
	}, func() <-chan struct{} {
		return signal
	})
}

func (m *NATSManager) enqueue(ctx context.Context, key, hash, owner string) (*natsTicket, error) {
	queueKey := natsQueuePrefix + hash + "." + owner
	opCtx, cancel := context.WithTimeout(ctx, natsOpTimeout)
	defer cancel()
	rev, err := m.locks.Create(opCtx, queueKey, nil, jetstream.KeyTTL(m.queueStale))
	if nil != err {
		return nil, fmt.Errorf("failed to join lock queue for key %s: %w", key, err)
	}
	t := &natsTicket{manager: m, key: queueKey, value: []byte(strconv.FormatUint(rev, 10)), order: rev, stop: make(chan struct{}), done: make(chan struct{})}
	t.rev.Store(rev)
	go t.refreshLoop()
	return t, nil
}

func (t *natsTicket) refreshLoop() {
	defer close(t.done)
	m := t.manager
	interval := m.queueStale / 3
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-m.stopChan:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), interval)
			ack, err := m.js.Publish(ctx, natsKVSubjectPrefix+natsLockBucket+"."+t.key, t.value,
				jetstream.WithExpectLastSequencePerSubject(t.rev.Load()), jetstream.WithMsgTTL(m.queueStale))
			if nil == err {
				t.rev.Store(ack.Sequence)
				cancel()
				continue
			}
			if isWrongSequence(err) {
				rev, createErr := m.locks.Create(ctx, t.key, t.value, jetstream.KeyTTL(m.queueStale))
				cancel()
				if nil != createErr {
					m.logger.Warn("Failed to restore lock queue entry", zap.String("queue_key", t.key), zap.Error(createErr))
					continue
				}
				t.rev.Store(rev)
				continue
			}
			cancel()
			m.logger.Warn("Failed to refresh lock queue entry", zap.String("queue_key", t.key), zap.Error(err))
		}
	}
}

func (t *natsTicket) leave(ctx context.Context) error {
	t.stopOnce.Do(func() { close(t.stop) })
	<-t.done
	leaveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), natsReleaseTimeout)
	defer cancel()
	err := t.manager.locks.Purge(leaveCtx, t.key, jetstream.PurgeTTL(t.manager.queueStale))
	if nil != err && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return err
	}
	return nil
}

func (m *NATSManager) tryAcquire(ctx context.Context, key, lockKey string, value []byte) (Lease, error) {
	opCtx, cancel := context.WithTimeout(ctx, natsOpTimeout)
	defer cancel()
	rev, err := m.locks.Create(opCtx, lockKey, value, jetstream.KeyTTL(m.ttl))
	if nil != err {
		if isWrongSequence(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to acquire lock for key %s: %w", key, err)
	}
	lease := &natsLease{manager: m, key: key, lockKey: lockKey, value: value, stop: make(chan struct{})}
	lease.rev.Store(rev)
	m.held.Store(key, lease)
	go lease.renewLoop()
	return lease, nil
}

func (l *natsLease) Token() uint64 {
	return l.rev.Load()
}

func (m *NATSManager) FenceToken(key string) (uint64, bool) {
	value, ok := m.held.Load(key)
	if !ok {
		return 0, false
	}
	lease, isLease := value.(*natsLease)
	if !isLease || lease.ended.Load() {
		return 0, false
	}
	return lease.Token(), true
}

func (l *natsLease) finish() {
	if l.ended.CompareAndSwap(false, true) {
		l.manager.held.CompareAndDelete(l.key, l)
		activeLocks.Dec()
	}
}

func (l *natsLease) renewLoop() {
	m := l.manager
	ticker := time.NewTicker(m.ttl / 3)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-m.stopChan:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), m.ttl/3)
			ack, err := m.js.Publish(ctx, natsKVSubjectPrefix+natsLockBucket+"."+l.lockKey, l.value,
				jetstream.WithExpectLastSequencePerSubject(l.rev.Load()), jetstream.WithMsgTTL(m.ttl))
			cancel()
			if nil != err {
				if isWrongSequence(err) {
					m.recordLost()
					l.finish()
					m.logger.Warn("Lock lost before release", zap.String("key", l.key))
					return
				}
				m.recordRenewError()
				m.logger.Warn("Failed to renew lock", zap.String("key", l.key), zap.Error(err))
				continue
			}
			l.rev.Store(ack.Sequence)
		}
	}
}

func (l *natsLease) Release(ctx context.Context) error {
	l.stopOnce.Do(func() { close(l.stop) })
	wasActive := !l.ended.Load()
	l.finish()
	if !wasActive {
		return nil
	}
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), natsReleaseTimeout)
	defer cancel()
	err := l.manager.locks.Delete(releaseCtx, l.lockKey, jetstream.LastRevision(l.rev.Load()))
	if nil != err && !isWrongSequence(err) && !errors.Is(err, jetstream.ErrKeyNotFound) {
		lockOperations.WithLabelValues("release", "error").Inc()
		return fmt.Errorf("failed to release lock %s: %w", l.key, err)
	}
	lockOperations.WithLabelValues("release", "ok").Inc()
	return nil
}

func (m *NATSManager) Snapshot(ctx context.Context) ([]HeldLock, error) {
	lister, err := m.locks.ListKeysFiltered(ctx, natsLockPrefix+"*")
	if nil != err {
		return nil, fmt.Errorf("failed to list held locks: %w", err)
	}
	var names []string
	for name := range lister.Keys() {
		names = append(names, name)
	}
	held := make([]HeldLock, 0, len(names))
	for _, name := range names {
		entry, err := m.locks.Get(ctx, name)
		if nil != err {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return nil, fmt.Errorf("failed to read lock record: %w", err)
		}
		var value natsLockValue
		if err := json.Unmarshal(entry.Value(), &value); nil != err {
			continue
		}
		held = append(held, HeldLock{
			Key:        value.Key,
			Backend:    natsBackendName,
			Holder:     value.Holder,
			AcquiredAt: value.AcquiredAt,
			ExpiresAt:  entry.Created().Add(m.ttl),
		})
	}
	sort.Slice(held, func(i, j int) bool { return held[i].Key < held[j].Key })
	return held, nil
}

func (m *NATSManager) ForceRelease(ctx context.Context, key string) (bool, error) {
	lockKey := natsLockKey(key)
	if _, err := m.locks.Get(ctx, lockKey); nil != err {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to read lock %s: %w", key, err)
	}
	if err := m.locks.Purge(ctx, lockKey); nil != err {
		return false, fmt.Errorf("failed to force release lock %s: %w", key, err)
	}
	lockOperations.WithLabelValues("force_release", "ok").Inc()
	return true, nil
}

func (m *NATSManager) Close() error {
	var stopErr error
	m.closeOnce.Do(func() {
		close(m.stopChan)
		stopErr = m.watcher.Stop()
	})
	return stopErr
}
