package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

// GetSingleUseLink retrieves a single-use link by token
func (s *PostgresStore) GetSingleUseLink(ctx context.Context, token string) (*metadata.SingleUseLink, error) {
	var link metadata.SingleUseLink
	var usedAt sql.NullTime
	var usedByIP sql.NullString

	query := `
		SELECT token, file_path, created_at, expires_at, status, used_at, used_by_ip
		FROM single_use_links
		WHERE token = $1`

	err := s.db.QueryRowContext(ctx, query, token).Scan(
		&link.Token,
		&link.FilePath,
		&link.CreatedAt,
		&link.ExpiresAt,
		&link.Status,
		&usedAt,
		&usedByIP,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get single-use link: %w", err)
	}

	// Handle nullable fields
	if usedAt.Valid {
		link.UsedAt = &usedAt.Time
	}
	if usedByIP.Valid {
		link.UsedByIP = &usedByIP.String
	}

	return &link, nil
}

// CreateSingleUseLink creates a new single-use download link
func (s *PostgresStore) CreateSingleUseLink(ctx context.Context, link *metadata.SingleUseLink) error {
	query := `
		INSERT INTO single_use_links (token, file_path, created_at, expires_at, status)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := s.db.ExecContext(ctx, query,
		link.Token,
		link.FilePath,
		link.CreatedAt,
		link.ExpiresAt,
		link.Status,
	)

	if err != nil {
		return fmt.Errorf("failed to create single-use link: %w", err)
	}

	return nil
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

	// Update status and usage information
	query := `
		UPDATE single_use_links 
		SET status = $2,
		    used_at = $3,
		    used_by_ip = $4
		WHERE token = $1`

	result, err := s.db.ExecContext(ctx, query, token, status, usedAtArg, usedByIPArg)
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

// CleanupExpiredLinks removes expired single-use links
func (s *PostgresStore) CleanupExpiredLinks(ctx context.Context, before time.Time) (int, error) {
	query := `DELETE FROM single_use_links WHERE expires_at < $1`

	result, err := s.db.ExecContext(ctx, query, before)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired links: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	s.logger.Debug("Cleaned up expired single-use links",
		zap.Int64("count", rowsAffected),
		zap.Time("before", before))

	return int(rowsAffected), nil
}

// CleanupUsedLinks removes used single-use links older than specified time
func (s *PostgresStore) CleanupUsedLinks(ctx context.Context, olderThan time.Time) (int, error) {
	query := `DELETE FROM single_use_links WHERE status = 'used' AND used_at < $1`

	result, err := s.db.ExecContext(ctx, query, olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup used links: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return int(rowsAffected), nil
}
