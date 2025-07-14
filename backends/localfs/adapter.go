package localfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ebogdum/callfs/internal/pathutil"
	"github.com/ebogdum/callfs/metadata"
)

// LocalFSAdapter implements the backends.Storage interface for local filesystem
type LocalFSAdapter struct {
	rootPath string
}

// NewLocalFSAdapter creates a new local filesystem adapter
func NewLocalFSAdapter(rootPath string) (*LocalFSAdapter, error) {
	// Ensure root path exists
	if err := os.MkdirAll(rootPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create root path %s: %w", rootPath, err)
	}

	// Verify path is accessible
	if _, err := os.Stat(rootPath); err != nil {
		return nil, fmt.Errorf("root path %s is not accessible: %w", rootPath, err)
	}

	return &LocalFSAdapter{
		rootPath: rootPath,
	}, nil
}

// Open opens a file for reading
func (a *LocalFSAdapter) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	fullPath, err := pathutil.SafeJoin(a.rootPath, path)
	if err != nil {
		return nil, metadata.ErrForbidden
	}

	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to open file %s: %w", path, err)
	}

	return file, nil
}

// Create creates a new file with content from the reader
func (a *LocalFSAdapter) Create(ctx context.Context, path string, reader io.Reader, size int64) error {
	fullPath, err := pathutil.SafeJoin(a.rootPath, path)
	if err != nil {
		return metadata.ErrForbidden
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Create file with exclusive flag
	file, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			return metadata.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}
	defer file.Close()

	// Copy content from reader
	_, err = io.Copy(file, reader)
	if err != nil {
		// Clean up partially created file
		os.Remove(fullPath)
		return fmt.Errorf("failed to write file content: %w", err)
	}

	return nil
}

// Update updates an existing file with new content from the reader
func (a *LocalFSAdapter) Update(ctx context.Context, path string, reader io.Reader, size int64) error {
	fullPath, err := pathutil.SafeJoin(a.rootPath, path)
	if err != nil {
		return metadata.ErrForbidden
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Create or truncate file
	file, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file for update %s: %w", path, err)
	}
	defer file.Close()

	// Copy content from reader
	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to write file content: %w", err)
	}

	return nil
}

// Delete removes a file or empty directory
func (a *LocalFSAdapter) Delete(ctx context.Context, path string) error {
	fullPath, err := pathutil.SafeJoin(a.rootPath, path)
	if err != nil {
		return metadata.ErrForbidden
	}

	err = os.Remove(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return metadata.ErrNotFound
		}
		return fmt.Errorf("failed to delete %s: %w", path, err)
	}

	return nil
}

// Stat returns metadata for a file or directory
func (a *LocalFSAdapter) Stat(ctx context.Context, path string) (*metadata.Metadata, error) {
	fullPath, err := pathutil.SafeJoin(a.rootPath, path)
	if err != nil {
		return nil, metadata.ErrForbidden
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to stat %s: %w", path, err)
	}

	md := &metadata.Metadata{
		Name:        info.Name(),
		Path:        path,
		Size:        info.Size(),
		MTime:       info.ModTime(),
		ATime:       info.ModTime(), // Use mtime as approximation
		CTime:       info.ModTime(), // Use mtime as approximation
		BackendType: "localfs",
	}

	// Determine type and extract platform-specific metadata
	if info.IsDir() {
		md.Type = "directory"
	} else {
		md.Type = "file"
	}

	// Extract platform-specific metadata (permissions, ownership, timestamps)
	md.Mode, md.UID, md.GID, md.ATime, md.CTime = extractUnixMetadata(info)

	return md, nil
}

// ListDirectory returns metadata for all children of a directory
func (a *LocalFSAdapter) ListDirectory(ctx context.Context, path string) ([]*metadata.Metadata, error) {
	fullPath, err := pathutil.SafeJoin(a.rootPath, path)
	if err != nil {
		return nil, metadata.ErrForbidden
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to read directory %s: %w", path, err)
	}

	var children []*metadata.Metadata
	for _, entry := range entries {
		childPath := filepath.Join(path, entry.Name())
		childMd, err := a.Stat(ctx, childPath)
		if err != nil {
			// Log error but continue with other entries
			continue
		}
		children = append(children, childMd)
	}

	return children, nil
}

// CreateDirectory creates a new directory
func (a *LocalFSAdapter) CreateDirectory(ctx context.Context, path string) error {
	fullPath, err := pathutil.SafeJoin(a.rootPath, path)
	if err != nil {
		return metadata.ErrForbidden
	}

	// Check if path already exists as a file
	if info, err := os.Stat(fullPath); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("path exists as file, not directory")
		}
		// Directory already exists - this is not an error for CreateDirectory
		return nil
	}

	err = os.MkdirAll(fullPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	return nil
}

// Close closes any resources used by the storage backend
func (a *LocalFSAdapter) Close() error {
	// No resources to close for local filesystem
	return nil
}
