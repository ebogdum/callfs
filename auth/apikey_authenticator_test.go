package auth

import (
	"context"
	"errors"
	"testing"
)

func TestAuthenticateValidKey(t *testing.T) {
	a := NewAPIKeyAuthenticator([]string{"secret-key-1", "secret-key-2"}, nil, "internal-secret")
	ctx := context.Background()

	user, err := a.Authenticate(ctx, "secret-key-1")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if user != "api-user-0" {
		t.Errorf("expected api-user-0, got: %s", user)
	}

	user, err = a.Authenticate(ctx, "secret-key-2")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if user != "api-user-1" {
		t.Errorf("expected api-user-1, got: %s", user)
	}
}

func TestAuthenticateInternalProxy(t *testing.T) {
	a := NewAPIKeyAuthenticator([]string{"key"}, nil, "proxy-secret")
	ctx := context.Background()

	user, err := a.Authenticate(ctx, "proxy-secret")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if user != "internal-proxy" {
		t.Errorf("expected internal-proxy, got: %s", user)
	}
}

func TestAuthenticateInvalidKey(t *testing.T) {
	a := NewAPIKeyAuthenticator([]string{"key"}, nil, "proxy")
	ctx := context.Background()

	_, err := a.Authenticate(ctx, "wrong-key")
	if !errors.Is(err, ErrAuthenticationFailed) {
		t.Errorf("expected ErrAuthenticationFailed, got: %v", err)
	}
}

func TestAuthenticateEmptyToken(t *testing.T) {
	a := NewAPIKeyAuthenticator([]string{"key"}, nil, "proxy")
	ctx := context.Background()

	_, err := a.Authenticate(ctx, "")
	if !errors.Is(err, ErrAuthenticationFailed) {
		t.Errorf("expected ErrAuthenticationFailed, got: %v", err)
	}
}

func TestAuthenticateBearerPrefix(t *testing.T) {
	a := NewAPIKeyAuthenticator([]string{"my-key"}, nil, "")
	ctx := context.Background()

	user, err := a.Authenticate(ctx, "Bearer my-key")
	if err != nil {
		t.Fatalf("expected success with Bearer prefix, got: %v", err)
	}
	if user != "api-user-0" {
		t.Errorf("expected api-user-0, got: %s", user)
	}
}

func TestAuthenticateEmptyKeys(t *testing.T) {
	// Empty strings in keys should be ignored
	a := NewAPIKeyAuthenticator([]string{"", "valid-key", ""}, nil, "")
	ctx := context.Background()

	user, err := a.Authenticate(ctx, "valid-key")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if user != "api-user-0" {
		t.Errorf("expected api-user-0 (empty strings skipped), got: %s", user)
	}
}
