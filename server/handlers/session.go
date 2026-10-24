package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/config"
)

const (
	sessionCookieName      = "callfs_session"
	sessionCreateBodyLimit = 4096
	bearerPrefix           = "Bearer "
)

type sessionCreateRequest struct {
	APIKey string `json:"api_key"`
}

type sessionCreateResponse struct {
	UserID    string    `json:"user_id"`
	IsAdmin   bool      `json:"is_admin"`
	ExpiresAt time.Time `json:"expires_at"`
	CSRFToken string    `json:"csrf_token"`
}

func V1CreateSession(authenticator auth.Authenticator, sessions *auth.SessionManager, admins *auth.AdminSet, uiConfig config.UIConfig, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request sessionCreateRequest
		body := http.MaxBytesReader(w, r.Body, sessionCreateBodyLimit)
		if err := json.NewDecoder(body).Decode(&request); nil != err {
			status := http.StatusBadRequest
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				status = http.StatusRequestEntityTooLarge
			}
			SendErrorResponse(w, logger, err, status)
			return
		}

		apiKey := normalizeAPIKey(request.APIKey)
		userID, err := authenticator.Authenticate(r.Context(), apiKey)
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}

		cookieValue, csrfToken, expiresAt, err := sessions.Issue(userID, auth.Fingerprint(apiKey))
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, sessionCookie(r, cookieValue, int(uiConfig.SessionTTL/time.Second)))
		SendJSONResponse(w, sessionCreateResponse{
			UserID:    userID,
			IsAdmin:   admins.IsAdmin(userID),
			ExpiresAt: expiresAt,
			CSRFToken: csrfToken,
		})
		logger.Info("Session started", zap.String("user_id", userID))
	}
}

func V1DeleteSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, sessionCookie(r, "", -1))
		w.WriteHeader(http.StatusNoContent)
	}
}

func sessionCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   nil != r.TLS,
		SameSite: http.SameSiteStrictMode,
	}
}

func normalizeAPIKey(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(bearerPrefix) <= len(trimmed) && strings.EqualFold(bearerPrefix, trimmed[:len(bearerPrefix)]) {
		trimmed = strings.TrimSpace(trimmed[len(bearerPrefix):])
	}
	return trimmed
}
