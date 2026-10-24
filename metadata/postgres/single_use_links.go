package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/metadata"
)

// GetSingleUseLink retrieves a single-use link by token
func (s *PostgresStore) GetSingleUseLink(ctx context.Context, token string) (*metadata.SingleUseLink, error) {
	query := `
		SELECT id, token, file_path, created_at, expires_at, status, used_at, used_by_ip, hmac_signature
		FROM single_use_links
		WHERE token = $1`

	link, err := scanSingleUseLink(s.db.QueryRowContext(ctx, query, hashSingleUseToken(token)))
	if nil != err {
		if sql.ErrNoRows == err {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get single-use link: %w", err)
	}

	link.Token = token
	return link, nil
}

func hashSingleUseToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func scanSingleUseLink(row *sql.Row) (*metadata.SingleUseLink, error) {
	var link metadata.SingleUseLink
	var usedAt sql.NullTime
	var usedByIP sql.NullString

	err := row.Scan(
		&link.ID,
		&link.Token,
		&link.FilePath,
		&link.CreatedAt,
		&link.ExpiresAt,
		&link.Status,
		&usedAt,
		&usedByIP,
		&link.HMACSignature,
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

func (s *PostgresStore) ConsumeSingleUseLink(ctx context.Context, token string, usedByIP string, now time.Time) (*metadata.SingleUseLink, bool, error) {
	var usedByIPArg sql.NullString
	if zone := strings.IndexByte(usedByIP, '%'); 0 <= zone {
		usedByIP = usedByIP[:zone]
	}
	if nil != net.ParseIP(usedByIP) {
		usedByIPArg = sql.NullString{String: usedByIP, Valid: true}
	}

	query := `
		UPDATE single_use_links
		SET status = 'used', used_at = $2, used_by_ip = $3
		WHERE token = $1 AND status = 'active' AND expires_at > $2
		RETURNING id, token, file_path, created_at, expires_at, status, used_at, used_by_ip, hmac_signature`

	link, err := scanSingleUseLink(s.db.QueryRowContext(ctx, query, hashSingleUseToken(token), now, usedByIPArg))
	if nil == err {
		link.Token = token
		return link, true, nil
	}
	if sql.ErrNoRows != err {
		return nil, false, fmt.Errorf("failed to consume single-use link: %w", err)
	}

	stored, err := s.GetSingleUseLink(ctx, token)
	if nil != err {
		return nil, false, err
	}

	return stored, false, nil
}

// CreateSingleUseLink creates a new single-use download link
func (s *PostgresStore) CreateSingleUseLink(ctx context.Context, link *metadata.SingleUseLink) error {
	query := `
		INSERT INTO single_use_links (token, file_path, owner, created_at, expires_at, status, hmac_signature)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := s.db.ExecContext(ctx, query,
		hashSingleUseToken(link.Token),
		link.FilePath,
		link.Owner,
		link.CreatedAt,
		link.ExpiresAt,
		link.Status,
		link.HMACSignature,
	)

	if err != nil {
		return fmt.Errorf("failed to create single-use link: %w", err)
	}

	return nil
}

func (s *PostgresStore) CountActiveSingleUseLinks(ctx context.Context, owner string, now time.Time) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM single_use_links
		WHERE owner = $1 AND status = 'active' AND expires_at > $2`

	var count int
	if err := s.db.QueryRowContext(ctx, query, owner, now).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count active single-use links: %w", err)
	}
	return count, nil
}

// UpdateSingleUseLink updates the status and usage information of a single-use link
func (s *PostgresStore) UpdateSingleUseLink(ctx context.Context, token string, status string, usedAt *time.Time, usedByIP *string) error {
	var usedAtArg sql.NullTime
	var usedByIPArg sql.NullString

	if usedAt != nil {
		usedAtArg = sql.NullTime{Time: *usedAt, Valid: true}
	}
	if usedByIP != nil {
		usedByIPArg = sql.NullString{String: *usedByIP, Valid: true}
	}

	result, err := s.db.ExecContext(ctx, _SQL_UPDATE_SINGLE_USE_LINK, hashSingleUseToken(token), status, usedAtArg, usedByIPArg)
	if err != nil {
		return fmt.Errorf("failed to update single-use link: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return metadata.ErrNotFound
	}

	return nil
}

const (
	singleUseLinkCleanupBatchSize    = 1000
	singleUseLinkCleanupBatchTimeout = 10 * time.Second
)

func (s *PostgresStore) CleanupExpiredLinks(ctx context.Context, before time.Time) (int, error) {
	rowsAffected, err := s.deleteLinksInBatches(ctx,
		`DELETE FROM single_use_links WHERE id IN (
			SELECT id FROM single_use_links WHERE status = 'active' AND expires_at < $1 LIMIT $2)`,
		before)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired links: %w", err)
	}

	s.logger.Debug("Cleaned up expired single-use links",
		zap.Int64("count", rowsAffected),
		zap.Time("before", before))

	return int(rowsAffected), nil
}

func (s *PostgresStore) CleanupUsedLinks(ctx context.Context, olderThan time.Time) (int, error) {
	rowsAffected, err := s.deleteLinksInBatches(ctx,
		`DELETE FROM single_use_links WHERE id IN (
			SELECT id FROM single_use_links WHERE status = 'used' AND used_at < $1 LIMIT $2)`,
		olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup used links: %w", err)
	}

	return int(rowsAffected), nil
}

func (s *PostgresStore) deleteLinksInBatches(ctx context.Context, query string, cutoff time.Time) (int64, error) {
	var total int64
	for {
		n, err := s.deleteLinkBatch(ctx, query, cutoff)
		total += n
		if err != nil || n < singleUseLinkCleanupBatchSize {
			return total, err
		}
	}
}

func (s *PostgresStore) deleteLinkBatch(ctx context.Context, query string, cutoff time.Time) (int64, error) {
	batchCtx, cancel := context.WithTimeout(ctx, singleUseLinkCleanupBatchTimeout)
	defer cancel()

	result, err := s.db.ExecContext(batchCtx, query, cutoff, singleUseLinkCleanupBatchSize)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
