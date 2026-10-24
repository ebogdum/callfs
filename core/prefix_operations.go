package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ebogdum/callfs/v2/metadata"
)

const (
	maxListObjects   = 1000
	deletePageLength = 1000
)

type DeletePlan struct {
	Files           int      `json:"files"`
	Directories     int      `json:"directories"`
	Bytes           int64    `json:"bytes"`
	ErasureFiles    int      `json:"erasure_files"`
	RemoteInstances []string `json:"remote_instances"`
}

type PrefixDeleteError struct {
	Deleted    int
	FailedPath string
	Err        error
}

func (e *PrefixDeleteError) Error() string {
	return fmt.Sprintf("failed to delete %s: %v", e.FailedPath, e.Err)
}

func (e *PrefixDeleteError) Unwrap() error {
	return e.Err
}

func WithinPrefixBoundary(path, prefix string) bool {
	return strings.HasPrefix(path, strings.TrimSuffix(prefix, "/")+"/")
}

func (e *Engine) ListObjects(ctx context.Context, prefix, startAfter string, limit int) ([]*metadata.Metadata, bool, error) {
	switch {
	case limit == 0:
		limit = maxListObjects
	case limit < 0:
		limit = 1
	case limit > maxListObjects:
		limit = maxListObjects
	}

	items, err := e.metadataStore.ListByPrefix(ctx, prefix, startAfter, limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("failed to list objects: %w", err)
	}
	if len(items) > limit {
		return items[:limit], true, nil
	}
	return items, false, nil
}

func (e *Engine) WalkPrefix(ctx context.Context, prefix string, boundary bool, visit func(page []*metadata.Metadata) (bool, error)) error {
	startAfter := ""
	for {
		items, err := e.metadataStore.ListByPrefix(ctx, prefix, startAfter, deletePageLength)
		if nil != err {
			return fmt.Errorf("failed to list files under %s: %w", prefix, err)
		}

		page := items
		if boundary {
			page = filterWithinPrefix(items, prefix)
		}
		again, err := visit(page)
		if nil != err {
			return err
		}
		if !again || len(items) < deletePageLength {
			return nil
		}
		startAfter = items[len(items)-1].Path
	}
}

func filterWithinPrefix(items []*metadata.Metadata, prefix string) []*metadata.Metadata {
	kept := make([]*metadata.Metadata, 0, len(items))
	for _, item := range items {
		if WithinPrefixBoundary(item.Path, prefix) {
			kept = append(kept, item)
		}
	}
	return kept
}

func (e *Engine) DeletePrefix(ctx context.Context, prefix string) (int, error) {
	return e.DeletePrefixWithin(ctx, prefix, false)
}

func (e *Engine) DeletePrefixWithin(ctx context.Context, prefix string, boundary bool) (int, error) {
	if "" == prefix || "/" == prefix {
		return 0, fmt.Errorf("refusing to delete prefix %q", prefix)
	}

	deleted := 0
	err := e.WalkPrefix(ctx, prefix, boundary, func(page []*metadata.Metadata) (bool, error) {
		for _, item := range page {
			if err := e.DeleteFile(ctx, item.Path); nil != err {
				return false, &PrefixDeleteError{Deleted: deleted, FailedPath: item.Path, Err: err}
			}
			deleted++
		}
		return true, nil
	})
	return deleted, err
}

func (e *Engine) PlanDelete(ctx context.Context, path string) (DeletePlan, error) {
	path = normalizeMovePath(path)
	md, err := e.GetMetadata(ctx, path)
	if nil != err {
		return DeletePlan{}, fmt.Errorf("failed to read %s: %w", path, err)
	}

	plan := DeletePlan{RemoteInstances: []string{}}
	current := e.GetCurrentInstanceID()
	remote := make(map[string]struct{})
	addItem := func(item *metadata.Metadata) {
		if "directory" == item.Type {
			plan.Directories++
		} else {
			plan.Files++
			plan.Bytes += item.Size
			if item.ErasureCoded {
				plan.ErasureFiles++
			}
		}
		if nil != item.CallFSInstanceID && "" != *item.CallFSInstanceID && current != *item.CallFSInstanceID {
			remote[*item.CallFSInstanceID] = struct{}{}
		}
	}

	addItem(md)
	if "directory" == md.Type {
		descendants, err := e.metadataStore.ListDescendants(ctx, path, 0, 0)
		if nil != err {
			return DeletePlan{}, fmt.Errorf("failed to list directory %s: %w", path, err)
		}
		for _, item := range descendants {
			addItem(item)
		}
	}

	for instanceID := range remote {
		plan.RemoteInstances = append(plan.RemoteInstances, instanceID)
	}
	sort.Strings(plan.RemoteInstances)
	return plan, nil
}

func (e *Engine) DeleteDirectoryRecursive(ctx context.Context, path string) (int, int, error) {
	path = normalizeMovePath(path)
	if path == "/" {
		return 0, 0, fmt.Errorf("refusing to delete root directory")
	}

	targets, err := e.treeDeleteTargets(ctx, path)
	if nil != err {
		return 0, 0, err
	}

	outcome := e.deleteTargets(ctx, targets)
	if nil != outcome.err {
		return outcome.files, outcome.dirs, deleteTargetError(outcome)
	}
	return outcome.files, outcome.dirs, nil
}

func deleteTargetError(outcome deleteOutcome) error {
	if outcome.failed.dir {
		return fmt.Errorf("failed to delete directory %s: %w", outcome.failed.path, outcome.err)
	}
	return fmt.Errorf("failed to delete %s: %w", outcome.failed.path, outcome.err)
}
