package erasure

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/backends"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/internal/peertls"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/metrics"
)

const (
	maxConcurrentDecodes = 32
	maxShardReadBytes    = 256 << 20
	maxIdleConnsPerPeer  = 64
	remoteRequestTimeout = 30 * time.Second
	maxConcurrentRepairs = 4
	repairTimeout        = 10 * time.Minute
	legacyShardPath      = ".erasure"
)

func ValidateShardPath(raw string) (string, error) {
	if "" == raw {
		return "", errors.New("erasure shard path must not be empty")
	}
	if strings.HasPrefix(raw, "/") || strings.Contains(raw, "\\") {
		return "", fmt.Errorf("erasure shard path %q must be relative", raw)
	}
	for _, segment := range strings.Split(raw, "/") {
		if ".." == segment {
			return "", fmt.Errorf("erasure shard path %q must not contain ..", raw)
		}
	}
	if cleaned := path.Clean(raw); cleaned != raw || "." == cleaned {
		return "", fmt.Errorf("erasure shard path %q must be clean", raw)
	}
	return raw, nil
}

type Manager struct {
	codec         *Codec
	placement     PlacementStrategy
	erasureStore  metadata.ErasureMetadataStore
	localBackend  backends.Storage
	config        *config.ErasureConfig
	instanceID    string
	selfEndpoint  string
	peerMu        sync.RWMutex
	peerEndpoints map[string]string
	internalToken string
	httpClient    *http.Client
	logger        *zap.Logger
	decodeSlots   chan struct{}
	repairSlots   chan struct{}
	healthMu      sync.RWMutex
	healthCache   map[string]FileHealth
}

func NewManager(
	erasureStore metadata.ErasureMetadataStore,
	localBackend backends.Storage,
	cfg *config.ErasureConfig,
	instanceID string,
	peerEndpoints map[string]string,
	internalToken string,
	logger *zap.Logger,
) *Manager {
	selfEndpoint := peerEndpoints[instanceID]

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = maxIdleConnsPerPeer * max(len(peerEndpoints), 1)
	transport.MaxIdleConnsPerHost = maxIdleConnsPerPeer
	transport.ResponseHeaderTimeout = remoteRequestTimeout
	transport.TLSClientConfig = peertls.ClientConfig()

	peers := make(map[string]string, len(peerEndpoints))
	maps.Copy(peers, peerEndpoints)

	return &Manager{
		codec:         NewCodec(),
		placement:     &RoundRobinPlacement{},
		erasureStore:  erasureStore,
		localBackend:  localBackend,
		config:        cfg,
		instanceID:    instanceID,
		selfEndpoint:  selfEndpoint,
		peerEndpoints: peers,
		internalToken: internalToken,
		httpClient:    &http.Client{Transport: transport},
		logger:        logger,
		decodeSlots:   make(chan struct{}, maxConcurrentDecodes),
		repairSlots:   make(chan struct{}, maxConcurrentRepairs),
		healthCache:   make(map[string]FileHealth),
	}
}

func (m *Manager) shardRoot() (string, error) {
	return ValidateShardPath(m.config.ShardPath)
}

func (m *Manager) AutoSplit(size int64) bool {
	threshold := m.config.MinFileSize.Int64()
	return 0 < threshold && 0 <= size && threshold <= size
}

func (m *Manager) peerEndpoint(instanceID string) (string, bool) {
	m.peerMu.RLock()
	defer m.peerMu.RUnlock()
	endpoint, ok := m.peerEndpoints[instanceID]
	return endpoint, ok
}

func (m *Manager) Close() {
	m.httpClient.CloseIdleConnections()
}

func (m *Manager) resolveShardCounts(opts *StoreOptions) (int, int) {
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
	return dataShards, parityShards
}

func (m *Manager) resolveInstances(opts *StoreOptions) []string {
	if opts != nil && len(opts.Instances) > 0 {
		return opts.Instances
	}
	m.peerMu.RLock()
	defer m.peerMu.RUnlock()
	instances := make([]string, 0, len(m.peerEndpoints)+1)
	instances = append(instances, m.instanceID)
	for id := range m.peerEndpoints {
		if id != m.instanceID {
			instances = append(instances, id)
		}
	}
	return instances
}

