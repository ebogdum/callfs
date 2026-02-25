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
	Op        string                  `json:"op"`
	Path      string                  `json:"path,omitempty"`
	Metadata  *metadata.Metadata      `json:"metadata,omitempty"`
	Token     string                  `json:"token,omitempty"`
	Link      *metadata.SingleUseLink `json:"link,omitempty"`
	Status    string                  `json:"status,omitempty"`
	UsedAt    *time.Time              `json:"used_at,omitempty"`
	UsedByIP  *string                 `json:"used_by_ip,omitempty"`
	Before    *time.Time              `json:"before,omitempty"`
	OlderThan *time.Time              `json:"older_than,omitempty"`
}

type CommandResult struct {
	CleanupCount int    `json:"cleanup_count,omitempty"`
	Err          string `json:"err,omitempty"`
}

type ForwardApplyRequest struct {
	Command Command `json:"command"`
}

type ForwardApplyResponse struct {
	CleanupCount int    `json:"cleanup_count,omitempty"`
	Error        string `json:"error,omitempty"`
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
	nodeID            string
	apiPeerMu         sync.RWMutex
	apiPeerEndpoints  map[string]string
	internalAuthToken string
	forwardClient     *http.Client
	applyTimeout      time.Duration
	logger            *zap.Logger
}

type state struct {
	MetadataByPath map[string]*metadata.Metadata      `json:"metadata_by_path"`
	LinksByToken   map[string]*metadata.SingleUseLink `json:"links_by_token"`
}

type fsm struct {
	mu    sync.RWMutex
	state state
}

type stateSnapshot struct {
	state state
}

