package locks

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

const (
	redisLockKeyPrefix       = "callfs:lock:"
	defaultRedisPoolSize     = 10
	defaultRedisMinIdleConns = 5
	redisScanBatchSize       = 100
	redisBackendName         = "redis"
)

var (
	redisReleaseScript = redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`)
	redisRenewScript = redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("pexpire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`)
)

type RedisOptions struct {
	Addr         string
	Password     string
	PoolSize     int
	MinIdleConns int
	TLSConfig    *tls.Config
}

type redisLockStore interface {
	Scan(ctx context.Context, cursor uint64, match string, count int64) ([]string, uint64, error)
	Get(ctx context.Context, key string) (string, error)
	PTTL(ctx context.Context, key string) (time.Duration, error)
	Del(ctx context.Context, key string) (int64, error)
}

type redisClientStore struct {
	client *redis.Client
}

func (s redisClientStore) Scan(ctx context.Context, cursor uint64, match string, count int64) ([]string, uint64, error) {
	return s.client.Scan(ctx, cursor, match, count).Result()
}

func (s redisClientStore) Get(ctx context.Context, key string) (string, error) {
	return s.client.Get(ctx, key).Result()
}

func (s redisClientStore) PTTL(ctx context.Context, key string) (time.Duration, error) {
	return s.client.PTTL(ctx, key).Result()
}

func (s redisClientStore) Del(ctx context.Context, key string) (int64, error) {
	return s.client.Del(ctx, key).Result()
}

type RedisManager struct {
	lockCounters
	client      *redis.Client
	inspector   redisLockStore
	logger      *zap.Logger
	ttl         time.Duration
	waitTimeout time.Duration
}

type redisLease struct {
	manager    *RedisManager
	key        string
	lockKey    string
	ownerToken string
	stopOnce   sync.Once
	stop       chan struct{}
	ended      atomic.Bool
}

func NewRedisManagerWithOptions(opts RedisOptions, logger *zap.Logger) (*RedisManager, error) {
	poolSize := opts.PoolSize
	if poolSize <= 0 {
		poolSize = defaultRedisPoolSize
	}
	minIdleConns := opts.MinIdleConns
	if minIdleConns < 0 {
		minIdleConns = defaultRedisMinIdleConns
	}
	client := redis.NewClient(&redis.Options{
		Addr:         opts.Addr,
		Password:     opts.Password,
		DB:           0,
		PoolSize:     poolSize,
		MinIdleConns: minIdleConns,
		TLSConfig:    opts.TLSConfig,
	})

	if err := client.Ping(context.Background()).Err(); nil != err {
		if closeErr := client.Close(); nil != closeErr {
			return nil, fmt.Errorf("failed to connect to Redis: %w (closing client: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisManager{
		client:      client,
		inspector:   redisClientStore{client: client},
		logger:      logger,
		ttl:         30 * time.Second,
		waitTimeout: 30 * time.Second,
	}, nil
}

func (m *RedisManager) SetWaitTimeout(d time.Duration) {
	m.waitTimeout = d
}

func (m *RedisManager) Acquire(ctx context.Context, key string) (Lease, error) {
	ownerToken, err := generateOwnerToken()
	if err != nil {
		return nil, err
	}

	return waitForLock(ctx, m.waitTimeout, &m.lockCounters, func() (Lease, error) {
		return m.tryAcquire(ctx, key, ownerToken)
	}, nil)
}

func (m *RedisManager) tryAcquire(ctx context.Context, key, ownerToken string) (Lease, error) {
	lockKey := redisLockKeyPrefix + key
	acquired, err := m.client.SetNX(ctx, lockKey, ownerToken, m.ttl).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock for key %s: %w", key, err)
	}
	if !acquired {
		return nil, nil
	}

	lease := &redisLease{
		manager:    m,
		key:        key,
		lockKey:    lockKey,
		ownerToken: ownerToken,
		stop:       make(chan struct{}),
	}
	go lease.renewLoop()
	return lease, nil
}

func (l *redisLease) finish() {
	if l.ended.CompareAndSwap(false, true) {
		activeLocks.Dec()
	}
}

func (l *redisLease) renewLoop() {
	m := l.manager
	ticker := time.NewTicker(m.ttl / 3)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			renewed, err := l.renew()
			if err != nil {
				m.recordRenewError()
				m.logger.Warn("Failed to renew lock", zap.String("key", l.key), zap.Error(err))
				continue
			}
			if !renewed {
				m.recordLost()
				l.finish()
				m.logger.Warn("Lock lost before release", zap.String("key", l.key))
				return
			}
		}
	}
}

func (l *redisLease) renew() (bool, error) {
	m := l.manager
	ctx, cancel := context.WithTimeout(context.Background(), m.ttl/3)
	defer cancel()

	renewed, err := redisRenewScript.Run(ctx, m.client, []string{l.lockKey}, l.ownerToken, m.ttl.Milliseconds()).Int64()
	if err != nil {
		return false, fmt.Errorf("failed to renew lock %s: %w", l.lockKey, err)
	}
	return renewed == 1, nil
}

func (l *redisLease) Release(ctx context.Context) error {
	l.stopOnce.Do(func() { close(l.stop) })
	l.finish()

	if _, err := redisReleaseScript.Run(ctx, l.manager.client, []string{l.lockKey}, l.ownerToken).Result(); err != nil {
		lockOperations.WithLabelValues("release", "error").Inc()
		return fmt.Errorf("failed to release lock %s: %w", l.lockKey, err)
	}
	lockOperations.WithLabelValues("release", "ok").Inc()
	return nil
}

func (m *RedisManager) Snapshot(ctx context.Context) ([]HeldLock, error) {
	var held []HeldLock
	var cursor uint64
	for {
		lockKeys, next, err := m.inspector.Scan(ctx, cursor, redisLockKeyPrefix+"*", redisScanBatchSize)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lock keys: %w", err)
		}
		for _, lockKey := range lockKeys {
			lock, found, err := m.describe(ctx, lockKey)
			if err != nil {
				return nil, err
			}
			if found {
				held = append(held, lock)
			}
		}
		if 0 == next {
			return held, nil
		}
		cursor = next
	}
}

func (m *RedisManager) describe(ctx context.Context, lockKey string) (HeldLock, bool, error) {
	token, err := m.inspector.Get(ctx, lockKey)
	if errors.Is(err, redis.Nil) {
		return HeldLock{}, false, nil
	}
	if err != nil {
		return HeldLock{}, false, fmt.Errorf("failed to read lock %s: %w", lockKey, err)
	}
	ttl, err := m.inspector.PTTL(ctx, lockKey)
	if err != nil {
		return HeldLock{}, false, fmt.Errorf("failed to read ttl of lock %s: %w", lockKey, err)
	}
	if 0 > ttl {
		return HeldLock{}, false, nil
	}
	return HeldLock{
		Key:       strings.TrimPrefix(lockKey, redisLockKeyPrefix),
		Backend:   redisBackendName,
		Holder:    holderHint(token),
		ExpiresAt: time.Now().Add(ttl),
	}, true, nil
}

func (m *RedisManager) ForceRelease(ctx context.Context, key string) (bool, error) {
	deleted, err := m.inspector.Del(ctx, redisLockKeyPrefix+key)
	if err != nil {
		return false, fmt.Errorf("failed to force release lock %s: %w", key, err)
	}
	lockOperations.WithLabelValues("force_release", "ok").Inc()
	return 0 < deleted, nil
}

func (m *RedisManager) Close() error {
	return m.client.Close()
}