func (m *Manager) cleanupShards(shardInfos []ShardInfo) {
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cleanupCancel()
	var wg sync.WaitGroup
	for _, si := range shardInfos {
		if si.Path == "" {
			continue
		}
		wg.Add(1)
		go func(si ShardInfo) {
			defer wg.Done()
			if si.InstanceID == m.instanceID {
				_ = m.localBackend.Delete(cleanupCtx, si.Path)
			} else {
				shardPrefix := m.extractShardPrefix(si.Path)
				_ = m.deleteRemoteShard(cleanupCtx, si.InstanceID, shardPrefix, si.Index)
			}
		}(si)
	}
	wg.Wait()
}

func shardInfosToMetadata(shardInfos []ShardInfo) []metadata.ErasureShardInfo {
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
	return mdShards
}

func (m *Manager) RetrieveFile(ctx context.Context, path string) ([]byte, error) {
	rc, size, err := m.OpenReader(ctx, path)
	if err != nil {
		return nil, err
	}
	data, err := readAllSized(rc, size)
	if err == nil {
		return data, nil
	}
	if ctx.Err() != nil {
		return nil, err
	}
	m.logger.Warn("Streamed erasure read failed, retrying with verified reconstruction",
		zap.String("path", path),
		zap.Error(err))
	return m.retrieveVerified(ctx, path)
}

func (m *Manager) retrieveVerified(ctx context.Context, path string) ([]byte, error) {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get erasure info: %w", err)
	}

	data, damaged, err := m.reconstruct(ctx, mdInfo)
	if err != nil {
		return nil, err
	}

	if repairErr := m.repairShards(ctx, mdInfo, data, damaged); repairErr != nil {
		m.logger.Warn("Shard repair failed",
			zap.String("path", path),
			zap.Int("damaged_shards", len(damaged)),
			zap.Error(repairErr))
	}
	return data, nil
}

func (m *Manager) RetrieveRange(ctx context.Context, path string, offset, length int64) ([]byte, error) {
	rc, n, err := m.openRange(ctx, path, offset, length)
	if err != nil {
		return nil, err
	}
	data, err := readAllSized(rc, n)
	if err == nil {
		return data, nil
	}
	if ctx.Err() != nil {
		return nil, err
	}
	m.logger.Warn("Streamed erasure range read failed, retrying with verified reconstruction",
		zap.String("path", path),
		zap.Error(err))

	full, err := m.retrieveVerified(ctx, path)
	if err != nil {
		return nil, err
	}
	n, err = RangeLength(int64(len(full)), offset, length)
	if err != nil {
		return nil, err
	}
	return full[offset : offset+n], nil
}

func readAllSized(rc io.ReadCloser, size int64) ([]byte, error) {
	data := make([]byte, size)
	_, err := io.ReadFull(rc, data)
	if closeErr := rc.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (m *Manager) RepairFile(ctx context.Context, path string) error {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to get erasure info: %w", err)
	}

	data, damaged, err := m.reconstruct(ctx, mdInfo)
	if err != nil {
		return err
	}
	return m.repairShards(ctx, mdInfo, data, damaged)
}

func (m *Manager) runBackground(path, operation string, task func(ctx context.Context) error) {
	go func() {
		m.repairSlots <- struct{}{}
		defer func() { <-m.repairSlots }()

		ctx, cancel := context.WithTimeout(context.Background(), repairTimeout)
		defer cancel()
		m.logger.Info("Background erasure task started",
			zap.String("operation", operation),
			zap.String("path", path))
		if err := task(ctx); nil != err {
			m.logger.Warn("Background erasure task failed",
				zap.String("operation", operation),
				zap.String("path", path),
				zap.Error(err))
			return
		}
		m.logger.Info("Background erasure task completed",
			zap.String("operation", operation),
			zap.String("path", path))
	}()
}

func (m *Manager) verifyParity(ctx context.Context, path string) error {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, path)
	if nil != err {
		return fmt.Errorf("failed to get erasure info: %w", err)
	}

	dataShards := mdInfo.DataShards
	total := dataShards + mdInfo.ParityShards
	var damaged []metadata.ErasureShardInfo
	for _, si := range mdInfo.Shards {
		if si.Index < dataShards || si.Index >= total {
			continue
		}
		data, fetchErr := m.fetchShard(ctx, si)
		if nil == fetchErr && si.Checksum == ShardChecksum(data) {
			continue
		}
		m.logger.Warn("Parity shard damaged",
			zap.Int("index", si.Index),
			zap.String("instance", si.InstanceID),
			zap.Error(fetchErr))
		damaged = append(damaged, si)
	}
	if 0 == len(damaged) {
		return nil
	}

	data, moreDamaged, err := m.reconstruct(ctx, mdInfo)
	if nil != err {
		return err
	}
	return m.repairShards(ctx, mdInfo, data, append(damaged, moreDamaged...))
}

