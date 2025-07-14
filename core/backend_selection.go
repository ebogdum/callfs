package core

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/backends"
	"github.com/ebogdum/callfs/backends/internalproxy"
	"github.com/ebogdum/callfs/metadata"
)

// selectBackend chooses the appropriate backend based on metadata
func (e *Engine) selectBackend(ctx context.Context, md *metadata.Metadata) (context.Context, backends.Storage) {
	// If the file belongs to this instance, use the appropriate local backend
	if md.CallFSInstanceID != nil && *md.CallFSInstanceID == e.currentInstanceID {
		switch md.BackendType {
		case "localfs":
			return ctx, e.localFSBackend
		case "s3":
			return ctx, e.s3Backend
		default:
			e.logger.Warn("Unknown backend type, defaulting to local FS",
				zap.String("backend_type", md.BackendType))
			return ctx, e.localFSBackend
		}
	}

	// File belongs to a different instance, use internal proxy
	if md.CallFSInstanceID != nil {
		ctx = internalproxy.WithInstanceID(ctx, *md.CallFSInstanceID)
		return ctx, e.internalProxyBackend
	}

	// Fallback for files without instance ID (legacy)
	switch md.BackendType {
	case "localfs":
		return ctx, e.localFSBackend
	case "s3":
		return ctx, e.s3Backend
	default:
		e.logger.Warn("Unknown backend type, defaulting to local FS",
			zap.String("backend_type", md.BackendType))
		return ctx, e.localFSBackend
	}
}

// selectBackendByType chooses backend by type string
func (e *Engine) selectBackendByType(backendType string) backends.Storage {
	switch backendType {
	case "localfs":
		return e.localFSBackend
	case "s3":
		return e.s3Backend
	default:
		return e.localFSBackend
	}
}

// ensureParentDirectories creates parent directories if they don't exist
func (e *Engine) ensureParentDirectories(ctx context.Context, path string) error {
	parentPath := filepath.Dir(path)
	if parentPath == "/" || parentPath == "." {
		return nil // Root directory should always exist
	}

	// Check if parent exists
	if _, err := e.metadataStore.Get(ctx, parentPath); err == nil {
		return nil // Parent exists
	}

	// Recursively ensure grandparent exists
	if err := e.ensureParentDirectories(ctx, parentPath); err != nil {
		return err
	}

	// Create parent directory
	parentMd := &metadata.Metadata{
		Name:        filepath.Base(parentPath),
		Type:        "directory",
		Mode:        "0755",
		UID:         1000,
		GID:         1000,
		BackendType: "localfs", // Default to local FS for auto-created directories
	}

	return e.CreateDirectory(ctx, parentPath, parentMd)
}

// EnsureRootDirectory ensures that the root directory metadata exists
func (e *Engine) EnsureRootDirectory(ctx context.Context) error {
	// Check if root directory already exists
	if _, err := e.metadataStore.Get(ctx, "/"); err == nil {
		e.logger.Debug("Root directory already exists")
		return nil
	}

	// Create root directory metadata
	rootMd := &metadata.Metadata{
		Name:        "/",
		Path:        "/",
		Type:        "directory",
		Mode:        "0755",
		UID:         0,         // Root user
		GID:         0,         // Root group
		BackendType: "localfs", // Default backend for root
		ATime:       time.Now(),
		MTime:       time.Now(),
		CTime:       time.Now(),
	}

	if err := e.metadataStore.Create(ctx, rootMd); err != nil {
		return fmt.Errorf("failed to create root directory metadata: %w", err)
	}

	e.logger.Info("Root directory created successfully")
	return nil
}
