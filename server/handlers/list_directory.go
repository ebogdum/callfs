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
func parseMaxDepth(r *http.Request) int {
	maxDepthStr := r.URL.Query().Get("max_depth")
	if maxDepthStr == "" {
		return 100
	}
	parsed, err := strconv.Atoi(maxDepthStr)
	if err != nil || parsed < 0 {
		return 100
	}
	if parsed > 1000 {
		return 1000
	}
	return parsed
}

func metadataToFileInfos(children []*metadata.Metadata) []FileInfo {
	fileInfos := make([]FileInfo, 0, len(children))
	for _, child := range children {
		fileInfos = append(fileInfos, FileInfo{
			Name:  child.Name,
			Path:  child.Path,
			Type:  child.Type,
			Size:  child.Size,
			Mode:  child.Mode,
			Owner: child.Owner,
			MTime: child.MTime.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return fileInfos
}

func V1ListDirectory(engine *core.Engine, authorizer auth.Authorizer, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			metrics.HTTPRequestDuration.WithLabelValues(r.Method, "/v1/directories/*").Observe(time.Since(start).Seconds())
		}()

		metadataCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		pathParam := chi.URLParam(r, "*")
		if pathParam == "" {
			pathParam = "/"
		} else {
			pathParam = "/" + pathParam
		}

		pathInfo := ParseFilePath(pathParam)
		if pathInfo.IsInvalid {
			SendErrorResponse(w, logger, fmt.Errorf("invalid path"), http.StatusBadRequest)
			return
		}

		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}

		enginePath := pathInfo.FullPath
		if pathInfo.IsDirectory && enginePath != "/" {
			enginePath = strings.TrimSuffix(enginePath, "/")
		}

		if err := authorizer.Authorize(metadataCtx, userID, enginePath, auth.ReadPerm); err != nil {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		md, err := engine.GetMetadata(metadataCtx, enginePath)
		if err != nil {
			SendErrorResponse(w, logger, err, http.StatusNotFound)
			return
		}

		if md.Type != "directory" {
			SendErrorResponse(w, logger, fmt.Errorf("path is not a directory"), http.StatusBadRequest)
			return
		}

		recursive := r.URL.Query().Get("recursive") == "true"
		maxDepth := parseMaxDepth(r)

		var children []*metadata.Metadata
		if recursive {
			children, err = engine.ListDirectoryRecursive(metadataCtx, enginePath, maxDepth)
		} else {
			children, err = engine.ListDirectory(metadataCtx, enginePath)
		}
		if err != nil {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}

		fileInfos := metadataToFileInfos(children)
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

		jsonBytes, marshalErr := json.Marshal(response)
		if marshalErr != nil {
			SendErrorResponse(w, logger, marshalErr, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-CallFS-Type", "directory")
		w.Header().Set("X-CallFS-Count", fmt.Sprintf("%d", len(fileInfos)))
		w.Header().Set("X-CallFS-Recursive", fmt.Sprintf("%t", recursive))

		if _, err := w.Write(jsonBytes); err != nil {
			logger.Error("Failed to write directory listing response", zap.Error(err))
		}

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
