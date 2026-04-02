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
	// Get metadata for the file/directory
	md, err := a.metadataStore.Get(ctx, path)
	if err != nil {
		if err == metadata.ErrNotFound {
			// For write operations on non-existent files, check parent directory
			if perm == WritePerm {
				return a.checkParentPermission(ctx, userID, path, WritePerm)
			}
			return metadata.ErrNotFound
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
	mode, err := parseModeBits(md.Mode)
	if err != nil {
		return fmt.Errorf("invalid mode format: %s", md.Mode)
	}

	// Derive UID and GID from userID: root=0/0, api-user-N uses UID/GID 1000+N, others get 1000
	var userUID, userGID int
	if userID == "root" {
		userUID = 0
		userGID = 0
	} else if strings.HasPrefix(userID, "api-user-") {
		if n, err := strconv.Atoi(strings.TrimPrefix(userID, "api-user-")); err == nil {
			userUID = 1000 + n
			userGID = 1000 + n
		} else {
			userUID = 1000
			userGID = 1000
		}
	} else {
		userUID = 1000
		userGID = 1000
	}

	// Determine permission bits to check: owner → group (using GID) → other
	var permBits uint64
	switch perm {
	case ReadPerm:
		if userUID == md.UID {
			permBits = mode >> 6 & 4
		} else if userGID == md.GID {
			permBits = mode >> 3 & 4
		} else {
			permBits = mode & 4
		}
	case WritePerm:
		if userUID == md.UID {
			permBits = mode >> 6 & 2
		} else if userGID == md.GID {
			permBits = mode >> 3 & 2
		} else {
			permBits = mode & 2
		}
	case DeletePerm:
		if userUID == md.UID {
			permBits = mode >> 6 & 2
		} else if userGID == md.GID {
			permBits = mode >> 3 & 2
		} else {
			permBits = mode & 2
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

func parseModeBits(mode string) (uint64, error) {
	switch mode {
	case "0644":
		return 0o644, nil
	case "0755":
		return 0o755, nil
	case "0777":
		return 0o777, nil
	case "0600":
		return 0o600, nil
	}
	return strconv.ParseUint(strings.TrimPrefix(mode, "0"), 8, 32)
}

// checkParentPermission checks permissions on parent directory
func (a *UnixAuthorizer) checkParentPermission(ctx context.Context, userID string, path string, perm PermissionType) error {
	// Extract parent directory path
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash <= 0 {
		// Root-level path: check root directory permissions
		parentMd, err := a.metadataStore.Get(ctx, "/")
		if err != nil {
			if err == metadata.ErrNotFound {
				return nil // Root doesn't exist yet, allow
			}
			return fmt.Errorf("failed to get root metadata: %w", err)
		}
		return a.checkUnixPermissions(parentMd, userID, perm)
	}

	parentPath := path[:lastSlash]
	if parentPath == "" {
		parentPath = "/"
	}

	// Get parent metadata
	parentMd, err := a.metadataStore.Get(ctx, parentPath)
	if err != nil {
		if err == metadata.ErrNotFound {
			return metadata.ErrNotFound
		}
		return fmt.Errorf("failed to get parent metadata: %w", err)
	}

	return a.checkUnixPermissions(parentMd, userID, perm)
}
