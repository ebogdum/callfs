package auth

import (
	"context"
	"testing"
)

// TestExplicitUserIDsSurviveKeyRemoval is the regression guard for the positional
// identity hazard.
//
// With the legacy api_keys list, a key's user ID is its index ("api-user-N").
// That ID is what gets persisted as a resource's Owner, so revoking one key
// shifts every later key's identity and silently hands the revoked user's files
// to whoever holds the next key. api_key_users binds identity to a name instead,
// so revocation leaves the other users' identities untouched.
func TestExplicitUserIDsSurviveKeyRemoval(t *testing.T) {
	ctx := context.Background()

	before := NewAPIKeyAuthenticator(nil, map[string]string{
		"alice": "alice-key-at-least-16-chars",
		"bob":   "bob-key-at-least-16-chars",
		"carol": "carol-key-at-least-16-chars",
	}, "")

	bobBefore, err := before.Authenticate(ctx, "bob-key-at-least-16-chars")
	if err != nil {
		t.Fatalf("authenticate bob: %v", err)
	}

	// Alice's key is revoked.
	after := NewAPIKeyAuthenticator(nil, map[string]string{
		"bob":   "bob-key-at-least-16-chars",
		"carol": "carol-key-at-least-16-chars",
	}, "")

	bobAfter, err := after.Authenticate(ctx, "bob-key-at-least-16-chars")
	if err != nil {
		t.Fatalf("authenticate bob after revocation: %v", err)
	}
	if bobBefore != bobAfter {
		t.Errorf("bob's identity changed from %q to %q after another user's key was revoked", bobBefore, bobAfter)
	}
	if bobAfter != "bob" {
		t.Errorf("bob authenticated as %q, want \"bob\"", bobAfter)
	}

	// Alice's key must no longer authenticate at all.
	if _, err := after.Authenticate(ctx, "alice-key-at-least-16-chars"); err == nil {
		t.Error("revoked key still authenticates")
	}
}

// TestPositionalIdentityShiftsOnRemoval documents the legacy behavior that
// api_key_users exists to avoid. It is kept deliberately: the positional form
// still works unchanged for existing deployments, and this test pins that
// contract so the hazard cannot be reintroduced silently under a new name.
func TestPositionalIdentityShiftsOnRemoval(t *testing.T) {
	ctx := context.Background()

	before := NewAPIKeyAuthenticator([]string{"key-alice", "key-bob"}, nil, "")
	bobBefore, err := before.Authenticate(ctx, "key-bob")
	if err != nil {
		t.Fatalf("authenticate bob: %v", err)
	}
	if bobBefore != "api-user-1" {
		t.Fatalf("bob = %q, want \"api-user-1\"", bobBefore)
	}

	after := NewAPIKeyAuthenticator([]string{"key-bob"}, nil, "")
	bobAfter, err := after.Authenticate(ctx, "key-bob")
	if err != nil {
		t.Fatalf("authenticate bob after removal: %v", err)
	}
	if bobAfter != "api-user-0" {
		t.Fatalf("bob = %q, want \"api-user-0\"", bobAfter)
	}
	if bobBefore == bobAfter {
		t.Error("expected positional identity to shift; the hazard this documents no longer reproduces")
	}
}

// TestExplicitUsersCoexistWithPositionalKeys checks both forms can be configured
// together, which is what a migration from api_keys to api_key_users looks like.
func TestExplicitUsersCoexistWithPositionalKeys(t *testing.T) {
	ctx := context.Background()

	a := NewAPIKeyAuthenticator(
		[]string{"legacy-key-at-least-16-chars"},
		map[string]string{"alice": "alice-key-at-least-16-chars"},
		"internal-secret-16-chars",
	)

	cases := map[string]string{
		"legacy-key-at-least-16-chars": "api-user-0",
		"alice-key-at-least-16-chars":  "alice",
		"internal-secret-16-chars":     "internal-proxy",
	}
	for token, wantUser := range cases {
		got, err := a.Authenticate(ctx, token)
		if err != nil {
			t.Errorf("Authenticate(%q) error: %v", token, err)
			continue
		}
		if got != wantUser {
			t.Errorf("Authenticate(%q) = %q, want %q", token, got, wantUser)
		}
	}
}

// TestExplicitUserTakesPrecedence confirms that when the same key appears in
// both forms, the explicit identity wins rather than the positional one. The
// config loader rejects this overlap, so this only pins the library's behavior
// for callers that construct the authenticator directly.
func TestExplicitUserTakesPrecedence(t *testing.T) {
	a := NewAPIKeyAuthenticator(
		[]string{"shared-key-at-least-16-chars"},
		map[string]string{"alice": "shared-key-at-least-16-chars"},
		"",
	)
	got, err := a.Authenticate(context.Background(), "shared-key-at-least-16-chars")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got != "alice" {
		t.Errorf("Authenticate = %q, want \"alice\" (explicit identity must win)", got)
	}
}

// TestEmptyExplicitEntriesIgnored confirms blank IDs or keys never become valid
// credentials.
func TestEmptyExplicitEntriesIgnored(t *testing.T) {
	a := NewAPIKeyAuthenticator(nil, map[string]string{
		"":      "orphan-key-at-least-16-chars",
		"alice": "",
	}, "")

	if _, err := a.Authenticate(context.Background(), "orphan-key-at-least-16-chars"); err == nil {
		t.Error("key with empty user ID authenticated")
	}
	if _, err := a.Authenticate(context.Background(), ""); err == nil {
		t.Error("empty token authenticated")
	}
}
