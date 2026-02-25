package links

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/links"
	"github.com/ebogdum/callfs/server/handlers"
	"github.com/ebogdum/callfs/server/middleware"
	"go.uber.org/zap"
)

// GenerateLinkRequest represents the request payload for generating a single-use link.
type GenerateLinkRequest struct {
	Path          string `json:"path" example:"/path/to/file"`
	ExpirySeconds int    `json:"expiry_seconds" example:"3600"`
}

// GenerateLinkResponse represents the response payload containing the generated link.
type GenerateLinkResponse struct {
	URL     string    `json:"url" example:"https://localhost:8443/download/token123"`
	Token   string    `json:"token" example:"token123"`
	Expires time.Time `json:"expires" example:"2025-07-13T13:34:56Z"`
}

// GenerateLinkHandler creates an HTTP handler for generating single-use download links.
// @Summary Generate single-use download link
// @Description Creates a secure, time-limited, single-use download link for a file
// @Tags links
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body GenerateLinkRequest true "Link generation request"
// @Success 201 {object} GenerateLinkResponse "Link generated successfully"
// @Failure 400 {object} handlers.ErrorResponse "Bad Request"
// @Failure 401 {object} handlers.ErrorResponse "Unauthorized"
// @Failure 500 {object} handlers.ErrorResponse "Internal Server Error"
// @Router /v1/links/generate [post]
func V1GenerateLinkHandler(manager *links.LinkManager, authorizer auth.Authorizer, apiHost string, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		userID, ok := middleware.GetUserID(ctx)
		if !ok {
			handlers.SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}

		// Parse JSON request
		var req GenerateLinkRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Warn("Invalid JSON in link generation request", zap.Error(err))
			handlers.SendErrorResponse(w, logger, errors.New("invalid JSON in request body"), http.StatusBadRequest)
			return
		}

		// Validate request
		if req.Path == "" {
			handlers.SendErrorResponse(w, logger, errors.New("path is required"), http.StatusBadRequest)
			return
		}

		pathInfo := handlers.ParseFilePath(strings.TrimPrefix(req.Path, "/"))
		if pathInfo.IsInvalid {
			handlers.SendErrorResponse(w, logger, errors.New("invalid path"), http.StatusBadRequest)
			return
		}

		enginePath := pathInfo.FullPath
		if pathInfo.IsDirectory && enginePath != "/" {
			enginePath = strings.TrimSuffix(enginePath, "/")
		}

		if err := authorizer.Authorize(ctx, userID, enginePath, auth.ReadPerm); err != nil {
			handlers.SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		if req.ExpirySeconds <= 0 || req.ExpirySeconds > 86400 { // Max 24 hours
			handlers.SendErrorResponse(w, logger, errors.New("expiry must be between 1 and 86400 seconds"), http.StatusBadRequest)
			return
		}

		// Generate expiry duration
		expiryDuration := time.Duration(req.ExpirySeconds) * time.Second

		// Generate single-use link
		token, err := manager.GenerateLink(ctx, enginePath, expiryDuration)
		if err != nil {
			logger.Error("Failed to generate single-use link",
				zap.String("path", enginePath),
				zap.Int("expiry_seconds", req.ExpirySeconds),
				zap.Error(err))
			handlers.SendErrorResponse(w, logger, errors.New("failed to generate download link"), http.StatusInternalServerError)
			return
		}

		// Build full download URL
		downloadURL := fmt.Sprintf("https://%s/download/%s", apiHost, token)

		// Prepare response
		response := GenerateLinkResponse{
			URL:     downloadURL,
			Token:   token,
			Expires: time.Now().Add(expiryDuration),
		}

		// Send JSON response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Error("Failed to encode response", zap.Error(err))
			return
		}

		logger.Info("Generated single-use download link",
			zap.String("path", enginePath),
			zap.String("user_id", userID),
			zap.String("token", links.TruncateToken(token)),
			zap.String("url", downloadURL),
			zap.Duration("expiry", expiryDuration))
	}
}
