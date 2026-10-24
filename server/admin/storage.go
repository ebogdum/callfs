package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/jobs"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/server/handlers"
	"github.com/ebogdum/callfs/telemetry"
)

const (
	stagingDefaultAgeSeconds = 86400
	stagingMaxAgeSeconds     = 315360000
	stagingListLimit         = 1000
	stagingTempPrefix        = ".callfs-tmp-"
	backupFailureWindow      = 24 * time.Hour
	replicationPathsLimit    = 1000
	adminPathLimit           = 4096
	retagPageSize            = 500
)

type stagingLocation struct {
	dir  string
	kind string
}

var stagingLocations = []stagingLocation{
	{dir: ".callfs-staging", kind: "staging"},
	{dir: ".callfs-uploads", kind: "upload_session"},
	{dir: ".callfs-probe", kind: "probe"},
}

type stagingItem struct {
	Storage  string    `json:"storage"`
	Path     string    `json:"path"`
	Kind     string    `json:"kind"`
	Bytes    int64     `json:"bytes"`
	Modified time.Time `json:"modified"`
	AgeS     int64     `json:"age_s"`
}

type stagingPage struct {
	Items      []stagingItem `json:"items"`
	TotalBytes int64         `json:"total_bytes"`
}

type stagingCleanResult struct {
	Removed    int   `json:"removed"`
	FreedBytes int64 `json:"freed_bytes"`
}

type stagingTotals struct {
	Count int   `json:"count"`
	Bytes int64 `json:"bytes"`
}

type replicationCounts struct {
	OK      *int `json:"ok"`
	Failed  int  `json:"failed"`
	Skipped *int `json:"skipped"`
}

type replicationPage struct {
	Enabled        bool                           `json:"enabled"`
	ReplicaStorage string                         `json:"replica_storage"`
	RequireSuccess bool                           `json:"require_success"`
	Counts24h      replicationCounts              `json:"counts_24h"`
	Failures       []telemetry.ReplicationFailure `json:"failures"`
}

type replicationRetryRequest struct {
	Paths []string `json:"paths"`
}

type replicationRetryResult struct {
	Requested int `json:"requested"`
	Copied    int `json:"copied"`
	Failed    int `json:"failed"`
}

type migrationsView struct {
	Driver  string `json:"driver"`
	Version *int64 `json:"version"`
	Dirty   *bool  `json:"dirty"`
	Latest  *int64 `json:"latest"`
}

type hygieneView struct {
	FilesWithoutOwner int            `json:"files_without_owner"`
	FilesWithoutETag  int            `json:"files_without_etag"`
	OrphanPieces      *int           `json:"orphan_pieces"`
	StaleErasure      *int           `json:"stale_erasure_records"`
	Staging           stagingTotals  `json:"staging"`
	Migrations        migrationsView `json:"migrations"`
}

type retagRequest struct {
	Prefix string `json:"prefix"`
}

