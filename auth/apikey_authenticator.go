package auth

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strings"
)

// APIKeyAuthenticator implements authentication using static API keys.
// The internal proxy secret is registered with a dedicated "internal-proxy" user ID
// so that cross-server proxy operations authenticate successfully on the public API.
type APIKeyAuthenticator struct {
	validKeys map[string]string
}

// NewAPIKeyAuthenticator creates a new API key authenticator.
//
// keys is the legacy positional form: each key is assigned the user ID
// "api-user-<index>". That identity is derived from list position, so removing
// or reordering an entry reassigns the IDs of every later key — and because the
// user ID is what gets persisted as a resource's Owner, the next key holder
// inherits the removed user's files. Callers should prefer keyUsers.
//
// keyUsers maps an explicit app user ID to its key. Identity is bound to the
// name rather than to ordering, so keys can be revoked or rotated without
// reassigning ownership of existing resources. Entries here take precedence over
// a positional assignment for the same key.
//
// The internalProxySecret is registered as a valid key with the "internal-proxy"
// user ID so cross-server operations (UpdateFileOnInstance, etc.) can
// authenticate on peers.
func NewAPIKeyAuthenticator(keys []string, keyUsers map[string]string, internalProxySecret string) *APIKeyAuthenticator {
	validKeys := make(map[string]string, len(keys)+len(keyUsers)+1)
	userIndex := 0
	for _, key := range keys {
		if key != "" {
			validKeys[key] = fmt.Sprintf("api-user-%d", userIndex)
			userIndex++
		}
	}
	for userID, key := range keyUsers {
		if key != "" && userID != "" {
			validKeys[key] = userID
		}
	}
	if internalProxySecret != "" {
		validKeys[internalProxySecret] = "internal-proxy"
	}

	return &APIKeyAuthenticator{
		validKeys: validKeys,
	}
}

// Authenticate validates a token and returns the associated user ID
func (a *APIKeyAuthenticator) Authenticate(ctx context.Context, token string) (string, error) {
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)

	if token == "" {
		return "", ErrAuthenticationFailed
	}

	// Iterate ALL keys with constant-time comparison to prevent timing attacks.
	// No early return: the number of iterations must be constant regardless of
	// which key (if any) matches, to avoid leaking key-position information.
	var foundUser string
	found := 0
	for key, user := range a.validKeys {
		if subtle.ConstantTimeCompare([]byte(token), []byte(key)) == 1 {
			foundUser = user
			found = 1
		}
	}
	if found == 0 {
		return "", ErrAuthenticationFailed
	}
	return foundUser, nil
}