func (m *Manager) reconstruct(ctx context.Context, mdInfo *metadata.ErasureFileInfo) ([]byte, []metadata.ErasureShardInfo, error) {
	dataShards := mdInfo.DataShards
	totalShards := dataShards + mdInfo.ParityShards
	shards := make([][]byte, totalShards)
	var damaged []metadata.ErasureShardInfo
	successCount := 0
	var mu sync.Mutex

	fetchCtx, fetchCancel := context.WithCancel(ctx)
	defer fetchCancel()

	fetchGroup := func(group []metadata.ErasureShardInfo) {
		var wg sync.WaitGroup
		for _, si := range group {
			wg.Add(1)
			go func() {
				defer wg.Done()

				data, fetchErr := m.fetchShard(fetchCtx, si)
				checksumOK := fetchErr == nil && ShardChecksum(data) == si.Checksum

				mu.Lock()
				defer mu.Unlock()
				if fetchErr != nil {
					m.logger.Warn("Failed to fetch shard",
						zap.Int("index", si.Index),
						zap.String("instance", si.InstanceID),
						zap.Error(fetchErr))
					if !errors.Is(fetchErr, context.Canceled) {
						damaged = append(damaged, si)
					}
					return
				}
				if !checksumOK {
					m.logger.Warn("Shard checksum mismatch",
						zap.Int("index", si.Index),
						zap.String("instance", si.InstanceID))
					damaged = append(damaged, si)
					return
				}

				shards[si.Index] = data
				successCount++
				if successCount >= dataShards {
					fetchCancel()
				}
			}()
		}
		wg.Wait()
	}

	var dataGroup, parityGroup []metadata.ErasureShardInfo
	for _, si := range mdInfo.Shards {
		switch {
		case si.Index < 0 || si.Index >= totalShards:
			continue
		case si.Index < dataShards:
			dataGroup = append(dataGroup, si)
		default:
			parityGroup = append(parityGroup, si)
		}
	}
	fetchGroup(dataGroup)
	if successCount < dataShards {
		fetchGroup(parityGroup)
	}
	if successCount < dataShards {
		return nil, nil, ErrInsufficientShards
	}

	profile := ErasureProfile{
		DataShards:   mdInfo.DataShards,
		ParityShards: mdInfo.ParityShards,
		ShardSize:    mdInfo.ShardSize,
	}

	data, err := m.codec.Decode(shards, profile, mdInfo.OriginalSize)
	if err != nil {
		return nil, nil, err
	}
	return data, damaged, nil
}

func (m *Manager) repairShards(ctx context.Context, mdInfo *metadata.ErasureFileInfo, data []byte, damaged []metadata.ErasureShardInfo) error {
	if len(damaged) == 0 {
		return nil
	}

	profile := ErasureProfile{
		DataShards:   mdInfo.DataShards,
		ParityShards: mdInfo.ParityShards,
		ShardSize:    mdInfo.ShardSize,
	}
	encoded, err := m.codec.Encode(data, profile)
	if err != nil {
		return fmt.Errorf("failed to re-encode file for repair: %w", err)
	}

	var mu sync.Mutex
	var repairErrs []error
	var wg sync.WaitGroup
	for _, si := range damaged {
		wg.Add(1)
		go func(si metadata.ErasureShardInfo) {
			defer wg.Done()
			if writeErr := m.rewriteShard(ctx, si, encoded[si.Index]); nil != writeErr {
				mu.Lock()
				defer mu.Unlock()
				repairErrs = append(repairErrs, fmt.Errorf("failed to repair shard %d on %s: %w", si.Index, si.InstanceID, writeErr))
			} else {
				metrics.ErasurePiecesRepairedTotal.Inc()
			}
		}(si)
	}
	wg.Wait()

	return errors.Join(repairErrs...)
}

