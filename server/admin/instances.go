package admin

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/server/handlers"
)

const (
	instanceProbeTimeout   = 5 * time.Second
	instanceProbeBodyLimit = 4 << 10
	topologyBodyLimit      = 16 << 10
	instancePageSize       = 500
	instanceSourceSelf     = "self"
	instanceSourceConfig   = "config"
	instanceSourceJoined   = "joined"
)

var (
	instanceStartedAt = time.Now().UTC()
	instanceIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
)

type instanceProbe struct {
	At         time.Time
	Reachable  bool
	RTTMs      int64
	StatusCode int
	Error      string
}

type instanceState struct {
	mu        sync.RWMutex
	probes    map[string]instanceProbe
	overrides map[string]string
}

var instanceRegistry = &instanceState{
	probes:    make(map[string]instanceProbe),
	overrides: make(map[string]string),
}

func (s *instanceState) probe(id string) (instanceProbe, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	probe, found := s.probes[id]
	return probe, found
}

func (s *instanceState) storeProbe(id string, probe instanceProbe) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probes[id] = probe
}

func (s *instanceState) setOverride(id, endpoint string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.overrides[id] = endpoint
}

func (s *instanceState) overrideSnapshot() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return maps.Clone(s.overrides)
}

type instanceEndpoint struct {
	id       string
	endpoint string
	source   string
	isSelf   bool
}

type instanceSelf struct {
	InstanceID  string    `json:"instance_id"`
	ExternalURL string    `json:"external_url"`
	StartedAt   time.Time `json:"started_at"`
}

type instanceView struct {
	InstanceID string     `json:"instance_id"`
	Endpoint   string     `json:"endpoint"`
	Source     string     `json:"source"`
	IsSelf     bool       `json:"is_self"`
	Scheme     string     `json:"scheme"`
	Reachable  *bool      `json:"reachable"`
	RTTMs      *int64     `json:"rtt_ms"`
	CheckedAt  *time.Time `json:"checked_at"`
	Files      int64      `json:"files"`
	Bytes      int64      `json:"bytes"`
	Pieces     int64      `json:"pieces"`
}

type instancesPage struct {
	Self  instanceSelf   `json:"self"`
	Items []instanceView `json:"items"`
}

type instanceEndpointRequest struct {
	Endpoint string `json:"endpoint"`
	Persist  bool   `json:"persist"`
}

type instanceProbeResponse struct {
	Reachable  bool    `json:"reachable"`
	RTTMs      int64   `json:"rtt_ms"`
	StatusCode *int    `json:"status_code"`
	Error      *string `json:"error"`
}

type instanceProberResult struct {
	Probed    int `json:"probed"`
	Reachable int `json:"reachable"`
}

type instanceUsage struct {
	Files  int64
	Bytes  int64
	Pieces int64
}

func (a *API) Instances(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	items, err := a.instanceViews(r.Context())
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	handlers.SendJSONResponse(w, instancesPage{
		Self: instanceSelf{
			InstanceID:  a.deps.Engine.GetCurrentInstanceID(),
			ExternalURL: a.deps.Config.Server.ExternalURL,
			StartedAt:   instanceStartedAt,
		},
		Items: items,
	})
}

func (a *API) SetInstanceEndpoint(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	id := chi.URLParam(r, "id")
	var req instanceEndpointRequest
	if !decodeTopologyBody(w, r, logger, &req) {
		return
	}
	if !instanceIDPattern.MatchString(id) {
		handlers.SendErrorResponse(w, logger, errors.New("server ID may use letters, digits, dot, dash and underscore"), http.StatusBadRequest)
		return
	}
	if id == a.deps.Engine.GetCurrentInstanceID() {
		handlers.SendErrorResponse(w, logger, errors.New("this server's address is set by external_url"), http.StatusBadRequest)
		return
	}
	endpoint, err := validateInstanceEndpoint(req.Endpoint)
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusBadRequest)
		return
	}
	if req.Persist {
		if err := a.savePeerEndpoints(func(peers map[string]string) { peers[id] = endpoint }); nil != err {
			handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
	}
	if err := a.deps.Engine.AddPeerEndpoint(id, endpoint); nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	instanceRegistry.setOverride(id, endpoint)
	items, err := a.instanceViews(r.Context())
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	item, _ := findInstanceView(items, id)
	handlers.SendJSONResponse(w, item)
	logger.Info("Server endpoint set", zap.String("instance_id", id), zap.Bool("persist", req.Persist))
}

