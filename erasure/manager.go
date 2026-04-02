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
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/backends"
	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/metadata"
)

// Manager orchestrates erasure coding: encoding, shard distribution, retrieval, and deletion.
type Manager struct {
	codec         *Codec
	placement     PlacementStrategy
	erasureStore  metadata.ErasureMetadataStore
	localBackend  backends.Storage
	config        *config.ErasureConfig
	instanceID    string
	selfEndpoint  string
	peerEndpoints map[string]string
	internalToken string
	httpClient    *http.Client
	logger        *zap.Logger
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
	// Derive selfEndpoint from peerEndpoints (includes self when populated in cmd/main.go)
	selfEndpoint := peerEndpoints[instanceID]

	// Warn if any peer endpoint uses unencrypted HTTP (internal secret would be sent in plaintext)
	for id, ep := range peerEndpoints {
		if strings.HasPrefix(ep, "http://") {
			logger.Warn("Peer endpoint uses unencrypted HTTP - internal token will be sent in plaintext",
				zap.String("peer_id", id),
				zap.String("endpoint", ep))
		}
	}

	return &Manager{
		codec:         NewCodec(),
		placement:     &RoundRobinPlacement{},
		erasureStore:  erasureStore,
		localBackend:  localBackend,
		config:        cfg,
		instanceID:    instanceID,
		selfEndpoint:  selfEndpoint,
		peerEndpoints: peerEndpoints,
		internalToken: internalToken,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		logger:        logger,
	}
}