func (m *Manager) rewriteShard(ctx context.Context, si metadata.ErasureShardInfo, shard []byte) error {
	if ShardChecksum(shard) != si.Checksum {
		return fmt.Errorf("re-encoded shard %d does not match recorded checksum", si.Index)
	}
	if si.InstanceID == m.instanceID {
		return m.localBackend.Create(ctx, si.Path, bytes.NewReader(shard), int64(len(shard)))
	}
	return m.storeRemoteShard(ctx, si.InstanceID, m.extractShardPrefix(si.Path), si.Index, bytes.NewReader(shard), int64(len(shard)))
}

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
			endpoint, _ = m.peerEndpoint(si.InstanceID)
		}

		if endpoint == "" {
			return nil, fmt.Errorf("no endpoint found for instance %s (shard %d)", si.InstanceID, si.Index)
		}

		shardEndpoints = append(shardEndpoints, ShardEndpoint{
			Index:    si.Index,
			Endpoint: strings.TrimRight(endpoint, "/") + fmt.Sprintf("/v1/shards/%s/%d", escapePathSegments(path), si.Index),
			Size:     si.Size,
			Checksum: si.Checksum,
		})
	}
	manifest.Shards = shardEndpoints

	return manifest, nil
}

func (m *Manager) DeleteFile(ctx context.Context, path string) error {
	found, err := m.DeleteFileShards(ctx, path)
	if nil != err || !found {
		return err
	}

	if err := m.erasureStore.DeleteErasureInfo(ctx, path); err != nil {
		return fmt.Errorf("failed to delete erasure metadata: %w", err)
	}
	m.forgetHealth(path)
	return nil
}

func (m *Manager) DeleteFileShards(ctx context.Context, path string) (bool, error) {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, path)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get erasure info for delete: %w", err)
	}

	if err := m.DeleteShards(ctx, mdInfo); err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) ForgetHealth(path string) {
	m.forgetHealth(path)
}

func (m *Manager) DeleteShards(ctx context.Context, mdInfo *metadata.ErasureFileInfo) error {
	var mu sync.Mutex
	var deleteErrs []error
	var wg sync.WaitGroup
	for _, si := range mdInfo.Shards {
		wg.Add(1)
		go func(si metadata.ErasureShardInfo) {
			defer wg.Done()
			if delErr := m.deleteShard(ctx, si); delErr != nil {
				mu.Lock()
				defer mu.Unlock()
				deleteErrs = append(deleteErrs, fmt.Errorf("failed to delete shard %d on %s: %w", si.Index, si.InstanceID, delErr))
			}
		}(si)
	}
	wg.Wait()

	if len(deleteErrs) > 0 {
		return errors.Join(deleteErrs...)
	}
	return nil
}

func (m *Manager) deleteShard(ctx context.Context, si metadata.ErasureShardInfo) error {
	if si.InstanceID == m.instanceID {
		if err := m.localBackend.Delete(ctx, si.Path); err != nil && !errors.Is(err, metadata.ErrNotFound) {
			return err
		}
		return nil
	}
	return m.deleteRemoteShard(ctx, si.InstanceID, m.extractShardPrefix(si.Path), si.Index)
}

func (m *Manager) RenameFile(ctx context.Context, oldPath, newPath string) error {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, oldPath)
	if err != nil {
		return fmt.Errorf("failed to get erasure info for rename: %w", err)
	}

	mdInfo.FilePath = newPath
	if err := m.erasureStore.CreateErasureInfo(ctx, newPath, mdInfo); err != nil {
		return fmt.Errorf("failed to write erasure metadata at new path: %w", err)
	}

	if err := m.erasureStore.DeleteErasureInfo(ctx, oldPath); err != nil {
		m.logger.Warn("Failed to delete old erasure metadata after rename",
			zap.String("old_path", oldPath),
			zap.String("new_path", newPath),
			zap.Error(err))
	}

	m.forgetHealth(oldPath)
	m.forgetHealth(newPath)
	m.logger.Info("Erasure-coded file metadata renamed",
		zap.String("old_path", oldPath),
		zap.String("new_path", newPath))
	return nil
}

