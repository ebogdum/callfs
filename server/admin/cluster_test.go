package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nats-io/nats.go"

	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/metadata/fsm"
	"github.com/ebogdum/callfs/v2/metadata/natsmeta"
)

type fakeClusterLog struct {
	info        natsmeta.LogInfo
	infoErr     error
	snapshot    natsmeta.SnapshotInfo
	snapshotErr error
	snapshots   []natsmeta.SnapshotInfo
	listErr     error
	retain      int
	stepLeader  string
	stepErr     error
	stepPrefer  string
	counts      fsm.StateCounts
	applied     uint64
}

func (f *fakeClusterLog) LogInfo(context.Context) (natsmeta.LogInfo, error) {
	return f.info, f.infoErr
}

func (f *fakeClusterLog) TakeSnapshot(context.Context) (natsmeta.SnapshotInfo, error) {
	return f.snapshot, f.snapshotErr
}

func (f *fakeClusterLog) Snapshots(context.Context) ([]natsmeta.SnapshotInfo, error) {
	return f.snapshots, f.listErr
}

func (f *fakeClusterLog) RetainSnapshots() int { return f.retain }

func (f *fakeClusterLog) StepDown(_ context.Context, preferred string) (string, error) {
	f.stepPrefer = preferred
	return f.stepLeader, f.stepErr
}

func (f *fakeClusterLog) StateCounts() fsm.StateCounts { return f.counts }

func (f *fakeClusterLog) AppliedSeq() uint64 { return f.applied }

func clusterTestConfig() *config.AppConfig {
	cfg := &config.AppConfig{}
	cfg.NATS.ServerName = "node-1"
	cfg.NATS.ClusterName = "callfs"
	cfg.NATS.ClusterListen = "10.0.0.1:6222"
	cfg.NATS.Mode = "embedded"
	cfg.Server.ExternalURL = "https://10.0.0.1:8443"
	cfg.InstanceDiscovery.InstanceID = "node-1"
	cfg.InstanceDiscovery.PeerEndpoints = map[string]string{
		"node-2": "https://10.0.0.2:8443",
		"node-3": "https://10.0.0.3:8443",
	}
	return cfg
}

