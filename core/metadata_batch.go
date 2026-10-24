package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/backends"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/metrics"
)

const deleteBatchSize = 500

func (e *Engine) applyMetadataBatch(ctx context.Context, ops []metadata.Op) error {
	if 0 == len(ops) {
		return nil
	}
	var err error
	if 1 == len(ops) {
		if err = ops[0].Validate(); nil == err {
			err = metadata.ApplyOp(ctx, e.metadataStore, ops[0])
		}
	} else {
		err = metadata.ApplyBatch(ctx, e.metadataStore, ops)
	}
	e.invalidateOps(ops)
	return err
}

func (e *Engine) invalidateOps(ops []metadata.Op) {
	for _, op := range ops {
		target := op.TargetPath()
		e.invalidateFileCache(target)
		if metadata.OpRename == op.Kind {
			e.invalidateFileCache(op.NewPath)
			e.metadataCache.InvalidatePrefix(target + "/")
			e.metadataCache.InvalidatePrefix(op.NewPath + "/")
		}
	}
}

type parentPlan struct {
	target      string
	backendType string
	owner       string
	storage     backends.Storage
	dirs        []string
	ops         []metadata.Op
	done        bool
}

func (e *Engine) planParentDirectories(ctx context.Context, path, backendType, owner string) (*parentPlan, error) {
	var missing []string
	for dir := filepath.Dir(path); dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		_, err := e.GetMetadata(ctx, dir)
		if nil == err {
			break
		}
		if !errors.Is(err, metadata.ErrNotFound) {
			return nil, fmt.Errorf("failed to get parent directory metadata: %w", err)
		}
		missing = append(missing, dir)
	}

	if "" == backendType {
		backendType = "localfs"
	}
	plan := &parentPlan{
		target:      path,
		backendType: backendType,
		owner:       owner,
		storage:     e.selectBackendByType(backendType),
	}
	for i := len(missing) - 1; 0 <= i; i-- {
		dirPath := missing[i]
		now := time.Now()
		md := &metadata.Metadata{
			Name:        filepath.Base(dirPath),
			Path:        dirPath,
			Type:        "directory",
			Mode:        "0755",
			Owner:       owner,
			BackendType: backendType,
			ATime:       now,
			MTime:       now,
			CTime:       now,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if "localfs" == backendType {
			md.CallFSInstanceID = &e.currentInstanceID
		}
		if err := plan.storage.CreateDirectory(ctx, strings.TrimPrefix(dirPath, "/")); nil != err {
			e.discardParentPlan(ctx, plan)
			return nil, fmt.Errorf("failed to create directory in backend: %w", err)
		}
		plan.dirs = append(plan.dirs, dirPath)
		plan.ops = append(plan.ops, metadata.Op{Kind: metadata.OpCreate, Metadata: md})
	}
	return plan, nil
}

func (e *Engine) discardParentPlan(ctx context.Context, plan *parentPlan) {
	if nil == plan || plan.done {
		return
	}
	plan.done = true
	for i := len(plan.dirs) - 1; 0 <= i; i-- {
		if err := plan.storage.Delete(context.WithoutCancel(ctx), strings.TrimPrefix(plan.dirs[i], "/")); nil != err {
			e.logger.Error("Failed to cleanup directory after metadata creation failure",
				zap.String("path", plan.dirs[i]), zap.Error(err))
		}
	}
}