func (m *Manager) RenameFileOverwrite(ctx context.Context, oldPath, newPath string) (*metadata.ErasureFileInfo, error) {
	mdInfo, err := m.erasureStore.GetErasureInfo(ctx, oldPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get erasure info for rename: %w", err)
	}

	replacedInfo, err := m.erasureStore.GetErasureInfo(ctx, newPath)
	if err != nil {
		if !errors.Is(err, metadata.ErrNotFound) {
			return nil, fmt.Errorf("failed to get erasure info at rename destination: %w", err)
		}
		replacedInfo = nil
	}
	if nil != replacedInfo {
		if err := m.erasureStore.DeleteErasureInfo(ctx, newPath); err != nil {
			return nil, fmt.Errorf("failed to delete replaced erasure metadata: %w", err)
		}
	}

	mdInfo.FilePath = newPath
	if err := m.erasureStore.CreateErasureInfo(ctx, newPath, mdInfo); err != nil {
		createErr := fmt.Errorf("failed to write erasure metadata at new path: %w", err)
		if nil != replacedInfo {
			if restoreErr := m.erasureStore.CreateErasureInfo(context.WithoutCancel(ctx), newPath, replacedInfo); nil != restoreErr {
				return nil, errors.Join(createErr, fmt.Errorf("failed to restore replaced erasure metadata: %w", restoreErr))
			}
		}
		return nil, createErr
	}

	if err := m.erasureStore.DeleteErasureInfo(ctx, oldPath); err != nil {
		m.logger.Warn("Failed to delete old erasure metadata after rename",
			zap.String("old_path", oldPath),
			zap.String("new_path", newPath),
			zap.Error(err))
	}

	m.forgetHealth(oldPath)
	m.forgetHealth(newPath)
	m.logger.Info("Erasure-coded file metadata renamed with overwrite",
		zap.String("old_path", oldPath),
		zap.String("new_path", newPath))
	return replacedInfo, nil
}

func newShardPrefix() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("failed to generate shard id: %w", err)
	}
	return hex.EncodeToString(id[:]), nil
}

func escapePathSegments(path string) string {
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}

func (m *Manager) extractShardPrefix(shardPath string) string {
	trimmed := strings.TrimPrefix(shardPath, legacyShardPath+"/")
	if root, err := m.shardRoot(); nil == err && strings.HasPrefix(shardPath, root+"/") {
		trimmed = strings.TrimPrefix(shardPath, root+"/")
	}
	lastSlash := strings.LastIndex(trimmed, "/")
	if lastSlash < 0 {
		return trimmed
	}
	return trimmed[:lastSlash]
}

func (m *Manager) fetchShard(ctx context.Context, si metadata.ErasureShardInfo) ([]byte, error) {
	if si.Size < 0 || si.Size > maxShardReadBytes {
		return nil, fmt.Errorf("shard %d has invalid size %d", si.Index, si.Size)
	}
	rc, err := m.openShard(ctx, si)
	if err != nil {
		return nil, err
	}
	return readAllSized(rc, si.Size)
}

func requireHTTPSEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if nil != err {
		return fmt.Errorf("peer endpoint %s is not a valid URL: %w", endpoint, err)
	}
	if "https" == parsed.Scheme {
		return nil
	}
	if ip := net.ParseIP(parsed.Hostname()); "http" == parsed.Scheme && (strings.EqualFold("localhost", parsed.Hostname()) || (nil != ip && ip.IsLoopback())) {
		return nil
	}
	return fmt.Errorf("peer endpoint %s must use https (http is allowed only for loopback peers)", endpoint)
}

func shardURL(endpoint, shardPrefix string, index int) string {
	return strings.TrimRight(endpoint, "/") + fmt.Sprintf("/v1/internal/shards/%s/%d", shardPrefix, index)
}

func (m *Manager) storeRemoteShard(ctx context.Context, instanceID, shardPrefix string, index int, body io.Reader, size int64) error {
	endpoint, ok := m.peerEndpoint(instanceID)
	if !ok {
		return fmt.Errorf("no endpoint for instance %s", instanceID)
	}

	if err := requireHTTPSEndpoint(endpoint); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, shardURL(endpoint, shardPrefix, index), body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.internalToken)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = size

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

func (m *Manager) openRemoteShard(ctx context.Context, instanceID, shardPrefix string, index int) (io.ReadCloser, error) {
	endpoint, ok := m.peerEndpoint(instanceID)
	if !ok {
		return nil, fmt.Errorf("no endpoint for instance %s", instanceID)
	}

	if err := requireHTTPSEndpoint(endpoint); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, shardURL(endpoint, shardPrefix, index), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.internalToken)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if http.StatusNotFound == resp.StatusCode {
			return nil, fmt.Errorf("remote shard fetch failed with status %d: %w", resp.StatusCode, metadata.ErrNotFound)
		}
		return nil, fmt.Errorf("remote shard fetch failed with status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func (m *Manager) deleteRemoteShard(ctx context.Context, instanceID, shardPrefix string, index int) error {
	endpoint, ok := m.peerEndpoint(instanceID)
	if !ok {
		return fmt.Errorf("no endpoint for instance %s", instanceID)
	}

	if err := requireHTTPSEndpoint(endpoint); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, remoteRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, shardURL(endpoint, shardPrefix, index), nil)
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

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("remote shard delete failed with status %d", resp.StatusCode)
	}
	return nil
}
