// Package backends provides storage backend adapters and interfaces for CallFS.
// It includes implementations for local filesystem, S3 object storage, and others.
package backends

import (
	"context"
	"io"

	"github.com/ebogdum/callfs/metadata"
)

// Storage defines the interface for backend storage operations
// This interface abstracts file operations across different storage backends
type Storage interface {
	// Open opens a file for reading and returns a ReadCloser
	Open(ctx context.Context, path string) (io.ReadCloser, error)

	// Create creates a new file with content from the reader
	Create(ctx context.Context, path string, reader io.Reader, size int64) error

	// Update updates an existing file with new content from the reader
	Update(ctx context.Context, path string, reader io.Reader, size int64) error

	// Delete removes a file or empty directory
	Delete(ctx context.Context, path string) error

	// Stat returns metadata for a file or directory
	Stat(ctx context.Context, path string) (*metadata.Metadata, error)

	// ListDirectory returns metadata for all children of a directory
	ListDirectory(ctx context.Context, path string) ([]*metadata.Metadata, error)

	// CreateDirectory creates a new directory
	CreateDirectory(ctx context.Context, path string) error

	// Close closes any resources used by the storage backend
	Close() error
}
