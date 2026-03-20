package erasure

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/backends"
	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/metadata"
)

// Manager orchestrates erasure coding: encoding, shard distribution, retrieval, and deletion.
type Manager struct {
	codec        *Codec
	placement    PlacementStrategy
	erasureStore metadata.ErasureMetadataStore
	localBackend backends.Storage
	config       *config.ErasureConfig
	instanceID   string
	peerEndpoints map[string]string
	internalToken string
	logger       *zap.Logger
}

// NewManager creates a new erasure Manager.
func NewManager(
	erasureStore metadata.ErasureMetadataStore,
	localBackend backends.Storage,
	cfg *config.ErasureConfig,
	instanceID string,
	peerEndpoints map[string]string,
	internalToken string,
	logger *zap.Logger,
) *Manager {
	return &Manager{
		codec:         NewCodec(),
		placement:     &RoundRobinPlacement{},
		erasureStore:  erasureStore,
		localBackend:  localBackend,
		config:        cfg,
		instanceID:    instanceID,
		peerEndpoints: peerEndpoints,
		internalToken: internalToken,
		logger:        logger,
	}
}

// StoreFile erasure-encodes data and distributes shards across instances.
func (m *Manager) StoreFile(ctx context.Context, path string, data []byte, originalSize int64, opts *StoreOptions) (*ErasureFileInfo, error) {
	dataShards := m.config.DataShards
	parityShards := m.config.ParityShards
	if dataShards <= 0 {
		dataShards = 4
	}
	if parityShards <= 0 {
		parityShards = 2
	}

	if opts != nil {
		if opts.DataShards > 0 {
			dataShards = opts.DataShards
		}
		if opts.ParityShards > 0 {
			parityShards = opts.ParityShards
		}
	}

	profile := ErasureProfile{
		DataShards:   dataShards,
		ParityShards: parityShards,
	}

	shards, err := m.codec.Encode(data, profile)
	if err != nil {
		return nil, fmt.Errorf("erasure encode failed: %w", err)
	}

	if len(shards) > 0 {
		profile.ShardSize = int64(len(shards[0]))
	}

	totalShards := dataShards + parityShards

	// Determine available instances for placement
	var availableInstances []string
	if opts != nil && len(opts.Instances) > 0 {
		availableInstances = opts.Instances
	} else {
		availableInstances = make([]string, 0, len(m.peerEndpoints)+1)
		availableInstances = append(availableInstances, m.instanceID)
		for id := range m.peerEndpoints {
			if id != m.instanceID {
				availableInstances = append(availableInstances, id)
			}
		}
	}

	assignments := m.placement.AssignShards(totalShards, m.instanceID, availableInstances)

	// Compute shard hash for storage path
	fileHash := sha256.Sum256(data)
	hashPrefix := hex.EncodeToString(fileHash[:8])

	shardInfos := make([]ShardInfo, totalShards)
	var storeErr error
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < totalShards; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			shardPath := fmt.Sprintf(".erasure/%s/%d", hashPrefix, idx)
			checksum := ShardChecksum(shards[idx])
			instanceForShard := assignments[idx]

			var writeErr error
			if instanceForShard == m.instanceID {
				writeErr = m.localBackend.Create(ctx, shardPath, bytes.NewReader(shards[idx]), int64(len(shards[idx])))
			} else {
				writeErr = m.storeRemoteShard(ctx, instanceForShard, path, idx, shards[idx])
			}

			mu.Lock()
			defer mu.Unlock()
			if writeErr != nil {
				if storeErr == nil {
					storeErr = fmt.Errorf("failed to store shard %d on %s: %w", idx, instanceForShard, writeErr)
				}
				return
			}

			shardInfos[idx] = ShardInfo{
				Index:       idx,
				InstanceID:  instanceForShard,
				BackendType: m.config.ShardBackend,
				Path:        shardPath,
				Size:        int64(len(shards[idx])),
				Checksum:    checksum,
			}
		}(i)
	}
	wg.Wait()

	if storeErr != nil {
		return nil, storeErr
	}

	info := &ErasureFileInfo{
		FilePath:     path,
		OriginalSize: originalSize,
		Profile:      profile,
		Shards:       shardInfos,
	}

	// Convert to metadata type and persist
	mdInfo := &metadata.ErasureFileInfo{
		FilePath:     path,
		OriginalSize: originalSize,
		DataShards:   profile.DataShards,
		ParityShards: profile.ParityShards,
		ShardSize:    profile.ShardSize,
	}
	mdShards := make([]metadata.ErasureShardInfo, len(shardInfos))
	for i, si := range shardInfos {
		mdShards[i] = metadata.ErasureShardInfo{
			Index:       si.Index,
			InstanceID:  si.InstanceID,
			BackendType: si.BackendType,
			Path:        si.Path,
			Size:        si.Size,
			Checksum:    si.Checksum,
		}
	}
	mdInfo.Shards = mdShards

	if err := m.erasureStore.CreateErasureInfo(ctx, path, mdInfo); err != nil {
		return nil, fmt.Errorf("failed to store erasure metadata: %w", err)
	}

	m.logger.Info("Erasure-coded file stored",
		zap.String("path", path),
		zap.Int("data_shards", dataShards),
		zap.Int("parity_shards", parityShards),
		zap.Int64("original_size", originalSize))

	return info, nil
}

