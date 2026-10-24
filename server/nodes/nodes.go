package nodes

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

const (
	OnBehalfHeader    = "X-CallFS-On-Behalf-Of"
	ForwardedByHeader = "X-CallFS-Forwarded-By"
	probeTimeout      = 2 * time.Second
	statusBodyLimit   = 1 << 20
)

var errUnknownNode = errors.New("unknown node")

type UserResolver func(ctx context.Context) (string, bool)

type AdminChecker func(userID string) bool

type Config struct {
	SelfID         string
	SelfEndpoint   string
	Peers          map[string]string
	InternalSecret string
	User           UserResolver
	IsAdmin        AdminChecker
	Role           func() (state, leaderID string)
	Logger         *zap.Logger
}

type Directory struct {
	cfg     Config
	peers   map[string]*url.URL
	client  *http.Client
	proxies map[string]*httputil.ReverseProxy
	local   http.Handler
	localMu sync.RWMutex
}

type Node struct {
	ID        string     `json:"id"`
	Endpoint  string     `json:"endpoint"`
	Self      bool       `json:"self"`
	Status    string     `json:"status"`
	LatencyMs int64      `json:"latency_ms"`
	Detail    string     `json:"detail,omitempty"`
	Role      string     `json:"role,omitempty"`
	LeaderID  string     `json:"leader_id,omitempty"`
	Version   string     `json:"version,omitempty"`
	UptimeS   int64      `json:"uptime_s,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
}

type listResponse struct {
	Self  string `json:"self"`
	Nodes []Node `json:"nodes"`
}

func New(cfg Config) (*Directory, error) {
	d := &Directory{
		cfg:     cfg,
		peers:   make(map[string]*url.URL, len(cfg.Peers)),
		client:  &http.Client{Timeout: probeTimeout},
		proxies: make(map[string]*httputil.ReverseProxy, len(cfg.Peers)),
	}
	for id, endpoint := range cfg.Peers {
		if id == cfg.SelfID || "" == strings.TrimSpace(endpoint) {
			continue
		}
		target, err := url.Parse(strings.TrimRight(endpoint, "/"))
		if nil != err || "" == target.Host || ("http" != target.Scheme && "https" != target.Scheme) {
			return nil, errors.New("invalid endpoint for node " + id)
		}
		d.peers[id] = target
		d.proxies[id] = d.newProxy(id, target)
	}
	return d, nil
}

func (d *Directory) SetLocal(h http.Handler) {
	d.localMu.Lock()
	d.local = h
	d.localMu.Unlock()
}

func (d *Directory) Mount(r chi.Router) {
	r.Get("/nodes", d.List)
	r.HandleFunc("/nodes/{node}/*", d.Forward)
}

func (d *Directory) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := d.cfg.User(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	admin := d.cfg.IsAdmin(userID)
	ids := make([]string, 0, len(d.peers))
	for id := range d.peers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Node, len(ids)+1)
	out[0] = Node{ID: d.cfg.SelfID, Endpoint: d.cfg.SelfEndpoint, Self: true, Status: "up"}
	if nil != d.cfg.Role {
		out[0].Role, out[0].LeaderID = d.cfg.Role()
	}
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			out[i+1] = d.probe(r.Context(), id, userID, admin)
		}(i, id)
	}
	if admin {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.fillStatus(r.Context(), &out[0], d.selfStatus(r, userID))
		}()
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, listResponse{Self: d.cfg.SelfID, Nodes: out})
}

func (d *Directory) selfStatus(r *http.Request, userID string) func(ctx context.Context) (*http.Response, error) {
	return func(ctx context.Context) (*http.Response, error) {
		d.localMu.RLock()
		local := d.local
		d.localMu.RUnlock()
		if nil == local {
			return nil, errors.New("local router not ready")
		}
		req, err := http.NewRequestWithContext(withoutRouting(ctx), http.MethodGet, "/v1/admin/status", nil)
		if nil != err {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+d.cfg.InternalSecret)
		req.Header.Set(OnBehalfHeader, userID)
		req.RemoteAddr = "127.0.0.1:0"
		rec := httptest.NewRecorder()
		local.ServeHTTP(rec, req)
		return rec.Result(), nil
	}
}

func (d *Directory) probe(ctx context.Context, id, userID string, admin bool) Node {
	target := d.peers[id]
	node := Node{ID: id, Endpoint: target.String(), Status: "down"}
	started := time.Now()
	health, err := d.get(ctx, target.String()+"/health")
	node.LatencyMs = time.Since(started).Milliseconds()
	if nil != err {
		node.Detail = err.Error()
		return node
	}
	var body struct {
		Status  string `json:"status"`
		Cluster *struct {
			State    string `json:"state"`
			LeaderID string `json:"leader_id"`
		} `json:"cluster"`
	}
	if decodeErr := json.Unmarshal(health, &body); nil != decodeErr {
		node.Detail = "unreadable health answer"
		return node
	}
	node.Status = "up"
	if nil != body.Cluster {
		node.Role = body.Cluster.State
		node.LeaderID = body.Cluster.LeaderID
	}
	if admin {
		d.fillStatus(ctx, &node, func(ctx context.Context) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String()+"/v1/admin/status", nil)
			if nil != err {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+d.cfg.InternalSecret)
			req.Header.Set(OnBehalfHeader, userID)
			req.Header.Set(ForwardedByHeader, d.cfg.SelfID)
			return d.client.Do(req)
		})
	}
	return node
}

func (d *Directory) fillStatus(ctx context.Context, node *Node, fetch func(ctx context.Context) (*http.Response, error)) {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	resp, err := fetch(probeCtx)
	if nil != err {
		node.Detail = err.Error()
		return
	}
	defer resp.Body.Close()
	if http.StatusOK != resp.StatusCode {
		node.Detail = "status answered " + resp.Status
		return
	}
	var status struct {
		Version   string    `json:"version"`
		UptimeS   int64     `json:"uptime_s"`
		StartedAt time.Time `json:"started_at"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, statusBodyLimit)).Decode(&status); nil != err {
		node.Detail = "unreadable status answer"
		return
	}
	node.Version = status.Version
	node.UptimeS = status.UptimeS
	if !status.StartedAt.IsZero() {
		started := status.StartedAt
		node.StartedAt = &started
	}
}

