package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

type SQLiteStore struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewSQLiteStore(dbPath string, logger *zap.Logger) (*SQLiteStore, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	store := &SQLiteStore{db: db, logger: logger}
	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) initSchema() error {
	schema := `
CREATE TABLE IF NOT EXISTS inodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    parent_id INTEGER,
    name TEXT NOT NULL,
    path TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL CHECK (type IN ('file', 'directory')),
    size INTEGER NOT NULL DEFAULT 0,
    mode TEXT NOT NULL,
    uid INTEGER NOT NULL,
    gid INTEGER NOT NULL,
    atime TEXT NOT NULL,
    mtime TEXT NOT NULL,
    ctime TEXT NOT NULL,
    backend_type TEXT NOT NULL,
    callfs_instance_id TEXT,
    symlink_target TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_inodes_path ON inodes(path);
CREATE INDEX IF NOT EXISTS idx_inodes_parent_id ON inodes(parent_id);

CREATE TABLE IF NOT EXISTS single_use_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token TEXT NOT NULL UNIQUE,
    file_path TEXT NOT NULL,
    status TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used_at TEXT,
    used_by_ip TEXT,
    hmac_signature TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_single_use_links_token ON single_use_links(token);
CREATE INDEX IF NOT EXISTS idx_single_use_links_status ON single_use_links(status);
CREATE INDEX IF NOT EXISTS idx_single_use_links_expires_at ON single_use_links(expires_at);
`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to initialize sqlite schema: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Get(ctx context.Context, path string) (*metadata.Metadata, error) {
	query := `
		SELECT id, parent_id, name, path, type, size, mode, uid, gid,
		       atime, mtime, ctime, backend_type, callfs_instance_id,
		       symlink_target, created_at, updated_at
		FROM inodes
		WHERE path = ?`

	var md metadata.Metadata
	var parentID sql.NullInt64
	var callfsInstanceID sql.NullString
	var symlinkTarget sql.NullString
	var aTime, mTime, cTime, createdAt, updatedAt string

	err := s.db.QueryRowContext(ctx, query, path).Scan(
		&md.ID,
		&parentID,
		&md.Name,
		&md.Path,
		&md.Type,
		&md.Size,
		&md.Mode,
		&md.UID,
		&md.GID,
		&aTime,
		&mTime,
		&cTime,
		&md.BackendType,
		&callfsInstanceID,
		&symlinkTarget,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	if parentID.Valid {
		md.ParentID = &parentID.Int64
	}
	if callfsInstanceID.Valid {
		md.CallFSInstanceID = &callfsInstanceID.String
	}
	if symlinkTarget.Valid {
		md.SymlinkTarget = &symlinkTarget.String
	}

	md.ATime = parseTimestamp(aTime)
	md.MTime = parseTimestamp(mTime)
	md.CTime = parseTimestamp(cTime)
	md.CreatedAt = parseTimestamp(createdAt)
	md.UpdatedAt = parseTimestamp(updatedAt)

	return &md, nil
}

func (s *SQLiteStore) Create(ctx context.Context, md *metadata.Metadata) error {
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

	query := `
		INSERT INTO inodes (
			parent_id, name, path, type, size, mode, uid, gid,
			atime, mtime, ctime, backend_type, callfs_instance_id,
			symlink_target, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := s.db.ExecContext(
		ctx,
		query,
		nullInt64(md.ParentID),
		md.Name,
		md.Path,
		md.Type,
		md.Size,
		md.Mode,
		md.UID,
		md.GID,
		md.ATime.UTC().Format(time.RFC3339Nano),
		md.MTime.UTC().Format(time.RFC3339Nano),
		md.CTime.UTC().Format(time.RFC3339Nano),
		md.BackendType,
		nullString(md.CallFSInstanceID),
		nullString(md.SymlinkTarget),
		md.CreatedAt.UTC().Format(time.RFC3339Nano),
		md.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: inodes.path") {
			return metadata.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create metadata: %w", err)
	}

	id, err := result.LastInsertId()
	if err == nil {
		md.ID = id
	}

	return nil
}

func (s *SQLiteStore) Update(ctx context.Context, md *metadata.Metadata) error {
	md.UpdatedAt = time.Now().UTC()
	query := `
		UPDATE inodes
		SET size = ?, mode = ?, uid = ?, gid = ?, atime = ?, mtime = ?, ctime = ?,
		    backend_type = ?, callfs_instance_id = ?, symlink_target = ?, updated_at = ?
		WHERE path = ?`

	result, err := s.db.ExecContext(
		ctx,
		query,
		md.Size,
		md.Mode,
		md.UID,
		md.GID,
		md.ATime.UTC().Format(time.RFC3339Nano),
		md.MTime.UTC().Format(time.RFC3339Nano),
		md.CTime.UTC().Format(time.RFC3339Nano),
		md.BackendType,
		nullString(md.CallFSInstanceID),
		nullString(md.SymlinkTarget),
		md.UpdatedAt.UTC().Format(time.RFC3339Nano),
		md.Path,
	)
	if err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
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

func (s *SQLiteStore) Delete(ctx context.Context, path string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM inodes WHERE path = ?`, path)
	if err != nil {
		return fmt.Errorf("failed to delete metadata: %w", err)
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

func (s *SQLiteStore) ListChildren(ctx context.Context, parentPath string) ([]*metadata.Metadata, error) {
	var (
		rows *sql.Rows
		err  error
	)

	if parentPath == "/" {
		query := `
			SELECT id, parent_id, name, path, type, size, mode, uid, gid,
			       atime, mtime, ctime, backend_type, callfs_instance_id,
			       symlink_target, created_at, updated_at
			FROM inodes
			WHERE path LIKE '/%' AND instr(substr(path, 2), '/') = 0 AND path != '/'
			ORDER BY type DESC, name ASC`
		rows, err = s.db.QueryContext(ctx, query)
	} else {
		query := `
			SELECT id, parent_id, name, path, type, size, mode, uid, gid,
			       atime, mtime, ctime, backend_type, callfs_instance_id,
			       symlink_target, created_at, updated_at
			FROM inodes
			WHERE path LIKE ? AND path NOT LIKE ?
			ORDER BY type DESC, name ASC`
		rows, err = s.db.QueryContext(ctx, query, parentPath+"/%", parentPath+"/%/%")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list children: %w", err)
	}
	defer rows.Close()

	children := make([]*metadata.Metadata, 0)
	for rows.Next() {
		md, scanErr := scanMetadataRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		children = append(children, md)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}
	return children, nil
}

func (s *SQLiteStore) GetSingleUseLink(ctx context.Context, token string) (*metadata.SingleUseLink, error) {
	query := `
		SELECT id, token, file_path, status, expires_at, used_at, used_by_ip, hmac_signature, created_at, updated_at
		FROM single_use_links
		WHERE token = ?`

	var link metadata.SingleUseLink
	var usedAt sql.NullString
	var usedByIP sql.NullString
	var expiresAt, createdAt, updatedAt string

	err := s.db.QueryRowContext(ctx, query, token).Scan(
		&link.ID,
		&link.Token,
		&link.FilePath,
		&link.Status,
		&expiresAt,
		&usedAt,
		&usedByIP,
		&link.HMACSignature,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get single-use link: %w", err)
	}

	link.ExpiresAt = parseTimestamp(expiresAt)
	link.CreatedAt = parseTimestamp(createdAt)
	link.UpdatedAt = parseTimestamp(updatedAt)
	if usedAt.Valid {
		t := parseTimestamp(usedAt.String)
		link.UsedAt = &t
	}
	if usedByIP.Valid {
		link.UsedByIP = &usedByIP.String
	}

	return &link, nil
}

func (s *SQLiteStore) CreateSingleUseLink(ctx context.Context, link *metadata.SingleUseLink) error {
	now := time.Now().UTC()
	if link.CreatedAt.IsZero() {
		link.CreatedAt = now
	}
	link.UpdatedAt = now

	query := `
		INSERT INTO single_use_links (token, file_path, status, expires_at, used_at, used_by_ip, hmac_signature, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	result, err := s.db.ExecContext(
		ctx,
		query,
		link.Token,
		link.FilePath,
		link.Status,
		link.ExpiresAt.UTC().Format(time.RFC3339Nano),
		nullStringTime(link.UsedAt),
		nullString(link.UsedByIP),
		link.HMACSignature,
		link.CreatedAt.UTC().Format(time.RFC3339Nano),
		link.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("failed to create single-use link: %w", err)
	}
	id, err := result.LastInsertId()
	if err == nil {
		link.ID = id
	}
	return nil
}

func (s *SQLiteStore) UpdateSingleUseLink(ctx context.Context, token string, status string, usedAt *time.Time, usedByIP *string) error {
	query := `
		UPDATE single_use_links
		SET status = ?, used_at = ?, used_by_ip = ?, updated_at = ?
		WHERE token = ?`
	result, err := s.db.ExecContext(
		ctx,
		query,
		status,
		nullStringTime(usedAt),
		nullString(usedByIP),
		time.Now().UTC().Format(time.RFC3339Nano),
		token,
	)
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

func (s *SQLiteStore) CleanupExpiredLinks(ctx context.Context, before time.Time) (int, error) {
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM single_use_links WHERE expires_at < ?`,
		before.UTC().Format(time.RFC3339Nano),
	)
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

func (s *SQLiteStore) CleanupUsedLinks(ctx context.Context, olderThan time.Time) (int, error) {
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM single_use_links WHERE status = 'used' AND used_at < ?`,
		olderThan.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup used links: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return int(rowsAffected), nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func scanMetadataRow(rows *sql.Rows) (*metadata.Metadata, error) {
	var md metadata.Metadata
	var parentID sql.NullInt64
	var callfsInstanceID sql.NullString
	var symlinkTarget sql.NullString
	var aTime, mTime, cTime, createdAt, updatedAt string

	err := rows.Scan(
		&md.ID,
		&parentID,
		&md.Name,
		&md.Path,
		&md.Type,
		&md.Size,
		&md.Mode,
		&md.UID,
		&md.GID,
		&aTime,
		&mTime,
		&cTime,
		&md.BackendType,
		&callfsInstanceID,
		&symlinkTarget,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan row: %w", err)
	}

	if parentID.Valid {
		md.ParentID = &parentID.Int64
	}
	if callfsInstanceID.Valid {
		md.CallFSInstanceID = &callfsInstanceID.String
	}
	if symlinkTarget.Valid {
		md.SymlinkTarget = &symlinkTarget.String
	}
	md.ATime = parseTimestamp(aTime)
	md.MTime = parseTimestamp(mTime)
	md.CTime = parseTimestamp(cTime)
	md.CreatedAt = parseTimestamp(createdAt)
	md.UpdatedAt = parseTimestamp(updatedAt)
	return &md, nil
}

func parseTimestamp(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, value)
		if err != nil {
			return time.Time{}
		}
	}
	return parsed
}

func nullInt64(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func nullString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func nullStringTime(value *time.Time) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: value.UTC().Format(time.RFC3339Nano), Valid: true}
}
