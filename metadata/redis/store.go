package redis

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/metadata"
)

// listChildrenBatchSize caps how many child keys are fetched per MGET, so a
// directory with a very large number of entries is read in bounded batches
// rather than as one oversized command.
const listChildrenBatchSize = 512

const linkCleanupBatchSize = 512

const maxTxAttempts = 8

type RedisStore struct {
	client *redis.Client
	prefix string
	logger *zap.Logger
}

func NewRedisStoreWithOptions(addr, username, password string, db int, prefix string, tlsConfig *tls.Config, logger *zap.Logger) (*RedisStore, error) {
	if prefix == "" {
		prefix = "callfs:"
	}

	client := redis.NewClient(&redis.Options{
		Addr:      addr,
		Username:  username,
		Password:  password,
		DB:        db,
		TLSConfig: tlsConfig,
	})
	if err := client.Ping(context.Background()).Err(); nil != err {
		return nil, errors.Join(fmt.Errorf("failed to connect to redis metadata store: %w", err), client.Close())
	}

	store := &RedisStore{client: client, prefix: prefix, logger: logger}
	if err := store.backfillPathIndex(context.Background()); err != nil {
		return nil, errors.Join(err, client.Close())
	}
	return store, nil
}

func (s *RedisStore) backfillPathIndex(ctx context.Context) error {
	exists, err := s.client.Exists(ctx, s.pathsKey()).Result()
	if err != nil {
		return fmt.Errorf("failed to check path index: %w", err)
	}
	if exists > 0 {
		return nil
	}

	var keys []string
	iter := s.client.Scan(ctx, 0, s.prefix+"md:*", 0).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("failed to scan metadata keys: %w", err)
	}

	for start := 0; start < len(keys); start += listChildrenBatchSize {
		end := start + listChildrenBatchSize
		if end > len(keys) {
			end = len(keys)
		}
		values, err := s.client.MGet(ctx, keys[start:end]...).Result()
		if err != nil {
			return fmt.Errorf("failed to read metadata for path index: %w", err)
		}

		members := make([]*redis.Z, 0, len(values))
		for _, value := range values {
			if value == nil {
				continue
			}
			raw, ok := value.(string)
			if !ok {
				return fmt.Errorf("unexpected redis value type %T while building path index", value)
			}
			var md metadata.Metadata
			if err := json.Unmarshal([]byte(raw), &md); err != nil {
				return fmt.Errorf("failed to decode metadata for path index: %w", err)
			}
			members = append(members, &redis.Z{Score: 0, Member: normalizePath(md.Path)})
		}
		if len(members) > 0 {
			if err := s.client.ZAdd(ctx, s.pathsKey(), members...).Err(); err != nil {
				return fmt.Errorf("failed to build path index: %w", err)
			}
		}
	}
	return nil
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

	// Atomic create: SetNX + SAdd in a single Lua script to prevent
	// invisible files (metadata exists but missing from parent's children set).
	luaCreate := `
		local stored = redis.call("SETNX", KEYS[1], ARGV[1])
		if stored == 0 then
			return redis.error_reply("already_exists")
		end
		redis.call("SADD", KEYS[2], ARGV[2])
		redis.call("ZADD", KEYS[3], 0, ARGV[3])
		return "OK"
	`
	mdKey := s.metadataKey(md.Path)
	childKey := s.childrenKey(parentPath(md.Path))
	result := s.client.Eval(ctx, luaCreate, []string{mdKey, childKey, s.pathsKey()}, raw, md.Path, normalizePath(md.Path))
	if err := result.Err(); err != nil {
		if strings.Contains(err.Error(), "already_exists") {
			return metadata.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create metadata: %w", err)
	}
	return nil
}

func (s *RedisStore) Update(ctx context.Context, md *metadata.Metadata) error {
	return s.updateChecked(ctx, md, nil)
}

