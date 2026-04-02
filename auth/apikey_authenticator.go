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
// The internalProxySecret is registered as a valid key with the "internal-proxy" user ID
// so cross-server operations (UpdateFileOnInstance, etc.) can authenticate on peers.
func NewAPIKeyAuthenticator(keys []string, internalProxySecret string) *APIKeyAuthenticator {
	validKeys := make(map[string]string)
	userIndex := 1
	for _, key := range keys {
		if key != "" {
			validKeys[key] = fmt.Sprintf("api-user-%d", userIndex)
			userIndex++
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
	var foundUID string
	found := 0
	for key, uid := range a.validKeys {
		if subtle.ConstantTimeCompare([]byte(token), []byte(key)) == 1 {
			foundUID = uid
			found = 1
		}
	}
	if found == 0 {
		return "", ErrAuthenticationFailed
	}
	return foundUID, nil
}
