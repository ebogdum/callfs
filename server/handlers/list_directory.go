package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/core/log"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/metrics"
	"github.com/ebogdum/callfs/server/middleware"
)

// DirectoryListingResponse represents the response for directory listing operations
type DirectoryListingResponse struct {
	Path      string     `json:"path"`
	Type      string     `json:"type"` // "directory"
	Recursive bool       `json:"recursive"`
	MaxDepth  int        `json:"max_depth,omitempty"`
	Count     int        `json:"count"`
	Items     []FileInfo `json:"items"`
}

// ListDirectory handles GET /api/directories/{path} requests
// @Summary List directory contents
// @Description Lists directory contents with optional recursive traversal
// @Tags directories
// @Security BearerAuth
// @Param path path string true "Directory path"
// @Param recursive query bool false "Recursively list subdirectories"
// @Param max_depth query int false "Maximum recursion depth (default: 100, max: 1000)"
// @Success 200 {object} DirectoryListingResponse "Directory listing"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Forbidden"
// @Failure 404 {object} ErrorResponse "Not Found"
// @Failure 400 {object} ErrorResponse "Bad Request"
// @Router /v1/directories/{path} [get]
func V1ListDirectory(engine *core.Engine, authorizer auth.Authorizer, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Start timing
		start := time.Now()
		defer func() {
			metrics.HTTPRequestDuration.WithLabelValues(r.Method, "/api/directories/*").Observe(time.Since(start).Seconds())
		}()

		// Create contexts with timeouts
		metadataCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		// Parse path from URL
		pathParam := chi.URLParam(r, "*")
		if pathParam == "" {
			pathParam = "/"
		} else {
			pathParam = "/" + pathParam
		}

		// Clean and validate path
		pathInfo := ParseFilePath(pathParam)
		if pathInfo.IsInvalid {
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, "/api/directories/*", "400").Inc()
			SendErrorResponse(w, logger, fmt.Errorf("invalid path"), http.StatusBadRequest)
			return
		}

		// Get user ID from context
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, "/api/directories/*", "401").Inc()
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}

		// Normalize path for engine calls (remove trailing slash for directories)
		enginePath := pathInfo.FullPath
		if pathInfo.IsDirectory && enginePath != "/" {
			enginePath = strings.TrimSuffix(enginePath, "/")
		}

		// Authorize access
		if err := authorizer.Authorize(metadataCtx, userID, enginePath, auth.ReadPerm); err != nil {
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, "/api/directories/*", "403").Inc()
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		// Check if path exists and is a directory
		md, err := engine.GetMetadata(metadataCtx, enginePath)
		if err != nil {
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, "/api/directories/*", "404").Inc()
			SendErrorResponse(w, logger, err, http.StatusNotFound)
			return
		}

		if md.Type != "directory" {
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, "/api/directories/*", "400").Inc()
			SendErrorResponse(w, logger, fmt.Errorf("path is not a directory"), http.StatusBadRequest)
			return
		}

		// Parse query parameters
		recursive := r.URL.Query().Get("recursive") == "true"
		maxDepthStr := r.URL.Query().Get("max_depth")
		maxDepth := 100 // Default

		if maxDepthStr != "" {
			if parsed, err := strconv.Atoi(maxDepthStr); err == nil {
				if parsed > 1000 {
					maxDepth = 1000 // Limit maximum depth
				} else if parsed >= 0 {
					maxDepth = parsed
				}
			}
		}

		var children []*metadata.Metadata
		if recursive {
			children, err = engine.ListDirectoryRecursive(metadataCtx, enginePath, maxDepth)
		} else {
			children, err = engine.ListDirectory(metadataCtx, enginePath)
		}

		if err != nil {
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, "/api/directories/*", "500").Inc()
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

		// Create response
		response := DirectoryListingResponse{
			Path:      enginePath,
			Type:      "directory",
			Recursive: recursive,
			Count:     len(fileInfos),
			Items:     fileInfos,
		}

		if recursive {
			response.MaxDepth = maxDepth
		}

		// Set headers
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-CallFS-Type", "directory")
		w.Header().Set("X-CallFS-Count", fmt.Sprintf("%d", len(fileInfos)))
		w.Header().Set("X-CallFS-Recursive", fmt.Sprintf("%t", recursive))

		// Send JSON response
		if err := json.NewEncoder(w).Encode(response); err != nil {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}

		// Track successful directory listing
		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, "/api/directories/*", "200").Inc()

		// Use secure logging with sanitized data
		logFields := log.LogFields{
			Path:      pathInfo.FullPath,
			UserID:    userID,
			Operation: "list_directory",
		}.Sanitize()

		logger.Info("Directory listed via API",
			zap.String("path", logFields.Path),
			zap.String("user_id", logFields.UserID),
			zap.Bool("recursive", recursive),
			zap.Int("max_depth", maxDepth),
			zap.Int("items_count", len(children)))
	}
}
