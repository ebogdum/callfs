package metadata

import (
	"context"
	"errors"
	"time"
)

// Common metadata errors
var (
	ErrNotFound      = errors.New("metadata not found")
	ErrAlreadyExists = errors.New("metadata already exists")
	ErrForbidden     = errors.New("access forbidden")
)

// Metadata represents filesystem metadata for an inode
type Metadata struct {
	ID               int64     `json:"id"`
	ParentID         *int64    `json:"parent_id"`
	Name             string    `json:"name"`
	Path             string    `json:"path"`
	Type             string    `json:"type"` // "file" or "directory"
	Size             int64     `json:"size"`
	Mode             string    `json:"mode"` // Unix permissions like "0644"
	UID              int       `json:"uid"`
	GID              int       `json:"gid"`
	ATime            time.Time `json:"atime"`
	MTime            time.Time `json:"mtime"`
	CTime            time.Time `json:"ctime"`
	BackendType      string    `json:"backend_type"`       // "localfs" or "s3"
	CallFSInstanceID *string   `json:"callfs_instance_id"` // Instance ID for the server that owns this file
	SymlinkTarget    *string   `json:"symlink_target"`     // For future symlink support
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// SingleUseLink represents a secure, single-use download link
type SingleUseLink struct {
	ID            int64      `json:"id"`
	Token         string     `json:"token"`
	FilePath      string     `json:"file_path"`
	Status        string     `json:"status"` // "active", "used", "expired"
	ExpiresAt     time.Time  `json:"expires_at"`
	UsedAt        *time.Time `json:"used_at"`
	UsedByIP      *string    `json:"used_by_ip"`
	HMACSignature string     `json:"hmac_signature"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
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

	// ListChildren returns all children of a directory
	ListChildren(ctx context.Context, parentPath string) ([]*Metadata, error)

	// GetSingleUseLink retrieves a single-use link by token
	GetSingleUseLink(ctx context.Context, token string) (*SingleUseLink, error)

	// CreateSingleUseLink creates a new single-use link
	CreateSingleUseLink(ctx context.Context, link *SingleUseLink) error

	// UpdateSingleUseLink atomically updates a single-use link status
	UpdateSingleUseLink(ctx context.Context, token string, status string, usedAt *time.Time, usedByIP *string) error

	// CleanupExpiredLinks removes expired single-use links and returns count of removed links
	CleanupExpiredLinks(ctx context.Context, before time.Time) (int, error)

	// CleanupUsedLinks removes used single-use links older than the given time and returns count of removed links
	CleanupUsedLinks(ctx context.Context, olderThan time.Time) (int, error)

	// Close closes the metadata store connection
	Close() error
}
