package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/core"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/server/middleware"
)

// V1DeleteFileEnhanced handles DELETE /files/{path} requests with cross-server support
// @Summary Delete file or directory with cross-server support
// @Description Deletes a file or directory, automatically routing to the correct server
// @Tags files
// @Security BearerAuth
// @Param path path string true "File or directory path"
// @Success 204 "No Content"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Forbidden"
// @Failure 404 {object} ErrorResponse "Not Found"
// @Failure 500 {object} ErrorResponse "Internal Server Error"
// @Failure 502 {object} ErrorResponse "Bad Gateway (cross-server proxy error)"
// @Router /v1/files/{path} [delete]
func V1DeleteFileEnhanced(engine *core.Engine, authorizer auth.Authorizer, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract and parse path from URL
		urlPath := chi.URLParam(r, "*")
		pathInfo := ParseFilePath(urlPath)
		if pathInfo.IsInvalid {
			SendErrorResponse(w, logger, &customError{message: "invalid path"}, http.StatusBadRequest)
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

		md, mdErr := engine.GetMetadata(r.Context(), enginePath)
		if nil != mdErr && !errors.Is(mdErr, metadata.ErrNotFound) {
			SendErrorResponse(w, logger, mdErr, http.StatusInternalServerError)
			return
		}

		if err := authorizer.AuthorizeWithMetadata(r.Context(), userID, enginePath, md, auth.DeletePerm); err != nil {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		if nil == md {
			SendErrorResponse(w, logger, mdErr, http.StatusNotFound)
			return
		}

		if "true" == r.URL.Query().Get("dry_run") {
			plan, planErr := engine.PlanDelete(r.Context(), enginePath)
			if planErr != nil {
				SendErrorResponse(w, logger, planErr, http.StatusInternalServerError)
				return
			}
			SendJSONResponse(w, plan)
			return
		}

		currentInstanceID := engine.GetCurrentInstanceID()

		// Check if file/directory is on this instance or needs cross-server proxy
		if md.CallFSInstanceID != nil && *md.CallFSInstanceID != currentInstanceID {
			// Resource is on another server - proxy the request
			proxyDelete := engine.DeleteFileOnInstance
			if "directory" == md.Type && "true" == r.URL.Query().Get("recursive") {
				proxyDelete = engine.DeleteTreeOnInstance
			}
			if err := proxyDelete(r.Context(), *md.CallFSInstanceID, enginePath); err != nil {
				if errors.Is(err, core.ErrDirectoryNotEmpty) {
					SendErrorResponse(w, logger, errFolderNotEmpty, http.StatusConflict)
					return
				}
				logger.Error("Failed to proxy DELETE request",
					zap.String("instance_id", *md.CallFSInstanceID),
					zap.String("path", enginePath),
					zap.Error(err))
				SendErrorResponse(w, logger, fmt.Errorf("failed to proxy request to owning server: %w", err), http.StatusBadGateway)
				return
			}

			// Proxy successful — now remove local metadata so it doesn't
			// point at a deleted remote file (orphan metadata).
			if delErr := engine.DeleteMetadataOnly(r.Context(), enginePath); nil != delErr && !errors.Is(delErr, metadata.ErrNotFound) {
				SendErrorResponse(w, logger, fmt.Errorf("remote file deleted but local metadata removal failed: %w", delErr), http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusNoContent)
			logger.Info("File/directory deleted via cross-server proxy",
				zap.String("path", pathInfo.FullPath),
				zap.String("user_id", userID),
				zap.String("target_instance", *md.CallFSInstanceID),
				zap.String("type", md.Type))
			return
		}

		// Resource exists on this instance - delete locally
		if md.Type == "directory" && r.URL.Query().Get("recursive") == "true" {
			deletedFiles, deletedDirs, delErr := engine.DeleteDirectoryRecursive(r.Context(), enginePath)
			if delErr != nil {
				SendErrorResponse(w, logger, delErr, http.StatusInternalServerError)
				return
			}
			SendJSONResponse(w, recursiveDeleteResponse{Deleted: deletedFiles, DeletedFiles: deletedFiles, DeletedDirectories: deletedDirs})
			logger.Info("Directory deleted recursively locally",
				zap.String("path", pathInfo.FullPath),
				zap.String("user_id", userID),
				zap.Int("deleted_files", deletedFiles), zap.Int("deleted_directories", deletedDirs))
			return
		}

		if err := engine.DeleteFile(r.Context(), enginePath); err != nil {
			if errors.Is(err, core.ErrDirectoryNotEmpty) {
				SendErrorResponse(w, logger, errFolderNotEmpty, http.StatusConflict)
				return
			}
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		logger.Info("File/directory deleted locally",
			zap.String("path", pathInfo.FullPath),
			zap.String("user_id", userID),
			zap.String("type", md.Type))
	}
}

// V1HeadFileEnhanced handles HEAD /files/{path} requests with cross-server support
// @Summary Get file metadata with cross-server support
// @Description Returns file metadata headers, automatically routing to the correct server
// @Tags files
// @Security BearerAuth
// @Param path path string true "File or directory path"
// @Success 200 "OK"
// @Header 200 {string} X-CallFS-Type "File type (file or directory)"
// @Header 200 {string} X-CallFS-Size "File size in bytes"
// @Header 200 {string} X-CallFS-Mode "File mode (permissions)"
// @Header 200 {string} X-CallFS-Owner "Resource owner (app user ID)"
// @Header 200 {string} X-CallFS-MTime "Last modified time"
// @Header 200 {string} X-CallFS-Instance-ID "Instance ID where file is located"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Forbidden"
// @Failure 404 {object} ErrorResponse "Not Found"
// @Failure 500 {object} ErrorResponse "Internal Server Error"
// @Failure 502 {object} ErrorResponse "Bad Gateway (cross-server proxy error)"
// @Router /v1/files/{path} [head]
func V1HeadFileEnhanced(engine *core.Engine, authorizer auth.Authorizer, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract and parse path from URL
		urlPath := chi.URLParam(r, "*")
		pathInfo := ParseFilePath(urlPath)
		if pathInfo.IsInvalid {
			SendErrorResponse(w, logger, &customError{message: "invalid path"}, http.StatusBadRequest)
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

		md, mdErr := engine.GetMetadata(r.Context(), enginePath)
		if nil != mdErr && !errors.Is(mdErr, metadata.ErrNotFound) {
			SendErrorResponse(w, logger, mdErr, http.StatusInternalServerError)
			return
		}

		if err := authorizer.AuthorizeWithMetadata(r.Context(), userID, enginePath, md, auth.ReadPerm); err != nil {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		if nil == md {
			SendErrorResponse(w, logger, mdErr, http.StatusNotFound)
			return
		}

		setMetadataHeaders(w, md)
		if "" != md.ETag {
			w.Header().Set("ETag", quotedETag(md.ETag))
		}
		w.WriteHeader(http.StatusOK)

		logger.Info("File metadata retrieved",
			zap.String("path", pathInfo.FullPath),
			zap.String("user_id", userID),
			zap.String("type", md.Type))
	}
}

// setMetadataHeaders sets standard metadata headers for responses
func setMetadataHeaders(w http.ResponseWriter, md *metadata.Metadata) {
	if md.Type == "file" {
		w.Header().Set("Content-Type", "application/octet-stream")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	w.Header().Set("X-CallFS-Type", md.Type)
	w.Header().Set("X-CallFS-Size", fmt.Sprintf("%d", md.Size))
	w.Header().Set("X-CallFS-Mode", md.Mode)
	w.Header().Set("X-CallFS-Owner", md.Owner)
	w.Header().Set("X-CallFS-MTime", md.MTime.Format("2006-01-02T15:04:05Z07:00"))

	if md.CallFSInstanceID != nil {
		w.Header().Set("X-CallFS-Instance-ID", *md.CallFSInstanceID)
	}
}
