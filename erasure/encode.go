package erasure

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"sync"

	"github.com/klauspost/reedsolomon"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

const maxShardsPerRequest = 256

func (m *Manager) StoreFile(ctx context.Context, path string, data []byte, originalSize int64, opts *StoreOptions) (*ErasureFileInfo, error) {
	if originalSize != int64(len(data)) {
		return nil, fmt.Errorf("originalSize %d does not match data length %d", originalSize, len(data))
	}
	return m.encodeAndStore(ctx, path, bytes.NewReader(data), originalSize, opts)
}

func (m *Manager) StoreReader(ctx context.Context, path string, r io.Reader, opts *StoreOptions) (*ErasureFileInfo, error) {
	spool, err := os.CreateTemp("", "callfs-erasure-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create erasure spool file: %w", err)
	}
	defer m.removeSpool(spool)

	buf := getBlock()
	defer putBlock(buf)
	size, err := io.CopyBuffer(spool, r, *buf)
	if err != nil {
		return nil, fmt.Errorf("failed to spool erasure upload: %w", err)
	}
	return m.encodeAndStore(ctx, path, spool, size, opts)
}

func (m *Manager) removeSpool(f *os.File) {
	if err := f.Close(); err != nil {
		m.logger.Warn("Failed to close erasure spool file", zap.String("spool", f.Name()), zap.Error(err))
	}
	if err := os.Remove(f.Name()); err != nil {
		m.logger.Warn("Failed to remove erasure spool file", zap.String("spool", f.Name()), zap.Error(err))
	}
}

