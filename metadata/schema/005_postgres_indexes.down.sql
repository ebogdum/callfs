DROP INDEX IF EXISTS idx_single_use_links_used_used_at;
DROP INDEX IF EXISTS idx_single_use_links_active_expires_at;
DROP INDEX IF EXISTS idx_inodes_path_c;

CREATE INDEX IF NOT EXISTS idx_erasure_shards_instance ON erasure_shards(instance_id);
CREATE INDEX IF NOT EXISTS idx_erasure_shards_file ON erasure_shards(file_path);
CREATE INDEX IF NOT EXISTS idx_single_use_links_file_path ON single_use_links(file_path);
CREATE INDEX IF NOT EXISTS idx_single_use_links_token ON single_use_links(token);
CREATE INDEX IF NOT EXISTS idx_inodes_callfs_instance_id ON inodes(callfs_instance_id);
CREATE INDEX IF NOT EXISTS idx_inodes_backend_type ON inodes(backend_type);
CREATE INDEX IF NOT EXISTS idx_inodes_name ON inodes(name);
CREATE INDEX IF NOT EXISTS idx_inodes_path ON inodes(path);
