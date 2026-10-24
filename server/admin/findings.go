package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/findings"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/metadata/schema"
	"github.com/ebogdum/callfs/server/handlers"
	"github.com/ebogdum/callfs/telemetry"
)

const (
	findingMutesPath   = "/.callfs/admin/finding-mutes.json"
	findingMutesName   = "finding-mutes.json"
	findingIgnoresPath = "/.callfs/admin/finding-ignores.json"
	findingIgnoresName = "finding-ignores.json"
	findingMutesLimit  = 1 << 20
	findingMutesOwner  = "root"
	maxMuteSeconds     = int64(7 * 24 * time.Hour / time.Second)
	recentWindow       = time.Hour
	factsCacheTTL      = 5 * time.Second
	authFailureWindow  = 10 * time.Minute
	linkOpenOp         = "link_open"
	linkFailureStatus  = 400
)

type factsMemory struct {
	cacheMu      sync.Mutex
	cached       findings.Facts
	cachedAt     time.Time
	cachedConfig *config.AppConfig
	mu           sync.Mutex
	downSince    map[string]time.Time
}

type findingsResponse struct {
	Items   []findings.Finding `json:"items"`
	Ignored []findings.Finding `json:"ignored"`
	Counts  findings.Counts    `json:"counts"`
}

type ignoreRequest struct {
	Reason string `json:"reason"`
}

type muteRequest struct {
	DurationS int64  `json:"duration_s"`
	Reason    string `json:"reason"`
}

type engineMuteStore struct {
	engine *core.Engine
}

func (s engineMuteStore) LoadMutes(ctx context.Context) (map[string]findings.Mute, error) {
	mutes := make(map[string]findings.Mute)
	if err := loadAdminJSON(ctx, s.engine, findingMutesPath, "finding mutes", &mutes); nil != err {
		return nil, err
	}
	return mutes, nil
}

func (s engineMuteStore) SaveMutes(ctx context.Context, mutes map[string]findings.Mute) error {
	return saveAdminJSON(ctx, s.engine, findingMutesPath, findingMutesName, "finding mutes", mutes)
}

func (s engineMuteStore) LoadIgnores(ctx context.Context) (map[string]findings.Ignore, error) {
	ignores := make(map[string]findings.Ignore)
	if err := loadAdminJSON(ctx, s.engine, findingIgnoresPath, "finding ignores", &ignores); nil != err {
		return nil, err
	}
	return ignores, nil
}

func (s engineMuteStore) SaveIgnores(ctx context.Context, ignores map[string]findings.Ignore) error {
	return saveAdminJSON(ctx, s.engine, findingIgnoresPath, findingIgnoresName, "finding ignores", ignores)
}

func loadAdminJSON(ctx context.Context, engine *core.Engine, path, label string, target any) error {
	rc, err := engine.GetFile(ctx, path)
	if errors.Is(err, metadata.ErrNotFound) {
		return nil
	}
	if nil != err {
		return fmt.Errorf("failed to open %s: %w", label, err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, findingMutesLimit))
	if nil != err {
		return fmt.Errorf("failed to read %s: %w", label, err)
	}
	if err := json.Unmarshal(data, target); nil != err {
		return fmt.Errorf("failed to decode %s: %w", label, err)
	}
	return nil
}

func saveAdminJSON(ctx context.Context, engine *core.Engine, path, name, label string, value any) error {
	data, err := json.Marshal(value)
	if nil != err {
		return fmt.Errorf("failed to encode %s: %w", label, err)
	}
	md := &metadata.Metadata{
		Name:        name,
		Type:        "file",
		Mode:        "0600",
		Owner:       findingMutesOwner,
		BackendType: "localfs",
	}
	if _, _, err := engine.PutFile(ctx, path, bytes.NewReader(data), int64(len(data)), md, "", false); nil != err {
		return fmt.Errorf("failed to save %s: %w", label, err)
	}
	return nil
}

