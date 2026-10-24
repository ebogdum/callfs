package core

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/backends"
	"github.com/ebogdum/callfs/v2/backends/internalproxy"
	"github.com/ebogdum/callfs/v2/locks"
	"github.com/ebogdum/callfs/v2/metadata"
)

func (e *Engine) selectBackend(ctx context.Context, md *metadata.Metadata) (context.Context, backends.Storage) {
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

	if md.CallFSInstanceID != nil {
		ctx = internalproxy.WithInstanceID(ctx, *md.CallFSInstanceID)
		return ctx, e.internalProxyBackend
	}

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

func (e *Engine) ensureParentDirectories(ctx context.Context, path string, backendType string, owner string) error {
	var missing []string
	for dir := filepath.Dir(path); dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		_, err := e.GetMetadata(ctx, dir)
		if err == nil {
			break
		}
		if !errors.Is(err, metadata.ErrNotFound) {
			return fmt.Errorf("failed to get parent directory metadata: %w", err)
		}
		missing = append(missing, dir)
	}

	if backendType == "" {
		backendType = "localfs"
	}

	for i := len(missing) - 1; i >= 0; i-- {
		dirPath := missing[i]
		parentMd := &metadata.Metadata{
			Name:        filepath.Base(dirPath),
			Path:        dirPath,
			Type:        "directory",
			Mode:        "0777",
			Owner:       owner,
			BackendType: backendType,
			ATime:       time.Now(),
			MTime:       time.Now(),
			CTime:       time.Now(),
		}

		if e.atomicNamespace() {
			err := e.createDirectoryLocked(ctx, dirPath, parentMd)
			if nil != err && !errors.Is(err, metadata.ErrAlreadyExists) {
				return err
			}
			e.metadataCache.Invalidate(dirPath)
			continue
		}
		release, err := e.lockPaths(ctx, dirPath)
		if nil != err {
			if errors.Is(err, locks.ErrLockHeld) {
				if _, getErr := e.GetMetadata(ctx, dirPath); nil == getErr {
					continue
				}
			}
			return fmt.Errorf("failed to acquire lock: %w", err)
		}
		err = e.createDirectoryLocked(ctx, dirPath, parentMd)
		release()
		if nil != err && !errors.Is(err, metadata.ErrAlreadyExists) {
			return err
		}
		e.metadataCache.Invalidate(dirPath)
	}
	return nil
}

func (e *Engine) EnsureRootDirectory(ctx context.Context) error {
	if _, err := e.metadataStore.Get(ctx, "/"); nil == err {
		e.logger.Debug("Root directory already exists")
		return nil
	} else if !errors.Is(err, metadata.ErrNotFound) {
		return fmt.Errorf("failed to read root directory metadata: %w", err)
	}

	rootMd := &metadata.Metadata{
		Name:        "/",
		Path:        "/",
		Type:        "directory",
		Mode:        "0777",
		Owner:       "root",
		BackendType: "localfs",
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