func (s *RedisStore) UpdateIfMatch(ctx context.Context, md *metadata.Metadata, expectedETag string) error {
	return s.updateChecked(ctx, md, func(stored *metadata.Metadata) error {
		if stored.ETag != expectedETag {
			return metadata.ErrPrecondition
		}
		return nil
	})
}

func (s *RedisStore) updateChecked(ctx context.Context, md *metadata.Metadata, check func(stored *metadata.Metadata) error) error {
	key := s.metadataKey(md.Path)
	return s.watchRetry(ctx, func(tx *redis.Tx) error {
		if check == nil {
			exists, err := tx.Exists(ctx, key).Result()
			if err != nil {
				return fmt.Errorf("failed to check metadata: %w", err)
			}
			if exists == 0 {
				return metadata.ErrNotFound
			}
		} else {
			stored, err := getTx(ctx, tx, key)
			if err != nil {
				return err
			}
			if err := check(stored); err != nil {
				return err
			}
		}

		md.UpdatedAt = time.Now().UTC()
		raw, err := json.Marshal(md)
		if err != nil {
			return fmt.Errorf("failed to encode metadata: %w", err)
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, raw, 0)
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to update metadata: %w", err)
		}
		return nil
	}, key)
}

func (s *RedisStore) watchRetry(ctx context.Context, fn func(tx *redis.Tx) error, keys ...string) error {
	var err error
	for attempt := 0; attempt < maxTxAttempts; attempt++ {
		err = s.client.Watch(ctx, fn, keys...)
		if !errors.Is(err, redis.TxFailedErr) {
			return err
		}
	}
	return fmt.Errorf("metadata transaction aborted after %d attempts: %w", maxTxAttempts, err)
}