func (a *API) RemoveInstanceEndpoint(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	id := chi.URLParam(r, "id")
	if !instanceIDPattern.MatchString(id) {
		handlers.SendErrorResponse(w, logger, errors.New("server ID may use letters, digits, dot, dash and underscore"), http.StatusBadRequest)
		return
	}
	if id == a.deps.Engine.GetCurrentInstanceID() {
		handlers.SendErrorResponse(w, logger, errors.New("this server's address is set by external_url"), http.StatusBadRequest)
		return
	}
	if _, inConfig := a.deps.Config.InstanceDiscovery.PeerEndpoints[id]; inConfig {
		if err := a.savePeerEndpoints(func(peers map[string]string) { delete(peers, id) }); nil != err {
			handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
	}
	a.deps.Engine.RemovePeerEndpoint(id)
	instanceRegistry.setOverride(id, "")
	items, err := a.instanceViews(r.Context())
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	item, found := findInstanceView(items, id)
	if !found {
		item = instanceView{InstanceID: id}
	}
	handlers.SendJSONResponse(w, item)
	logger.Info("Server endpoint removed", zap.String("instance_id", id))
}

func (a *API) ProbeInstance(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	id := chi.URLParam(r, "id")
	endpoints, err := a.instanceEndpoints()
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	target, found := findInstanceEndpoint(endpoints, id)
	if !found {
		handlers.SendErrorResponse(w, logger, errors.New("server not found"), http.StatusNotFound)
		return
	}
	if "" == target.endpoint {
		handlers.SendErrorResponse(w, logger, errors.New("server has no address to probe"), http.StatusBadRequest)
		return
	}
	probe := a.probeInstance(r.Context(), target.endpoint)
	instanceRegistry.storeProbe(id, probe)
	handlers.SendJSONResponse(w, newInstanceProbeResponse(probe))
	logger.Info("Server probed", zap.String("instance_id", id), zap.Bool("reachable", probe.Reachable))
}

func (a *API) InstanceProberWorker(ctx context.Context) (any, error) {
	endpoints, err := a.instanceEndpoints()
	if nil != err {
		return nil, err
	}
	result := instanceProberResult{}
	for _, endpoint := range endpoints {
		if endpoint.isSelf || "" == endpoint.endpoint {
			continue
		}
		if err := ctx.Err(); nil != err {
			return result, err
		}
		probe := a.probeInstance(ctx, endpoint.endpoint)
		instanceRegistry.storeProbe(endpoint.id, probe)
		result.Probed++
		if probe.Reachable {
			result.Reachable++
		}
	}
	return result, nil
}

func (a *API) instanceEndpoints() ([]instanceEndpoint, error) {
	selfID := a.deps.Engine.GetCurrentInstanceID()
	endpoints := []instanceEndpoint{{id: selfID, endpoint: a.deps.Config.Server.ExternalURL, source: instanceSourceSelf, isSelf: true}}
	seen := map[string]bool{selfID: true}
	peers := a.peerEndpoints()
	for _, id := range slices.Sorted(maps.Keys(peers)) {
		if seen[id] {
			continue
		}
		seen[id] = true
		endpoints = append(endpoints, instanceEndpoint{id: id, endpoint: peers[id], source: instanceSourceConfig})
	}
	return endpoints, nil
}

func (a *API) peerEndpoints() map[string]string {
	peers := maps.Clone(a.deps.Config.InstanceDiscovery.PeerEndpoints)
	if nil == peers {
		peers = make(map[string]string)
	}
	for id, endpoint := range instanceRegistry.overrideSnapshot() {
		if "" == endpoint {
			delete(peers, id)
			continue
		}
		peers[id] = endpoint
	}
	return peers
}

func (a *API) instanceViews(ctx context.Context) ([]instanceView, error) {
	endpoints, err := a.instanceEndpoints()
	if nil != err {
		return nil, err
	}
	usage, err := a.instanceUsage(ctx)
	if nil != err {
		return nil, err
	}
	views := make([]instanceView, 0, len(endpoints))
	for _, endpoint := range endpoints {
		view := instanceView{
			InstanceID: endpoint.id,
			Endpoint:   endpoint.endpoint,
			Source:     endpoint.source,
			IsSelf:     endpoint.isSelf,
			Scheme:     instanceScheme(endpoint.endpoint),
		}
		if probe, found := instanceRegistry.probe(endpoint.id); found {
			reachable := probe.Reachable
			rtt := probe.RTTMs
			checked := probe.At
			view.Reachable = &reachable
			view.RTTMs = &rtt
			view.CheckedAt = &checked
		}
		counts := usage[endpoint.id]
		view.Files = counts.Files
		view.Bytes = counts.Bytes
		view.Pieces = counts.Pieces
		views = append(views, view)
	}
	return views, nil
}

func (a *API) instanceUsage(ctx context.Context) (map[string]instanceUsage, error) {
	usage := make(map[string]instanceUsage)
	startAfter := ""
	for {
		files, err := a.deps.Store.ListByPrefix(ctx, "/", startAfter, instancePageSize)
		if nil != err {
			return nil, err
		}
		for _, md := range files {
			owner := ""
			if nil != md.CallFSInstanceID {
				owner = *md.CallFSInstanceID
			}
			entry := usage[owner]
			entry.Files++
			entry.Bytes += md.Size
			usage[owner] = entry
		}
		if instancePageSize > len(files) {
			break
		}
		startAfter = files[len(files)-1].Path
	}
	cursor := ""
	for {
		infos, next, err := a.deps.Store.ListErasureInfos(ctx, "/", cursor, instancePageSize)
		if errors.Is(err, metadata.ErrNotImplemented) {
			break
		}
		if nil != err {
			return nil, err
		}
		for _, info := range infos {
			for _, shard := range info.Shards {
				entry := usage[shard.InstanceID]
				entry.Pieces++
				usage[shard.InstanceID] = entry
			}
		}
		if "" == next || next == cursor {
			break
		}
		cursor = next
	}
	return usage, nil
}

func (a *API) probeInstance(ctx context.Context, endpoint string) instanceProbe {
	probe := instanceProbe{At: time.Now().UTC()}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/health", nil)
	if nil != err {
		probe.Error = err.Error()
		return probe
	}
	started := time.Now()
	response, err := a.instanceProbeClient().Do(request)
	probe.RTTMs = time.Since(started).Milliseconds()
	if nil != err {
		probe.Error = err.Error()
		return probe
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, instanceProbeBodyLimit))
	probe.StatusCode = response.StatusCode
	probe.Reachable = 500 > response.StatusCode
	if !probe.Reachable {
		probe.Error = fmt.Sprintf("server answered with status %d", response.StatusCode)
	}
	return probe
}

