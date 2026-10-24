package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/metadata"
)

var dsnPathEscaper = strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23")

type execQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

var _ metadata.Batcher = (*SQLiteStore)(nil)

type SQLiteStore struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewSQLiteStore(dbPath string, logger *zap.Logger) (*SQLiteStore, error) {
	if err := restrictDatabaseFile(dbPath); err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "synchronous(NORMAL)")
	query.Set("_txlock", "immediate")
	dsnURL := url.URL{Scheme: "file", Opaque: dsnPathEscaper.Replace(dbPath), RawQuery: query.Encode()}
	db, err := sql.Open("sqlite", dsnURL.String())
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	// SQLite serializes writes internally; limit open connections to avoid
	// "database is locked" errors under concurrent load while still allowing
	// concurrent readers via WAL mode.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(0) // Reuse connections indefinitely

	store := &SQLiteStore{db: db, logger: logger}
	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.initErasureSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func restrictDatabaseFile(dbPath string) error {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return fmt.Errorf("failed to create sqlite database directory: %w", err)
	}
	file, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("failed to create sqlite database file: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("failed to restrict sqlite database file permissions: %w", err)
	}
	if err := file.Close(); nil != err {
		return fmt.Errorf("failed to close sqlite database file: %w", err)
	}
	return nil
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
    owner TEXT NOT NULL DEFAULT '',
    atime TEXT NOT NULL,
    mtime TEXT NOT NULL,
    ctime TEXT NOT NULL,
    backend_type TEXT NOT NULL,
    erasure_coded INTEGER NOT NULL DEFAULT 0,
    callfs_instance_id TEXT,
    symlink_target TEXT,
    etag TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);


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

