package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/internal/pathutil"
	"github.com/ebogdum/callfs/jobs"
	"github.com/ebogdum/callfs/locks"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/server/middleware"
)

const (
	batchMaxOps       = 10000
	batchSyncMaxOps   = 100
	batchBodyLimit    = 4 << 20
	batchModeSync     = "sync"
	batchModeJob      = "job"
	batchKind         = "batch"
	batchStatusOK     = "ok"
	batchStatusFailed = "failed"
	batchCodeOK       = "OK"
)

var (
	errBatchInvalid   = errors.New("invalid batch operation")
	errBatchForbidden = errors.New("batch operation not allowed")
	batchModePattern  = regexp.MustCompile(`^0?[0-7]{3}$`)
)

type batchOp struct {
	Op            string `json:"op"`
	Path          string `json:"path"`
	Destination   string `json:"destination"`
	Name          string `json:"name"`
	Mode          string `json:"mode"`
	Recursive     bool   `json:"recursive"`
	Overwrite     bool   `json:"overwrite"`
	CreateParents bool   `json:"create_parents"`
	Storage       string `json:"storage"`
	Instance      string `json:"instance"`
}

type batchRequest struct {
	Ops         []batchOp `json:"ops"`
	Mode        string    `json:"mode"`
	StopOnError bool      `json:"stop_on_error"`
}

type batchResult struct {
	Index   int    `json:"index"`
	Status  string `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path"`
}

type batchResponse struct {
	Results []batchResult `json:"results"`
}

type batchJobResponse struct {
	JobID string `json:"job_id"`
}

type batchJobResult struct {
	Processed int `json:"processed"`
	Failed    int `json:"failed"`
}

type batchItem struct {
	index int
	op    batchOp
}

type batchExecutor struct {
	userID     string
	engine     *core.Engine
	authorizer auth.Authorizer
	trash      *core.Trash
}

func V1Batch(engine *core.Engine, authorizer auth.Authorizer, runner *jobs.Runner, trash *core.Trash) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := trash.Logger()
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		var req batchRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, batchBodyLimit)).Decode(&req); nil != err {
			sendBadRequest(w, logger, "invalid JSON body")
			return
		}
		if err := validateBatchRequest(req); nil != err {
			sendBadRequest(w, logger, err.Error())
			return
		}
		items := make([]batchItem, len(req.Ops))
		for index, op := range req.Ops {
			items[index] = batchItem{index: index, op: op}
		}
		executor := batchExecutor{userID: userID, engine: engine, authorizer: authorizer, trash: trash}

		if batchModeJob == req.Mode {
			job := runner.Submit(batchKind, userID, executor.job(items, req.StopOnError))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			if err := json.NewEncoder(w).Encode(batchJobResponse{JobID: job.ID}); nil != err {
				logger.Error("Failed to encode job response", zap.Error(err))
			}
			logger.Info("Batch job submitted", zap.String("job_id", job.ID), zap.String("user_id", userID), zap.Int("ops", len(items)))
			return
		}

		results := make([]batchResult, 0, len(items))
		executor.execute(r.Context(), items, req.StopOnError, func(result batchResult) {
			results = append(results, result)
		})
		SendJSONResponse(w, batchResponse{Results: results})
		logger.Info("Batch completed", zap.String("user_id", userID), zap.Int("ops", len(items)), zap.Int("results", len(results)))
	}
}

func validateBatchRequest(req batchRequest) error {
	count := len(req.Ops)
	if 0 == count || batchMaxOps < count {
		return fmt.Errorf("ops must contain 1 to %d items", batchMaxOps)
	}
	switch req.Mode {
	case "", batchModeSync:
		if batchSyncMaxOps < count {
			return fmt.Errorf("sync batches accept at most %d ops, use mode job", batchSyncMaxOps)
		}
	case batchModeJob:
	default:
		return errors.New("mode must be sync or job")
	}
	for index, op := range req.Ops {
		if err := validateBatchOp(op); nil != err {
			return fmt.Errorf("op %d: %s", index, err.Error())
		}
	}
	return nil
}

