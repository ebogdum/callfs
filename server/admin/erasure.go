package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/erasure"
	"github.com/ebogdum/callfs/v2/jobs"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/server/handlers"
	"github.com/ebogdum/callfs/v2/server/middleware"
)

const (
	adminBodyLimitBytes  = 64 << 10
	adminPathMaxBytes    = 4096
	adminInstanceIDMax   = 255
	erasureListPageSize  = 1000
	erasureMaxShards     = 256
	erasurePlacement     = "round_robin"
	erasureVerifyJobKind = "erasure.verify_all"
	erasureVerifyFailure = "VERIFY_FAILED"
)

type fieldProblem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type erasureHealthCounts struct {
	OK         int `json:"ok"`
	Reduced    int `json:"reduced"`
	Critical   int `json:"critical"`
	Unreadable int `json:"unreadable"`
	Unknown    int `json:"unknown"`
}

type erasureSummaryResponse struct {
	Enabled          bool                `json:"enabled"`
	DataShards       int                 `json:"data_shards"`
	ParityShards     int                 `json:"parity_shards"`
	MinFileSize      int64               `json:"min_file_size"`
	ShardStorage     string              `json:"shard_storage"`
	ShardPath        string              `json:"shard_path"`
	Placement        string              `json:"placement"`
	Instances        []string            `json:"instances"`
	Files            int                 `json:"files"`
	Bytes            int64               `json:"bytes"`
	RawBytes         int64               `json:"raw_bytes"`
	Health           erasureHealthCounts `json:"health"`
	PiecesByInstance map[string]int      `json:"pieces_by_instance"`
}

type erasurePlanRequest struct {
	DataShards   int      `json:"data_shards"`
	ParityShards int      `json:"parity_shards"`
	Instances    []string `json:"instances"`
}

type erasureVerifyRequest struct {
	Prefix string `json:"prefix"`
}

type erasureVerifyResult struct {
	Files  int `json:"files"`
	Failed int `json:"failed"`
}

func (a *API) ErasureSummary(w http.ResponseWriter, r *http.Request) {
	logger := a.Deps.Logger
	summary := a.newErasureSummary()
	if nil != a.Deps.Erasure {
		if err := a.collectErasureSummary(r.Context(), &summary); nil != err {
			handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
	}
	handlers.SendJSONResponse(w, summary)
}

func (a *API) ErasurePlan(w http.ResponseWriter, r *http.Request) {
	logger := a.Deps.Logger
	if nil == a.Deps.Erasure {
		writeFeatureDisabled(w, "erasure")
		return
	}
	var req erasurePlanRequest
	if err := decodeAdminBody(w, r, &req); nil != err {
		respondBodyError(w, logger, err)
		return
	}
	if problems := validateErasurePlan(req); 0 < len(problems) {
		writeFieldProblems(w, problems)
		return
	}
	handlers.SendJSONResponse(w, a.Deps.Erasure.Plan(req.DataShards, req.ParityShards, req.Instances))
}

func (a *API) ErasureVerifyAll(w http.ResponseWriter, r *http.Request) {
	logger := a.Deps.Logger
	if nil == a.Deps.Erasure || nil == a.Deps.Jobs {
		writeFeatureDisabled(w, "erasure")
		return
	}
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		handlers.SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
		return
	}
	var req erasureVerifyRequest
	if err := decodeAdminBody(w, r, &req); nil != err && !errors.Is(err, io.EOF) {
		respondBodyError(w, logger, err)
		return
	}
	prefix := req.Prefix
	if "" == prefix {
		prefix = "/"
	}
	if !validAdminPath(prefix) {
		writeFieldProblems(w, []fieldProblem{{Path: "prefix", Message: "prefix must start with / and must not contain .."}})
		return
	}
	job := a.Deps.Jobs.Submit(erasureVerifyJobKind, userID, func(ctx context.Context, p *jobs.Progress) (any, error) {
		return a.verifyErasurePrefix(ctx, prefix, p)
	})
	sendAccepted(w, logger, jobAccepted{JobID: job.ID})
}

func (a *API) newErasureSummary() erasureSummaryResponse {
	cfg := a.Deps.Config.Erasure
	summary := erasureSummaryResponse{
		Enabled:          nil != a.Deps.Erasure,
		DataShards:       cfg.DataShards,
		ParityShards:     cfg.ParityShards,
		ShardStorage:     cfg.ShardBackend,
		Placement:        erasurePlacement,
		Instances:        a.erasureInstanceIDs(),
		PiecesByInstance: map[string]int{},
	}
	if "localfs" == strings.ToLower(strings.TrimSpace(cfg.ShardBackend)) {
		summary.ShardPath = a.Deps.Config.Backend.LocalFSRootPath
	}
	return summary
}

