ALTER TABLE inodes ADD COLUMN erasure_coded BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE erasure_profiles (
    file_path    TEXT PRIMARY KEY,
    data_shards  INTEGER NOT NULL,
    parity_shards INTEGER NOT NULL,
    shard_size   BIGINT NOT NULL,
    original_size BIGINT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE erasure_shards (
    id           BIGSERIAL PRIMARY KEY,
    file_path    TEXT NOT NULL,
    shard_index  INTEGER NOT NULL,
    instance_id  VARCHAR(100) NOT NULL,
    backend_type VARCHAR(50) NOT NULL,
    shard_path   TEXT NOT NULL,
    shard_size   BIGINT NOT NULL,
    checksum     VARCHAR(64) NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(file_path, shard_index)
);

CREATE INDEX idx_erasure_shards_file ON erasure_shards(file_path);
CREATE INDEX idx_erasure_shards_instance ON erasure_shards(instance_id);
