package admin

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/internal/buildinfo"
	"github.com/ebogdum/callfs/metadata/schema"
	"github.com/ebogdum/callfs/server/handlers"
)

const (
	restartReasonLimit = 200
	cpuSecondsMetric   = "/cpu/classes/total:cpu-seconds"
)

var processStartedAt = time.Now()

type listenerView struct {
	Kind string `json:"kind"`
	Addr string `json:"addr"`
	TLS  bool   `json:"tls"`
	Auth bool   `json:"auth"`
}

type storagesView struct {
	LocalFS       bool `json:"localfs"`
	S3            bool `json:"s3"`
	InternalProxy bool `json:"internal_proxy"`
	Erasure       bool `json:"erasure"`
}

type runtimeView struct {
	Goroutines int     `json:"goroutines"`
	HeapBytes  uint64  `json:"heap_bytes"`
	OpenFDs    *int    `json:"open_fds"`
	CPUSeconds float64 `json:"cpu_seconds"`
}

type statusView struct {
	Version            string         `json:"version"`
	Commit             string         `json:"commit"`
	BuildDate          string         `json:"build_date"`
	GoVersion          string         `json:"go_version"`
	InstanceID         string         `json:"instance_id"`
	NodeID             string         `json:"node_id"`
	StartedAt          time.Time      `json:"started_at"`
	UptimeS            int64          `json:"uptime_s"`
	ConfigPath         string         `json:"config_path"`
	ConfigFormat       string         `json:"config_format"`
	KeysFromEnv        []string       `json:"keys_from_env"`
	KeysAtDefault      []string       `json:"keys_at_default"`
	Listeners          []listenerView `json:"listeners"`
	ProtocolConfigured string         `json:"protocol_configured"`
	ProtocolEffective  string         `json:"protocol_effective"`
	FileIndex          string         `json:"file_index"`
	LockType           string         `json:"lock_type"`
	Storages           storagesView   `json:"storages"`
	Runtime            runtimeView    `json:"runtime"`
}

type restartRequest struct {
	Reason string `json:"reason"`
}

type restartAccepted struct {
	Restarting bool `json:"restarting"`
}

type migrationsResponse struct {
	Driver         string             `json:"driver"`
	CurrentVersion *int64             `json:"current_version"`
	Dirty          *bool              `json:"dirty"`
	Available      []schema.Migration `json:"available"`
}

func (a *API) Status(w http.ResponseWriter, _ *http.Request) {
	logger := a.logger()
	cfg := a.deps.Config
	if nil == cfg {
		handlers.SendErrorResponse(w, logger, errors.New("configuration is not loaded"), http.StatusInternalServerError)
		return
	}
	peers, err := a.peerCount()
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	effective, _ := effectiveProtocol(cfg)
	instanceID := cfg.InstanceDiscovery.InstanceID
	if nil != a.deps.Engine {
		instanceID = a.deps.Engine.GetCurrentInstanceID()
	}
	handlers.SendJSONResponse(w, statusView{
		Version:            buildinfo.Version,
		Commit:             buildinfo.Commit,
		BuildDate:          buildinfo.Date,
		GoVersion:          runtime.Version(),
		InstanceID:         instanceID,
		NodeID:             cfg.NATS.ServerName,
		StartedAt:          processStartedAt.UTC(),
		UptimeS:            int64(time.Since(processStartedAt).Seconds()),
		ConfigPath:         a.deps.ConfigPath,
		ConfigFormat:       configFormat(a.deps.ConfigPath),
		KeysFromEnv:        config.KeysFromEnv(a.deps.ConfigSources),
		KeysAtDefault:      config.KeysAtDefault(a.deps.ConfigSources),
		Listeners:          listenersOf(cfg, effective),
		ProtocolConfigured: cfg.Server.Protocol,
		ProtocolEffective:  effective,
		FileIndex:          cfg.MetadataStore.Type,
		LockType:           cfg.DLM.Type,
		Storages: storagesView{
			LocalFS:       "" != cfg.Backend.LocalFSRootPath,
			S3:            "" != cfg.Backend.S3BucketName,
			InternalProxy: 0 < peers,
			Erasure:       cfg.Erasure.Enabled,
		},
		Runtime: runtimeSnapshot(),
	})
}

