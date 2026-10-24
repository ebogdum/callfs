package redis

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/metadata"
)

const cursorVersionPrefix = "v1:"

var errLinkFound = errors.New("single-use link found")

func encodeCursor(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursorVersionPrefix + value))
}

func decodeCursor(cursor string) (string, error) {
	if "" == cursor {
		return "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if nil != err || !strings.HasPrefix(string(raw), cursorVersionPrefix) {
		return "", fmt.Errorf("%w: invalid cursor", metadata.ErrInvalidQuery)
	}
	return strings.TrimPrefix(string(raw), cursorVersionPrefix), nil
}

func decodeLinkCursor(cursor string) (int64, error) {
	value, err := decodeCursor(cursor)
	if nil != err {
		return 0, err
	}
	if "" == value {
		return 0, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if nil != err || 0 >= id {
		return 0, fmt.Errorf("%w: invalid cursor", metadata.ErrInvalidQuery)
	}
	return id, nil
}

func (s *RedisStore) scanValues(ctx context.Context, pattern string, visit func(key, raw string) error) error {
	keys := make([]string, 0, listChildrenBatchSize)
	iter := s.client.Scan(ctx, 0, pattern, listChildrenBatchSize).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		if len(keys) < listChildrenBatchSize {
			continue
		}
		if err := s.visitValueBatch(ctx, keys, visit); err != nil {
			return err
		}
		keys = keys[:0]
	}
	if err := iter.Err(); nil != err {
		return fmt.Errorf("failed to scan keys matching %s: %w", pattern, err)
	}
	if 0 < len(keys) {
		return s.visitValueBatch(ctx, keys, visit)
	}
	return nil
}

func (s *RedisStore) visitValueBatch(ctx context.Context, keys []string, visit func(key, raw string) error) error {
	values, err := s.client.MGet(ctx, keys...).Result()
	if nil != err {
		return fmt.Errorf("failed to read keys: %w", err)
	}
	for i, value := range values {
		raw, ok := value.(string)
		if !ok {
			continue
		}
		if err := visit(keys[i], raw); nil != err {
			return err
		}
	}
	return nil
}

func (s *RedisStore) scanMetadata(ctx context.Context, visit func(md *metadata.Metadata)) error {
	return s.scanValues(ctx, s.prefix+"md:*", func(key, raw string) error {
		var md metadata.Metadata
		if err := json.Unmarshal([]byte(raw), &md); nil != err {
			return fmt.Errorf("failed to decode metadata for %s: %w", key, err)
		}
		visit(&md)
		return nil
	})
}

func (s *RedisStore) decodeLink(key, raw string) (*metadata.SingleUseLink, bool) {
	var link metadata.SingleUseLink
	if err := json.Unmarshal([]byte(raw), &link); nil != err {
		s.logger.Warn("Skipping undecodable single-use link", zap.String("key", key), zap.Error(err))
		return nil, false
	}
	return &link, true
}

func (s *RedisStore) findLinkByID(ctx context.Context, id int64) (string, string, *metadata.SingleUseLink, error) {
	var foundKey, foundRaw string
	var found *metadata.SingleUseLink
	err := s.scanValues(ctx, s.prefix+"sul:*", func(key, raw string) error {
		link, ok := s.decodeLink(key, raw)
		if !ok || link.ID != id {
			return nil
		}
		foundKey, foundRaw, found = key, raw, link
		return errLinkFound
	})
	if errors.Is(err, errLinkFound) {
		return foundKey, foundRaw, found, nil
	}
	if nil != err {
		return "", "", nil, fmt.Errorf("failed to find single-use link: %w", err)
	}
	return "", "", nil, metadata.ErrNotFound
}

func linkStatusAt(link *metadata.SingleUseLink, now time.Time) string {
	if "active" == link.Status && link.ExpiresAt.Before(now) {
		return "expired"
	}
	return link.Status
}

func linkMatches(link *metadata.SingleUseLink, filter metadata.LinkFilter) bool {
	if "" != filter.Status && filter.Status != link.Status {
		return false
	}
	if "" != filter.FilePath && filter.FilePath != link.FilePath {
		return false
	}
	if "" != filter.CreatedBy && filter.CreatedBy != link.Owner {
		return false
	}
	return true
}

func (s *RedisStore) ListSingleUseLinks(ctx context.Context, filter metadata.LinkFilter) ([]metadata.SingleUseLink, string, error) {
	if 0 >= filter.Limit {
		return []metadata.SingleUseLink{}, "", nil
	}
	beforeID, err := decodeLinkCursor(filter.Cursor)
	if nil != err {
		return nil, "", err
	}
	now := filter.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	matches := make([]metadata.SingleUseLink, 0)
	err = s.scanValues(ctx, s.prefix+"sul:*", func(key, raw string) error {
		link, ok := s.decodeLink(key, raw)
		if !ok {
			return nil
		}
		if 0 < beforeID && link.ID >= beforeID {
			return nil
		}
		link.Status = linkStatusAt(link, now)
		if !linkMatches(link, filter) {
			return nil
		}
		matches = append(matches, *link)
		return nil
	})
	if nil != err {
		return nil, "", fmt.Errorf("failed to list single-use links: %w", err)
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].ID > matches[j].ID
	})
	next := ""
	if filter.Limit < len(matches) {
		next = encodeCursor(strconv.FormatInt(matches[filter.Limit-1].ID, 10))
		matches = matches[:filter.Limit]
	}
	return matches, next, nil
}

