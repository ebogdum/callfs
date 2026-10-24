package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/backends"
	"github.com/ebogdum/callfs/v2/backends/internalproxy"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/metrics"
)

const (
	maxSubtreeMoveWorkers = 8
	maxSubtreeMoveEntries = 10000
)

// MoveOptions controls a move operation. Empty Backend/Instance mean "keep the
// source's"; a different value triggers a physical relocation of the bytes.
type MoveOptions struct {
	Backend       string // target backend type ("localfs", "s3", "erasure"); "" keeps source's
	Instance      string // target instance ID; "" keeps source's
	Overwrite     bool   // replace an existing destination instead of failing
	CreateParents bool   // create missing destination parent directories

	extraOps   []metadata.Op
	extraUndo  []metadata.Op
	extraLocks []string
}

type moveCommit struct {
	parents *parentPlan
	extra   []metadata.Op
	undo    []metadata.Op
}

// ErrInvalidRename indicates a malformed rename/move request (bad name, moving a
// path into its own subtree, etc.). Mapped to 400 by the handler.
type ErrInvalidRename struct{ msg string }

func (e *ErrInvalidRename) Error() string { return e.msg }

// ErrUnsupportedMove indicates a relocation the current release does not perform
// (e.g. erasure re-tiering or cross-backend/instance directory moves). Mapped to 400.
type ErrUnsupportedMove struct{ msg string }

func (e *ErrUnsupportedMove) Error() string { return e.msg }

type ErrProxyFailed struct{ err error }

func (e *ErrProxyFailed) Error() string { return "cross-instance proxy failed: " + e.err.Error() }

func (e *ErrProxyFailed) Unwrap() error { return e.err }

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

func (e *Engine) doMove(ctx context.Context, srcPath, dstPath string, opts MoveOptions) (err error) {
	start := time.Now()
	var movedBackend string
	defer func() {
		if nil == err && "" != movedBackend {
			metrics.FileOperationsTotal.WithLabelValues("rename", movedBackend).Inc()
			metrics.BackendOpDuration.WithLabelValues(movedBackend, "rename").Observe(time.Since(start).Seconds())
		}
	}()

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

	release, err := e.lockPaths(ctx, append([]string{srcPath, dstPath}, opts.extraLocks...)...)
	if err != nil {
		return err
	}
	defer release()

	mc := &moveCommit{extra: opts.extraOps, undo: opts.extraUndo}
	defer func() { e.discardParentPlan(ctx, mc.parents) }()

	srcMd, err := e.GetMetadataUncached(ctx, srcPath)
	if err != nil {
		return err
	}
	movedBackend = srcMd.BackendType

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

	isRelocation := targetBackend != srcMd.BackendType || targetInstance != srcInstance
	localRelocation := isRelocation && ("" == targetInstance || targetInstance == currentInstance)
	if localRelocation {
		if "directory" == srcMd.Type {
			return &ErrUnsupportedMove{msg: "moving a directory across backends or instances is not supported; move its files individually or keep it on the same backend/instance"}
		}
		if "erasure" == targetBackend || "erasure" == srcMd.BackendType {
			return &ErrUnsupportedMove{msg: "moving a file to or from the erasure backend (re-tiering) is not supported in this release"}
		}
	}

	var replaced *metadata.Metadata
	if !samePath {
		parents, err := e.ensureMoveDestinationParent(ctx, dstPath, srcMd.Owner, opts.CreateParents)
		if err != nil {
			return err
		}
		mc.parents = parents
		dstMd, err := e.handleExistingDestination(ctx, srcMd, dstPath, opts.Overwrite)
		if err != nil {
			return err
		}
		replaced = dstMd
	}

	if "directory" == srcMd.Type {
		if err := e.ensureSubtreeOwnedBy(ctx, srcMd); err != nil {
			return err
		}
	}

	if !isRelocation {
		// Pure rename / folder move: same backend and instance. Execute on the
		// node that owns the bytes.
		if srcInstance != "" && srcInstance != currentInstance {
			return e.moveThenCommit(ctx, mc, func() error {
				return e.proxyMove(ctx, srcInstance, srcPath, dstPath, MoveOptions{Overwrite: opts.Overwrite}, srcMd.Type == "directory")
			})
		}
		return e.moveWithinBackend(ctx, srcMd, dstPath, replaced, mc)
	}

	// Relocation: execute on the target instance (it ends up owning the bytes).
	if targetInstance != "" && targetInstance != currentInstance {
		return e.moveThenCommit(ctx, mc, func() error {
			return e.proxyMove(ctx, targetInstance, srcPath, dstPath, opts, srcMd.Type == "directory")
		})
	}

	return e.moveThenCommit(ctx, mc, func() error {
		return e.relocateFile(ctx, srcMd, dstPath, targetBackend, srcInstance, replaced)
	})
}

