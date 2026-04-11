package postgres

// SQL query constants for metadata operations

const (
	// _SQL_CREATE_INODE creates a new inode entry
	_SQL_CREATE_INODE = `
		INSERT INTO inodes
		(parent_id, name, path, type, size, mode, owner, atime, mtime, ctime,
		 backend_type, erasure_coded, callfs_instance_id, symlink_target)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, created_at, updated_at`

	// _SQL_UPDATE_INODE updates an existing inode entry
	_SQL_UPDATE_INODE = `
		UPDATE inodes
		SET size = $1, mode = $2, owner = $3, atime = $4, mtime = $5,
		    ctime = $6, backend_type = $7, erasure_coded = $8, callfs_instance_id = $9,
		    symlink_target = $10
		WHERE path = $11`

	// _SQL_UPDATE_SINGLE_USE_LINK atomically updates a single-use link status
	_SQL_UPDATE_SINGLE_USE_LINK = `
		UPDATE single_use_links
		SET status = $2, used_at = $3, used_by_ip = $4
		WHERE token = $1 AND status = 'active'`
)
