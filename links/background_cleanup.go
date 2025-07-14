package links

import (
	"context"
	"fmt"
	"time"

	"github.com/ebogdum/callfs/metadata"
	"go.uber.org/zap"
)

// StartCleanupWorker starts a background goroutine that periodically cleans up
// expired and used single-use links from the metadata store.
func StartCleanupWorker(ctx context.Context, metadataStore metadata.Store, interval time.Duration, logger *zap.Logger) {
	if metadataStore == nil {
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
				cleanupLinks(metadataStore, logger)
			case <-ctx.Done():
				logger.Info("Cleanup worker shutting down")
				return
			}
		}
	}()
}

// cleanupLinks removes expired and used single-use links from the metadata store.
func cleanupLinks(metadataStore metadata.Store, logger *zap.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Clean up expired active links
	expiredCount, err := cleanupExpiredLinks(ctx, metadataStore, logger)
	if err != nil {
		logger.Error("Failed to cleanup expired links", zap.Error(err))
	} else if expiredCount > 0 {
		logger.Info("Cleaned up expired single-use links",
			zap.Int("count", expiredCount))
	}

	// Clean up used links older than 24 hours
	usedCount, err := cleanupUsedLinks(ctx, metadataStore, logger)
	if err != nil {
		logger.Error("Failed to cleanup used links", zap.Error(err))
	} else if usedCount > 0 {
		logger.Info("Cleaned up used single-use links",
			zap.Int("count", usedCount))
	}
}

// cleanupExpiredLinks removes active links that have expired.
func cleanupExpiredLinks(ctx context.Context, metadataStore metadata.Store, logger *zap.Logger) (int, error) {
	now := time.Now()

	// Clean up all expired active links
	count, err := metadataStore.CleanupExpiredLinks(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired links: %w", err)
	}

	if count > 0 {
		logger.Debug("Cleaned up expired single-use links",
			zap.Int("count", count),
			zap.Time("before", now))
	}

	return count, nil
}

// cleanupUsedLinks removes used links that are older than the retention period.
func cleanupUsedLinks(ctx context.Context, metadataStore metadata.Store, logger *zap.Logger) (int, error) {
	// Clean up used links older than 24 hours
	olderThan := time.Now().Add(-24 * time.Hour)

	count, err := metadataStore.CleanupUsedLinks(ctx, olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup used links: %w", err)
	}

	if count > 0 {
		logger.Debug("Cleaned up used single-use links",
			zap.Int("count", count),
			zap.Time("older_than", olderThan))
	}

	return count, nil
}
