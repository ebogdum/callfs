package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/backends"
	"github.com/ebogdum/callfs/backends/s3"
	"github.com/ebogdum/callfs/erasure"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/metrics"
)

const stagingDirName = ".callfs-staging"

var ErrDirectoryNotEmpty = errors.New("directory not empty")

var ErrNotAFile = errors.New("path is not a file")

var ErrReplicaCommitPending = errors.New("write committed on primary, replica commit pending")

type replicaStage struct {
	backendType string
	storage     backends.Storage
	stagingPath string
	finalPath   string
}

func (e *Engine) GetFile(ctx context.Context, path string) (io.ReadCloser, error) {
	md, err := e.GetMetadata(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get file metadata: %w", err)
	}

	return e.OpenFile(ctx, md, path)
}

func (e *Engine) OpenFile(ctx context.Context, md *metadata.Metadata, path string) (io.ReadCloser, error) {
	if md.Type != "file" {
		return nil, ErrNotAFile
	}

	if em := e.GetErasureManager(); md.ErasureCoded && em != nil {
		reader, _, err := em.OpenReader(ctx, path)
		if nil != err {
			return nil, fmt.Errorf("failed to retrieve erasure-coded file: %w", err)
		}
		return reader, nil
	}

	ctx, storage := e.selectBackend(ctx, md)

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

func (e *Engine) lockCreateTarget(ctx context.Context, path string, md *metadata.Metadata) (func(), error) {
	if err := e.ensureParentDirectories(ctx, path, md.BackendType, md.Owner); nil != err {
		return nil, fmt.Errorf("failed to ensure parent directories: %w", err)
	}
	if e.atomicNamespace() {
		release, err := e.lockPaths(ctx, path)
		if nil != err {
			return nil, fmt.Errorf("failed to acquire lock: %w", err)
		}
		return release, nil
	}

	parent := filepath.Dir(path)
	release, err := e.lockPaths(ctx, path, parent)
	if nil != err {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	if "/" == parent || "." == parent {
		return release, nil
	}
	if _, err := e.metadataStore.Get(ctx, parent); nil != err {
		release()
		return nil, fmt.Errorf("failed to get parent directory: %w", err)
	}
	return release, nil
}

func (e *Engine) CreateFile(ctx context.Context, path string, reader io.Reader, size int64, md *metadata.Metadata) error {
	release, err := e.lockCreateOnly(ctx, path, md)
	if err != nil {
		return err
	}
	defer release()

	_, err = e.metadataStore.Get(ctx, path)
	switch {
	case err == nil:
		return metadata.ErrAlreadyExists
	case !errors.Is(err, metadata.ErrNotFound):
		return fmt.Errorf("failed to get existing metadata: %w", err)
	}

	_, err = e.createLocked(ctx, path, reader, size, md)
	return err
}

func (e *Engine) PutFile(ctx context.Context, path string, reader io.Reader, size int64, md *metadata.Metadata, ifMatch string, ifNoneMatch bool) (*metadata.Metadata, bool, error) {
	release, err := e.lockCreateTarget(ctx, path, md)
	if nil != err {
		return nil, false, err
	}
	defer release()

	existingMd, err := e.metadataStore.Get(ctx, path)
	if nil != err && !errors.Is(err, metadata.ErrNotFound) {
		return nil, false, fmt.Errorf("failed to get existing metadata: %w", err)
	}
	exists := nil == err

	if ifNoneMatch && exists {
		return nil, false, metadata.ErrPrecondition
	}
	if "" != ifMatch && (!exists || existingMd.ETag != ifMatch) {
		return nil, false, metadata.ErrPrecondition
	}

	if !exists {
		committed, err := e.createLocked(ctx, path, reader, size, md)
		return committed, true, err
	}

	committed, err := e.updateLocked(ctx, path, reader, size, existingMd)
	return committed, false, err
}

func (e *Engine) UpdateFile(ctx context.Context, path string, reader io.Reader, size int64, md *metadata.Metadata) error {
	release, err := e.lockPaths(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer release()

	existingMd, err := e.metadataStore.Get(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to get existing metadata: %w", err)
	}

	_, err = e.updateLocked(ctx, path, reader, size, existingMd)
	return err
}

func withBackendMode(ctx context.Context, backendType, mode string) context.Context {
	if backendType == "s3" {
		return s3.WithMode(ctx, mode)
	}
	return ctx
}

func (e *Engine) createLocked(ctx context.Context, path string, reader io.Reader, size int64, md *metadata.Metadata) (committed *metadata.Metadata, err error) {
	e.logger.Debug("File create started",
		zap.String("path", path),
		zap.String("backend", md.BackendType),
		zap.Int64("size", size))
	defer func() {
		if nil != err {
			e.logger.Error("File create failed", zap.String("path", path), zap.Error(err))
			return
		}
		e.logger.Debug("File created successfully",
			zap.String("path", path),
			zap.String("backend", md.BackendType),
			zap.Int64("size", md.Size))
	}()

	pending, err := e.writePendingFile(ctx, path, reader, size, md)
	if nil != err {
		return nil, err
	}

	if err := e.metadataStore.Create(ctx, md); nil != err {
		e.abortFile(ctx, pending)
		return nil, fmt.Errorf("failed to store metadata: %w", err)
	}

	if err := e.finishFile(ctx, pending); nil != err {
		return nil, err
	}
	return md, nil
}

func (e *Engine) updateLocked(ctx context.Context, path string, reader io.Reader, size int64, existingMd *metadata.Metadata) (committed *metadata.Metadata, err error) {
	start := time.Now()
	defer func() {
		if nil == err && nil != existingMd {
			metrics.FileOperationsTotal.WithLabelValues("update", existingMd.BackendType).Inc()
			metrics.BackendOpDuration.WithLabelValues(existingMd.BackendType, "update").Observe(time.Since(start).Seconds())
		}
	}()

	if "file" != existingMd.Type {
		return nil, ErrNotAFile
	}
	if existingMd.ErasureCoded {
		return nil, fmt.Errorf("%w: erasure-coded file cannot be overwritten in place", metadata.ErrPrecondition)
	}

	backendCtx, storage := e.selectBackend(ctx, existingMd)
	relativePath := strings.TrimPrefix(path, "/")

	e.logger.Debug("File update started",
		zap.String("path", path),
		zap.String("backend", existingMd.BackendType),
		zap.Int64("size", size))
	defer func() {
		if nil != err {
			e.logger.Error("File update failed", zap.String("path", path), zap.Error(err))
			return
		}
		e.logger.Debug("File updated successfully",
			zap.String("path", path),
			zap.String("backend", committed.BackendType),
			zap.Int64("size", committed.Size))
	}()

	newMd := *existingMd
	newMd.MTime = time.Now()
	newMd.UpdatedAt = time.Now()
	if nil == newMd.CallFSInstanceID && "localfs" == newMd.BackendType {
		newMd.CallFSInstanceID = &e.currentInstanceID
	}

	if storage == e.internalProxyBackend {
		counter := &countingReader{reader: reader}
		content, etagOf := hashingReader(counter)
		stage, err := e.writeWithReplica(ctx, path, newMd.BackendType, relativePath, size, content, func(body io.Reader) error {
			return storage.Update(withBackendMode(backendCtx, newMd.BackendType, newMd.Mode), relativePath, body, size)
		})
		if nil != err {
			metrics.BackendOpFailuresTotal.WithLabelValues(newMd.BackendType, "update").Inc()
			e.invalidateFileCache(path)
			return nil, fmt.Errorf("failed to update file in backend: %w", err)
		}

		newMd.Size = counter.count
		newMd.ETag = etagOf()
		if err := e.metadataStore.Update(ctx, &newMd); nil != err {
			e.discardReplicaStage(ctx, path, stage)
			e.logger.Error("Metadata update failed after backend write - inconsistent state",
				zap.String("path", path), zap.Error(err))
			e.metadataCache.Invalidate(path)
			return nil, fmt.Errorf("failed to update metadata: %w", err)
		}

		if err := e.commitReplicaStage(ctx, path, stage); nil != err {
			e.invalidateFileCache(path)
			return &newMd, fmt.Errorf("%w: %w", ErrReplicaCommitPending, err)
		}

		e.scheduleReplication(&newMd, path, storage, relativePath)
		e.invalidateFileCache(path)
		return &newMd, nil
	}

	stagingRelativePath, err := newStagingRelativePath()
	if nil != err {
		return nil, err
	}

	counter := &countingReader{reader: reader}
	content, etagOf := hashingReader(counter)
	stage, err := e.writeWithReplica(ctx, path, newMd.BackendType, relativePath, size, content, func(body io.Reader) error {
		return storage.Create(withBackendMode(backendCtx, newMd.BackendType, newMd.Mode), stagingRelativePath, body, size)
	})
	if nil != err {
		metrics.BackendOpFailuresTotal.WithLabelValues(newMd.BackendType, "update").Inc()
		e.discardBackendObject(backendCtx, storage, path, stagingRelativePath)
		e.invalidateFileCache(path)
		return nil, fmt.Errorf("failed to stage file in backend: %w", err)
	}
	newMd.Size = counter.count
	newMd.ETag = etagOf()
	if err := e.verifyStored(backendCtx, storage, stagingRelativePath, newMd.Size, newMd.ETag); nil != err {
		e.discardBackendObject(backendCtx, storage, path, stagingRelativePath)
		e.discardReplicaStage(ctx, path, stage)
		e.invalidateFileCache(path)
		return nil, err
	}
	oldETag := existingMd.ETag

	restore := func(cause error) error {
		if restoreErr := e.metadataStore.UpdateIfMatch(ctx, existingMd, newMd.ETag); nil != restoreErr {
			return errors.Join(cause, fmt.Errorf("failed to restore metadata: %w", restoreErr))
		}
		return cause
	}

	if err := e.metadataStore.UpdateIfMatch(ctx, &newMd, oldETag); nil != err {
		e.discardBackendObject(backendCtx, storage, path, stagingRelativePath)
		e.discardReplicaStage(ctx, path, stage)
		return nil, fmt.Errorf("failed to update metadata: %w", err)
	}

	if err := e.commitReplicaStage(ctx, path, stage); nil != err {
		e.discardBackendObject(backendCtx, storage, path, stagingRelativePath)
		e.invalidateFileCache(path)
		return nil, restore(err)
	}

	if err := storage.Move(backendCtx, stagingRelativePath, relativePath); nil != err {
		metrics.BackendOpFailuresTotal.WithLabelValues(newMd.BackendType, "update").Inc()
		err = restore(fmt.Errorf("failed to commit file in backend: %w", err))
		e.discardBackendObject(backendCtx, storage, path, stagingRelativePath)
		e.invalidateFileCache(path)
		return nil, err
	}

	e.scheduleReplication(&newMd, path, storage, relativePath)
	e.invalidateFileCache(path)

	return &newMd, nil
}

func (e *Engine) DeleteFile(ctx context.Context, path string) error {
	release, err := e.lockPaths(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer release()

	md, err := e.metadataStore.Get(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to get metadata: %w", err)
	}

	if md.Type == "directory" {
		empty, err := e.directoryIsEmpty(ctx, path)
		if err != nil {
			return fmt.Errorf("failed to check directory contents: %w", err)
		}
		if !empty {
			return ErrDirectoryNotEmpty
		}
	}

	start := time.Now()
	if err := e.deleteLockedResource(ctx, path, md); err != nil {
		if "directory" == md.Type && errors.Is(err, metadata.ErrPrecondition) {
			return ErrDirectoryNotEmpty
		}
		return err
	}
	metrics.FileOperationsTotal.WithLabelValues("delete", md.BackendType).Inc()
	metrics.BackendOpDuration.WithLabelValues(md.BackendType, "delete").Observe(time.Since(start).Seconds())
	return nil
}

func (e *Engine) directoryIsEmpty(ctx context.Context, path string) (bool, error) {
	hasChildren, err := e.metadataStore.HasChildren(ctx, path)
	return !hasChildren, err
}

func (e *Engine) deleteLockedResource(ctx context.Context, path string, md *metadata.Metadata) error {
	staged, err := e.stageDelete(ctx, path, md)
	if nil != err {
		return err
	}
	if err := e.applyMetadataBatch(ctx, staged.ops); nil != err {
		return fmt.Errorf("failed to delete metadata: %w", err)
	}
	return staged.finish()
}

func (e *Engine) CreateErasureFile(ctx context.Context, path string, r io.Reader, opts *erasure.StoreOptions, build func(*erasure.ErasureFileInfo) *metadata.Metadata) (*metadata.Metadata, error) {
	em := e.GetErasureManager()
	if nil == em {
		return nil, errors.New("erasure coding is not enabled")
	}

	lockTargets := []string{path, filepath.Dir(path)}
	if e.atomicNamespace() {
		lockTargets = lockTargets[:1]
	}
	release, err := e.lockPaths(ctx, lockTargets...)
	if nil != err {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer release()

	_, err = e.metadataStore.Get(ctx, path)
	switch {
	case nil == err:
		return nil, metadata.ErrPrecondition
	case !errors.Is(err, metadata.ErrNotFound):
		return nil, fmt.Errorf("failed to get existing metadata: %w", err)
	}

	info, err := em.StoreReader(ctx, path, r, opts)
	if nil != err {
		return nil, fmt.Errorf("failed to store erasure shards: %w", err)
	}

	md := build(info)
	md.Path = path
	md.CreatedAt = time.Now()
	md.UpdatedAt = time.Now()

	plan, err := e.planParentDirectories(ctx, path, "localfs", md.Owner)
	if nil == err {
		err = e.applyWithParents(ctx, plan, []metadata.Op{
			{Kind: metadata.OpCreate, Metadata: md},
			{Kind: metadata.OpCreateErasureInfo, Path: info.FilePath, ErasureInfo: em.ErasureMetadata(info)},
		})
	}
	if nil != err {
		em.DiscardFile(info)
		return nil, fmt.Errorf("failed to store erasure metadata: %w", err)
	}

	e.invalidateFileCache(path)
	return md, nil
}

func (e *Engine) UpdateMetadataOnly(ctx context.Context, md *metadata.Metadata) error {
	if err := e.metadataStore.Update(ctx, md); err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}
	e.metadataCache.Invalidate(md.Path)
	return nil
}

func (e *Engine) DeleteMetadataOnly(ctx context.Context, path string) error {
	if err := e.metadataStore.Delete(ctx, path); err != nil {
		return fmt.Errorf("failed to delete metadata: %w", err)
	}
	e.invalidateFileCache(path)
	return nil
}

func (e *Engine) GetMetadata(ctx context.Context, path string) (*metadata.Metadata, error) {
	md, _, err := e.GetMetadataWithSource(ctx, path)
	return md, err
}

func (e *Engine) GetMetadataWithSource(ctx context.Context, path string) (*metadata.Metadata, bool, error) {
	if cachedMd, found := e.metadataCache.Get(path); found {
		e.logger.Debug("Cache hit for metadata", zap.String("path", path))
		return cachedMd, true, nil
	}
	if e.metadataCache.IsAbsent(path) {
		return nil, false, metadata.ErrNotFound
	}

	gen := e.metadataCache.Generation(path)
	md, err := e.metadataStore.Get(ctx, path)
	if nil != err {
		if errors.Is(err, metadata.ErrNotFound) {
			e.metadataCache.SetAbsent(path)
		}
		return nil, false, err
	}

	e.metadataCache.SetIfGeneration(path, md, gen)
	e.logger.Debug("Cache miss for metadata - stored in cache", zap.String("path", path))

	return md, false, nil
}

func (e *Engine) GetMetadataUncached(ctx context.Context, path string) (*metadata.Metadata, error) {
	gen := e.metadataCache.Generation(path)
	md, err := e.metadataStore.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	e.metadataCache.SetIfGeneration(path, md, gen)
	return md, nil
}

func ETagFor(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

var ErrVerifyMismatch = errors.New("stored file does not match the uploaded bytes")

func (e *Engine) verifyStored(ctx context.Context, storage backends.Storage, relativePath string, size int64, etag string) error {
	if !e.strict || storage == e.internalProxyBackend {
		return nil
	}
	stored, err := storage.Open(ctx, relativePath)
	if nil != err {
		return fmt.Errorf("failed to verify stored file: %w", err)
	}
	counter := &countingReader{reader: stored}
	content, etagOf := hashingReader(counter)
	_, copyErr := io.Copy(io.Discard, content)
	closeErr := stored.Close()
	if err := errors.Join(copyErr, closeErr); nil != err {
		return fmt.Errorf("failed to verify stored file: %w", err)
	}
	if size != counter.count || etag != etagOf() {
		return fmt.Errorf("%w: %s", ErrVerifyMismatch, relativePath)
	}
	return nil
}

func hashingReader(reader io.Reader) (io.Reader, func() string) {
	h := sha256.New()
	return io.TeeReader(reader, h), func() string {
		return hex.EncodeToString(h.Sum(nil))
	}
}

type countingReader struct {
	reader io.Reader
	count  int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.count += int64(n)
	return n, err
}

func newStagingRelativePath() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate staging name: %w", err)
	}
	return stagingDirName + "/" + hex.EncodeToString(b), nil
}

func (e *Engine) discardBackendObject(ctx context.Context, storage backends.Storage, path, relativePath string) {
	if err := storage.Delete(ctx, relativePath); err != nil {
		e.logger.Error("Failed to remove backend object",
			zap.String("path", path), zap.String("object", relativePath), zap.Error(err))
	}
}

func (e *Engine) invalidateFileCache(path string) {
	e.metadataCache.Invalidate(path)
	e.metadataCache.Invalidate(filepath.Dir(path))
}

func (e *Engine) replicaBackendFor(primaryBackend string) string {
	if !e.replicationEnabled {
		return ""
	}
	replicaBackend := strings.ToLower(strings.TrimSpace(e.replicaBackend))
	if replicaBackend == "" || replicaBackend == strings.ToLower(primaryBackend) {
		return ""
	}
	return replicaBackend
}

func (e *Engine) writeWithReplica(ctx context.Context, path, primaryBackend, finalPath string, size int64, content io.Reader, writePrimary func(io.Reader) error) (*replicaStage, error) {
	replicaBackend := e.replicaBackendFor(primaryBackend)
	if "" == replicaBackend || !e.requireReplicaAck {
		return nil, writePrimary(content)
	}

	replicaStorage := e.selectBackendByType(replicaBackend)
	stagingPath, err := newStagingRelativePath()
	if nil != err {
		return nil, err
	}

	pipeReader, pipeWriter := io.Pipe()
	replicaDone := make(chan error, 1)
	go func() {
		err := replicaStorage.Create(ctx, stagingPath, pipeReader, size)
		if _, drainErr := io.Copy(io.Discard, pipeReader); nil == err {
			err = drainErr
		}
		replicaDone <- err
	}()

	primaryErr := writePrimary(io.TeeReader(content, pipeWriter))
	pipeWriter.CloseWithError(primaryErr)
	replicaErr := <-replicaDone

	if nil != primaryErr {
		if nil == replicaErr {
			e.discardBackendObject(ctx, replicaStorage, path, stagingPath)
		}
		return nil, primaryErr
	}
	if nil != replicaErr {
		metrics.ReplicationTotal.WithLabelValues("failed").Inc()
		return nil, fmt.Errorf("failed to replicate file to secondary backend: %w", replicaErr)
	}

	return &replicaStage{
		backendType: replicaBackend,
		storage:     replicaStorage,
		stagingPath: stagingPath,
		finalPath:   finalPath,
	}, nil
}

func (e *Engine) commitReplicaStage(ctx context.Context, path string, stage *replicaStage) error {
	if nil == stage {
		return nil
	}
	if err := stage.storage.Move(ctx, stage.stagingPath, stage.finalPath); nil != err {
		e.discardBackendObject(ctx, stage.storage, path, stage.stagingPath)
		metrics.ReplicationTotal.WithLabelValues("failed").Inc()
		return fmt.Errorf("failed to replicate file to secondary backend: %w", err)
	}
	metrics.ReplicationTotal.WithLabelValues("succeeded").Inc()
	e.logger.Debug("Replicated file to secondary backend",
		zap.String("path", path),
		zap.String("replica_backend", stage.backendType))
	return nil
}

func (e *Engine) discardReplicaStage(ctx context.Context, path string, stage *replicaStage) {
	if stage != nil {
		e.discardBackendObject(ctx, stage.storage, path, stage.stagingPath)
	}
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

type replicaJob struct {
	path           string
	primaryBackend string
	mode           string
	replicaBackend string
	source         backends.Storage
	relativePath   string
	size           int64
}

func (e *Engine) scheduleReplication(md *metadata.Metadata, path string, source backends.Storage, relativePath string) {
	replicaBackend := e.replicaBackendFor(md.BackendType)
	if "" == replicaBackend || e.requireReplicaAck {
		return
	}
	job := replicaJob{
		path:           path,
		primaryBackend: md.BackendType,
		mode:           md.Mode,
		replicaBackend: replicaBackend,
		source:         source,
		relativePath:   relativePath,
		size:           md.Size,
	}
	select {
	case e.replicaQueue <- job:
	default:
		metrics.ReplicationTotal.WithLabelValues("dropped").Inc()
		e.logger.Warn("Replication queue full, file left unreplicated",
			zap.String("path", path),
			zap.String("replica_backend", replicaBackend))
	}
}

func (e *Engine) replicaWorker() {
	for {
		select {
		case <-e.replicaCtx.Done():
			return
		case job := <-e.replicaQueue:
			e.runReplicaJob(job)
		}
	}
}

func (e *Engine) runReplicaJob(job replicaJob) {
	e.logger.Info("Replicating file to secondary backend started",
		zap.String("path", job.path),
		zap.String("replica_backend", job.replicaBackend),
		zap.Int64("size", job.size))
	if err := e.replicateObject(job); nil != err {
		metrics.ReplicationTotal.WithLabelValues("failed").Inc()
		e.logger.Warn("Replication to secondary backend failed",
			zap.String("path", job.path),
			zap.String("replica_backend", job.replicaBackend),
			zap.Error(err))
		return
	}
	metrics.ReplicationTotal.WithLabelValues("succeeded").Inc()
	e.logger.Info("Replicated file to secondary backend",
		zap.String("path", job.path),
		zap.String("replica_backend", job.replicaBackend))
}

func (e *Engine) replicateObject(job replicaJob) error {
	ctx := withBackendMode(e.replicaCtx, job.primaryBackend, job.mode)
	source, err := job.source.Open(ctx, job.relativePath)
	if nil != err {
		return fmt.Errorf("failed to open primary object: %w", err)
	}

	replicaStorage := e.selectBackendByType(job.replicaBackend)
	stagingPath, err := newStagingRelativePath()
	if nil != err {
		return errors.Join(err, source.Close())
	}
	if err := errors.Join(replicaStorage.Create(e.replicaCtx, stagingPath, source, job.size), source.Close()); nil != err {
		e.discardBackendObject(e.replicaCtx, replicaStorage, job.path, stagingPath)
		return fmt.Errorf("failed to write replica object: %w", err)
	}

	if err := replicaStorage.Move(e.replicaCtx, stagingPath, job.relativePath); nil != err {
		e.discardBackendObject(e.replicaCtx, replicaStorage, job.path, stagingPath)
		return fmt.Errorf("failed to commit replica object: %w", err)
	}
	return nil
}