func (m *Manager) encodeAndStore(ctx context.Context, path string, src io.ReaderAt, size int64, opts *StoreOptions) (*ErasureFileInfo, error) {
	dataShards, parityShards := m.resolveShardCounts(opts)
	totalShards := dataShards + parityShards
	if totalShards > maxShardsPerRequest {
		return nil, fmt.Errorf("erasure encode failed: %d total shards exceeds limit %d: %w", totalShards, maxShardsPerRequest, ErrInvalidProfile)
	}
	if size <= 0 {
		return nil, fmt.Errorf("erasure encode failed: %w", ErrEmptyUpload)
	}
	enc, err := m.codec.encoder(dataShards, parityShards)
	if err != nil {
		return nil, fmt.Errorf("erasure encode failed: erasure: failed to create encoder: %w", err)
	}

	profile := ErasureProfile{
		DataShards:   dataShards,
		ParityShards: parityShards,
		ShardSize:    (size + int64(dataShards) - 1) / int64(dataShards),
	}
	assignments := m.placement.AssignShards(totalShards, m.instanceID, m.resolveInstances(opts))

	shardPrefix, err := newShardPrefix()
	if err != nil {
		return nil, err
	}
	paths := make([]string, totalShards)
	for idx := range paths {
		paths[idx] = fmt.Sprintf(".erasure/%s/%d", shardPrefix, idx)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	writers := make([]*io.PipeWriter, totalShards)
	hashes := make([]hash.Hash, totalShards)
	stored := make([]bool, totalShards)
	var storeErr error
	var mu sync.Mutex
	var wg sync.WaitGroup
	for idx := range totalShards {
		pr, pw := io.Pipe()
		writers[idx] = pw
		hashes[idx] = sha256.New()
		wg.Add(1)
		go func() {
			defer wg.Done()
			var writeErr error
			if assignments[idx] == m.instanceID {
				writeErr = m.localBackend.Create(ctx, paths[idx], pr, profile.ShardSize)
			} else {
				writeErr = m.storeRemoteShard(ctx, assignments[idx], shardPrefix, idx, pr, profile.ShardSize)
			}
			_ = pr.CloseWithError(writeErr)

			mu.Lock()
			defer mu.Unlock()
			if writeErr != nil {
				if storeErr == nil {
					storeErr = fmt.Errorf("failed to store shard %d on %s: %w", idx, assignments[idx], writeErr)
					cancel()
				}
				return
			}
			stored[idx] = true
		}()
	}

	encodeErr := encodeBlocks(ctx, src, size, enc, profile, writers, hashes)
	for _, pw := range writers {
		_ = pw.CloseWithError(encodeErr)
	}
	wg.Wait()

	shardInfos := make([]ShardInfo, totalShards)
	for idx := range shardInfos {
		if stored[idx] {
			shardInfos[idx] = ShardInfo{
				Index:       idx,
				InstanceID:  assignments[idx],
				BackendType: m.config.ShardBackend,
				Path:        paths[idx],
				Size:        profile.ShardSize,
				Checksum:    hex.EncodeToString(hashes[idx].Sum(nil)),
			}
		}
	}

	if storeErr != nil {
		m.cleanupShards(shardInfos)
		return nil, storeErr
	}
	if encodeErr != nil {
		m.cleanupShards(shardInfos)
		return nil, fmt.Errorf("erasure encode failed: %w", encodeErr)
	}

	m.logger.Info("Erasure-coded file stored",
		zap.String("path", path),
		zap.Int("data_shards", dataShards),
		zap.Int("parity_shards", parityShards),
		zap.Int64("original_size", size))

	return &ErasureFileInfo{
		FilePath:     path,
		OriginalSize: size,
		Profile:      profile,
		Shards:       shardInfos,
	}, nil
}

func (m *Manager) ErasureMetadata(info *ErasureFileInfo) *metadata.ErasureFileInfo {
	return &metadata.ErasureFileInfo{
		FilePath:     info.FilePath,
		OriginalSize: info.OriginalSize,
		DataShards:   info.Profile.DataShards,
		ParityShards: info.Profile.ParityShards,
		ShardSize:    info.Profile.ShardSize,
		Shards:       shardInfosToMetadata(info.Shards),
	}
}

func (m *Manager) CommitErasureInfo(ctx context.Context, info *ErasureFileInfo) error {
	mdInfo := m.ErasureMetadata(info)
	if err := m.erasureStore.CreateErasureInfo(ctx, info.FilePath, mdInfo); err != nil {
		m.cleanupShards(info.Shards)
		return fmt.Errorf("failed to store erasure metadata: %w", err)
	}
	return nil
}

func (m *Manager) DiscardFile(info *ErasureFileInfo) {
	m.cleanupShards(info.Shards)
}

func encodeBlocks(ctx context.Context, src io.ReaderAt, size int64, enc reedsolomon.Encoder, profile ErasureProfile, writers []*io.PipeWriter, hashes []hash.Hash) error {
	dataShards := profile.DataShards
	totalShards := len(writers)
	bufs := make([]*[]byte, totalShards)
	for j := range bufs {
		bufs[j] = getBlock()
		defer putBlock(bufs[j])
	}

	shards := make([][]byte, totalShards)
	for pos := int64(0); pos < profile.ShardSize; {
		if err := ctx.Err(); err != nil {
			return err
		}
		n := min(int64(blockSize), profile.ShardSize-pos)
		for i := 0; i < dataShards; i++ {
			shards[i] = (*bufs[i])[:n]
			if err := readSourceBlock(src, size, int64(i)*profile.ShardSize+pos, shards[i]); err != nil {
				return err
			}
		}
		for j := dataShards; j < totalShards; j++ {
			shards[j] = (*bufs[j])[:n]
		}
		if err := enc.Encode(shards); err != nil {
			return fmt.Errorf("erasure: failed to encode parity: %w", err)
		}
		for j, shard := range shards {
			if _, err := hashes[j].Write(shard); err != nil {
				return err
			}
			if _, err := writers[j].Write(shard); err != nil {
				return err
			}
		}
		pos += n
	}
	return nil
}

func readSourceBlock(src io.ReaderAt, size, fileOff int64, dst []byte) error {
	valid := min(max(size-fileOff, 0), int64(len(dst)))
	if valid > 0 {
		if _, err := src.ReadAt(dst[:valid], fileOff); err != nil {
			return fmt.Errorf("failed to read erasure source: %w", err)
		}
	}
	clear(dst[valid:])
	return nil
}