func getTx(ctx context.Context, tx *redis.Tx, key string) (*metadata.Metadata, error) {
	raw, err := tx.Get(ctx, key).Result()
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

func (s *RedisStore) Delete(ctx context.Context, path string) error {
	// Atomic delete: check existence, DEL metadata, SREM from parent, DEL children set
	// all in a single Lua script to prevent stale children-set entries.
	luaDelete := `
		local exists = redis.call("EXISTS", KEYS[1])
		if exists == 0 then
			return redis.error_reply("not_found")
		end
		if 0 < redis.call("SCARD", KEYS[3]) then
			return redis.error_reply("not_empty")
		end
		redis.call("DEL", KEYS[1])
		redis.call("SREM", KEYS[2], ARGV[1])
		redis.call("DEL", KEYS[3])
		redis.call("ZREM", KEYS[4], ARGV[2])
		redis.call("DEL", KEYS[5])
		return "OK"
	`
	mdKey := s.metadataKey(path)
	parentChildKey := s.childrenKey(parentPath(path))
	ownChildKey := s.childrenKey(path)
	result := s.client.Eval(ctx, luaDelete, []string{mdKey, parentChildKey, ownChildKey, s.pathsKey(), s.erasureKey(path)}, path, normalizePath(path))
	if err := result.Err(); err != nil {
		if strings.Contains(err.Error(), "not_found") {
			return metadata.ErrNotFound
		}
		if strings.Contains(err.Error(), "not_empty") {
			return fmt.Errorf("%w: directory not empty", metadata.ErrPrecondition)
		}
		return fmt.Errorf("failed to delete metadata: %w", err)
	}
	return nil
}

var renameScript = redis.NewScript(`
	local pathsKey = KEYS[1]
	local oldParentChildren = KEYS[2]
	local newParentChildren = KEYS[3]
	local prefix = ARGV[1]
	local oldPath = ARGV[2]
	local newPath = ARGV[3]
	local now = ARGV[4]

	local function mdKey(p) return prefix .. "md:" .. p end
	local function childrenKey(p) return prefix .. "children:" .. p end
	local function erasureKey(p) return prefix .. "erasure:" .. p end

	local srcRaw = redis.call("GET", mdKey(oldPath))
	if not srcRaw then
		return redis.error_reply("not_found")
	end
	if redis.call("EXISTS", mdKey(newPath)) == 1 then
		return redis.error_reply("already_exists")
	end

	local oldPaths = {oldPath}
	local raws = {srcRaw}
	if cjson.decode(srcRaw).type == "directory" then
		local descendants = redis.call("ZRANGEBYLEX", pathsKey, "[" .. oldPath .. "/", "[" .. oldPath .. "/\255")
		for _, p in ipairs(descendants) do
			local raw = redis.call("GET", mdKey(p))
			if not raw then
				return redis.error_reply("metadata_missing " .. p)
			end
			table.insert(oldPaths, p)
			table.insert(raws, raw)
		end
	end

	local moves = {}
	for i, p in ipairs(oldPaths) do
		local md = cjson.decode(raws[i])
		local target = newPath .. string.sub(p, #oldPath + 1)
		local isDir = md.type == "directory"
		md.path = target
		md.name = string.match(target, "([^/]*)$")
		md.updated_at = now
		local members = {}
		if isDir then
			local cursor = "0"
			repeat
				local page = redis.call("SSCAN", childrenKey(p), cursor, "COUNT", 512)
				cursor = page[1]
				for _, member in ipairs(page[2]) do
					table.insert(members, newPath .. string.sub(member, #oldPath + 1))
				end
			until "0" == cursor
		end
		table.insert(moves, {
			old = p,
			target = target,
			raw = cjson.encode(md),
			isDir = isDir,
			members = members,
			erasure = redis.call("GET", erasureKey(p)),
		})
	end

	for _, m in ipairs(moves) do
		redis.call("DEL", mdKey(m.old))
		redis.call("ZREM", pathsKey, m.old)
		redis.call("DEL", erasureKey(m.old))
		if m.isDir then
			redis.call("DEL", childrenKey(m.old))
		end
	end

	for _, m in ipairs(moves) do
		redis.call("SET", mdKey(m.target), m.raw)
		redis.call("ZADD", pathsKey, 0, m.target)
		if m.erasure then
			redis.call("SET", erasureKey(m.target), m.erasure)
		end
		for _, member in ipairs(m.members) do
			redis.call("SADD", childrenKey(m.target), member)
		end
	end

	redis.call("SREM", oldParentChildren, oldPath)
	redis.call("SADD", newParentChildren, newPath)
	return "OK"
`)

func (s *RedisStore) Rename(ctx context.Context, oldPath, newPath string) error {
	oldPath = normalizePath(oldPath)
	newPath = normalizePath(newPath)
	keys := []string{s.pathsKey(), s.childrenKey(parentPath(oldPath)), s.childrenKey(parentPath(newPath))}
	args := []interface{}{s.prefix, oldPath, newPath, time.Now().UTC().Format(time.RFC3339Nano)}
	err := renameScript.Run(ctx, s.client, keys, args...).Err()
	if nil == err {
		return nil
	}
	if strings.Contains(err.Error(), "not_found") {
		return metadata.ErrNotFound
	}
	if strings.Contains(err.Error(), "already_exists") {
		return metadata.ErrAlreadyExists
	}
	return fmt.Errorf("failed to rename metadata: %w", err)
}

func (s *RedisStore) HasChildren(ctx context.Context, parentPath string) (bool, error) {
	count, err := s.client.SCard(ctx, s.childrenKey(parentPath)).Result()
	if err != nil {
		return false, fmt.Errorf("failed to count child paths: %w", err)
	}
	return 0 < count, nil
}

func (s *RedisStore) ListDescendants(ctx context.Context, dirPath string, maxDepth, limit int) ([]*metadata.Metadata, error) {
	dirPath = normalizePath(dirPath)
	base := dirPath
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}

	paths, err := s.client.ZRangeByLex(ctx, s.pathsKey(), &redis.ZRangeBy{
		Min: "[" + base,
		Max: "[" + base + "\xff",
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list descendant paths: %w", err)
	}

	selected := make([]string, 0, len(paths))
	for _, p := range paths {
		rel := strings.TrimPrefix(p, base)
		if "" == rel {
			continue
		}
		depth := strings.Count(rel, "/") + 1
		if 0 == maxDepth || depth <= maxDepth {
			selected = append(selected, p)
		}
		if 0 < limit && len(selected) >= limit {
			break
		}
	}

	descendants := make([]*metadata.Metadata, 0, len(selected))
	if 0 == len(selected) {
		return descendants, nil
	}

	keys := make([]string, 0, len(selected))
	for _, p := range selected {
		keys = append(keys, s.metadataKey(p))
	}
	chunks := make([]*redis.SliceCmd, 0, (len(keys)+listChildrenBatchSize-1)/listChildrenBatchSize)
	_, err = s.client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for start := 0; start < len(keys); start += listChildrenBatchSize {
			end := start + listChildrenBatchSize
			if end > len(keys) {
				end = len(keys)
			}
			chunks = append(chunks, pipe.MGet(ctx, keys[start:end]...))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get descendants: %w", err)
	}

	for chunk, cmd := range chunks {
		values := cmd.Val()
		for i, value := range values {
			if nil == value {
				continue
			}
			raw, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("unexpected redis value type %T for path %s", value, selected[chunk*listChildrenBatchSize+i])
			}
			var md metadata.Metadata
			if err := json.Unmarshal([]byte(raw), &md); err != nil {
				return nil, fmt.Errorf("failed to decode metadata for %s: %w", selected[chunk*listChildrenBatchSize+i], err)
			}
			descendants = append(descendants, &md)
		}
	}
	return descendants, nil
}

