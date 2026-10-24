package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ebogdum/callfs/auth"
)

func TestRequireAdminStatusByIdentity(t *testing.T) {
	admins := auth.NewAdminSet([]string{"alice"})

	cases := []struct {
		name       string
		userID     string
		authed     bool
		wantStatus int
		wantCode   string
	}{
		{"admin user", "alice", true, http.StatusOK, ""},
		{"root is admin", "root", true, http.StatusOK, ""},
		{"member is refused", "bob", true, http.StatusForbidden, "ADMIN_REQUIRED"},
		{"unauthenticated is refused", "", false, http.StatusUnauthorized, "AUTHENTICATION_FAILED"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			guard := RequireAdmin(admins)(okHandler())

			req := httptest.NewRequest(http.MethodGet, "/v1/admin/status", nil)
			if tc.authed {
				scope := &requestScope{userID: tc.userID, authenticated: true}
				req = req.WithContext(context.WithValue(req.Context(), requestScopeKey, scope))
			}

			rec := httptest.NewRecorder()
			guard.ServeHTTP(rec, req)

			if tc.wantStatus != rec.Code {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if "" != tc.wantCode && !strings.Contains(rec.Body.String(), tc.wantCode) {
				t.Fatalf("body %q does not contain %q", rec.Body.String(), tc.wantCode)
			}
		})
	}
}
