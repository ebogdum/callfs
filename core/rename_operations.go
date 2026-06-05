package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

// MoveOptions controls a move operation. Empty Backend/Instance mean "keep the
// source's"; a different value triggers a physical relocation of the bytes.
type MoveOptions struct {
	Backend       string // target backend type ("localfs", "s3", "erasure"); "" keeps source's
	Instance      string // target instance ID; "" keeps source's
	Overwrite     bool   // replace an existing destination instead of failing
	CreateParents bool   // create missing destination parent directories
}

// ErrInvalidRename indicates a malformed rename/move request (bad name, moving a
// path into its own subtree, etc.). Mapped to 400 by the handler.
type ErrInvalidRename struct{ msg string }

func (e *ErrInvalidRename) Error() string { return e.msg }

// ErrUnsupportedMove indicates a relocation the current release does not perform
// (e.g. erasure re-tiering or cross-backend/instance directory moves). Mapped to 400.
type ErrUnsupportedMove struct{ msg string }

func (e *ErrUnsupportedMove) Error() string { return e.msg }

func relOf(p string) string { return strings.TrimPrefix(p, "/") }

func dirOf(p string) string {
	d := path.Dir(p)
	if d == "." {
		return "/"
	}
	return d
}

// Rename changes only the name of a file or directory, leaving it in the same
// folder, backend, and instance. newName must be a single path segment.
func (e *Engine) Rename(ctx context.Context, srcPath, newName string) error {
	if newName == "" || strings.Contains(newName, "/") || newName == "." || newName == ".." {
		return &ErrInvalidRename{msg: "name must be a single path segment"}
	}
	parent := dirOf(srcPath)
	var dstPath string
	if parent == "/" {
		dstPath = "/" + newName
	} else {
		dstPath = parent + "/" + newName
	}
	return e.doMove(ctx, srcPath, dstPath, MoveOptions{})
}

// Move relocates a file or directory to dstPath, optionally changing its backend
// and/or owning instance.
func (e *Engine) Move(ctx context.Context, srcPath, dstPath string, opts MoveOptions) error {
	return e.doMove(ctx, srcPath, dstPath, opts)
}

func (e *Engine) doMove(ctx context.Context, srcPath, dstPath string, opts MoveOptions) error {
	srcPath = normalizeMovePath(srcPath)
	dstPath = normalizeMovePath(dstPath)

	if srcPath == "/" || dstPath == "/" {
		return &ErrInvalidRename{msg: "cannot rename or move the root directory"}
	}
	relocation := opts.Backend != "" || opts.Instance != ""
	samePath := dstPath == srcPath
	if samePath && !relocation {
		return &ErrInvalidRename{msg: "destination is the same as the source"}
	}
	if !samePath && strings.HasPrefix(dstPath, srcPath+"/") {
		return &ErrInvalidRename{msg: "cannot move a path into itself"}
	}

	// Lock both endpoints in a stable order to avoid deadlock.
	first, second := srcPath, dstPath
	if second < first {
		first, second = second, first
	}
	rel1, err := e.acquireMoveLock(ctx, first)
	if err != nil {
		return err
	}
	defer rel1()
	if second != first {
		rel2, err := e.acquireMoveLock(ctx, second)
		if err != nil {
			return err
		}
		defer rel2()
	}

	srcMd, err := e.GetMetadataUncached(ctx, srcPath)
	if err != nil {
		return err
	}

	currentInstance := e.currentInstanceID
	srcInstance := ""
	if srcMd.CallFSInstanceID != nil {
		srcInstance = *srcMd.CallFSInstanceID
	}

	targetBackend := opts.Backend
	if targetBackend == "" {
		targetBackend = srcMd.BackendType
	}
	if targetBackend != "localfs" && targetBackend != "s3" && targetBackend != "erasure" {
		return &ErrInvalidRename{msg: fmt.Sprintf("unknown backend %q", targetBackend)}
	}
	targetInstance := opts.Instance
	if targetInstance == "" {
		targetInstance = srcInstance
	}
	if targetInstance != "" && targetInstance != currentInstance && e.GetPeerEndpoint(targetInstance) == "" {
		return &ErrInvalidRename{msg: fmt.Sprintf("unknown instance %q", targetInstance)}
	}

	if !samePath {
		if err := e.ensureMoveDestinationParent(ctx, dstPath, srcMd.Owner, opts.CreateParents); err != nil {
			return err
		}
		if err := e.handleExistingDestination(ctx, srcMd, dstPath, opts.Overwrite); err != nil {
			return err
		}
	}

	isRelocation := targetBackend != srcMd.BackendType || targetInstance != srcInstance

	if !isRelocation {
		// Pure rename / folder move: same backend and instance. Execute on the
		// node that owns the bytes.
		if srcInstance != "" && srcInstance != currentInstance {
			return e.proxyMove(ctx, srcInstance, srcPath, dstPath, MoveOptions{})
		}
		return e.moveWithinBackend(ctx, srcMd, dstPath)
	}

	// Relocation: execute on the target instance (it ends up owning the bytes).
	if targetInstance != "" && targetInstance != currentInstance {
		return e.proxyMove(ctx, targetInstance, srcPath, dstPath, opts)
	}

	if srcMd.Type == "directory" {
		return &ErrUnsupportedMove{msg: "moving a directory across backends or instances is not supported; move its files individually or keep it on the same backend/instance"}
	}
	if targetBackend == "erasure" || srcMd.BackendType == "erasure" {
		return &ErrUnsupportedMove{msg: "moving a file to or from the erasure backend (re-tiering) is not supported in this release"}
	}
	return e.relocateFile(ctx, srcMd, dstPath, targetBackend, srcInstance)
}