func (e *Engine) moveThenCommit(ctx context.Context, mc *moveCommit, move func() error) error {
	if err := e.applyWithParents(ctx, mc.parents, nil); nil != err {
		return fmt.Errorf("failed to create destination parents: %w", err)
	}
	if err := move(); nil != err {
		return err
	}
	if err := e.applyMetadataBatch(ctx, mc.extra); nil != err {
		return &moveExtrasError{err: err}
	}
	return nil
}

type moveExtrasError struct{ err error }

func (e *moveExtrasError) Error() string {
	return "move completed but follow-up metadata failed: " + e.err.Error()
}

func (e *moveExtrasError) Unwrap() error { return e.err }

// moveWithinBackend performs a rename / folder move where the backend and
// instance do not change. Bytes are moved in place by the backend; metadata is
// re-keyed afterwards.
func (e *Engine) moveWithinBackend(ctx context.Context, srcMd *metadata.Metadata, dstPath string, replaced *metadata.Metadata, mc *moveCommit) error {
	srcPath := srcMd.Path

	if isErasureMetadata(srcMd) {
		if err := e.moveErasureResource(ctx, srcMd, dstPath, replaced, mc); err != nil {
			return err
		}
		e.invalidateMovePaths(srcPath, dstPath, srcMd.Type == "directory")
		e.logger.Info("Erasure-coded resource renamed", zap.String("src", srcPath), zap.String("dst", dstPath))
		return nil
	}

	if srcMd.Type == "directory" {
		rollback, err := e.moveDirectoryBytes(ctx, srcMd, dstPath)
		if err != nil {
			return err
		}
		if err := e.rekeyMetadata(ctx, srcPath, dstPath, replaced, mc); err != nil {
			if rbErr := rollback(); rbErr != nil {
				e.logger.Error("Failed to roll back directory byte move after metadata failure",
					zap.String("src", srcPath), zap.String("dst", dstPath), zap.Error(rbErr))
			}
			return err
		}
		e.invalidateMovePaths(srcPath, dstPath, srcMd.Type == "directory")
		e.logger.Info("Directory moved", zap.String("src", srcPath), zap.String("dst", dstPath))
		return nil
	}

	storage := e.selectBackendByType(srcMd.BackendType)
	if err := storage.Move(ctx, relOf(srcPath), relOf(dstPath)); err != nil {
		metrics.BackendOpFailuresTotal.WithLabelValues(srcMd.BackendType, "rename").Inc()
		return fmt.Errorf("failed to move file bytes: %w", err)
	}
	if err := e.rekeyMetadata(ctx, srcPath, dstPath, replaced, mc); err != nil {
		if rbErr := storage.Move(ctx, relOf(dstPath), relOf(srcPath)); rbErr != nil {
			e.logger.Error("Failed to roll back file byte move after metadata failure",
				zap.String("src", srcPath), zap.String("dst", dstPath), zap.Error(rbErr))
		}
		return err
	}
	e.invalidateMovePaths(srcPath, dstPath, srcMd.Type == "directory")
	if isErasureMetadata(replaced) {
		e.discardErasureReplacement(ctx, dstPath)
	}
	e.logger.Info("File moved", zap.String("src", srcPath), zap.String("dst", dstPath))
	return nil
}

