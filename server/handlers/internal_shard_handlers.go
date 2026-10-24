package handlers

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/backends"
	"github.com/ebogdum/callfs/v2/internal/pathutil"
	"github.com/ebogdum/callfs/v2/metadata"
)

// InternalStoreShardHandler handles PUT /v1/internal/shards/{path}/{index}
// Stores a shard on this node (authenticated via InternalProxySecret).
func InternalStoreShardHandler(localBackend backends.Storage, shardRoot string, internalSecret string, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorizeInternal(r, internalSecret) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		shardPath, _, err := parseShardPath(shardRoot, r.URL.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		const maxShardBytes = 256 << 20
		if r.ContentLength < 0 {
			http.Error(w, "content length required", http.StatusLengthRequired)
			return
		}
		if r.ContentLength > maxShardBytes {
			http.Error(w, "shard too large", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxShardBytes)

		created, err := storeShard(r.Context(), localBackend, shardPath, r.Body, r.ContentLength)
		if err != nil {
			logger.Error("Failed to store shard", zap.String("path", shardPath), zap.Error(err))
			http.Error(w, "failed to store shard", http.StatusInternalServerError)
			return
		}

		if created {
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// InternalGetShardHandler handles GET /v1/internal/shards/{path}/{index}
// Retrieves a shard from this node.
func InternalGetShardHandler(localBackend backends.Storage, shardRoot string, internalSecret string, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorizeInternal(r, internalSecret) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		shardPath, _, err := parseShardPath(shardRoot, r.URL.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		reader, err := localBackend.Open(r.Context(), shardPath)
		if err != nil {
			if errors.Is(err, metadata.ErrNotFound) {
				http.Error(w, "shard not found", http.StatusNotFound)
				return
			}
			logger.Error("Failed to open shard", zap.String("path", shardPath), zap.Error(err))
			http.Error(w, "failed to open shard", http.StatusInternalServerError)
			return
		}
		defer reader.Close()

		w.Header().Set("Content-Type", "application/octet-stream")
		buf := getCopyBuffer()
		defer putCopyBuffer(buf)
		if _, err := io.CopyBuffer(w, reader, *buf); err != nil {
			logger.Error("Failed to stream shard", zap.String("path", shardPath), zap.Error(err))
		}
	}
}

// InternalDeleteShardHandler handles DELETE /v1/internal/shards/{path}/{index}
// Deletes a shard from this node.
func InternalDeleteShardHandler(localBackend backends.Storage, shardRoot string, internalSecret string, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorizeInternal(r, internalSecret) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		shardPath, _, err := parseShardPath(shardRoot, r.URL.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := localBackend.Delete(r.Context(), shardPath); err != nil && !errors.Is(err, metadata.ErrNotFound) {
			logger.Error("Failed to delete shard", zap.String("path", shardPath), zap.Error(err))
			http.Error(w, "failed to delete shard", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

var copyBufferPool = sync.Pool{New: func() any {
	buf := make([]byte, 256<<10)
	return &buf
}}

func getCopyBuffer() *[]byte { return copyBufferPool.Get().(*[]byte) }

func putCopyBuffer(buf *[]byte) { copyBufferPool.Put(buf) }

func storeShard(ctx context.Context, backend backends.Storage, shardPath string, body io.Reader, size int64) (bool, error) {
	_, statErr := backend.Stat(ctx, shardPath)
	switch {
	case nil == statErr:
		return false, backend.Update(ctx, shardPath, body, size)
	case errors.Is(statErr, metadata.ErrNotFound):
		return true, backend.Create(ctx, shardPath, body, size)
	default:
		return false, statErr
	}
}

func authorizeInternal(r *http.Request, secret string) bool {
	if secret == "" {
		return false // Reject all requests if no internal secret is configured
	}
	auth := r.Header.Get("Authorization")
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	tokenDigest := sha256.Sum256([]byte(token))
	secretDigest := sha256.Sum256([]byte(secret))
	return subtle.ConstantTimeCompare(tokenDigest[:], secretDigest[:]) == 1
}

// parseShardPath extracts the shard storage path and index from a URL like
// /v1/internal/shards/some/file/path/3
func parseShardPath(shardRoot, urlPath string) (string, int, error) {
	// Strip the prefix
	trimmed := strings.TrimPrefix(urlPath, "/v1/internal/shards/")
	if trimmed == urlPath {
		return "", 0, fmt.Errorf("invalid shard path")
	}

	// Last segment is the index
	lastSlash := strings.LastIndex(trimmed, "/")
	if lastSlash < 0 {
		return "", 0, fmt.Errorf("invalid shard path: missing index")
	}

	filePath := trimmed[:lastSlash]
	indexStr := trimmed[lastSlash+1:]
	if 0 == len(indexStr) || "" != strings.TrimLeft(indexStr, "0123456789") {
		return "", 0, fmt.Errorf("invalid shard index: must be a non-negative integer")
	}
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid shard index: %w", err)
	}

	// Validate the file path to prevent directory traversal
	if err := pathutil.ValidatePath(filePath); err != nil {
		return "", 0, fmt.Errorf("invalid shard file path: %w", err)
	}

	// Use content-hash based path matching erasure/manager.go convention.
	// The filePath is used as-is since the caller (storeRemoteShard) already
	// sends the hash-based prefix as the path component.
	shardPath := fmt.Sprintf("%s/%s/%d", shardRoot, filePath, index)
	// NOTE: This path scheme must match erasure.Manager.StoreFile's shardPath construction.
	return shardPath, index, nil
}
