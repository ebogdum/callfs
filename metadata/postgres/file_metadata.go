package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/lib/pq"

	"github.com/ebogdum/callfs/v2/metadata"
)

// escapeLikePattern escapes SQL LIKE metacharacters (\, %, _)
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

// Get retrieves metadata for a file or directory by path
func (s *PostgresStore) Get(ctx context.Context, path string) (*metadata.Metadata, error) {
	var md metadata.Metadata
	var parentID sql.NullInt64
	var callfsInstanceID sql.NullString
	var symlinkTarget sql.NullString

	query := `
		SELECT id, parent_id, name, path, type, size, mode, owner,
		       atime, mtime, ctime, backend_type, erasure_coded, callfs_instance_id,
		       symlink_target, etag, created_at, updated_at
		FROM inodes
		WHERE path = $1`

	err := s.db.QueryRowContext(ctx, query, path).Scan(
		&md.ID,
		&parentID,
		&md.Name,
		&md.Path,
		&md.Type,
		&md.Size,
		&md.Mode,
		&md.Owner,
		&md.ATime,
		&md.MTime,
		&md.CTime,
		&md.BackendType,
		&md.ErasureCoded,
		&callfsInstanceID,
		&symlinkTarget,
		&md.ETag,
		&md.CreatedAt,
		&md.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	// Handle nullable fields
	if parentID.Valid {
		md.ParentID = &parentID.Int64
	}
	if callfsInstanceID.Valid {
		md.CallFSInstanceID = &callfsInstanceID.String
	}
	if symlinkTarget.Valid {
		md.SymlinkTarget = &symlinkTarget.String
	}

	return &md, nil
}

// Create creates a new inode entry
func (s *PostgresStore) Create(ctx context.Context, md *metadata.Metadata) error {
	return createInode(ctx, s.db, md)
}

func createInode(ctx context.Context, q dbQuerier, md *metadata.Metadata) error {
	if 255 < utf8.RuneCountInString(md.Name) {
		return metadata.ErrNameTooLong
	}

	var parentID sql.NullInt64
	var callfsInstanceID sql.NullString
	var symlinkTarget sql.NullString

	if md.ParentID != nil {
		parentID = sql.NullInt64{Int64: *md.ParentID, Valid: true}
	}
	if md.CallFSInstanceID != nil {
		callfsInstanceID = sql.NullString{String: *md.CallFSInstanceID, Valid: true}
	}
	if md.SymlinkTarget != nil {
		symlinkTarget = sql.NullString{String: *md.SymlinkTarget, Valid: true}
	}

	err := q.QueryRowContext(ctx, _SQL_CREATE_INODE,
		parentID,
		md.Name,
		md.Path,
		md.Type,
		md.Size,
		md.Mode,
		md.Owner,
		md.ATime,
		md.MTime,
		md.CTime,
		md.BackendType,
		md.ErasureCoded,
		callfsInstanceID,
		symlinkTarget,
		md.ETag,
	).Scan(&md.ID, &md.CreatedAt, &md.UpdatedAt)

	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return metadata.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create metadata: %w", err)
	}

	return nil
}

// Update updates an existing inode
func (s *PostgresStore) Update(ctx context.Context, md *metadata.Metadata) error {
	return updateInode(ctx, s.db, md)
}

