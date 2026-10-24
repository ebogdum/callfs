package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/ebogdum/callfs/v2/metadata"
)

const (
	uiPageDefaultLimit = 50
	uiPageMaxLimit     = 500
)

type rowScanner interface {
	Scan(dest ...any) error
}

func clampPageLimit(limit int) int {
	if 0 >= limit {
		return uiPageDefaultLimit
	}
	if uiPageMaxLimit < limit {
		return uiPageMaxLimit
	}
	return limit
}

func scanLinkSummary(row rowScanner) (*metadata.SingleUseLink, error) {
	var link metadata.SingleUseLink
	var usedAt sql.NullTime
	var usedByIP sql.NullString

	err := row.Scan(
		&link.ID,
		&link.Token,
		&link.FilePath,
		&link.Owner,
		&link.ExpiresAt,
		&link.Status,
		&usedAt,
		&usedByIP,
		&link.CreatedAt,
		&link.UpdatedAt,
	)
	if nil != err {
		return nil, err
	}

	if usedAt.Valid {
		link.UsedAt = &usedAt.Time
	}
	if usedByIP.Valid {
		link.UsedByIP = &usedByIP.String
	}

	return &link, nil
}

func (s *PostgresStore) ListSingleUseLinks(ctx context.Context, filter metadata.LinkFilter) ([]metadata.SingleUseLink, string, error) {
	var afterID int64
	if "" != filter.Cursor {
		parsed, err := strconv.ParseInt(filter.Cursor, 10, 64)
		if nil != err || 0 >= parsed {
			return nil, "", fmt.Errorf("invalid link cursor: %w", metadata.ErrInvalidQuery)
		}
		afterID = parsed
	}

	now := filter.Now
	if now.IsZero() {
		now = time.Now()
	}
	limit := clampPageLimit(filter.Limit)

	rows, err := s.db.QueryContext(ctx, _SQL_LIST_SINGLE_USE_LINKS,
		filter.Status, filter.FilePath, filter.CreatedBy, afterID, now, limit)
	if nil != err {
		return nil, "", fmt.Errorf("failed to list single-use links: %w", err)
	}
	defer rows.Close()

	links := make([]metadata.SingleUseLink, 0, limit)
	for rows.Next() {
		link, err := scanLinkSummary(rows)
		if nil != err {
			return nil, "", fmt.Errorf("failed to scan single-use link: %w", err)
		}
		links = append(links, *link)
	}
	if err := rows.Err(); nil != err {
		return nil, "", fmt.Errorf("failed to iterate single-use links: %w", err)
	}

	next := ""
	if limit == len(links) {
		next = strconv.FormatInt(links[len(links)-1].ID, 10)
	}
	return links, next, nil
}

func (s *PostgresStore) GetSingleUseLinkByID(ctx context.Context, id int64) (*metadata.SingleUseLink, error) {
	link, err := scanLinkSummary(s.db.QueryRowContext(ctx, _SQL_GET_SINGLE_USE_LINK_BY_ID, id))
	if nil != err {
		if sql.ErrNoRows == err {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get single-use link by id: %w", err)
	}
	return link, nil
}

func (s *PostgresStore) RevokeSingleUseLink(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, _SQL_REVOKE_SINGLE_USE_LINK, id)
	if nil != err {
		return fmt.Errorf("failed to revoke single-use link: %w", err)
	}

	affected, err := result.RowsAffected()
	if nil != err {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if 0 < affected {
		return nil
	}

	if _, err := s.GetSingleUseLinkByID(ctx, id); nil != err {
		return err
	}
	return metadata.ErrPrecondition
}

func (s *PostgresStore) SearchMetadata(ctx context.Context, q metadata.SearchQuery) (metadata.SearchResult, error) {
	return metadata.SearchByScan(ctx, s, q)
}

func (s *PostgresStore) OwnerStats(ctx context.Context, cursor string, limit int) ([]metadata.OwnerStat, string, error) {
	limit = clampPageLimit(limit)

	rows, err := s.db.QueryContext(ctx, _SQL_OWNER_STATS, cursor, limit)
	if nil != err {
		return nil, "", fmt.Errorf("failed to query owner stats: %w", err)
	}
	defer rows.Close()

	stats := make([]metadata.OwnerStat, 0, limit)
	for rows.Next() {
		var stat metadata.OwnerStat
		if err := rows.Scan(&stat.Owner, &stat.Files, &stat.Bytes); nil != err {
			return nil, "", fmt.Errorf("failed to scan owner stat: %w", err)
		}
		stats = append(stats, stat)
	}
	if err := rows.Err(); nil != err {
		return nil, "", fmt.Errorf("failed to iterate owner stats: %w", err)
	}

	next := ""
	if limit == len(stats) {
		next = stats[len(stats)-1].Owner
	}
	return stats, next, nil
}

func (s *PostgresStore) CountEmptyETags(ctx context.Context) (int, error) {
	return s.countRows(ctx, _SQL_COUNT_EMPTY_ETAGS)
}

func (s *PostgresStore) CountEmptyOwners(ctx context.Context) (int, error) {
	return s.countRows(ctx, _SQL_COUNT_EMPTY_OWNERS)
}

func (s *PostgresStore) countRows(ctx context.Context, query string) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, query).Scan(&count); nil != err {
		return 0, fmt.Errorf("failed to count metadata rows: %w", err)
	}
	return count, nil
}

func (s *PostgresStore) ListErasureInfos(ctx context.Context, prefix, cursor string, limit int) ([]*metadata.ErasureFileInfo, string, error) {
	limit = clampPageLimit(limit)

	rows, err := s.db.QueryContext(ctx, _SQL_LIST_ERASURE_INFOS, escapeLikePattern(prefix)+"%", cursor, limit)
	if nil != err {
		return nil, "", fmt.Errorf("failed to list erasure info: %w", err)
	}
	defer rows.Close()

	infos := make([]*metadata.ErasureFileInfo, 0, limit)
	var current *metadata.ErasureFileInfo
	for rows.Next() {
		var filePath string
		var dataShards, parityShards int
		var profileShardSize, originalSize int64
		var shardIndex, shardSize sql.NullInt64
		var instanceID, backendType, shardPath, checksum sql.NullString

		if err := rows.Scan(
			&filePath, &dataShards, &parityShards, &profileShardSize, &originalSize,
			&shardIndex, &instanceID, &backendType, &shardPath, &shardSize, &checksum,
		); nil != err {
			return nil, "", fmt.Errorf("failed to scan erasure info: %w", err)
		}

		if nil == current || current.FilePath != filePath {
			current = &metadata.ErasureFileInfo{
				FilePath:     filePath,
				OriginalSize: originalSize,
				DataShards:   dataShards,
				ParityShards: parityShards,
				ShardSize:    profileShardSize,
			}
			infos = append(infos, current)
		}

		if shardIndex.Valid {
			current.Shards = append(current.Shards, metadata.ErasureShardInfo{
				Index:       int(shardIndex.Int64),
				InstanceID:  instanceID.String,
				BackendType: backendType.String,
				Path:        shardPath.String,
				Size:        shardSize.Int64,
				Checksum:    checksum.String,
			})
		}
	}
	if err := rows.Err(); nil != err {
		return nil, "", fmt.Errorf("failed to iterate erasure info: %w", err)
	}

	next := ""
	if limit == len(infos) {
		next = infos[len(infos)-1].FilePath
	}
	return infos, next, nil
}