func (a *API) erasureInstanceIDs() []string {
	seen := map[string]struct{}{}
	ids := []string{}
	add := func(id string) {
		if "" == id {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	add(a.Deps.Config.InstanceDiscovery.InstanceID)
	for id := range a.Deps.Config.InstanceDiscovery.PeerEndpoints {
		add(id)
	}
	sort.Strings(ids)
	return ids
}

func (a *API) collectErasureSummary(ctx context.Context, summary *erasureSummaryResponse) error {
	return a.forEachErasureInfo(ctx, "", func(info *metadata.ErasureFileInfo) error {
		summary.Files++
		summary.Bytes += info.OriginalSize
		for _, shard := range info.Shards {
			summary.RawBytes += shard.Size
			summary.PiecesByInstance[shard.InstanceID]++
		}
		health, known := a.Deps.Erasure.LastHealth(info.FilePath)
		switch {
		case !known:
			summary.Health.Unknown++
		case erasure.HealthOK == health.Health:
			summary.Health.OK++
		case erasure.HealthReduced == health.Health:
			summary.Health.Reduced++
		case erasure.HealthCritical == health.Health:
			summary.Health.Critical++
		case erasure.HealthUnreadable == health.Health:
			summary.Health.Unreadable++
		default:
			summary.Health.Unknown++
		}
		return nil
	})
}

func (a *API) forEachErasureInfo(ctx context.Context, prefix string, visit func(*metadata.ErasureFileInfo) error) error {
	cursor := ""
	for {
		infos, next, err := a.Deps.Store.ListErasureInfos(ctx, prefix, cursor, erasureListPageSize)
		if nil != err {
			return fmt.Errorf("failed to list erasure records: %w", err)
		}
		for _, info := range infos {
			if visitErr := visit(info); nil != visitErr {
				return visitErr
			}
		}
		if "" == next || 0 == len(infos) || next == cursor {
			return nil
		}
		cursor = next
	}
}

func (a *API) verifyErasurePrefix(ctx context.Context, prefix string, p *jobs.Progress) (any, error) {
	logger := a.Deps.Logger
	logger.Info("Erasure verification started", zap.String("prefix", prefix))
	result := erasureVerifyResult{}
	err := a.forEachErasureInfo(ctx, prefix, func(info *metadata.ErasureFileInfo) error {
		if ctxErr := ctx.Err(); nil != ctxErr {
			return ctxErr
		}
		result.Files++
		if _, verifyErr := a.Deps.Erasure.Verify(ctx, info.FilePath); nil != verifyErr {
			result.Failed++
			p.Fail(jobs.FailedItem{Path: info.FilePath, Code: erasureVerifyFailure, Reason: verifyErr.Error()})
		}
		p.Advance(1)
		return nil
	})
	if nil != err {
		logger.Error("Erasure verification failed", zap.String("prefix", prefix), zap.Error(err))
		return result, err
	}
	logger.Info("Erasure verification finished", zap.String("prefix", prefix), zap.Int("files", result.Files), zap.Int("failed", result.Failed))
	return result, nil
}

func validateErasurePlan(req erasurePlanRequest) []fieldProblem {
	var problems []fieldProblem
	if 1 > req.DataShards || erasureMaxShards < req.DataShards {
		problems = append(problems, fieldProblem{Path: "data_shards", Message: "data_shards must be between 1 and 256"})
	}
	if 1 > req.ParityShards || erasureMaxShards < req.ParityShards {
		problems = append(problems, fieldProblem{Path: "parity_shards", Message: "parity_shards must be between 1 and 256"})
	}
	if erasureMaxShards < req.DataShards+req.ParityShards {
		problems = append(problems, fieldProblem{Path: "parity_shards", Message: "data_shards and parity_shards must total at most 256"})
	}
	if 0 == len(req.Instances) {
		problems = append(problems, fieldProblem{Path: "instances", Message: "at least one server is required"})
	}
	if erasureMaxShards < len(req.Instances) {
		problems = append(problems, fieldProblem{Path: "instances", Message: "at most 256 servers can be planned"})
	}
	for index, id := range req.Instances {
		if "" == strings.TrimSpace(id) || adminInstanceIDMax < len(id) {
			problems = append(problems, fieldProblem{Path: fmt.Sprintf("instances[%d]", index), Message: "server id must be 1 to 255 characters"})
		}
	}
	return problems
}

func validAdminPath(value string) bool {
	return strings.HasPrefix(value, "/") &&
		adminPathMaxBytes >= len(value) &&
		!strings.Contains(value, "..") &&
		!strings.ContainsRune(value, 0)
}

func decodeAdminBody(w http.ResponseWriter, r *http.Request, dst any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, adminBodyLimitBytes)).Decode(dst)
}

func respondBodyError(w http.ResponseWriter, logger *zap.Logger, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		handlers.SendErrorResponse(w, logger, err, http.StatusRequestEntityTooLarge)
		return
	}
	handlers.SendErrorResponse(w, logger, err, http.StatusBadRequest)
}

func writeAdminError(w http.ResponseWriter, status int, code, message string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(handlers.ErrorResponse{Code: code, Message: message, Details: details})
}

func writeFeatureDisabled(w http.ResponseWriter, feature string) {
	writeAdminError(w, http.StatusNotFound, "FEATURE_DISABLED", "this feature is not enabled on this server", map[string]any{"feature": feature})
}

func writeFieldProblems(w http.ResponseWriter, problems []fieldProblem) {
	writeAdminError(w, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "request has invalid fields", map[string]any{"errors": problems})
}
