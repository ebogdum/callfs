package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/server/middleware"
)

// V1PutFileEnhanced handles PUT /files/{path} requests with cross-server support
// @Summary Update file with cross-server support
// @Description Updates an existing file with new binary content, automatically routing to the correct server
// @Tags files
// @Security BearerAuth
// @Param path path string true "File path (no trailing slash)"
// @Param file body string true "File content (application/octet-stream)"
// @Success 200 "OK"
// @Success 201 "Created"
// @Failure 400 {object} ErrorResponse "Bad Request"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Forbidden"
// @Failure 404 {object} ErrorResponse "Not Found"
// @Failure 500 {object} ErrorResponse "Internal Server Error"
// @Failure 502 {object} ErrorResponse "Bad Gateway (cross-server proxy error)"
// @Router /v1/files/{path} [put]
func V1PutFileEnhanced(engine *core.Engine, authorizer auth.Authorizer, backendConfig *config.BackendConfig, cfg *config.ServerConfig, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract and parse path from URL
		urlPath := chi.URLParam(r, "*")
		pathInfo := ParseFilePath(urlPath)

		// PUT is only for files, not directories
		if pathInfo.IsDirectory {
			SendErrorResponse(w, logger,
				&customError{message: "PUT method cannot be used with directory paths (trailing slash)"},
				http.StatusBadRequest)
			return
		}

		// Get user ID from context
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}

		// Normalize path for engine calls
		enginePath := pathInfo.FullPath
		if pathInfo.IsDirectory && enginePath != "/" {
			enginePath = strings.TrimSuffix(enginePath, "/")
		}

		// Authorize write access FIRST
		if err := authorizer.Authorize(r.Context(), userID, enginePath, auth.WritePerm); err != nil {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		// Check if the target exists and determine location
		existingMd, err := engine.GetMetadata(r.Context(), enginePath)
		statusCode := http.StatusOK // Default for update
		currentInstanceID := engine.GetCurrentInstanceID()

		if err != nil {
			if err == metadata.ErrNotFound {
				// File doesn't exist, we'll create it locally
				statusCode = http.StatusCreated
				existingMd = &metadata.Metadata{
					Name:        pathInfo.Name,
					Type:        "file",
					Mode:        "0644",
					UID:         1000,
					GID:         1000,
					BackendType: backendConfig.DefaultBackend,
					ATime:       time.Now(),
					MTime:       time.Now(),
					CTime:       time.Now(),
				}

				// Create the file locally
				if err := engine.CreateFile(r.Context(), enginePath, r.Body, r.ContentLength, existingMd); err != nil {
					SendErrorResponse(w, logger, err, http.StatusInternalServerError)
					return
				}
			} else {
				SendErrorResponse(w, logger, err, http.StatusInternalServerError)
				return
			}
		} else {
			// File exists, check if it's actually a file
			if existingMd.Type != "file" {
				SendErrorResponse(w, logger,
					&customError{message: "cannot update directory with file content"},
					http.StatusBadRequest)
				return
			}
			// Check if file is on this instance or needs cross-server proxy
			if existingMd.CallFSInstanceID != nil && *existingMd.CallFSInstanceID != currentInstanceID {
				// File is on another server - use the internal proxy backend
				if err := engine.UpdateFileOnInstance(r.Context(), *existingMd.CallFSInstanceID, enginePath, r.Body, r.ContentLength); err != nil {
					logger.Error("Failed to update file via cross-server proxy",
						zap.String("instance_id", *existingMd.CallFSInstanceID),
						zap.String("path", enginePath),
						zap.Error(err))
					SendErrorResponse(w, logger, fmt.Errorf("failed to update file on remote server: %w", err), http.StatusBadGateway)
					return
				}

				// Proxy successful
				w.WriteHeader(http.StatusOK)
				logger.Info("File updated via cross-server proxy",
					zap.String("path", pathInfo.FullPath),
					zap.String("user_id", userID),
					zap.String("target_instance", *existingMd.CallFSInstanceID),
					zap.Int64("size", r.ContentLength))
				return
			}

			// File exists on this instance - update locally
			if err := engine.UpdateFile(r.Context(), enginePath, r.Body, r.ContentLength, existingMd); err != nil {
				SendErrorResponse(w, logger, err, http.StatusInternalServerError)
				return
			}
		}

		w.WriteHeader(statusCode)
		logger.Info("File updated locally",
			zap.String("path", pathInfo.FullPath),
			zap.String("user_id", userID),
			zap.Int64("size", r.ContentLength),
			zap.Int("status_code", statusCode))
	}
}
