package core

import (
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/backends"
	"github.com/ebogdum/callfs/backends/internalproxy"
	"github.com/ebogdum/callfs/locks"
	"github.com/ebogdum/callfs/metadata"
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
	peerEndpoints        map[string]string // Instance ID -> endpoint URL
	replicationEnabled   bool
	replicaBackend       string
	requireReplicaAck    bool
	metadataCache        *MetadataCache
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
	return &Engine{
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
		metadataCache:        NewMetadataCache(5*time.Minute, 1000), // 5 min TTL, max 1000 entries
		logger:               logger,
	}
}

// GetCurrentInstanceID returns the current instance ID
func (e *Engine) GetCurrentInstanceID() string {
	return e.currentInstanceID
}

// GetPeerEndpoint returns the endpoint URL for a given instance ID
func (e *Engine) GetPeerEndpoint(instanceID string) string {
	if endpoint, exists := e.peerEndpoints[instanceID]; exists {
		return endpoint
	}
	return ""
}