func (e *Engine) applyWithParents(ctx context.Context, plan *parentPlan, ops []metadata.Op) error {
	if nil == plan || plan.done || 0 == len(plan.ops) {
		return e.applyMetadataBatch(ctx, ops)
	}
	combined := make([]metadata.Op, 0, len(plan.ops)+len(ops))
	combined = append(combined, plan.ops...)
	combined = append(combined, ops...)
	err := e.applyMetadataBatch(ctx, combined)
	if nil == err {
		plan.done = true
		for _, dir := range plan.dirs {
			e.logger.Debug("Directory created successfully",
				zap.String("path", dir),
				zap.String("backend", plan.backendType))
		}
		return nil
	}
	if !errors.Is(err, metadata.ErrAlreadyExists) {
		e.discardParentPlan(ctx, plan)
		return err
	}
	plan.done = true
	if err := e.ensureParentDirectories(ctx, plan.target, plan.backendType, plan.owner); nil != err {
		return fmt.Errorf("failed to ensure parent directories: %w", err)
	}
	return e.applyMetadataBatch(ctx, ops)
}

type deleteTarget struct {
	path string
	dir  bool
}

type stagedDelete struct {
	ops    []metadata.Op
	finish func() error
}

func (e *Engine) stageDelete(ctx context.Context, path string, md *metadata.Metadata) (stagedDelete, error) {
	if em := e.GetErasureManager(); md.ErasureCoded && nil != em {
		found, err := em.DeleteFileShards(ctx, path)
		if nil != err {
			return stagedDelete{}, fmt.Errorf("failed to delete erasure-coded file: %w", err)
		}
		ops := make([]metadata.Op, 0, 2)
		if found {
			ops = append(ops, metadata.Op{Kind: metadata.OpDeleteErasureInfo, Path: path})
		}
		ops = append(ops, metadata.Op{Kind: metadata.OpDelete, Path: path})
		return stagedDelete{ops: ops, finish: func() error {
			if found {
				em.ForgetHealth(path)
			}
			e.invalidateFileCache(path)
			e.logger.Info("Erasure-coded file deleted", zap.String("path", path))
			return nil
		}}, nil
	}

	backendCtx, storage := e.selectBackend(ctx, md)
	relativePath := strings.TrimPrefix(path, "/")
	if err := storage.Delete(backendCtx, relativePath); nil != err {
		metrics.BackendOpFailuresTotal.WithLabelValues(md.BackendType, "delete").Inc()
		return stagedDelete{}, fmt.Errorf("failed to delete file from backend: %w", err)
	}
	return stagedDelete{ops: []metadata.Op{{Kind: metadata.OpDelete, Path: path}}, finish: func() error {
		if err := e.deleteReplicatedFile(backendCtx, path, md.BackendType); nil != err {
			return err
		}
		e.invalidateFileCache(path)
		e.logger.Info("File deleted successfully",
			zap.String("path", path),
			zap.String("backend", md.BackendType))
		return nil
	}}, nil
}

func (e *Engine) removeDetachedBytes(ctx context.Context, path string, md *metadata.Metadata) error {
	backendCtx, storage := e.selectBackend(ctx, md)
	if err := storage.Delete(backendCtx, strings.TrimPrefix(path, "/")); nil != err {
		metrics.BackendOpFailuresTotal.WithLabelValues(md.BackendType, "delete").Inc()
		return fmt.Errorf("failed to delete file from backend: %w", err)
	}
	if err := e.deleteReplicatedFile(backendCtx, path, md.BackendType); nil != err {
		return err
	}
	e.invalidateFileCache(path)
	return nil
}

func (e *Engine) treeDeleteTargets(ctx context.Context, root string) ([]deleteTarget, error) {
	descendants, err := e.metadataStore.ListDescendants(ctx, root, 0, 0)
	if nil != err {
		return nil, fmt.Errorf("failed to list directory %s: %w", root, err)
	}
	targets := make([]deleteTarget, 0, len(descendants)+1)
	for _, item := range descendants {
		if "directory" != item.Type {
			targets = append(targets, deleteTarget{path: item.Path})
		}
	}
	for i := len(descendants) - 1; 0 <= i; i-- {
		if "directory" == descendants[i].Type {
			targets = append(targets, deleteTarget{path: descendants[i].Path, dir: true})
		}
	}
	return append(targets, deleteTarget{path: root, dir: true}), nil
}