func (s *RedisStore) ListByPrefix(ctx context.Context, prefix, startAfter string, limit int) ([]*metadata.Metadata, error) {
	entries := make([]*metadata.Metadata, 0)
	if limit <= 0 {
		return entries, nil
	}

	lower := "[" + prefix
	if startAfter != "" && startAfter >= prefix {
		lower = "(" + startAfter
	}
	upper := "[" + prefix + "\xff"

	var offset int64
	for len(entries) < limit {
		count := int64(limit - len(entries))
		paths, err := s.client.ZRangeByLex(ctx, s.pathsKey(), &redis.ZRangeBy{
			Min:    lower,
			Max:    upper,
			Offset: offset,
			Count:  count,
		}).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to list paths: %w", err)
		}
		if len(paths) == 0 {
			break
		}

		keys := make([]string, 0, len(paths))
		for _, p := range paths {
			keys = append(keys, s.metadataKey(p))
		}
		values, err := s.client.MGet(ctx, keys...).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to get entries: %w", err)
		}
		for i, value := range values {
			if value == nil {
				continue
			}
			raw, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("unexpected redis value type %T for key %s", value, keys[i])
			}
			var md metadata.Metadata
			if err := json.Unmarshal([]byte(raw), &md); err != nil {
				return nil, fmt.Errorf("failed to decode metadata for %s: %w", keys[i], err)
			}
			if md.Type == "file" {
				entries = append(entries, &md)
			}
		}
		if int64(len(paths)) < count {
			break
		}
		offset += int64(len(paths))
	}
	return entries, nil
}

