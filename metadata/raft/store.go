package raft

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	hashiraft "github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/metadata"
)

type Config struct {
	NodeID              string
	BindAddr            string
	DataDir             string
	Bootstrap           bool
	Peers               map[string]string
	APIPeerEndpoints    map[string]string
	ApplyTimeout        time.Duration
	ForwardTimeout      time.Duration
	SnapshotInterval    time.Duration
	SnapshotThreshold   uint64
	RetainSnapshotCount int
	InternalAuthToken   string
}

type Command struct {
	Op          string                    `json:"op"`
	Path        string                    `json:"path,omitempty"`
	NewPath     string                    `json:"new_path,omitempty"`
	Metadata    *metadata.Metadata        `json:"metadata,omitempty"`
	Token       string                    `json:"token,omitempty"`
	Link        *metadata.SingleUseLink   `json:"link,omitempty"`
	Status      string                    `json:"status,omitempty"`
	UsedAt      *time.Time                `json:"used_at,omitempty"`
	UsedByIP    *string                   `json:"used_by_ip,omitempty"`
	Before      *time.Time                `json:"before,omitempty"`
	OlderThan   *time.Time                `json:"older_than,omitempty"`
	ErasureInfo *metadata.ErasureFileInfo `json:"erasure_info,omitempty"`

	// Timestamp is stamped once by the leader in applyAsLeader, immediately before
	// the command enters the Raft log. The FSM must use this value rather than
	// calling time.Now(): Apply runs independently on every replica (and again on
	// log replay after a restart), so a wall-clock read inside Apply would produce
	// a different value per replica and rewrite history on replay.
	Timestamp time.Time `json:"timestamp,omitempty"`
}

type CommandResult struct {
	CleanupCount int                     `json:"cleanup_count,omitempty"`
	Err          string                  `json:"err,omitempty"`
	Metadata     *metadata.Metadata      `json:"metadata,omitempty"`
	Link         *metadata.SingleUseLink `json:"link,omitempty"`
	Children     []*metadata.Metadata    `json:"children,omitempty"`
}

type ForwardApplyRequest struct {
	Command Command `json:"command"`
}

type ForwardApplyResponse struct {
	CleanupCount int                     `json:"cleanup_count,omitempty"`
	Error        string                  `json:"error,omitempty"`
	Metadata     *metadata.Metadata      `json:"metadata,omitempty"`
	Link         *metadata.SingleUseLink `json:"link,omitempty"`
	Children     []*metadata.Metadata    `json:"children,omitempty"`
}

type JoinRequest struct {
	NodeID      string `json:"node_id"`
	RaftAddr    string `json:"raft_addr"`
	APIEndpoint string `json:"api_endpoint"`
}

type JoinResponse struct {
	Status   string `json:"status"`
	LeaderID string `json:"leader_id,omitempty"`
	Error    string `json:"error,omitempty"`
}

type Store struct {
	raft              *hashiraft.Raft
	fsm               *fsm
	logStore          *raftboltdb.BoltStore
	stableStore       *raftboltdb.BoltStore
	nodeID            string
	apiPeerMu         sync.RWMutex
	apiPeerEndpoints  map[string]string
	internalAuthToken string
	forwardClient     *http.Client
	applyTimeout      time.Duration
	logger            *zap.Logger
}

type state struct {
	MetadataByPath map[string]*metadata.Metadata        `json:"metadata_by_path"`
	LinksByToken   map[string]*metadata.SingleUseLink   `json:"links_by_token"`
	ErasureByPath  map[string]*metadata.ErasureFileInfo `json:"erasure_by_path"`
}

type fsm struct {
	mu    sync.RWMutex
	state state

	// childIndex maps a directory path to the set of its direct children. It is
	// derived entirely from state.MetadataByPath and is deliberately NOT part of
	// the serialized snapshot: it is rebuilt by rebuildChildIndex on Restore, so
	// the on-disk snapshot format is unchanged and a bug here can never corrupt
	// replicated state.
	//
	// Without it, listing one directory scanned every metadata entry in the
	// cluster while holding the FSM lock, so listing cost grew with total
	// filesystem size rather than with the size of the directory being listed.
	childIndex map[string]map[string]struct{}
}