type deleteOutcome struct {
	files  int
	dirs   int
	failed *deleteTarget
	err    error
}

func (e *Engine) deleteTargets(ctx context.Context, targets []deleteTarget) deleteOutcome {
	var outcome deleteOutcome
	for start := 0; start < len(targets); start += deleteBatchSize {
		chunk := targets[start:min(start+deleteBatchSize, len(targets))]
		e.deleteChunk(ctx, chunk, &outcome)
		if nil != outcome.err {
			return outcome
		}
	}
	return outcome
}

func (e *Engine) deleteChunk(ctx context.Context, chunk []deleteTarget, outcome *deleteOutcome) {
	paths := make([]string, 0, len(chunk))
	for _, t := range chunk {
		paths = append(paths, t.path)
	}
	release, err := e.lockPaths(ctx, paths...)
	if nil != err {
		outcome.failed = &chunk[0]
		outcome.err = fmt.Errorf("failed to acquire lock: %w", err)
		return
	}
	defer release()

	type stagedItem struct {
		target  deleteTarget
		md      *metadata.Metadata
		staged  stagedDelete
		started time.Time
	}
	items := make([]stagedItem, 0, len(chunk))
	staged := make(map[string]struct{}, len(chunk))
	stagedChildren := make(map[string]int, len(chunk))
	ops := make([]metadata.Op, 0, len(chunk))
	var failed *deleteTarget
	var failure error
	for i := range chunk {
		t := chunk[i]
		md, err := e.metadataStore.Get(ctx, t.path)
		if nil != err {
			failed, failure = &chunk[i], fmt.Errorf("failed to get metadata: %w", err)
			break
		}
		if "directory" == md.Type {
			empty, err := e.directoryEmptyAfter(ctx, t.path, staged, stagedChildren)
			if nil != err {
				failed, failure = &chunk[i], fmt.Errorf("failed to check directory contents: %w", err)
				break
			}
			if !empty {
				failed, failure = &chunk[i], ErrDirectoryNotEmpty
				break
			}
		}
		started := time.Now()
		sd, err := e.stageDelete(ctx, t.path, md)
		if nil != err {
			if "directory" == md.Type && errors.Is(err, metadata.ErrPrecondition) {
				err = ErrDirectoryNotEmpty
			}
			failed, failure = &chunk[i], err
			break
		}
		items = append(items, stagedItem{target: t, md: md, staged: sd, started: started})
		staged[t.path] = struct{}{}
		stagedChildren[dirOf(t.path)]++
		ops = append(ops, sd.ops...)
	}

	if 0 < len(ops) {
		if err := e.applyMetadataBatch(ctx, ops); nil != err {
			outcome.failed = &items[0].target
			outcome.err = fmt.Errorf("failed to delete metadata: %w", err)
			return
		}
	}

	for i := range items {
		item := items[i]
		if err := item.staged.finish(); nil != err {
			outcome.failed = &items[i].target
			outcome.err = err
			return
		}
		metrics.FileOperationsTotal.WithLabelValues("delete", item.md.BackendType).Inc()
		metrics.BackendOpDuration.WithLabelValues(item.md.BackendType, "delete").Observe(time.Since(item.started).Seconds())
		if item.target.dir {
			outcome.dirs++
		} else {
			outcome.files++
		}
	}
	if nil != failure {
		outcome.failed = failed
		outcome.err = failure
	}
}

func (e *Engine) directoryEmptyAfter(ctx context.Context, path string, staged map[string]struct{}, stagedChildren map[string]int) (bool, error) {
	if 0 == stagedChildren[path] {
		return e.directoryIsEmpty(ctx, path)
	}
	children, err := e.metadataStore.ListChildren(ctx, path)
	if nil != err {
		return false, err
	}
	for _, child := range children {
		if _, ok := staged[child.Path]; !ok {
			return false, nil
		}
	}
	return true, nil
}

