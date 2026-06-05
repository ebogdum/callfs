package localfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/ebogdum/callfs/internal/pathutil"
	"github.com/ebogdum/callfs/metadata"
)

// isCrossDevice reports whether err is an EXDEV ("invalid cross-device link")
// error, which os.Rename returns when source and destination live on different
// filesystems. syscall.EXDEV is defined on every platform Go supports.
func isCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

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

	// Write to a temp file, fsync, then rename for atomicity
	tmpFile, err := os.CreateTemp(filepath.Dir(fullPath), ".callfs-tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	_, copyErr := io.Copy(tmpFile, reader)
	if copyErr != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write file content: %w", copyErr)
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to fsync file: %w", err)
	}
	tmpFile.Close()

	if err := os.Chmod(tmpPath, 0644); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	// Atomic exclusive create: use os.Link to hard-link the temp file to the
	// destination. Link fails with EEXIST if the destination already exists,
	// providing an atomic O_EXCL equivalent without a TOCTOU window.
	if err := os.Link(tmpPath, fullPath); err != nil {
		os.Remove(tmpPath)
		if os.IsExist(err) {
			return metadata.ErrAlreadyExists
		}
		return fmt.Errorf("failed to link temp file to destination: %w", err)
	}

	// Remove the temp file (the data now lives at fullPath via the hard link)
	os.Remove(tmpPath)

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

	// Write to temp file, fsync, then atomic rename to avoid data loss
	tmpFile, err := os.CreateTemp(filepath.Dir(fullPath), ".callfs-tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file for update: %w", err)
	}
	tmpPath := tmpFile.Name()

	_, copyErr := io.Copy(tmpFile, reader)
	if copyErr != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write file content: %w", copyErr)
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to fsync file: %w", err)
	}
	tmpFile.Close()

	if err := os.Chmod(tmpPath, 0644); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	if err := os.Rename(tmpPath, fullPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
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

// Move relocates a file or directory subtree from oldPath to newPath.
// os.Rename is atomic on the same filesystem and moves a whole directory tree
// in one call. If the destination lives on a different device (EXDEV), it falls
// back to a recursive copy followed by removal of the source.
func (a *LocalFSAdapter) Move(ctx context.Context, oldPath, newPath string) error {
	srcFull, err := pathutil.SafeJoin(a.rootPath, oldPath)
	if err != nil {
		return metadata.ErrForbidden
	}
	dstFull, err := pathutil.SafeJoin(a.rootPath, newPath)
	if err != nil {
		return metadata.ErrForbidden
	}

	if err := os.MkdirAll(filepath.Dir(dstFull), 0755); err != nil {
		return fmt.Errorf("failed to create destination parent directory: %w", err)
	}

	if err := os.Rename(srcFull, dstFull); err != nil {
		if os.IsNotExist(err) {
			return metadata.ErrNotFound
		}
		if isCrossDevice(err) {
			if copyErr := copyPath(srcFull, dstFull); copyErr != nil {
				return fmt.Errorf("failed to copy across devices: %w", copyErr)
			}
			if rmErr := os.RemoveAll(srcFull); rmErr != nil {
				return fmt.Errorf("failed to remove source after cross-device copy: %w", rmErr)
			}
			return nil
		}
		return fmt.Errorf("failed to move %s to %s: %w", oldPath, newPath, err)
	}

	return nil
}

// copyPath recursively copies a file or directory tree from src to dst.
func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyPath(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
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

	// Extract platform-specific timestamps and mode from the filesystem.
	// Ownership is tracked via Metadata.Owner — no OS-level UIDs involved.
	md.Mode, md.ATime, md.CTime = extractPlatformMetadata(info)

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
