package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/internal/buildinfo"
	"github.com/ebogdum/callfs/v2/server/middleware"
)

const (
	sessionTestAliceKey       = "alice-key-0123456789"
	sessionTestInternalSecret = "internal-secret-0123456789abcdef0123"
)

func newSessionTestFixture(t *testing.T) (*auth.APIKeyAuthenticator, *auth.SessionManager, *auth.AdminSet) {
	t.Helper()
	authenticator := auth.NewAPIKeyAuthenticator(
		[]string{"positional-key-0123456789"},
		map[string]string{"alice": sessionTestAliceKey},
		sessionTestInternalSecret,
	)
	key, err := auth.DeriveSessionKey(sessionTestInternalSecret)
	if nil != err {
		t.Fatalf("DeriveSessionKey: %v", err)
	}
	return authenticator, auth.NewSessionManager(key, 8*time.Hour), auth.NewAdminSet([]string{"alice"})
}

func sessionTestChain(authenticator *auth.APIKeyAuthenticator, sessions *auth.SessionManager, handler http.Handler) http.Handler {
	return middleware.V1RequestIDMiddleware()(middleware.V1AuthMiddleware(authenticator, sessions, authenticator, zap.NewNop(), nil)(handler))
}

func TestV1CreateSessionIssuesCookieAndCSRF(t *testing.T) {
	authenticator, sessions, admins := newSessionTestFixture(t)
	handler := V1CreateSession(authenticator, sessions, admins, config.UIConfig{SessionTTL: 8 * time.Hour}, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/v1/session", strings.NewReader(`{"api_key":"`+sessionTestAliceKey+`"}`))
	rec := httptest.NewRecorder()
	handler(rec, req)
	if http.StatusOK != rec.Code {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body sessionCreateResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); nil != err {
		t.Fatalf("decode body: %v", err)
	}
	if "alice" != body.UserID || !body.IsAdmin || "" == body.CSRFToken {
		t.Fatalf("body = %+v", body)
	}
	cookies := rec.Result().Cookies()
	if 1 != len(cookies) || sessionCookieName != cookies[0].Name {
		t.Fatalf("cookies = %+v", cookies)
	}
	cookie := cookies[0]
	if !cookie.HttpOnly || http.SameSiteStrictMode != cookie.SameSite || 28800 != cookie.MaxAge || "/" != cookie.Path {
		t.Fatalf("cookie = %+v", cookie)
	}
}

func TestV1CreateSessionRejectsWrongKey(t *testing.T) {
	authenticator, sessions, admins := newSessionTestFixture(t)
	handler := V1CreateSession(authenticator, sessions, admins, config.UIConfig{SessionTTL: 8 * time.Hour}, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/v1/session", strings.NewReader(`{"api_key":"wrong-key-0123456789"}`))
	rec := httptest.NewRecorder()
	handler(rec, req)
	if http.StatusUnauthorized != rec.Code {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); nil != err {
		t.Fatalf("decode body: %v", err)
	}
	if "AUTHENTICATION_FAILED" != body.Code {
		t.Fatalf("code = %q", body.Code)
	}
	if 0 != len(rec.Result().Cookies()) {
		t.Fatalf("failed sign-in set a cookie")
	}
}

func TestV1CreateSessionRejectsOversizeBody(t *testing.T) {
	authenticator, sessions, admins := newSessionTestFixture(t)
	handler := V1CreateSession(authenticator, sessions, admins, config.UIConfig{SessionTTL: 8 * time.Hour}, zap.NewNop())
	req := httptest.NewRequest(http.MethodPost, "/v1/session", strings.NewReader(`{"api_key":"`+strings.Repeat("a", 5000)+`"}`))
	rec := httptest.NewRecorder()
	handler(rec, req)
	if http.StatusRequestEntityTooLarge != rec.Code {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestV1DeleteSessionClearsCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	V1DeleteSession(nil, zap.NewNop())(rec, httptest.NewRequest(http.MethodDelete, "/v1/session", nil))
	if http.StatusNoContent != rec.Code {
		t.Fatalf("status = %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if 1 != len(cookies) || sessionCookieName != cookies[0].Name || 0 <= cookies[0].MaxAge {
		t.Fatalf("cookies = %+v", cookies)
	}
}

func TestV1MeReportsSessionAuth(t *testing.T) {
	authenticator, sessions, admins := newSessionTestFixture(t)
	cookieValue, csrfToken, expiresAt, err := sessions.Issue("alice", auth.Fingerprint(sessionTestAliceKey))
	if nil != err {
		t.Fatalf("Issue: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: cookieValue})
	rec := httptest.NewRecorder()
	sessionTestChain(authenticator, sessions, V1Me(admins, config.UIConfig{IdleTimeout: time.Hour})).ServeHTTP(rec, req)
	if http.StatusOK != rec.Code {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body meResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); nil != err {
		t.Fatalf("decode body: %v", err)
	}
	if "session" != body.AuthMethod || nil == body.CSRFToken || csrfToken != *body.CSRFToken || nil == body.SessionExpiresAt || !body.SessionExpiresAt.Equal(expiresAt) || !body.IsAdmin || "alice" != body.UserID {
		t.Fatalf("body = %+v", body)
	}
}

func TestV1MeReportsAPIKeyAuth(t *testing.T) {
	authenticator, sessions, _ := newSessionTestFixture(t)
	admins := auth.NewAdminSet(nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+sessionTestAliceKey)
	rec := httptest.NewRecorder()
	sessionTestChain(authenticator, sessions, V1Me(admins, config.UIConfig{IdleTimeout: time.Hour})).ServeHTTP(rec, req)
	if http.StatusOK != rec.Code {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body meResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); nil != err {
		t.Fatalf("decode body: %v", err)
	}
	if "api_key" != body.AuthMethod || body.IsAdmin || nil != body.CSRFToken || nil != body.SessionExpiresAt || "alice" != body.UserID || "owner" != body.PermissionsModel {
		t.Fatalf("body = %+v", body)
	}
}

func TestV1CapabilitiesReportsFeaturesAndLimits(t *testing.T) {
	cfg := &config.AppConfig{
		Backend:       config.BackendConfig{DefaultBackend: "localfs"},
		MetadataStore: config.MetadataStoreConfig{Type: "sqlite"},
		DLM:           config.DLMConfig{Type: "local"},
		Trash:         config.TrashConfig{Enabled: true, Retention: 720 * time.Hour},
		Erasure:       config.ErasureConfig{DataShards: 4, ParityShards: 2},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/capabilities", nil)
	rec := httptest.NewRecorder()
	V1Capabilities(CapabilitiesSource{Config: cfg, InstanceID: "node-a"})(rec, req)
	if http.StatusOK != rec.Code {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body capabilitiesResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); nil != err {
		t.Fatalf("decode body: %v", err)
	}
	if buildinfo.Version != body.Version || "node-a" != body.InstanceID || nil != body.NodeID || 1 != len(body.Storages) || "localfs" != body.Storages[0] || 2592000 != body.TrashRetentionS || !body.Features.Trash || body.Features.Cluster || "sqlite" != body.FileIndex || 1000 != body.Limits.ListPageMax || 4 != body.ErasureDefaults.DataShards {
		t.Fatalf("body = %+v", body)
	}
}