// RetrieveFile fetches shards in parallel and reconstructs the original file.
func (m *Manager) RetrieveFile(ctx context.Context, path string) ([]byte, error) {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get erasure info: %w", err)
	}

	totalShards := mdInfo.DataShards + mdInfo.ParityShards
	shards := make([][]byte, totalShards)
	var mu sync.Mutex
	var wg sync.WaitGroup
	successCount := 0

	for _, si := range mdInfo.Shards {
		wg.Add(1)
		go func(si metadata.ErasureShardInfo) {
			defer wg.Done()

			var data []byte
			var fetchErr error

			if si.InstanceID == m.instanceID {
				data, fetchErr = m.readLocalShard(ctx, si.Path)
			} else {
				data, fetchErr = m.fetchRemoteShard(ctx, si.InstanceID, path, si.Index)
			}

			mu.Lock()
			defer mu.Unlock()
			if fetchErr != nil {
				m.logger.Warn("Failed to fetch shard",
					zap.Int("index", si.Index),
					zap.String("instance", si.InstanceID),
					zap.Error(fetchErr))
				return
			}

			// Verify checksum
			if ShardChecksum(data) != si.Checksum {
				m.logger.Warn("Shard checksum mismatch",
					zap.Int("index", si.Index),
					zap.String("instance", si.InstanceID))
				return
			}

			shards[si.Index] = data
			successCount++
		}(si)
	}
	wg.Wait()

	if successCount < mdInfo.DataShards {
		return nil, ErrInsufficientShards
	}

	profile := ErasureProfile{
		DataShards:   mdInfo.DataShards,
		ParityShards: mdInfo.ParityShards,
		ShardSize:    mdInfo.ShardSize,
	}

	return m.codec.Decode(shards, profile, mdInfo.OriginalSize)
}