func (s *RedisStore) ListChildren(ctx context.Context, parentPath string) ([]*metadata.Metadata, error) {
	children := make([]*metadata.Metadata, 0)
	seen := make(map[string]struct{})
	var cursor uint64
	for {
		paths, next, err := s.client.SScan(ctx, s.childrenKey(parentPath), cursor, "", listChildrenBatchSize).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to list child paths: %w", err)
		}

		keys := make([]string, 0, len(paths))
		for _, path := range paths {
			if _, dup := seen[path]; dup {
				continue
			}
			seen[path] = struct{}{}
			keys = append(keys, s.metadataKey(path))
		}

		for start := 0; start < len(keys); start += listChildrenBatchSize {
			end := start + listChildrenBatchSize
			if end > len(keys) {
				end = len(keys)
			}
			batch := keys[start:end]

			values, err := s.client.MGet(ctx, batch...).Result()
			if err != nil {
				return nil, fmt.Errorf("failed to get child metadata: %w", err)
			}

			for i, value := range values {
				if nil == value {
					continue
				}
				raw, ok := value.(string)
				if !ok {
					return nil, fmt.Errorf("unexpected redis value type %T for key %s", value, batch[i])
				}
				var md metadata.Metadata
				if err := json.Unmarshal([]byte(raw), &md); err != nil {
					return nil, fmt.Errorf("failed to decode metadata for %s: %w", batch[i], err)
				}
				children = append(children, &md)
			}
		}

		if 0 == next {
			break
		}
		cursor = next
	}

	type sortableChild struct {
		md    *metadata.Metadata
		lower string
	}
	sortable := make([]sortableChild, len(children))
	for i, child := range children {
		sortable[i] = sortableChild{md: child, lower: strings.ToLower(child.Name)}
	}
	sort.Slice(sortable, func(i, j int) bool {
		if sortable[i].md.Type != sortable[j].md.Type {
			return sortable[i].md.Type > sortable[j].md.Type
		}
		return sortable[i].lower < sortable[j].lower
	})
	for i := range sortable {
		children[i] = sortable[i].md
	}

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
	link.Token = token
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

	raw, err := encodeStoredLink(link)
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

func (s *RedisStore) CountActiveSingleUseLinks(ctx context.Context, owner string, now time.Time) (int, error) {
	count := 0
	keys := make([]string, 0, linkCleanupBatchSize)
	iter := s.client.Scan(ctx, 0, s.prefix+"sul:*", linkCleanupBatchSize).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		if linkCleanupBatchSize > len(keys) {
			continue
		}
		matched, err := s.countActiveLinkBatch(ctx, keys, owner, now)
		count += matched
		if err != nil {
			return count, fmt.Errorf("failed to count active single-use links: %w", err)
		}
		keys = keys[:0]
	}
	if err := iter.Err(); err != nil {
		return count, fmt.Errorf("failed to count active single-use links: %w", err)
	}
	if len(keys) > 0 {
		matched, err := s.countActiveLinkBatch(ctx, keys, owner, now)
		count += matched
		if err != nil {
			return count, fmt.Errorf("failed to count active single-use links: %w", err)
		}
	}
	return count, nil
}

func (s *RedisStore) countActiveLinkBatch(ctx context.Context, keys []string, owner string, now time.Time) (int, error) {
	values, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to read links: %w", err)
	}

	matched := 0
	for i, value := range values {
		raw, ok := value.(string)
		if !ok {
			continue
		}
		var link metadata.SingleUseLink
		if err := json.Unmarshal([]byte(raw), &link); nil != err {
			s.logger.Warn("Skipping undecodable single-use link during count", zap.String("key", keys[i]), zap.Error(err))
			continue
		}
		if owner == link.Owner && "active" == link.Status && now.Before(link.ExpiresAt) {
			matched++
		}
	}
	return matched, nil
}

var consumeLinkScript = redis.NewScript(`
	if redis.call("GET", KEYS[1]) ~= ARGV[1] then
		return 0
	end
	redis.call("SET", KEYS[1], ARGV[2])
	return 1
`)

func (s *RedisStore) ConsumeSingleUseLink(ctx context.Context, token string, usedByIP string, now time.Time) (*metadata.SingleUseLink, bool, error) {
	key := s.linkKey(token)
	for attempt := 0; attempt < maxTxAttempts; attempt++ {
		raw, err := s.client.Get(ctx, key).Result()
		if err != nil {
			if redis.Nil == err {
				return nil, false, metadata.ErrNotFound
			}
			return nil, false, fmt.Errorf("failed to get single-use link: %w", err)
		}

		var link metadata.SingleUseLink
		if err := json.Unmarshal([]byte(raw), &link); err != nil {
			return nil, false, fmt.Errorf("failed to decode single-use link: %w", err)
		}
		link.Token = token
		if "active" != link.Status || !now.Before(link.ExpiresAt) {
			return &link, false, nil
		}

		usedAt := now.UTC()
		link.Status = "used"
		link.UsedAt = &usedAt
		link.UsedByIP = &usedByIP
		link.UpdatedAt = usedAt
		next, err := encodeStoredLink(&link)
		if err != nil {
			return nil, false, fmt.Errorf("failed to encode single-use link: %w", err)
		}

		swapped, err := consumeLinkScript.Run(ctx, s.client, []string{key}, raw, next).Int()
		if err != nil {
			return nil, false, fmt.Errorf("failed to consume single-use link: %w", err)
		}
		if 1 == swapped {
			return &link, true, nil
		}
	}
	return nil, false, fmt.Errorf("single-use link changed during consume after %d attempts", maxTxAttempts)
}