func (a *API) muteStore() findings.MuteStore {
	if nil == a.deps.Engine {
		return nil
	}
	return engineMuteStore{engine: a.deps.Engine}
}

func (a *API) ignoreStore() findings.IgnoreStore {
	if nil == a.deps.Engine {
		return nil
	}
	return engineMuteStore{engine: a.deps.Engine}
}

func (a *API) currentConfig() *config.AppConfig {
	return a.deps.Config
}

func (a *API) AttachFindings() {
	engine := findings.NewEngine(a.currentConfig, a.collectFacts, a.muteStore(), a.ignoreStore())
	a.Deps.Findings = engine
	a.deps.Findings = engine
}

func (a *API) collectFacts(ctx context.Context) (findings.Facts, error) {
	cfg := a.deps.Config
	if nil == cfg {
		return findings.Facts{}, findings.ErrNoConfig
	}
	memory := &a.factsMemory
	memory.cacheMu.Lock()
	defer memory.cacheMu.Unlock()
	if cfg == memory.cachedConfig && !memory.cachedAt.IsZero() && factsCacheTTL > time.Since(memory.cachedAt) {
		facts := memory.cached
		facts.InstancesDown = slices.Clone(facts.InstancesDown)
		return facts, nil
	}
	facts, err := a.gatherFacts(ctx)
	if nil != err {
		memory.cachedAt = time.Time{}
		return facts, err
	}
	memory.cached = facts
	memory.cachedAt = time.Now()
	memory.cachedConfig = cfg
	facts.InstancesDown = slices.Clone(facts.InstancesDown)
	return facts, nil
}

func (a *API) gatherFacts(ctx context.Context) (findings.Facts, error) {
	var facts findings.Facts
	now := time.Now().UTC()
	if err := a.tlsFacts(&facts); nil != err {
		return facts, err
	}
	if err := a.instanceFacts(ctx, &facts, now); nil != err {
		return facts, err
	}
	if err := a.diskFacts(&facts, now); nil != err {
		return facts, err
	}
	if err := a.erasureFacts(ctx, &facts); nil != err {
		return facts, err
	}
	if err := a.telemetryFacts(&facts, now); nil != err {
		return facts, err
	}
	if err := a.metadataFacts(ctx, &facts); nil != err {
		return facts, err
	}
	if err := a.configFacts(&facts); nil != err {
		return facts, err
	}
	return facts, nil
}

func (a *API) tlsFacts(facts *findings.Facts) error {
	cfg := a.deps.Config
	effective, fellBack := effectiveProtocol(cfg)
	facts.AutoFellBack = fellBack
	if protocolHTTP == effective {
		return nil
	}
	if "" != cfg.Server.CertFile {
		cert, err := loadLeafCertificate(cfg.Server.CertFile)
		if nil != err {
			facts.CertUnreadable = true
		} else {
			facts.CertNotAfter = cert.NotAfter
		}
	}
	if "" != cfg.Server.KeyFile {
		perm, err := keyPermissions(cfg.Server.KeyFile)
		if nil != err {
			return err
		}
		facts.KeyWorldReadable = 0 != perm&0o004
	}
	return nil
}

func (a *API) instanceFacts(ctx context.Context, facts *findings.Facts, now time.Time) error {
	views, err := a.instanceViews(ctx)
	if nil != err {
		return err
	}
	a.factsMemory.mu.Lock()
	defer a.factsMemory.mu.Unlock()
	down := make(map[string]time.Time)
	for _, view := range views {
		if view.IsSelf || nil == view.Reachable || *view.Reachable {
			continue
		}
		since, known := a.factsMemory.downSince[view.InstanceID]
		if !known {
			since = now
		}
		down[view.InstanceID] = since
		facts.InstancesDown = append(facts.InstancesDown, findings.DownInstance{ID: view.InstanceID, DownSince: since})
	}
	a.factsMemory.downSince = down
	return nil
}

