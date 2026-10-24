package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ebogdum/callfs/v2/backends/internalproxy"
)

// UpdateFileOnInstance updates a file on a specific instance using the internal proxy
func (e *Engine) UpdateFileOnInstance(ctx context.Context, instanceID, path string, reader io.Reader, size int64, ifMatch string, ifNoneMatch bool) (string, error) {
	if e.internalProxyAdapter == nil {
		return "", fmt.Errorf("internal proxy not configured: no peer endpoints available")
	}

	relativePath := strings.TrimPrefix(path, "/")
	etag, err := e.internalProxyAdapter.UpdateOnInstance(ctx, instanceID, relativePath, reader, size, ifMatch, ifNoneMatch)
	if err != nil {
		return "", err
	}
	e.metadataCache.Invalidate(path)
	return etag, nil
}

// DeleteFileOnInstance deletes a file on a specific instance using the internal proxy
func (e *Engine) DeleteFileOnInstance(ctx context.Context, instanceID, path string) error {
	if e.internalProxyAdapter == nil {
		return fmt.Errorf("internal proxy not configured: no peer endpoints available")
	}

	relativePath := strings.TrimPrefix(path, "/")
	err := e.internalProxyAdapter.DeleteOnInstance(ctx, instanceID, relativePath)
	if err == nil {
		// Invalidate local cache since remote state changed
		e.metadataCache.Invalidate(path)
	}
	return err
}

func (e *Engine) DeleteTreeOnInstance(ctx context.Context, instanceID, path string) error {
	if nil == e.internalProxyAdapter {
		return fmt.Errorf("internal proxy not configured: no peer endpoints available")
	}
	err := e.internalProxyAdapter.DeleteTreeOnInstance(ctx, instanceID, strings.TrimPrefix(path, "/"))
	if errors.Is(err, internalproxy.ErrRemoteConflict) {
		return ErrDirectoryNotEmpty
	}
	if nil == err {
		_ = e.metadataCache.InvalidatePrefix(path)
	}
	return err
}