// newFSM returns an FSM with empty state and an initialized child index. Both
// production and tests must build FSMs through this, so a new state field cannot
// be left uninitialized in one path and not the other.
func newFSM() *fsm {
	return &fsm{
		state: state{
			MetadataByPath: map[string]*metadata.Metadata{},
			LinksByToken:   map[string]*metadata.SingleUseLink{},
			ErasureByPath:  map[string]*metadata.ErasureFileInfo{},
		},
		childIndex: map[string]map[string]struct{}{},
	}
}

// indexChild records child under its parent directory. Caller must hold the lock.
func (f *fsm) indexChild(path string) {
	if path == "/" {
		return
	}
	parent := pathDir(path)
	if f.childIndex[parent] == nil {
		f.childIndex[parent] = make(map[string]struct{})
	}
	f.childIndex[parent][path] = struct{}{}
}

// unindexChild removes child from its parent's set. Caller must hold the lock.
func (f *fsm) unindexChild(path string) {
	if path == "/" {
		return
	}
	parent := pathDir(path)
	if set := f.childIndex[parent]; set != nil {
		delete(set, path)
		if len(set) == 0 {
			delete(f.childIndex, parent)
		}
	}
}

// rebuildChildIndex regenerates the whole index from metadata. Caller must hold
// the lock. Used on snapshot restore, where state is replaced wholesale.
func (f *fsm) rebuildChildIndex() {
	f.childIndex = make(map[string]map[string]struct{}, len(f.state.MetadataByPath))
	for path := range f.state.MetadataByPath {
		f.indexChild(path)
	}
}

// childrenOf returns the direct children of parentPath. Caller must hold at
// least a read lock.
func (f *fsm) childrenOf(parentPath string) []*metadata.Metadata {
	children := make([]*metadata.Metadata, 0, len(f.childIndex[parentPath]))
	for path := range f.childIndex[parentPath] {
		if md, ok := f.state.MetadataByPath[path]; ok {
			children = append(children, cloneMetadata(md))
		}
	}
	return children
}

type stateSnapshot struct {
	state state
}

func (cfg *Config) applyDefaults() error {
	if cfg.NodeID == "" {
		return fmt.Errorf("raft node id is required")
	}
	if cfg.BindAddr == "" {
		return fmt.Errorf("raft bind address is required")
	}
	if cfg.DataDir == "" {
		return fmt.Errorf("raft data_dir is required")
	}

	defaults := []struct {
		ptr          *time.Duration
		defaultValue time.Duration
	}{
		{&cfg.ApplyTimeout, 10 * time.Second},
		{&cfg.ForwardTimeout, 10 * time.Second},
		{&cfg.SnapshotInterval, 60 * time.Second},
	}
	for _, d := range defaults {
		if *d.ptr <= 0 {
			*d.ptr = d.defaultValue
		}
	}
	if cfg.SnapshotThreshold == 0 {
		cfg.SnapshotThreshold = 256
	}
	if cfg.RetainSnapshotCount <= 0 {
		cfg.RetainSnapshotCount = 2
	}

	return nil
}

func NewRaftStore(cfg Config, logger *zap.Logger) (*Store, error) {
	if err := cfg.applyDefaults(); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create raft data dir: %w", err)
	}

	fsmInstance := newFSM()

	raftCfg := hashiraft.DefaultConfig()
	raftCfg.LocalID = hashiraft.ServerID(cfg.NodeID)
	raftCfg.SnapshotInterval = cfg.SnapshotInterval
	raftCfg.SnapshotThreshold = cfg.SnapshotThreshold

	raftLogWriter := zap.NewStdLog(logger).Writer()
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.DataDir, "raft-log.db"))
	if err != nil {
		return nil, fmt.Errorf("failed to create raft log store: %w", err)
	}
	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.DataDir, "raft-stable.db"))
	if err != nil {
		return nil, fmt.Errorf("failed to create raft stable store: %w", err)
	}
	snapshotStore, err := hashiraft.NewFileSnapshotStore(cfg.DataDir, cfg.RetainSnapshotCount, raftLogWriter)
	if err != nil {
		return nil, fmt.Errorf("failed to create raft snapshot store: %w", err)
	}
	transport, err := hashiraft.NewTCPTransport(cfg.BindAddr, nil, 3, 10*time.Second, raftLogWriter)
	if err != nil {
		return nil, fmt.Errorf("failed to create raft transport: %w", err)
	}

	raftNode, err := hashiraft.NewRaft(raftCfg, fsmInstance, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		return nil, fmt.Errorf("failed to create raft node: %w", err)
	}

	store := &Store{
		raft:              raftNode,
		fsm:               fsmInstance,
		logStore:          logStore,
		stableStore:       stableStore,
		nodeID:            cfg.NodeID,
		apiPeerEndpoints:  copyStringMap(cfg.APIPeerEndpoints),
		internalAuthToken: cfg.InternalAuthToken,
		forwardClient: &http.Client{
			Timeout: cfg.ForwardTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        64,
				MaxIdleConnsPerHost: 16,
				MaxConnsPerHost:     32,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		applyTimeout: cfg.ApplyTimeout,
		logger:       logger,
	}

	if cfg.Bootstrap {
		if err := store.bootstrapCluster(cfg); err != nil {
			return nil, err
		}
	}

	return store, nil
}