func (a *API) diskFacts(facts *findings.Facts, now time.Time) error {
	root := a.deps.Config.Backend.LocalFSRootPath
	if "" == root {
		return nil
	}
	free, total, _, err := localDiskUsage(root)
	if nil != err {
		return fmt.Errorf("failed to read disk usage: %w", err)
	}
	facts.DiskFreeBytes = free
	facts.DiskTotalBytes = total
	items, err := scanLocalStaging(root, now)
	if nil != err {
		return err
	}
	facts.StagingCount = len(items)
	for _, item := range items {
		facts.StagingBytes += uint64(item.Bytes)
	}
	return nil
}

func (a *API) erasureFacts(ctx context.Context, facts *findings.Facts) error {
	if nil == a.deps.Erasure || !a.deps.Config.Erasure.Enabled {
		return nil
	}
	plan := a.deps.Erasure.Plan(a.deps.Config.Erasure.DataShards, a.deps.Config.Erasure.ParityShards, a.erasureInstanceIDs())
	facts.ErasureMaxOnOneInstance = plan.MaxOnOneInstance
	summary := a.newErasureSummary()
	if err := a.collectErasureSummary(ctx, &summary); nil != err {
		return err
	}
	facts.ErasureReduced = summary.Health.Reduced
	facts.ErasureCritical = summary.Health.Critical
	return nil
}

func (a *API) telemetryFacts(facts *findings.Facts, now time.Time) error {
	hub := a.deps.Telemetry
	if nil == hub {
		return nil
	}
	failures, err := hub.Replication.Recent(telemetry.MaxPageLimit)
	if nil != err {
		return err
	}
	for _, failure := range failures {
		if !failure.At.Before(now.Add(-recentWindow)) {
			facts.ReplicationFailures1h++
		}
	}
	authPage, err := hub.AuthFailures.List(now.Add(-authFailureWindow), 1)
	if nil != err {
		return err
	}
	facts.AuthFailures10m = authPage.Total
	return auditFacts(hub, facts, now)
}

func auditFacts(hub *telemetry.Hub, facts *findings.Facts, now time.Time) error {
	clients := make(map[string]int)
	total := 0
	err := hub.Audit.Scan(telemetry.AuditQuery{Since: now.Add(-recentWindow)}, func(record *telemetry.AuditRecord) bool {
		total++
		clients[record.Client]++
		if linkOpenOp == record.Op {
			facts.LinkOpens1h++
			if linkFailureStatus <= record.Status {
				facts.LinkFailedOpens1h++
			}
		}
		return true
	})
	if nil != err {
		return err
	}
	top := 0
	for _, count := range clients {
		if top < count {
			top = count
		}
	}
	if 0 < total {
		facts.TopClientShare = float64(top) / float64(total)
	}
	return nil
}

func (a *API) metadataFacts(ctx context.Context, facts *findings.Facts) error {
	store := a.deps.Store
	if nil == store {
		return nil
	}
	owners, err := store.CountEmptyOwners(ctx)
	if nil != err {
		return err
	}
	etags, err := store.CountEmptyETags(ctx)
	if nil != err {
		return err
	}
	facts.FilesNoOwner = owners
	facts.FilesNoETag = etags
	if "postgres" != migrationDriver(a.deps.Config.MetadataStore.Type) {
		return nil
	}
	_, dirty, err := schema.CurrentVersion(ctx, migrationDSN(a.deps.Config))
	if nil != err {
		return err
	}
	facts.MigrationsDirty = dirty
	return nil
}

func (a *API) configFacts(facts *findings.Facts) error {
	data, err := a.readConfigFile()
	if nil != err {
		return err
	}
	saved, err := a.savedConfig(data)
	if nil != err {
		return err
	}
	if nil == saved {
		return nil
	}
	savedValues := config.Flatten(*saved)
	runningValues := config.Flatten(*a.deps.Config)
	for _, key := range config.Schema() {
		if config.IsEnvOnly(key.Path) {
			continue
		}
		if !reflect.DeepEqual(savedValues[key.Path], runningValues[key.Path]) {
			facts.PendingRestart++
		}
	}
	return nil
}

