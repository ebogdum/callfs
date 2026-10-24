package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ebogdum/callfs/metadata"
)

const (
	uiDefaultLimit = 200
	uiMaxLimit     = 1000

	singleUseLinkListColumns = `id, file_path, owner, status, expires_at, used_at, used_by_ip, created_at, updated_at`
)

type rowScanner interface {
	Scan(dest ...any) error
}

func clampUILimit(limit int) int {
	switch {
	case 0 >= limit:
		return uiDefaultLimit
	case uiMaxLimit < limit:
		return uiMaxLimit
	}
	return limit
}

func scanUILink(row rowScanner) (*metadata.SingleUseLink, error) {
	var link metadata.SingleUseLink
	var usedAt sql.NullString
	var usedByIP sql.NullString
	var expiresAt, createdAt, updatedAt string

	if err := row.Scan(&link.ID, &link.FilePath, &link.Owner, &link.Status, &expiresAt, &usedAt, &usedByIP, &createdAt, &updatedAt); nil != err {
		return nil, fmt.Errorf("failed to scan single-use link: %w", err)
	}
	var err error
	if link.ExpiresAt, err = parseTimestamp(expiresAt); nil != err {
		return nil, err
	}
	if link.CreatedAt, err = parseTimestamp(createdAt); nil != err {
		return nil, err
	}
	if link.UpdatedAt, err = parseTimestamp(updatedAt); nil != err {
		return nil, err
	}
	if usedAt.Valid {
		t, parseErr := parseTimestamp(usedAt.String)
		if nil != parseErr {
			return nil, parseErr
		}
		link.UsedAt = &t
	}
	if usedByIP.Valid {
		link.UsedByIP = &usedByIP.String
	}
	return &link, nil
}

func (s *SQLiteStore) ListSingleUseLinks(ctx context.Context, filter metadata.LinkFilter) ([]metadata.SingleUseLink, string, error) {
	now := filter.Now
	if now.IsZero() {
		now = time.Now()
	}
	stamp := now.UTC().Format(time.RFC3339Nano)

	var clauses []string
	var args []any
	switch filter.Status {
	case "":
	case "active":
		clauses = append(clauses, `status = 'active' AND julianday(expires_at) > julianday(?)`)
		args = append(args, stamp)
	case "expired":
		clauses = append(clauses, `(status = 'expired' OR (status = 'active' AND julianday(expires_at) <= julianday(?)))`)
		args = append(args, stamp)
	case "used", "revoked":
		clauses = append(clauses, `status = ?`)
		args = append(args, filter.Status)
	default:
		return nil, "", metadata.ErrInvalidQuery
	}
	if "" != filter.FilePath {
		clauses = append(clauses, `file_path = ?`)
		args = append(args, filter.FilePath)
	}
	if "" != filter.CreatedBy {
		clauses = append(clauses, `owner = ?`)
		args = append(args, filter.CreatedBy)
	}
	if "" != filter.Cursor {
		afterID, err := strconv.ParseInt(filter.Cursor, 10, 64)
		if nil != err || 0 >= afterID {
			return nil, "", metadata.ErrInvalidQuery
		}
		clauses = append(clauses, `id < ?`)
		args = append(args, afterID)
	}

	query := `SELECT ` + singleUseLinkListColumns + ` FROM single_use_links`
	if 0 < len(clauses) {
		query += ` WHERE ` + strings.Join(clauses, ` AND `)
	}
	limit := clampUILimit(filter.Limit)
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if nil != err {
		return nil, "", fmt.Errorf("failed to list single-use links: %w", err)
	}
	defer rows.Close()

	links := make([]metadata.SingleUseLink, 0, limit+1)
	for rows.Next() {
		link, scanErr := scanUILink(rows)
		if nil != scanErr {
			return nil, "", scanErr
		}
		links = append(links, *link)
	}
	if err := rows.Err(); nil != err {
		return nil, "", fmt.Errorf("failed to iterate single-use links: %w", err)
	}

	next := ""
	if limit < len(links) {
		links = links[:limit]
		next = strconv.FormatInt(links[limit-1].ID, 10)
	}
	return links, next, nil
}

func (s *SQLiteStore) GetSingleUseLinkByID(ctx context.Context, id int64) (*metadata.SingleUseLink, error) {
	link, err := scanUILink(s.db.QueryRowContext(ctx,
		`SELECT `+singleUseLinkListColumns+` FROM single_use_links WHERE id = ?`, id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, metadata.ErrNotFound
	}
	return link, err
}

func (s *SQLiteStore) RevokeSingleUseLink(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE single_use_links SET status = 'revoked', updated_at = ? WHERE id = ? AND status = 'active'`,
		time.Now().UTC().Format(time.RFC3339Nano), id,
	)
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

func (s *SQLiteStore) SearchMetadata(ctx context.Context, q metadata.SearchQuery) (metadata.SearchResult, error) {
	return metadata.SearchByScan(ctx, s, q)
}

func (s *SQLiteStore) OwnerStats(ctx context.Context, cursor string, limit int) ([]metadata.OwnerStat, string, error) {
	limit = clampUILimit(limit)
	rows, err := s.db.QueryContext(ctx, `
		SELECT owner, COUNT(*), COALESCE(SUM(size), 0)
		FROM inodes
		WHERE type = 'file' AND (? = '' OR owner > ?)
		GROUP BY owner
		ORDER BY owner
		LIMIT ?`, cursor, cursor, limit+1)
	if nil != err {
		return nil, "", fmt.Errorf("failed to query owner stats: %w", err)
	}
	defer rows.Close()

	stats := make([]metadata.OwnerStat, 0, limit+1)
	for rows.Next() {
		var stat metadata.OwnerStat
		if scanErr := rows.Scan(&stat.Owner, &stat.Files, &stat.Bytes); nil != scanErr {
			return nil, "", fmt.Errorf("failed to scan owner stat: %w", scanErr)
		}
		stats = append(stats, stat)
	}
	if err := rows.Err(); nil != err {
		return nil, "", fmt.Errorf("failed to iterate owner stats: %w", err)
	}

	next := ""
	if limit < len(stats) {
		stats = stats[:limit]
		next = stats[limit-1].Owner
	}
	return stats, next, nil
}

func (s *SQLiteStore) CountEmptyETags(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inodes WHERE type = 'file' AND etag = ''`).Scan(&count); nil != err {
		return 0, fmt.Errorf("failed to count empty etags: %w", err)
	}
	return count, nil
}

func (s *SQLiteStore) CountEmptyOwners(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inodes WHERE owner = ''`).Scan(&count); nil != err {
		return 0, fmt.Errorf("failed to count empty owners: %w", err)
	}
	return count, nil
}
