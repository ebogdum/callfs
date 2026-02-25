package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

type RedisStore struct {
	client *redis.Client
	prefix string
	logger *zap.Logger
}

func NewRedisStore(addr, password string, db int, prefix string, logger *zap.Logger) (*RedisStore, error) {
	if prefix == "" {
		prefix = "callfs:"
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis metadata store: %w", err)
	}

	return &RedisStore{client: client, prefix: prefix, logger: logger}, nil
}

func (s *RedisStore) Get(ctx context.Context, path string) (*metadata.Metadata, error) {
	key := s.metadataKey(path)
	raw, err := s.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	var md metadata.Metadata
	if err := json.Unmarshal([]byte(raw), &md); err != nil {
		return nil, fmt.Errorf("failed to decode metadata: %w", err)
	}
	return &md, nil
}

func (s *RedisStore) Create(ctx context.Context, md *metadata.Metadata) error {
	now := time.Now().UTC()
	if md.ATime.IsZero() {
		md.ATime = now
	}
	if md.MTime.IsZero() {
		md.MTime = now
	}
	if md.CTime.IsZero() {
		md.CTime = now
	}
	md.CreatedAt = now
	md.UpdatedAt = now

	id, err := s.client.Incr(ctx, s.sequenceKey("inode")).Result()
	if err != nil {
		return fmt.Errorf("failed to allocate metadata id: %w", err)
	}
	md.ID = id

	raw, err := json.Marshal(md)
	if err != nil {
		return fmt.Errorf("failed to encode metadata: %w", err)
	}

	stored, err := s.client.SetNX(ctx, s.metadataKey(md.Path), raw, 0).Result()
	if err != nil {
		return fmt.Errorf("failed to create metadata: %w", err)
	}
	if !stored {
		return metadata.ErrAlreadyExists
	}

	if err := s.client.SAdd(ctx, s.childrenKey(parentPath(md.Path)), md.Path).Err(); err != nil {
		return fmt.Errorf("failed to index child metadata: %w", err)
	}
	return nil
}

func (s *RedisStore) Update(ctx context.Context, md *metadata.Metadata) error {
	if _, err := s.Get(ctx, md.Path); err != nil {
		return err
	}

	md.UpdatedAt = time.Now().UTC()
	raw, err := json.Marshal(md)
	if err != nil {
		return fmt.Errorf("failed to encode metadata: %w", err)
	}

	if err := s.client.Set(ctx, s.metadataKey(md.Path), raw, 0).Err(); err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}
	return nil
}

func (s *RedisStore) Delete(ctx context.Context, path string) error {
	if _, err := s.Get(ctx, path); err != nil {
		return err
	}

	if err := s.client.Del(ctx, s.metadataKey(path)).Err(); err != nil {
		return fmt.Errorf("failed to delete metadata: %w", err)
	}
	if err := s.client.SRem(ctx, s.childrenKey(parentPath(path)), path).Err(); err != nil {
		return fmt.Errorf("failed to remove child index: %w", err)
	}
	_ = s.client.Del(ctx, s.childrenKey(path)).Err()
	return nil
}

func (s *RedisStore) ListChildren(ctx context.Context, parentPath string) ([]*metadata.Metadata, error) {
	paths, err := s.client.SMembers(ctx, s.childrenKey(parentPath)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list child paths: %w", err)
	}

	children := make([]*metadata.Metadata, 0, len(paths))
	for _, path := range paths {
		md, getErr := s.Get(ctx, path)
		if getErr != nil {
			if getErr == metadata.ErrNotFound {
				continue
			}
			return nil, getErr
		}
		children = append(children, md)
	}

	sort.Slice(children, func(i, j int) bool {
		if children[i].Type != children[j].Type {
			return children[i].Type > children[j].Type
		}
		return strings.ToLower(children[i].Name) < strings.ToLower(children[j].Name)
	})

	return children, nil
}

func (s *RedisStore) GetSingleUseLink(ctx context.Context, token string) (*metadata.SingleUseLink, error) {
	raw, err := s.client.Get(ctx, s.linkKey(token)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get single-use link: %w", err)
	}

	var link metadata.SingleUseLink
	if err := json.Unmarshal([]byte(raw), &link); err != nil {
		return nil, fmt.Errorf("failed to decode single-use link: %w", err)
	}
	return &link, nil
}

