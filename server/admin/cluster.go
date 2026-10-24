package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/metadata/fsm"
	"github.com/ebogdum/callfs/v2/metadata/natsmeta"
	"github.com/ebogdum/callfs/v2/server/handlers"
)

const (
	clusterOfflineAfter      = 10 * time.Second
	clusterPeerRemoveWait    = 10 * time.Second
	clusterPeerRemoveSubject = "$JS.API.STREAM.PEER.REMOVE."
	clusterModeNATS          = "nats"
	clusterModeSingle        = "single"
	clusterStateLeader       = "leader"
	clusterStateFollower     = "follower"
	clusterStateOffline      = "offline"
	clusterPeerNotMemberMsg  = "not a member"
)

var clusterStreamPrefixes = []string{"CALLFS_", "KV_CALLFS_", "OBJ_CALLFS_"}

type ClusterMetadataLog interface {
	LogInfo(ctx context.Context) (natsmeta.LogInfo, error)
	TakeSnapshot(ctx context.Context) (natsmeta.SnapshotInfo, error)
	Snapshots(ctx context.Context) ([]natsmeta.SnapshotInfo, error)
	RetainSnapshots() int
	StepDown(ctx context.Context, preferred string) (string, error)
	StateCounts() fsm.StateCounts
	AppliedSeq() uint64
}

type clusterSnapshotView struct {
	ID        string `json:"id"`
	Seq       uint64 `json:"seq"`
	Bytes     int64  `json:"bytes"`
	CreatedAt string `json:"created_at"`
}

type clusterLogView struct {
	FirstSeq     uint64               `json:"first_seq"`
	LastSeq      uint64               `json:"last_seq"`
	Messages     uint64               `json:"messages"`
	Bytes        uint64               `json:"bytes"`
	AppliedSeq   uint64               `json:"applied_seq"`
	LastSnapshot *clusterSnapshotView `json:"last_snapshot"`

	SnapshotIntervalMs int64 `json:"snapshot_interval_ms"`
}

type clusterMemberView struct {
	NodeID      string  `json:"node_id"`
	InstanceID  *string `json:"instance_id"`
	APIEndpoint *string `json:"api_endpoint"`
	State       string  `json:"state"`
	IsSelf      bool    `json:"is_self"`
	Online      bool    `json:"online"`
	LastSeenMs  *int64  `json:"last_seen_ms"`
	BehindBy    *uint64 `json:"behind_by"`
}

type clusterInfoView struct {
	Mode           string              `json:"mode"`
	SelfID         string              `json:"self_id"`
	ClusterName    string              `json:"cluster_name"`
	LeaderID       *string             `json:"leader_id"`
	LeaderEndpoint *string             `json:"leader_endpoint"`
	Replicas       int                 `json:"replicas"`
	HasQuorum      bool                `json:"has_quorum"`
	CanLose        int                 `json:"can_lose"`
	Log            clusterLogView      `json:"log"`
	Members        []clusterMemberView `json:"members"`
}

type clusterLinkCounts struct {
	Active  int `json:"active"`
	Used    int `json:"used"`
	Expired int `json:"expired"`
	Revoked int `json:"revoked"`
}

type clusterStateStats struct {
	Files          int               `json:"files"`
	Directories    int               `json:"directories"`
	Links          clusterLinkCounts `json:"links"`
	ErasureEntries int               `json:"erasure_entries"`
	IndexEntries   int               `json:"index_entries"`
}

type leadershipRequest struct {
	NodeID *string `json:"node_id"`
}

type leadershipResponse struct {
	Status   string  `json:"status"`
	LeaderID *string `json:"leader_id"`
}

type nodeRemovedResponse struct {
	Status    string `json:"status"`
	Remaining int    `json:"remaining"`
}

type snapshotsResponse struct {
	Retain int                   `json:"retain"`
	Items  []clusterSnapshotView `json:"items"`
}

type snapshotTakenResponse struct {
	ID     string `json:"id"`
	Seq    uint64 `json:"seq"`
	Bytes  int64  `json:"bytes"`
	TookMs int64  `json:"took_ms"`
}

type joinConfigResponse struct {
	YAML  string   `json:"yaml"`
	Steps []string `json:"steps"`
}

func (a *API) clusterSelfID() string {
	cfg := a.deps.Config
	if nil == cfg {
		return ""
	}
	if "" != cfg.NATS.ServerName {
		return cfg.NATS.ServerName
	}
	return cfg.InstanceDiscovery.InstanceID
}

