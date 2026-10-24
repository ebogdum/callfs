package localfs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/ebogdum/callfs/v2/backends"
	"github.com/ebogdum/callfs/v2/internal/pathutil"
	"github.com/ebogdum/callfs/v2/metadata"
)

// isCrossDevice reports whether err is an EXDEV ("invalid cross-device link")
// error, which os.Rename returns when source and destination live on different
// filesystems. syscall.EXDEV is defined on every platform Go supports.
func isCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

var ErrInsufficientStorage = errors.New("insufficient storage space")

const tempFilePrefix = ".callfs-tmp-"

func ensureFreeSpace(rootPath string, size int64) error {
	if 0 >= size {
		return nil
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(rootPath, &stat); err != nil {
		return fmt.Errorf("failed to check free space: %w", err)
	}
	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	if uint64(size) > available {
		return ErrInsufficientStorage
	}
	return nil
}

// LocalFSAdapter implements the backends.Storage interface for local filesystem
type LocalFSAdapter struct {
	rootPath string
	root     *os.Root
	syncMode SyncMode
	syncer   *dirSyncer
	subRoots *subRootCache
	flusher  *flusher
	ack      backends.WriteAck
	flushCfg FlushConfig
	space    freeSpaceCache
}

// NewLocalFSAdapter creates a new local filesystem adapter
func NewLocalFSAdapter(rootPath string, opts ...Option) (*LocalFSAdapter, error) {
	adapter := &LocalFSAdapter{rootPath: rootPath, syncMode: SyncFull}
	for _, opt := range opts {
		opt(adapter)
	}
	mode, err := ParseSyncMode(string(adapter.syncMode))
	if nil != err {
		return nil, err
	}

	if err := os.MkdirAll(rootPath, 0700); err != nil {
		return nil, fmt.Errorf("failed to create root path %s: %w", rootPath, err)
	}

	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, fmt.Errorf("root path %s is not accessible: %w", rootPath, err)
	}

	adapter.root = root
	adapter.subRoots = newSubRootCache(root)
	adapter.syncMode = mode
	adapter.syncer = newDirSyncer(mode, groupCommitWindow, groupCommitMaxBatch, func(dirRel string) error {
		return syncDirectory(root, dirRel, mode)
	})
	if "" == adapter.ack {
		adapter.ack = backends.WriteAckDisk
	}
	if SyncNone != mode {
		adapter.flusher = newFlusher(mode, adapter.flushCfg.Workers, adapter.flushCfg.MaxPendingBytes, adapter.flushCfg.MaxDelay, func(dirRel string) (*os.File, error) {
			return root.Open(dirRel)
		})
	}
	return adapter, nil
}

func relPath(path string) (string, error) {
	cleaned, err := pathutil.Clean(path)
	if err != nil {
		return "", metadata.ErrForbidden
	}
	rel := strings.TrimPrefix(cleaned, "/")
	if "" == rel {
		return ".", nil
	}
	return rel, nil
}

func isEscape(err error) bool {
	return nil != err && strings.Contains(err.Error(), "path escapes from parent")
}

func newTempName() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); nil != err {
		return "", err
	}
	return tempFilePrefix + hex.EncodeToString(buf[:]), nil
}