func (s *RedisStore) CreateSingleUseLink(ctx context.Context, link *metadata.SingleUseLink) error {
	now := time.Now().UTC()
	if link.CreatedAt.IsZero() {
		link.CreatedAt = now
	}
	link.UpdatedAt = now

	id, err := s.client.Incr(ctx, s.sequenceKey("link")).Result()
	if err != nil {
		return fmt.Errorf("failed to allocate single-use link id: %w", err)
	}
	link.ID = id

	raw, err := json.Marshal(link)
	if err != nil {
		return fmt.Errorf("failed to encode single-use link: %w", err)
	}

	stored, err := s.client.SetNX(ctx, s.linkKey(link.Token), raw, 0).Result()
	if err != nil {
		return fmt.Errorf("failed to create single-use link: %w", err)
	}
	if !stored {
		return metadata.ErrAlreadyExists
	}

	return nil
}

func (s *RedisStore) UpdateSingleUseLink(ctx context.Context, token string, status string, usedAt *time.Time, usedByIP *string) error {
	link, err := s.GetSingleUseLink(ctx, token)
	if err != nil {
		return err
	}

	link.Status = status
	link.UsedAt = usedAt
	link.UsedByIP = usedByIP
	link.UpdatedAt = time.Now().UTC()

	raw, err := json.Marshal(link)
	if err != nil {
		return fmt.Errorf("failed to encode single-use link: %w", err)
	}

	if err := s.client.Set(ctx, s.linkKey(token), raw, 0).Err(); err != nil {
		return fmt.Errorf("failed to update single-use link: %w", err)
	}
	return nil
}

func (s *RedisStore) CleanupExpiredLinks(ctx context.Context, before time.Time) (int, error) {
	count := 0
	iter := s.client.Scan(ctx, 0, s.linkKey("*"), 0).Iterator()
	for iter.Next(ctx) {
		raw, err := s.client.Get(ctx, iter.Val()).Result()
		if err != nil {
			continue
		}

		var link metadata.SingleUseLink
		if err := json.Unmarshal([]byte(raw), &link); err != nil {
			continue
		}

		if link.ExpiresAt.Before(before) {
			if err := s.client.Del(ctx, iter.Val()).Err(); err == nil {
				count++
			}
		}
	}
	if err := iter.Err(); err != nil {
		return count, fmt.Errorf("failed to cleanup expired links: %w", err)
	}
	return count, nil
}

func (s *RedisStore) CleanupUsedLinks(ctx context.Context, olderThan time.Time) (int, error) {
	count := 0
	iter := s.client.Scan(ctx, 0, s.linkKey("*"), 0).Iterator()
	for iter.Next(ctx) {
		raw, err := s.client.Get(ctx, iter.Val()).Result()
		if err != nil {
			continue
		}

		var link metadata.SingleUseLink
		if err := json.Unmarshal([]byte(raw), &link); err != nil {
			continue
		}

		if link.Status == "used" && link.UsedAt != nil && link.UsedAt.Before(olderThan) {
			if err := s.client.Del(ctx, iter.Val()).Err(); err == nil {
				count++
			}
		}
	}
	if err := iter.Err(); err != nil {
		return count, fmt.Errorf("failed to cleanup used links: %w", err)
	}
	return count, nil
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}

func (s *RedisStore) metadataKey(path string) string {
	return s.prefix + "md:" + normalizePath(path)
}

func (s *RedisStore) childrenKey(path string) string {
	return s.prefix + "children:" + normalizePath(path)
}

func (s *RedisStore) linkKey(token string) string {
	return s.prefix + "sul:" + token
}

func (s *RedisStore) sequenceKey(name string) string {
	return s.prefix + "seq:" + name
}

func parentPath(path string) string {
	if path == "/" {
		return "/"
	}
	parent := filepath.Dir(path)
	if parent == "." {
		return "/"
	}
	if !strings.HasPrefix(parent, "/") {
		return "/" + parent
	}
	return parent
}

func normalizePath(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}
