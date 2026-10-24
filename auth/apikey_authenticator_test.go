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
	if want := positionalUserID("secret-key-1"); user != want {
		t.Errorf("expected %s, got: %s", want, user)
	}

	user, err = a.Authenticate(ctx, "secret-key-2")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if want := positionalUserID("secret-key-2"); user != want {
		t.Errorf("expected %s, got: %s", want, user)
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
	if want := positionalUserID("my-key"); user != want {
		t.Errorf("expected %s, got: %s", want, user)
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
	if want := positionalUserID("valid-key"); user != want {
		t.Errorf("expected %s (empty strings skipped), got: %s", want, user)
	}
}

func TestLegacyAliases(t *testing.T) {
	a := NewAPIKeyAuthenticator([]string{"key-a", "key-b"}, map[string]string{"alice": "alice-key"}, "")

	if got := a.LegacyAliases(positionalUserID("key-a")); 1 != len(got) || "api-user-0" != got[0] {
		t.Errorf("LegacyAliases(key-a) = %v, want [api-user-0]", got)
	}
	if got := a.LegacyAliases(positionalUserID("key-b")); 1 != len(got) || "api-user-1" != got[0] {
		t.Errorf("LegacyAliases(key-b) = %v, want [api-user-1]", got)
	}
	if got := a.LegacyAliases("alice"); 0 != len(got) {
		t.Errorf("LegacyAliases(alice) = %v, want none", got)
	}
}