func clusterTestInfo() natsmeta.LogInfo {
	return natsmeta.LogInfo{
		FirstSeq:   1,
		LastSeq:    9120,
		Messages:   9120,
		Bytes:      812345,
		AppliedSeq: 9120,
		Leader:     "node-2",
		Replicas: []natsmeta.ReplicaInfo{
			{Name: "node-1", Current: true, Active: 120 * time.Millisecond, Lag: 3},
			{Name: "node-3", Current: false, Offline: true, Active: time.Minute, Lag: 500},
		},
		LastSnapshot: &natsmeta.SnapshotInfo{ID: "snap-9000", Seq: 9000, Bytes: 40213, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
	}
}

func newClusterTestHandler(fake *fakeClusterLog, remove func(context.Context, string) error) http.Handler {
	deps := Deps{Config: clusterTestConfig(), ClusterName: "callfs"}
	if nil != fake {
		deps.ClusterLog = fake
	}
	api := NewAPI(deps)
	api.removePeer = remove
	router := chi.NewRouter()
	router.Get("/cluster", api.Cluster)
	router.Get("/cluster/state-stats", api.ClusterStateStats)
	router.Post("/cluster/leadership-transfer", api.ClusterLeadershipTransfer)
	router.Delete("/cluster/nodes/{id}", api.ClusterRemoveNode)
	router.Get("/cluster/snapshots", api.ClusterSnapshots)
	router.Post("/cluster/snapshot", api.ClusterTakeSnapshot)
	router.Get("/cluster/join-config", api.ClusterJoinConfig)
	return router
}

func doClusterRequest(handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeCluster[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); nil != err {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return out
}

func TestClusterInfoNATS(t *testing.T) {
	fake := &fakeClusterLog{info: clusterTestInfo(), applied: 9118}
	rec := doClusterRequest(newClusterTestHandler(fake, nil), http.MethodGet, "/cluster", "")
	if http.StatusOK != rec.Code {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	view := decodeCluster[clusterInfoView](t, rec)
	if clusterModeNATS != view.Mode || "node-1" != view.SelfID || "callfs" != view.ClusterName {
		t.Fatalf("header fields %+v", view)
	}
	if nil == view.LeaderID || "node-2" != *view.LeaderID || nil == view.LeaderEndpoint || "https://10.0.0.2:8443" != *view.LeaderEndpoint {
		t.Fatalf("leader %v %v", view.LeaderID, view.LeaderEndpoint)
	}
	if 3 != view.Replicas || !view.HasQuorum || 0 != view.CanLose {
		t.Fatalf("replicas %d quorum %v can_lose %d", view.Replicas, view.HasQuorum, view.CanLose)
	}
	if 9118 != view.Log.AppliedSeq || 9120 != view.Log.LastSeq || nil == view.Log.LastSnapshot || "snap-9000" != view.Log.LastSnapshot.ID || "2026-01-02T03:04:05Z" != view.Log.LastSnapshot.CreatedAt {
		t.Fatalf("log %+v", view.Log)
	}
	if 3 != len(view.Members) {
		t.Fatalf("members %d", len(view.Members))
	}
	leader, self, down := view.Members[0], view.Members[1], view.Members[2]
	if clusterStateLeader != leader.State || !leader.Online || leader.IsSelf {
		t.Fatalf("leader member %+v", leader)
	}
	if clusterStateFollower != self.State || !self.IsSelf || nil == self.LastSeenMs || 120 != *self.LastSeenMs || nil == self.BehindBy || 3 != *self.BehindBy {
		t.Fatalf("self member %+v", self)
	}
	if nil == self.InstanceID || "node-1" != *self.InstanceID || nil == self.APIEndpoint || "https://10.0.0.1:8443" != *self.APIEndpoint {
		t.Fatalf("self identity %+v", self)
	}
	if clusterStateOffline != down.State || down.Online || nil == down.BehindBy || 500 != *down.BehindBy {
		t.Fatalf("offline member %+v", down)
	}
}

func TestClusterInfoOfflineByActivity(t *testing.T) {
	info := clusterTestInfo()
	info.Replicas = []natsmeta.ReplicaInfo{
		{Name: "node-1", Current: true},
		{Name: "node-3", Current: false, Active: 11 * time.Second},
	}
	rec := doClusterRequest(newClusterTestHandler(&fakeClusterLog{info: info}, nil), http.MethodGet, "/cluster", "")
	view := decodeCluster[clusterInfoView](t, rec)
	if view.Members[2].Online || 0 != view.CanLose || !view.HasQuorum {
		t.Fatalf("members %+v can_lose %d quorum %v", view.Members, view.CanLose, view.HasQuorum)
	}
}

func TestClusterInfoNoLeaderNoQuorum(t *testing.T) {
	info := clusterTestInfo()
	info.Leader = ""
	info.LastSnapshot = nil
	rec := doClusterRequest(newClusterTestHandler(&fakeClusterLog{info: info}, nil), http.MethodGet, "/cluster", "")
	view := decodeCluster[clusterInfoView](t, rec)
	if nil != view.LeaderID || nil != view.LeaderEndpoint || view.HasQuorum || nil != view.Log.LastSnapshot {
		t.Fatalf("view %+v", view)
	}
}

func TestClusterInfoLogError(t *testing.T) {
	rec := doClusterRequest(newClusterTestHandler(&fakeClusterLog{infoErr: errors.New("boom")}, nil), http.MethodGet, "/cluster", "")
	if http.StatusInternalServerError != rec.Code {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestClusterInfoSingle(t *testing.T) {
	rec := doClusterRequest(newClusterTestHandler(nil, nil), http.MethodGet, "/cluster", "")
	if http.StatusOK != rec.Code {
		t.Fatalf("status %d", rec.Code)
	}
	view := decodeCluster[clusterInfoView](t, rec)
	if clusterModeSingle != view.Mode || 1 != view.Replicas || 1 != len(view.Members) || !view.Members[0].IsSelf || nil == view.LeaderID || "node-1" != *view.LeaderID {
		t.Fatalf("view %+v", view)
	}
}

func TestClusterStateStats(t *testing.T) {
	fake := &fakeClusterLog{counts: fsm.StateCounts{Files: 5, Directories: 2, LinksActive: 1, LinksUsed: 2, LinksExpired: 3, LinksRevoked: 4, ErasureEntries: 6, IndexEntries: 7}}
	rec := doClusterRequest(newClusterTestHandler(fake, nil), http.MethodGet, "/cluster/state-stats", "")
	if http.StatusOK != rec.Code {
		t.Fatalf("status %d", rec.Code)
	}
	stats := decodeCluster[clusterStateStats](t, rec)
	want := clusterStateStats{Files: 5, Directories: 2, Links: clusterLinkCounts{Active: 1, Used: 2, Expired: 3, Revoked: 4}, ErasureEntries: 6, IndexEntries: 7}
	if want != stats {
		t.Fatalf("stats %+v", stats)
	}
	if http.StatusConflict != doClusterRequest(newClusterTestHandler(nil, nil), http.MethodGet, "/cluster/state-stats", "").Code {
		t.Fatal("single mode must return 409")
	}
}

func TestClusterLeadershipTransfer(t *testing.T) {
	fake := &fakeClusterLog{info: clusterTestInfo(), stepLeader: "node-1"}
	handler := newClusterTestHandler(fake, nil)
	rec := doClusterRequest(handler, http.MethodPost, "/cluster/leadership-transfer", `{"node_id":"node-1"}`)
	if http.StatusOK != rec.Code {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	response := decodeCluster[leadershipResponse](t, rec)
	if "transferred" != response.Status || nil == response.LeaderID || "node-1" != *response.LeaderID || "node-1" != fake.stepPrefer {
		t.Fatalf("response %+v preferred %q", response, fake.stepPrefer)
	}

	fake.stepPrefer = "x"
	rec = doClusterRequest(handler, http.MethodPost, "/cluster/leadership-transfer", `{"node_id":null}`)
	if http.StatusOK != rec.Code || "" != fake.stepPrefer {
		t.Fatalf("null node status %d preferred %q", rec.Code, fake.stepPrefer)
	}
	rec = doClusterRequest(handler, http.MethodPost, "/cluster/leadership-transfer", "")
	if http.StatusOK != rec.Code {
		t.Fatalf("empty body status %d", rec.Code)
	}
}

func TestClusterLeadershipTransferErrors(t *testing.T) {
	fake := &fakeClusterLog{info: clusterTestInfo(), stepLeader: "node-1"}
	handler := newClusterTestHandler(fake, nil)
	cases := []struct {
		name string
		body string
		code int
	}{
		{"malformed", `{`, http.StatusBadRequest},
		{"blank", `{"node_id":"  "}`, http.StatusUnprocessableEntity},
		{"unknown", `{"node_id":"node-9"}`, http.StatusNotFound},
		{"offline", `{"node_id":"node-3"}`, http.StatusConflict},
	}
	for _, tc := range cases {
		if rec := doClusterRequest(handler, http.MethodPost, "/cluster/leadership-transfer", tc.body); tc.code != rec.Code {
			t.Fatalf("%s status %d: %s", tc.name, rec.Code, rec.Body.String())
		}
	}
	failing := newClusterTestHandler(&fakeClusterLog{info: clusterTestInfo(), stepErr: errors.New("no")}, nil)
	if rec := doClusterRequest(failing, http.MethodPost, "/cluster/leadership-transfer", `{"node_id":"node-1"}`); http.StatusInternalServerError != rec.Code {
		t.Fatalf("step down failure status %d", rec.Code)
	}
	failingInfo := newClusterTestHandler(&fakeClusterLog{infoErr: errors.New("no")}, nil)
	if rec := doClusterRequest(failingInfo, http.MethodPost, "/cluster/leadership-transfer", `{"node_id":"node-1"}`); http.StatusInternalServerError != rec.Code {
		t.Fatalf("log info failure status %d", rec.Code)
	}
	if rec := doClusterRequest(newClusterTestHandler(nil, nil), http.MethodPost, "/cluster/leadership-transfer", `{}`); http.StatusConflict != rec.Code {
		t.Fatalf("single mode status %d", rec.Code)
	}
}

func TestClusterRemoveNode(t *testing.T) {
	var removed string
	remove := func(_ context.Context, peer string) error {
		removed = peer
		return nil
	}
	handler := newClusterTestHandler(&fakeClusterLog{info: clusterTestInfo()}, remove)
	rec := doClusterRequest(handler, http.MethodDelete, "/cluster/nodes/node-3", "")
	if http.StatusOK != rec.Code {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	response := decodeCluster[nodeRemovedResponse](t, rec)
	if "removed" != response.Status || 2 != response.Remaining || "node-3" != removed {
		t.Fatalf("response %+v removed %q", response, removed)
	}
}

func TestClusterRemoveNodeErrors(t *testing.T) {
	called := false
	remove := func(context.Context, string) error {
		called = true
		return nil
	}
	handler := newClusterTestHandler(&fakeClusterLog{info: clusterTestInfo()}, remove)
	cases := []struct {
		name string
		id   string
		code int
	}{
		{"unknown", "node-9", http.StatusNotFound},
		{"self", "node-1", http.StatusConflict},
		{"online", "node-2", http.StatusConflict},
	}
	for _, tc := range cases {
		if rec := doClusterRequest(handler, http.MethodDelete, "/cluster/nodes/"+tc.id, ""); tc.code != rec.Code {
			t.Fatalf("%s status %d: %s", tc.name, rec.Code, rec.Body.String())
		}
	}
	if called {
		t.Fatal("peer removal must not run for rejected requests")
	}

	failing := newClusterTestHandler(&fakeClusterLog{info: clusterTestInfo()}, func(context.Context, string) error { return errors.New("stream failed") })
	if rec := doClusterRequest(failing, http.MethodDelete, "/cluster/nodes/node-3", ""); http.StatusInternalServerError != rec.Code {
		t.Fatalf("removal failure status %d", rec.Code)
	}
	noConn := newClusterTestHandler(&fakeClusterLog{info: clusterTestInfo()}, nil)
	if rec := doClusterRequest(noConn, http.MethodDelete, "/cluster/nodes/node-3", ""); http.StatusServiceUnavailable != rec.Code {
		t.Fatalf("missing connection status %d", rec.Code)
	}
	badInfo := newClusterTestHandler(&fakeClusterLog{infoErr: errors.New("no")}, remove)
	if rec := doClusterRequest(badInfo, http.MethodDelete, "/cluster/nodes/node-3", ""); http.StatusInternalServerError != rec.Code {
		t.Fatalf("log info failure status %d", rec.Code)
	}
	if rec := doClusterRequest(newClusterTestHandler(nil, nil), http.MethodDelete, "/cluster/nodes/node-3", ""); http.StatusConflict != rec.Code {
		t.Fatalf("single mode status %d", rec.Code)
	}
}

func TestClusterStreamOwned(t *testing.T) {
	for _, name := range []string{"CALLFS_META", "KV_CALLFS_LOCKS", "OBJ_CALLFS_META_SNAP"} {
		if !clusterStreamOwned(name) {
			t.Fatalf("%s must be owned", name)
		}
	}
	for _, name := range []string{"OTHER", "KV_OTHER", "XCALLFS_META"} {
		if clusterStreamOwned(name) {
			t.Fatalf("%s must not be owned", name)
		}
	}
}

func TestClusterPeerRemoveReply(t *testing.T) {
	cases := []struct {
		body    string
		wantErr bool
	}{
		{`{"success":true}`, false},
		{`{"error":{"code":500,"description":"peer not a member"}}`, false},
		{`{"error":{"code":500,"description":"no leader"}}`, true},
		{`nope`, true},
	}
	for _, tc := range cases {
		err := clusterPeerRemoveReply(&nats.Msg{Data: []byte(tc.body)})
		if tc.wantErr != (nil != err) {
			t.Fatalf("%s err %v", tc.body, err)
		}
	}
}

func TestClusterSnapshots(t *testing.T) {
	fake := &fakeClusterLog{
		retain:    3,
		snapshots: []natsmeta.SnapshotInfo{{ID: "snap-9000", Seq: 9000, Bytes: 40213, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}},
	}
	handler := newClusterTestHandler(fake, nil)
	rec := doClusterRequest(handler, http.MethodGet, "/cluster/snapshots", "")
	if http.StatusOK != rec.Code {
		t.Fatalf("status %d", rec.Code)
	}
	response := decodeCluster[snapshotsResponse](t, rec)
	if 3 != response.Retain || 1 != len(response.Items) || "snap-9000" != response.Items[0].ID || 9000 != response.Items[0].Seq {
		t.Fatalf("response %+v", response)
	}
	fake.snapshots = nil
	empty := doClusterRequest(handler, http.MethodGet, "/cluster/snapshots", "")
	if !strings.Contains(empty.Body.String(), `"items":[]`) {
		t.Fatalf("empty list body %s", empty.Body.String())
	}
	fake.listErr = errors.New("no")
	if rec := doClusterRequest(handler, http.MethodGet, "/cluster/snapshots", ""); http.StatusInternalServerError != rec.Code {
		t.Fatalf("failure status %d", rec.Code)
	}
	single := doClusterRequest(newClusterTestHandler(nil, nil), http.MethodGet, "/cluster/snapshots", "")
	if http.StatusOK != single.Code || !strings.Contains(single.Body.String(), `"items":[]`) {
		t.Fatalf("single mode %d %s", single.Code, single.Body.String())
	}
}

func TestClusterTakeSnapshot(t *testing.T) {
	fake := &fakeClusterLog{snapshot: natsmeta.SnapshotInfo{ID: "snap-9120", Seq: 9120, Bytes: 41000, Took: 12 * time.Millisecond}}
	handler := newClusterTestHandler(fake, nil)
	rec := doClusterRequest(handler, http.MethodPost, "/cluster/snapshot", "")
	if http.StatusOK != rec.Code {
		t.Fatalf("status %d", rec.Code)
	}
	response := decodeCluster[snapshotTakenResponse](t, rec)
	if "snap-9120" != response.ID || 9120 != response.Seq || 41000 != response.Bytes || 12 != response.TookMs {
		t.Fatalf("response %+v", response)
	}
	fake.snapshotErr = errors.New("no")
	if rec := doClusterRequest(handler, http.MethodPost, "/cluster/snapshot", ""); http.StatusInternalServerError != rec.Code {
		t.Fatalf("failure status %d", rec.Code)
	}
	if rec := doClusterRequest(newClusterTestHandler(nil, nil), http.MethodPost, "/cluster/snapshot", ""); http.StatusConflict != rec.Code {
		t.Fatalf("single mode status %d", rec.Code)
	}
}

func TestClusterJoinConfig(t *testing.T) {
	handler := newClusterTestHandler(&fakeClusterLog{info: clusterTestInfo()}, nil)
	target := "/cluster/join-config?node_id=node-4&api_endpoint=https://10.0.0.4:8443&cluster_addr=10.0.0.4:6222"
	rec := doClusterRequest(handler, http.MethodGet, target, "")
	if http.StatusOK != rec.Code {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	response := decodeCluster[joinConfigResponse](t, rec)
	for _, want := range []string{
		`enabled: true`,
		`mode: "embedded"`,
		`server_name: "node-4"`,
		`cluster_name: "callfs"`,
		`cluster_listen: "10.0.0.4:6222"`,
		`"nats-route://10.0.0.1:6222"`,
		`"nats-route://10.0.0.2:6222"`,
		`"nats-route://10.0.0.3:6222"`,
		`"nats-route://10.0.0.4:6222"`,
		`instance_id: "node-4"`,
		`"node-1": "https://10.0.0.1:8443"`,
		`"node-2": "https://10.0.0.2:8443"`,
		`"node-3": "https://10.0.0.3:8443"`,
		`"node-4": "https://10.0.0.4:8443"`,
		`type: "nats"`,
	} {
		if !strings.Contains(response.YAML, want) {
			t.Fatalf("yaml missing %q:\n%s", want, response.YAML)
		}
	}
	if strings.Contains(strings.ToLower(response.YAML), "secret") {
		t.Fatalf("yaml must not hold secrets:\n%s", response.YAML)
	}
	if 0 == len(response.Steps) || !strings.Contains(response.Steps[0], "auth.internal_proxy_secret") || !strings.Contains(response.Steps[0], "auth.single_use_link_secret") {
		t.Fatalf("steps %v", response.Steps)
	}
}

func TestClusterJoinConfigErrors(t *testing.T) {
	handler := newClusterTestHandler(&fakeClusterLog{info: clusterTestInfo()}, nil)
	cases := []struct {
		name   string
		target string
		code   int
	}{
		{"missing all", "/cluster/join-config", http.StatusUnprocessableEntity},
		{"bad node", "/cluster/join-config?node_id=a%20b&api_endpoint=https://h:1&cluster_addr=h:2", http.StatusUnprocessableEntity},
		{"bad endpoint", "/cluster/join-config?node_id=n&api_endpoint=ftp://h:1&cluster_addr=h:2", http.StatusUnprocessableEntity},
		{"bad addr", "/cluster/join-config?node_id=n&api_endpoint=https://h:1&cluster_addr=h", http.StatusUnprocessableEntity},
		{"existing member", "/cluster/join-config?node_id=node-2&api_endpoint=https://h:1&cluster_addr=h:2", http.StatusConflict},
	}
	for _, tc := range cases {
		if rec := doClusterRequest(handler, http.MethodGet, tc.target, ""); tc.code != rec.Code {
			t.Fatalf("%s status %d: %s", tc.name, rec.Code, rec.Body.String())
		}
	}
	failing := newClusterTestHandler(&fakeClusterLog{infoErr: errors.New("no")}, nil)
	if rec := doClusterRequest(failing, http.MethodGet, "/cluster/join-config?node_id=n&api_endpoint=https://h:1&cluster_addr=h:2", ""); http.StatusInternalServerError != rec.Code {
		t.Fatalf("log info failure status %d", rec.Code)
	}
	single := newClusterTestHandler(nil, nil)
	if rec := doClusterRequest(single, http.MethodGet, "/cluster/join-config?node_id=n&api_endpoint=https://h:1&cluster_addr=h:2", ""); http.StatusConflict != rec.Code {
		t.Fatalf("single mode status %d", rec.Code)
	}
}
