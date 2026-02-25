package core

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/metrics"
)

// GetFile retrieves file content
func (e *Engine) GetFile(ctx context.Context, path string) (io.ReadCloser, error) {
	// Get metadata to determine storage location
	md, err := e.GetMetadata(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get file metadata: %w", err)
	}

	if md.Type != "file" {
		return nil, fmt.Errorf("path is not a file")
	}

	// Route to appropriate backend
	ctx, storage := e.selectBackend(ctx, md)

	// Convert absolute path to relative path for backend
	relativePath := strings.TrimPrefix(path, "/")
	reader, err := storage.Open(ctx, relativePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	e.logger.Debug("File opened successfully",
		zap.String("path", path),
		zap.String("backend", md.BackendType),
		zap.Int64("size", md.Size))

	return reader, nil
}

// CreateFile creates a new file with content
func (e *Engine) CreateFile(ctx context.Context, path string, reader io.Reader, size int64, md *metadata.Metadata) error {
	start := time.Now()
	defer func() {
		metrics.FileOperationsTotal.WithLabelValues("create", md.BackendType).Inc()
		metrics.BackendOpDuration.WithLabelValues(md.BackendType, "create").Observe(time.Since(start).Seconds())
	}()

	lockKey := fmt.Sprintf("file:%s", path)

	// Acquire distributed lock
	acquired, err := e.lockManager.Acquire(ctx, lockKey)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !acquired {
		return fmt.Errorf("failed to acquire lock for file creation")
	}
	defer func() {
		if err := e.lockManager.Release(context.Background(), lockKey); err != nil {
			e.logger.Error("Failed to release lock", zap.String("lock_key", lockKey), zap.Error(err))
		}
	}()

	// Check if file already exists
	if _, err := e.metadataStore.Get(ctx, path); err == nil {
		return metadata.ErrAlreadyExists
	}

	// Ensure parent directories exist
	if err := e.ensureParentDirectories(ctx, path, md.BackendType); err != nil {
		return fmt.Errorf("failed to ensure parent directories: %w", err)
	}

	if md.BackendType == "localfs" {
		md.CallFSInstanceID = &e.currentInstanceID
	}

	// Create file in appropriate backend
	storage := e.selectBackendByType(md.BackendType)
	// Convert absolute path to relative path for backend
	relativePath := strings.TrimPrefix(path, "/")
	if err := storage.Create(ctx, relativePath, reader, size); err != nil {
		return fmt.Errorf("failed to create file in backend: %w", err)
	}

	// Store metadata
	md.Path = path
	md.Size = size
	md.CreatedAt = time.Now()
	md.UpdatedAt = time.Now()

	if err := e.metadataStore.Create(ctx, md); err != nil {
		// Attempt to clean up file from backend
		if deleteErr := storage.Delete(ctx, relativePath); deleteErr != nil {
			e.logger.Error("Failed to cleanup file after metadata creation failure",
				zap.String("path", path), zap.Error(deleteErr))
		}
		return fmt.Errorf("failed to store metadata: %w", err)
	}

	if err := e.replicateFileToSecondaryBackend(ctx, path, size, md.BackendType); err != nil {
		return err
	}

	// Invalidate parent directory cache entries
	e.metadataCache.InvalidatePrefix(filepath.Dir(path))

	e.logger.Info("File created successfully",
		zap.String("path", path),
		zap.String("backend", md.BackendType),
		zap.Int64("size", size))

	return nil
}

// UpdateFile updates an existing file with new content
func (e *Engine) UpdateFile(ctx context.Context, path string, reader io.Reader, size int64, md *metadata.Metadata) error {
	lockKey := fmt.Sprintf("file:%s", path)

	// Acquire distributed lock
	acquired, err := e.lockManager.Acquire(ctx, lockKey)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !acquired {
		return fmt.Errorf("failed to acquire lock for file update")
	}
	defer func() {
		if err := e.lockManager.Release(context.Background(), lockKey); err != nil {
			e.logger.Error("Failed to release lock", zap.String("lock_key", lockKey), zap.Error(err))
		}
	}()

	// Get existing metadata
	existingMd, err := e.metadataStore.Get(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to get existing metadata: %w", err)
	}

	if existingMd.Type != "file" {
		return fmt.Errorf("path is not a file")
	}

	// Update file in appropriate backend
	ctx, storage := e.selectBackend(ctx, existingMd)
	// Convert absolute path to relative path for backend
	relativePath := strings.TrimPrefix(path, "/")
	if err := storage.Update(ctx, relativePath, reader, size); err != nil {
		return fmt.Errorf("failed to update file in backend: %w", err)
	}

	// Update metadata
	existingMd.Size = size
	existingMd.MTime = time.Now()
	existingMd.UpdatedAt = time.Now()

	if existingMd.CallFSInstanceID == nil && existingMd.BackendType == "localfs" {
		existingMd.CallFSInstanceID = &e.currentInstanceID
	}

	if err := e.metadataStore.Update(ctx, existingMd); err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	if err := e.replicateFileToSecondaryBackend(ctx, path, size, existingMd.BackendType); err != nil {
		return err
	}

	// Invalidate cache for this file
	e.metadataCache.Invalidate(path)

	e.logger.Info("File updated successfully",
		zap.String("path", path),
		zap.String("backend", existingMd.BackendType),
		zap.Int64("size", size))

	return nil
}

// DeleteFile removes a file
func (e *Engine) DeleteFile(ctx context.Context, path string) error {
	lockKey := fmt.Sprintf("file:%s", path)

	// Acquire distributed lock
	acquired, err := e.lockManager.Acquire(ctx, lockKey)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !acquired {
		return fmt.Errorf("failed to acquire lock for file deletion")
	}
	defer func() {
		if err := e.lockManager.Release(context.Background(), lockKey); err != nil {
			e.logger.Error("Failed to release lock", zap.String("lock_key", lockKey), zap.Error(err))
		}
	}()

	// Get metadata
	md, err := e.metadataStore.Get(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to get metadata: %w", err)
	}

	// Check if it's a directory and if it's empty
	if md.Type == "directory" {
		children, err := e.metadataStore.ListChildren(ctx, path)
		if err != nil {
			return fmt.Errorf("failed to check directory contents: %w", err)
		}
		if len(children) > 0 {
			return fmt.Errorf("directory not empty")
		}
	}

	// Delete from backend
	ctx, storage := e.selectBackend(ctx, md)
	// Convert absolute path to relative path for backend
	relativePath := strings.TrimPrefix(path, "/")
	if err := storage.Delete(ctx, relativePath); err != nil {
		return fmt.Errorf("failed to delete from backend: %w", err)
	}

	// Delete metadata
	if err := e.metadataStore.Delete(ctx, path); err != nil {
		return fmt.Errorf("failed to delete metadata: %w", err)
	}

	if err := e.deleteReplicatedFile(ctx, path, md.BackendType); err != nil {
		return err
	}

	// Invalidate cache for this file and parent directory
	e.metadataCache.Invalidate(path)
	e.metadataCache.InvalidatePrefix(filepath.Dir(path))

	e.logger.Info("File deleted successfully",
		zap.String("path", path),
		zap.String("backend", md.BackendType))

	return nil
}

// GetMetadata retrieves metadata with cache support
func (e *Engine) GetMetadata(ctx context.Context, path string) (*metadata.Metadata, error) {
	// Try cache first
	if cachedMd, found := e.metadataCache.Get(path); found {
		e.logger.Debug("Cache hit for metadata", zap.String("path", path))
		return cachedMd, nil
	}

	// Cache miss - fetch from store
	md, err := e.metadataStore.Get(ctx, path)
	if err != nil {
		return nil, err
	}

	// Store in cache
	e.metadataCache.Set(path, md)
	e.logger.Debug("Cache miss for metadata - stored in cache", zap.String("path", path))

	return md, nil
}

func (e *Engine) replicateFileToSecondaryBackend(ctx context.Context, path string, size int64, primaryBackend string) error {
	if !e.replicationEnabled {
		return nil
	}

	replicaBackend := strings.ToLower(strings.TrimSpace(e.replicaBackend))
	if replicaBackend == "" || replicaBackend == strings.ToLower(primaryBackend) {
		return nil
	}

	primaryStorage := e.selectBackendByType(primaryBackend)
	replicaStorage := e.selectBackendByType(replicaBackend)
	relativePath := strings.TrimPrefix(path, "/")

	reader, err := primaryStorage.Open(ctx, relativePath)
	if err != nil {
		if e.requireReplicaAck {
			return fmt.Errorf("failed to open source for replication: %w", err)
		}
		e.logger.Warn("Replication skipped: failed opening source",
			zap.String("path", path),
			zap.String("primary_backend", primaryBackend),
			zap.String("replica_backend", replicaBackend),
			zap.Error(err))
		return nil
	}
	defer reader.Close()

	err = replicaStorage.Update(ctx, relativePath, reader, size)
	if err != nil {
		reader2, openErr := primaryStorage.Open(ctx, relativePath)
		if openErr != nil {
			if e.requireReplicaAck {
				return fmt.Errorf("failed to reopen source for replica create: %w", openErr)
			}
			e.logger.Warn("Replication skipped: failed reopening source",
				zap.String("path", path),
				zap.String("replica_backend", replicaBackend),
				zap.Error(openErr))
			return nil
		}
		defer reader2.Close()

		err = replicaStorage.Create(ctx, relativePath, reader2, size)
		if err != nil {
			if e.requireReplicaAck {
				return fmt.Errorf("failed to replicate file to secondary backend: %w", err)
			}
			e.logger.Warn("Replication to secondary backend failed",
				zap.String("path", path),
				zap.String("replica_backend", replicaBackend),
				zap.Error(err))
			return nil
		}
	}

	e.logger.Debug("Replicated file to secondary backend",
		zap.String("path", path),
		zap.String("primary_backend", primaryBackend),
		zap.String("replica_backend", replicaBackend))
	return nil
}

func (e *Engine) deleteReplicatedFile(ctx context.Context, path string, primaryBackend string) error {
	if !e.replicationEnabled {
		return nil
	}

	replicaBackend := strings.ToLower(strings.TrimSpace(e.replicaBackend))
	if replicaBackend == "" || replicaBackend == strings.ToLower(primaryBackend) {
		return nil
	}

	replicaStorage := e.selectBackendByType(replicaBackend)
	relativePath := strings.TrimPrefix(path, "/")
	err := replicaStorage.Delete(ctx, relativePath)
	if err != nil {
		if e.requireReplicaAck {
			return fmt.Errorf("failed to delete replicated file: %w", err)
		}
		e.logger.Warn("Failed deleting replicated file",
			zap.String("path", path),
			zap.String("replica_backend", replicaBackend),
			zap.Error(err))
	}

	return nil
}
