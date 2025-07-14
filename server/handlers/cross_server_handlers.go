package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/server/middleware"
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

		// Authorize delete access FIRST
		if err := authorizer.Authorize(r.Context(), userID, enginePath, auth.DeletePerm); err != nil {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		// Get metadata to check if it exists and determine location
		md, err := engine.GetMetadata(r.Context(), enginePath)
		if err != nil {
			SendErrorResponse(w, logger, err, http.StatusNotFound)
			return
		}

		currentInstanceID := engine.GetCurrentInstanceID()

		// Check if file/directory is on this instance or needs cross-server proxy
		if md.CallFSInstanceID != nil && *md.CallFSInstanceID != currentInstanceID {
			// Resource is on another server - proxy the request
			if err := engine.DeleteFileOnInstance(r.Context(), *md.CallFSInstanceID, enginePath); err != nil {
				logger.Error("Failed to proxy DELETE request",
					zap.String("instance_id", *md.CallFSInstanceID),
					zap.String("path", enginePath),
					zap.Error(err))
				SendErrorResponse(w, logger, fmt.Errorf("failed to proxy request to owning server: %w", err), http.StatusBadGateway)
				return
			}

			// Proxy successful
			w.WriteHeader(http.StatusNoContent)
			logger.Info("File/directory deleted via cross-server proxy",
				zap.String("path", pathInfo.FullPath),
				zap.String("user_id", userID),
				zap.String("target_instance", *md.CallFSInstanceID),
				zap.String("type", md.Type))
			return
		}

		// Resource exists on this instance - delete locally
		if err := engine.DeleteFile(r.Context(), enginePath); err != nil {
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
// @Header 200 {string} X-CallFS-UID "User ID"
// @Header 200 {string} X-CallFS-GID "Group ID"
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

		// Authorize read access FIRST
		if err := authorizer.Authorize(r.Context(), userID, enginePath, auth.ReadPerm); err != nil {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		// Get metadata to check location
		md, err := engine.GetMetadata(r.Context(), enginePath)
		if err != nil {
			SendErrorResponse(w, logger, err, http.StatusNotFound)
			return
		}

		currentInstanceID := engine.GetCurrentInstanceID()

		// Check if file/directory is on this instance or needs cross-server proxy
		if md.CallFSInstanceID != nil && *md.CallFSInstanceID != currentInstanceID {
			// Resource is on another server - proxy the request to get metadata
			remoteMd, err := engine.StatFileOnInstance(r.Context(), *md.CallFSInstanceID, enginePath)
			if err != nil {
				logger.Error("Failed to proxy HEAD request",
					zap.String("instance_id", *md.CallFSInstanceID),
					zap.String("path", enginePath),
					zap.Error(err))
				SendErrorResponse(w, logger, fmt.Errorf("failed to proxy request to owning server: %w", err), http.StatusBadGateway)
				return
			}

			// Set headers from remote metadata and add instance info
			setMetadataHeaders(w, remoteMd)
			w.Header().Set("X-CallFS-Instance-ID", *md.CallFSInstanceID)
			w.WriteHeader(http.StatusOK)

			logger.Info("File metadata retrieved via cross-server proxy",
				zap.String("path", pathInfo.FullPath),
				zap.String("user_id", userID),
				zap.String("target_instance", *md.CallFSInstanceID))
			return
		}

		// Resource exists on this instance - return metadata headers
		setMetadataHeaders(w, md)
		w.WriteHeader(http.StatusOK)

		logger.Info("File metadata retrieved locally",
			zap.String("path", pathInfo.FullPath),
			zap.String("user_id", userID),
			zap.String("type", md.Type))
	}
}

// setMetadataHeaders sets standard metadata headers for responses
func setMetadataHeaders(w http.ResponseWriter, md *metadata.Metadata) {
	w.Header().Set("X-CallFS-Type", md.Type)
	w.Header().Set("X-CallFS-Size", fmt.Sprintf("%d", md.Size))
	w.Header().Set("X-CallFS-Mode", md.Mode)
	w.Header().Set("X-CallFS-UID", fmt.Sprintf("%d", md.UID))
	w.Header().Set("X-CallFS-GID", fmt.Sprintf("%d", md.GID))
	w.Header().Set("X-CallFS-MTime", md.MTime.Format("2006-01-02T15:04:05Z07:00"))

	if md.CallFSInstanceID != nil {
		w.Header().Set("X-CallFS-Instance-ID", *md.CallFSInstanceID)
	}
}
