DROP INDEX IF EXISTS idx_erasure_shards_instance;
DROP INDEX IF EXISTS idx_erasure_shards_file;
DROP TABLE IF EXISTS erasure_shards;
DROP TABLE IF EXISTS erasure_profiles;
ALTER TABLE inodes DROP COLUMN IF EXISTS erasure_coded;
