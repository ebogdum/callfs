package redis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-redis/redis/v8"

	"github.com/ebogdum/callfs/metadata"
)

func (s *RedisStore) erasureKey(filePath string) string {
	return s.prefix + "erasure:" + normalizePath(filePath)
}

// CreateErasureInfo stores erasure coding metadata for a file.
func (s *RedisStore) CreateErasureInfo(ctx context.Context, filePath string, info *metadata.ErasureFileInfo) error {
	raw, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to encode erasure info: %w", err)
	}
	stored, err := s.client.SetNX(ctx, s.erasureKey(filePath), raw, 0).Result()
	if err != nil {
		return fmt.Errorf("failed to store erasure info: %w", err)
	}
	if !stored {
		return metadata.ErrAlreadyExists
	}
	return nil
}

// GetErasureInfo retrieves erasure coding metadata for a file.
func (s *RedisStore) GetErasureInfo(ctx context.Context, filePath string) (*metadata.ErasureFileInfo, error) {
	raw, err := s.client.Get(ctx, s.erasureKey(filePath)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get erasure info: %w", err)
	}

	var info metadata.ErasureFileInfo
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		return nil, fmt.Errorf("failed to decode erasure info: %w", err)
	}
	return &info, nil
}

// DeleteErasureInfo removes erasure coding metadata for a file.
func (s *RedisStore) DeleteErasureInfo(ctx context.Context, filePath string) error {
	if err := s.client.Del(ctx, s.erasureKey(filePath)).Err(); err != nil {
		return fmt.Errorf("failed to delete erasure info: %w", err)
	}
	return nil
}
