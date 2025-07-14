package core

import (
	"context"
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
	return e.internalProxyBackend.Update(ctx, relativePath, reader, size)
}

// DeleteFileOnInstance deletes a file on a specific instance using the internal proxy
func (e *Engine) DeleteFileOnInstance(ctx context.Context, instanceID, path string) error {
	// Convert absolute path to relative path for backend
	relativePath := strings.TrimPrefix(path, "/")

	// Use the internal proxy adapter to delete the file
	return e.internalProxyAdapter.DeleteOnInstance(ctx, instanceID, relativePath)
}

// StatFileOnInstance gets file metadata from a specific instance using the internal proxy
func (e *Engine) StatFileOnInstance(ctx context.Context, instanceID, path string) (*metadata.Metadata, error) {
	// Convert absolute path to relative path for backend
	relativePath := strings.TrimPrefix(path, "/")

	// Use the internal proxy adapter to get file metadata
	return e.internalProxyAdapter.StatOnInstance(ctx, instanceID, relativePath)
}
