package handlers

import (
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/server/middleware"
)

type meResponse struct {
	UserID           string     `json:"user_id"`
	IsAdmin          bool       `json:"is_admin"`
	AuthMethod       string     `json:"auth_method"`
	SessionExpiresAt *time.Time `json:"session_expires_at"`
	CSRFToken        *string    `json:"csrf_token"`
	PermissionsModel string     `json:"permissions_model"`
	IdleTimeoutSecs  int        `json:"idle_timeout_seconds"`
}

func V1Me(admins *auth.AdminSet, uiConfig config.UIConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, zap.L(), auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		response := meResponse{
			UserID:           userID,
			IsAdmin:          isAdminUser(userID, admins),
			AuthMethod:       "api_key",
			PermissionsModel: "owner",
			IdleTimeoutSecs:  int(uiConfig.IdleTimeout / time.Second),
		}
		if claims, isSession := middleware.GetSession(r.Context()); isSession {
			expiresAt := claims.ExpiresAt
			csrfToken := claims.CSRF
			response.AuthMethod = "session"
			response.SessionExpiresAt = &expiresAt
			response.CSRFToken = &csrfToken
		}
		SendJSONResponse(w, response)
	}
}