func (s *Store) bootstrapCluster(cfg Config) error {
	servers := []hashiraft.Server{
		{ID: hashiraft.ServerID(cfg.NodeID), Address: hashiraft.ServerAddress(cfg.BindAddr), Suffrage: hashiraft.Voter},
	}
	for peerID, peerAddr := range cfg.Peers {
		servers = append(servers, hashiraft.Server{
			ID:       hashiraft.ServerID(peerID),
			Address:  hashiraft.ServerAddress(peerAddr),
			Suffrage: hashiraft.Voter,
		})
	}
	future := s.raft.BootstrapCluster(hashiraft.Configuration{Servers: servers})
	if err := future.Error(); err != nil && !errors.Is(err, hashiraft.ErrCantBootstrap) {
		return fmt.Errorf("failed to bootstrap raft cluster: %w", err)
	}
	return nil
}

func (s *Store) IsLeader() bool {
	return s.raft.State() == hashiraft.Leader
}

func (s *Store) LeaderID() string {
	_, leaderID := s.raft.LeaderWithID()
	return string(leaderID)
}

func (s *Store) SetAPIPeerEndpoint(nodeID, endpoint string) {
	nodeID = strings.TrimSpace(nodeID)
	endpoint = strings.TrimSpace(endpoint)
	if nodeID == "" || endpoint == "" {
		return
	}
	s.apiPeerMu.Lock()
	defer s.apiPeerMu.Unlock()
	s.apiPeerEndpoints[nodeID] = endpoint
}

func (s *Store) APIPeerEndpoint(nodeID string) (string, bool) {
	s.apiPeerMu.RLock()
	defer s.apiPeerMu.RUnlock()
	endpoint, ok := s.apiPeerEndpoints[nodeID]
	return endpoint, ok
}

// removeConflictingServers removes any existing servers that conflict with the
// incoming nodeID/raftAddr pair. Returns true if the node already exists as a
// matching voter (no-op join).
func (s *Store) removeConflictingServers(nodeID, raftAddr string, servers []hashiraft.Server) (bool, error) {
	for _, server := range servers {
		serverID := string(server.ID)
		serverAddr := string(server.Address)

		if serverID == nodeID {
			if serverAddr == raftAddr && server.Suffrage == hashiraft.Voter {
				return true, nil
			}
			removeFuture := s.raft.RemoveServer(server.ID, 0, s.applyTimeout)
			if err := removeFuture.Error(); err != nil {
				return false, fmt.Errorf("failed to remove existing node %s before rejoin: %w", nodeID, err)
			}
		}

		if serverAddr == raftAddr && serverID != nodeID {
			removeFuture := s.raft.RemoveServer(server.ID, 0, s.applyTimeout)
			if err := removeFuture.Error(); err != nil {
				return false, fmt.Errorf("failed to remove stale node %s on address %s: %w", serverID, raftAddr, err)
			}
		}
	}
	return false, nil
}

