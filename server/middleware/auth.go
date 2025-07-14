package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
)

// userIDKey is the context key for storing user ID
type contextKey string

const (
	userIDKey    contextKey = "userID"
	RequestIDKey contextKey = "request_id"
)

// V1AuthMiddleware creates middleware for API key authentication
func V1AuthMiddleware(authenticator auth.Authenticator, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				logger.Debug("Missing Authorization header")
				sendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
				return
			}

			// Authenticate the token
			userID, err := authenticator.Authenticate(r.Context(), authHeader)
			if err != nil {
				logger.Debug("Authentication failed", zap.Error(err))
				sendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
				return
			}

			// Store user ID in context
			ctx := context.WithValue(r.Context(), userIDKey, userID)
			r = r.WithContext(ctx)

			logger.Debug("User authenticated", zap.String("user_id", userID))

			next.ServeHTTP(w, r)
		})
	}
}

// V1RequestIDMiddleware adds a unique request ID to each request context
func V1RequestIDMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Generate a random request ID
			requestID := generateRequestID()

			// Add request ID to response header
			w.Header().Set("X-Request-ID", requestID)

			// Add request ID to context
			ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}

// generateRequestID creates a random request ID
func generateRequestID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GetUserID extracts the user ID from request context
func GetUserID(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDKey).(string)
	return userID, ok
}

// sendErrorResponse sends a JSON error response
func sendErrorResponse(w http.ResponseWriter, logger *zap.Logger, err error, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	var errorCode string
	switch err {
	case auth.ErrAuthenticationFailed:
		errorCode = "AUTHENTICATION_FAILED"
	case auth.ErrPermissionDenied:
		errorCode = "PERMISSION_DENIED"
	default:
		errorCode = "INTERNAL_ERROR"
	}

	response := map[string]string{
		"code":    errorCode,
		"message": err.Error(),
	}

	// For simplicity, write a basic JSON response
	// In production, you'd use a proper JSON encoder
	jsonResponse := `{"code":"` + response["code"] + `","message":"` + response["message"] + `"}`
	if _, err := w.Write([]byte(jsonResponse)); err != nil {
		logger.Error("Failed to write error response", zap.Error(err))
	}

	logger.Info("Error response sent",
		zap.String("error_code", errorCode),
		zap.Int("status_code", statusCode),
		zap.Error(err))
}
