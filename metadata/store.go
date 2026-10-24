package metadata

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Common metadata errors
var (
	ErrNotFound        = errors.New("metadata not found")
	ErrAlreadyExists   = errors.New("metadata already exists")
	ErrForbidden       = errors.New("access forbidden")
	ErrPrecondition    = errors.New("precondition failed")
	ErrSubtreeTooLarge = errors.New("directory subtree too large")
	ErrNameTooLong     = errors.New("file or directory name too long")
	ErrNotImplemented  = errors.New("not implemented")
)

// Metadata represents filesystem metadata for an inode
type Metadata struct {
	ID               int64     `json:"id"`
	ParentID         *int64    `json:"parent_id"`
	Name             string    `json:"name"`
	Path             string    `json:"path"`
	Type             string    `json:"type"` // "file" or "directory"
	Size             int64     `json:"size"`
	Mode             string    `json:"mode"`  // Permission mode string like "0644"
	Owner            string    `json:"owner"` // App-level owner (user ID string, e.g. "api-user-0"). NOT an OS user.
	ATime            time.Time `json:"atime"`
	MTime            time.Time `json:"mtime"`
	CTime            time.Time `json:"ctime"`
	BackendType      string    `json:"backend_type"`       // "localfs", "s3", or "erasure"
	ErasureCoded     bool      `json:"erasure_coded"`      // true if file is erasure-coded
	CallFSInstanceID *string   `json:"callfs_instance_id"` // Instance ID for the server that owns this file
	SymlinkTarget    *string   `json:"symlink_target"`     // For future symlink support
	ETag             string    `json:"etag"`               // Hex SHA-256 of the file content; empty for directories
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ChildCounter is implemented by stores that can count a directory's direct
// children without materialising them.
type AtomicNamespace interface {
	AtomicNamespace() bool
}

type ChildCounter interface {
	CountChildren(ctx context.Context, parentPath string) (int, error)
}

// SingleUseLink represents a secure, single-use download link
type SingleUseLink struct {
	ID            int64      `json:"id"`
	Token         string     `json:"token"`
	FilePath      string     `json:"file_path"`
	Owner         string     `json:"owner"`
	Status        string     `json:"status"` // "active", "used", "expired"
	ExpiresAt     time.Time  `json:"expires_at"`
	UsedAt        *time.Time `json:"used_at"`
	UsedByIP      *string    `json:"used_by_ip"`
	HMACSignature string     `json:"hmac_signature"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// ErasureFileInfo holds erasure coding metadata (imported by metadata stores)
type ErasureFileInfo struct {
	FilePath     string             `json:"file_path"`
	OriginalSize int64              `json:"original_size"`
	DataShards   int                `json:"data_shards"`
	ParityShards int                `json:"parity_shards"`
	ShardSize    int64              `json:"shard_size"`
	Shards       []ErasureShardInfo `json:"shards"`
}

// ErasureShardInfo describes a single shard's storage location.
type ErasureShardInfo struct {
	Index       int    `json:"index"`
	InstanceID  string `json:"instance_id"`
	BackendType string `json:"backend_type"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	Checksum    string `json:"checksum"`
}

// ErasureMetadataStore defines the interface for erasure coding metadata operations.
type ErasureMetadataStore interface {
	CreateErasureInfo(ctx context.Context, filePath string, info *ErasureFileInfo) error
	GetErasureInfo(ctx context.Context, filePath string) (*ErasureFileInfo, error)
	DeleteErasureInfo(ctx context.Context, filePath string) error
}

// Store defines the interface for metadata storage operations
type Store interface {
	// Get retrieves metadata for a file or directory by path
	Get(ctx context.Context, path string) (*Metadata, error)

	// Create creates a new inode entry
	Create(ctx context.Context, md *Metadata) error

	// Update updates an existing inode entry
	Update(ctx context.Context, md *Metadata) error

	// Delete removes an inode entry by path
	Delete(ctx context.Context, path string) error

	// Rename re-keys an inode from oldPath to newPath, updating its name and
	// path. When oldPath is a directory, every descendant's path is rewritten in
	// the same atomic operation. It does not move bytes (that is the backend's
	// job) and does not change ownership, backend, or instance. Returns
	// ErrNotFound if oldPath does not exist and ErrAlreadyExists if newPath does.
	Rename(ctx context.Context, oldPath, newPath string) error

	// UpdateIfMatch updates an existing inode entry only when its stored ETag
	// equals expectedETag, as one atomic operation. Returns ErrNotFound if the
	// path does not exist and ErrPrecondition if the stored ETag differs.
	UpdateIfMatch(ctx context.Context, md *Metadata, expectedETag string) error

	// ListChildren returns all children of a directory
	ListChildren(ctx context.Context, parentPath string) ([]*Metadata, error)

	// HasChildren reports whether a directory has at least one child entry.
	HasChildren(ctx context.Context, parentPath string) (bool, error)

	// ListDescendants returns every entry (files and directories) strictly
	// below dirPath, at most maxDepth levels deep (0 means unlimited), ordered
	// by path ascending, in one store query or scan, returning at most limit
	// entries (0 means unlimited).
	ListDescendants(ctx context.Context, dirPath string, maxDepth, limit int) ([]*Metadata, error)

	// ListByPrefix returns entries of type "file" whose path starts with prefix
	// (case-sensitive, byte-wise) and sorts byte-wise strictly after startAfter,
	// ordered by path ascending, at most limit entries. An empty startAfter
	// starts at the beginning.
	ListByPrefix(ctx context.Context, prefix, startAfter string, limit int) ([]*Metadata, error)

	// GetSingleUseLink retrieves a single-use link by token
	GetSingleUseLink(ctx context.Context, token string) (*SingleUseLink, error)

	// CreateSingleUseLink creates a new single-use link
	CreateSingleUseLink(ctx context.Context, link *SingleUseLink) error

	CountActiveSingleUseLinks(ctx context.Context, owner string, now time.Time) (int, error)

	// UpdateSingleUseLink atomically updates a single-use link status
	UpdateSingleUseLink(ctx context.Context, token string, status string, usedAt *time.Time, usedByIP *string) error

	// ConsumeSingleUseLink atomically marks an active, unexpired link as used
	// and returns it with consumed true. When the link exists but cannot be
	// consumed it returns the stored link with consumed false. Returns
	// ErrNotFound when no link has that token.
	ConsumeSingleUseLink(ctx context.Context, token string, usedByIP string, now time.Time) (*SingleUseLink, bool, error)

	// CleanupExpiredLinks removes expired single-use links and returns count of removed links
	CleanupExpiredLinks(ctx context.Context, before time.Time) (int, error)

	// CleanupUsedLinks removes used single-use links older than the given time and returns count of removed links
	CleanupUsedLinks(ctx context.Context, olderThan time.Time) (int, error)

	ListSingleUseLinks(ctx context.Context, filter LinkFilter) ([]SingleUseLink, string, error)

	GetSingleUseLinkByID(ctx context.Context, id int64) (*SingleUseLink, error)

	RevokeSingleUseLink(ctx context.Context, id int64) error

	SearchMetadata(ctx context.Context, q SearchQuery) (SearchResult, error)

	OwnerStats(ctx context.Context, cursor string, limit int) ([]OwnerStat, string, error)

	CountEmptyETags(ctx context.Context) (int, error)

	CountEmptyOwners(ctx context.Context) (int, error)

	ListErasureInfos(ctx context.Context, prefix, cursor string, limit int) ([]*ErasureFileInfo, string, error)

	// Close closes the metadata store connection
	Close() error
}

type LocalState interface {
	LocalState() bool
}

type DescendantSearcher interface {
	SearchDescendants(ctx context.Context, dirPath string, maxDepth, limit int, match func(*Metadata) bool) ([]*Metadata, int, error)
}

func SearchDescendants(ctx context.Context, store Store, dirPath string, maxDepth, limit int, match func(*Metadata) bool) ([]*Metadata, int, error) {
	if searcher, ok := store.(DescendantSearcher); ok {
		return searcher.SearchDescendants(ctx, dirPath, maxDepth, limit, match)
	}
	rows, err := store.ListDescendants(ctx, dirPath, maxDepth, limit)
	if nil != err {
		return nil, 0, err
	}
	matched := make([]*Metadata, 0)
	for _, md := range rows {
		if match(md) {
			matched = append(matched, md)
		}
	}
	return matched, len(rows), nil
}

type OpKind string

const (
	OpCreate            OpKind = "create"
	OpUpdate            OpKind = "update"
	OpUpdateIfMatch     OpKind = "update_if_match"
	OpDelete            OpKind = "delete"
	OpRename            OpKind = "rename"
	OpCreateErasureInfo OpKind = "create_erasure_info"
	OpDeleteErasureInfo OpKind = "delete_erasure_info"
)

var ErrInvalidOp = errors.New("invalid batch operation")

type Op struct {
	Kind         OpKind
	Path         string
	NewPath      string
	Metadata     *Metadata
	ExpectedETag string
	ErasureInfo  *ErasureFileInfo
}

func (op Op) TargetPath() string {
	if (OpCreate == op.Kind || OpUpdate == op.Kind || OpUpdateIfMatch == op.Kind) && nil != op.Metadata {
		return op.Metadata.Path
	}
	return op.Path
}

func (op Op) Validate() error {
	switch op.Kind {
	case OpCreate, OpUpdate, OpUpdateIfMatch:
		if nil == op.Metadata {
			return fmt.Errorf("%w: %s needs metadata", ErrInvalidOp, op.Kind)
		}
	case OpDelete, OpDeleteErasureInfo:
	case OpRename:
		if "" == op.NewPath {
			return fmt.Errorf("%w: rename needs a new path", ErrInvalidOp)
		}
	case OpCreateErasureInfo:
		if nil == op.ErasureInfo {
			return fmt.Errorf("%w: create_erasure_info needs erasure info", ErrInvalidOp)
		}
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidOp, op.Kind)
	}
	return nil
}

