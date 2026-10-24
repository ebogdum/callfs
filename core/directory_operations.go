package core

import (
	"context"
	"errors"
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

// CountDirectoryChildren returns the number of direct children of a directory
// whose metadata the caller has already resolved.
func (e *Engine) CountDirectoryChildren(ctx context.Context, path string) (int, error) {
	if counter, ok := e.metadataStore.(metadata.ChildCounter); ok {
		count, err := counter.CountChildren(ctx, path)
		if nil != err {
			return 0, fmt.Errorf("failed to count directory children: %w", err)
		}
		return count, nil
	}
	children, err := e.metadataStore.ListChildren(ctx, path)
	if nil != err {
		return 0, fmt.Errorf("failed to list directory children: %w", err)
	}
	return len(children), nil
}

// ListDirectoryRecursive lists directory contents recursively
func (e *Engine) ListDirectoryRecursive(ctx context.Context, path string, maxDepth int) ([]*metadata.Metadata, error) {
	return e.ListDirectoryRecursiveLimited(ctx, path, maxDepth, 0)
}

// ListDirectoryRecursiveLimited lists directory contents recursively, returning at most limit entries (0 means unlimited)
func (e *Engine) ListDirectoryRecursiveLimited(ctx context.Context, path string, maxDepth, limit int) ([]*metadata.Metadata, error) {
	md, err := e.metadataStore.Get(ctx, path)
	if nil != err {
		return nil, fmt.Errorf("failed to get directory metadata: %w", err)
	}
	if "directory" != md.Type {
		return nil, fmt.Errorf("path is not a directory")
	}

	storeDepth := 0
	if 0 <= maxDepth {
		storeDepth = maxDepth + 1
	}
	descendants, err := e.metadataStore.ListDescendants(ctx, path, storeDepth, limit)
	if nil != err {
		return nil, fmt.Errorf("failed to list directory %s: %w", path, err)
	}
	return descendants, nil
}

func (e *Engine) SearchDirectoryRecursiveLimited(ctx context.Context, path string, maxDepth, limit int, match func(*metadata.Metadata) bool) ([]*metadata.Metadata, int, error) {
	md, err := e.metadataStore.Get(ctx, path)
	if nil != err {
		return nil, 0, fmt.Errorf("failed to get directory metadata: %w", err)
	}
	if "directory" != md.Type {
		return nil, 0, fmt.Errorf("path is not a directory")
	}

	storeDepth := 0
	if 0 <= maxDepth {
		storeDepth = maxDepth + 1
	}
	matched, scanned, err := metadata.SearchDescendants(ctx, e.metadataStore, path, storeDepth, limit, match)
	if nil != err {
		return nil, 0, fmt.Errorf("failed to list directory %s: %w", path, err)
	}
	return matched, scanned, nil
}

// CreateDirectory creates a new directory
func (e *Engine) CreateDirectory(ctx context.Context, path string, md *metadata.Metadata) error {
	if e.atomicNamespace() {
		if err := e.ensureParentDirectories(ctx, path, md.BackendType, md.Owner); nil != err {
			return fmt.Errorf("failed to ensure parent directories: %w", err)
		}
		return e.createDirectoryLocked(ctx, path, md)
	}
	release, err := e.lockPaths(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer release()

	if err := e.ensureParentDirectories(ctx, path, md.BackendType, md.Owner); nil != err {
		return fmt.Errorf("failed to ensure parent directories: %w", err)
	}

	return e.createDirectoryLocked(ctx, path, md)
}

func (e *Engine) createDirectoryLocked(ctx context.Context, path string, md *metadata.Metadata) error {
	if _, err := e.metadataStore.Get(ctx, path); nil == err {
		return metadata.ErrAlreadyExists
	} else if !errors.Is(err, metadata.ErrNotFound) {
		return fmt.Errorf("failed to check existing directory: %w", err)
	}

	if "localfs" == md.BackendType {
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
	e.metadataCache.Invalidate(path)

	e.logger.Debug("Directory created successfully",
		zap.String("path", path),
		zap.String("backend", md.BackendType))

	return nil
}