func (a *API) clusterName() string {
	if "" != a.deps.ClusterName {
		return a.deps.ClusterName
	}
	if nil == a.deps.Config {
		return ""
	}
	return a.deps.Config.NATS.ClusterName
}

func (a *API) clusterEndpointOf(nodeID string) *string {
	cfg := a.deps.Config
	if nil == cfg {
		return nil
	}
	if nodeID == a.clusterSelfID() {
		if "" != cfg.Server.ExternalURL {
			endpoint := cfg.Server.ExternalURL
			return &endpoint
		}
		return nil
	}
	if endpoint, found := cfg.InstanceDiscovery.PeerEndpoints[nodeID]; found {
		return &endpoint
	}
	return nil
}

func (a *API) clusterInstanceOf(nodeID string) *string {
	cfg := a.deps.Config
	if nil == cfg {
		return nil
	}
	if nodeID == a.clusterSelfID() && "" != cfg.InstanceDiscovery.InstanceID {
		instanceID := cfg.InstanceDiscovery.InstanceID
		return &instanceID
	}
	if _, found := cfg.InstanceDiscovery.PeerEndpoints[nodeID]; found {
		instanceID := nodeID
		return &instanceID
	}
	return nil
}

func clusterReplicaOffline(replica natsmeta.ReplicaInfo) bool {
	return replica.Offline || (!replica.Current && clusterOfflineAfter < replica.Active)
}

