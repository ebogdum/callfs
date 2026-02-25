package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ebogdum/callfs/metadata"
)

// Get retrieves metadata for a file or directory by path
func (s *PostgresStore) Get(ctx context.Context, path string) (*metadata.Metadata, error) {
	var md metadata.Metadata
	var parentID sql.NullInt64
	var callfsInstanceID sql.NullString
	var symlinkTarget sql.NullString

	query := `
		SELECT id, parent_id, name, path, type, size, mode, uid, gid, 
		       atime, mtime, ctime, backend_type, callfs_instance_id,
		       symlink_target, created_at, updated_at
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
		&md.UID,
		&md.GID,
		&md.ATime,
		&md.MTime,
		&md.CTime,
		&md.BackendType,
		&callfsInstanceID,
		&symlinkTarget,
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

	err := s.db.QueryRowContext(ctx, _SQL_CREATE_INODE,
		parentID,
		md.Name,
		md.Path,
		md.Type,
		md.Size,
		md.Mode,
		md.UID,
		md.GID,
		md.ATime,
		md.MTime,
		md.CTime,
		md.BackendType,
		callfsInstanceID,
		symlinkTarget,
	).Scan(&md.ID, &md.CreatedAt, &md.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create metadata: %w", err)
	}

	return nil
}

// Update updates an existing inode
func (s *PostgresStore) Update(ctx context.Context, md *metadata.Metadata) error {
	var callfsInstanceID sql.NullString
	var symlinkTarget sql.NullString

	if md.CallFSInstanceID != nil {
		callfsInstanceID = sql.NullString{String: *md.CallFSInstanceID, Valid: true}
	}
	if md.SymlinkTarget != nil {
		symlinkTarget = sql.NullString{String: *md.SymlinkTarget, Valid: true}
	}

	_, err := s.db.ExecContext(ctx, _SQL_UPDATE_INODE,
		md.Size,
		md.Mode,
		md.UID,
		md.GID,
		md.ATime,
		md.MTime,
		md.CTime,
		md.BackendType,
		callfsInstanceID,
		symlinkTarget,
		md.Path,
	)

	if err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	return nil
}

// Delete removes an inode by path
func (s *PostgresStore) Delete(ctx context.Context, path string) error {
	query := `DELETE FROM inodes WHERE path = $1`

	result, err := s.db.ExecContext(ctx, query, path)
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

// ListChildren lists all direct children of a directory
func (s *PostgresStore) ListChildren(ctx context.Context, parentPath string) ([]*metadata.Metadata, error) {
	query := `
		SELECT id, parent_id, name, path, type, size, mode, uid, gid, 
		       atime, mtime, ctime, backend_type, callfs_instance_id,
		       symlink_target, created_at, updated_at
		FROM inodes
		WHERE path LIKE $1 || '/%' AND path NOT LIKE $1 || '/%/%'
		ORDER BY type DESC, name ASC`

	var (
		rows *sql.Rows
		err  error
	)

	if parentPath == "/" {
		rootQuery := `
			SELECT id, parent_id, name, path, type, size, mode, uid, gid,
			       atime, mtime, ctime, backend_type, callfs_instance_id,
			       symlink_target, created_at, updated_at
			FROM inodes
			WHERE path LIKE '/%' AND path NOT LIKE '/%/%' AND path != '/'
			ORDER BY type DESC, name ASC`
		rows, err = s.db.QueryContext(ctx, rootQuery)
	} else {
		rows, err = s.db.QueryContext(ctx, query, parentPath)
	}
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
			&md.UID,
			&md.GID,
			&md.ATime,
			&md.MTime,
			&md.CTime,
			&md.BackendType,
			&callfsInstanceID,
			&symlinkTarget,
			&md.CreatedAt,
			&md.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
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

		children = append(children, &md)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rows: %w", err)
	}

	return children, nil
}