func updateInode(ctx context.Context, q dbQuerier, md *metadata.Metadata) error {
	var callfsInstanceID sql.NullString
	var symlinkTarget sql.NullString

	if md.CallFSInstanceID != nil {
		callfsInstanceID = sql.NullString{String: *md.CallFSInstanceID, Valid: true}
	}
	if md.SymlinkTarget != nil {
		symlinkTarget = sql.NullString{String: *md.SymlinkTarget, Valid: true}
	}

	result, err := q.ExecContext(ctx, _SQL_UPDATE_INODE,
		md.Size,
		md.Mode,
		md.Owner,
		md.ATime,
		md.MTime,
		md.CTime,
		md.BackendType,
		md.ErasureCoded,
		callfsInstanceID,
		symlinkTarget,
		md.ETag,
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

// Delete removes an inode by path
func (s *PostgresStore) Delete(ctx context.Context, path string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin delete transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := deleteInode(ctx, tx, path); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit delete: %w", err)
	}
	return nil
}

func deleteInode(ctx context.Context, tx dbQuerier, path string) error {
	result, err := tx.ExecContext(ctx, `DELETE FROM inodes WHERE path = $1`, path)
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

	if _, err := tx.ExecContext(ctx, `DELETE FROM erasure_shards WHERE file_path = $1`, path); err != nil {
		return fmt.Errorf("failed to delete erasure shards: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM erasure_profiles WHERE file_path = $1`, path); err != nil {
		return fmt.Errorf("failed to delete erasure profile: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateIfMatch(ctx context.Context, md *metadata.Metadata, expectedETag string) error {
	return updateInodeIfMatch(ctx, s.db, md, expectedETag)
}

func updateInodeIfMatch(ctx context.Context, q dbQuerier, md *metadata.Metadata, expectedETag string) error {
	var callfsInstanceID sql.NullString
	var symlinkTarget sql.NullString

	if md.CallFSInstanceID != nil {
		callfsInstanceID = sql.NullString{String: *md.CallFSInstanceID, Valid: true}
	}
	if md.SymlinkTarget != nil {
		symlinkTarget = sql.NullString{String: *md.SymlinkTarget, Valid: true}
	}

	result, err := q.ExecContext(ctx, _SQL_UPDATE_INODE_IF_MATCH,
		md.Size,
		md.Mode,
		md.Owner,
		md.ATime,
		md.MTime,
		md.CTime,
		md.BackendType,
		md.ErasureCoded,
		callfsInstanceID,
		symlinkTarget,
		md.ETag,
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

	var exists bool
	if err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM inodes WHERE path = $1)`, md.Path).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check metadata existence: %w", err)
	}
	if !exists {
		return metadata.ErrNotFound
	}
	return metadata.ErrPrecondition
}

func (s *PostgresStore) ListByPrefix(ctx context.Context, prefix, startAfter string, limit int) ([]*metadata.Metadata, error) {
	rows, err := s.db.QueryContext(ctx, _SQL_LIST_FILES_BY_PREFIX, escapeLikePattern(prefix)+"%", startAfter, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list metadata by prefix: %w", err)
	}
	defer rows.Close()

	var entries []*metadata.Metadata
	for rows.Next() {
		var md metadata.Metadata
		var parentID sql.NullInt64
		var callfsInstanceID sql.NullString
		var symlinkTarget sql.NullString

		err := rows.Scan(
			&md.ID,
			&parentID,
			&md.Name,
			&md.Path,
			&md.Type,
			&md.Size,
			&md.Mode,
			&md.Owner,
			&md.ATime,
			&md.MTime,
			&md.CTime,
			&md.BackendType,
			&md.ErasureCoded,
			&callfsInstanceID,
			&symlinkTarget,
			&md.ETag,
			&md.CreatedAt,
			&md.UpdatedAt,
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

		entries = append(entries, &md)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return entries, nil
}

// pathBase returns the last path segment (the inode name) of an absolute path.
func pathBase(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

// Rename re-keys an inode (and, for a directory, its whole subtree) from
// oldPath to newPath in a single transaction. Only name and path change.
func (s *PostgresStore) Rename(ctx context.Context, oldPath, newPath string) error {
	if 255 < utf8.RuneCountInString(pathBase(newPath)) {
		return metadata.ErrNameTooLong
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin rename transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := renameInode(ctx, tx, oldPath, newPath); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit rename: %w", err)
	}
	return nil
}

func renameInode(ctx context.Context, tx dbQuerier, oldPath, newPath string) error {
	if 255 < utf8.RuneCountInString(pathBase(newPath)) {
		return metadata.ErrNameTooLong
	}

	var srcType string
	err := tx.QueryRowContext(ctx, `SELECT type FROM inodes WHERE path = $1`, oldPath).Scan(&srcType)
	if err != nil {
		if err == sql.ErrNoRows {
			return metadata.ErrNotFound
		}
		return fmt.Errorf("failed to read source inode: %w", err)
	}

	var destExists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM inodes WHERE path = $1)`, newPath).Scan(&destExists); err != nil {
		return fmt.Errorf("failed to check destination inode: %w", err)
	}
	if destExists {
		return metadata.ErrAlreadyExists
	}

	newName := pathBase(newPath)
	if _, err := tx.ExecContext(ctx,
		`UPDATE inodes SET name = $1, path = $2, updated_at = NOW() WHERE path = $3`,
		newName, newPath, oldPath,
	); err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return metadata.ErrAlreadyExists
		}
		return fmt.Errorf("failed to rename inode: %w", err)
	}

	if srcType == "directory" {
		// substring is 1-indexed; +2 skips the old path plus its trailing '/'.
		prefixLen := utf8.RuneCountInString(oldPath) + 2
		lower, upper := subtreeBounds(oldPath)
		var subtreeSize int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM (SELECT 1 FROM inodes WHERE path COLLATE "C" > $1 AND path COLLATE "C" < $2 LIMIT $3) AS capped`,
			lower, upper, maxRenameSubtreeEntries+1,
		).Scan(&subtreeSize); err != nil {
			return fmt.Errorf("failed to count descendants: %w", err)
		}
		if maxRenameSubtreeEntries < subtreeSize {
			return fmt.Errorf("%w: directory subtree exceeds %d entries", metadata.ErrSubtreeTooLarge, maxRenameSubtreeEntries)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE inodes SET path = $1 || substring(path FROM $2), updated_at = NOW() WHERE path COLLATE "C" > $3 AND path COLLATE "C" < $4`,
			newPath+"/", prefixLen, lower, upper,
		); err != nil {
			return fmt.Errorf("failed to rename descendants: %w", err)
		}
	}

	return renameErasureRows(ctx, tx, oldPath, newPath, srcType == "directory")
}