func (e *Engine) rekeyMetadata(ctx context.Context, srcPath, dstPath string, replaced *metadata.Metadata, mc *moveCommit) error {
	ops := make([]metadata.Op, 0, 2)
	if nil != replaced {
		ops = append(ops, metadata.Op{Kind: metadata.OpDelete, Path: dstPath})
	}
	ops = append(ops, metadata.Op{Kind: metadata.OpRename, Path: srcPath, NewPath: dstPath})
	if nil == mc {
		return e.applyMetadataBatch(ctx, ops)
	}
	ops = append(ops, mc.extra...)
	return e.applyWithParents(ctx, mc.parents, ops)
}

func isErasureMetadata(md *metadata.Metadata) bool {
	return nil != md && (md.ErasureCoded || "erasure" == md.BackendType)
}

func (e *Engine) unrekeyMetadata(ctx context.Context, srcPath, dstPath string, replaced *metadata.Metadata, mc *moveCommit) error {
	ops := []metadata.Op{{Kind: metadata.OpRename, Path: dstPath, NewPath: srcPath}}
	if nil != replaced {
		ops = append(ops, metadata.Op{Kind: metadata.OpCreate, Metadata: replaced})
	}
	if nil != mc {
		ops = append(ops, mc.undo...)
	}
	return e.applyMetadataBatch(ctx, ops)
}

func (e *Engine) moveErasureResource(ctx context.Context, srcMd *metadata.Metadata, dstPath string, replaced *metadata.Metadata, mc *moveCommit) error {
	srcPath := srcMd.Path
	if srcMd.Type == "directory" {
		restore, err := e.moveErasureSubtreeMetadata(ctx, srcPath, dstPath)
		if err != nil {
			return err
		}
		if err := e.rekeyMetadata(ctx, srcPath, dstPath, replaced, mc); err != nil {
			if rbErr := restore(); nil != rbErr {
				e.logger.Error("Failed to roll back erasure metadata re-key after metadata failure",
					zap.String("src", srcPath), zap.String("dst", dstPath), zap.Error(rbErr))
			}
			return err
		}
		return nil
	}

	em := e.GetErasureManager()
	if nil == em {
		return fmt.Errorf("cannot move %s: erasure manager not configured", srcPath)
	}
	if err := e.rekeyMetadata(ctx, srcPath, dstPath, replaced, mc); err != nil {
		return err
	}
	replacedInfo, err := em.RenameFileOverwrite(ctx, srcPath, dstPath)
	if err != nil {
		if rbErr := e.unrekeyMetadata(context.WithoutCancel(ctx), srcPath, dstPath, replaced, mc); nil != rbErr {
			e.logger.Error("Failed to roll back metadata re-key after erasure rename failure",
				zap.String("src", srcPath), zap.String("dst", dstPath), zap.Error(rbErr))
		}
		return fmt.Errorf("failed to rename erasure-coded file: %w", err)
	}
	if nil != replacedInfo {
		if err := em.DeleteShards(ctx, replacedInfo); nil != err {
			e.logger.Warn("Failed to delete replaced erasure shards after move",
				zap.String("path", dstPath), zap.Error(err))
		}
	}
	if nil != replaced && !isErasureMetadata(replaced) {
		e.discardReplacedBytes(ctx, replaced, dstPath)
	}
	return nil
}

func (e *Engine) discardErasureReplacement(ctx context.Context, dstPath string) {
	em := e.GetErasureManager()
	if nil == em {
		e.logger.Error("Cannot delete replaced erasure shards: erasure manager not configured", zap.String("path", dstPath))
		return
	}
	if err := em.DeleteFile(ctx, dstPath); nil != err {
		e.logger.Warn("Failed to delete replaced erasure shards after move",
			zap.String("path", dstPath), zap.Error(err))
	}
}

func (e *Engine) discardReplacedBytes(ctx context.Context, replaced *metadata.Metadata, dstPath string) {
	if err := e.selectBackendByType(replaced.BackendType).Delete(ctx, relOf(dstPath)); nil != err {
		e.logger.Warn("Failed to delete replaced destination bytes",
			zap.String("path", dstPath), zap.String("backend", replaced.BackendType), zap.Error(err))
	}
}