func (s *RedisStore) CleanupExpiredLinks(ctx context.Context, before time.Time) (int, error) {
	return s.cleanupLinks(ctx, "expired links", func(link *metadata.SingleUseLink) bool {
		return link.Status == "active" && link.ExpiresAt.Before(before)
	})
}

func (s *RedisStore) CleanupUsedLinks(ctx context.Context, olderThan time.Time) (int, error) {
	return s.cleanupLinks(ctx, "used links", func(link *metadata.SingleUseLink) bool {
		return link.Status == "used" && link.UsedAt != nil && link.UsedAt.Before(olderThan)
	})
}

func (s *RedisStore) cleanupLinks(ctx context.Context, label string, stale func(link *metadata.SingleUseLink) bool) (int, error) {
	count := 0
	keys := make([]string, 0, linkCleanupBatchSize)
	iter := s.client.Scan(ctx, 0, s.prefix+"sul:*", linkCleanupBatchSize).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		if len(keys) < linkCleanupBatchSize {
			continue
		}
		deleted, err := s.cleanupLinkBatch(ctx, keys, stale)
		count += deleted
		if err != nil {
			return count, fmt.Errorf("failed to cleanup %s: %w", label, err)
		}
		keys = keys[:0]
	}
	if err := iter.Err(); err != nil {
		return count, fmt.Errorf("failed to cleanup %s: %w", label, err)
	}
	if len(keys) > 0 {
		deleted, err := s.cleanupLinkBatch(ctx, keys, stale)
		count += deleted
		if err != nil {
			return count, fmt.Errorf("failed to cleanup %s: %w", label, err)
		}
	}
	return count, nil
}

func (s *RedisStore) cleanupLinkBatch(ctx context.Context, keys []string, stale func(link *metadata.SingleUseLink) bool) (int, error) {
	values, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to read links: %w", err)
	}

	doomed := make([]string, 0, len(keys))
	for i, value := range values {
		raw, ok := value.(string)
		if !ok {
			continue
		}
		var link metadata.SingleUseLink
		if err := json.Unmarshal([]byte(raw), &link); nil != err {
			s.logger.Warn("Skipping undecodable single-use link during cleanup", zap.String("key", keys[i]), zap.Error(err))
			continue
		}
		if stale(&link) {
			doomed = append(doomed, keys[i])
		}
	}
	if len(doomed) == 0 {
		return 0, nil
	}

	deleted, err := s.client.Del(ctx, doomed...).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to delete links: %w", err)
	}
	return int(deleted), nil
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}

func (s *RedisStore) metadataKey(path string) string {
	return s.prefix + "md:" + normalizePath(path)
}

func (s *RedisStore) pathsKey() string {
	return s.prefix + "paths"
}

func (s *RedisStore) childrenKey(path string) string {
	return s.prefix + "children:" + normalizePath(path)
}

func (s *RedisStore) linkKey(token string) string {
	digest := sha256.Sum256([]byte(token))
	return s.prefix + "sul:" + hex.EncodeToString(digest[:])
}

func encodeStoredLink(link *metadata.SingleUseLink) ([]byte, error) {
	stored := *link
	stored.Token = ""
	return json.Marshal(&stored)
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
