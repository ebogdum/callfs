package postgres

// SQL query constants for metadata operations

const (
	// _SQL_CREATE_INODE creates a new inode entry
	_SQL_CREATE_INODE = `
		INSERT INTO inodes
		(parent_id, name, path, type, size, mode, owner, atime, mtime, ctime,
		 backend_type, erasure_coded, callfs_instance_id, symlink_target, etag)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, created_at, updated_at`

	// _SQL_UPDATE_INODE updates an existing inode entry
	_SQL_UPDATE_INODE = `
		UPDATE inodes
		SET size = $1, mode = $2, owner = $3, atime = $4, mtime = $5,
		    ctime = $6, backend_type = $7, erasure_coded = $8, callfs_instance_id = $9,
		    symlink_target = $10, etag = $11
		WHERE path = $12`

	_SQL_UPDATE_INODE_IF_MATCH = _SQL_UPDATE_INODE + ` AND etag = $13`

	_SQL_LIST_FILES_BY_PREFIX = `
		SELECT id, parent_id, name, path, type, size, mode, owner,
		       atime, mtime, ctime, backend_type, erasure_coded, callfs_instance_id,
		       symlink_target, etag, created_at, updated_at
		FROM inodes
		WHERE type = 'file' AND path LIKE $1 ESCAPE '\' AND path COLLATE "C" > $2
		ORDER BY path COLLATE "C"
		LIMIT $3`

	// _SQL_UPDATE_SINGLE_USE_LINK atomically updates a single-use link status
	_SQL_UPDATE_SINGLE_USE_LINK = `
		UPDATE single_use_links
		SET status = $2, used_at = $3, used_by_ip = $4
		WHERE token = $1 AND status = 'active'`

	_SQL_SELECT_SINGLE_USE_LINK_SUMMARY = `
		SELECT id, token, file_path, owner, expires_at, status, used_at, used_by_ip, created_at, updated_at
		FROM single_use_links`

	_SQL_LIST_SINGLE_USE_LINKS = _SQL_SELECT_SINGLE_USE_LINK_SUMMARY + `
		WHERE ($1::text = '' OR CASE
			WHEN $1::text = 'expired' THEN status = 'active' AND expires_at < $5::timestamptz
			WHEN $1::text = 'active' THEN status = 'active' AND expires_at >= $5::timestamptz
			ELSE status = $1::text
		END)
		AND ($2::text = '' OR file_path = $2::text)
		AND ($3::text = '' OR owner = $3::text)
		AND ($4::bigint = 0 OR id < $4::bigint)
		ORDER BY id DESC
		LIMIT $6`

	_SQL_GET_SINGLE_USE_LINK_BY_ID = _SQL_SELECT_SINGLE_USE_LINK_SUMMARY + `
		WHERE id = $1`

	_SQL_REVOKE_SINGLE_USE_LINK = `
		UPDATE single_use_links
		SET status = 'revoked'
		WHERE id = $1 AND status = 'active'`

	_SQL_OWNER_STATS = `
		SELECT owner, COUNT(*)::bigint, COALESCE(SUM(size), 0)::bigint
		FROM inodes
		WHERE type = 'file' AND owner <> '' AND owner COLLATE "C" > $1
		GROUP BY owner
		ORDER BY owner COLLATE "C"
		LIMIT $2`

	_SQL_COUNT_EMPTY_ETAGS = `
		SELECT COUNT(*) FROM inodes WHERE type = 'file' AND etag = ''`

	_SQL_COUNT_EMPTY_OWNERS = `
		SELECT COUNT(*) FROM inodes WHERE type = 'file' AND owner = ''`

	_SQL_LIST_ERASURE_INFOS = `
		SELECT p.file_path, p.data_shards, p.parity_shards, p.shard_size, p.original_size,
		       s.shard_index, s.instance_id, s.backend_type, s.shard_path, s.shard_size, s.checksum
		FROM (
			SELECT file_path, data_shards, parity_shards, shard_size, original_size
			FROM erasure_profiles
			WHERE file_path LIKE $1 ESCAPE '\' AND file_path COLLATE "C" > $2
			ORDER BY file_path COLLATE "C"
			LIMIT $3
		) p
		LEFT JOIN erasure_shards s ON s.file_path = p.file_path
		ORDER BY p.file_path COLLATE "C", s.shard_index`
)
