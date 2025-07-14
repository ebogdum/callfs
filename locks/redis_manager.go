package locks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

// RedisManager implements distributed locking using Redis with Redlock algorithm
type RedisManager struct {
	client  *redis.Client
	logger  *zap.Logger
	ttl     time.Duration
	ownerID string // Unique identifier for this lock manager instance
}

// NewRedisManager creates a new Redis-based lock manager
func NewRedisManager(redisAddr, redisPassword string, logger *zap.Logger) (*RedisManager, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         redisAddr,
		Password:     redisPassword,
		DB:           0, // Default DB
		PoolSize:     10,
		MinIdleConns: 5,
	})

	// Test connection
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// Generate unique owner ID for this instance
	ownerBytes := make([]byte, 16)
	if _, err := rand.Read(ownerBytes); err != nil {
		return nil, fmt.Errorf("failed to generate owner ID: %w", err)
	}
	ownerID := hex.EncodeToString(ownerBytes)

	return &RedisManager{
		client:  client,
		logger:  logger,
		ttl:     30 * time.Second, // Lock TTL to prevent deadlocks
		ownerID: ownerID,
	}, nil
}

// Acquire attempts to acquire a distributed lock for the given key
func (m *RedisManager) Acquire(ctx context.Context, key string) (bool, error) {
	lockKey := fmt.Sprintf("callfs:lock:%s", key)

	// Use SET with NX (only if not exists) and EX (expiration) with unique owner value
	result := m.client.SetNX(ctx, lockKey, m.ownerID, m.ttl)
	if err := result.Err(); err != nil {
		return false, fmt.Errorf("failed to acquire lock for key %s: %w", key, err)
	}

	acquired := result.Val()

	if acquired {
		m.logger.Debug("Lock acquired",
			zap.String("key", key),
			zap.String("owner", m.ownerID),
			zap.Duration("ttl", m.ttl))
	} else {
		m.logger.Debug("Lock already held", zap.String("key", key))
	}

	return acquired, nil
}

// Release releases a previously acquired lock for the given key
func (m *RedisManager) Release(ctx context.Context, key string) error {
	lockKey := fmt.Sprintf("callfs:lock:%s", key)

	// Use Lua script to ensure atomicity (only delete if we own the lock)
	luaScript := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`

	result := m.client.Eval(ctx, luaScript, []string{lockKey}, m.ownerID)
	if err := result.Err(); err != nil {
		return fmt.Errorf("failed to release lock for key %s: %w", key, err)
	}

	deleted := result.Val().(int64)
	if deleted == 1 {
		m.logger.Debug("Lock released",
			zap.String("key", key),
			zap.String("owner", m.ownerID))
	} else {
		m.logger.Debug("Lock not owned or already released",
			zap.String("key", key),
			zap.String("owner", m.ownerID))
	}

	return nil
}

// Close closes the Redis client connection
func (m *RedisManager) Close() error {
	return m.client.Close()
}