func validateBatchOp(op batchOp) error {
	switch op.Op {
	case "restore":
		if "" == op.Path {
			return errors.New("restore needs path, the trash id")
		}
		return nil
	case "delete", "trash", "move", "rename", "mode":
	default:
		return errors.New("op must be delete, trash, move, rename, mode or restore")
	}
	if "" == op.Path {
		return errors.New("path is required")
	}
	switch op.Op {
	case "move":
		if "" == op.Destination {
			return errors.New("move needs destination")
		}
	case "rename":
		if "" == op.Name {
			return errors.New("rename needs name")
		}
	case "mode":
		if !batchModePattern.MatchString(op.Mode) {
			return errors.New("mode must be an octal value such as 0644")
		}
	}
	return nil
}

func (x batchExecutor) execute(ctx context.Context, items []batchItem, stopOnError bool, record func(batchResult)) {
	for _, item := range items {
		if nil != ctx.Err() {
			return
		}
		err := x.apply(ctx, item.op)
		result := batchResult{Index: item.index, Status: batchStatusOK, Code: batchCodeOK, Path: item.op.Path}
		if nil != err {
			result.Status = batchStatusFailed
			result.Code, result.Message = batchErrorFields(err)
		}
		record(result)
		if nil != err && stopOnError {
			return
		}
	}
}

func (x batchExecutor) job(items []batchItem, stopOnError bool) jobs.JobFunc {
	return func(ctx context.Context, p *jobs.Progress) (any, error) {
		selected := items
		if retrying := p.Retrying(); 0 < len(retrying) {
			selected = batchItemsForRetry(items, retrying)
		}
		p.SetTotal(len(selected))
		var outcome batchJobResult
		x.execute(ctx, selected, stopOnError, func(result batchResult) {
			outcome.Processed++
			p.Advance(1)
			if batchStatusFailed == result.Status {
				outcome.Failed++
				p.Fail(jobs.FailedItem{Path: result.Path, Code: result.Code, Reason: result.Message})
			}
		})
		return outcome, nil
	}
}

func batchItemsForRetry(items []batchItem, failed []jobs.FailedItem) []batchItem {
	paths := make(map[string]struct{}, len(failed))
	for _, item := range failed {
		paths[item.Path] = struct{}{}
	}
	selected := make([]batchItem, 0, len(failed))
	for _, item := range items {
		if _, ok := paths[item.op.Path]; ok {
			selected = append(selected, item)
		}
	}
	return selected
}

func (x batchExecutor) apply(ctx context.Context, op batchOp) error {
	if "restore" == op.Op {
		return x.restore(ctx, op)
	}
	path, err := batchPath(op.Path)
	if nil != err {
		return err
	}
	switch op.Op {
	case "delete":
		return x.deletePath(ctx, path, op.Recursive)
	case "trash":
		return x.trashPath(ctx, path)
	case "move":
		return x.move(ctx, path, op)
	case "rename":
		return x.rename(ctx, path, op.Name)
	default:
		return x.changeMode(ctx, path, op.Mode)
	}
}

func batchPath(raw string) (string, error) {
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	if err := pathutil.ValidatePath(raw); nil != err {
		return "", fmt.Errorf("%w: path is not valid", errBatchInvalid)
	}
	path, err := canonicalDestination(raw)
	if nil != err {
		return "", fmt.Errorf("%w: %s", errBatchInvalid, err.Error())
	}
	if "/" == path {
		return "", fmt.Errorf("%w: the root directory cannot be changed", errBatchInvalid)
	}
	return path, nil
}

func (x batchExecutor) lookup(ctx context.Context, path string, perm auth.PermissionType) (*metadata.Metadata, error) {
	md, err := x.engine.GetMetadata(ctx, path)
	if nil != err && !errors.Is(err, metadata.ErrNotFound) {
		return nil, err
	}
	if err := x.authorizer.AuthorizeWithMetadata(ctx, x.userID, path, md, perm); nil != err {
		return nil, err
	}
	if nil == md {
		return nil, metadata.ErrNotFound
	}
	return md, nil
}