func (a *API) Restart(w http.ResponseWriter, r *http.Request) {
	logger := a.logger()
	if nil == a.deps.Config || !a.deps.Config.UI.AllowRemoteRestart {
		writeAdminError(w, http.StatusForbidden, "FEATURE_DISABLED", "remote restart is disabled; set ui.allow_remote_restart to true", nil)
		return
	}
	if nil == a.deps.Restart {
		a.unavailable(w, "restart control")
		return
	}
	var req restartRequest
	if err := decodeAdminBody(w, r, &req); nil != err && !errors.Is(err, io.EOF) {
		respondBodyError(w, logger, err)
		return
	}
	if restartReasonLimit < utf8.RuneCountInString(req.Reason) {
		writeFieldProblems(w, []fieldProblem{{Path: "reason", Message: "reason must be at most 200 characters"}})
		return
	}
	actor, ok := jobActor(w, r, logger)
	if !ok {
		return
	}
	sendAccepted(w, logger, restartAccepted{Restarting: true})
	logger.Warn("Remote restart requested", zap.String("user_id", actor), zap.String("reason", req.Reason))
	restart := a.deps.Restart
	reason := req.Reason
	go func() {
		if err := restart(reason); nil != err {
			logger.Error("Remote restart failed", zap.Error(err))
		}
	}()
}

func (a *API) Migrations(w http.ResponseWriter, r *http.Request) {
	logger := a.logger()
	cfg := a.deps.Config
	if nil == cfg {
		handlers.SendErrorResponse(w, logger, errors.New("configuration is not loaded"), http.StatusInternalServerError)
		return
	}
	available, err := schema.AvailableMigrations()
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	response := migrationsResponse{Driver: migrationDriver(cfg.MetadataStore.Type), Available: available}
	if "postgres" == response.Driver {
		version, dirty, err := schema.CurrentVersion(r.Context(), migrationDSN(cfg))
		if nil != err {
			handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
		response.CurrentVersion = &version
		response.Dirty = &dirty
	}
	handlers.SendJSONResponse(w, response)
}

func migrationDSN(cfg *config.AppConfig) string {
	if "" != cfg.MetadataStore.MigrationDSN {
		return cfg.MetadataStore.MigrationDSN
	}
	return cfg.MetadataStore.DSN
}

func configFormat(path string) string {
	switch {
	case "" == path:
		return "none"
	case ".json" == strings.ToLower(filepath.Ext(path)):
		return "json"
	}
	return "yaml"
}

func listenersOf(cfg *config.AppConfig, effective string) []listenerView {
	listeners := []listenerView{{Kind: effective, Addr: cfg.Server.ListenAddr, TLS: "https" == effective, Auth: true}}
	if cfg.Server.EnableQUIC {
		listeners = append(listeners, listenerView{Kind: "quic", Addr: cfg.Server.QUICListenAddr, TLS: true, Auth: true})
	}
	if "" != cfg.Metrics.ListenAddr {
		listeners = append(listeners, listenerView{Kind: "metrics", Addr: cfg.Metrics.ListenAddr})
	}
	return listeners
}

func runtimeSnapshot() runtimeView {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	samples := []metrics.Sample{{Name: cpuSecondsMetric}}
	metrics.Read(samples)
	view := runtimeView{
		Goroutines: runtime.NumGoroutine(),
		HeapBytes:  memory.HeapAlloc,
		OpenFDs:    openFileDescriptors(),
	}
	if metrics.KindFloat64 == samples[0].Value.Kind() {
		view.CPUSeconds = samples[0].Value.Float64()
	}
	return view
}

func openFileDescriptors() *int {
	entries, err := os.ReadDir("/dev/fd")
	if nil != err {
		return nil
	}
	count := len(entries)
	return &count
}