// moveWithinBackend performs a rename / folder move where the backend and
// instance do not change. Bytes are moved in place by the backend; metadata is
// re-keyed afterwards.
func (e *Engine) moveWithinBackend(ctx context.Context, srcMd *metadata.Metadata, dstPath string) error {
	srcPath := srcMd.Path

	if srcMd.ErasureCoded || srcMd.BackendType == "erasure" {
		if srcMd.Type == "directory" {
			if err := e.moveErasureSubtreeMetadata(ctx, srcPath, dstPath); err != nil {
				return err
			}
		} else if em := e.GetErasureManager(); em != nil {
			if err := em.RenameFile(ctx, srcPath, dstPath); err != nil {
				return fmt.Errorf("failed to rename erasure-coded file: %w", err)
			}
		}
		if err := e.metadataStore.Rename(ctx, srcPath, dstPath); err != nil {
			return err
		}
		e.invalidateMovePaths(srcPath, dstPath)
		e.logger.Info("Erasure-coded resource renamed", zap.String("src", srcPath), zap.String("dst", dstPath))
		return nil
	}

	if srcMd.Type == "directory" {
		if err := e.moveDirectoryBytes(ctx, srcMd, dstPath); err != nil {
			return err
		}
		if err := e.metadataStore.Rename(ctx, srcPath, dstPath); err != nil {
			// Roll back the byte move so backend and metadata stay consistent.
			if rbErr := e.moveDirectoryBytesBack(ctx, srcMd, dstPath); rbErr != nil {
				e.logger.Error("Failed to roll back directory byte move after metadata failure",
					zap.String("src", srcPath), zap.String("dst", dstPath), zap.Error(rbErr))
			}
			return err
		}
		e.invalidateMovePaths(srcPath, dstPath)
		e.logger.Info("Directory moved", zap.String("src", srcPath), zap.String("dst", dstPath))
		return nil
	}

	storage := e.selectBackendByType(srcMd.BackendType)
	if err := storage.Move(ctx, relOf(srcPath), relOf(dstPath)); err != nil {
		return fmt.Errorf("failed to move file bytes: %w", err)
	}
	if err := e.metadataStore.Rename(ctx, srcPath, dstPath); err != nil {
		if rbErr := storage.Move(ctx, relOf(dstPath), relOf(srcPath)); rbErr != nil {
			e.logger.Error("Failed to roll back file byte move after metadata failure",
				zap.String("src", srcPath), zap.String("dst", dstPath), zap.Error(rbErr))
		}
		return err
	}
	e.invalidateMovePaths(srcPath, dstPath)
	e.logger.Info("File moved", zap.String("src", srcPath), zap.String("dst", dstPath))
	return nil
}

// moveDirectoryBytes moves the backend bytes of a directory subtree. localfs
// moves the whole tree atomically; object stores move each descendant file.
func (e *Engine) moveDirectoryBytes(ctx context.Context, srcMd *metadata.Metadata, dstPath string) error {
	srcPath := srcMd.Path
	if srcMd.BackendType == "localfs" {
		return e.localFSBackend.Move(ctx, relOf(srcPath), relOf(dstPath))
	}
	storage := e.selectBackendByType(srcMd.BackendType)
	files, err := e.listDescendantFiles(ctx, srcPath)
	if err != nil {
		return err
	}
	for _, f := range files {
		newPath := dstPath + f.Path[len(srcPath):]
		if err := storage.Move(ctx, relOf(f.Path), relOf(newPath)); err != nil {
			return fmt.Errorf("failed to move %s: %w", f.Path, err)
		}
	}
	return nil
}

func (e *Engine) moveDirectoryBytesBack(ctx context.Context, srcMd *metadata.Metadata, dstPath string) error {
	reverse := &metadata.Metadata{Path: dstPath, Type: "directory", BackendType: srcMd.BackendType}
	return e.moveDirectoryBytes(ctx, reverse, srcMd.Path)
}

