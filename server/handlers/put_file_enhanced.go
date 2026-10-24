package handlers

import (
	"encoding/json"
	"errors"
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
// putResolveTarget handles metadata lookup and decides whether to create, proxy, or update locally.
// Returns the resolved metadata, the HTTP status code, and whether the response was already written.
func sendWriteError(w http.ResponseWriter, logger *zap.Logger, err error, defaultStatusCode int) {
	var maxBytesErr *http.MaxBytesError
	if !errors.As(err, &maxBytesErr) {
		SendErrorResponse(w, logger, err, defaultStatusCode)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	jsonBytes, marshalErr := json.Marshal(ErrorResponse{Code: "PAYLOAD_TOO_LARGE", Message: maxBytesErr.Error()})
	if marshalErr != nil {
		logger.Error("Failed to marshal error response", zap.Error(marshalErr))
		return
	}
	if _, writeErr := w.Write(jsonBytes); writeErr != nil {
		logger.Error("Failed to write error response", zap.Error(writeErr))
	}
	logger.Debug("Error response sent",
		zap.String("error_code", "PAYLOAD_TOO_LARGE"),
		zap.Int("status_code", http.StatusRequestEntityTooLarge),
		zap.Error(err))
}

func putResolveTarget(w http.ResponseWriter, r *http.Request, engine *core.Engine, backendConfig *config.BackendConfig, enginePath string, pathInfo PathInfo, userID string, existingMd *metadata.Metadata, size int64, countReader *CountingReader, ifMatch string, ifNoneMatch bool, logger *zap.Logger) (*metadata.Metadata, int, bool) {
	currentInstanceID := engine.GetCurrentInstanceID()

	if nil == existingMd {
		newMd := &metadata.Metadata{
			Name:        pathInfo.Name,
			Type:        "file",
			Mode:        "0644",
			Owner:       userID,
			BackendType: backendConfig.DefaultBackend,
			ATime:       time.Now(),
			MTime:       time.Now(),
			CTime:       time.Now(),
		}
		committedMd, created, putErr := engine.PutFile(r.Context(), enginePath, r.Body, size, newMd, ifMatch, ifNoneMatch)
		if putErr != nil {
			sendWriteError(w, logger, putErr, http.StatusInternalServerError)
			return nil, 0, true
		}
		if !created {
			return committedMd, http.StatusOK, false
		}
		return committedMd, http.StatusCreated, false
	}

	if existingMd.Type != "file" {
		SendErrorResponse(w, logger,
			&customError{message: "cannot update directory with file content"},
			http.StatusBadRequest)
		return nil, 0, true
	}
	if existingMd.CallFSInstanceID != nil && *existingMd.CallFSInstanceID != currentInstanceID {
		handleCrossServerPut(w, r, engine, existingMd, enginePath, pathInfo, userID, size, countReader, ifMatch, ifNoneMatch, logger)
		return nil, 0, true
	}
	if ifMatch == "*" {
		ifMatch = existingMd.ETag
	}
	committedMd, _, putErr := engine.PutFile(r.Context(), enginePath, r.Body, size, existingMd, ifMatch, ifNoneMatch)
	if putErr != nil {
		sendWriteError(w, logger, putErr, http.StatusInternalServerError)
		return nil, 0, true
	}
	return committedMd, http.StatusOK, false
}

func handleCrossServerPut(w http.ResponseWriter, r *http.Request, engine *core.Engine, existingMd *metadata.Metadata, enginePath string, pathInfo PathInfo, userID string, size int64, countReader *CountingReader, ifMatch string, ifNoneMatch bool, logger *zap.Logger) {
	etag, err := engine.UpdateFileOnInstance(r.Context(), *existingMd.CallFSInstanceID, enginePath, r.Body, size, ifMatch, ifNoneMatch)
	if err != nil {
		logger.Error("Failed to update file via cross-server proxy",
			zap.String("instance_id", *existingMd.CallFSInstanceID),
			zap.String("path", enginePath),
			zap.Error(err))
		sendWriteError(w, logger, fmt.Errorf("failed to update file on remote server: %w", err), http.StatusBadGateway)
		return
	}

	proxySize := size
	if countReader != nil {
		proxySize = countReader.BytesRead()
	}

	if etag != "" {
		w.Header().Set("ETag", quotedETag(etag))
	}
	w.WriteHeader(http.StatusOK)
	logger.Info("File updated via cross-server proxy",
		zap.String("path", pathInfo.FullPath),
		zap.String("user_id", userID),
		zap.String("target_instance", *existingMd.CallFSInstanceID),
		zap.Int64("size", proxySize))
}

func parseWriteConditions(r *http.Request) (string, bool, error) {
	ifMatch := strings.TrimSpace(r.Header.Get("If-Match"))
	if ifMatch != "" && ifMatch != "*" {
		ifMatch = strings.Trim(strings.TrimPrefix(ifMatch, "W/"), `"`)
	}

	switch strings.TrimSpace(r.Header.Get("If-None-Match")) {
	case "":
		return ifMatch, false, nil
	case "*":
		return ifMatch, true, nil
	default:
		return "", false, errors.New(`If-None-Match only supports "*"`)
	}
}

func V1PutFileEnhanced(engine *core.Engine, authorizer auth.Authorizer, backendConfig *config.BackendConfig, cfg *config.ServerConfig, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		urlPath := chi.URLParam(r, "*")
		pathInfo := ParseFilePath(urlPath)
		if pathInfo.IsInvalid {
			SendErrorResponse(w, logger, &customError{message: "invalid path"}, http.StatusBadRequest)
			return
		}

		if pathInfo.IsDirectory {
			SendErrorResponse(w, logger,
				&customError{message: "PUT method cannot be used with directory paths (trailing slash)"},
				http.StatusBadRequest)
			return
		}

		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}

		enginePath := pathInfo.FullPath

		ifMatch, ifNoneMatch, condErr := parseWriteConditions(r)
		if condErr != nil {
			sendBadRequest(w, logger, condErr.Error())
			return
		}

		const maxUploadBytes int64 = 10 << 30
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

		size := r.ContentLength
		isChunked := size < 0
		if isChunked {
			size = 0
		}

		var countReader *CountingReader
		if isChunked {
			countReader = NewCountingReader(r.Body)
			r.Body = io.NopCloser(countReader)
		}

		existingMd, mdErr := engine.GetMetadata(r.Context(), enginePath)
		if nil != mdErr && !errors.Is(mdErr, metadata.ErrNotFound) {
			SendErrorResponse(w, logger, mdErr, http.StatusInternalServerError)
			return
		}

		if err := authorizer.AuthorizeWithMetadata(r.Context(), userID, enginePath, existingMd, auth.WritePerm); err != nil {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		committedMd, statusCode, done := putResolveTarget(w, r, engine, backendConfig, enginePath, pathInfo, userID, existingMd, size, countReader, ifMatch, ifNoneMatch, logger)
		if done {
			return
		}

		if committedMd.ETag != "" {
			w.Header().Set("ETag", quotedETag(committedMd.ETag))
		}
		w.WriteHeader(statusCode)
		logger.Debug("File updated locally",
			zap.String("path", pathInfo.FullPath),
			zap.String("user_id", userID),
			zap.Int64("size", committedMd.Size),
			zap.Int("status_code", statusCode))
	}
}