const maxRenameSubtreeEntries = 100000

func subtreeBounds(dir string) (string, string) {
	lower := dir + "/"
	if dir == "/" {
		lower = "/"
	}
	return lower, strings.TrimSuffix(lower, "/") + "0"
}

const childrenSelect = `
		SELECT id, parent_id, name, path, type, size, mode, owner,
		       atime, mtime, ctime, backend_type, erasure_coded, callfs_instance_id,
		       symlink_target, etag, created_at, updated_at
		FROM inodes`

const childrenWhere = ` WHERE path COLLATE "C" > $1 AND path COLLATE "C" < $2
		AND strpos(substr(path, length($1::text) + 1), '/') = 0`

// ListChildren lists all direct children of a directory
func (s *PostgresStore) ListChildren(ctx context.Context, parentPath string) ([]*metadata.Metadata, error) {
	lower, upper := subtreeBounds(parentPath)
	return s.queryChildren(ctx, childrenSelect+childrenWhere+`
		ORDER BY type DESC, name ASC`, lower, upper)
}

func (s *PostgresStore) HasChildren(ctx context.Context, parentPath string) (bool, error) {
	lower, upper := subtreeBounds(parentPath)
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM inodes
			WHERE path COLLATE "C" > $1 AND path COLLATE "C" < $2
			AND strpos(substr(path, length($1::text) + 1), '/') = 0
		)`, lower, upper).Scan(&exists)
	if nil != err {
		return false, fmt.Errorf("failed to check children: %w", err)
	}
	return exists, nil
}

func (s *PostgresStore) ListDescendants(ctx context.Context, dirPath string, maxDepth, limit int) ([]*metadata.Metadata, error) {
	lower, upper := subtreeBounds(dirPath)
	return s.queryChildren(ctx, childrenSelect+` WHERE path COLLATE "C" > $1 AND path COLLATE "C" < $2
		AND ($3::int <= 0 OR length(substr(path, length($1::text) + 1)) - length(replace(substr(path, length($1::text) + 1), '/', '')) < $3::int)
		ORDER BY path COLLATE "C"
		LIMIT NULLIF($4::int, 0)`, lower, upper, maxDepth, limit)
}

func (s *PostgresStore) ListChildrenPage(ctx context.Context, parentPath, afterType, afterName string, limit int) ([]*metadata.Metadata, error) {
	lower, upper := subtreeBounds(parentPath)
	return s.queryChildren(ctx, childrenSelect+childrenWhere+`
		AND ($3::text = '' OR type < $3::text OR (type = $3::text AND name > $4::text))
		ORDER BY type DESC, name ASC
		LIMIT $5`, lower, upper, afterType, afterName, limit)
}

func (s *PostgresStore) queryChildren(ctx context.Context, query string, args ...any) ([]*metadata.Metadata, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list children: %w", err)
	}
	defer rows.Close()

	var children []*metadata.Metadata
	for rows.Next() {
		var md metadata.Metadata
		var parentID sql.NullInt64
		var callfsInstanceID sql.NullString
		var symlinkTarget sql.NullString

		err := rows.Scan(
			&md.ID,
			&parentID,
			&md.Name,
			&md.Path,
			&md.Type,
			&md.Size,
			&md.Mode,
			&md.Owner,
			&md.ATime,
			&md.MTime,
			&md.CTime,
			&md.BackendType,
			&md.ErasureCoded,
			&callfsInstanceID,
			&symlinkTarget,
			&md.ETag,
			&md.CreatedAt,
			&md.UpdatedAt,
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

		children = append(children, &md)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return children, nil
}

func renameErasureRows(ctx context.Context, tx dbQuerier, oldPath, newPath string, isDir bool) error {
	for _, table := range []string{"erasure_profiles", "erasure_shards"} {
		if _, err := tx.ExecContext(ctx,
			`UPDATE `+table+` SET file_path = $1 WHERE file_path = $2`,
			newPath, oldPath,
		); err != nil {
			return fmt.Errorf("failed to rename %s: %w", table, err)
		}
		if isDir {
			if _, err := tx.ExecContext(ctx,
				`UPDATE `+table+` SET file_path = $1 || substring(file_path FROM $2) WHERE file_path LIKE $3 ESCAPE '\'`,
				newPath+"/", utf8.RuneCountInString(oldPath)+2, escapeLikePattern(oldPath)+"/%",
			); err != nil {
				return fmt.Errorf("failed to rename %s descendants: %w", table, err)
			}
		}
	}
	return nil
}