type pendingFile struct {
	path         string
	relativePath string
	md           *metadata.Metadata
	storage      backends.Storage
	stage        *replicaStage
	started      time.Time
}

func (e *Engine) writePendingFile(ctx context.Context, path string, reader io.Reader, size int64, md *metadata.Metadata) (*pendingFile, error) {
	started := time.Now()
	if "localfs" == md.BackendType {
		md.CallFSInstanceID = &e.currentInstanceID
	}

	storage := e.selectBackendByType(md.BackendType)
	relativePath := strings.TrimPrefix(path, "/")
	primaryCtx := withBackendMode(ctx, md.BackendType, md.Mode)
	counter := &countingReader{reader: reader}
	content, etagOf := hashingReader(counter)
	stage, err := e.writeWithReplica(ctx, path, md.BackendType, relativePath, size, content, func(body io.Reader) error {
		return storage.Create(primaryCtx, relativePath, body, size)
	})
	if nil != err {
		metrics.BackendOpFailuresTotal.WithLabelValues(md.BackendType, "create").Inc()
		if errors.Is(err, metadata.ErrAlreadyExists) {
			e.invalidateFileCache(path)
			return nil, fmt.Errorf("failed to create file in backend: %w", err)
		}
		e.discardBackendObject(ctx, storage, path, relativePath)
		e.invalidateFileCache(path)
		return nil, fmt.Errorf("failed to create file in backend: %w", err)
	}

	md.Path = path
	md.Size = counter.count
	md.ETag = etagOf()
	if err := e.verifyStored(primaryCtx, storage, relativePath, md.Size, md.ETag); nil != err {
		e.discardBackendObject(ctx, storage, path, relativePath)
		e.discardReplicaStage(ctx, path, stage)
		e.invalidateFileCache(path)
		return nil, err
	}
	md.CreatedAt = time.Now()
	md.UpdatedAt = time.Now()
	return &pendingFile{
		path:         path,
		relativePath: relativePath,
		md:           md,
		storage:      storage,
		stage:        stage,
		started:      started,
	}, nil
}

func (e *Engine) prepareFile(ctx context.Context, path string, reader io.Reader, size int64, md *metadata.Metadata) (*pendingFile, error) {
	_, err := e.metadataStore.Get(ctx, path)
	switch {
	case nil == err:
		return nil, metadata.ErrAlreadyExists
	case !errors.Is(err, metadata.ErrNotFound):
		return nil, fmt.Errorf("failed to get existing metadata: %w", err)
	}
	return e.writePendingFile(ctx, path, reader, size, md)
}

func (p *pendingFile) createOp() metadata.Op {
	return metadata.Op{Kind: metadata.OpCreate, Metadata: p.md}
}

func (e *Engine) abortFile(ctx context.Context, p *pendingFile) {
	e.discardBackendObject(ctx, p.storage, p.path, p.relativePath)
	e.discardReplicaStage(ctx, p.path, p.stage)
	e.invalidateFileCache(p.path)
}

func (e *Engine) finishFile(ctx context.Context, p *pendingFile) error {
	if err := e.commitReplicaStage(ctx, p.path, p.stage); nil != err {
		e.discardBackendObject(ctx, p.storage, p.path, p.relativePath)
		if deleteErr := e.metadataStore.Delete(ctx, p.path); nil != deleteErr {
			e.logger.Error("Failed to remove metadata after replication failure",
				zap.String("path", p.path), zap.Error(deleteErr))
		}
		e.invalidateFileCache(p.path)
		return err
	}

	e.scheduleReplication(p.md, p.path, p.storage, p.relativePath)
	e.invalidateFileCache(p.path)
	metrics.FileOperationsTotal.WithLabelValues("create", p.md.BackendType).Inc()
	metrics.BackendOpDuration.WithLabelValues(p.md.BackendType, "create").Observe(time.Since(p.started).Seconds())
	return nil
}