// listDescendantFiles returns all file inodes under dirPath (recursively).
func (e *Engine) listDescendantFiles(ctx context.Context, dirPath string) ([]*metadata.Metadata, error) {
	items, err := e.ListDirectoryRecursive(ctx, dirPath, -1)
	if err != nil {
		return nil, err
	}
	files := make([]*metadata.Metadata, 0, len(items))
	for _, it := range items {
		if it.Type == "file" {
			files = append(files, it)
		}
	}
	return files, nil
}

// moveErasureSubtreeMetadata re-keys the erasure info of every erasure-coded
// file under a directory being renamed.
func (e *Engine) moveErasureSubtreeMetadata(ctx context.Context, srcDir, dstDir string) error {
	em := e.GetErasureManager()
	if em == nil {
		return nil
	}
	files, err := e.listDescendantFiles(ctx, srcDir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if !f.ErasureCoded {
			continue
		}
		newPath := dstDir + f.Path[len(srcDir):]
		if err := em.RenameFile(ctx, f.Path, newPath); err != nil {
			return fmt.Errorf("failed to rename erasure file %s: %w", f.Path, err)
		}
	}
	return nil
}

// relocateFile physically moves a single file's bytes to targetBackend on the
// current (target) instance, reading from the source instance if necessary.
// Bytes are copied before the source is removed, so a failure leaves the source
// intact.
func (e *Engine) relocateFile(ctx context.Context, srcMd *metadata.Metadata, dstPath, targetBackend, srcInstance string) error {
	srcPath := srcMd.Path
	currentInstance := e.currentInstanceID

	reader, size, err := e.openSourceForRelocation(ctx, srcMd, srcInstance)
	if err != nil {
		return fmt.Errorf("failed to read source for relocation: %w", err)
	}
	defer reader.Close()

	targetStorage := e.selectBackendByType(targetBackend)
	if err := targetStorage.Create(ctx, relOf(dstPath), reader, size); err != nil {
		return fmt.Errorf("failed to write relocated file: %w", err)
	}

	sameInstance := srcInstance == "" || srcInstance == currentInstance

	if sameInstance {
		// Cross-backend move on this instance: re-key metadata (unless the path
		// is unchanged — an in-place re-tier), point it at the new backend, then
		// delete the old bytes.
		if srcPath != dstPath {
			if err := e.metadataStore.Rename(ctx, srcPath, dstPath); err != nil {
				_ = targetStorage.Delete(ctx, relOf(dstPath))
				return err
			}
		}
		newMd, getErr := e.metadataStore.Get(ctx, dstPath)
		if getErr != nil {
			return getErr
		}
		newMd.BackendType = targetBackend
		newMd.CallFSInstanceID = &currentInstance
		newMd.UpdatedAt = time.Now()
		if err := e.metadataStore.Update(ctx, newMd); err != nil {
			return err
		}
		// The backend changed (same-instance relocation), so the source-backend
		// copy is always stale and must be removed, even for an in-place re-tier.
		if err := e.selectBackendByType(srcMd.BackendType).Delete(ctx, relOf(srcPath)); err != nil {
			e.logger.Warn("Failed to delete source bytes after cross-backend move",
				zap.String("src", srcPath), zap.Error(err))
		}
		e.invalidateMovePaths(srcPath, dstPath)
		e.logger.Info("File relocated across backends",
			zap.String("src", srcPath), zap.String("dst", dstPath), zap.String("backend", targetBackend))
		return nil
	}

	// Cross-instance move: create the destination inode here, then delete the
	// source (bytes + metadata row) on the originating instance.
	newMd := cloneForRelocation(srcMd, dstPath, targetBackend, currentInstance)
	if err := e.metadataStore.Create(ctx, newMd); err != nil {
		_ = targetStorage.Delete(ctx, relOf(dstPath))
		return err
	}
	if err := e.DeleteFileOnInstance(ctx, srcInstance, srcPath); err != nil {
		e.logger.Warn("Failed to delete source after cross-instance move; source bytes may need cleanup",
			zap.String("src", srcPath), zap.String("src_instance", srcInstance), zap.Error(err))
	}
	e.invalidateMovePaths(srcPath, dstPath)
	e.logger.Info("File relocated across instances",
		zap.String("src", srcPath), zap.String("dst", dstPath), zap.String("instance", currentInstance))
	return nil
}

func (e *Engine) openSourceForRelocation(ctx context.Context, srcMd *metadata.Metadata, srcInstance string) (io.ReadCloser, int64, error) {
	if srcMd.ErasureCoded {
		em := e.GetErasureManager()
		if em == nil {
			return nil, 0, fmt.Errorf("erasure manager not configured")
		}
		data, err := em.RetrieveFile(ctx, srcMd.Path)
		if err != nil {
			return nil, 0, err
		}
		return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
	}
	if srcInstance == "" || srcInstance == e.currentInstanceID {
		reader, err := e.selectBackendByType(srcMd.BackendType).Open(ctx, relOf(srcMd.Path))
		return reader, srcMd.Size, err
	}
	reader, err := e.internalProxyAdapter.OpenFromInstance(ctx, srcInstance, srcMd.Path)
	return reader, srcMd.Size, err
}

