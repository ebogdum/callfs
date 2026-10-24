package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/erasure"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/server/middleware"
	"github.com/ebogdum/callfs/v2/telemetry"
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

		rc, size, err := em.OpenShard(r.Context(), filePath, index)
		if err != nil {
			statusCode := http.StatusInternalServerError
			if errors.Is(err, erasure.ErrShardNotFound) {
				statusCode = http.StatusNotFound
			}
			SendErrorResponse(w, logger, err, statusCode)
			return
		}
		defer rc.Close()

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		buf := getCopyBuffer()
		defer putCopyBuffer(buf)
		if _, err := io.CopyBuffer(w, rc, *buf); err != nil {
			logger.Error("Failed to stream shard data", zap.Error(err))
		}
	}
}

// HandleErasureDownload performs server-side reassembly and streams the file.
func HandleErasureDownload(w http.ResponseWriter, r *http.Request, em *erasure.Manager, path string, md *metadata.Metadata, logger *zap.Logger) {
	if handleValidators(w, r, md) {
		return
	}
	offset, length, partial, ok := resolveRange(w, r, md)
	if !ok {
		return
	}

	n, err := erasure.RangeLength(md.Size, offset, length)
	if err != nil {
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	rc, err := em.OpenRangeReader(r.Context(), path, offset, length)
	if err != nil {
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(n, 10))
	if partial {
		w.Header().Set("Content-Range", contentRangeHeader(offset, n, md.Size))
		w.WriteHeader(http.StatusPartialContent)
	}
	userID, _ := middleware.GetUserID(r.Context())
	transfer := telemetry.Transfers.BeginIfLarge("download", userID, path, transferClient(r), n)
	defer transfer.Finish()
	buf := getCopyBuffer()
	defer putCopyBuffer(buf)
	if _, err := io.CopyBuffer(transfer.Writer(w), rc, *buf); err != nil {
		logger.Error("Failed to stream reassembled file", zap.Error(err))
	}
}
