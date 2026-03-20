package handlers

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/backends"
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

		if err := localBackend.Create(r.Context(), shardPath, r.Body, r.ContentLength); err != nil {
			// Try update if create fails (shard already exists)
			if updateErr := localBackend.Update(r.Context(), shardPath, r.Body, r.ContentLength); updateErr != nil {
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
	auth := r.Header.Get("Authorization")
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer"))
	return token == secret
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

	// Build the shard storage path using same convention as manager
	// The actual storage path is determined by erasure metadata, but for internal
	// shard endpoints we use the filePath+index to look up from metadata.
	// For direct storage, we use the erasure shard path convention.
	shardPath := fmt.Sprintf(".erasure/%s/%d", filePath, index)
	return shardPath, index, nil
}