func (a *LocalFSAdapter) createTemp(dirRel string) (*os.File, string, error) {
	var lastErr error
	for range 8 {
		name, err := newTempName()
		if nil != err {
			return nil, "", err
		}
		tmpRel := path.Join(filepath.ToSlash(dirRel), name)
		file, err := a.root.OpenFile(tmpRel, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		if nil == err {
			return file, tmpRel, nil
		}
		if !os.IsExist(err) {
			return nil, "", err
		}
		lastErr = err
	}
	return nil, "", lastErr
}

func (a *LocalFSAdapter) ensureParent(rel string) (string, error) {
	dirRel := path.Dir(filepath.ToSlash(rel))
	if "." != dirRel {
		if err := a.root.MkdirAll(dirRel, 0700); err != nil {
			if isEscape(err) {
				return "", metadata.ErrForbidden
			}
			return "", fmt.Errorf("failed to create parent directory: %w", err)
		}
	}
	return dirRel, nil
}

// Open opens a file for reading
func (a *LocalFSAdapter) Open(ctx context.Context, p string) (io.ReadCloser, error) {
	rel, err := relPath(p)
	if err != nil {
		return nil, err
	}

	file, err := a.openFile(rel)
	if err != nil {
		if isEscape(err) {
			return nil, metadata.ErrForbidden
		}
		if os.IsNotExist(err) {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to open file %s: %w", p, err)
	}

	return file, nil
}

type rangeReader struct {
	io.Reader
	io.Closer
}

func (a *LocalFSAdapter) OpenRange(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	rel, err := relPath(p)
	if err != nil {
		return nil, err
	}

	if length < -1 {
		return nil, fmt.Errorf("invalid range length %d for %s", length, p)
	}

	file, err := a.openFile(rel)
	if err != nil {
		if isEscape(err) {
			return nil, metadata.ErrForbidden
		}
		if os.IsNotExist(err) {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to open file %s: %w", p, err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat %s: %w", p, err)
	}

	if offset < 0 || offset > info.Size() {
		file.Close()
		return nil, fmt.Errorf("range offset %d out of bounds for %s (size %d): %w", offset, p, info.Size(), io.ErrUnexpectedEOF)
	}

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to seek in %s: %w", p, err)
	}

	var reader io.Reader = file
	if length >= 0 {
		reader = io.LimitReader(file, length)
	}

	return rangeReader{Reader: reader, Closer: file}, nil
}

type tempFile struct {
	file    *os.File
	rel     string
	target  string
	size    int64
	durable bool
}

func (t *tempFile) discard(a *LocalFSAdapter) {
	if nil != t.file {
		t.file.Close()
	}
	a.root.Remove(t.rel)
}

func (a *LocalFSAdapter) writeTemp(ctx context.Context, dirRel string, reader io.Reader, tempErrMsg string) (*tempFile, error) {
	file, tmpRel, err := a.createTemp(dirRel)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", tempErrMsg, err)
	}
	tmp := &tempFile{file: file, rel: tmpRel}
	written, copyErr := copyBuffered(file, reader)
	if nil != copyErr {
		tmp.discard(a)
		return nil, fmt.Errorf("failed to write file content: %w", copyErr)
	}
	tmp.size = written
	if nil == a.flusher {
		err := file.Close()
		tmp.file = nil
		if nil != err {
			a.root.Remove(tmpRel)
			return nil, fmt.Errorf("failed to close file: %w", err)
		}
		return tmp, nil
	}
	if backends.WriteAckDisk != backends.WriteAckFrom(ctx, a.ack) {
		return tmp, nil
	}
	done := make(chan error, 1)
	if err := a.flusher.submit(&flushJob{file: file, size: written, done: done}); nil != err {
		tmp.discard(a)
		return nil, fmt.Errorf("failed to fsync file: %w", err)
	}
	tmp.file = nil
	if err := <-done; nil != err {
		a.root.Remove(tmpRel)
		return nil, fmt.Errorf("failed to fsync file: %w", err)
	}
	tmp.durable = true
	return tmp, nil
}

func (a *LocalFSAdapter) settle(tmp *tempFile, dirRel string) error {
	if nil == a.flusher {
		return nil
	}
	if tmp.durable {
		done := make(chan error, 1)
		if err := a.flusher.submit(&flushJob{dirRel: dirRel, done: done}); nil != err {
			return fmt.Errorf("failed to sync directory: %w", err)
		}
		return <-done
	}
	return a.flusher.submit(&flushJob{file: tmp.file, dirRel: dirRel, path: tmp.target, size: tmp.size})
}

func (a *LocalFSAdapter) syncDirFor(ctx context.Context, dirRel string) error {
	if nil == a.flusher {
		return nil
	}
	if backends.WriteAckDisk != backends.WriteAckFrom(ctx, a.ack) {
		return a.flusher.submit(&flushJob{dirRel: dirRel})
	}
	return a.syncer.sync(dirRel)
}

// Create creates a new file with content from the reader
func (a *LocalFSAdapter) Create(ctx context.Context, p string, reader io.Reader, size int64) error {
	rel, err := relPath(p)
	if err != nil {
		return err
	}

	if err := a.space.ensure(a.rootPath, size); err != nil {
		return err
	}

	dirRel, err := a.ensureParent(rel)
	if err != nil {
		return err
	}

	tmp, err := a.writeTemp(ctx, dirRel, reader, "failed to create temp file")
	if err != nil {
		return err
	}
	tmp.target = rel

	if err := a.publishExclusive(dirRel, tmp.rel, rel); err != nil {
		tmp.discard(a)
		if isEscape(err) {
			return metadata.ErrForbidden
		}
		if errors.Is(err, fs.ErrExist) {
			return metadata.ErrAlreadyExists
		}
		return fmt.Errorf("failed to move temp file to destination: %w", err)
	}

	return a.settle(tmp, dirRel)
}

func (a *LocalFSAdapter) publishExclusive(dirRel, tmpRel, rel string) error {
	err := renameExclusive(a.root, dirRel, path.Base(tmpRel), path.Base(rel))
	if !errors.Is(err, errExclusiveRenameUnsupported) {
		return err
	}
	if err := a.root.Link(tmpRel, rel); nil != err {
		return err
	}
	a.root.Remove(tmpRel)
	return nil
}