func (x batchExecutor) deletePath(ctx context.Context, path string, recursive bool) error {
	md, err := x.lookup(ctx, path, auth.DeletePerm)
	if nil != err {
		return err
	}
	if nil != md.CallFSInstanceID && x.engine.GetCurrentInstanceID() != *md.CallFSInstanceID {
		if err := x.engine.DeleteFileOnInstance(ctx, *md.CallFSInstanceID, path); nil != err {
			return err
		}
		return x.engine.DeleteMetadataOnly(ctx, path)
	}
	if "directory" == md.Type && recursive {
		_, _, err := x.engine.DeleteDirectoryRecursive(ctx, path)
		return err
	}
	return x.engine.DeleteFile(ctx, path)
}

func (x batchExecutor) trashPath(ctx context.Context, path string) error {
	if _, err := x.lookup(ctx, path, auth.DeletePerm); nil != err {
		return err
	}
	_, err := x.trash.MoveToTrash(ctx, x.userID, path)
	return err
}

func (x batchExecutor) move(ctx context.Context, src string, op batchOp) error {
	dst, err := resolveDestination(src, renameMoveRequest{Destination: op.Destination})
	if nil != err {
		return fmt.Errorf("%w: %s", errBatchInvalid, err.Error())
	}
	if !isAdminAppUser(x.userID) && ("" != op.Storage || "" != op.Instance) {
		return fmt.Errorf("%w: moving to a different storage or instance requires admin", errBatchForbidden)
	}
	if _, err := x.lookup(ctx, src, auth.DeletePerm); nil != err {
		return err
	}
	if err := x.authorizer.Authorize(ctx, x.userID, dst, auth.WritePerm); nil != err {
		return err
	}
	return x.engine.Move(ctx, src, dst, core.MoveOptions{
		Backend:       op.Storage,
		Instance:      op.Instance,
		Overwrite:     op.Overwrite,
		CreateParents: op.CreateParents,
	})
}

func (x batchExecutor) rename(ctx context.Context, src, name string) error {
	dst, err := resolveDestination(src, renameMoveRequest{Name: name})
	if nil != err {
		return fmt.Errorf("%w: %s", errBatchInvalid, err.Error())
	}
	if _, err := x.lookup(ctx, src, auth.DeletePerm); nil != err {
		return err
	}
	if err := x.authorizer.Authorize(ctx, x.userID, dst, auth.WritePerm); nil != err {
		return err
	}
	return x.engine.Rename(ctx, src, name)
}

func (x batchExecutor) changeMode(ctx context.Context, path, mode string) error {
	md, err := x.lookup(ctx, path, auth.WritePerm)
	if nil != err {
		return err
	}
	if x.userID != md.Owner && !isAdminAppUser(x.userID) {
		return fmt.Errorf("%w: only the owner or an admin can change the mode", errBatchForbidden)
	}
	md.Mode = mode
	return x.engine.UpdateMetadataOnly(ctx, md)
}

func (x batchExecutor) restore(ctx context.Context, op batchOp) error {
	onConflict := "fail"
	if op.Overwrite {
		onConflict = "replace"
	}
	_, err := x.trash.Restore(ctx, x.userID, op.Path, op.Destination, onConflict)
	return err
}

func batchErrorFields(err error) (string, string) {
	var invalidRename *core.ErrInvalidRename
	var unsupportedMove *core.ErrUnsupportedMove
	var proxyFailed *core.ErrProxyFailed
	switch {
	case errors.Is(err, errBatchInvalid), errors.Is(err, core.ErrTrashInvalid), errors.As(err, &invalidRename), errors.As(err, &unsupportedMove):
		return "BAD_REQUEST", err.Error()
	case errors.Is(err, errBatchForbidden), errors.Is(err, auth.ErrPermissionDenied):
		return "PERMISSION_DENIED", "permission denied"
	case errors.Is(err, metadata.ErrNotFound):
		return "FILE_NOT_FOUND", "file not found"
	case errors.Is(err, metadata.ErrAlreadyExists):
		return "FILE_ALREADY_EXISTS", "file already exists"
	case errors.Is(err, core.ErrDirectoryNotEmpty), errors.Is(err, core.ErrTrashConflict):
		return "CONFLICT", err.Error()
	case errors.Is(err, locks.ErrLockHeld):
		return "RESOURCE_LOCKED", "resource is locked, retry"
	case errors.As(err, &proxyFailed):
		return "BAD_GATEWAY", "owning server failed the request"
	default:
		return "INTERNAL_ERROR", "operation failed"
	}
}