CREATE INDEX IF NOT EXISTS idx_single_use_links_status ON single_use_links(status);
CREATE INDEX IF NOT EXISTS idx_single_use_links_expires_at ON single_use_links(expires_at);
`

	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to initialize sqlite schema: %w", err)
	}
	if _, err := s.db.Exec(`ALTER TABLE inodes ADD COLUMN etag TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return fmt.Errorf("failed to add etag column: %w", err)
	}
	if _, err := s.db.Exec(`ALTER TABLE single_use_links ADD COLUMN owner TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return fmt.Errorf("failed to add owner column: %w", err)
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_single_use_links_owner ON single_use_links(owner, status)`); err != nil {
		return fmt.Errorf("failed to create link owner index: %w", err)
	}
	if _, err := s.db.Exec(`DROP INDEX IF EXISTS idx_inodes_path`); err != nil {
		return fmt.Errorf("failed to drop redundant path index: %w", err)
	}
	if _, err := s.db.Exec(`DROP INDEX IF EXISTS idx_inodes_parent_id`); err != nil {
		return fmt.Errorf("failed to drop unused parent_id index: %w", err)
	}
	if _, err := s.db.Exec(`DROP INDEX IF EXISTS idx_single_use_links_token`); err != nil {
		return fmt.Errorf("failed to drop redundant token index: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Get(ctx context.Context, path string) (*metadata.Metadata, error) {
	query := `
		SELECT id, parent_id, name, path, type, size, mode, owner,
		       atime, mtime, ctime, backend_type, erasure_coded, callfs_instance_id,
		       symlink_target, etag, created_at, updated_at
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
		&md.Owner,
		&aTime,
		&mTime,
		&cTime,
		&md.BackendType,
		&md.ErasureCoded,
		&callfsInstanceID,
		&symlinkTarget,
		&md.ETag,
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

	if md.ATime, err = parseTimestamp(aTime); nil != err {
		return nil, err
	}
	if md.MTime, err = parseTimestamp(mTime); nil != err {
		return nil, err
	}
	if md.CTime, err = parseTimestamp(cTime); nil != err {
		return nil, err
	}
	if md.CreatedAt, err = parseTimestamp(createdAt); nil != err {
		return nil, err
	}
	if md.UpdatedAt, err = parseTimestamp(updatedAt); nil != err {
		return nil, err
	}

	return &md, nil
}

func (s *SQLiteStore) Create(ctx context.Context, md *metadata.Metadata) error {
	return createInode(ctx, s.db, md)
}

func createInode(ctx context.Context, q execQuerier, md *metadata.Metadata) error {
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
			parent_id, name, path, type, size, mode, owner,
			atime, mtime, ctime, backend_type, erasure_coded, callfs_instance_id,
			symlink_target, etag, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := q.ExecContext(
		ctx,
		query,
		nullInt64(md.ParentID),
		md.Name,
		md.Path,
		md.Type,
		md.Size,
		md.Mode,
		md.Owner,
		md.ATime.UTC().Format(time.RFC3339Nano),
		md.MTime.UTC().Format(time.RFC3339Nano),
		md.CTime.UTC().Format(time.RFC3339Nano),
		md.BackendType,
		md.ErasureCoded,
		nullString(md.CallFSInstanceID),
		nullString(md.SymlinkTarget),
		md.ETag,
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
	if nil != err {
		return fmt.Errorf("failed to read created metadata id: %w", err)
	}
	md.ID = id

	return nil
}

func (s *SQLiteStore) Update(ctx context.Context, md *metadata.Metadata) error {
	return updateInode(ctx, s.db, md)
}

func updateInode(ctx context.Context, q execQuerier, md *metadata.Metadata) error {
	md.UpdatedAt = time.Now().UTC()
	query := `
		UPDATE inodes
		SET size = ?, mode = ?, owner = ?, atime = ?, mtime = ?, ctime = ?,
		    backend_type = ?, erasure_coded = ?, callfs_instance_id = ?, symlink_target = ?,
		    etag = ?, updated_at = ?
		WHERE path = ?`

	result, err := q.ExecContext(
		ctx,
		query,
		md.Size,
		md.Mode,
		md.Owner,
		md.ATime.UTC().Format(time.RFC3339Nano),
		md.MTime.UTC().Format(time.RFC3339Nano),
		md.CTime.UTC().Format(time.RFC3339Nano),
		md.BackendType,
		md.ErasureCoded,
		nullString(md.CallFSInstanceID),
		nullString(md.SymlinkTarget),
		md.ETag,
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

func (s *SQLiteStore) UpdateIfMatch(ctx context.Context, md *metadata.Metadata, expectedETag string) error {
	return updateInodeIfMatch(ctx, s.db, md, expectedETag)
}

func updateInodeIfMatch(ctx context.Context, q execQuerier, md *metadata.Metadata, expectedETag string) error {
	md.UpdatedAt = time.Now().UTC()
	query := `
		UPDATE inodes
		SET size = ?, mode = ?, owner = ?, atime = ?, mtime = ?, ctime = ?,
		    backend_type = ?, erasure_coded = ?, callfs_instance_id = ?, symlink_target = ?,
		    etag = ?, updated_at = ?
		WHERE path = ? AND etag = ?`

	result, err := q.ExecContext(
		ctx,
		query,
		md.Size,
		md.Mode,
		md.Owner,
		md.ATime.UTC().Format(time.RFC3339Nano),
		md.MTime.UTC().Format(time.RFC3339Nano),
		md.CTime.UTC().Format(time.RFC3339Nano),
		md.BackendType,
		md.ErasureCoded,
		nullString(md.CallFSInstanceID),
		nullString(md.SymlinkTarget),
		md.ETag,
		md.UpdatedAt.UTC().Format(time.RFC3339Nano),
		md.Path,
		expectedETag,
	)
	if err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected > 0 {
		return nil
	}

	var exists int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(1) FROM inodes WHERE path = ?`, md.Path).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check inode existence: %w", err)
	}
	if exists == 0 {
		return metadata.ErrNotFound
	}
	return metadata.ErrPrecondition
}

func (s *SQLiteStore) Delete(ctx context.Context, path string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin delete transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := deleteInode(ctx, tx, path); nil != err {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit delete: %w", err)
	}
	return nil
}

func deleteInode(ctx context.Context, q execQuerier, path string) error {
	result, err := q.ExecContext(ctx, `DELETE FROM inodes WHERE path = ?`, path)
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

	if _, err := q.ExecContext(ctx, `DELETE FROM erasure_shards WHERE file_path = ?`, path); err != nil {
		return fmt.Errorf("failed to delete erasure shards: %w", err)
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM erasure_profiles WHERE file_path = ?`, path); err != nil {
		return fmt.Errorf("failed to delete erasure profile: %w", err)
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM single_use_links WHERE file_path = ?`, path); err != nil {
		return fmt.Errorf("failed to delete single-use links: %w", err)
	}
	return nil
}