func cloneForRelocation(srcMd *metadata.Metadata, dstPath, targetBackend, instance string) *metadata.Metadata {
	now := time.Now()
	return &metadata.Metadata{
		Name:        pathBase(dstPath),
		Path:        dstPath,
		Type:        srcMd.Type,
		Size:        srcMd.Size,
		Mode:        srcMd.Mode,
		Owner:       srcMd.Owner,
		BackendType: targetBackend,
		ATime:       srcMd.ATime,
		MTime:       srcMd.MTime,
		CTime:       now,
		CallFSInstanceID: func() *string {
			if targetBackend == "localfs" {
				return &instance
			}
			return nil
		}(),
	}
}

func pathBase(p string) string {
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		return p[idx+1:]
	}
	return p
}

// proxyMove forwards a rename/move to another instance via the internal proxy.
func (e *Engine) proxyMove(ctx context.Context, instanceID, srcPath, dstPath string, opts MoveOptions) error {
	if e.internalProxyAdapter == nil {
		return fmt.Errorf("internal proxy not configured: no peer endpoints available")
	}
	body := map[string]any{"destination": dstPath}
	if opts.Backend != "" {
		body["backend"] = opts.Backend
	}
	if opts.Instance != "" {
		body["instance"] = opts.Instance
	}
	if opts.Overwrite {
		body["overwrite"] = true
	}
	if opts.CreateParents {
		body["create_parents"] = true
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to encode move request: %w", err)
	}
	if err := e.internalProxyAdapter.MoveOnInstance(ctx, instanceID, srcPath, payload); err != nil {
		return err
	}
	e.invalidateMovePaths(srcPath, dstPath)
	return nil
}

// ensureMoveDestinationParent verifies (and optionally creates) the destination's
// parent directory.
func (e *Engine) ensureMoveDestinationParent(ctx context.Context, dstPath, owner string, createParents bool) error {
	parent := dirOf(dstPath)
	if parent == "/" {
		return nil
	}
	if _, err := e.metadataStore.Get(ctx, parent); err == nil {
		return nil
	}
	if !createParents {
		return &ErrInvalidRename{msg: fmt.Sprintf("destination parent %q does not exist", parent)}
	}
	if err := e.ensureParentDirectories(ctx, dstPath, "localfs", owner); err != nil {
		return fmt.Errorf("failed to create destination parents: %w", err)
	}
	return nil
}

// handleExistingDestination enforces the overwrite policy when the destination
// already exists.
func (e *Engine) handleExistingDestination(ctx context.Context, srcMd *metadata.Metadata, dstPath string, overwrite bool) error {
	dstMd, err := e.metadataStore.Get(ctx, dstPath)
	if err != nil {
		return nil // does not exist
	}
	if !overwrite {
		return metadata.ErrAlreadyExists
	}
	if dstMd.Type != srcMd.Type {
		return fmt.Errorf("cannot overwrite %s with %s", dstMd.Type, srcMd.Type)
	}
	if dstMd.Type == "directory" {
		children, listErr := e.metadataStore.ListChildren(ctx, dstPath)
		if listErr != nil {
			return listErr
		}
		if len(children) > 0 {
			return fmt.Errorf("cannot overwrite non-empty directory")
		}
	}
	// The destination lock is already held by doMove, so delete without
	// re-acquiring it.
	return e.deleteLockedResource(ctx, dstPath, dstMd)
}

func (e *Engine) acquireMoveLock(ctx context.Context, p string) (func(), error) {
	lockKey := fmt.Sprintf("file:%s", p)
	acquired, err := e.lockManager.Acquire(ctx, lockKey)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !acquired {
		return nil, fmt.Errorf("failed to acquire lock for %s", p)
	}
	return func() {
		if relErr := e.lockManager.Release(context.Background(), lockKey); relErr != nil {
			e.logger.Error("Failed to release lock", zap.String("lock_key", lockKey), zap.Error(relErr))
		}
	}, nil
}

func (e *Engine) invalidateMovePaths(srcPath, dstPath string) {
	e.metadataCache.Invalidate(srcPath)
	e.metadataCache.Invalidate(dstPath)
	e.metadataCache.InvalidatePrefix(dirOf(srcPath))
	e.metadataCache.InvalidatePrefix(dirOf(dstPath))
	e.metadataCache.InvalidatePrefix(srcPath)
}

func normalizeMovePath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}
