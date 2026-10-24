package admin

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/core"
	"github.com/ebogdum/callfs/v2/internal/diskspace"
	"github.com/ebogdum/callfs/v2/server/handlers"
)

type backendProbeRecord struct {
	At        time.Time `json:"at"`
	OK        bool      `json:"ok"`
	LatencyMs int64     `json:"latency_ms"`
	Error     *string   `json:"error"`
}

type backendView struct {
	Name       string              `json:"name"`
	Enabled    bool                `json:"enabled"`
	Status     string              `json:"status"`
	RootPath   *string             `json:"root_path"`
	FreeBytes  *uint64             `json:"free_bytes"`
	TotalBytes *uint64             `json:"total_bytes"`
	UsedBytes  *uint64             `json:"used_bytes"`
	Bucket     *string             `json:"bucket"`
	Region     *string             `json:"region"`
	Endpoint   *string             `json:"endpoint"`
	Encryption *string             `json:"encryption"`
	ACL        *string             `json:"acl"`
	KMSKeySet  *bool               `json:"kms_key_set"`
	Peers      int                 `json:"peers"`
	LastProbe  *backendProbeRecord `json:"last_probe"`
}

type backendsPage struct {
	Default string        `json:"default"`
	Items   []backendView `json:"items"`
}

type backendProbeResponse struct {
	OK       bool    `json:"ok"`
	WriteMS  int64   `json:"write_ms"`
	ReadMS   int64   `json:"read_ms"`
	DeleteMS int64   `json:"delete_ms"`
	Error    *string `json:"error"`
}

type backendProbeTable struct {
	mu      sync.RWMutex
	records map[string]backendProbeRecord
}

var backendProbes = &backendProbeTable{records: make(map[string]backendProbeRecord)}

func (t *backendProbeTable) record(name string) (backendProbeRecord, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	record, found := t.records[name]
	return record, found
}

func (t *backendProbeTable) store(name string, record backendProbeRecord) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records[name] = record
}

func (a *API) Backends(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	peers, err := a.peerCount()
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	handlers.SendJSONResponse(w, backendsPage{
		Default: a.deps.Config.Backend.DefaultBackend,
		Items: []backendView{
			a.localFSBackend(),
			a.s3Backend(),
			a.internalProxyBackend(peers),
			a.erasureBackend(peers),
		},
	})
}

func (a *API) ProbeBackend(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	name := chi.URLParam(r, "name")
	peers, err := a.peerCount()
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	storage, enabled, known := a.probeTarget(name, peers)
	if !known {
		handlers.SendErrorResponse(w, logger, errors.New("backend not found"), http.StatusNotFound)
		return
	}
	if !enabled {
		handlers.SendErrorResponse(w, logger, errors.New("backend is not enabled"), http.StatusConflict)
		return
	}
	probe, err := a.deps.Engine.ProbeStorage(r.Context(), storage)
	if nil != err {
		status := http.StatusInternalServerError
		if errors.Is(err, core.ErrStorageUnavailable) {
			status = http.StatusConflict
		}
		handlers.SendErrorResponse(w, logger, err, status)
		return
	}
	backendProbes.store(name, backendProbeRecord{
		At:        time.Now().UTC(),
		OK:        probe.OK,
		LatencyMs: probe.WriteMS + probe.ReadMS + probe.DeleteMS,
		Error:     nullableString(probe.Error),
	})
	handlers.SendJSONResponse(w, backendProbeResponse{
		OK:       probe.OK,
		WriteMS:  probe.WriteMS,
		ReadMS:   probe.ReadMS,
		DeleteMS: probe.DeleteMS,
		Error:    nullableString(probe.Error),
	})
	logger.Info("Backend probed", zap.String("backend", name), zap.Bool("ok", probe.OK))
}

func (a *API) peerCount() (int, error) {
	endpoints, err := a.instanceEndpoints()
	if nil != err {
		return 0, err
	}
	count := 0
	for _, endpoint := range endpoints {
		if !endpoint.isSelf {
			count++
		}
	}
	return count, nil
}

func (a *API) probeTarget(name string, peers int) (string, bool, bool) {
	cfg := a.deps.Config
	switch name {
	case "localfs":
		return "localfs", "" != cfg.Backend.LocalFSRootPath, true
	case "s3":
		return "s3", "" != cfg.Backend.S3BucketName, true
	case "internal_proxy":
		return "internal_proxy", 0 < peers, true
	case "erasure":
		return cfg.Erasure.ShardBackend, cfg.Erasure.Enabled, true
	}
	return "", false, false
}

func (a *API) localFSBackend() backendView {
	root := a.deps.Config.Backend.LocalFSRootPath
	view := backendView{
		Name:      "localfs",
		Enabled:   "" != root,
		RootPath:  nullableString(root),
		LastProbe: backendProbeView("localfs"),
	}
	if !view.Enabled {
		view.Status = backendStatus(false, nil, 0, 0)
		return view
	}
	free, total, used, err := localDiskUsage(root)
	if nil != err {
		view.Status = "down"
		return view
	}
	view.FreeBytes = &free
	view.TotalBytes = &total
	view.UsedBytes = &used
	view.Status = backendStatus(true, view.LastProbe, free, total)
	return view
}

func (a *API) s3Backend() backendView {
	cfg := a.deps.Config.Backend
	kmsSet := "" != cfg.S3KMSKeyID
	view := backendView{
		Name:       "s3",
		Enabled:    "" != cfg.S3BucketName,
		Bucket:     nullableString(cfg.S3BucketName),
		Region:     nullableString(cfg.S3Region),
		Endpoint:   nullableString(cfg.S3Endpoint),
		Encryption: nullableString(cfg.S3ServerSideEncryption),
		ACL:        nullableString(cfg.S3ACL),
		KMSKeySet:  &kmsSet,
		LastProbe:  backendProbeView("s3"),
	}
	view.Status = backendStatus(view.Enabled, view.LastProbe, 0, 0)
	return view
}

func (a *API) internalProxyBackend(peers int) backendView {
	view := backendView{
		Name:      "internal_proxy",
		Enabled:   0 < peers,
		Peers:     peers,
		LastProbe: backendProbeView("internal_proxy"),
	}
	view.Status = backendStatus(view.Enabled, view.LastProbe, 0, 0)
	return view
}

func (a *API) erasureBackend(peers int) backendView {
	view := backendView{
		Name:      "erasure",
		Enabled:   a.deps.Config.Erasure.Enabled,
		Peers:     peers,
		LastProbe: backendProbeView("erasure"),
	}
	view.Status = backendStatus(view.Enabled, view.LastProbe, 0, 0)
	return view
}

func backendProbeView(name string) *backendProbeRecord {
	record, found := backendProbes.record(name)
	if !found {
		return nil
	}
	return &record
}

func backendStatus(enabled bool, probe *backendProbeRecord, free, total uint64) string {
	switch {
	case !enabled:
		return "disabled"
	case nil != probe && !probe.OK:
		return "down"
	case 0 < total && total/10 > free:
		return "warn"
	}
	return "ok"
}

func localDiskUsage(root string) (uint64, uint64, uint64, error) {
	usage, err := diskspace.Of(root)
	if nil != err {
		return 0, 0, 0, err
	}
	return usage.Free, usage.Total, usage.Used, nil
}