// Rename re-keys an inode (and, for a directory, its whole subtree) from
// oldPath to newPath in a single transaction. Only name and path change.
func (s *SQLiteStore) Rename(ctx context.Context, oldPath, newPath string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin rename transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := renameInode(ctx, tx, oldPath, newPath); nil != err {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit rename: %w", err)
	}
	return nil
}

func renameInode(ctx context.Context, q execQuerier, oldPath, newPath string) error {
	var srcType string
	err := q.QueryRowContext(ctx, `SELECT type FROM inodes WHERE path = ?`, oldPath).Scan(&srcType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return metadata.ErrNotFound
		}
		return fmt.Errorf("failed to read source inode: %w", err)
	}

	var destExists int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(1) FROM inodes WHERE path = ?`, newPath).Scan(&destExists); err != nil {
		return fmt.Errorf("failed to check destination inode: %w", err)
	}
	if destExists > 0 {
		return metadata.ErrAlreadyExists
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	newName := pathBase(newPath)

	if _, err := q.ExecContext(ctx,
		`UPDATE inodes SET name = ?, path = ?, updated_at = ? WHERE path = ?`,
		newName, newPath, now, oldPath,
	); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: inodes.path") {
			return metadata.ErrAlreadyExists
		}
		return fmt.Errorf("failed to rename inode: %w", err)
	}

	if srcType == "directory" {
		// Rewrite every descendant path: strip the old prefix, prepend the new one.
		oldPrefix := oldPath + "/"
		if _, err := q.ExecContext(ctx,
			`UPDATE inodes SET path = ? || substr(path, length(?) + 1), updated_at = ? WHERE path >= ? AND path < ?`,
			newPath+"/", oldPrefix, now, oldPrefix, subtreeUpperBound(oldPrefix),
		); err != nil {
			return fmt.Errorf("failed to rename descendants: %w", err)
		}
	}

	if err := renameFileRows(ctx, q, oldPath, newPath, srcType == "directory"); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) ApplyBatch(ctx context.Context, ops []metadata.Op) error {
	if 0 == len(ops) {
		return nil
	}
	if err := metadata.ValidateOps(ops); nil != err {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if nil != err {
		return fmt.Errorf("failed to begin batch transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, op := range ops {
		if err := applyBatchOp(ctx, tx, op); nil != err {
			return err
		}
	}

	if err := tx.Commit(); nil != err {
		return fmt.Errorf("failed to commit batch: %w", err)
	}
	return nil
}

func applyBatchOp(ctx context.Context, q execQuerier, op metadata.Op) error {
	switch op.Kind {
	case metadata.OpCreate:
		return createInode(ctx, q, op.Metadata)
	case metadata.OpUpdate:
		return updateInode(ctx, q, op.Metadata)
	case metadata.OpUpdateIfMatch:
		return updateInodeIfMatch(ctx, q, op.Metadata, op.ExpectedETag)
	case metadata.OpDelete:
		return deleteInode(ctx, q, op.Path)
	case metadata.OpRename:
		return renameInode(ctx, q, op.Path, op.NewPath)
	case metadata.OpCreateErasureInfo:
		return createErasureInfo(ctx, q, op.Path, op.ErasureInfo)
	case metadata.OpDeleteErasureInfo:
		return deleteErasureInfo(ctx, q, op.Path)
	}
	return fmt.Errorf("%w: unknown kind %q", metadata.ErrInvalidOp, op.Kind)
}

func subtreeUpperBound(prefix string) string {
	return prefix[:len(prefix)-1] + "0"
}

func renameFileRows(ctx context.Context, q execQuerier, oldPath, newPath string, isDir bool) error {
	oldPrefix := oldPath + "/"
	for _, table := range []string{"erasure_profiles", "erasure_shards", "single_use_links"} {
		if _, err := q.ExecContext(ctx,
			`UPDATE `+table+` SET file_path = ? WHERE file_path = ?`,
			newPath, oldPath,
		); err != nil {
			return fmt.Errorf("failed to rename %s: %w", table, err)
		}
		if isDir {
			if _, err := q.ExecContext(ctx,
				`UPDATE `+table+` SET file_path = ? || substr(file_path, length(?) + 1) WHERE file_path >= ? AND file_path < ?`,
				newPath+"/", oldPrefix, oldPrefix, subtreeUpperBound(oldPrefix),
			); err != nil {
				return fmt.Errorf("failed to rename %s descendants: %w", table, err)
			}
		}
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
			SELECT id, parent_id, name, path, type, size, mode, owner,
			       atime, mtime, ctime, backend_type, erasure_coded, callfs_instance_id,
			       symlink_target, etag, created_at, updated_at
			FROM inodes
			WHERE path >= '/' AND path < '0' AND instr(substr(path, 2), '/') = 0 AND path != '/'
			ORDER BY type DESC, name ASC`
		rows, err = s.db.QueryContext(ctx, query)
	} else {
		query := `
			SELECT id, parent_id, name, path, type, size, mode, owner,
			       atime, mtime, ctime, backend_type, erasure_coded, callfs_instance_id,
			       symlink_target, etag, created_at, updated_at
			FROM inodes
			WHERE path >= ? AND path < ? AND instr(substr(path, length(?) + 1), '/') = 0
			ORDER BY type DESC, name ASC`
		prefix := parentPath + "/"
		rows, err = s.db.QueryContext(ctx, query, prefix, subtreeUpperBound(prefix), prefix)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list children: %w", err)
	}
	defer rows.Close()

	return collectMetadataRows(rows)
}