// Update updates an existing file with new content from the reader
func (a *LocalFSAdapter) Update(ctx context.Context, p string, reader io.Reader, size int64) error {
	rel, err := relPath(p)
	if err != nil {
		return err
	}

	if err := a.space.ensure(a.rootPath, size); err != nil {
		return err
	}

	dirRel, err := a.ensureParent(rel)
	if err != nil {
		return err
	}

	tmp, err := a.writeTemp(ctx, dirRel, reader, "failed to create temp file for update")
	if err != nil {
		return err
	}
	tmp.target = rel

	if err := a.root.Rename(tmp.rel, rel); err != nil {
		tmp.discard(a)
		if isEscape(err) {
			return metadata.ErrForbidden
		}
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return a.settle(tmp, dirRel)
}

// Delete removes a file or empty directory
func (a *LocalFSAdapter) Delete(ctx context.Context, p string) error {
	rel, err := relPath(p)
	if err != nil {
		return err
	}

	a.subRoots.invalidate(rel)
	err = a.root.Remove(rel)
	a.subRoots.invalidate(rel)
	if err != nil {
		if isEscape(err) {
			return metadata.ErrForbidden
		}
		if os.IsNotExist(err) {
			return metadata.ErrNotFound
		}
		return fmt.Errorf("failed to delete %s: %w", p, err)
	}

	if nil != a.flusher {
		a.flusher.forget(rel)
	}
	return a.syncDirFor(ctx, path.Dir(filepath.ToSlash(rel)))
}

// Move relocates a file or directory subtree from oldPath to newPath.
// os.Rename is atomic on the same filesystem and moves a whole directory tree
// in one call. If the destination lives on a different device (EXDEV), it falls
// back to a recursive copy followed by removal of the source.
func (a *LocalFSAdapter) Move(ctx context.Context, oldPath, newPath string) error {
	srcRel, err := relPath(oldPath)
	if err != nil {
		return err
	}
	dstRel, err := relPath(newPath)
	if err != nil {
		return err
	}

	if _, err := a.root.Lstat(srcRel); nil != err {
		if isEscape(err) {
			return metadata.ErrForbidden
		}
		if os.IsNotExist(err) {
			return metadata.ErrNotFound
		}
		return fmt.Errorf("failed to stat source %s: %w", oldPath, err)
	}

	dstDir := path.Dir(filepath.ToSlash(dstRel))
	srcDir := path.Dir(filepath.ToSlash(srcRel))
	if "." != dstDir {
		if err := a.root.MkdirAll(dstDir, 0700); err != nil {
			if isEscape(err) {
				return metadata.ErrForbidden
			}
			return fmt.Errorf("failed to create destination parent directory: %w", err)
		}
	}

	a.subRoots.invalidate(srcRel)
	a.subRoots.invalidate(dstRel)
	defer a.subRoots.invalidate(srcRel)
	defer a.subRoots.invalidate(dstRel)

	if err := a.root.Rename(srcRel, dstRel); err != nil {
		if isEscape(err) {
			return metadata.ErrForbidden
		}
		if os.IsNotExist(err) {
			return metadata.ErrNotFound
		}
		if isCrossDevice(err) {
			if info, statErr := a.root.Lstat(dstRel); statErr == nil && !info.IsDir() {
				if rmErr := a.root.Remove(dstRel); rmErr != nil {
					return fmt.Errorf("failed to remove existing destination %s: %w", newPath, rmErr)
				}
			}
			if copyErr := copyTreeSynced(a.root, srcRel, dstRel, a.syncMode); nil != copyErr {
				if rmErr := a.root.RemoveAll(dstRel); nil != rmErr {
					copyErr = errors.Join(copyErr, fmt.Errorf("failed to remove partial destination: %w", rmErr))
				}
				return fmt.Errorf("failed to copy across devices: %w", copyErr)
			}
			if syncErr := a.syncer.sync(dstDir); nil != syncErr {
				return syncErr
			}
			if rmErr := a.root.RemoveAll(srcRel); nil != rmErr {
				return fmt.Errorf("copied %s to %s but failed to remove source, source may be partially removed: %w", oldPath, newPath, rmErr)
			}
			return a.syncer.sync(srcDir)
		}
		return fmt.Errorf("failed to move %s to %s: %w", oldPath, newPath, err)
	}

	if err := a.syncer.sync(dstDir); err != nil {
		return err
	}

	return a.syncer.sync(srcDir)
}

const copySyncWorkers = 8

func copyTreeSynced(root *os.Root, src, dst string, mode SyncMode) error {
	files := make(chan *os.File)
	syncErrs := make([]error, copySyncWorkers)
	var wg sync.WaitGroup
	for worker := range copySyncWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for out := range files {
				err := syncData(out, mode)
				err = errors.Join(err, out.Close())
				syncErrs[worker] = errors.Join(syncErrs[worker], err)
			}
		}()
	}

	copyErr := copyPath(root, src, dst, files)
	close(files)
	wg.Wait()
	return errors.Join(copyErr, errors.Join(syncErrs...))
}