// StoreFile erasure-encodes data and distributes shards across instances.
func (m *Manager) StoreFile(ctx context.Context, path string, data []byte, originalSize int64, opts *StoreOptions) (*ErasureFileInfo, error) {
	// Validate originalSize matches actual data length
	if originalSize != int64(len(data)) {
		return nil, fmt.Errorf("originalSize %d does not match data length %d", originalSize, len(data))
	}

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
				writeErr = m.storeRemoteShard(ctx, instanceForShard, hashPrefix, idx, shards[idx])
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
		// Cleanup orphaned shards — use a background context since the request ctx may already be cancelled
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		var cleanupWg sync.WaitGroup
		for i := 0; i < totalShards; i++ {
			if shardInfos[i].Path == "" {
				continue
			}
			cleanupWg.Add(1)
			go func(si ShardInfo) {
				defer cleanupWg.Done()
				if si.InstanceID == m.instanceID {
					_ = m.localBackend.Delete(cleanupCtx, si.Path)
				} else {
					shardPrefix := extractShardPrefix(si.Path)
					_ = m.deleteRemoteShard(cleanupCtx, si.InstanceID, shardPrefix, si.Index)
				}
			}(shardInfos[i])
		}
		go func() {
			cleanupWg.Wait()
			cleanupCancel()
		}()
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
		// Clean up all successfully-written shards since metadata write failed
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		var cleanupWg sync.WaitGroup
		for i := 0; i < totalShards; i++ {
			if shardInfos[i].Path == "" {
				continue
			}
			cleanupWg.Add(1)
			go func(si ShardInfo) {
				defer cleanupWg.Done()
				if si.InstanceID == m.instanceID {
					_ = m.localBackend.Delete(cleanupCtx, si.Path)
				} else {
					shardPrefix := extractShardPrefix(si.Path)
					_ = m.deleteRemoteShard(cleanupCtx, si.InstanceID, shardPrefix, si.Index)
				}
			}(shardInfos[i])
		}
		go func() {
			cleanupWg.Wait()
			cleanupCancel()
		}()
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

	// Use a cancellable context so we can abort remaining fetches once we have enough shards
	fetchCtx, fetchCancel := context.WithCancel(ctx)
	defer fetchCancel()

	for _, si := range mdInfo.Shards {
		wg.Add(1)
		go func(si metadata.ErasureShardInfo) {
			defer wg.Done()

			var data []byte
			var fetchErr error

			if si.InstanceID == m.instanceID {
				data, fetchErr = m.readLocalShard(fetchCtx, si.Path)
			} else {
				shardPrefix := extractShardPrefix(si.Path)
				data, fetchErr = m.fetchRemoteShard(fetchCtx, si.InstanceID, shardPrefix, si.Index)
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

			if ShardChecksum(data) != si.Checksum {
				m.logger.Warn("Shard checksum mismatch",
					zap.Int("index", si.Index),
					zap.String("instance", si.InstanceID))
				return
			}

			shards[si.Index] = data
			successCount++

			// Short-circuit: cancel remaining fetches once we have enough shards
			if successCount >= mdInfo.DataShards {
				fetchCancel()
			}
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
		var endpoint string
		if si.InstanceID == m.instanceID {
			endpoint = m.selfEndpoint
		} else {
			endpoint = m.peerEndpoints[si.InstanceID]
		}

		if endpoint == "" {
			return nil, fmt.Errorf("no endpoint found for instance %s (shard %d)", si.InstanceID, si.Index)
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

// GetShard reads a single local shard by path and index, verifying checksum before returning.
func (m *Manager) GetShard(ctx context.Context, path string, index int) ([]byte, error) {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get erasure info: %w", err)
	}

	for _, si := range mdInfo.Shards {
		if si.Index == index && si.InstanceID == m.instanceID {
			data, readErr := m.readLocalShard(ctx, si.Path)
			if readErr != nil {
				return nil, readErr
			}
			if ShardChecksum(data) != si.Checksum {
				return nil, fmt.Errorf("shard %d checksum mismatch: data corrupted on disk", index)
			}
			return data, nil
		}
	}

	return nil, ErrShardNotFound
}

// DeleteFile removes erasure metadata first, then best-effort deletes all shards.
// Metadata-first ordering ensures a crash leaves orphan shards (reclaimable) rather
// than orphan metadata pointing to partially-deleted shards (irrecoverable).
func (m *Manager) DeleteFile(ctx context.Context, path string) error {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to get erasure info for delete: %w", err)
	}

	// Delete metadata first — makes the file logically gone
	if err := m.erasureStore.DeleteErasureInfo(ctx, path); err != nil {
		return fmt.Errorf("failed to delete erasure metadata: %w", err)
	}

	// Best-effort shard cleanup
	var wg sync.WaitGroup
	for _, si := range mdInfo.Shards {
		wg.Add(1)
		go func(si metadata.ErasureShardInfo) {
			defer wg.Done()
			if si.InstanceID == m.instanceID {
				if delErr := m.localBackend.Delete(ctx, si.Path); delErr != nil {
					m.logger.Warn("Failed to delete local shard",
						zap.Int("index", si.Index),
						zap.Error(delErr))
				}
			} else {
				shardPrefix := extractShardPrefix(si.Path)
				if delErr := m.deleteRemoteShard(ctx, si.InstanceID, shardPrefix, si.Index); delErr != nil {
					m.logger.Warn("Failed to delete remote shard",
						zap.Int("index", si.Index),
						zap.String("instance", si.InstanceID),
						zap.Error(delErr))
				}
			}
		}(si)
	}
	wg.Wait()

	return nil
}

// extractShardPrefix extracts the hash prefix from a shard path like ".erasure/<prefix>/<idx>".
func extractShardPrefix(shardPath string) string {
	trimmed := strings.TrimPrefix(shardPath, ".erasure/")
	lastSlash := strings.LastIndex(trimmed, "/")
	if lastSlash < 0 {
		return trimmed
	}
	return trimmed[:lastSlash]
}

func (m *Manager) readLocalShard(ctx context.Context, shardPath string) ([]byte, error) {
	reader, err := m.localBackend.Open(ctx, shardPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (m *Manager) storeRemoteShard(ctx context.Context, instanceID, hashPrefix string, index int, data []byte) error {
	endpoint, ok := m.peerEndpoints[instanceID]
	if !ok {
		return fmt.Errorf("no endpoint for instance %s", instanceID)
	}

	url := strings.TrimRight(endpoint, "/") + fmt.Sprintf("/v1/internal/shards/%s/%d", hashPrefix, index)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.internalToken)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(data))

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("remote shard store failed with status %d", resp.StatusCode)
	}
	return nil
}

func (m *Manager) fetchRemoteShard(ctx context.Context, instanceID, shardPrefix string, index int) ([]byte, error) {
	endpoint, ok := m.peerEndpoints[instanceID]
	if !ok {
		return nil, fmt.Errorf("no endpoint for instance %s", instanceID)
	}

	url := strings.TrimRight(endpoint, "/") + fmt.Sprintf("/v1/internal/shards/%s/%d", shardPrefix, index)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.internalToken)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote shard fetch failed with status %d", resp.StatusCode)
	}

	const maxShardReadBytes = 256 << 20
	return io.ReadAll(io.LimitReader(resp.Body, maxShardReadBytes))
}

func (m *Manager) deleteRemoteShard(ctx context.Context, instanceID, shardPrefix string, index int) error {
	endpoint, ok := m.peerEndpoints[instanceID]
	if !ok {
		return fmt.Errorf("no endpoint for instance %s", instanceID)
	}

	url := strings.TrimRight(endpoint, "/") + fmt.Sprintf("/v1/internal/shards/%s/%d", shardPrefix, index)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.internalToken)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("remote shard delete failed with status %d", resp.StatusCode)
	}
	return nil
}
