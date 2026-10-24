package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/cluster"
	"github.com/ebogdum/callfs/v2/metadata"
)

const (
	replicaPool             = "replicate"
	replicaEnqueueTimeout   = 5 * time.Second
	replicaProgressInterval = 30 * time.Second
)

type replicaTask struct {
	Path           string `json:"path"`
	PrimaryBackend string `json:"primary_backend"`
	ReplicaBackend string `json:"replica_backend"`
	ETag           string `json:"etag"`
}

func (e *Engine) UseTaskQueue(queue cluster.Queue) {
	if nil == queue || nil != e.taskQueue {
		return
	}
	e.taskQueue = queue
	e.stopTasks = queue.Consume(replicaPool, replicaWorkerCount, e.handleReplicaTask)
}

func (e *Engine) enqueueReplica(md *metadata.Metadata, path, replicaBackend string) {
	task := replicaTask{Path: path, PrimaryBackend: md.BackendType, ReplicaBackend: replicaBackend, ETag: md.ETag}
	ctx, cancel := context.WithTimeout(e.replicaCtx, replicaEnqueueTimeout)
	defer cancel()
	if err := e.taskQueue.Enqueue(ctx, replicaPool, e.currentInstanceID, path+":"+md.ETag, task); nil != err {
		e.countReplication("dropped")
		if record := e.replicationFailures.Load(); nil != record {
			(*record)(path, err.Error())
		}
		e.logger.Warn("Replication task was not queued, file left unreplicated",
			zap.String("path", path),
			zap.String("replica_backend", replicaBackend),
			zap.Error(err))
	}
}

func (e *Engine) handleReplicaTask(ctx context.Context, t *cluster.Task) error {
	var task replicaTask
	if err := t.Decode(&task); nil != err {
		return cluster.Permanent(fmt.Errorf("decode replication task %s: %w", t.ID, err))
	}
	md, err := e.metadataStore.Get(ctx, task.Path)
	if errors.Is(err, metadata.ErrNotFound) {
		e.countReplication("skipped")
		e.logger.Info("Replication skipped, file is gone", zap.String("path", task.Path))
		return nil
	}
	if nil != err {
		return fmt.Errorf("read metadata for %s: %w", task.Path, err)
	}
	if "file" != md.Type || task.PrimaryBackend != md.BackendType || task.ETag != md.ETag {
		e.countReplication("skipped")
		e.logger.Info("Replication skipped, file changed since it was queued", zap.String("path", task.Path))
		return nil
	}
	if task.ReplicaBackend != e.replicaBackendFor(md.BackendType) {
		e.countReplication("skipped")
		e.logger.Info("Replication skipped, replica backend changed", zap.String("path", task.Path))
		return nil
	}
	_, source := e.selectBackend(ctx, md)
	job := replicaJob{
		path:           task.Path,
		primaryBackend: md.BackendType,
		mode:           md.Mode,
		replicaBackend: task.ReplicaBackend,
		source:         source,
		relativePath:   strings.TrimPrefix(task.Path, "/"),
		size:           md.Size,
	}
	done := make(chan struct{})
	defer close(done)
	go e.reportReplicaProgress(t, done)
	return e.runReplicaJob(job)
}

func (e *Engine) reportReplicaProgress(t *cluster.Task, done <-chan struct{}) {
	ticker := time.NewTicker(replicaProgressInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := t.InProgress(); nil != err {
				e.logger.Warn("Replication task progress was not recorded", zap.String("path", t.ID), zap.Error(err))
			}
		}
	}
}
