package auth

import (
	"errors"
	"testing"
	"time"
)

func newTestSessionManager(t *testing.T, secret string, ttl time.Duration) *SessionManager {
	t.Helper()
	key, err := DeriveSessionKey(secret)
	if nil != err {
		t.Fatalf("DeriveSessionKey: %v", err)
	}
	return NewSessionManager(key, ttl)
}

func TestSessionIssueVerifyRoundTrip(t *testing.T) {
	m := newTestSessionManager(t, "internal-secret", time.Hour)

	cookie, csrf, exp, err := m.Issue("alice", "sha256:0123456789abcdef")
	if nil != err {
		t.Fatalf("Issue: %v", err)
	}
	if "" == cookie || "" == csrf {
		t.Fatalf("Issue returned empty cookie or csrf")
	}
	if !exp.After(time.Now()) {
		t.Errorf("expires_at %v is not in the future", exp)
	}

	claims, err := m.Verify(cookie)
	if nil != err {
		t.Fatalf("Verify: %v", err)
	}
	if "alice" != claims.UserID {
		t.Errorf("UserID = %q, want alice", claims.UserID)
	}
	if "sha256:0123456789abcdef" != claims.Fingerprint {
		t.Errorf("Fingerprint = %q", claims.Fingerprint)
	}
	if csrf != claims.CSRF {
		t.Errorf("CSRF = %q, want %q", claims.CSRF, csrf)
	}
}

func TestSessionRejectsOtherSecret(t *testing.T) {
	issuer := newTestSessionManager(t, "secret-one", time.Hour)
	verifier := newTestSessionManager(t, "secret-two", time.Hour)

	cookie, _, _, err := issuer.Issue("alice", "sha256:x")
	if nil != err {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := verifier.Verify(cookie); !errors.Is(err, ErrInvalidSession) {
		t.Errorf("Verify error = %v, want ErrInvalidSession", err)
	}
}

func TestSessionRejectsMalformedCookie(t *testing.T) {
	m := newTestSessionManager(t, "secret", time.Hour)

	for _, cookie := range []string{"", "nodot", ".", "abc.", ".abc"} {
		if _, err := m.Verify(cookie); !errors.Is(err, ErrInvalidSession) {
			t.Errorf("Verify(%q) error = %v, want ErrInvalidSession", cookie, err)
		}
	}
}

func TestSessionExpires(t *testing.T) {
	m := newTestSessionManager(t, "secret", time.Hour)
	base := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return base }

	cookie, _, exp, err := m.Issue("alice", "sha256:x")
	if nil != err {
		t.Fatalf("Issue: %v", err)
	}
	if want := base.Add(time.Hour); !exp.Equal(want) {
		t.Errorf("expires_at = %v, want %v", exp, want)
	}

	m.now = func() time.Time { return base.Add(59 * time.Minute) }
	if _, err := m.Verify(cookie); nil != err {
		t.Errorf("Verify before expiry: %v", err)
	}

	m.now = func() time.Time { return base.Add(time.Hour) }
	if _, err := m.Verify(cookie); !errors.Is(err, ErrSessionExpired) {
		t.Errorf("Verify at expiry error = %v, want ErrSessionExpired", err)
	}
}

func TestSessionIssueWithoutKeyFails(t *testing.T) {
	m := NewSessionManager(nil, time.Hour)

	if _, _, _, err := m.Issue("alice", "sha256:x"); !errors.Is(err, ErrSessionKeyMissing) {
		t.Errorf("Issue error = %v, want ErrSessionKeyMissing", err)
	}
	if _, err := m.Verify("anything.sig"); !errors.Is(err, ErrSessionKeyMissing) {
		t.Errorf("Verify error = %v, want ErrSessionKeyMissing", err)
	}
}

func TestDeriveSessionKeyDependsOnSecret(t *testing.T) {
	first, err := DeriveSessionKey("secret-one")
	if nil != err {
		t.Fatalf("DeriveSessionKey: %v", err)
	}
	second, err := DeriveSessionKey("secret-two")
	if nil != err {
		t.Fatalf("DeriveSessionKey: %v", err)
	}
	if 32 != len(first) {
		t.Errorf("key length = %d, want 32", len(first))
	}
	if string(first) == string(second) {
		t.Errorf("keys for different secrets are equal")
	}
}