func (s *Store) AddVoter(ctx context.Context, nodeID, raftAddr, apiEndpoint string) error {
	nodeID = strings.TrimSpace(nodeID)
	raftAddr = strings.TrimSpace(raftAddr)
	apiEndpoint = strings.TrimSpace(apiEndpoint)

	if nodeID == "" {
		return fmt.Errorf("node_id is required")
	}
	if raftAddr == "" {
		return fmt.Errorf("raft_addr is required")
	}
	if !s.IsLeader() {
		return fmt.Errorf("not leader")
	}

	configFuture := s.raft.GetConfiguration()
	if err := configFuture.Error(); err != nil {
		return fmt.Errorf("failed to get raft configuration: %w", err)
	}

	alreadyVoter, err := s.removeConflictingServers(nodeID, raftAddr, configFuture.Configuration().Servers)
	if err != nil {
		return err
	}
	if alreadyVoter {
		s.SetAPIPeerEndpoint(nodeID, apiEndpoint)
		return nil
	}

	addFuture := s.raft.AddVoter(hashiraft.ServerID(nodeID), hashiraft.ServerAddress(raftAddr), 0, s.applyTimeout)
	if err := addFuture.Error(); err != nil {
		return fmt.Errorf("failed to add raft voter %s: %w", nodeID, err)
	}

	s.SetAPIPeerEndpoint(nodeID, apiEndpoint)

	// Force a snapshot after adding a voter so the new node can install it
	// and catch up with the leader's state. Without this, followers that
	// join after the leader has already committed entries will fail to
	// replicate via log entries (since the leader's log may have been
	// compacted, or the follower starts at index 0 while the leader is ahead).
	snapshotFuture := s.raft.Snapshot()
	if err := snapshotFuture.Error(); err != nil {
		s.logger.Warn("Post-join snapshot failed (follower will catch up via log replication if possible)",
			zap.String("node_id", nodeID), zap.Error(err))
	}

	// Barrier ensures all pending entries (including the snapshot) are committed
	if err := s.raft.Barrier(s.applyTimeout).Error(); err != nil {
		s.logger.Warn("Barrier after voter join returned error",
			zap.String("node_id", nodeID), zap.Error(err))
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (s *Store) Get(ctx context.Context, path string) (*metadata.Metadata, error) {
	// Try local FSM first (fast path)
	s.fsm.mu.RLock()
	md, ok := s.fsm.state.MetadataByPath[path]
	s.fsm.mu.RUnlock()
	if ok {
		return cloneMetadata(md), nil
	}

	// On followers, if not found locally, forward to leader for consistent read.
	// This handles the case where the follower's FSM hasn't applied recent writes yet.
	if !s.IsLeader() {
		result, err := s.forwardToLeader(ctx, Command{Op: "get_metadata", Path: path})
		if err != nil {
			return nil, metadata.ErrNotFound
		}
		if result.Metadata != nil {
			return cloneMetadata(result.Metadata), nil
		}
		return nil, metadata.ErrNotFound
	}

	return nil, metadata.ErrNotFound
}

func (s *Store) Create(ctx context.Context, md *metadata.Metadata) error {
	if md == nil {
		return fmt.Errorf("metadata is required")
	}
	_, err := s.applyCommand(ctx, Command{Op: "create_metadata", Metadata: cloneMetadata(md)})
	return err
}

func (s *Store) Update(ctx context.Context, md *metadata.Metadata) error {
	if md == nil {
		return fmt.Errorf("metadata is required")
	}
	_, err := s.applyCommand(ctx, Command{Op: "update_metadata", Metadata: cloneMetadata(md)})
	return err
}

func (s *Store) Delete(ctx context.Context, path string) error {
	_, err := s.applyCommand(ctx, Command{Op: "delete_metadata", Path: path})
	return err
}

func (s *Store) Rename(ctx context.Context, oldPath, newPath string) error {
	_, err := s.applyCommand(ctx, Command{Op: "rename_metadata", Path: oldPath, NewPath: newPath})
	return err
}

func (s *Store) ListChildren(ctx context.Context, parentPath string) ([]*metadata.Metadata, error) {
	// Leader reads directly from the FSM (authoritative)
	if s.IsLeader() {
		return s.listChildrenLocal(parentPath), nil
	}

	// Followers always forward to leader for consistent reads.
	// A follower's FSM may be partially replicated (e.g., some children
	// arrived via Raft but not all), so returning local results risks
	// missing recently-created entries.
	result, err := s.forwardToLeader(ctx, Command{Op: "list_children", Path: parentPath})
	if err != nil {
		// Fallback to local FSM if leader is unreachable
		return s.listChildrenLocal(parentPath), nil
	}
	if result.Children != nil {
		sort.Slice(result.Children, func(i, j int) bool { return result.Children[i].Path < result.Children[j].Path })
		return result.Children, nil
	}
	return make([]*metadata.Metadata, 0), nil
}

// listChildrenLocal reads children from the local FSM state.
func (s *Store) listChildrenLocal(parentPath string) []*metadata.Metadata {
	s.fsm.mu.RLock()
	children := s.fsm.childrenOf(parentPath)
	s.fsm.mu.RUnlock()
	sort.Slice(children, func(i, j int) bool { return children[i].Path < children[j].Path })
	return children
}

func (s *Store) GetSingleUseLink(ctx context.Context, token string) (*metadata.SingleUseLink, error) {
	// Try local FSM first
	s.fsm.mu.RLock()
	link, ok := s.fsm.state.LinksByToken[token]
	s.fsm.mu.RUnlock()
	if ok {
		return cloneLink(link), nil
	}

	// On followers, forward to leader for consistent read
	if !s.IsLeader() {
		result, err := s.forwardToLeader(ctx, Command{Op: "get_link", Token: token})
		if err != nil {
			return nil, metadata.ErrNotFound
		}
		if result.Link != nil {
			return cloneLink(result.Link), nil
		}
		return nil, metadata.ErrNotFound
	}

	return nil, metadata.ErrNotFound
}

func (s *Store) CreateSingleUseLink(ctx context.Context, link *metadata.SingleUseLink) error {
	if link == nil {
		return fmt.Errorf("link is required")
	}
	_, err := s.applyCommand(ctx, Command{Op: "create_link", Link: cloneLink(link)})
	return err
}

func (s *Store) UpdateSingleUseLink(ctx context.Context, token string, status string, usedAt *time.Time, usedByIP *string) error {
	_, err := s.applyCommand(ctx, Command{Op: "update_link", Token: token, Status: status, UsedAt: usedAt, UsedByIP: usedByIP})
	return err
}

func (s *Store) CleanupExpiredLinks(ctx context.Context, before time.Time) (int, error) {
	res, err := s.applyCommand(ctx, Command{Op: "cleanup_expired_links", Before: &before})
	if err != nil {
		return 0, err
	}
	return res.CleanupCount, nil
}

func (s *Store) CleanupUsedLinks(ctx context.Context, olderThan time.Time) (int, error) {
	res, err := s.applyCommand(ctx, Command{Op: "cleanup_used_links", OlderThan: &olderThan})
	if err != nil {
		return 0, err
	}
	return res.CleanupCount, nil
}

// TakeSnapshot forces a Raft snapshot. This is useful after initial setup
// so that followers joining later can install the snapshot and catch up.
func (s *Store) TakeSnapshot() error {
	return s.raft.Snapshot().Error()
}

func (s *Store) Close() error {
	f := s.raft.Shutdown()
	if err := f.Error(); err != nil {
		return fmt.Errorf("failed to shutdown raft: %w", err)
	}
	s.forwardClient.CloseIdleConnections()
	var firstErr error
	if s.logStore != nil {
		if err := s.logStore.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to close raft log store: %w", err)
		}
	}
	if s.stableStore != nil {
		if err := s.stableStore.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to close raft stable store: %w", err)
		}
	}
	return firstErr
}

