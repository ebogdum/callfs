package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/erasure"
	"github.com/ebogdum/callfs/jobs"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/server/middleware"
)

const (
	erasurePathMaxBytes       = 4096
	erasureRepairJobKind      = "erasure_repair"
	erasureRepairFailureCode  = "REPAIR_FAILED"
	erasureFeatureName        = "erasure"
	erasureShardDataKind      = "data"
	erasureShardParityKind    = "parity"
	erasureFeatureDisabledMsg = "this feature is not enabled on this server"
)

const erasureShardUnknown erasure.ShardState = "unknown"

type erasureRecordStore interface {
	metadata.ErasureMetadataStore
	ListErasureInfos(ctx context.Context, prefix, cursor string, limit int) ([]*metadata.ErasureFileInfo, string, error)
}

type erasureFileItem struct {
	Path          string         `json:"path"`
	OriginalSize  int64          `json:"original_size"`
	DataShards    int            `json:"data_shards"`
	ParityShards  int            `json:"parity_shards"`
	ShardSize     int64          `json:"shard_size"`
	ShardsTotal   int            `json:"shards_total"`
	HealthyShards *int           `json:"healthy_shards"`
	LastCheckedAt *string        `json:"last_checked_at"`
	Health        erasure.Health `json:"health"`
}

type erasureFileListResponse struct {
	Items      []erasureFileItem `json:"items"`
	NextCursor *string           `json:"next_cursor"`
}

type erasureProfile struct {
	DataShards   int   `json:"data_shards"`
	ParityShards int   `json:"parity_shards"`
	ShardSize    int64 `json:"shard_size"`
}

type erasureShardDetail struct {
	Index      int                `json:"index"`
	Kind       string             `json:"kind"`
	InstanceID string             `json:"instance_id"`
	Storage    string             `json:"storage"`
	Path       string             `json:"path"`
	Size       int64              `json:"size"`
	Checksum   string             `json:"checksum"`
	State      erasure.ShardState `json:"state"`
}

type erasureFileDetail struct {
	Path          string               `json:"path"`
	OriginalSize  int64                `json:"original_size"`
	Profile       erasureProfile       `json:"profile"`
	Shards        []erasureShardDetail `json:"shards"`
	Tolerance     int                  `json:"tolerance"`
	LastCheckedAt *string              `json:"last_checked_at"`
}

type erasureRepairFailure struct {
	Index int    `json:"index"`
	Error string `json:"error"`
}

type erasureRepairResult struct {
	Repaired []int                  `json:"repaired"`
	Failed   []erasureRepairFailure `json:"failed"`
}

type erasureRepairAccepted struct {
	JobID string `json:"job_id"`
}

func V1ListErasureFiles(store erasureRecordStore, em *erasure.Manager, authorizer auth.Authorizer, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if erasureUnavailable(store, em) {
			writeErasureFeatureDisabled(w, logger)
			return
		}
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		query := r.URL.Query()
		prefix := query.Get("prefix")
		if erasurePathMaxBytes < len(prefix) {
			SendErrorResponse(w, logger, fmt.Errorf("prefix is too long"), http.StatusBadRequest)
			return
		}
		limit, err := parseTrashLimit(query.Get("limit"))
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusBadRequest)
			return
		}
		healthFilter, err := parseErasureHealthFilter(query.Get("health"))
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusBadRequest)
			return
		}
		cursor, err := decodeErasureCursor(query.Get("cursor"))
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusBadRequest)
			return
		}
		infos, next, err := store.ListErasureInfos(r.Context(), prefix, cursor, limit)
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
		items := make([]erasureFileItem, 0, len(infos))
		for _, info := range infos {
			if authErr := authorizer.Authorize(r.Context(), userID, info.FilePath, auth.ReadPerm); nil != authErr {
				if errors.Is(authErr, auth.ErrPermissionDenied) || errors.Is(authErr, metadata.ErrNotFound) {
					continue
				}
				SendErrorResponse(w, logger, authErr, http.StatusInternalServerError)
				return
			}
			fileHealth, known := em.LastHealth(info.FilePath)
			item := erasureFileItemOf(info, fileHealth, known)
			if "" != healthFilter && healthFilter != item.Health {
				continue
			}
			items = append(items, item)
		}
		SendJSONResponse(w, erasureFileListResponse{Items: items, NextCursor: encodeErasureCursor(next)})
	}
}