func (a *API) instanceProbeClient() *http.Client {
	return &http.Client{
		Timeout: instanceProbeTimeout,
		Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: a.deps.Config.Backend.InternalProxySkipTLSVerify},
		},
	}
}

func (a *API) savePeerEndpoints(change func(peers map[string]string)) error {
	peers := maps.Clone(a.deps.Config.InstanceDiscovery.PeerEndpoints)
	if nil == peers {
		peers = make(map[string]string)
	}
	change(peers)
	data, doc, err := config.ReadConfigDocument(a.deps.ConfigPath)
	if nil != err {
		return err
	}
	if setErr := config.SetDocumentValue(doc, "instance_discovery.peer_endpoints", peers); nil != setErr {
		return setErr
	}
	_, _, err = config.WriteConfigDocument(a.deps.ConfigPath, data, doc, time.Now())
	return err
}

func decodeTopologyBody(w http.ResponseWriter, r *http.Request, logger *zap.Logger, dst any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, topologyBodyLimit)).Decode(dst); nil != err && !errors.Is(err, io.EOF) {
		handlers.SendErrorResponse(w, logger, errors.New("request body must be valid JSON"), http.StatusBadRequest)
		return false
	}
	return true
}

func findInstanceView(items []instanceView, id string) (instanceView, bool) {
	for _, item := range items {
		if id == item.InstanceID {
			return item, true
		}
	}
	return instanceView{}, false
}

func findInstanceEndpoint(endpoints []instanceEndpoint, id string) (instanceEndpoint, bool) {
	for _, endpoint := range endpoints {
		if id == endpoint.id {
			return endpoint, true
		}
	}
	return instanceEndpoint{}, false
}

func newInstanceProbeResponse(probe instanceProbe) instanceProbeResponse {
	response := instanceProbeResponse{Reachable: probe.Reachable, RTTMs: probe.RTTMs, Error: nullableString(probe.Error)}
	if 0 < probe.StatusCode {
		status := probe.StatusCode
		response.StatusCode = &status
	}
	return response
}

func validateInstanceEndpoint(raw string) (string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(endpoint)
	if nil != err {
		return "", fmt.Errorf("endpoint is not a valid URL: %w", err)
	}
	if "https" != parsed.Scheme && "http" != parsed.Scheme {
		return "", errors.New("endpoint must start with https:// or http://")
	}
	if "" == parsed.Host || nil != parsed.User || "" != parsed.RawQuery || "" != parsed.Fragment {
		return "", errors.New("endpoint must be a host address with no user, query or fragment")
	}
	return endpoint, nil
}

func instanceScheme(endpoint string) string {
	if strings.HasPrefix(endpoint, "https://") {
		return "https"
	}
	return "http"
}

func nullableString(value string) *string {
	if "" == value {
		return nil
	}
	return &value
}