func (s *RedisStore) GetSingleUseLinkByID(ctx context.Context, id int64) (*metadata.SingleUseLink, error) {
	_, _, link, err := s.findLinkByID(ctx, id)
	if nil != err {
		return nil, err
	}
	link.Status = linkStatusAt(link, time.Now().UTC())
	return link, nil
}

func (s *RedisStore) RevokeSingleUseLink(ctx context.Context, id int64) error {
	key, raw, link, err := s.findLinkByID(ctx, id)
	if nil != err {
		return err
	}
	if "active" != link.Status {
		return metadata.ErrPrecondition
	}
	link.Status = "revoked"
	link.UpdatedAt = time.Now().UTC()
	next, err := encodeStoredLink(link)
	if nil != err {
		return fmt.Errorf("failed to encode single-use link: %w", err)
	}
	swapped, err := consumeLinkScript.Run(ctx, s.client, []string{key}, raw, next).Int()
	if nil != err {
		return fmt.Errorf("failed to revoke single-use link: %w", err)
	}
	if 1 != swapped {
		return metadata.ErrPrecondition
	}
	return nil
}

func (s *RedisStore) SearchMetadata(ctx context.Context, q metadata.SearchQuery) (metadata.SearchResult, error) {
	return metadata.SearchByScan(ctx, s, q)
}

func (s *RedisStore) OwnerStats(ctx context.Context, cursor string, limit int) ([]metadata.OwnerStat, string, error) {
	if 0 >= limit {
		return []metadata.OwnerStat{}, "", nil
	}
	after, err := decodeCursor(cursor)
	if nil != err {
		return nil, "", err
	}

	totals := make(map[string]*metadata.OwnerStat)
	err = s.scanMetadata(ctx, func(md *metadata.Metadata) {
		if "file" != md.Type {
			return
		}
		stat, ok := totals[md.Owner]
		if !ok {
			stat = &metadata.OwnerStat{Owner: md.Owner}
			totals[md.Owner] = stat
		}
		stat.Files++
		stat.Bytes += md.Size
	})
	if nil != err {
		return nil, "", fmt.Errorf("failed to compute owner stats: %w", err)
	}

	owners := make([]string, 0, len(totals))
	for owner := range totals {
		if "" != cursor && owner <= after {
			continue
		}
		owners = append(owners, owner)
	}
	sort.Strings(owners)

	next := ""
	if limit < len(owners) {
		next = encodeCursor(owners[limit-1])
		owners = owners[:limit]
	}
	stats := make([]metadata.OwnerStat, 0, len(owners))
	for _, owner := range owners {
		stats = append(stats, *totals[owner])
	}
	return stats, next, nil
}

func (s *RedisStore) CountEmptyETags(ctx context.Context) (int, error) {
	count := 0
	err := s.scanMetadata(ctx, func(md *metadata.Metadata) {
		if "file" == md.Type && "" == md.ETag {
			count++
		}
	})
	if nil != err {
		return 0, fmt.Errorf("failed to count empty etags: %w", err)
	}
	return count, nil
}

func (s *RedisStore) CountEmptyOwners(ctx context.Context) (int, error) {
	count := 0
	err := s.scanMetadata(ctx, func(md *metadata.Metadata) {
		if "" == md.Owner {
			count++
		}
	})
	if nil != err {
		return 0, fmt.Errorf("failed to count empty owners: %w", err)
	}
	return count, nil
}

func (s *RedisStore) ListErasureInfos(ctx context.Context, prefix, cursor string, limit int) ([]*metadata.ErasureFileInfo, string, error) {
	if 0 >= limit {
		return []*metadata.ErasureFileInfo{}, "", nil
	}
	after, err := decodeCursor(cursor)
	if nil != err {
		return nil, "", err
	}

	matches := make([]*metadata.ErasureFileInfo, 0)
	err = s.scanValues(ctx, s.prefix+"erasure:*", func(key, raw string) error {
		var info metadata.ErasureFileInfo
		if err := json.Unmarshal([]byte(raw), &info); nil != err {
			return fmt.Errorf("failed to decode erasure info for %s: %w", key, err)
		}
		if !strings.HasPrefix(info.FilePath, prefix) {
			return nil
		}
		if "" != cursor && info.FilePath <= after {
			return nil
		}
		matches = append(matches, &info)
		return nil
	})
	if nil != err {
		return nil, "", fmt.Errorf("failed to list erasure infos: %w", err)
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].FilePath < matches[j].FilePath
	})
	next := ""
	if limit < len(matches) {
		next = encodeCursor(matches[limit-1].FilePath)
		matches = matches[:limit]
	}
	return matches, next, nil
}
