package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ebogdum/callfs/v2/metadata"
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

CREATE INDEX IF NOT EXISTS idx_erasure_shards_instance ON erasure_shards(instance_id);
`
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to initialize erasure schema: %w", err)
	}
	if _, err := s.db.Exec(`DROP INDEX IF EXISTS idx_erasure_shards_file`); err != nil {
		return fmt.Errorf("failed to drop redundant erasure shard index: %w", err)
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

	if err := createErasureInfo(ctx, tx, filePath, info); nil != err {
		return err
	}

	return tx.Commit()
}

func createErasureInfo(ctx context.Context, q execQuerier, filePath string, info *metadata.ErasureFileInfo) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO erasure_profiles (file_path, data_shards, parity_shards, shard_size, original_size)
		 VALUES (?, ?, ?, ?, ?)`,
		filePath, info.DataShards, info.ParityShards, info.ShardSize, info.OriginalSize,
	)
	if err != nil {
		return fmt.Errorf("failed to insert erasure profile: %w", err)
	}

	for _, shard := range info.Shards {
		_, err = q.ExecContext(ctx,
			`INSERT INTO erasure_shards (file_path, shard_index, instance_id, backend_type, shard_path, shard_size, checksum)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			filePath, shard.Index, shard.InstanceID, shard.BackendType, shard.Path, shard.Size, shard.Checksum,
		)
		if err != nil {
			return fmt.Errorf("failed to insert erasure shard %d: %w", shard.Index, err)
		}
	}

	return nil
}

// GetErasureInfo retrieves erasure coding metadata for a file.
func (s *SQLiteStore) GetErasureInfo(ctx context.Context, filePath string) (*metadata.ErasureFileInfo, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var info metadata.ErasureFileInfo
	err = tx.QueryRowContext(ctx,
		`SELECT file_path, data_shards, parity_shards, shard_size, original_size
		 FROM erasure_profiles WHERE file_path = ?`, filePath,
	).Scan(&info.FilePath, &info.DataShards, &info.ParityShards, &info.ShardSize, &info.OriginalSize)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, metadata.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get erasure profile: %w", err)
	}

	rows, err := tx.QueryContext(ctx,
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

	if err := tx.Commit(); nil != err {
		return nil, fmt.Errorf("failed to commit erasure read: %w", err)
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

	if err := deleteErasureInfo(ctx, tx, filePath); nil != err {
		return err
	}

	return tx.Commit()
}

func deleteErasureInfo(ctx context.Context, q execQuerier, filePath string) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM erasure_shards WHERE file_path = ?`, filePath); err != nil {
		return fmt.Errorf("failed to delete erasure shards: %w", err)
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM erasure_profiles WHERE file_path = ?`, filePath); err != nil {
		return fmt.Errorf("failed to delete erasure profile: %w", err)
	}

	return nil
}

func (s *SQLiteStore) ListErasureInfos(ctx context.Context, prefix, cursor string, limit int) ([]*metadata.ErasureFileInfo, string, error) {
	limit = clampUILimit(limit)
	infos, err := s.listErasureProfiles(ctx, prefix, cursor, limit+1)
	if nil != err {
		return nil, "", err
	}

	next := ""
	if limit < len(infos) {
		infos = infos[:limit]
		next = infos[limit-1].FilePath
	}
	if 0 == len(infos) {
		return infos, next, nil
	}

	if err := s.attachErasureShards(ctx, infos); nil != err {
		return nil, "", err
	}
	return infos, next, nil
}

func (s *SQLiteStore) listErasureProfiles(ctx context.Context, prefix, cursor string, limit int) ([]*metadata.ErasureFileInfo, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT file_path, data_shards, parity_shards, shard_size, original_size
		 FROM erasure_profiles
		 WHERE substr(file_path, 1, length(?)) = ? AND file_path > ?
		 ORDER BY file_path
		 LIMIT ?`, prefix, prefix, cursor, limit,
	)
	if nil != err {
		return nil, fmt.Errorf("failed to list erasure profiles: %w", err)
	}
	defer rows.Close()

	infos := make([]*metadata.ErasureFileInfo, 0, limit)
	for rows.Next() {
		var info metadata.ErasureFileInfo
		if scanErr := rows.Scan(&info.FilePath, &info.DataShards, &info.ParityShards, &info.ShardSize, &info.OriginalSize); nil != scanErr {
			return nil, fmt.Errorf("failed to scan erasure profile: %w", scanErr)
		}
		infos = append(infos, &info)
	}
	if err := rows.Err(); nil != err {
		return nil, fmt.Errorf("failed to iterate erasure profiles: %w", err)
	}
	return infos, nil
}

func (s *SQLiteStore) attachErasureShards(ctx context.Context, infos []*metadata.ErasureFileInfo) error {
	byPath := make(map[string]*metadata.ErasureFileInfo, len(infos))
	for _, info := range infos {
		byPath[info.FilePath] = info
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT file_path, shard_index, instance_id, backend_type, shard_path, shard_size, checksum
		 FROM erasure_shards
		 WHERE file_path >= ? AND file_path <= ?
		 ORDER BY file_path, shard_index`, infos[0].FilePath, infos[len(infos)-1].FilePath,
	)
	if nil != err {
		return fmt.Errorf("failed to query erasure shards: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var filePath string
		var si metadata.ErasureShardInfo
		if scanErr := rows.Scan(&filePath, &si.Index, &si.InstanceID, &si.BackendType, &si.Path, &si.Size, &si.Checksum); nil != scanErr {
			return fmt.Errorf("failed to scan erasure shard: %w", scanErr)
		}
		if info, ok := byPath[filePath]; ok {
			info.Shards = append(info.Shards, si)
		}
	}
	if err := rows.Err(); nil != err {
		return fmt.Errorf("failed to iterate erasure shards: %w", err)
	}
	return nil
}
