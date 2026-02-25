package auth

import (
	"context"
	"fmt"
	"strings"
)

// APIKeyAuthenticator implements authentication using static API keys
type APIKeyAuthenticator struct {
	validKeys            map[string]string
	internalProxySecret string
}

// NewAPIKeyAuthenticator creates a new API key authenticator
func NewAPIKeyAuthenticator(keys []string, internalProxySecret string) *APIKeyAuthenticator {
	validKeys := make(map[string]string)
	userIndex := 1
	for _, key := range keys {
		if key != "" {
			validKeys[key] = fmt.Sprintf("api-user-%d", userIndex)
			userIndex++
		}
	}

	return &APIKeyAuthenticator{
		validKeys:            validKeys,
		internalProxySecret: internalProxySecret,
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

	if a.internalProxySecret != "" && token == a.internalProxySecret {
		return "internal-proxy", nil
	}

	userID, ok := a.validKeys[token]
	if !ok {
		return "", ErrAuthenticationFailed
	}

	return userID, nil
}
