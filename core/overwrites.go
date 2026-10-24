package core

import (
	"context"
	"errors"
	"fmt"
	"io"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/backends"
	"github.com/ebogdum/callfs/v2/backends/localfs"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/metrics"
)

type asideWriter interface {
	SupportsOverwriteAside(path, oldETag string) bool
	WriteOverwrite(ctx context.Context, path string, reader io.Reader, size int64) (*localfs.Overwrite, error)
	OverwriteLeftovers(ctx context.Context, skip ...string) ([]localfs.OverwriteLeftover, error)
	RestoreLeftover(leftover localfs.OverwriteLeftover) error
	RemoveLeftover(leftover localfs.OverwriteLeftover) error
}

func resolveAsideWriter(storage backends.Storage) asideWriter {
	for nil != storage {
		if writer, ok := storage.(asideWriter); ok {
			return writer
		}
		wrapped, ok := storage.(interface{ Unwrap() backends.Storage })
		if !ok {
			return nil
		}
		storage = wrapped.Unwrap()
	}
	return nil
}

func (e *Engine) awaitOldCopyDrop(path string) {
	if pending, ok := e.oldCopyDrops.Load(path); ok {
		<-pending.(chan struct{})
	}
}

func (e *Engine) dropOldCopy(path string, overwrite *localfs.Overwrite) {
	if "" == overwrite.OldPath() {
		return
	}
	done := make(chan struct{})
	e.oldCopyDrops.Store(path, done)
	go func() {
		defer close(done)
		defer e.oldCopyDrops.CompareAndDelete(path, done)
		if err := overwrite.DropOld(); nil != err {
			e.logger.Error("Failed to remove previous file copy", zap.String("path", path), zap.Error(err))
		}
	}()
}

func (e *Engine) servesLocally(md *metadata.Metadata) bool {
	return "file" == md.Type &&
		"localfs" == md.BackendType &&
		!md.ErasureCoded &&
		(nil == md.CallFSInstanceID || e.currentInstanceID == *md.CallFSInstanceID)
}

func (e *Engine) RecoverOverwrites(ctx context.Context) error {
	if nil == e.asideWriter {
		return nil
	}
	leftovers, err := e.asideWriter.OverwriteLeftovers(ctx, stagingDirName, uploadSpoolDirName)
	if nil != err {
		return err
	}
	var errs []error
	restored := 0
	for _, leftover := range leftovers {
		restoredOne, err := e.recoverOverwrite(ctx, leftover)
		if nil != err {
			errs = append(errs, err)
			continue
		}
		if restoredOne {
			restored++
		}
	}
	e.logger.Info("Overwrite recovery finished",
		zap.Int("leftovers", len(leftovers)),
		zap.Int("restored", restored),
		zap.Int("failed", len(errs)))
	return errors.Join(errs...)
}

func (e *Engine) recoverOverwrite(ctx context.Context, leftover localfs.OverwriteLeftover) (bool, error) {
	_, err := e.metadataStore.Get(ctx, "/"+leftover.Path)
	if nil == err {
		return false, nil
	}
	if !errors.Is(err, metadata.ErrNotFound) {
		return false, fmt.Errorf("failed to read metadata for %s: %w", leftover.Path, err)
	}
	if leftover.Old {
		md, err := e.metadataStore.Get(ctx, "/"+leftover.Target)
		if nil != err && !errors.Is(err, metadata.ErrNotFound) {
			return false, fmt.Errorf("failed to read metadata for %s: %w", leftover.Target, err)
		}
		if nil == err && e.servesLocally(md) && leftover.ETag == md.ETag {
			if err := e.asideWriter.RestoreLeftover(leftover); nil != err {
				return false, err
			}
			metrics.OverwriteRestoresTotal.Inc()
			e.invalidateFileCache("/" + leftover.Target)
			return true, nil
		}
	}
	return false, e.asideWriter.RemoveLeftover(leftover)
}