type subtreeMove struct {
	from string
	to   string
}

func (e *Engine) moveDirectoryBytes(ctx context.Context, srcMd *metadata.Metadata, dstPath string) (func() error, error) {
	srcPath := srcMd.Path
	if srcMd.BackendType == "localfs" {
		if err := e.localFSBackend.Move(ctx, relOf(srcPath), relOf(dstPath)); err != nil {
			return nil, err
		}
		return func() error {
			return e.localFSBackend.Move(context.WithoutCancel(ctx), relOf(dstPath), relOf(srcPath))
		}, nil
	}
	storage := e.selectBackendByType(srcMd.BackendType)
	items, err := e.listSubtreeWithinLimit(ctx, srcPath)
	if err != nil {
		return nil, err
	}
	moves := make([]subtreeMove, 0, len(items))
	markers := []subtreeMove{{from: srcPath, to: dstPath}}
	for _, it := range items {
		m := subtreeMove{from: it.Path, to: dstPath + it.Path[len(srcPath):]}
		if "file" == it.Type {
			moves = append(moves, m)
		} else if "directory" == it.Type {
			markers = append(markers, m)
		}
	}
	moved, err := e.runSubtreeMoves(ctx, storage, moves)
	if nil == err {
		err = moveDirectoryMarkers(ctx, storage, markers)
	}
	if nil != err {
		restoreCtx := context.WithoutCancel(ctx)
		if rbErr := errors.Join(
			e.reverseSubtreeMoves(restoreCtx, storage, moved),
			moveDirectoryMarkers(restoreCtx, storage, swapSubtreeMoves(markers)),
		); nil != rbErr {
			e.logger.Error("Failed to roll back partial directory byte move",
				zap.String("src", srcPath), zap.String("dst", dstPath), zap.Error(rbErr))
		}
		return nil, err
	}
	return func() error {
		restoreCtx := context.WithoutCancel(ctx)
		return errors.Join(
			e.reverseSubtreeMoves(restoreCtx, storage, moves),
			moveDirectoryMarkers(restoreCtx, storage, swapSubtreeMoves(markers)),
		)
	}, nil
}

func moveDirectoryMarkers(ctx context.Context, storage backends.Storage, markers []subtreeMove) error {
	for _, m := range markers {
		if err := storage.CreateDirectory(ctx, relOf(m.to)); nil != err {
			return fmt.Errorf("failed to create directory marker %s: %w", m.to, err)
		}
		if err := storage.Delete(ctx, relOf(m.from)+"/"); nil != err {
			return fmt.Errorf("failed to delete directory marker %s: %w", m.from, err)
		}
	}
	return nil
}

func swapSubtreeMoves(moves []subtreeMove) []subtreeMove {
	swapped := make([]subtreeMove, 0, len(moves))
	for _, m := range moves {
		swapped = append(swapped, subtreeMove{from: m.to, to: m.from})
	}
	return swapped
}

func (e *Engine) runSubtreeMoves(ctx context.Context, storage backends.Storage, moves []subtreeMove) ([]subtreeMove, error) {
	moveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, maxSubtreeMoveWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	moved := make([]subtreeMove, 0, len(moves))
	for _, m := range moves {
		if moveCtx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(m subtreeMove) {
			defer wg.Done()
			defer func() { <-sem }()
			err := storage.Move(moveCtx, relOf(m.from), relOf(m.to))
			mu.Lock()
			defer mu.Unlock()
			if nil != err {
				if nil == firstErr {
					firstErr = fmt.Errorf("failed to move %s: %w", m.from, err)
					cancel()
				}
				return
			}
			moved = append(moved, m)
		}(m)
	}
	wg.Wait()
	if nil != firstErr {
		return moved, firstErr
	}
	return moved, ctx.Err()
}

