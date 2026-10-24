DROP INDEX IF EXISTS idx_inodes_path;
DROP INDEX IF EXISTS idx_inodes_name;
DROP INDEX IF EXISTS idx_inodes_backend_type;
DROP INDEX IF EXISTS idx_inodes_callfs_instance_id;
DROP INDEX IF EXISTS idx_single_use_links_token;
DROP INDEX IF EXISTS idx_single_use_links_file_path;
DROP INDEX IF EXISTS idx_erasure_shards_file;
DROP INDEX IF EXISTS idx_erasure_shards_instance;

CREATE INDEX IF NOT EXISTS idx_inodes_path_c ON inodes (path COLLATE "C");
CREATE INDEX IF NOT EXISTS idx_single_use_links_active_expires_at ON single_use_links (expires_at) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_single_use_links_used_used_at ON single_use_links (used_at) WHERE status = 'used';