type Batcher interface {
	ApplyBatch(ctx context.Context, ops []Op) error
}

func ValidateOps(ops []Op) error {
	for _, op := range ops {
		if err := op.Validate(); nil != err {
			return err
		}
	}
	return nil
}

func ApplyBatch(ctx context.Context, store Store, ops []Op) error {
	if 0 == len(ops) {
		return nil
	}
	if err := ValidateOps(ops); nil != err {
		return err
	}
	if batcher, ok := store.(Batcher); ok {
		return batcher.ApplyBatch(ctx, ops)
	}
	return ApplySequential(ctx, store, ops)
}

func ApplySequential(ctx context.Context, store Store, ops []Op) error {
	for _, op := range ops {
		if err := ApplyOp(ctx, store, op); nil != err {
			return err
		}
	}
	return nil
}

func ApplyOp(ctx context.Context, store Store, op Op) error {
	switch op.Kind {
	case OpCreate:
		return store.Create(ctx, op.Metadata)
	case OpUpdate:
		return store.Update(ctx, op.Metadata)
	case OpUpdateIfMatch:
		return store.UpdateIfMatch(ctx, op.Metadata, op.ExpectedETag)
	case OpDelete:
		return store.Delete(ctx, op.Path)
	case OpRename:
		return store.Rename(ctx, op.Path, op.NewPath)
	}
	erasureStore, ok := store.(ErasureMetadataStore)
	if !ok {
		return fmt.Errorf("%w: store does not support erasure metadata", ErrNotImplemented)
	}
	switch op.Kind {
	case OpCreateErasureInfo:
		return erasureStore.CreateErasureInfo(ctx, op.Path, op.ErasureInfo)
	case OpDeleteErasureInfo:
		return erasureStore.DeleteErasureInfo(ctx, op.Path)
	}
	return fmt.Errorf("%w: unknown kind %q", ErrInvalidOp, op.Kind)
}
