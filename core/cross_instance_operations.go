package core

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ebogdum/callfs/backends/internalproxy"
	"github.com/ebogdum/callfs/metadata"
)

// UpdateFileOnInstance updates a file on a specific instance using the internal proxy
func (e *Engine) UpdateFileOnInstance(ctx context.Context, instanceID, path string, reader io.Reader, size int64) error {
	// Use internal proxy with instance ID context
	ctx = internalproxy.WithInstanceID(ctx, instanceID)

	// Convert absolute path to relative path for backend
	relativePath := strings.TrimPrefix(path, "/")

	// Use the internal proxy backend to update the file
	err := e.internalProxyBackend.Update(ctx, relativePath, reader, size)
	if err == nil {
		// Invalidate local cache since remote state changed
		e.metadataCache.Invalidate(path)
	}
	return err
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

// StatFileOnInstance gets file metadata from a specific instance using the internal proxy
func (e *Engine) StatFileOnInstance(ctx context.Context, instanceID, path string) (*metadata.Metadata, error) {
	if e.internalProxyAdapter == nil {
		return nil, fmt.Errorf("internal proxy not configured: no peer endpoints available")
	}

	relativePath := strings.TrimPrefix(path, "/")
	return e.internalProxyAdapter.StatOnInstance(ctx, instanceID, relativePath)
}
