package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
)

const (
	sessionTestAPIKey = "bearer-key"
	sessionTestUser   = "alice"
	sessionTestSecret = "0123456789abcdef0123456789abcdef"
	sessionTestKey    = "session-key"
)

type stubKeyAuthenticator struct{}

func (stubKeyAuthenticator) Authenticate(_ context.Context, token string) (string, error) {
	if "Bearer "+sessionTestAPIKey == token {
		return sessionTestUser, nil
	}
	return "", auth.ErrAuthenticationFailed
}

type stubFingerprints map[string]string

func (s stubFingerprints) UserForFingerprint(fingerprint string) (string, bool) {
	userID, ok := s[fingerprint]
	return userID, ok
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestV1AuthMiddlewareSessionCookieFlows(t *testing.T) {
	key := []byte(sessionTestSecret)
	manager := auth.NewSessionManager(key, time.Hour)
	expiredManager := auth.NewSessionManager(key, -time.Second)
	fingerprint := auth.Fingerprint(sessionTestKey)
	live := stubFingerprints{fingerprint: sessionTestUser}

	cookie, csrf, _, err := manager.Issue(sessionTestUser, fingerprint)
	if nil != err {
		t.Fatalf("issue session: %v", err)
	}
	expiredCookie, _, _, err := expiredManager.Issue(sessionTestUser, fingerprint)
	if nil != err {
		t.Fatalf("issue expired session: %v", err)
	}

	cases := []struct {
		name         string
		method       string
		cookie       string
		csrfHeader   string
		authHeader   string
		fingerprints stubFingerprints
		wantStatus   int
		wantReason   string
		wantCode     string
	}{
		{"cookie read needs no csrf", http.MethodGet, cookie, "", "", live, http.StatusOK, "", ""},
		{"cookie write with csrf", http.MethodPost, cookie, csrf, "", live, http.StatusOK, "", ""},
		{"cookie write without csrf", http.MethodPost, cookie, "", "", live, http.StatusForbidden, "csrf", "CSRF_FAILED"},
		{"cookie write with wrong csrf", http.MethodPut, cookie, "wrong", "", live, http.StatusForbidden, "csrf", "CSRF_FAILED"},
		{"revoked key ends session", http.MethodGet, cookie, "", "", stubFingerprints{}, http.StatusUnauthorized, "wrong_key", "AUTHENTICATION_FAILED"},
		{"expired session", http.MethodGet, expiredCookie, "", "", live, http.StatusUnauthorized, "expired_session", "AUTHENTICATION_FAILED"},
		{"tampered session", http.MethodGet, cookie + "x", "", "", live, http.StatusUnauthorized, "wrong_key", "AUTHENTICATION_FAILED"},
		{"no credentials", http.MethodGet, "", "", "", live, http.StatusUnauthorized, "missing_key", "AUTHENTICATION_FAILED"},
		{"bearer key still accepted", http.MethodGet, "", "", "Bearer " + sessionTestAPIKey, live, http.StatusOK, "", ""},
		{"bearer wrong key", http.MethodGet, "", "", "Bearer nope", live, http.StatusUnauthorized, "wrong_key", "AUTHENTICATION_FAILED"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reasons []string
			guard := V1AuthMiddleware(stubKeyAuthenticator{}, manager, tc.fingerprints, zap.NewNop(), func(reason string, _ *http.Request) {
				reasons = append(reasons, reason)
			})

			req := httptest.NewRequest(tc.method, "/v1/files/a.txt", nil)
			if "" != tc.cookie {
				req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tc.cookie})
			}
			if "" != tc.csrfHeader {
				req.Header.Set(csrfHeaderName, tc.csrfHeader)
			}
			if "" != tc.authHeader {
				req.Header.Set("Authorization", tc.authHeader)
			}

			rec := httptest.NewRecorder()
			guard(okHandler()).ServeHTTP(rec, req)

			if tc.wantStatus != rec.Code {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if "" != tc.wantCode && !strings.Contains(rec.Body.String(), tc.wantCode) {
				t.Fatalf("body %q does not contain %q", rec.Body.String(), tc.wantCode)
			}
			wantReasons := []string{}
			if "" != tc.wantReason {
				wantReasons = []string{tc.wantReason}
			}
			if !slices.Equal(reasons, wantReasons) {
				t.Fatalf("reported reasons = %v, want %v", reasons, wantReasons)
			}
		})
	}
}

func TestV1AuthMiddlewareSessionExposesClaims(t *testing.T) {
	manager := auth.NewSessionManager([]byte(sessionTestSecret), time.Hour)
	fingerprint := auth.Fingerprint(sessionTestKey)
	cookie, csrf, _, err := manager.Issue(sessionTestUser, fingerprint)
	if nil != err {
		t.Fatalf("issue session: %v", err)
	}

	var gotUser, gotCSRF string
	var gotFound bool
	capture := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetSession(r.Context())
		gotFound = ok
		gotCSRF = claims.CSRF
		gotUser, _ = GetUserID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	guard := V1AuthMiddleware(stubKeyAuthenticator{}, manager, stubFingerprints{fingerprint: sessionTestUser}, zap.NewNop(), nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookie})
	rec := httptest.NewRecorder()
	guard(capture).ServeHTTP(rec, req)

	if http.StatusOK != rec.Code {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !gotFound || gotCSRF != csrf || gotUser != sessionTestUser {
		t.Fatalf("session claims = (%v, %q, %q), want (true, %q, %q)", gotFound, gotCSRF, gotUser, csrf, sessionTestUser)
	}
}