func childPrefix(dirPath string) string {
	if "/" == dirPath {
		return "/"
	}
	return dirPath + "/"
}

func (s *SQLiteStore) HasChildren(ctx context.Context, parentPath string) (bool, error) {
	prefix := childPrefix(parentPath)
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM inodes
			WHERE path > ? AND path < ? AND instr(substr(path, length(?) + 1), '/') = 0
			LIMIT 1
		)`,
		prefix, subtreeUpperBound(prefix), prefix,
	).Scan(&exists)
	if nil != err {
		return false, fmt.Errorf("failed to check children: %w", err)
	}
	return exists, nil
}

func (s *SQLiteStore) ListDescendants(ctx context.Context, dirPath string, maxDepth, limit int) ([]*metadata.Metadata, error) {
	prefix := childPrefix(dirPath)
	sqlLimit := -1
	if 0 < limit {
		sqlLimit = limit
	}
	query := `
		SELECT id, parent_id, name, path, type, size, mode, owner,
		       atime, mtime, ctime, backend_type, erasure_coded, callfs_instance_id,
		       symlink_target, etag, created_at, updated_at
		FROM inodes
		WHERE path > ? AND path < ?
		  AND (? <= 0 OR length(substr(path, length(?) + 1)) - length(replace(substr(path, length(?) + 1), '/', '')) < ?)
		ORDER BY path
		LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, prefix, subtreeUpperBound(prefix), maxDepth, prefix, prefix, maxDepth, sqlLimit)
	if nil != err {
		return nil, fmt.Errorf("failed to list descendants: %w", err)
	}
	defer rows.Close()

	return collectMetadataRows(rows)
}