func NewRaftStore(cfg Config, logger *zap.Logger) (*Store, error) {
	if cfg.NodeID == "" {
		return nil, fmt.Errorf("raft node id is required")
	}
	if cfg.BindAddr == "" {
		return nil, fmt.Errorf("raft bind address is required")
	}
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("raft data_dir is required")
	}
	if cfg.ApplyTimeout <= 0 {
		cfg.ApplyTimeout = 10 * time.Second
	}
	if cfg.ForwardTimeout <= 0 {
		cfg.ForwardTimeout = 10 * time.Second
	}
	if cfg.SnapshotInterval <= 0 {
		cfg.SnapshotInterval = 60 * time.Second
	}
	if cfg.SnapshotThreshold == 0 {
		cfg.SnapshotThreshold = 256
	}
	if cfg.RetainSnapshotCount <= 0 {
		cfg.RetainSnapshotCount = 2
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create raft data dir: %w", err)
	}

	fsmInstance := &fsm{state: state{MetadataByPath: map[string]*metadata.Metadata{}, LinksByToken: map[string]*metadata.SingleUseLink{}}}

	raftCfg := hashiraft.DefaultConfig()
	raftCfg.LocalID = hashiraft.ServerID(cfg.NodeID)
	raftCfg.SnapshotInterval = cfg.SnapshotInterval
	raftCfg.SnapshotThreshold = cfg.SnapshotThreshold

	logStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.DataDir, "raft-log.db"))
	if err != nil {
		return nil, fmt.Errorf("failed to create raft log store: %w", err)
	}
	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(cfg.DataDir, "raft-stable.db"))
	if err != nil {
		return nil, fmt.Errorf("failed to create raft stable store: %w", err)
	}
	snapshotStore, err := hashiraft.NewFileSnapshotStore(cfg.DataDir, cfg.RetainSnapshotCount, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to create raft snapshot store: %w", err)
	}
	transport, err := hashiraft.NewTCPTransport(cfg.BindAddr, nil, 3, 10*time.Second, os.Stderr)
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
		nodeID:            cfg.NodeID,
		apiPeerEndpoints:  copyStringMap(cfg.APIPeerEndpoints),
		internalAuthToken: cfg.InternalAuthToken,
		forwardClient:     &http.Client{Timeout: cfg.ForwardTimeout},
		applyTimeout:      cfg.ApplyTimeout,
		logger:            logger,
	}

	if cfg.Bootstrap {
		future := raftNode.BootstrapCluster(hashiraft.Configuration{Servers: []hashiraft.Server{
			{ID: hashiraft.ServerID(cfg.NodeID), Address: hashiraft.ServerAddress(cfg.BindAddr), Suffrage: hashiraft.Voter},
		}})
		if err := future.Error(); err != nil && !errors.Is(err, hashiraft.ErrCantBootstrap) {
			return nil, fmt.Errorf("failed to bootstrap raft cluster: %w", err)
		}
	}

	return store, nil
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

	for _, server := range configFuture.Configuration().Servers {
		serverID := string(server.ID)
		serverAddr := string(server.Address)

		if serverID == nodeID {
			if serverAddr == raftAddr && server.Suffrage == hashiraft.Voter {
				s.SetAPIPeerEndpoint(nodeID, apiEndpoint)
				return nil
			}

			removeFuture := s.raft.RemoveServer(server.ID, 0, s.applyTimeout)
			if err := removeFuture.Error(); err != nil {
				return fmt.Errorf("failed to remove existing node %s before rejoin: %w", nodeID, err)
			}
		}

		if serverAddr == raftAddr && serverID != nodeID {
			removeFuture := s.raft.RemoveServer(server.ID, 0, s.applyTimeout)
			if err := removeFuture.Error(); err != nil {
				return fmt.Errorf("failed to remove stale node %s on address %s: %w", serverID, raftAddr, err)
			}
		}
	}

	addFuture := s.raft.AddVoter(hashiraft.ServerID(nodeID), hashiraft.ServerAddress(raftAddr), 0, s.applyTimeout)
	if err := addFuture.Error(); err != nil {
		return fmt.Errorf("failed to add raft voter %s: %w", nodeID, err)
	}

	s.SetAPIPeerEndpoint(nodeID, apiEndpoint)

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (s *Store) Get(ctx context.Context, path string) (*metadata.Metadata, error) {
	s.fsm.mu.RLock()
	defer s.fsm.mu.RUnlock()
	md, ok := s.fsm.state.MetadataByPath[path]
	if !ok {
		return nil, metadata.ErrNotFound
	}
	return cloneMetadata(md), nil
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

func (s *Store) ListChildren(ctx context.Context, parentPath string) ([]*metadata.Metadata, error) {
	s.fsm.mu.RLock()
	defer s.fsm.mu.RUnlock()
	children := make([]*metadata.Metadata, 0)
	for _, md := range s.fsm.state.MetadataByPath {
		if md.Path == "/" {
			continue
		}
		if filepath.Dir(md.Path) == parentPath {
			children = append(children, cloneMetadata(md))
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Path < children[j].Path })
	return children, nil
}

func (s *Store) GetSingleUseLink(ctx context.Context, token string) (*metadata.SingleUseLink, error) {
	s.fsm.mu.RLock()
	defer s.fsm.mu.RUnlock()
	link, ok := s.fsm.state.LinksByToken[token]
	if !ok {
		return nil, metadata.ErrNotFound
	}
	return cloneLink(link), nil
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

func (s *Store) Close() error {
	f := s.raft.Shutdown()
	if err := f.Error(); err != nil {
		return fmt.Errorf("failed to shutdown raft: %w", err)
	}
	return nil
}

func (s *Store) ApplyForwardedCommand(ctx context.Context, cmd Command) (CommandResult, error) {
	if !s.IsLeader() {
		return CommandResult{}, fmt.Errorf("not leader")
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
		payload, _ := io.ReadAll(resp.Body)
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
	return CommandResult{CleanupCount: applyResp.CleanupCount}, nil
}

func (f *fsm) Apply(log *hashiraft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return CommandResult{Err: fmt.Sprintf("invalid_command:%v", err)}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	switch cmd.Op {
	case "create_metadata":
		if cmd.Metadata == nil {
			return CommandResult{Err: "metadata_required"}
		}
		if _, exists := f.state.MetadataByPath[cmd.Metadata.Path]; exists {
			return CommandResult{Err: "already_exists"}
		}
		f.state.MetadataByPath[cmd.Metadata.Path] = cloneMetadata(cmd.Metadata)
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
		return CommandResult{}
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
		if !exists {
			return CommandResult{Err: "not_found"}
		}
		link.Status = cmd.Status
		link.UsedAt = cloneTimePtr(cmd.UsedAt)
		link.UsedByIP = cloneStringPtr(cmd.UsedByIP)
		link.UpdatedAt = time.Now().UTC()
		f.state.LinksByToken[cmd.Token] = link
		return CommandResult{}
	case "cleanup_expired_links":
		if cmd.Before == nil {
			return CommandResult{Err: "before_required"}
		}
		count := 0
		for token, link := range f.state.LinksByToken {
			if link.ExpiresAt.Before(*cmd.Before) {
				delete(f.state.LinksByToken, token)
				count++
			}
		}
		return CommandResult{CleanupCount: count}
	case "cleanup_used_links":
		if cmd.OlderThan == nil {
			return CommandResult{Err: "older_than_required"}
		}
		count := 0
		for token, link := range f.state.LinksByToken {
			if link.Status == "used" && link.UsedAt != nil && link.UsedAt.Before(*cmd.OlderThan) {
				delete(f.state.LinksByToken, token)
				count++
			}
		}
		return CommandResult{CleanupCount: count}
	default:
		return CommandResult{Err: "unknown_operation"}
	}
}

func (f *fsm) Snapshot() (hashiraft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return &stateSnapshot{state: state{MetadataByPath: cloneMetadataMap(f.state.MetadataByPath), LinksByToken: cloneLinkMap(f.state.LinksByToken)}}, nil
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
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = state{MetadataByPath: cloneMetadataMap(restored.MetadataByPath), LinksByToken: cloneLinkMap(restored.LinksByToken)}
	return nil
}

func (s *stateSnapshot) Persist(sink hashiraft.SnapshotSink) error {
	defer sink.Close()
	if err := json.NewEncoder(sink).Encode(s.state); err != nil {
		sink.Cancel()
		return err
	}
	return nil
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

func copyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