// GetManifest builds a ChunkManifest with direct endpoints for each shard.
func (m *Manager) GetManifest(ctx context.Context, path string) (*ChunkManifest, error) {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get erasure info: %w", err)
	}

	manifest := &ChunkManifest{
		Path:         path,
		OriginalSize: mdInfo.OriginalSize,
		ErasureProfile: ErasureProfile{
			DataShards:   mdInfo.DataShards,
			ParityShards: mdInfo.ParityShards,
			ShardSize:    mdInfo.ShardSize,
		},
	}

	shardEndpoints := make([]ShardEndpoint, 0, len(mdInfo.Shards))
	for _, si := range mdInfo.Shards {
		endpoint := m.peerEndpoints[si.InstanceID]
		if si.InstanceID == m.instanceID {
			// Use own external endpoint
			for id, ep := range m.peerEndpoints {
				if id == m.instanceID {
					endpoint = ep
					break
				}
			}
		}

		shardEndpoints = append(shardEndpoints, ShardEndpoint{
			Index:    si.Index,
			Endpoint: strings.TrimRight(endpoint, "/") + fmt.Sprintf("/v1/shards/%s/%d", strings.TrimPrefix(path, "/"), si.Index),
			Size:     si.Size,
			Checksum: si.Checksum,
		})
	}
	manifest.Shards = shardEndpoints

	return manifest, nil
}

// GetShard reads a single local shard by path and index.
func (m *Manager) GetShard(ctx context.Context, path string, index int) ([]byte, error) {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get erasure info: %w", err)
	}

	for _, si := range mdInfo.Shards {
		if si.Index == index && si.InstanceID == m.instanceID {
			return m.readLocalShard(ctx, si.Path)
		}
	}

	return nil, ErrShardNotFound
}

// DeleteFile removes all shards (local + remote) and erasure metadata.
func (m *Manager) DeleteFile(ctx context.Context, path string) error {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to get erasure info for delete: %w", err)
	}

	var wg sync.WaitGroup
	for _, si := range mdInfo.Shards {
		wg.Add(1)
		go func(si metadata.ErasureShardInfo) {
			defer wg.Done()
			if si.InstanceID == m.instanceID {
				if err := m.localBackend.Delete(ctx, si.Path); err != nil {
					m.logger.Warn("Failed to delete local shard",
						zap.Int("index", si.Index),
						zap.Error(err))
				}
			} else {
				if err := m.deleteRemoteShard(ctx, si.InstanceID, path, si.Index); err != nil {
					m.logger.Warn("Failed to delete remote shard",
						zap.Int("index", si.Index),
						zap.String("instance", si.InstanceID),
						zap.Error(err))
				}
			}
		}(si)
	}
	wg.Wait()

	return m.erasureStore.DeleteErasureInfo(ctx, path)
}

func (m *Manager) readLocalShard(ctx context.Context, shardPath string) ([]byte, error) {
	reader, err := m.localBackend.Open(ctx, shardPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (m *Manager) storeRemoteShard(ctx context.Context, instanceID, filePath string, index int, data []byte) error {
	endpoint, ok := m.peerEndpoints[instanceID]
	if !ok {
		return fmt.Errorf("no endpoint for instance %s", instanceID)
	}

	url := strings.TrimRight(endpoint, "/") + fmt.Sprintf("/v1/internal/shards/%s/%d", strings.TrimPrefix(filePath, "/"), index)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.internalToken)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(data))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("remote shard store failed with status %d", resp.StatusCode)
	}
	return nil
}

func (m *Manager) fetchRemoteShard(ctx context.Context, instanceID, filePath string, index int) ([]byte, error) {
	endpoint, ok := m.peerEndpoints[instanceID]
	if !ok {
		return nil, fmt.Errorf("no endpoint for instance %s", instanceID)
	}

	url := strings.TrimRight(endpoint, "/") + fmt.Sprintf("/v1/internal/shards/%s/%d", strings.TrimPrefix(filePath, "/"), index)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.internalToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote shard fetch failed with status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (m *Manager) deleteRemoteShard(ctx context.Context, instanceID, filePath string, index int) error {
	endpoint, ok := m.peerEndpoints[instanceID]
	if !ok {
		return fmt.Errorf("no endpoint for instance %s", instanceID)
	}

	url := strings.TrimRight(endpoint, "/") + fmt.Sprintf("/v1/internal/shards/%s/%d", strings.TrimPrefix(filePath, "/"), index)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.internalToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("remote shard delete failed with status %d", resp.StatusCode)
	}
	return nil
}