func (s *Store) ApplyForwardedCommand(ctx context.Context, cmd Command) (CommandResult, error) {
	if !s.IsLeader() {
		return CommandResult{}, fmt.Errorf("not leader")
	}

	// Read-only queries can be served directly from the leader's FSM without
	// going through Raft Apply, which would needlessly pollute the consensus log.
	switch cmd.Op {
	case "get_metadata":
		s.fsm.mu.RLock()
		md, ok := s.fsm.state.MetadataByPath[cmd.Path]
		s.fsm.mu.RUnlock()
		if !ok {
			return CommandResult{}, metadata.ErrNotFound
		}
		return CommandResult{Metadata: cloneMetadata(md)}, nil
	case "get_link":
		s.fsm.mu.RLock()
		link, ok := s.fsm.state.LinksByToken[cmd.Token]
		s.fsm.mu.RUnlock()
		if !ok {
			return CommandResult{}, metadata.ErrNotFound
		}
		return CommandResult{Link: cloneLink(link)}, nil
	case "list_children":
		s.fsm.mu.RLock()
		children := s.fsm.childrenOf(cmd.Path)
		s.fsm.mu.RUnlock()
		return CommandResult{Children: children}, nil
	}

	return s.applyAsLeader(cmd)
}

func (s *Store) applyCommand(ctx context.Context, cmd Command) (CommandResult, error) {
	if s.IsLeader() {
		return s.applyAsLeader(cmd)
	}
	return s.forwardToLeader(ctx, cmd)
}