func V1GetErasureFile(store erasureRecordStore, em *erasure.Manager, authorizer auth.Authorizer, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if erasureUnavailable(store, em) {
			writeErasureFeatureDisabled(w, logger)
			return
		}
		userID, filePath, ok := erasureFileRequest(w, r, logger)
		if !ok {
			return
		}
		if err := authorizer.Authorize(r.Context(), userID, filePath, auth.ReadPerm); nil != err {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}
		info, err := store.GetErasureInfo(r.Context(), filePath)
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
		report, err := em.Verify(r.Context(), filePath)
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
		verdicts := make(map[int]erasure.ShardVerdict, len(report.Shards))
		for _, verdict := range report.Shards {
			verdicts[verdict.Index] = verdict
		}
		shards := make([]erasureShardDetail, 0, len(info.Shards))
		for _, shard := range info.Shards {
			state := erasureShardUnknown
			if verdict, found := verdicts[shard.Index]; found {
				state = verdict.State
			}
			shards = append(shards, erasureShardDetail{
				Index:      shard.Index,
				Kind:       erasureShardKind(shard.Index, info.DataShards),
				InstanceID: shard.InstanceID,
				Storage:    shard.BackendType,
				Path:       shard.Path,
				Size:       shard.Size,
				Checksum:   shard.Checksum,
				State:      state,
			})
		}
		SendJSONResponse(w, erasureFileDetail{
			Path:         filePath,
			OriginalSize: info.OriginalSize,
			Profile: erasureProfile{
				DataShards:   info.DataShards,
				ParityShards: info.ParityShards,
				ShardSize:    info.ShardSize,
			},
			Shards:        shards,
			Tolerance:     info.ParityShards,
			LastCheckedAt: lastCheckedAtOf(em, filePath),
		})
	}
}

func HandleErasureManifest(em *erasure.Manager, authorizer auth.Authorizer, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if nil == em {
			writeErasureFeatureDisabled(w, logger)
			return
		}
		userID, filePath, ok := erasureFileRequest(w, r, logger)
		if !ok {
			return
		}
		if err := authorizer.Authorize(r.Context(), userID, filePath, auth.ReadPerm); nil != err {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}
		manifest, err := em.GetManifest(r.Context(), filePath)
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
		SendJSONResponse(w, manifest)
	}
}

func V1VerifyErasureFile(em *erasure.Manager, authorizer auth.Authorizer, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if nil == em {
			writeErasureFeatureDisabled(w, logger)
			return
		}
		userID, filePath, ok := erasureFileRequest(w, r, logger)
		if !ok {
			return
		}
		if err := authorizer.Authorize(r.Context(), userID, filePath, auth.ReadPerm); nil != err {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}
		logger.Info("Erasure verify started", zap.String("path", filePath), zap.String("user_id", userID))
		report, err := em.Verify(r.Context(), filePath)
		if nil != err {
			logger.Warn("Erasure verify failed", zap.String("path", filePath), zap.Error(err))
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
		logger.Info("Erasure verify finished", zap.String("path", filePath), zap.Bool("readable", report.Readable), zap.Int("healthy", report.Healthy))
		SendJSONResponse(w, report)
	}
}

