package postgres

// SQL query constants for metadata operations

const (
	// _SQL_GET_INODE_BY_PATH retrieves inode metadata by path
	_SQL_GET_INODE_BY_PATH = `
		SELECT id, parent_id, name, path, type, size, mode, uid, gid, 
		       atime, mtime, ctime, backend_type, callfs_instance_id, 
		       symlink_target, created_at, updated_at
		FROM inodes 
		WHERE path = $1`

	// _SQL_CREATE_INODE creates a new inode entry
	_SQL_CREATE_INODE = `
		INSERT INTO inodes 
		(parent_id, name, path, type, size, mode, uid, gid, atime, mtime, ctime, 
		 backend_type, callfs_instance_id, symlink_target)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, created_at, updated_at`

	// _SQL_UPDATE_INODE updates an existing inode entry
	_SQL_UPDATE_INODE = `
		UPDATE inodes 
		SET size = $1, mode = $2, uid = $3, gid = $4, atime = $5, mtime = $6, 
		    ctime = $7, backend_type = $8, callfs_instance_id = $9, symlink_target = $10
		WHERE path = $11`

	// _SQL_DELETE_INODE deletes an inode entry by path
	_SQL_DELETE_INODE = `
		DELETE FROM inodes 
		WHERE path = $1`

	// _SQL_LIST_CHILDREN lists all children of a directory
	_SQL_LIST_CHILDREN = `
		SELECT id, parent_id, name, path, type, size, mode, uid, gid, 
		       atime, mtime, ctime, backend_type, callfs_instance_id, 
		       symlink_target, created_at, updated_at
		FROM inodes 
		WHERE path LIKE $1 || '%' AND path != $1 
		  AND position('/' in substring(path from length($1) + 2)) = 0
		ORDER BY type DESC, name ASC`

	// _SQL_GET_SINGLE_USE_LINK retrieves a single-use link by token
	_SQL_GET_SINGLE_USE_LINK = `
		SELECT id, token, file_path, status, expires_at, used_at, used_by_ip, 
		       hmac_signature, created_at, updated_at
		FROM single_use_links 
		WHERE token = $1`

	// _SQL_CREATE_SINGLE_USE_LINK creates a new single-use link
	_SQL_CREATE_SINGLE_USE_LINK = `
		INSERT INTO single_use_links 
		(token, file_path, status, expires_at, hmac_signature)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	// _SQL_UPDATE_SINGLE_USE_LINK atomically updates a single-use link status
	_SQL_UPDATE_SINGLE_USE_LINK = `
		UPDATE single_use_links 
		SET status = $2, used_at = $3, used_by_ip = $4
		WHERE token = $1 AND status = 'active'`

	// _SQL_CLEANUP_EXPIRED_LINKS removes expired active links
	_SQL_CLEANUP_EXPIRED_LINKS = `
		DELETE FROM single_use_links 
		WHERE status = 'active' AND expires_at < $1`

	// _SQL_CLEANUP_USED_LINKS removes used links older than the given time
	_SQL_CLEANUP_USED_LINKS = `
		DELETE FROM single_use_links 
		WHERE status = 'used' AND used_at < $1`
)