func (s *Store) applyAsLeader(cmd Command) (CommandResult, error) {
	// Stamp the command once, here, from the leader's clock. This is the single
	// point where every command (local or forwarded from a follower) enters the
	// Raft log, so stamping here guarantees all replicas apply an identical value.
	// Always overwrite rather than honouring an inbound value: a forwarded command
	// arrives over the wire, and the leader's clock must be the only source.
	cmd.Timestamp = time.Now().UTC()
	data, err := json.Marshal(cmd)
	if err != nil {
		return CommandResult{}, fmt.Errorf("failed to marshal command: %w", err)
	}
	f := s.raft.Apply(data, s.applyTimeout)
	if err := f.Error(); err != nil {
		return CommandResult{}, fmt.Errorf("raft apply failed: %w", err)
	}
	if f.Response() == nil {
		return CommandResult{}, nil
	}
	res, ok := f.Response().(CommandResult)
	if !ok {
		return CommandResult{}, fmt.Errorf("unexpected raft response type: %T", f.Response())
	}
	if res.Err != "" {
		switch res.Err {
		case "not_found":
			return CommandResult{}, metadata.ErrNotFound
		case "already_exists":
			return CommandResult{}, metadata.ErrAlreadyExists
		default:
			return CommandResult{}, fmt.Errorf("%s", res.Err)
		}
	}
	return res, nil
}