func (s *SQLiteStore) ListByPrefix(ctx context.Context, prefix, startAfter string, limit int) ([]*metadata.Metadata, error) {
	query := `
		SELECT id, parent_id, name, path, type, size, mode, owner,
		       atime, mtime, ctime, backend_type, erasure_coded, callfs_instance_id,
		       symlink_target, etag, created_at, updated_at
		FROM inodes
		WHERE type = 'file' AND substr(path, 1, length(?)) = ? AND path > ?
		ORDER BY path
		LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, prefix, prefix, startAfter, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list by prefix: %w", err)
	}
	defer rows.Close()

	return collectMetadataRows(rows)
}

func collectMetadataRows(rows *sql.Rows) ([]*metadata.Metadata, error) {
	entries := make([]*metadata.Metadata, 0)
	for rows.Next() {
		md, scanErr := scanMetadataRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		entries = append(entries, md)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}
	return entries, nil
}

func (s *SQLiteStore) GetSingleUseLink(ctx context.Context, token string) (*metadata.SingleUseLink, error) {
	return querySingleUseLink(ctx, s.db, token)
}

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func singleUseTokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func querySingleUseLink(ctx context.Context, q rowQuerier, token string) (*metadata.SingleUseLink, error) {
	query := `
		SELECT id, token, file_path, status, expires_at, used_at, used_by_ip, hmac_signature, created_at, updated_at
		FROM single_use_links
		WHERE token = ?`

	var link metadata.SingleUseLink
	var usedAt sql.NullString
	var usedByIP sql.NullString
	var expiresAt, createdAt, updatedAt string

	err := q.QueryRowContext(ctx, query, singleUseTokenDigest(token)).Scan(
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
	link.Token = token

	return &link, nil
}

func (s *SQLiteStore) CreateSingleUseLink(ctx context.Context, link *metadata.SingleUseLink) error {
	now := time.Now().UTC()
	if link.CreatedAt.IsZero() {
		link.CreatedAt = now
	}
	link.UpdatedAt = now

	query := `
		INSERT INTO single_use_links (token, file_path, owner, status, expires_at, used_at, used_by_ip, hmac_signature, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	result, err := s.db.ExecContext(
		ctx,
		query,
		singleUseTokenDigest(link.Token),
		link.FilePath,
		link.Owner,
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
	if nil != err {
		return fmt.Errorf("failed to read single-use link id: %w", err)
	}
	link.ID = id
	return nil
}

func (s *SQLiteStore) CountActiveSingleUseLinks(ctx context.Context, owner string, now time.Time) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM single_use_links
		WHERE owner = ? AND status = 'active' AND julianday(expires_at) > julianday(?)`
	var count int
	if err := s.db.QueryRowContext(ctx, query, owner, now.UTC().Format(time.RFC3339Nano)).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count active single-use links: %w", err)
	}
	return count, nil
}

func (s *SQLiteStore) ConsumeSingleUseLink(ctx context.Context, token string, usedByIP string, now time.Time) (*metadata.SingleUseLink, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if nil != err {
		return nil, false, fmt.Errorf("failed to begin single-use link transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stamp := now.UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `
		UPDATE single_use_links
		SET status = 'used', used_at = ?, used_by_ip = ?, updated_at = ?
		WHERE token = ? AND status = 'active' AND julianday(expires_at) > julianday(?)`,
		stamp, usedByIP, stamp, singleUseTokenDigest(token), stamp,
	)
	if nil != err {
		return nil, false, fmt.Errorf("failed to consume single-use link: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if nil != err {
		return nil, false, fmt.Errorf("failed to get rows affected: %w", err)
	}

	link, err := querySingleUseLink(ctx, tx, token)
	if nil != err {
		return nil, false, err
	}
	if err = tx.Commit(); nil != err {
		return nil, false, fmt.Errorf("failed to commit single-use link consume: %w", err)
	}
	return link, 1 == rowsAffected, nil
}

func (s *SQLiteStore) CleanupExpiredLinks(ctx context.Context, before time.Time) (int, error) {
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM single_use_links WHERE status = 'active' AND julianday(expires_at) < julianday(?)`,
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
		`DELETE FROM single_use_links WHERE status = 'used' AND julianday(used_at) < julianday(?)`,
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
		&md.Owner,
		&aTime,
		&mTime,
		&cTime,
		&md.BackendType,
		&md.ErasureCoded,
		&callfsInstanceID,
		&symlinkTarget,
		&md.ETag,
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
	if md.ATime, err = parseTimestamp(aTime); nil != err {
		return nil, err
	}
	if md.MTime, err = parseTimestamp(mTime); nil != err {
		return nil, err
	}
	if md.CTime, err = parseTimestamp(cTime); nil != err {
		return nil, err
	}
	if md.CreatedAt, err = parseTimestamp(createdAt); nil != err {
		return nil, err
	}
	if md.UpdatedAt, err = parseTimestamp(updatedAt); nil != err {
		return nil, err
	}
	return &md, nil
}

func parseTimestamp(value string) (time.Time, error) {
	if "" == value {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if nil != err {
		parsed, err = time.Parse(time.RFC3339, value)
		if nil != err {
			return time.Time{}, fmt.Errorf("invalid timestamp %q: %w", value, err)
		}
	}
	return parsed, nil
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

// pathBase returns the last path segment (the inode name) of an absolute path.
func pathBase(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

func nullStringTime(value *time.Time) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: value.UTC().Format(time.RFC3339Nano), Valid: true}
}
