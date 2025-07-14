package auth

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/ebogdum/callfs/metadata"
)

// UnixAuthorizer implements Unix-style permission checking
type UnixAuthorizer struct {
	metadataStore metadata.Store
}

// NewUnixAuthorizer creates a new Unix-style authorizer
func NewUnixAuthorizer(metadataStore metadata.Store) *UnixAuthorizer {
	return &UnixAuthorizer{
		metadataStore: metadataStore,
	}
}

// Authorize checks if a user has the specified permission for a path
func (a *UnixAuthorizer) Authorize(ctx context.Context, userID string, path string, perm PermissionType) error {
	// Root user bypasses all permission checks
	if userID == "root" {
		return nil
	}

	// Get metadata for the file/directory
	md, err := a.metadataStore.Get(ctx, path)
	if err != nil {
		if err == metadata.ErrNotFound {
			// For write operations on non-existent files, check parent directory
			if perm == WritePerm {
				return a.checkParentPermission(ctx, userID, path, WritePerm)
			}
			return ErrPermissionDenied
		}
		return fmt.Errorf("failed to get metadata for authorization: %w", err)
	}

	// For now, implement basic permission logic
	// In a real implementation, you would map userID to actual Unix UID/GID
	return a.checkUnixPermissions(md, userID, perm)
}

// checkUnixPermissions performs Unix-style permission checking
func (a *UnixAuthorizer) checkUnixPermissions(md *metadata.Metadata, userID string, perm PermissionType) error {
	// Parse mode string (e.g., "0644" -> 644)
	mode, err := strconv.ParseUint(strings.TrimPrefix(md.Mode, "0"), 8, 32)
	if err != nil {
		return fmt.Errorf("invalid mode format: %s", md.Mode)
	}

	// For simplicity, assume userID "root" has UID 0, others have UID 1000
	var userUID int
	if userID == "root" {
		userUID = 0
	} else {
		userUID = 1000
	}

	// Determine permission bits to check
	var permBits uint64
	switch perm {
	case ReadPerm:
		if userUID == md.UID {
			permBits = mode >> 6 & 4 // Owner read
		} else if userUID == md.GID {
			permBits = mode >> 3 & 4 // Group read
		} else {
			permBits = mode & 4 // Other read
		}
	case WritePerm:
		if userUID == md.UID {
			permBits = mode >> 6 & 2 // Owner write
		} else if userUID == md.GID {
			permBits = mode >> 3 & 2 // Group write
		} else {
			permBits = mode & 2 // Other write
		}
	case DeletePerm:
		// For delete, check write permission on parent directory
		// For now, check write permission on the file itself
		if userUID == md.UID {
			permBits = mode >> 6 & 2 // Owner write
		} else if userUID == md.GID {
			permBits = mode >> 3 & 2 // Group write
		} else {
			permBits = mode & 2 // Other write
		}
	}

	// Root user bypasses permission checks
	if userUID == 0 {
		return nil
	}

	if permBits == 0 {
		return ErrPermissionDenied
	}

	return nil
}

// checkParentPermission checks permissions on parent directory
func (a *UnixAuthorizer) checkParentPermission(ctx context.Context, userID string, path string, perm PermissionType) error {
	// Extract parent directory path
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash <= 0 {
		// Root directory
		return nil
	}

	parentPath := path[:lastSlash]
	if parentPath == "" {
		parentPath = "/"
	}

	// Get parent metadata
	parentMd, err := a.metadataStore.Get(ctx, parentPath)
	if err != nil {
		return fmt.Errorf("failed to get parent metadata: %w", err)
	}

	return a.checkUnixPermissions(parentMd, userID, perm)
}