func (s *Store) forwardToLeader(ctx context.Context, cmd Command) (CommandResult, error) {
	_, leaderID := s.raft.LeaderWithID()
	if leaderID == "" {
		return CommandResult{}, fmt.Errorf("no raft leader available")
	}
	leaderEndpoint, ok := s.APIPeerEndpoint(string(leaderID))
	if !ok || strings.TrimSpace(leaderEndpoint) == "" {
		return CommandResult{}, fmt.Errorf("leader endpoint not configured for node id %s", leaderID)
	}

	body, err := json.Marshal(ForwardApplyRequest{Command: cmd})
	if err != nil {
		return CommandResult{}, fmt.Errorf("failed to marshal forward request: %w", err)
	}
	url := strings.TrimRight(leaderEndpoint, "/") + "/v1/internal/raft/metadata/apply"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return CommandResult{}, fmt.Errorf("failed to create forward request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.internalAuthToken))

	resp, err := s.forwardClient.Do(req)
	if err != nil {
		return CommandResult{}, fmt.Errorf("failed to forward request to leader: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Limit error body read to prevent unbounded memory allocation from a misbehaving leader
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return CommandResult{}, fmt.Errorf("leader forward failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var applyResp ForwardApplyResponse
	if err := json.NewDecoder(resp.Body).Decode(&applyResp); err != nil {
		return CommandResult{}, fmt.Errorf("failed to decode forward response: %w", err)
	}
	if applyResp.Error != "" {
		switch applyResp.Error {
		case "not_found":
			return CommandResult{}, metadata.ErrNotFound
		case "already_exists":
			return CommandResult{}, metadata.ErrAlreadyExists
		default:
			return CommandResult{}, fmt.Errorf("%s", applyResp.Error)
		}
	}
	return CommandResult{CleanupCount: applyResp.CleanupCount, Metadata: applyResp.Metadata, Link: applyResp.Link, Children: applyResp.Children}, nil
}

func (f *fsm) applyMetadataOp(cmd *Command) CommandResult {
	switch cmd.Op {
	case "get_metadata":
		md, ok := f.state.MetadataByPath[cmd.Path]
		if !ok {
			return CommandResult{Err: "not_found"}
		}
		return CommandResult{Metadata: cloneMetadata(md)}
	case "create_metadata":
		if cmd.Metadata == nil {
			return CommandResult{Err: "metadata_required"}
		}
		if _, exists := f.state.MetadataByPath[cmd.Metadata.Path]; exists {
			return CommandResult{Err: "already_exists"}
		}
		f.state.MetadataByPath[cmd.Metadata.Path] = cloneMetadata(cmd.Metadata)
		f.indexChild(cmd.Metadata.Path)
		return CommandResult{}
	case "update_metadata":
		if cmd.Metadata == nil {
			return CommandResult{Err: "metadata_required"}
		}
		if _, exists := f.state.MetadataByPath[cmd.Metadata.Path]; !exists {
			return CommandResult{Err: "not_found"}
		}
		f.state.MetadataByPath[cmd.Metadata.Path] = cloneMetadata(cmd.Metadata)
		return CommandResult{}
	case "delete_metadata":
		if _, exists := f.state.MetadataByPath[cmd.Path]; !exists {
			return CommandResult{Err: "not_found"}
		}
		delete(f.state.MetadataByPath, cmd.Path)
		f.unindexChild(cmd.Path)
		return CommandResult{}
	case "rename_metadata":
		src, exists := f.state.MetadataByPath[cmd.Path]
		if !exists {
			return CommandResult{Err: "not_found"}
		}
		if _, exists := f.state.MetadataByPath[cmd.NewPath]; exists {
			return CommandResult{Err: "already_exists"}
		}
		// Use the leader-stamped timestamp, never time.Now(): see Command.Timestamp.
		// A zero timestamp means a pre-upgrade log entry is being replayed; preserve
		// the existing UpdatedAt in that case so replay stays deterministic.
		now := cmd.Timestamp
		// Collect affected paths first to avoid mutating the map while iterating.
		affected := []string{cmd.Path}
		if src.Type == "directory" {
			prefix := cmd.Path + "/"
			for p := range f.state.MetadataByPath {
				if strings.HasPrefix(p, prefix) {
					affected = append(affected, p)
				}
			}
		}
		for _, oldP := range affected {
			md := f.state.MetadataByPath[oldP]
			newP := cmd.NewPath + oldP[len(cmd.Path):]
			md.Path = newP
			md.Name = pathBaseRaft(newP)
			if !now.IsZero() {
				md.UpdatedAt = now
			}
			delete(f.state.MetadataByPath, oldP)
			f.unindexChild(oldP)
			f.state.MetadataByPath[newP] = md
			f.indexChild(newP)
		}
		return CommandResult{}
	default:
		return CommandResult{Children: f.childrenOf(cmd.Path)}
	}
}

func (f *fsm) cleanupLinks(status string, predicate func(link *metadata.SingleUseLink) bool) CommandResult {
	count := 0
	for token, link := range f.state.LinksByToken {
		if link.Status == status && predicate(link) {
			delete(f.state.LinksByToken, token)
			count++
		}
	}
	return CommandResult{CleanupCount: count}
}

func (f *fsm) applyLinkOp(cmd *Command) CommandResult {
	switch cmd.Op {
	case "get_link":
		link, ok := f.state.LinksByToken[cmd.Token]
		if !ok {
			return CommandResult{Err: "not_found"}
		}
		return CommandResult{Link: cloneLink(link)}
	case "create_link":
		if cmd.Link == nil {
			return CommandResult{Err: "link_required"}
		}
		if _, exists := f.state.LinksByToken[cmd.Link.Token]; exists {
			return CommandResult{Err: "already_exists"}
		}
		f.state.LinksByToken[cmd.Link.Token] = cloneLink(cmd.Link)
		return CommandResult{}
	case "update_link":
		link, exists := f.state.LinksByToken[cmd.Token]
		if !exists || link.Status != "active" {
			return CommandResult{Err: "not_found"}
		}
		link.Status = cmd.Status
		link.UsedAt = cloneTimePtr(cmd.UsedAt)
		link.UsedByIP = cloneStringPtr(cmd.UsedByIP)
		// Leader-stamped, never time.Now(): see Command.Timestamp.
		if !cmd.Timestamp.IsZero() {
			link.UpdatedAt = cmd.Timestamp
		}
		f.state.LinksByToken[cmd.Token] = link
		return CommandResult{}
	case "cleanup_expired_links":
		if cmd.Before == nil {
			return CommandResult{Err: "before_required"}
		}
		return f.cleanupLinks("active", func(link *metadata.SingleUseLink) bool {
			return link.ExpiresAt.Before(*cmd.Before)
		})
	default:
		if cmd.OlderThan == nil {
			return CommandResult{Err: "older_than_required"}
		}
		return f.cleanupLinks("used", func(link *metadata.SingleUseLink) bool {
			return link.UsedAt != nil && link.UsedAt.Before(*cmd.OlderThan)
		})
	}
}

func (f *fsm) applyErasureOp(cmd *Command) CommandResult {
	if cmd.Op == "create_erasure_info" {
		if cmd.ErasureInfo == nil {
			return CommandResult{Err: "erasure_info_required"}
		}
		if _, exists := f.state.ErasureByPath[cmd.Path]; exists {
			return CommandResult{Err: "already_exists"}
		}
		f.state.ErasureByPath[cmd.Path] = cloneErasureFileInfo(cmd.ErasureInfo)
		return CommandResult{}
	}
	// delete_erasure_info
	delete(f.state.ErasureByPath, cmd.Path)
	return CommandResult{}
}

// opCategory maps operation names to handler categories.
var opCategory = map[string]string{
	"get_metadata":          "metadata",
	"list_children":         "metadata",
	"create_metadata":       "metadata",
	"update_metadata":       "metadata",
	"delete_metadata":       "metadata",
	"rename_metadata":       "metadata",
	"get_link":              "link",
	"create_link":           "link",
	"update_link":           "link",
	"cleanup_expired_links": "link",
	"cleanup_used_links":    "link",
	"create_erasure_info":   "erasure",
	"delete_erasure_info":   "erasure",
}

func (f *fsm) Apply(log *hashiraft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return CommandResult{Err: fmt.Sprintf("invalid_command:%v", err)}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	switch opCategory[cmd.Op] {
	case "metadata":
		return f.applyMetadataOp(&cmd)
	case "link":
		return f.applyLinkOp(&cmd)
	case "erasure":
		return f.applyErasureOp(&cmd)
	default:
		return CommandResult{Err: "unknown_operation"}
	}
}

func (f *fsm) Snapshot() (hashiraft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return &stateSnapshot{state: state{
		MetadataByPath: cloneMetadataMap(f.state.MetadataByPath),
		LinksByToken:   cloneLinkMap(f.state.LinksByToken),
		ErasureByPath:  cloneErasureMap(f.state.ErasureByPath),
	}}, nil
}

func (f *fsm) Restore(rc io.ReadCloser) error {
	defer rc.Close()
	var restored state
	if err := json.NewDecoder(rc).Decode(&restored); err != nil {
		return fmt.Errorf("failed to decode raft snapshot: %w", err)
	}
	if restored.MetadataByPath == nil {
		restored.MetadataByPath = map[string]*metadata.Metadata{}
	}
	if restored.LinksByToken == nil {
		restored.LinksByToken = map[string]*metadata.SingleUseLink{}
	}
	if restored.ErasureByPath == nil {
		restored.ErasureByPath = map[string]*metadata.ErasureFileInfo{}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = state{
		MetadataByPath: cloneMetadataMap(restored.MetadataByPath),
		LinksByToken:   cloneLinkMap(restored.LinksByToken),
		ErasureByPath:  cloneErasureMap(restored.ErasureByPath),
	}
	// The index is derived, not restored: state was just replaced wholesale.
	f.rebuildChildIndex()
	return nil
}

func (s *stateSnapshot) Persist(sink hashiraft.SnapshotSink) error {
	if err := json.NewEncoder(sink).Encode(s.state); err != nil {
		sink.Cancel()
		return err
	}
	return sink.Close()
}

func (s *stateSnapshot) Release() {}

func cloneMetadata(in *metadata.Metadata) *metadata.Metadata {
	if in == nil {
		return nil
	}
	out := *in
	out.ParentID = cloneInt64Ptr(in.ParentID)
	out.CallFSInstanceID = cloneStringPtr(in.CallFSInstanceID)
	out.SymlinkTarget = cloneStringPtr(in.SymlinkTarget)
	return &out
}

func cloneLink(in *metadata.SingleUseLink) *metadata.SingleUseLink {
	if in == nil {
		return nil
	}
	out := *in
	out.UsedAt = cloneTimePtr(in.UsedAt)
	out.UsedByIP = cloneStringPtr(in.UsedByIP)
	return &out
}

func cloneMetadataMap(in map[string]*metadata.Metadata) map[string]*metadata.Metadata {
	out := make(map[string]*metadata.Metadata, len(in))
	for k, v := range in {
		out[k] = cloneMetadata(v)
	}
	return out
}

func cloneLinkMap(in map[string]*metadata.SingleUseLink) map[string]*metadata.SingleUseLink {
	out := make(map[string]*metadata.SingleUseLink, len(in))
	for k, v := range in {
		out[k] = cloneLink(v)
	}
	return out
}

func cloneStringPtr(in *string) *string {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func cloneInt64Ptr(in *int64) *int64 {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func cloneTimePtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

// pathDir returns the parent directory using forward-slash separator (not filepath.Dir
// which uses OS-specific separators and breaks on Windows for Unix-style paths).
func pathDir(p string) string {
	if p == "/" {
		return "/"
	}
	lastSlash := strings.LastIndex(p, "/")
	if lastSlash <= 0 {
		return "/"
	}
	return p[:lastSlash]
}

func pathBaseRaft(p string) string {
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		return p[idx+1:]
	}
	return p
}

func copyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