func V1RepairErasureFile(store erasureRecordStore, runner *jobs.Runner, em *erasure.Manager, authorizer auth.Authorizer, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if erasureUnavailable(store, em) || nil == runner {
			writeErasureFeatureDisabled(w, logger)
			return
		}
		userID, filePath, ok := erasureFileRequest(w, r, logger)
		if !ok {
			return
		}
		if err := authorizer.Authorize(r.Context(), userID, filePath, auth.WritePerm); nil != err {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}
		if _, err := store.GetErasureInfo(r.Context(), filePath); nil != err {
			SendErrorResponse(w, logger, err, http.StatusNotFound)
			return
		}
		job := runner.Submit(erasureRepairJobKind, userID, func(jobCtx context.Context, p *jobs.Progress) (any, error) {
			return runErasureRepair(jobCtx, em, filePath, p, logger)
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		SendJSONResponse(w, erasureRepairAccepted{JobID: job.ID})
	}
}

func runErasureRepair(ctx context.Context, em *erasure.Manager, filePath string, p *jobs.Progress, logger *zap.Logger) (any, error) {
	logger.Info("Erasure repair started", zap.String("path", filePath))
	before, err := em.Verify(ctx, filePath)
	if nil != err {
		logger.Warn("Erasure repair verify failed", zap.String("path", filePath), zap.Error(err))
		return nil, fmt.Errorf("verify before repair: %w", err)
	}
	damaged := damagedShardCount(before)
	p.SetTotal(damaged)
	after := before
	var repairErr error
	if 0 < damaged {
		repairErr = em.RepairFile(ctx, filePath)
		if nil == repairErr {
			after, err = em.Verify(ctx, filePath)
			if nil != err {
				logger.Warn("Erasure repair verify failed", zap.String("path", filePath), zap.Error(err))
				return nil, fmt.Errorf("verify after repair: %w", err)
			}
		}
	}
	result := settleErasureRepair(before, after, repairErr)
	for _, failure := range result.Failed {
		p.Fail(jobs.FailedItem{Path: filePath, Code: erasureRepairFailureCode, Reason: failure.Error})
	}
	p.Advance(damaged)
	logger.Info("Erasure repair finished",
		zap.String("path", filePath),
		zap.Int("repaired", len(result.Repaired)),
		zap.Int("failed", len(result.Failed)))
	return result, nil
}

func settleErasureRepair(before, after erasure.VerifyReport, repairErr error) erasureRepairResult {
	result := erasureRepairResult{Repaired: []int{}, Failed: []erasureRepairFailure{}}
	afterByIndex := make(map[int]erasure.ShardVerdict, len(after.Shards))
	for _, verdict := range after.Shards {
		afterByIndex[verdict.Index] = verdict
	}
	for _, verdict := range before.Shards {
		if erasure.ShardOK == verdict.State {
			continue
		}
		if nil != repairErr {
			result.Failed = append(result.Failed, erasureRepairFailure{Index: verdict.Index, Error: repairErr.Error()})
			continue
		}
		current, found := afterByIndex[verdict.Index]
		switch {
		case !found:
			result.Failed = append(result.Failed, erasureRepairFailure{Index: verdict.Index, Error: "shard not verified after repair"})
		case erasure.ShardOK == current.State:
			result.Repaired = append(result.Repaired, verdict.Index)
		default:
			result.Failed = append(result.Failed, erasureRepairFailure{Index: verdict.Index, Error: "shard still " + string(current.State)})
		}
	}
	return result
}

func damagedShardCount(report erasure.VerifyReport) int {
	damaged := 0
	for _, verdict := range report.Shards {
		if erasure.ShardOK != verdict.State {
			damaged++
		}
	}
	return damaged
}

func erasureFileItemOf(info *metadata.ErasureFileInfo, fileHealth erasure.FileHealth, known bool) erasureFileItem {
	item := erasureFileItem{
		Path:         info.FilePath,
		OriginalSize: info.OriginalSize,
		DataShards:   info.DataShards,
		ParityShards: info.ParityShards,
		ShardSize:    info.ShardSize,
		ShardsTotal:  info.DataShards + info.ParityShards,
		Health:       erasure.HealthUnknown,
	}
	if !known {
		return item
	}
	healthy := fileHealth.HealthyShards
	checked := fileHealth.CheckedAt.UTC().Format(time.RFC3339)
	item.HealthyShards = &healthy
	item.LastCheckedAt = &checked
	item.Health = fileHealth.Health
	return item
}

func lastCheckedAtOf(em *erasure.Manager, filePath string) *string {
	fileHealth, known := em.LastHealth(filePath)
	if !known {
		return nil
	}
	checked := fileHealth.CheckedAt.UTC().Format(time.RFC3339)
	return &checked
}

func erasureShardKind(index, dataShards int) string {
	if index < dataShards {
		return erasureShardDataKind
	}
	return erasureShardParityKind
}

func parseErasureHealthFilter(raw string) (erasure.Health, error) {
	switch erasure.Health(raw) {
	case "":
		return "", nil
	case erasure.HealthOK, erasure.HealthReduced, erasure.HealthCritical, erasure.HealthUnreadable:
		return erasure.Health(raw), nil
	default:
		return "", fmt.Errorf("health must be one of ok, reduced, critical, unreadable")
	}
}

func encodeErasureCursor(next string) *string {
	if "" == next {
		return nil
	}
	encoded := base64.RawURLEncoding.EncodeToString([]byte(next))
	return &encoded
}

func decodeErasureCursor(raw string) (string, error) {
	if "" == raw {
		return "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if nil != err {
		return "", fmt.Errorf("invalid cursor")
	}
	return string(decoded), nil
}

func erasureUnavailable(store erasureRecordStore, em *erasure.Manager) bool {
	return nil == store || nil == em
}

func erasureFileRequest(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (string, string, bool) {
	rawPath := chi.URLParam(r, "*")
	pathInfo := ParseFilePath(rawPath)
	if "" == rawPath || pathInfo.IsInvalid || pathInfo.IsDirectory {
		SendErrorResponse(w, logger, fmt.Errorf("invalid erasure file path"), http.StatusBadRequest)
		return "", "", false
	}
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
		return "", "", false
	}
	return userID, pathInfo.FullPath, true
}

func writeErasureFeatureDisabled(w http.ResponseWriter, logger *zap.Logger) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	body := ErrorResponse{
		Code:    "FEATURE_DISABLED",
		Message: erasureFeatureDisabledMsg,
		Details: map[string]any{"feature": erasureFeatureName},
	}
	if err := json.NewEncoder(w).Encode(body); nil != err {
		logger.Error("Failed to encode feature disabled response", zap.Error(err))
	}
}
