package noop

import (
	"context"
	"fmt"
	"io"

	"github.com/ebogdum/callfs/backends"
	"github.com/ebogdum/callfs/metadata"
)

// NoopAdapter is a no-operation storage backend that always returns errors
// This is used when a backend is not configured/enabled
type NoopAdapter struct{}

// NewNoopAdapter creates a new noop storage adapter
func NewNoopAdapter() backends.Storage {
	return &NoopAdapter{}
}

// Open always returns an error for noop backend
func (n *NoopAdapter) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("backend not enabled: cannot open file %s", path)
}

// Create always returns an error for noop backend
func (n *NoopAdapter) Create(ctx context.Context, path string, reader io.Reader, size int64) error {
	return fmt.Errorf("backend not enabled: cannot create file %s", path)
}

// Update always returns an error for noop backend
func (n *NoopAdapter) Update(ctx context.Context, path string, reader io.Reader, size int64) error {
	return fmt.Errorf("backend not enabled: cannot update file %s", path)
}

// Delete always returns an error for noop backend
func (n *NoopAdapter) Delete(ctx context.Context, path string) error {
	return fmt.Errorf("backend not enabled: cannot delete file %s", path)
}

// Stat always returns an error for noop backend
func (n *NoopAdapter) Stat(ctx context.Context, path string) (*metadata.Metadata, error) {
	return nil, fmt.Errorf("backend not enabled: cannot stat file %s", path)
}

// ListDirectory always returns an error for noop backend
func (n *NoopAdapter) ListDirectory(ctx context.Context, path string) ([]*metadata.Metadata, error) {
	return nil, fmt.Errorf("backend not enabled: cannot list directory %s", path)
}

// CreateDirectory always returns an error for noop backend
func (n *NoopAdapter) CreateDirectory(ctx context.Context, path string) error {
	return fmt.Errorf("backend not enabled: cannot create directory %s", path)
}

// Close does nothing for noop backend
func (n *NoopAdapter) Close() error {
	return nil
}
