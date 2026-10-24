package core

import (
	"context"
	"fmt"
	"sort"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/locks"
	"github.com/ebogdum/callfs/metadata"
)

func pathLockKey(path string) string {
	return "path:" + path
}

func (e *Engine) lockPaths(ctx context.Context, paths ...string) (func(), error) {
	keys := make([]string, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		k := pathLockKey(p)
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	leases := make([]locks.Lease, 0, len(keys))
	release := func() {
		for i := len(leases) - 1; i >= 0; i-- {
			if err := leases[i].Release(context.Background()); err != nil {
				e.logger.Error("Failed to release lock", zap.String("lock_key", keys[i]), zap.Error(err))
			}
		}
	}
	for _, k := range keys {
		lease, err := e.lockManager.Acquire(ctx, k)
		if err != nil {
			release()
			return nil, err
		}
		leases = append(leases, lease)
	}
	return release, nil
}

func (e *Engine) atomicNamespace() bool {
	atomic, ok := e.metadataStore.(metadata.AtomicNamespace)
	return !e.strict && ok && atomic.AtomicNamespace()
}

func (e *Engine) lockCreateOnly(ctx context.Context, path string, md *metadata.Metadata) (func(), error) {
	if e.atomicNamespace() && "localfs" == md.BackendType {
		if err := e.ensureParentDirectories(ctx, path, md.BackendType, md.Owner); nil != err {
			return nil, fmt.Errorf("failed to ensure parent directories: %w", err)
		}
		return func() {}, nil
	}
	return e.lockCreateTarget(ctx, path, md)
}
