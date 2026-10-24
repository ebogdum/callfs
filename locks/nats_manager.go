package locks

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
	logger      *zap.Logger
	waitTimeout atomic.Int64
	stopChan    chan struct{}
	closeOnce   sync.Once
	watcher     jetstream.KeyWatcher

	mu      sync.Mutex
	waiters map[string]map[chan struct{}]struct{}
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

func NewNATSManager(ctx context.Context, js jetstream.JetStream, nodeID string, ttl time.Duration, logger *zap.Logger) (*NATSManager, error) {
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
	watcher, err := locks.WatchAll(context.WithoutCancel(ctx), jetstream.UpdatesOnly(), jetstream.MetaOnly())
	if nil != err {
		return nil, fmt.Errorf("failed to watch lock bucket %s: %w", natsLockBucket, err)
	}
	m := &NATSManager{
		js:       js,
		locks:    locks,
		nodeID:   nodeID,
		ttl:      ttl,
		logger:   logger,
		stopChan: make(chan struct{}),
		watcher:  watcher,
		waiters:  map[string]map[chan struct{}]struct{}{},
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
		if jetstream.KeyValueDelete != op && jetstream.KeyValuePurge != op {
			continue
		}
		m.mu.Lock()
		for signal := range m.waiters[entry.Key()] {
			select {
			case signal <- struct{}{}:
			default:
			}
		}
		m.mu.Unlock()
	}
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
	signal, unsubscribe := m.subscribeRelease(lockKey)
	defer unsubscribe()
	return waitForLock(ctx, time.Duration(m.waitTimeout.Load()), &m.lockCounters, func() (Lease, error) {
		return m.tryAcquire(ctx, key, lockKey, value)
	}, func() <-chan struct{} {
		return signal
	})
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
	go lease.renewLoop()
	return lease, nil
}

func (l *natsLease) Token() uint64 {
	return l.rev.Load()
}

func (l *natsLease) finish() {
	if l.ended.CompareAndSwap(false, true) {
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
	m.closeOnce.Do(func() {
		close(m.stopChan)
		_ = m.watcher.Stop()
	})
	return nil
}
