package auth

import (
	"context"
	"strings"
)

// APIKeyAuthenticator implements authentication using static API keys
type APIKeyAuthenticator struct {
	validKeys map[string]bool
}

// NewAPIKeyAuthenticator creates a new API key authenticator
func NewAPIKeyAuthenticator(keys []string, internalProxySecret string) *APIKeyAuthenticator {
	validKeys := make(map[string]bool)
	for _, key := range keys {
		if key != "" {
			validKeys[key] = true
		}
	}

	// Also add the internal proxy secret as a valid key
	if internalProxySecret != "" {
		validKeys[internalProxySecret] = true
	}

	return &APIKeyAuthenticator{
		validKeys: validKeys,
	}
}

// Authenticate validates a token and returns the associated user ID
func (a *APIKeyAuthenticator) Authenticate(ctx context.Context, token string) (string, error) {
	// Remove "Bearer " prefix if present
	token = strings.TrimPrefix(token, "Bearer ")
	token = strings.TrimSpace(token)

	if token == "" {
		return "", ErrAuthenticationFailed
	}

	if !a.validKeys[token] {
		return "", ErrAuthenticationFailed
	}

	// For now, return a fixed user ID for any valid API key
	// In a real implementation, you might map API keys to specific users
	// For testing, return "root" to bypass Unix permission checks
	return "root", nil
}
