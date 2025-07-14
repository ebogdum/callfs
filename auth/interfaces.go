// Package auth provides authentication and authorization interfaces and implementations for CallFS.
// It includes API key authentication for REST endpoints and Unix socket authorization for local access.
package auth

import (
	"context"
	"errors"
)

// PermissionType represents different permission types for authorization
type PermissionType int

const (
	ReadPerm PermissionType = iota
	WritePerm
	DeletePerm
)

// Common authentication/authorization errors
var (
	ErrAuthenticationFailed = errors.New("authentication failed")
	ErrPermissionDenied     = errors.New("permission denied")
	ErrInvalidToken         = errors.New("invalid token")
)

// Authenticator defines the interface for user authentication
type Authenticator interface {
	// Authenticate validates a token and returns the associated user ID
	Authenticate(ctx context.Context, token string) (userID string, err error)
}

// Authorizer defines the interface for authorization checks
type Authorizer interface {
	// Authorize checks if a user has the specified permission for a path
	Authorize(ctx context.Context, userID string, path string, perm PermissionType) error
}
