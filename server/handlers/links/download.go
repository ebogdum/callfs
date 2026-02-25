package links

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/links"
	"github.com/ebogdum/callfs/server/handlers"
)

// DownloadLinkHandler creates an HTTP handler for downloading files via single-use links.
// @Summary Download file via single-use link
// @Description Downloads a file using a single-use token. The token becomes invalid after one use.
// @Tags links
// @Param token path string true "Single-use download token"
// @Produce application/octet-stream
// @Success 200 {string} binary "File content"
// @Failure 400 {object} handlers.ErrorResponse "Bad Request"
// @Failure 404 {object} handlers.ErrorResponse "Token not found"
// @Failure 410 {object} handlers.ErrorResponse "Token expired or already used"
// @Failure 500 {object} handlers.ErrorResponse "Internal Server Error"
// @Router /download/{token} [get]
func V1DownloadLinkHandler(engine *core.Engine, manager *links.LinkManager, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract token from URL path
		token := chi.URLParam(r, "token")
		if token == "" {
			handlers.SendErrorResponse(w, logger, errors.New("missing token"), http.StatusBadRequest)
			return
		}

		// Get user IP address
		userIP := getUserIP(r)

		// Validate and invalidate the single-use link
		filePath, err := manager.ValidateAndInvalidateLink(ctx, token, userIP)
		if err != nil {
			logger.Warn("Invalid single-use link access attempt",
				zap.String("token", links.TruncateToken(token)),
				zap.String("user_ip", userIP),
				zap.Error(err))

			// Map link errors to appropriate HTTP status codes
			switch {
			case errors.Is(err, links.ErrLinkNotFound):
				handlers.SendErrorResponse(w, logger, err, http.StatusNotFound)
			case errors.Is(err, links.ErrLinkExpired):
				handlers.SendErrorResponse(w, logger, err, http.StatusGone)
			case errors.Is(err, links.ErrLinkInvalid):
				handlers.SendErrorResponse(w, logger, err, http.StatusGone)
			default:
				handlers.SendErrorResponse(w, logger, errors.New("link validation failed"), http.StatusInternalServerError)
			}
			return
		}

		// Get file from the core engine
		reader, err := engine.GetFile(ctx, filePath)
		if err != nil {
			logger.Error("Failed to get file for single-use link",
				zap.String("token", links.TruncateToken(token)),
				zap.String("file_path", filePath),
				zap.String("user_ip", userIP),
				zap.Error(err))
			handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
		defer func() {
			if closer, ok := reader.(io.Closer); ok {
				closer.Close()
			}
		}()

		// Set appropriate headers for file download
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment")

		// Stream the file content
		_, err = io.Copy(w, reader)
		if err != nil {
			logger.Error("Failed to stream file content for single-use link",
				zap.String("token", links.TruncateToken(token)),
				zap.String("file_path", filePath),
				zap.String("user_ip", userIP),
				zap.Error(err))
			return
		}

		logger.Info("Successfully served file via single-use link",
			zap.String("token", links.TruncateToken(token)),
			zap.String("file_path", filePath),
			zap.String("user_ip", userIP))
	}
}

// getUserIP extracts the real user IP address from the request.
func getUserIP(r *http.Request) string {
	// Check X-Forwarded-For header (from load balancer/proxy)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return ip
	}

	return r.RemoteAddr
}