func (e *Engine) reverseSubtreeMoves(ctx context.Context, storage backends.Storage, moves []subtreeMove) error {
	restoreCtx := context.WithoutCancel(ctx)
	var firstErr error
	for _, m := range moves {
		if err := storage.Move(restoreCtx, relOf(m.to), relOf(m.from)); nil != err && nil == firstErr {
			firstErr = fmt.Errorf("failed to restore %s: %w", m.from, err)
		}
	}
	return firstErr
}

func (e *Engine) listSubtreeWithinLimit(ctx context.Context, dirPath string) ([]*metadata.Metadata, error) {
	items, err := e.ListDirectoryRecursive(ctx, dirPath, -1)
	if err != nil {
		return nil, err
	}
	if maxSubtreeMoveEntries < len(items) {
		return nil, &ErrUnsupportedMove{msg: fmt.Sprintf("directory %s has more than %d entries; move it in smaller parts", dirPath, maxSubtreeMoveEntries)}
	}
	return items, nil
}

func (e *Engine) listDescendantFiles(ctx context.Context, dirPath string) ([]*metadata.Metadata, error) {
	items, err := e.listSubtreeWithinLimit(ctx, dirPath)
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

func (e *Engine) ensureSubtreeOwnedBy(ctx context.Context, dirMd *metadata.Metadata) error {
	items, err := e.listSubtreeWithinLimit(ctx, dirMd.Path)
	if err != nil {
		return err
	}
	for _, it := range items {
		if dirMd.Owner != it.Owner {
			return fmt.Errorf("cannot move %s: %s owned by another user: %w", dirMd.Path, it.Path, auth.ErrPermissionDenied)
		}
	}
	return nil
}

// moveErasureSubtreeMetadata re-keys the erasure info of every erasure-coded
// file under a directory being renamed.
func (e *Engine) moveErasureSubtreeMetadata(ctx context.Context, srcDir, dstDir string) (func() error, error) {
	files, err := e.listDescendantFiles(ctx, srcDir)
	if nil != err {
		return nil, err
	}
	erasureFiles := make([]*metadata.Metadata, 0, len(files))
	for _, f := range files {
		if f.ErasureCoded {
			erasureFiles = append(erasureFiles, f)
		}
	}
	noop := func() error { return nil }
	if 0 == len(erasureFiles) {
		return noop, nil
	}
	em := e.GetErasureManager()
	if nil == em {
		return nil, fmt.Errorf("cannot move %s: erasure manager not configured", srcDir)
	}
	renamed := make([]subtreeMove, 0, len(erasureFiles))
	rollback := func() error {
		restoreCtx := context.WithoutCancel(ctx)
		var errs []error
		for _, m := range renamed {
			if err := em.RenameFile(restoreCtx, m.to, m.from); nil != err {
				errs = append(errs, fmt.Errorf("failed to restore erasure file %s: %w", m.from, err))
			}
		}
		return errors.Join(errs...)
	}
	for _, f := range erasureFiles {
		newPath := dstDir + f.Path[len(srcDir):]
		if err := em.RenameFile(ctx, f.Path, newPath); nil != err {
			err = fmt.Errorf("failed to rename erasure file %s: %w", f.Path, err)
			if rbErr := rollback(); nil != rbErr {
				return nil, errors.Join(err, rbErr)
			}
			return nil, err
		}
		renamed = append(renamed, subtreeMove{from: f.Path, to: newPath})
	}
	return rollback, nil
}

// relocateFile physically moves a single file's bytes to targetBackend on the
// current (target) instance, reading from the source instance if necessary.
// Bytes are copied before the source is removed, so a failure leaves the source
// intact.
func (e *Engine) relocateFile(ctx context.Context, srcMd *metadata.Metadata, dstPath, targetBackend, srcInstance string, replaced *metadata.Metadata) error {
	srcPath := srcMd.Path
	currentInstance := e.currentInstanceID

	reader, size, err := e.openSourceForRelocation(ctx, srcMd, srcInstance)
	if err != nil {
		return fmt.Errorf("failed to read source for relocation: %w", err)
	}
	defer reader.Close()

	targetStorage := e.selectBackendByType(targetBackend)
	writePath := relOf(dstPath)
	if nil != replaced {
		if writePath, err = newStagingRelativePath(); nil != err {
			return err
		}
	}
	if err := targetStorage.Create(ctx, writePath, reader, size); err != nil {
		if nil != replaced {
			e.discardBackendObject(ctx, targetStorage, dstPath, writePath)
		}
		return fmt.Errorf("failed to write relocated file: %w", err)
	}
	if nil != replaced {
		if err := targetStorage.Move(ctx, writePath, relOf(dstPath)); err != nil {
			e.discardBackendObject(ctx, targetStorage, dstPath, writePath)
			return fmt.Errorf("failed to commit relocated file: %w", err)
		}
	}

	sameInstance := srcInstance == "" || srcInstance == currentInstance

	if sameInstance {
		if srcPath != dstPath {
			if err := e.rekeyMetadata(ctx, srcPath, dstPath, replaced, nil); err != nil {
				e.rollbackRelocatedBytes(ctx, targetStorage, dstPath, replaced)
				return err
			}
		}
		carried := *srcMd
		newMd := &carried
		newMd.Path = dstPath
		newMd.Name = pathBase(dstPath)
		newMd.BackendType = targetBackend
		newMd.CallFSInstanceID = &currentInstance
		newMd.UpdatedAt = time.Now()
		if err := e.metadataStore.Update(ctx, newMd); nil != err {
			restoreCtx := context.WithoutCancel(ctx)
			if srcPath != dstPath {
				if rbErr := e.unrekeyMetadata(restoreCtx, srcPath, dstPath, replaced, nil); nil != rbErr {
					return errors.Join(err, fmt.Errorf("failed to restore source path: %w", rbErr))
				}
			}
			e.rollbackRelocatedBytes(restoreCtx, targetStorage, dstPath, replaced)
			return err
		}
		if err := e.selectBackendByType(srcMd.BackendType).Delete(ctx, relOf(srcPath)); err != nil {
			e.logger.Warn("Failed to delete source bytes after cross-backend move",
				zap.String("src", srcPath), zap.Error(err))
		}
		e.discardRelocationReplacement(ctx, replaced, dstPath, targetBackend)
		e.invalidateMovePaths(srcPath, dstPath, srcMd.Type == "directory")
		e.logger.Info("File relocated across backends",
			zap.String("src", srcPath), zap.String("dst", dstPath), zap.String("backend", targetBackend))
		return nil
	}

	newMd := cloneForRelocation(srcMd, dstPath, targetBackend, currentInstance)
	sourceDeleted := false
	if srcPath == dstPath {
		if err := e.DeleteFileOnInstance(ctx, srcInstance, srcPath); err != nil {
			e.rollbackRelocatedBytes(ctx, targetStorage, dstPath, replaced)
			return fmt.Errorf("failed to delete source before same-path cross-instance move: %w", err)
		}
		sourceDeleted = true
	}
	if nil != replaced {
		if err := e.metadataStore.Delete(ctx, dstPath); nil != err {
			e.rollbackRelocatedBytes(ctx, targetStorage, dstPath, replaced)
			return fmt.Errorf("failed to delete replaced destination metadata: %w", err)
		}
	}
	if err := e.metadataStore.Create(ctx, newMd); err != nil {
		if nil != replaced {
			if restoreErr := e.metadataStore.Create(context.WithoutCancel(ctx), replaced); nil != restoreErr {
				err = errors.Join(err, fmt.Errorf("failed to restore replaced destination metadata: %w", restoreErr))
			}
		}
		e.rollbackRelocatedBytes(ctx, targetStorage, dstPath, replaced)
		return err
	}
	if !sourceDeleted {
		if err := e.DeleteFileOnInstance(ctx, srcInstance, srcPath); err != nil {
			e.logger.Warn("Failed to delete source after cross-instance move; source bytes may need cleanup",
				zap.String("src", srcPath), zap.String("src_instance", srcInstance), zap.Error(err))
		}
	}
	e.discardRelocationReplacement(ctx, replaced, dstPath, targetBackend)
	e.invalidateMovePaths(srcPath, dstPath, srcMd.Type == "directory")
	e.logger.Info("File relocated across instances",
		zap.String("src", srcPath), zap.String("dst", dstPath), zap.String("instance", currentInstance))
	return nil
}

func (e *Engine) rollbackRelocatedBytes(ctx context.Context, storage backends.Storage, dstPath string, replaced *metadata.Metadata) {
	if nil != replaced {
		return
	}
	if err := storage.Delete(ctx, relOf(dstPath)); nil != err {
		e.logger.Error("Failed to delete relocated bytes during rollback",
			zap.String("path", dstPath), zap.Error(err))
	}
}

func (e *Engine) discardRelocationReplacement(ctx context.Context, replaced *metadata.Metadata, dstPath, targetBackend string) {
	if nil == replaced {
		return
	}
	if isErasureMetadata(replaced) {
		e.discardErasureReplacement(ctx, dstPath)
		return
	}
	if replaced.BackendType == targetBackend {
		return
	}
	e.discardReplacedBytes(ctx, replaced, dstPath)
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
func (e *Engine) proxyMove(ctx context.Context, instanceID, srcPath, dstPath string, opts MoveOptions, subtree bool) error {
	if e.internalProxyAdapter == nil {
		return &ErrProxyFailed{err: fmt.Errorf("internal proxy not configured: no peer endpoints available")}
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
		var badRequest *internalproxy.BadRequestError
		if errors.As(err, &badRequest) {
			return &ErrInvalidRename{msg: badRequest.Error()}
		}
		return &ErrProxyFailed{err: err}
	}
	e.invalidateMovePaths(srcPath, dstPath, subtree)
	return nil
}

// ensureMoveDestinationParent verifies (and optionally creates) the destination's
// parent directory.
func (e *Engine) ensureMoveDestinationParent(ctx context.Context, dstPath, owner string, createParents bool) (*parentPlan, error) {
	parent := dirOf(dstPath)
	if parent == "/" {
		return nil, nil
	}
	_, err := e.metadataStore.Get(ctx, parent)
	if nil == err {
		return nil, nil
	}
	if !errors.Is(err, metadata.ErrNotFound) {
		return nil, err
	}
	if !createParents {
		return nil, &ErrInvalidRename{msg: fmt.Sprintf("destination parent %q does not exist", parent)}
	}
	plan, err := e.planParentDirectories(ctx, dstPath, "localfs", owner)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination parents: %w", err)
	}
	return plan, nil
}

// handleExistingDestination enforces the overwrite policy when the destination
// already exists.
func (e *Engine) handleExistingDestination(ctx context.Context, srcMd *metadata.Metadata, dstPath string, overwrite bool) (*metadata.Metadata, error) {
	dstMd, err := e.metadataStore.Get(ctx, dstPath)
	if nil != err {
		if errors.Is(err, metadata.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if !overwrite {
		return nil, metadata.ErrAlreadyExists
	}
	if dstMd.Owner != srcMd.Owner {
		return nil, fmt.Errorf("cannot overwrite %s owned by another user: %w", dstPath, auth.ErrPermissionDenied)
	}
	if dstMd.Type != srcMd.Type {
		return nil, fmt.Errorf("cannot overwrite %s with %s: %w", dstMd.Type, srcMd.Type, metadata.ErrAlreadyExists)
	}
	if "directory" == dstMd.Type {
		hasChildren, hasErr := e.metadataStore.HasChildren(ctx, dstPath)
		if nil != hasErr {
			return nil, hasErr
		}
		if hasChildren {
			return nil, fmt.Errorf("cannot overwrite non-empty directory %s: %w", dstPath, metadata.ErrAlreadyExists)
		}
	}
	return dstMd, nil
}

func (e *Engine) invalidateMovePaths(srcPath, dstPath string, subtree bool) {
	e.metadataCache.Invalidate(srcPath)
	e.metadataCache.Invalidate(dstPath)
	e.metadataCache.Invalidate(dirOf(srcPath))
	e.metadataCache.Invalidate(dirOf(dstPath))
	if subtree {
		e.metadataCache.InvalidatePrefix(srcPath)
	}
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
