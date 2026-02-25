package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/core/log"
	"github.com/ebogdum/callfs/metrics"
	"github.com/ebogdum/callfs/server/middleware"
)

// FileInfo represents file/directory information for JSON responses
type FileInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Type  string `json:"type"`
	Size  int64  `json:"size"`
	Mode  string `json:"mode"`
	UID   int    `json:"uid"`
	GID   int    `json:"gid"`
	MTime string `json:"mtime"`
}

// GetFile handles GET /files/{path} requests
// @Summary Get file or directory
// @Description Retrieves file content as octet-stream or directory listing as JSON
// @Tags files
// @Security BearerAuth
// @Param path path string true "File or directory path"
// @Success 200 {object} []FileInfo "Directory listing (if path is directory)"
// @Success 200 {string} binary "File content (if path is file)"
// @Header 200 {string} X-CallFS-Size "File size in bytes"
// @Header 200 {string} X-CallFS-Mode "File mode (permissions)"
// @Header 200 {string} X-CallFS-UID "User ID"
// @Header 200 {string} X-CallFS-GID "Group ID"
// @Header 200 {string} X-CallFS-MTime "Last modified time"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Forbidden"
// @Failure 404 {object} ErrorResponse "Not Found"
// @Failure 500 {object} ErrorResponse "Internal Server Error"
// @Router /v1/files/{path} [get]
func V1GetFile(engine *core.Engine, authorizer auth.Authorizer, cfg *config.ServerConfig, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Track HTTP metrics
		defer func() {
			duration := time.Since(start)
			metrics.HTTPRequestDuration.WithLabelValues(r.Method, "/files/*").Observe(duration.Seconds())
		}()

		// Create context with timeout for metadata operations
		metadataCtx, metadataCancel := context.WithTimeout(r.Context(), cfg.MetadataOpTimeout)
		defer metadataCancel()

		// Create context with timeout for file operations (will be used later if needed)
		fileCtx, fileCancel := context.WithTimeout(r.Context(), cfg.FileOpTimeout)
		defer fileCancel()

		// Extract and parse path from URL
		urlPath := chi.URLParam(r, "*")
		pathInfo := ParseFilePath(urlPath)
		if pathInfo.IsInvalid {
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, "/files/*", "400").Inc()
			SendErrorResponse(w, logger, fmt.Errorf("invalid path"), http.StatusBadRequest)
			return
		}

		// Get user ID from context
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, "/files/*", "401").Inc()
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}

		// Normalize path for engine calls (remove trailing slash for directories)
		enginePath := pathInfo.FullPath
		if pathInfo.IsDirectory && enginePath != "/" {
			enginePath = strings.TrimSuffix(enginePath, "/")
		}

		// SECURITY FIX: Authorize BEFORE checking existence to prevent timing attacks
		if err := authorizer.Authorize(metadataCtx, userID, enginePath, auth.ReadPerm); err != nil {
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, "/files/*", "403").Inc()
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		// Now check if file/directory exists
		md, err := engine.GetMetadata(metadataCtx, enginePath)
		if err != nil {
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, "/files/*", "404").Inc()
			SendErrorResponse(w, logger, err, http.StatusNotFound)
			return
		}

		if md.Type == "file" {
			// Stream file content using file operation timeout
			reader, err := engine.GetFile(fileCtx, enginePath)
			if err != nil {
				metrics.HTTPRequestsTotal.WithLabelValues(r.Method, "/files/*", "500").Inc()
				SendErrorResponse(w, logger, err, http.StatusInternalServerError)
				return
			}
			defer reader.Close()

			// Set headers
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", md.Size))
			w.Header().Set("X-CallFS-Type", "file")
			w.Header().Set("X-CallFS-Size", fmt.Sprintf("%d", md.Size))
			w.Header().Set("X-CallFS-Mode", md.Mode)
			w.Header().Set("X-CallFS-UID", fmt.Sprintf("%d", md.UID))
			w.Header().Set("X-CallFS-GID", fmt.Sprintf("%d", md.GID))
			w.Header().Set("X-CallFS-MTime", md.MTime.Format("2006-01-02T15:04:05Z07:00"))

			// Stream content
			if _, err := io.Copy(w, reader); err != nil {
				logger.Error("Failed to stream file content", zap.Error(err))
			}

			// Track successful file operation
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, "/files/*", "200").Inc()
			metrics.FileOperationsTotal.WithLabelValues("read", md.BackendType).Inc()

			// Use secure logging with sanitized data
			logFields := log.LogFields{
				Path:      pathInfo.FullPath,
				UserID:    userID,
				Backend:   md.BackendType,
				Size:      md.Size,
				Operation: "download",
			}.Sanitize()

			logger.Info("File downloaded",
				zap.String("path", logFields.Path),
				zap.String("user_id", logFields.UserID),
				zap.String("backend", logFields.Backend),
				zap.Int64("size", logFields.Size))

		} else if md.Type == "directory" {
			// List directory contents using metadata timeout
			children, err := engine.ListDirectory(metadataCtx, enginePath)
			if err != nil {
				metrics.HTTPRequestsTotal.WithLabelValues(r.Method, "/files/*", "500").Inc()
				SendErrorResponse(w, logger, err, http.StatusInternalServerError)
				return
			}

			// Convert to response format
			var fileInfos []FileInfo
			for _, child := range children {
				fileInfo := FileInfo{
					Name:  child.Name,
					Path:  child.Path,
					Type:  child.Type,
					Size:  child.Size,
					Mode:  child.Mode,
					UID:   child.UID,
					GID:   child.GID,
					MTime: child.MTime.Format("2006-01-02T15:04:05Z07:00"),
				}
				fileInfos = append(fileInfos, fileInfo)
			}

			// Set headers
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-CallFS-Type", "directory")
			w.Header().Set("X-CallFS-Size", "0")
			w.Header().Set("X-CallFS-Mode", md.Mode)
			w.Header().Set("X-CallFS-UID", fmt.Sprintf("%d", md.UID))
			w.Header().Set("X-CallFS-GID", fmt.Sprintf("%d", md.GID))
			w.Header().Set("X-CallFS-MTime", md.MTime.Format("2006-01-02T15:04:05Z07:00"))

			// Send JSON response
			if err := json.NewEncoder(w).Encode(fileInfos); err != nil {
				SendErrorResponse(w, logger, err, http.StatusInternalServerError)
				return
			}

			// Track successful directory listing
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, "/files/*", "200").Inc()

			// Use secure logging with sanitized data
			logFields := log.LogFields{
				Path:      pathInfo.FullPath,
				UserID:    userID,
				Operation: "list",
			}.Sanitize()

			logger.Info("Directory listed",
				zap.String("path", logFields.Path),
				zap.String("user_id", logFields.UserID),
				zap.Int("children_count", len(children)))
		}
	}
}
