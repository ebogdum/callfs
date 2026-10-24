package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrReplicationDisabled = errors.New("replication is disabled")

func (e *Engine) ReplicatePath(ctx context.Context, path string) error {
	if !e.replicationEnabled {
		return ErrReplicationDisabled
	}

	md, err := e.GetMetadata(ctx, path)
	if nil != err {
		return fmt.Errorf("failed to get file metadata: %w", err)
	}
	if "file" != md.Type {
		return ErrNotAFile
	}

	replicaBackend := e.replicaBackendFor(md.BackendType)
	if "" == replicaBackend {
		return ErrReplicationDisabled
	}

	_, source := e.selectBackend(ctx, md)
	job := replicaJob{
		path:           path,
		primaryBackend: md.BackendType,
		mode:           md.Mode,
		replicaBackend: replicaBackend,
		source:         source,
		relativePath:   strings.TrimPrefix(path, "/"),
		size:           md.Size,
	}
	if err := e.replicateObject(job); nil != err {
		return fmt.Errorf("failed to replicate file: %w", err)
	}
	return nil
}