func copyPath(root *os.Root, src, dst string, files chan<- *os.File) error {
	info, err := root.Lstat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		if err := root.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		dir, err := root.Open(src)
		if err != nil {
			return err
		}
		entries, err := dir.ReadDir(-1)
		dir.Close()
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyPath(root, path.Join(src, entry.Name()), path.Join(dst, entry.Name()), files); err != nil {
				return err
			}
		}
		return nil
	}

	if 0 != info.Mode()&os.ModeSymlink {
		return fmt.Errorf("refusing to copy symlink %s", src)
	}

	in, err := root.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := root.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	files <- out
	return nil
}

// Stat returns metadata for a file or directory
func (a *LocalFSAdapter) Stat(ctx context.Context, p string) (*metadata.Metadata, error) {
	rel, err := relPath(p)
	if err != nil {
		return nil, err
	}

	info, err := a.root.Stat(rel)
	if err != nil {
		if isEscape(err) {
			return nil, metadata.ErrForbidden
		}
		if os.IsNotExist(err) {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to stat %s: %w", p, err)
	}

	return metadataFromInfo(p, info), nil
}

func metadataFromInfo(path string, info os.FileInfo) *metadata.Metadata {
	md := &metadata.Metadata{
		Name:        info.Name(),
		Path:        path,
		Size:        info.Size(),
		MTime:       info.ModTime(),
		ATime:       info.ModTime(),
		CTime:       info.ModTime(),
		BackendType: "localfs",
	}

	if info.IsDir() {
		md.Type = "directory"
	} else {
		md.Type = "file"
	}

	md.Mode, md.ATime, md.CTime = extractPlatformMetadata(info)

	return md
}

// ListDirectory returns metadata for all children of a directory
func (a *LocalFSAdapter) ListDirectory(ctx context.Context, path string) ([]*metadata.Metadata, error) {
	return a.ListDirectoryPage(ctx, path, 0, 0)
}

func (a *LocalFSAdapter) ListDirectoryPage(ctx context.Context, path string, offset, limit int) ([]*metadata.Metadata, error) {
	if offset < 0 || limit < 0 {
		return nil, fmt.Errorf("invalid directory page offset %d or limit %d", offset, limit)
	}

	rel, err := relPath(path)
	if err != nil {
		return nil, err
	}

	dir, err := a.root.Open(rel)
	if err != nil {
		if isEscape(err) {
			return nil, metadata.ErrForbidden
		}
		if os.IsNotExist(err) {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to read directory %s: %w", path, err)
	}
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to read directory %s: %w", path, err)
	}
	slices.SortFunc(entries, func(x, y fs.DirEntry) int { return strings.Compare(x.Name(), y.Name()) })

	visible := entries[:0]
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), tempFilePrefix) {
			visible = append(visible, entry)
		}
	}
	entries = visible

	if offset >= len(entries) {
		return nil, nil
	}
	entries = entries[offset:]
	if limit > 0 && limit < len(entries) {
		entries = entries[:limit]
	}

	var children []*metadata.Metadata
	for _, entry := range entries {
		childMd, err := a.childMetadata(ctx, path, entry)
		if err != nil {
			if errors.Is(err, metadata.ErrNotFound) || os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("failed to read directory entry %s: %w", entry.Name(), err)
		}
		children = append(children, childMd)
	}

	return children, nil
}

func (a *LocalFSAdapter) childMetadata(ctx context.Context, dirPath string, entry fs.DirEntry) (*metadata.Metadata, error) {
	childPath := filepath.Join(dirPath, entry.Name())
	if entry.Type()&fs.ModeSymlink != 0 {
		return a.Stat(ctx, childPath)
	}
	info, err := entry.Info()
	if err != nil {
		return nil, err
	}
	return metadataFromInfo(childPath, info), nil
}

// CreateDirectory creates a new directory
func (a *LocalFSAdapter) CreateDirectory(ctx context.Context, p string) error {
	rel, err := relPath(p)
	if err != nil {
		return err
	}

	if info, err := a.root.Stat(rel); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("path exists as file, not directory: %w", metadata.ErrAlreadyExists)
		}
		return nil
	} else if isEscape(err) {
		return metadata.ErrForbidden
	}

	if err := a.root.MkdirAll(rel, 0700); err != nil {
		if isEscape(err) {
			return metadata.ErrForbidden
		}
		return fmt.Errorf("failed to create directory %s: %w", p, err)
	}

	return nil
}

// Close closes any resources used by the storage backend
func (a *LocalFSAdapter) Close() error {
	if nil != a.flusher {
		a.flusher.close()
	}
	a.subRoots.invalidateAll()
	return a.root.Close()
}
