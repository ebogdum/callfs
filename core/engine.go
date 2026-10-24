package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/backends"
	"github.com/ebogdum/callfs/v2/backends/internalproxy"
	"github.com/ebogdum/callfs/v2/cluster"
	"github.com/ebogdum/callfs/v2/erasure"
	"github.com/ebogdum/callfs/v2/locks"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/metrics"
)

const (
	defaultMetadataCacheTTL  = 5 * time.Minute
	defaultMetadataCacheSize = 1000
	replicaWorkerCount       = 4
	replicaQueueSize         = 1024
)

// Engine represents the core CallFS engine that orchestrates operations
type Engine struct {
	metadataStore        metadata.Store
	localFSBackend       backends.Storage
	s3Backend            backends.Storage
	internalProxyBackend backends.Storage
	internalProxyAdapter *internalproxy.InternalProxyAdapter // Direct access for instance-specific methods
	lockManager          locks.Manager
	currentInstanceID    string
	peerMu               sync.RWMutex
	peerEndpoints        map[string]string // Instance ID -> endpoint URL
	replicationEnabled   bool
	replicaBackend       string
	requireReplicaAck    bool
	erasureMu            sync.RWMutex
	erasureManager       *erasure.Manager
	metadataCache        *MetadataCache
	replicaQueue         chan replicaJob
	taskQueue            cluster.Queue
	stopTasks            func()
	replicaCtx           context.Context
	replicaCancel        context.CancelFunc
	replicationFailures  atomic.Pointer[func(path, message string)]
	replicationOutcomes  atomic.Pointer[func(outcome string)]
	strict               bool
	asideWriter          asideWriter
	oldCopyDrops         sync.Map
	trueOverwrite        bool
	tmpSaveMaxSize       int64
	logger               *zap.Logger
}

// NewEngine creates a new core engine instance
func NewEngine(
	metadataStore metadata.Store,
	localFSBackend backends.Storage,
	s3Backend backends.Storage,
	internalProxyBackend backends.Storage,
	internalProxyAdapter *internalproxy.InternalProxyAdapter,
	lockManager locks.Manager,
	currentInstanceID string,
	peerEndpoints map[string]string,
	replicationEnabled bool,
	replicaBackend string,
	requireReplicaAck bool,
	logger *zap.Logger,
) *Engine {
	replicaCtx, replicaCancel := context.WithCancel(context.Background())
	engine := &Engine{
		metadataStore:        metadataStore,
		localFSBackend:       localFSBackend,
		s3Backend:            s3Backend,
		internalProxyBackend: internalProxyBackend,
		internalProxyAdapter: internalProxyAdapter,
		lockManager:          lockManager,
		currentInstanceID:    currentInstanceID,
		peerEndpoints:        peerEndpoints,
		replicationEnabled:   replicationEnabled,
		replicaBackend:       replicaBackend,
		requireReplicaAck:    requireReplicaAck,
		metadataCache:        NewMetadataCache(defaultMetadataCacheTTL, defaultMetadataCacheSize),
		replicaQueue:         make(chan replicaJob, replicaQueueSize),
		replicaCtx:           replicaCtx,
		replicaCancel:        replicaCancel,
		asideWriter:          resolveAsideWriter(localFSBackend),
		logger:               logger,
	}
	if localState, ok := metadataStore.(metadata.LocalState); ok && localState.LocalState() {
		engine.metadataCache.SetBypass(true)
	}
	for range replicaWorkerCount {
		go engine.replicaWorker()
	}
	return engine
}

func (e *Engine) SetReplicationFailureRecorder(record func(path, message string)) {
	e.replicationFailures.Store(&record)
}

func (e *Engine) SetReplicationOutcomeRecorder(record func(outcome string)) {
	e.replicationOutcomes.Store(&record)
}

func (e *Engine) countReplication(outcome string) {
	metrics.ReplicationTotal.WithLabelValues(outcome).Inc()
	if record := e.replicationOutcomes.Load(); nil != record {
		(*record)(outcome)
	}
}

func (e *Engine) SetStrict(strict bool) {
	e.strict = strict
}

func (e *Engine) SetTrueOverwrite(enabled bool, tmpSaveMaxSize int64) {
	e.trueOverwrite = enabled
	e.tmpSaveMaxSize = tmpSaveMaxSize
}

func (e *Engine) SetMetadataCacheTTL(ttl time.Duration) {
	e.metadataCache.SetTTL(ttl)
}

func (e *Engine) SetMetadataCacheSize(size int) {
	e.metadataCache.SetMaxSize(size)
}

func (e *Engine) MetadataCache() *MetadataCache {
	return e.metadataCache
}