func clusterSnapshotOf(info natsmeta.SnapshotInfo) clusterSnapshotView {
	return clusterSnapshotView{
		ID:        info.ID,
		Seq:       info.Seq,
		Bytes:     info.Bytes,
		CreatedAt: info.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (a *API) clusterMembers(info natsmeta.LogInfo) []clusterMemberView {
	selfID := a.clusterSelfID()
	members := make([]clusterMemberView, 0, 1+len(info.Replicas))
	if "" != info.Leader {
		var seen int64
		var behind uint64
		members = append(members, clusterMemberView{
			NodeID:      info.Leader,
			InstanceID:  a.clusterInstanceOf(info.Leader),
			APIEndpoint: a.clusterEndpointOf(info.Leader),
			State:       clusterStateLeader,
			IsSelf:      info.Leader == selfID,
			Online:      true,
			LastSeenMs:  &seen,
			BehindBy:    &behind,
		})
	}
	for _, replica := range info.Replicas {
		if info.Leader == replica.Name {
			continue
		}
		offline := clusterReplicaOffline(replica)
		state := clusterStateFollower
		if offline {
			state = clusterStateOffline
		}
		seen := replica.Active.Milliseconds()
		behind := replica.Lag
		members = append(members, clusterMemberView{
			NodeID:      replica.Name,
			InstanceID:  a.clusterInstanceOf(replica.Name),
			APIEndpoint: a.clusterEndpointOf(replica.Name),
			State:       state,
			IsSelf:      replica.Name == selfID,
			Online:      !offline,
			LastSeenMs:  &seen,
			BehindBy:    &behind,
		})
	}
	return members
}

func (a *API) singleClusterInfo() clusterInfoView {
	selfID := a.clusterSelfID()
	var seen int64
	var behind uint64
	return clusterInfoView{
		Mode:           clusterModeSingle,
		SelfID:         selfID,
		ClusterName:    a.clusterName(),
		LeaderID:       &selfID,
		LeaderEndpoint: a.clusterEndpointOf(selfID),
		Replicas:       1,
		HasQuorum:      true,
		Members: []clusterMemberView{{
			NodeID:      selfID,
			InstanceID:  a.clusterInstanceOf(selfID),
			APIEndpoint: a.clusterEndpointOf(selfID),
			State:       clusterStateLeader,
			IsSelf:      true,
			Online:      true,
			LastSeenMs:  &seen,
			BehindBy:    &behind,
		}},
	}
}

func (a *API) conflictSingle(w http.ResponseWriter) {
	writeAdminError(w, http.StatusConflict, "CONFLICT", "this server runs without a NATS metadata cluster; enable nats and set metadata_store.type to nats", nil)
}

func (a *API) Cluster(w http.ResponseWriter, r *http.Request) {
	logger := a.logger()
	clusterLog := a.deps.ClusterLog
	if nil == clusterLog {
		handlers.SendJSONResponse(w, a.singleClusterInfo())
		return
	}
	info, err := clusterLog.LogInfo(r.Context())
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	members := a.clusterMembers(info)
	offline := 0
	for _, member := range members {
		if !member.Online {
			offline++
		}
	}
	replicas := len(members)
	canLose := (replicas-1)/2 - offline
	if 0 > canLose {
		canLose = 0
	}
	view := clusterInfoView{
		Mode:        clusterModeNATS,
		SelfID:      a.clusterSelfID(),
		ClusterName: a.clusterName(),
		Replicas:    replicas,
		HasQuorum:   "" != info.Leader && replicas < 2*(replicas-offline),
		CanLose:     canLose,
		Log: clusterLogView{
			FirstSeq:   info.FirstSeq,
			LastSeq:    info.LastSeq,
			Messages:   info.Messages,
			Bytes:      info.Bytes,
			AppliedSeq: clusterLog.AppliedSeq(),
		},
		Members: members,
	}
	if nil != a.deps.Config {
		view.Log.SnapshotIntervalMs = a.deps.Config.NATS.Metadata.SnapshotInterval.Milliseconds()
	}
	if "" != info.Leader {
		leader := info.Leader
		view.LeaderID = &leader
		view.LeaderEndpoint = a.clusterEndpointOf(leader)
	}
	if nil != info.LastSnapshot {
		snapshot := clusterSnapshotOf(*info.LastSnapshot)
		view.Log.LastSnapshot = &snapshot
	}
	handlers.SendJSONResponse(w, view)
}

func (a *API) ClusterStateStats(w http.ResponseWriter, _ *http.Request) {
	clusterLog := a.deps.ClusterLog
	if nil == clusterLog {
		a.conflictSingle(w)
		return
	}
	counts := clusterLog.StateCounts()
	handlers.SendJSONResponse(w, clusterStateStats{
		Files:       counts.Files,
		Directories: counts.Directories,
		Links: clusterLinkCounts{
			Active:  counts.LinksActive,
			Used:    counts.LinksUsed,
			Expired: counts.LinksExpired,
			Revoked: counts.LinksRevoked,
		},
		ErasureEntries: counts.ErasureEntries,
		IndexEntries:   counts.IndexEntries,
	})
}

func (a *API) ClusterLeadershipTransfer(w http.ResponseWriter, r *http.Request) {
	logger := a.logger()
	clusterLog := a.deps.ClusterLog
	if nil == clusterLog {
		a.conflictSingle(w)
		return
	}
	var req leadershipRequest
	if err := decodeAdminBody(w, r, &req); nil != err && !errors.Is(err, io.EOF) {
		respondBodyError(w, logger, err)
		return
	}
	preferred := ""
	if nil != req.NodeID {
		preferred = strings.TrimSpace(*req.NodeID)
		if "" == preferred {
			writeFieldProblems(w, []fieldProblem{{Path: "node_id", Message: "node_id must not be empty"}})
			return
		}
		info, err := clusterLog.LogInfo(r.Context())
		if nil != err {
			handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
		members := a.clusterMembers(info)
		index := slices.IndexFunc(members, func(member clusterMemberView) bool { return preferred == member.NodeID })
		if 0 > index {
			writeAdminError(w, http.StatusNotFound, "NODE_NOT_FOUND", "node "+preferred+" is not a member of the cluster", nil)
			return
		}
		if !members[index].Online {
			writeAdminError(w, http.StatusConflict, "CONFLICT", "node "+preferred+" is offline and cannot take leadership", nil)
			return
		}
	}
	leader, err := clusterLog.StepDown(r.Context(), preferred)
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	logger.Info("Cluster leadership transferred", zap.String("requested", preferred), zap.String("leader_id", leader))
	response := leadershipResponse{Status: "transferred"}
	if "" != leader {
		response.LeaderID = &leader
	}
	handlers.SendJSONResponse(w, response)
}

func (a *API) ClusterRemoveNode(w http.ResponseWriter, r *http.Request) {
	logger := a.logger()
	clusterLog := a.deps.ClusterLog
	if nil == clusterLog {
		a.conflictSingle(w)
		return
	}
	nodeID := chi.URLParam(r, "id")
	info, err := clusterLog.LogInfo(r.Context())
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	members := a.clusterMembers(info)
	index := slices.IndexFunc(members, func(member clusterMemberView) bool { return nodeID == member.NodeID })
	if 0 > index {
		writeAdminError(w, http.StatusNotFound, "NODE_NOT_FOUND", "node "+nodeID+" is not a member of the cluster", nil)
		return
	}
	if members[index].IsSelf {
		writeAdminError(w, http.StatusConflict, "CONFLICT", "a node cannot remove itself from the cluster", nil)
		return
	}
	if members[index].Online {
		writeAdminError(w, http.StatusConflict, "CONFLICT", "node "+nodeID+" is online; stop it before removing it", nil)
		return
	}
	remove := a.removePeer
	if nil == remove {
		if nil == a.deps.NATS || nil == a.deps.JetStream {
			a.unavailable(w, "NATS connection")
			return
		}
		remove = a.removePeerFromStreams
	}
	if err := remove(r.Context(), nodeID); nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	if nil != a.deps.Engine {
		a.deps.Engine.RemovePeerEndpoint(nodeID)
	}
	logger.Info("Cluster node removed", zap.String("node_id", nodeID))
	handlers.SendJSONResponse(w, nodeRemovedResponse{Status: "removed", Remaining: len(members) - 1})
}

func clusterStreamOwned(name string) bool {
	for _, prefix := range clusterStreamPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func (a *API) removePeerFromStreams(ctx context.Context, peer string) error {
	ctx, cancel := context.WithTimeout(ctx, clusterPeerRemoveWait)
	defer cancel()
	body, err := json.Marshal(map[string]string{"peer": peer})
	if nil != err {
		return err
	}
	names := a.deps.JetStream.StreamNames(ctx)
	var failures []error
	for name := range names.Name() {
		if !clusterStreamOwned(name) {
			continue
		}
		reply, err := a.deps.NATS.RequestWithContext(ctx, clusterPeerRemoveSubject+name, body)
		if nil != err {
			failures = append(failures, fmt.Errorf("%s: %w", name, err))
			continue
		}
		if err := clusterPeerRemoveReply(reply); nil != err {
			failures = append(failures, fmt.Errorf("%s: %w", name, err))
		}
	}
	if err := names.Err(); nil != err {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func clusterPeerRemoveReply(reply *nats.Msg) error {
	var parsed struct {
		Error *struct {
			Description string `json:"description"`
		} `json:"error"`
	}
	if err := json.Unmarshal(reply.Data, &parsed); nil != err {
		return err
	}
	if nil == parsed.Error || strings.Contains(parsed.Error.Description, clusterPeerNotMemberMsg) {
		return nil
	}
	return errors.New(parsed.Error.Description)
}

func (a *API) ClusterSnapshots(w http.ResponseWriter, r *http.Request) {
	logger := a.logger()
	clusterLog := a.deps.ClusterLog
	if nil == clusterLog {
		handlers.SendJSONResponse(w, snapshotsResponse{Items: []clusterSnapshotView{}})
		return
	}
	snapshots, err := clusterLog.Snapshots(r.Context())
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	items := make([]clusterSnapshotView, 0, len(snapshots))
	for _, snapshot := range snapshots {
		items = append(items, clusterSnapshotOf(snapshot))
	}
	handlers.SendJSONResponse(w, snapshotsResponse{Retain: clusterLog.RetainSnapshots(), Items: items})
}

func (a *API) ClusterTakeSnapshot(w http.ResponseWriter, r *http.Request) {
	logger := a.logger()
	clusterLog := a.deps.ClusterLog
	if nil == clusterLog {
		a.conflictSingle(w)
		return
	}
	snapshot, err := clusterLog.TakeSnapshot(r.Context())
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	logger.Info("Cluster snapshot taken", zap.String("id", snapshot.ID), zap.Uint64("seq", snapshot.Seq))
	handlers.SendJSONResponse(w, snapshotTakenResponse{
		ID:     snapshot.ID,
		Seq:    snapshot.Seq,
		Bytes:  snapshot.Bytes,
		TookMs: snapshot.Took.Milliseconds(),
	})
}

func (a *API) ClusterJoinConfig(w http.ResponseWriter, r *http.Request) {
	logger := a.logger()
	clusterLog := a.deps.ClusterLog
	cfg := a.deps.Config
	if nil == clusterLog || nil == cfg {
		a.conflictSingle(w)
		return
	}
	query := r.URL.Query()
	nodeID := strings.TrimSpace(query.Get("node_id"))
	apiEndpoint := strings.TrimSpace(query.Get("api_endpoint"))
	clusterAddr := strings.TrimSpace(query.Get("cluster_addr"))
	var problems []fieldProblem
	if !instanceIDPattern.MatchString(nodeID) {
		problems = append(problems, fieldProblem{Path: "node_id", Message: "node_id must be 1 to 128 letters, digits, dot, dash or underscore"})
	}
	if parsed, err := url.Parse(apiEndpoint); nil != err || ("https" != parsed.Scheme && "http" != parsed.Scheme) || "" == parsed.Hostname() {
		problems = append(problems, fieldProblem{Path: "api_endpoint", Message: "api_endpoint must be an http or https URL with a host"})
	}
	if host, port, err := net.SplitHostPort(clusterAddr); nil != err || "" == host || "" == port {
		problems = append(problems, fieldProblem{Path: "cluster_addr", Message: "cluster_addr must be host:port"})
	}
	if 0 < len(problems) {
		writeFieldProblems(w, problems)
		return
	}
	info, err := clusterLog.LogInfo(r.Context())
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	members := a.clusterMembers(info)
	if slices.ContainsFunc(members, func(member clusterMemberView) bool { return nodeID == member.NodeID }) {
		writeAdminError(w, http.StatusConflict, "CONFLICT", "node "+nodeID+" is already a member of the cluster", nil)
		return
	}
	handlers.SendJSONResponse(w, a.buildJoinConfig(members, nodeID, apiEndpoint, clusterAddr))
}

func (a *API) buildJoinConfig(members []clusterMemberView, nodeID, apiEndpoint, clusterAddr string) joinConfigResponse {
	cfg := a.deps.Config
	_, clusterPort, _ := net.SplitHostPort(cfg.NATS.ClusterListen)
	peers := make(map[string]string, len(members)+1)
	order := make([]string, 0, len(members)+1)
	routes := slices.Clone(cfg.NATS.Routes)
	for _, member := range members {
		if nil == member.APIEndpoint {
			continue
		}
		peers[member.NodeID] = *member.APIEndpoint
		order = append(order, member.NodeID)
		if "" == clusterPort {
			continue
		}
		if parsed, err := url.Parse(*member.APIEndpoint); nil == err && "" != parsed.Hostname() {
			routes = append(routes, "nats-route://"+net.JoinHostPort(parsed.Hostname(), clusterPort))
		}
	}
	peers[nodeID] = apiEndpoint
	order = append(order, nodeID)
	routes = append(routes, "nats-route://"+clusterAddr)
	slices.Sort(routes)
	routes = slices.Compact(routes)

	var b strings.Builder
	b.WriteString("nats:\n")
	b.WriteString("  enabled: true\n")
	b.WriteString("  mode: " + strconv.Quote(clusterModeValue(cfg.NATS.Mode)) + "\n")
	b.WriteString("  server_name: " + strconv.Quote(nodeID) + "\n")
	b.WriteString("  cluster_name: " + strconv.Quote(a.clusterName()) + "\n")
	b.WriteString("  cluster_listen: " + strconv.Quote(clusterAddr) + "\n")
	b.WriteString("  routes:\n")
	for _, route := range routes {
		b.WriteString("    - " + strconv.Quote(route) + "\n")
	}
	b.WriteString("instance_discovery:\n")
	b.WriteString("  instance_id: " + strconv.Quote(nodeID) + "\n")
	b.WriteString("  peer_endpoints:\n")
	for _, id := range order {
		b.WriteString("    " + strconv.Quote(id) + ": " + strconv.Quote(peers[id]) + "\n")
	}
	b.WriteString("metadata_store:\n")
	b.WriteString("  type: \"nats\"\n")
	b.WriteString("dlm:\n")
	b.WriteString("  type: \"nats\"\n")

	return joinConfigResponse{
		YAML: b.String(),
		Steps: []string{
			"Copy auth.internal_proxy_secret, auth.single_use_link_secret and the API keys from an existing node into the new node's config. They are not included here.",
			"Open " + clusterAddr + " between the new node and the existing nodes, and give the cluster port the same TLS files as the other nodes when it is not a loopback address.",
			"Start the new node. It joins the metadata log and the lock bucket and catches up on its own.",
			"Add nats-route://" + clusterAddr + " to nats.routes and " + nodeID + " to instance_discovery.peer_endpoints on the other nodes. They pick it up on their next restart.",
		},
	}
}

func clusterModeValue(mode string) string {
	if "" == mode {
		return "embedded"
	}
	return mode
}
