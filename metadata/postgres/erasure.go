package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"

	"github.com/ebogdum/callfs/metadata"
)

func (s *PostgresStore) CreateErasureInfo(ctx context.Context, filePath string, info *metadata.ErasureFileInfo) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := createErasureInfo(ctx, tx, filePath, info); err != nil {
		return err
	}

	return tx.Commit()
}

func createErasureInfo(ctx context.Context, tx dbQuerier, filePath string, info *metadata.ErasureFileInfo) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO erasure_profiles (file_path, data_shards, parity_shards, shard_size, original_size)
		 VALUES ($1, $2, $3, $4, $5)`,
		filePath, info.DataShards, info.ParityShards, info.ShardSize, info.OriginalSize,
	)
	if err != nil {
		return fmt.Errorf("failed to insert erasure profile: %w", err)
	}

	count := len(info.Shards)
	indexes := make([]int64, count)
	instanceIDs := make([]string, count)
	backendTypes := make([]string, count)
	shardPaths := make([]string, count)
	sizes := make([]int64, count)
	checksums := make([]string, count)
	for i, shard := range info.Shards {
		indexes[i] = int64(shard.Index)
		instanceIDs[i] = shard.InstanceID
		backendTypes[i] = shard.BackendType
		shardPaths[i] = shard.Path
		sizes[i] = shard.Size
		checksums[i] = shard.Checksum
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO erasure_shards (file_path, shard_index, instance_id, backend_type, shard_path, shard_size, checksum)
		 SELECT $1, * FROM unnest($2::int[], $3::text[], $4::text[], $5::text[], $6::bigint[], $7::text[])`,
		filePath, pq.Array(indexes), pq.Array(instanceIDs), pq.Array(backendTypes), pq.Array(shardPaths), pq.Array(sizes), pq.Array(checksums),
	)
	if err != nil {
		return fmt.Errorf("failed to insert erasure shards: %w", err)
	}

	return nil
}

func (s *PostgresStore) GetErasureInfo(ctx context.Context, filePath string) (*metadata.ErasureFileInfo, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.file_path, p.data_shards, p.parity_shards, p.shard_size, p.original_size,
		        s.shard_index, s.instance_id, s.backend_type, s.shard_path, s.shard_size, s.checksum
		 FROM erasure_profiles p
		 LEFT JOIN erasure_shards s ON s.file_path = p.file_path
		 WHERE p.file_path = $1
		 ORDER BY s.shard_index`, filePath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get erasure info: %w", err)
	}
	defer rows.Close()

	var info metadata.ErasureFileInfo
	found := false
	for rows.Next() {
		var shardIndex, shardSize sql.NullInt64
		var instanceID, backendType, shardPath, checksum sql.NullString
		if err := rows.Scan(
			&info.FilePath, &info.DataShards, &info.ParityShards, &info.ShardSize, &info.OriginalSize,
			&shardIndex, &instanceID, &backendType, &shardPath, &shardSize, &checksum,
		); err != nil {
			return nil, fmt.Errorf("failed to scan erasure info: %w", err)
		}
		found = true
		if shardIndex.Valid {
			info.Shards = append(info.Shards, metadata.ErasureShardInfo{
				Index:       int(shardIndex.Int64),
				InstanceID:  instanceID.String,
				BackendType: backendType.String,
				Path:        shardPath.String,
				Size:        shardSize.Int64,
				Checksum:    checksum.String,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate erasure info: %w", err)
	}
	if !found {
		return nil, metadata.ErrNotFound
	}

	return &info, nil
}

func (s *PostgresStore) DeleteErasureInfo(ctx context.Context, filePath string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := deleteErasureInfo(ctx, tx, filePath); err != nil {
		return err
	}

	return tx.Commit()
}

func deleteErasureInfo(ctx context.Context, tx dbQuerier, filePath string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM erasure_shards WHERE file_path = $1`, filePath); err != nil {
		return fmt.Errorf("failed to delete erasure shards: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM erasure_profiles WHERE file_path = $1`, filePath); err != nil {
		return fmt.Errorf("failed to delete erasure profile: %w", err)
	}

	return nil
}
