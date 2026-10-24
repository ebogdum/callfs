package links

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/ebogdum/callfs/v2/metadata"
	"go.uber.org/zap"
)

const (
	linkCleanupTimeout = 30 * time.Second
	linkRetention      = 24 * time.Hour
)

type CleanupResult struct {
	At             time.Time
	ExpiredRemoved int
	UsedRemoved    int
	Err            error
}

var lastCleanup atomic.Pointer[CleanupResult]

func LastCleanup() (CleanupResult, bool) {
	result := lastCleanup.Load()
	if nil == result {
		return CleanupResult{}, false
	}
	return *result, true
}

func StartCleanupWorker(ctx context.Context, metadataStore metadata.Store, interval time.Duration, logger *zap.Logger) {
	if nil == metadataStore {
		logger.Error("Cannot start cleanup worker: metadata store is nil")
		return
	}

	go func() {
		logger.Info("Starting single-use link cleanup worker",
			zap.Duration("interval", interval))

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				cleanupLinks(ctx, metadataStore, logger)
			case <-ctx.Done():
				logger.Info("Cleanup worker shutting down")
				return
			}
		}
	}()
}

func cleanupLinks(parentCtx context.Context, metadataStore metadata.Store, logger *zap.Logger) {
	result := CleanupResult{At: time.Now()}

	expiredCount, expiredErr := cleanupExpiredLinks(parentCtx, metadataStore, logger)
	if nil != expiredErr {
		logger.Error("Failed to cleanup expired links", zap.Error(expiredErr))
	} else {
		logger.Info("Expired single-use link cleanup finished",
			zap.Int("count", expiredCount))
	}

	usedCount, usedErr := cleanupUsedLinks(parentCtx, metadataStore, logger)
	if nil != usedErr {
		logger.Error("Failed to cleanup used links", zap.Error(usedErr))
	} else {
		logger.Info("Used single-use link cleanup finished",
			zap.Int("count", usedCount))
	}

	result.ExpiredRemoved = expiredCount
	result.UsedRemoved = usedCount
	result.Err = errors.Join(expiredErr, usedErr)
	lastCleanup.Store(&result)
}

func cleanupExpiredLinks(parentCtx context.Context, metadataStore metadata.Store, logger *zap.Logger) (int, error) {
	ctx, cancel := context.WithTimeout(parentCtx, linkCleanupTimeout)
	defer cancel()

	expiredBefore := time.Now().Add(-linkRetention)
	logger.Debug("Starting expired single-use link cleanup", zap.Time("before", expiredBefore))

	count, err := metadataStore.CleanupExpiredLinks(ctx, expiredBefore)
	if nil != err {
		return 0, fmt.Errorf("failed to cleanup expired links: %w", err)
	}

	return count, nil
}

func cleanupUsedLinks(parentCtx context.Context, metadataStore metadata.Store, logger *zap.Logger) (int, error) {
	ctx, cancel := context.WithTimeout(parentCtx, linkCleanupTimeout)
	defer cancel()

	olderThan := time.Now().Add(-linkRetention)
	logger.Debug("Starting used single-use link cleanup", zap.Time("older_than", olderThan))

	count, err := metadataStore.CleanupUsedLinks(ctx, olderThan)
	if nil != err {
		return 0, fmt.Errorf("failed to cleanup used links: %w", err)
	}

	return count, nil
}