type retagResult struct {
	Tagged  int `json:"tagged"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

func (a *API) Replication(w http.ResponseWriter, r *http.Request) {
	hub, ok := a.telemetryHub(w)
	if !ok {
		return
	}
	failures, err := hub.Replication.Recent(telemetry.MaxPageLimit)
	if nil != err {
		a.telemetryError(w, err)
		return
	}
	handlers.SendJSONResponse(w, replicationPage{
		Enabled:        a.deps.Config.HA.ReplicationEnabled,
		ReplicaStorage: a.deps.Config.HA.ReplicaBackend,
		RequireSuccess: a.deps.Config.HA.RequireReplicaSuccess,
		Counts24h: replicationCounts{
			Failed: hub.Replication.CountSince(time.Now().Add(-backupFailureWindow)),
		},
		Failures: failures,
	})
}

func (a *API) RetryReplication(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	actor, ok := jobActor(w, r, logger)
	if !ok {
		return
	}
	var req replicationRetryRequest
	if !decodeTopologyBody(w, r, logger, &req) {
		return
	}
	hub, ok := a.telemetryHub(w)
	if !ok {
		return
	}
	paths, err := a.replicationRetryPaths(hub, req.Paths)
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusBadRequest)
		return
	}
	job := a.deps.Jobs.Submit("replication.retry", actor, func(ctx context.Context, p *jobs.Progress) (any, error) {
		return a.retryReplication(ctx, paths, p, logger)
	})
	sendAccepted(w, logger, jobAccepted{JobID: job.ID})
}

func (a *API) replicationRetryPaths(hub *telemetry.Hub, requested []string) ([]string, error) {
	if nil != requested {
		if replicationPathsLimit < len(requested) {
			return nil, errors.New("at most 1000 paths can be retried at once")
		}
		paths := make([]string, 0, len(requested))
		for _, raw := range requested {
			cleaned, err := normalizeAdminPath(raw)
			if nil != err {
				return nil, err
			}
			paths = append(paths, cleaned)
		}
		return paths, nil
	}
	failures, err := hub.Replication.Recent(telemetry.MaxPageLimit)
	if nil != err {
		return nil, err
	}
	seen := make(map[string]bool, len(failures))
	paths := make([]string, 0, len(failures))
	for _, failure := range failures {
		if seen[failure.Path] {
			continue
		}
		seen[failure.Path] = true
		paths = append(paths, failure.Path)
	}
	return paths, nil
}

func (a *API) retryReplication(ctx context.Context, paths []string, p *jobs.Progress, logger *zap.Logger) (any, error) {
	logger.Info("Backup copy retry started", zap.Int("paths", len(paths)))
	p.SetTotal(len(paths))
	result := replicationRetryResult{Requested: len(paths)}
	for _, filePath := range paths {
		if err := ctx.Err(); nil != err {
			logger.Info("Backup copy retry cancelled", zap.Int("copied", result.Copied), zap.Int("failed", result.Failed))
			return result, err
		}
		if err := a.deps.Engine.ReplicatePath(ctx, filePath); nil != err {
			result.Failed++
			p.Fail(jobs.FailedItem{Path: filePath, Code: "REPLICATION_FAILED", Reason: err.Error()})
			continue
		}
		result.Copied++
		p.Advance(1)
	}
	logger.Info("Backup copy retry finished", zap.Int("copied", result.Copied), zap.Int("failed", result.Failed))
	return result, nil
}

func (a *API) Staging(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	items, err := scanLocalStaging(a.deps.Config.Backend.LocalFSRootPath, time.Now().UTC())
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	var total int64
	for _, item := range items {
		total += item.Bytes
	}
	handlers.SendJSONResponse(w, stagingPage{Items: items[:min(len(items), stagingListLimit)], TotalBytes: total})
}

func (a *API) CleanStaging(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	actor, ok := jobActor(w, r, logger)
	if !ok {
		return
	}
	olderThan, err := parseStagingAge(r.URL.Query().Get("older_than_s"))
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusBadRequest)
		return
	}
	root := a.deps.Config.Backend.LocalFSRootPath
	if "" == root {
		handlers.SendErrorResponse(w, logger, errors.New("local disk storage is not enabled"), http.StatusConflict)
		return
	}
	job := a.deps.Jobs.Submit("staging.clean", actor, func(ctx context.Context, p *jobs.Progress) (any, error) {
		return cleanLocalStaging(ctx, root, olderThan, p, logger)
	})
	sendAccepted(w, logger, jobAccepted{JobID: job.ID})
}

func (a *API) Hygiene(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	ctx := r.Context()
	owners, err := a.deps.Store.CountEmptyOwners(ctx)
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	etags, err := a.deps.Store.CountEmptyETags(ctx)
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	items, err := scanLocalStaging(a.deps.Config.Backend.LocalFSRootPath, time.Now().UTC())
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	staging := stagingTotals{Count: len(items)}
	for _, item := range items {
		staging.Bytes += item.Bytes
	}
	staleErasure, err := a.countStaleErasureRecords(ctx)
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	handlers.SendJSONResponse(w, hygieneView{
		FilesWithoutOwner: owners,
		FilesWithoutETag:  etags,
		StaleErasure:      staleErasure,
		Staging:           staging,
		Migrations:        migrationsView{Driver: migrationDriver(a.deps.Config.MetadataStore.Type)},
	})
}

func (a *API) countStaleErasureRecords(ctx context.Context) (*int, error) {
	stale := 0
	cursor := ""
	for {
		infos, next, err := a.deps.Store.ListErasureInfos(ctx, "/", cursor, instancePageSize)
		if errors.Is(err, metadata.ErrNotImplemented) {
			return nil, nil
		}
		if nil != err {
			return nil, err
		}
		for _, info := range infos {
			_, err := a.deps.Store.Get(ctx, info.FilePath)
			if errors.Is(err, metadata.ErrNotFound) {
				stale++
				continue
			}
			if nil != err {
				return nil, err
			}
		}
		if "" == next || next == cursor {
			return &stale, nil
		}
		cursor = next
	}
}

func (a *API) Retag(w http.ResponseWriter, r *http.Request) {
	logger := zap.L()
	actor, ok := jobActor(w, r, logger)
	if !ok {
		return
	}
	var req retagRequest
	if !decodeTopologyBody(w, r, logger, &req) {
		return
	}
	prefix := "/"
	if "" != req.Prefix {
		cleaned, err := normalizeAdminPath(req.Prefix)
		if nil != err {
			handlers.SendErrorResponse(w, logger, err, http.StatusBadRequest)
			return
		}
		prefix = cleaned
	}
	job := a.deps.Jobs.Submit("retag", actor, func(ctx context.Context, p *jobs.Progress) (any, error) {
		return a.retagFiles(ctx, prefix, p, logger)
	})
	sendAccepted(w, logger, jobAccepted{JobID: job.ID})
}

func (a *API) retagFiles(ctx context.Context, prefix string, p *jobs.Progress, logger *zap.Logger) (any, error) {
	logger.Info("Version tag rebuild started", zap.String("prefix", prefix))
	var result retagResult
	startAfter := ""
	for {
		files, err := a.deps.Store.ListByPrefix(ctx, prefix, startAfter, retagPageSize)
		if nil != err {
			logger.Error("Version tag rebuild failed", zap.Error(err))
			return result, err
		}
		for _, md := range files {
			if "" != md.ETag {
				continue
			}
			if err := ctx.Err(); nil != err {
				logger.Info("Version tag rebuild cancelled", zap.Int("tagged", result.Tagged))
				return result, err
			}
			switch err := a.retagFile(ctx, md); {
			case nil == err:
				result.Tagged++
			case errors.Is(err, metadata.ErrPrecondition):
				result.Skipped++
			default:
				result.Failed++
				p.Fail(jobs.FailedItem{Path: md.Path, Code: "RETAG_FAILED", Reason: err.Error()})
			}
			p.Advance(1)
		}
		if retagPageSize > len(files) {
			break
		}
		startAfter = files[len(files)-1].Path
	}
	logger.Info("Version tag rebuild finished", zap.Int("tagged", result.Tagged), zap.Int("skipped", result.Skipped), zap.Int("failed", result.Failed))
	return result, nil
}

func (a *API) retagFile(ctx context.Context, md *metadata.Metadata) error {
	reader, err := a.deps.Engine.GetFile(ctx, md.Path)
	if nil != err {
		return err
	}
	digest, err := hashContent(reader)
	if nil != err {
		return err
	}
	updated := *md
	updated.ETag = digest
	if err := a.deps.Store.UpdateIfMatch(ctx, &updated, ""); nil != err {
		return err
	}
	a.deps.Engine.MetadataCache().Invalidate(md.Path)
	return nil
}

func hashContent(reader io.ReadCloser) (string, error) {
	hasher := sha256.New()
	_, copyErr := io.Copy(hasher, reader)
	closeErr := reader.Close()
	if err := errors.Join(copyErr, closeErr); nil != err {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func scanLocalStaging(root string, now time.Time) ([]stagingItem, error) {
	items := make([]stagingItem, 0)
	if "" == root {
		return items, nil
	}
	for _, location := range stagingLocations {
		entries, err := os.ReadDir(filepath.Join(root, location.dir))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if nil != err {
			return nil, err
		}
		for _, entry := range entries {
			item, err := stagingEntry(root, path.Join("/", location.dir, entry.Name()), location.kind, now)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if nil != err {
				return nil, err
			}
			items = append(items, item)
		}
	}
	entries, err := os.ReadDir(root)
	if nil != err {
		return nil, err
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), stagingTempPrefix) {
			continue
		}
		item, err := stagingEntry(root, path.Join("/", entry.Name()), "tmp", now)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if nil != err {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func stagingEntry(root, relative, kind string, now time.Time) (stagingItem, error) {
	target, err := stagingTarget(root, relative)
	if nil != err {
		return stagingItem{}, err
	}
	info, err := os.Lstat(target)
	if nil != err {
		return stagingItem{}, err
	}
	size, err := stagingSize(target, info)
	if nil != err {
		return stagingItem{}, err
	}
	modified := info.ModTime().UTC()
	return stagingItem{
		Storage:  "localfs",
		Path:     relative,
		Kind:     kind,
		Bytes:    size,
		Modified: modified,
		AgeS:     int64(now.Sub(modified) / time.Second),
	}, nil
}

func stagingSize(target string, info fs.FileInfo) (int64, error) {
	if !info.IsDir() {
		return info.Size(), nil
	}
	var total int64
	err := filepath.WalkDir(target, func(_ string, entry fs.DirEntry, walkErr error) error {
		if nil != walkErr {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		fileInfo, err := entry.Info()
		if nil != err {
			return err
		}
		total += fileInfo.Size()
		return nil
	})
	return total, err
}

func stagingTarget(root, relative string) (string, error) {
	cleaned := path.Clean("/" + relative)
	if "/" == cleaned {
		return "", errors.New("staging path must name an entry")
	}
	target := filepath.Join(root, filepath.FromSlash(cleaned))
	if !strings.HasPrefix(target, filepath.Clean(root)+string(filepath.Separator)) {
		return "", errors.New("staging path escapes the storage root")
	}
	return target, nil
}

func cleanLocalStaging(ctx context.Context, root string, olderThan time.Duration, p *jobs.Progress, logger *zap.Logger) (stagingCleanResult, error) {
	logger.Info("Staging cleanup started", zap.Duration("older_than", olderThan))
	now := time.Now().UTC()
	items, err := scanLocalStaging(root, now)
	if nil != err {
		logger.Error("Staging cleanup failed", zap.Error(err))
		return stagingCleanResult{}, err
	}
	eligible := make([]stagingItem, 0, len(items))
	for _, item := range items {
		if now.Sub(item.Modified) >= olderThan {
			eligible = append(eligible, item)
		}
	}
	p.SetTotal(len(eligible))
	var result stagingCleanResult
	for _, item := range eligible {
		if err := ctx.Err(); nil != err {
			logger.Info("Staging cleanup cancelled", zap.Int("removed", result.Removed))
			return result, err
		}
		target, err := stagingTarget(root, item.Path)
		if nil != err {
			p.Fail(jobs.FailedItem{Path: item.Path, Code: "INVALID_PATH", Reason: err.Error()})
			continue
		}
		if err := os.RemoveAll(target); nil != err {
			p.Fail(jobs.FailedItem{Path: item.Path, Code: "DELETE_FAILED", Reason: err.Error()})
			continue
		}
		result.Removed++
		result.FreedBytes += item.Bytes
		p.Advance(1)
	}
	logger.Info("Staging cleanup finished", zap.Int("removed", result.Removed), zap.Int64("freed_bytes", result.FreedBytes))
	return result, nil
}

func parseStagingAge(raw string) (time.Duration, error) {
	if "" == raw {
		return stagingDefaultAgeSeconds * time.Second, nil
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if nil != err || 0 > seconds || stagingMaxAgeSeconds < seconds {
		return 0, errors.New("older_than_s must be a whole number of seconds between 0 and 315360000")
	}
	return time.Duration(seconds) * time.Second, nil
}

func normalizeAdminPath(raw string) (string, error) {
	if adminPathLimit < len(raw) || !strings.HasPrefix(raw, "/") || strings.ContainsRune(raw, 0) {
		return "", errors.New("path must start with / and be at most 4096 bytes")
	}
	return path.Clean(raw), nil
}

func migrationDriver(storeType string) string {
	if "postgres" == storeType {
		return "postgres"
	}
	return "none"
}
