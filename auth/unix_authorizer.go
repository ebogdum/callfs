package auth

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/ebogdum/callfs/v2/metadata"
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
	admins        *AdminSet
}

func NewUnixAuthorizer(metadataStore metadata.Store, aliases OwnerAliases) *UnixAuthorizer {
	return &UnixAuthorizer{
		metadataStore: metadataStore,
		aliases:       aliases,
	}
}

func (a *UnixAuthorizer) SetAdmins(admins *AdminSet) {
	a.admins = admins
}

func (a *UnixAuthorizer) isAdmin(userID string) bool {
	return isSuperUser(userID) || (nil != a.admins && a.admins.IsAdmin(userID))
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
	case a.isAdmin(userID):
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

const (
	otherReadBit  = 04
	otherWriteBit = 02

	defaultFileMode      = 0644
	defaultDirectoryMode = 0777
)

func otherBits(md *metadata.Metadata) uint64 {
	fallback := uint64(defaultFileMode)
	if "directory" == md.Type {
		fallback = defaultDirectoryMode
	}
	mode, err := strconv.ParseUint(md.Mode, 8, 32)
	if nil != err || "" == md.Mode {
		mode = fallback
	}
	return mode & 07
}

func (a *UnixAuthorizer) checkPermissions(md *metadata.Metadata, userID string, perm PermissionType) error {
	if a.isAdmin(userID) {
		return nil
	}

	if a.isOwner(md.Owner, userID) {
		return nil
	}

	switch perm {
	case ReadPerm:
		if 0 != otherBits(md)&otherReadBit {
			return nil
		}
	case WritePerm:
		if 0 != otherBits(md)&otherWriteBit {
			return nil
		}
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
