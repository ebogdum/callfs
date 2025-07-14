package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

// ListDirectory lists directory contents
func (e *Engine) ListDirectory(ctx context.Context, path string) ([]*metadata.Metadata, error) {
	// Get directory metadata
	md, err := e.metadataStore.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get directory metadata: %w", err)
	}

	if md.Type != "directory" {
		return nil, fmt.Errorf("path is not a directory")
	}

	// List children from metadata store
	children, err := e.metadataStore.ListChildren(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory children: %w", err)
	}

	return children, nil
}

// ListDirectoryRecursive lists directory contents recursively
func (e *Engine) ListDirectoryRecursive(ctx context.Context, path string, maxDepth int) ([]*metadata.Metadata, error) {
	if maxDepth < 0 {
		maxDepth = 100 // Default maximum depth to prevent infinite recursion
	}

	var allItems []*metadata.Metadata
	return e.listDirectoryRecursiveHelper(ctx, path, 0, maxDepth, allItems)
}

// listDirectoryRecursiveHelper is the recursive helper function
func (e *Engine) listDirectoryRecursiveHelper(ctx context.Context, path string, currentDepth, maxDepth int, allItems []*metadata.Metadata) ([]*metadata.Metadata, error) {
	if currentDepth > maxDepth {
		return allItems, nil
	}

	// Get immediate children
	children, err := e.ListDirectory(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory %s: %w", path, err)
	}

	// Add all children to results
	allItems = append(allItems, children...)

	// Recursively process subdirectories
	for _, child := range children {
		if child.Type == "directory" {
			subItems, err := e.listDirectoryRecursiveHelper(ctx, child.Path, currentDepth+1, maxDepth, nil)
			if err != nil {
				e.logger.Warn("Failed to list subdirectory",
					zap.String("path", child.Path),
					zap.Error(err))
				continue // Continue with other directories instead of failing completely
			}
			allItems = append(allItems, subItems...)
		}
	}

	return allItems, nil
}

// CreateDirectory creates a new directory
func (e *Engine) CreateDirectory(ctx context.Context, path string, md *metadata.Metadata) error {
	lockKey := fmt.Sprintf("dir:%s", path)

	// Acquire distributed lock
	acquired, err := e.lockManager.Acquire(ctx, lockKey)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !acquired {
		return fmt.Errorf("failed to acquire lock for directory creation")
	}
	defer func() {
		if err := e.lockManager.Release(ctx, lockKey); err != nil {
			e.logger.Error("Failed to release lock", zap.String("lock_key", lockKey), zap.Error(err))
		}
	}()

	// Check if directory already exists
	if _, err := e.metadataStore.Get(ctx, path); err == nil {
		return metadata.ErrAlreadyExists
	}

	// Ensure parent directories exist
	if err := e.ensureParentDirectories(ctx, path); err != nil {
		return fmt.Errorf("failed to ensure parent directories: %w", err)
	}

	// Set instance ID for local FS directories
	if md.BackendType == "localfs" {
		md.CallFSInstanceID = &e.currentInstanceID
	}

	// Create directory in appropriate backend
	storage := e.selectBackendByType(md.BackendType)
	// Convert absolute path to relative path for backend
	relativePath := strings.TrimPrefix(path, "/")
	if err := storage.CreateDirectory(ctx, relativePath); err != nil {
		return fmt.Errorf("failed to create directory in backend: %w", err)
	}

	// Store metadata
	md.Path = path
	md.Type = "directory"
	md.Size = 0
	md.CreatedAt = time.Now()
	md.UpdatedAt = time.Now()

	if err := e.metadataStore.Create(ctx, md); err != nil {
		// Attempt to clean up directory from backend
		if deleteErr := storage.Delete(ctx, relativePath); deleteErr != nil {
			e.logger.Error("Failed to cleanup directory after metadata creation failure",
				zap.String("path", path), zap.Error(deleteErr))
		}
		return fmt.Errorf("failed to store metadata: %w", err)
	}

	e.logger.Info("Directory created successfully",
		zap.String("path", path),
		zap.String("backend", md.BackendType))

	return nil
}
