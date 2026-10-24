package middleware

import (
	"net/http"

	"github.com/ebogdum/callfs/v2/auth"
)

var adminRequiredBody = []byte(`{"code":"ADMIN_REQUIRED","message":"Admin access required"}`)

func RequireAdmin(admins *auth.AdminSet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := GetUserID(r.Context())
			if !ok {
				writeJSONBody(w, http.StatusUnauthorized, authenticationFailedBody)
				return
			}
			if !admins.IsAdmin(userID) {
				writeJSONBody(w, http.StatusForbidden, adminRequiredBody)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeJSONBody(w http.ResponseWriter, statusCode int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}
