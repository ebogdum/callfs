package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/ebogdum/callfs/metadata"
)

// UnixAuthorizer implements permission checking using app-level ownership.
// Authorization is based on the app user ID string (Owner field in metadata),
// NOT on OS-level UIDs/GIDs. App users have no relationship to OS users.
type UnixAuthorizer struct {
	metadataStore metadata.Store
}

// NewUnixAuthorizer creates a new authorizer
func NewUnixAuthorizer(metadataStore metadata.Store) *UnixAuthorizer {
	return &UnixAuthorizer{
		metadataStore: metadataStore,
	}
}

// Authorize checks if a user has the specified permission for a path
func (a *UnixAuthorizer) Authorize(ctx context.Context, userID string, path string, perm PermissionType) error {
	// Get metadata for the file/directory
	md, err := a.metadataStore.Get(ctx, path)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			// For write operations on non-existent files, check parent directory
			if perm == WritePerm {
				return a.checkParentPermission(ctx, userID, path, WritePerm)
			}
			return metadata.ErrNotFound
		}
		return fmt.Errorf("failed to get metadata for authorization: %w", err)
	}

	return a.checkPermissions(md, userID, perm)
}

// checkPermissions verifies the app user has the requested permission.
//
// Rules:
//  1. The "root" and "internal-proxy" app users bypass all checks (admin).
//  2. The resource owner has full read/write/delete access.
//  3. Directories: all authenticated users can read and write (create children).
//     Only the owner can delete the directory itself.
//  4. Files: all authenticated users can read. Only the owner can write or delete.
func (a *UnixAuthorizer) checkPermissions(md *metadata.Metadata, userID string, perm PermissionType) error {
	// Admin app users bypass all permission checks
	if userID == "root" || userID == "internal-proxy" {
		return nil
	}

	// Owner has full access
	if md.Owner == userID {
		return nil
	}

	// Directory permissions: all authenticated users can read and create children.
	// Only the owner can delete the directory.
	if md.Type == "directory" {
		if perm == DeletePerm {
			return ErrPermissionDenied
		}
		return nil
	}

	// File permissions: non-owners can read but not write or delete.
	if perm == ReadPerm {
		return nil
	}
	return ErrPermissionDenied
}

// checkParentPermission checks permissions on parent directory
func (a *UnixAuthorizer) checkParentPermission(ctx context.Context, userID string, path string, perm PermissionType) error {
	// Extract parent directory path
	lastSlash := -1
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			lastSlash = i
			break
		}
	}

	if lastSlash <= 0 {
		// Root-level path: check root directory permissions
		parentMd, err := a.metadataStore.Get(ctx, "/")
		if err != nil {
			if errors.Is(err, metadata.ErrNotFound) {
				return nil // Root doesn't exist yet, allow
			}
			return fmt.Errorf("failed to get root metadata: %w", err)
		}
		return a.checkPermissions(parentMd, userID, perm)
	}

	parentPath := path[:lastSlash]
	if parentPath == "" {
		parentPath = "/"
	}

	// Get parent metadata
	parentMd, err := a.metadataStore.Get(ctx, parentPath)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("failed to get parent metadata: %w", err)
	}

	return a.checkPermissions(parentMd, userID, perm)
}