func (a *API) Findings(w http.ResponseWriter, r *http.Request) {
	logger := a.logger()
	if nil == a.deps.Findings {
		a.unavailable(w, "findings engine")
		return
	}
	area := r.URL.Query().Get("area")
	if "" != area && !findings.ValidArea(area) {
		writeFieldProblems(w, []fieldProblem{{Path: "area", Message: "area must be one of security, storage, cluster, config, runtime"}})
		return
	}
	all, err := a.deps.Findings.Evaluate(r.Context())
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, findingsErrorStatus(err))
		return
	}
	selection := findings.Select(all, area, "true" == r.URL.Query().Get("include_muted"))
	handlers.SendJSONResponse(w, findingsResponse{Items: selection.Items, Ignored: selection.Ignored, Counts: selection.Counts})
}

func (a *API) MuteFinding(w http.ResponseWriter, r *http.Request) {
	logger := a.logger()
	if nil == a.deps.Findings {
		a.unavailable(w, "findings engine")
		return
	}
	actor, ok := jobActor(w, r, logger)
	if !ok {
		return
	}
	var req muteRequest
	if err := decodeAdminBody(w, r, &req); nil != err {
		respondBodyError(w, logger, err)
		return
	}
	if 0 > req.DurationS || maxMuteSeconds < req.DurationS {
		writeFieldProblems(w, []fieldProblem{{Path: "duration_s", Message: "duration_s must be from 60 to 604800 seconds"}})
		return
	}
	finding, err := a.deps.Findings.Mute(r.Context(), chi.URLParam(r, "id"), actor, time.Duration(req.DurationS)*time.Second, req.Reason)
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, findingsErrorStatus(err))
		return
	}
	handlers.SendJSONResponse(w, finding)
}

func (a *API) UnmuteFinding(w http.ResponseWriter, r *http.Request) {
	logger := a.logger()
	if nil == a.deps.Findings {
		a.unavailable(w, "findings engine")
		return
	}
	finding, err := a.deps.Findings.Unmute(r.Context(), chi.URLParam(r, "id"))
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, findingsErrorStatus(err))
		return
	}
	handlers.SendJSONResponse(w, finding)
}

func (a *API) IgnoreFinding(w http.ResponseWriter, r *http.Request) {
	logger := a.logger()
	if nil == a.deps.Findings {
		a.unavailable(w, "findings engine")
		return
	}
	actor, ok := jobActor(w, r, logger)
	if !ok {
		return
	}
	var req ignoreRequest
	if err := decodeAdminBody(w, r, &req); nil != err {
		respondBodyError(w, logger, err)
		return
	}
	finding, err := a.deps.Findings.Ignore(r.Context(), chi.URLParam(r, "id"), actor, req.Reason)
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, findingsErrorStatus(err))
		return
	}
	handlers.SendJSONResponse(w, finding)
}

func (a *API) UnignoreFinding(w http.ResponseWriter, r *http.Request) {
	logger := a.logger()
	if nil == a.deps.Findings {
		a.unavailable(w, "findings engine")
		return
	}
	if err := a.deps.Findings.Unignore(r.Context(), chi.URLParam(r, "id")); nil != err {
		handlers.SendErrorResponse(w, logger, err, findingsErrorStatus(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func findingsErrorStatus(err error) int {
	switch {
	case errors.Is(err, findings.ErrUnknownFinding):
		return http.StatusNotFound
	case errors.Is(err, findings.ErrFindingNotActive):
		return http.StatusConflict
	case errors.Is(err, findings.ErrInvalidMute), errors.Is(err, findings.ErrInvalidIgnore):
		return http.StatusUnprocessableEntity
	case errors.Is(err, findings.ErrNoConfig), errors.Is(err, findings.ErrNoMuteStore), errors.Is(err, findings.ErrNoIgnoreStore):
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}