func (d *Directory) get(ctx context.Context, url string) ([]byte, error) {
	probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
	if nil != err {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if nil != err {
		return nil, err
	}
	defer resp.Body.Close()
	if http.StatusOK != resp.StatusCode {
		return nil, errors.New("answered " + resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, statusBodyLimit))
}

func (d *Directory) Forward(w http.ResponseWriter, r *http.Request) {
	userID, ok := d.cfg.User(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	if "" != r.Header.Get(ForwardedByHeader) {
		writeError(w, http.StatusLoopDetected, "FORWARD_LOOP", "requests can be forwarded only once")
		return
	}
	nodeID := chi.URLParam(r, "node")
	rest := "/" + chi.URLParam(r, "*")
	if !forwardable(rest) {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "this path cannot be sent to another server")
		return
	}
	if nodeID == d.cfg.SelfID {
		d.serveLocal(w, r, rest)
		return
	}
	proxy, known := d.proxies[nodeID]
	if !known {
		writeError(w, http.StatusNotFound, "NODE_NOT_FOUND", errUnknownNode.Error()+": "+nodeID)
		return
	}
	out := r.Clone(r.Context())
	out.URL.Path = rest
	out.URL.RawPath = ""
	out.Header.Set(OnBehalfHeader, userID)
	proxy.ServeHTTP(w, out)
}

func (d *Directory) serveLocal(w http.ResponseWriter, r *http.Request, rest string) {
	d.localMu.RLock()
	local := d.local
	d.localMu.RUnlock()
	if nil == local {
		writeError(w, http.StatusServiceUnavailable, "NOT_READY", "router not ready")
		return
	}
	out := r.Clone(withoutRouting(r.Context()))
	out.URL.Path = rest
	out.URL.RawPath = ""
	out.RequestURI = out.URL.RequestURI()
	local.ServeHTTP(w, out)
}

func withoutRouting(ctx context.Context) context.Context {
	return context.WithValue(ctx, chi.RouteCtxKey, nil)
}

func forwardable(path string) bool {
	if strings.Contains(path, "/../") || strings.HasSuffix(path, "/..") {
		return false
	}
	if "/health" == path || "/health/ready" == path {
		return true
	}
	if !strings.HasPrefix(path, "/v1/") {
		return false
	}
	for _, blocked := range []string{"/v1/session", "/v1/nodes"} {
		if path == blocked || strings.HasPrefix(path, blocked+"/") {
			return false
		}
	}
	return true
}

func (d *Directory) newProxy(id string, target *url.URL) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.URL.Path = target.Path + pr.In.URL.Path
			pr.Out.URL.RawPath = ""
			pr.Out.Host = target.Host
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Del("X-CallFS-CSRF")
			pr.Out.Header.Set("Authorization", "Bearer "+d.cfg.InternalSecret)
			pr.Out.Header.Set(OnBehalfHeader, pr.In.Header.Get(OnBehalfHeader))
			pr.Out.Header.Set(ForwardedByHeader, d.cfg.SelfID)
			pr.SetXForwarded()
		},
		ModifyResponse: func(resp *http.Response) error {
			resp.Header.Del("Set-Cookie")
			resp.Header.Set("X-CallFS-Node", id)
			return nil
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if nil != d.cfg.Logger {
				d.cfg.Logger.Warn("Forward to node failed", zap.String("node", id), zap.String("path", r.URL.Path), zap.Error(err))
			}
			writeError(w, http.StatusBadGateway, "NODE_UNREACHABLE", "server "+id+" did not answer")
		},
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "message": message})
}
