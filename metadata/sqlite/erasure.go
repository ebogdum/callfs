package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ebogdum/callfs/metadata"
)

func (s *SQLiteStore) initErasureSchema() error {
	schema := `
CREATE TABLE IF NOT EXISTS erasure_profiles (
    file_path     TEXT PRIMARY KEY,
    data_shards   INTEGER NOT NULL,
    parity_shards INTEGER NOT NULL,
    shard_size    INTEGER NOT NULL,
    original_size INTEGER NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS erasure_shards (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    file_path     TEXT NOT NULL,
    shard_index   INTEGER NOT NULL,
    instance_id   TEXT NOT NULL,
    backend_type  TEXT NOT NULL,
    shard_path    TEXT NOT NULL,
    shard_size    INTEGER NOT NULL,
    checksum      TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(file_path, shard_index)
);

CREATE INDEX IF NOT EXISTS idx_erasure_shards_file ON erasure_shards(file_path);
CREATE INDEX IF NOT EXISTS idx_erasure_shards_instance ON erasure_shards(instance_id);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to initialize erasure schema: %w", err)
	}
	return nil
}

// CreateErasureInfo stores erasure coding metadata for a file.
func (s *SQLiteStore) CreateErasureInfo(ctx context.Context, filePath string, info *metadata.ErasureFileInfo) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO erasure_profiles (file_path, data_shards, parity_shards, shard_size, original_size)
		 VALUES (?, ?, ?, ?, ?)`,
		filePath, info.DataShards, info.ParityShards, info.ShardSize, info.OriginalSize,
	)
	if err != nil {
		return fmt.Errorf("failed to insert erasure profile: %w", err)
	}

	for _, shard := range info.Shards {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO erasure_shards (file_path, shard_index, instance_id, backend_type, shard_path, shard_size, checksum)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			filePath, shard.Index, shard.InstanceID, shard.BackendType, shard.Path, shard.Size, shard.Checksum,
		)
		if err != nil {
			return fmt.Errorf("failed to insert erasure shard %d: %w", shard.Index, err)
		}
	}

	return tx.Commit()
}

// GetErasureInfo retrieves erasure coding metadata for a file.
func (s *SQLiteStore) GetErasureInfo(ctx context.Context, filePath string) (*metadata.ErasureFileInfo, error) {
	var info metadata.ErasureFileInfo
	err := s.db.QueryRowContext(ctx,
		`SELECT file_path, data_shards, parity_shards, shard_size, original_size
		 FROM erasure_profiles WHERE file_path = ?`, filePath,
	).Scan(&info.FilePath, &info.DataShards, &info.ParityShards, &info.ShardSize, &info.OriginalSize)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get erasure profile: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT shard_index, instance_id, backend_type, shard_path, shard_size, checksum
		 FROM erasure_shards WHERE file_path = ? ORDER BY shard_index`, filePath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query erasure shards: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var si metadata.ErasureShardInfo
		if err := rows.Scan(&si.Index, &si.InstanceID, &si.BackendType, &si.Path, &si.Size, &si.Checksum); err != nil {
			return nil, fmt.Errorf("failed to scan erasure shard: %w", err)
		}
		info.Shards = append(info.Shards, si)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate erasure shards: %w", err)
	}

	return &info, nil
}

// DeleteErasureInfo removes erasure coding metadata for a file.
func (s *SQLiteStore) DeleteErasureInfo(ctx context.Context, filePath string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM erasure_shards WHERE file_path = ?`, filePath); err != nil {
		return fmt.Errorf("failed to delete erasure shards: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM erasure_profiles WHERE file_path = ?`, filePath); err != nil {
		return fmt.Errorf("failed to delete erasure profile: %w", err)
	}

	return tx.Commit()
}
