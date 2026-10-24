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
type OwnerAliases interface {
	LegacyAliases(userID string) []string
}

type UnixAuthorizer struct {
	metadataStore metadata.Store
	aliases       OwnerAliases
}

func NewUnixAuthorizer(metadataStore metadata.Store, aliases OwnerAliases) *UnixAuthorizer {
	return &UnixAuthorizer{
		metadataStore: metadataStore,
		aliases:       aliases,
	}
}

func (a *UnixAuthorizer) isOwner(owner string, userID string) bool {
	if owner == userID {
		return true
	}
	if nil == a.aliases {
		return false
	}
	for _, alias := range a.aliases.LegacyAliases(userID) {
		if owner == alias {
			return true
		}
	}
	return false
}

// Authorize checks if a user has the specified permission for a path
func (a *UnixAuthorizer) Authorize(ctx context.Context, userID string, path string, perm PermissionType) error {
	// Get metadata for the file/directory
	md, err := a.metadataStore.Get(ctx, path)
	if nil != err {
		if !errors.Is(err, metadata.ErrNotFound) {
			return fmt.Errorf("failed to get metadata for authorization: %w", err)
		}
		md = nil
	}

	return a.AuthorizeWithMetadata(ctx, userID, path, md, perm)
}

func (a *UnixAuthorizer) AuthorizeWithMetadata(ctx context.Context, userID string, path string, md *metadata.Metadata, perm PermissionType) error {
	if nil == md {
		if WritePerm == perm {
			return a.checkParentPermission(ctx, userID, path, WritePerm)
		}
		return metadata.ErrNotFound
	}

	return a.checkPermissions(md, userID, perm)
}

type Decision struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
	Owner   string `json:"owner"`
	Type    string `json:"type"`
}

func (a *UnixAuthorizer) Explain(ctx context.Context, userID string, path string, perm PermissionType) (Decision, error) {
	md, err := a.metadataStore.Get(ctx, path)
	if nil != err {
		if !errors.Is(err, metadata.ErrNotFound) {
			return Decision{}, fmt.Errorf("failed to get metadata for authorization: %w", err)
		}
		md = nil
	}

	err = a.AuthorizeWithMetadata(ctx, userID, path, md, perm)
	if nil != err && !errors.Is(err, ErrPermissionDenied) && !errors.Is(err, metadata.ErrNotFound) {
		return Decision{}, err
	}
	allowed := nil == err

	if nil == md {
		if allowed {
			return Decision{Allowed: true, Reason: "parent_open"}, nil
		}
		if errors.Is(err, metadata.ErrNotFound) {
			return Decision{Reason: "not_found"}, nil
		}
		return Decision{Reason: "not_owner"}, nil
	}

	decision := Decision{Allowed: allowed, Owner: md.Owner, Type: md.Type, Reason: "not_owner"}
	switch {
	case isSuperUser(userID):
		decision.Reason = "admin"
	case a.isOwner(md.Owner, userID):
		decision.Reason = "owner"
	case allowed && "directory" == md.Type:
		decision.Reason = "open_directory"
	}
	return decision, nil
}

func isSuperUser(userID string) bool {
	return "root" == userID || "internal-proxy" == userID
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
	if isSuperUser(userID) {
		return nil
	}

	// Owner has full access
	if a.isOwner(md.Owner, userID) {
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
			return a.checkParentPermission(ctx, userID, parentPath, perm)
		}
		return fmt.Errorf("failed to get parent metadata: %w", err)
	}

	return a.checkPermissions(parentMd, userID, perm)
}
