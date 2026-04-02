package handlers

import (
	"bytes"
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/backends"
	"github.com/ebogdum/callfs/internal/pathutil"
)

// InternalStoreShardHandler handles PUT /v1/internal/shards/{path}/{index}
// Stores a shard on this node (authenticated via InternalProxySecret).
func InternalStoreShardHandler(localBackend backends.Storage, internalSecret string, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorizeInternal(r, internalSecret) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		shardPath, _, err := parseShardPath(r.URL.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Limit shard size to 256 MB and buffer body to allow retry
		const maxShardBytes = 256 << 20
		r.Body = http.MaxBytesReader(w, r.Body, maxShardBytes)
		data, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			http.Error(w, "failed to read shard body", http.StatusBadRequest)
			return
		}
		dataSize := int64(len(data))

		if err := localBackend.Create(r.Context(), shardPath, bytes.NewReader(data), dataSize); err != nil {
			// Try update if create fails (shard already exists)
			if updateErr := localBackend.Update(r.Context(), shardPath, bytes.NewReader(data), dataSize); updateErr != nil {
				logger.Error("Failed to store shard", zap.String("path", shardPath), zap.Error(err))
				http.Error(w, "failed to store shard", http.StatusInternalServerError)
				return
			}
		}

		w.WriteHeader(http.StatusCreated)
	}
}

// InternalGetShardHandler handles GET /v1/internal/shards/{path}/{index}
// Retrieves a shard from this node.
func InternalGetShardHandler(localBackend backends.Storage, internalSecret string, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorizeInternal(r, internalSecret) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		shardPath, _, err := parseShardPath(r.URL.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		reader, err := localBackend.Open(r.Context(), shardPath)
		if err != nil {
			http.Error(w, "shard not found", http.StatusNotFound)
			return
		}
		defer reader.Close()

		w.Header().Set("Content-Type", "application/octet-stream")
		if _, err := io.Copy(w, reader); err != nil {
			logger.Error("Failed to stream shard", zap.String("path", shardPath), zap.Error(err))
		}
	}
}

// InternalDeleteShardHandler handles DELETE /v1/internal/shards/{path}/{index}
// Deletes a shard from this node.
func InternalDeleteShardHandler(localBackend backends.Storage, internalSecret string, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorizeInternal(r, internalSecret) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		shardPath, _, err := parseShardPath(r.URL.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := localBackend.Delete(r.Context(), shardPath); err != nil {
			logger.Warn("Failed to delete shard", zap.String("path", shardPath), zap.Error(err))
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func authorizeInternal(r *http.Request, secret string) bool {
	if secret == "" {
		return false // Reject all requests if no internal secret is configured
	}
	auth := r.Header.Get("Authorization")
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	return subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1
}

// parseShardPath extracts the shard storage path and index from a URL like
// /v1/internal/shards/some/file/path/3
func parseShardPath(urlPath string) (string, int, error) {
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
	shardPath := fmt.Sprintf(".erasure/%s/%d", filePath, index)
	// NOTE: This path scheme must match erasure.Manager.StoreFile's shardPath construction.
	return shardPath, index, nil
}