// GetCurrentInstanceID returns the current instance ID
func (e *Engine) GetCurrentInstanceID() string {
	return e.currentInstanceID
}

// SetErasureManager sets the erasure manager on the engine
func (e *Engine) SetErasureManager(em *erasure.Manager) {
	e.erasureMu.Lock()
	defer e.erasureMu.Unlock()
	e.erasureManager = em
}

// GetErasureManager returns the erasure manager (may be nil)
func (e *Engine) GetErasureManager() *erasure.Manager {
	e.erasureMu.RLock()
	defer e.erasureMu.RUnlock()
	return e.erasureManager
}

// Close shuts down the engine and releases background resources.
func (e *Engine) Close() {
	e.replicaCancel()
	if nil != e.stopTasks {
		e.stopTasks()
	}
	e.metadataCache.Close()
}

// GetPeerEndpoint returns the endpoint URL for a given instance ID
func (e *Engine) GetPeerEndpoint(instanceID string) string {
	e.peerMu.RLock()
	defer e.peerMu.RUnlock()
	if endpoint, exists := e.peerEndpoints[instanceID]; exists {
		return endpoint
	}
	return ""
}

func (e *Engine) AddPeerEndpoint(instanceID, endpoint string) error {
	if nil != e.internalProxyAdapter {
		if err := e.internalProxyAdapter.AddPeerEndpoint(instanceID, endpoint); nil != err {
			return err
		}
	}
	e.peerMu.Lock()
	e.peerEndpoints[instanceID] = endpoint
	e.peerMu.Unlock()
	return nil
}

func (e *Engine) RemovePeerEndpoint(instanceID string) {
	e.peerMu.Lock()
	defer e.peerMu.Unlock()
	delete(e.peerEndpoints, instanceID)
	if em := e.GetErasureManager(); nil != em {
		em.RemovePeer(instanceID)
	}
}

var (
	ErrUnknownStorage     = errors.New("unknown storage backend")
	ErrStorageUnavailable = errors.New("storage backend is not configured")
	errProbeMismatch      = errors.New("probe content mismatch")
)

const storageProbePayload = "callfs-probe"

type StorageProbe struct {
	OK       bool
	WriteMS  int64
	ReadMS   int64
	DeleteMS int64
	Error    string
}

func (e *Engine) ProbeStorage(ctx context.Context, name string) (StorageProbe, error) {
	if "internal_proxy" == name && nil != e.internalProxyAdapter {
		started := time.Now()
		err := e.internalProxyAdapter.Ping(ctx)
		probe := StorageProbe{OK: nil == err, ReadMS: time.Since(started).Milliseconds()}
		if nil != err {
			probe.Error = err.Error()
		}
		return probe, nil
	}
	backend, err := e.storageByName(name)
	if nil != err {
		return StorageProbe{}, err
	}

	var suffix [16]byte
	if _, err := rand.Read(suffix[:]); nil != err {
		return StorageProbe{}, fmt.Errorf("generate probe name: %w", err)
	}
	path := "/.callfs-probe/" + hex.EncodeToString(suffix[:])

	probe := StorageProbe{}
	started := time.Now()
	err = backend.Create(ctx, path, strings.NewReader(storageProbePayload), int64(len(storageProbePayload)))
	probe.WriteMS = time.Since(started).Milliseconds()
	if nil != err {
		probe.Error = err.Error()
		return probe, nil
	}

	started = time.Now()
	err = readStorageProbe(ctx, backend, path)
	probe.ReadMS = time.Since(started).Milliseconds()

	started = time.Now()
	deleteErr := backend.Delete(ctx, path)
	probe.DeleteMS = time.Since(started).Milliseconds()

	switch {
	case nil != err:
		probe.Error = err.Error()
	case nil != deleteErr:
		probe.Error = deleteErr.Error()
	default:
		probe.OK = true
	}
	return probe, nil
}

func (e *Engine) storageByName(name string) (backends.Storage, error) {
	candidates := map[string]backends.Storage{
		"localfs":        e.localFSBackend,
		"s3":             e.s3Backend,
		"internal_proxy": e.internalProxyBackend,
	}
	backend, known := candidates[name]
	if !known {
		return nil, fmt.Errorf("%w: %s", ErrUnknownStorage, name)
	}
	if nil == backend {
		return nil, fmt.Errorf("%w: %s", ErrStorageUnavailable, name)
	}
	return backend, nil
}

func readStorageProbe(ctx context.Context, backend backends.Storage, path string) error {
	reader, err := backend.Open(ctx, path)
	if nil != err {
		return err
	}
	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if nil != readErr {
		return readErr
	}
	if nil != closeErr {
		return closeErr
	}
	if storageProbePayload != string(data) {
		return errProbeMismatch
	}
	return nil
}
