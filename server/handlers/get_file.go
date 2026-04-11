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
	"github.com/ebogdum/callfs/metadata"
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
	Owner string `json:"owner"`
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
// @Header 200 {string} X-CallFS-Owner "Resource owner (app user ID)"
// @Header 200 {string} X-CallFS-MTime "Last modified time"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Forbidden"
// @Failure 404 {object} ErrorResponse "Not Found"
// @Failure 500 {object} ErrorResponse "Internal Server Error"
// @Router /v1/files/{path} [get]
func serveFileContent(w http.ResponseWriter, r *http.Request, engine *core.Engine, md *metadata.Metadata, metadataCtx, fileCtx context.Context, enginePath string, pathInfo PathInfo, userID string, logger *zap.Logger) {
	if md.ErasureCoded {
		em := engine.GetErasureManager()
		if em != nil {
			if r.URL.Query().Get("manifest") == "true" {
				HandleErasureManifest(w, r, em, enginePath, logger)
				return
			}
			HandleErasureDownload(w, r, em, enginePath, md.Size, logger)
			metrics.FileOperationsTotal.WithLabelValues("read", "erasure").Inc()
			return
		}
	}

	currentInstanceID := engine.GetCurrentInstanceID()
	if md.CallFSInstanceID != nil && *md.CallFSInstanceID != currentInstanceID {
		if freshMd, freshErr := engine.GetMetadataUncached(metadataCtx, enginePath); freshErr == nil {
			md = freshMd
		}
	}

	reader, err := engine.GetFile(fileCtx, enginePath)
	if err != nil {
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", md.Size))
	w.Header().Set("X-CallFS-Type", "file")
	w.Header().Set("X-CallFS-Size", fmt.Sprintf("%d", md.Size))
	w.Header().Set("X-CallFS-Mode", md.Mode)
	w.Header().Set("X-CallFS-Owner", md.Owner)
	w.Header().Set("X-CallFS-MTime", md.MTime.Format("2006-01-02T15:04:05Z07:00"))

	if _, err := io.Copy(w, reader); err != nil {
		logger.Error("Failed to stream file content", zap.Error(err))
	}

	metrics.FileOperationsTotal.WithLabelValues("read", md.BackendType).Inc()

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
}

func serveDirectoryListing(w http.ResponseWriter, engine *core.Engine, md *metadata.Metadata, metadataCtx context.Context, enginePath string, pathInfo PathInfo, userID string, logger *zap.Logger) {
	children, err := engine.ListDirectory(metadataCtx, enginePath)
	if err != nil {
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}

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

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-CallFS-Type", "directory")
	w.Header().Set("X-CallFS-Size", "0")
	w.Header().Set("X-CallFS-Mode", md.Mode)
	w.Header().Set("X-CallFS-Owner", md.Owner)
	w.Header().Set("X-CallFS-MTime", md.MTime.Format("2006-01-02T15:04:05Z07:00"))

	jsonBytes, marshalErr := json.Marshal(fileInfos)
	if marshalErr != nil {
		SendErrorResponse(w, logger, marshalErr, http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(jsonBytes); err != nil {
		logger.Error("Failed to write directory listing", zap.Error(err))
	}

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

func V1GetFile(engine *core.Engine, authorizer auth.Authorizer, cfg *config.ServerConfig, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			metrics.HTTPRequestDuration.WithLabelValues(r.Method, "/files/*").Observe(time.Since(start).Seconds())
		}()

		metadataCtx, metadataCancel := context.WithTimeout(r.Context(), cfg.MetadataOpTimeout)
		defer metadataCancel()

		fileCtx, fileCancel := context.WithTimeout(r.Context(), cfg.FileOpTimeout)
		defer fileCancel()

		urlPath := chi.URLParam(r, "*")
		pathInfo := ParseFilePath(urlPath)
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

		switch md.Type {
		case "file":
			serveFileContent(w, r, engine, md, metadataCtx, fileCtx, enginePath, pathInfo, userID, logger)
		case "directory":
			serveDirectoryListing(w, engine, md, metadataCtx, enginePath, pathInfo, userID, logger)
		default:
			SendErrorResponse(w, logger, fmt.Errorf("unsupported resource type: %s", md.Type), http.StatusInternalServerError)
		}
	}
}
