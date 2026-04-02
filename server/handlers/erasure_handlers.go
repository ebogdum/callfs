package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/erasure"
	"github.com/ebogdum/callfs/server/middleware"
)

// V1GetShard handles GET /v1/shards/{path}/{index} - public shard download (authenticated).
func V1GetShard(em *erasure.Manager, authorizer auth.Authorizer, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		urlPath := chi.URLParam(r, "*")
		if urlPath == "" {
			SendErrorResponse(w, logger, fmt.Errorf("missing path"), http.StatusBadRequest)
			return
		}

		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}

		// Parse: last segment is index, rest is file path
		lastSlash := strings.LastIndex(urlPath, "/")
		if lastSlash < 0 {
			SendErrorResponse(w, logger, fmt.Errorf("invalid shard path"), http.StatusBadRequest)
			return
		}

		rawFilePath := urlPath[:lastSlash]
		indexStr := urlPath[lastSlash+1:]
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			SendErrorResponse(w, logger, fmt.Errorf("invalid shard index"), http.StatusBadRequest)
			return
		}

		// Validate and sanitize the file path before authorization
		pathInfo := ParseFilePath(rawFilePath)
		if pathInfo.IsInvalid {
			SendErrorResponse(w, logger, fmt.Errorf("invalid shard file path"), http.StatusBadRequest)
			return
		}
		filePath := pathInfo.FullPath

		if err := authorizer.Authorize(r.Context(), userID, filePath, auth.ReadPerm); err != nil {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		data, err := em.GetShard(r.Context(), filePath, index)
		if err != nil {
			statusCode := http.StatusInternalServerError
			if err == erasure.ErrShardNotFound {
				statusCode = http.StatusNotFound
			}
			SendErrorResponse(w, logger, err, statusCode)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if _, err := w.Write(data); err != nil {
			logger.Error("Failed to write shard data", zap.Error(err))
		}
	}
}

// HandleErasureManifest writes the chunk manifest as JSON when ?manifest=true on an erasure-coded file.
func HandleErasureManifest(w http.ResponseWriter, r *http.Request, em *erasure.Manager, path string, logger *zap.Logger) {
	manifest, err := em.GetManifest(r.Context(), path)
	if err != nil {
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(manifest); err != nil {
		logger.Error("Failed to encode manifest", zap.Error(err))
	}
}

// HandleErasureDownload performs server-side reassembly and streams the file.
func HandleErasureDownload(w http.ResponseWriter, r *http.Request, em *erasure.Manager, path string, size int64, logger *zap.Logger) {
	data, err := em.RetrieveFile(r.Context(), path)
	if err != nil {
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
		logger.Error("Failed to stream reassembled file", zap.Error(err))
	}
}
