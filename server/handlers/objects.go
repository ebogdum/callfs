package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/core"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/server/middleware"
)

const (
	objectsPageSize     = 1000
	maxCountedObjects   = 1000000
	objectDeleteTimeout = 5 * time.Minute
)

type ObjectInfo struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	ETag  string `json:"etag"`
	MTime string `json:"mtime"`
}

type ObjectListResponse struct {
	Objects        []ObjectInfo `json:"objects"`
	Truncated      bool         `json:"truncated"`
	NextStartAfter string       `json:"next_start_after,omitempty"`
}

type ObjectCountResponse struct {
	Count     int   `json:"count"`
	Bytes     int64 `json:"bytes"`
	Truncated bool  `json:"truncated"`
}

type ObjectDeleteResponse struct {
	Deleted int `json:"deleted"`
}

type recursiveDeleteResponse struct {
	Deleted            int `json:"deleted"`
	DeletedFiles       int `json:"deleted_files"`
	DeletedDirectories int `json:"deleted_directories"`
}

var errFolderNotEmpty = errors.New("folder is not empty")

type ObjectDeletePlan struct {
	WouldDelete int   `json:"would_delete"`
	Bytes       int64 `json:"bytes"`
	Forbidden   int   `json:"forbidden"`
}

func V1ListObjects(engine *core.Engine, authorizer auth.Authorizer, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}

		prefix := r.URL.Query().Get("prefix")

		if "true" == r.URL.Query().Get("count_only") {
			summary, err := countObjects(r.Context(), engine, authorizer, userID, prefix)
			if err != nil {
				status := http.StatusInternalServerError
				if errors.Is(err, errObjectForbidden) {
					status = http.StatusForbidden
				}
				SendErrorResponse(w, logger, err, status)
				return
			}
			SendJSONResponse(w, summary)
			return
		}

		limit := 0
		if limitParam := r.URL.Query().Get("limit"); limitParam != "" {
			parsed, err := strconv.Atoi(limitParam)
			if err != nil {
				SendErrorResponse(w, logger, fmt.Errorf("invalid limit"), http.StatusBadRequest)
				return
			}
			limit = parsed
		}

		startAfter := r.URL.Query().Get("start_after")

		items, truncated, err := engine.ListObjects(r.Context(), prefix, startAfter, limit)
		if err != nil {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}

		if err := authorizeObjects(r.Context(), authorizer, userID, items, auth.ReadPerm); err != nil {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		response := ObjectListResponse{
			Objects:   toObjectInfos(items),
			Truncated: truncated,
		}
		if truncated && len(items) > 0 {
			response.NextStartAfter = items[len(items)-1].Path
		}

		jsonBytes, err := json.Marshal(response)
		if err != nil {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(jsonBytes); err != nil {
			logger.Error("Failed to write object listing response", zap.Error(err))
		}
	}
}

func V1DeleteObjects(engine *core.Engine, authorizer auth.Authorizer, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}

		prefix := r.URL.Query().Get("prefix")
		if prefix == "" {
			SendErrorResponse(w, logger, fmt.Errorf("prefix is required"), http.StatusBadRequest)
			return
		}
		if prefix == "/" {
			SendErrorResponse(w, logger, fmt.Errorf("refusing to delete prefix %q", prefix), http.StatusBadRequest)
			return
		}
		boundary := "true" == r.URL.Query().Get("boundary")

		ctx, cancel := context.WithTimeout(r.Context(), objectDeleteTimeout)
		defer cancel()

		if "true" == r.URL.Query().Get("dry_run") {
			plan, err := planPrefixDelete(ctx, engine, authorizer, userID, prefix, boundary)
			if err != nil {
				SendErrorResponse(w, logger, err, http.StatusInternalServerError)
				return
			}
			SendJSONResponse(w, plan)
			return
		}

		if err := authorizeObjectsUnder(ctx, engine, authorizer, userID, prefix, boundary); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, errObjectForbidden) {
				status = http.StatusForbidden
			}
			SendErrorResponse(w, logger, err, status)
			return
		}

		logger.Info("Prefix delete started",
			zap.String("prefix", prefix),
			zap.Bool("boundary", boundary),
			zap.String("user_id", userID))

		deleted, err := engine.DeletePrefixWithin(ctx, prefix, boundary)
		if err != nil {
			var deleteErr *core.PrefixDeleteError
			if errors.As(err, &deleteErr) {
				sendPrefixDeleteFailure(w, deleteErr)
			} else {
				SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			}
			logger.Error("Prefix delete failed",
				zap.String("prefix", prefix),
				zap.Int("deleted", deleted),
				zap.String("user_id", userID),
				zap.Error(err))
			return
		}

		SendJSONResponse(w, ObjectDeleteResponse{Deleted: deleted})
		logger.Info("Prefix delete finished",
			zap.String("prefix", prefix),
			zap.Int("deleted", deleted),
			zap.String("user_id", userID))
	}
}

func sendPrefixDeleteFailure(w http.ResponseWriter, deleteErr *core.PrefixDeleteError) {
	code, message := genericErrorFields(http.StatusInternalServerError, nil)
	body, err := json.Marshal(ErrorResponse{
		Code:    code,
		Message: message,
		Details: map[string]any{
			"deleted":     deleteErr.Deleted,
			"failed_path": deleteErr.FailedPath,
		},
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	if err != nil {
		return
	}
	_, _ = w.Write(body)
}

var errObjectForbidden = fmt.Errorf("forbidden")

func countObjects(ctx context.Context, engine *core.Engine, authorizer auth.Authorizer, userID, prefix string) (ObjectCountResponse, error) {
	var summary ObjectCountResponse
	err := engine.WalkPrefix(ctx, prefix, false, func(page []*metadata.Metadata) (bool, error) {
		if err := authorizeObjects(ctx, authorizer, userID, page, auth.ReadPerm); err != nil {
			return false, err
		}
		for _, item := range page {
			if maxCountedObjects <= summary.Count {
				summary.Truncated = true
				return false, nil
			}
			summary.Count++
			summary.Bytes += item.Size
		}
		return true, nil
	})
	return summary, err
}

func planPrefixDelete(ctx context.Context, engine *core.Engine, authorizer auth.Authorizer, userID, prefix string, boundary bool) (ObjectDeletePlan, error) {
	var plan ObjectDeletePlan
	err := engine.WalkPrefix(ctx, prefix, boundary, func(page []*metadata.Metadata) (bool, error) {
		for _, item := range page {
			plan.WouldDelete++
			plan.Bytes += item.Size
			err := authorizer.AuthorizeWithMetadata(ctx, userID, item.Path, item, auth.DeletePerm)
			if nil == err {
				continue
			}
			if !errors.Is(err, auth.ErrPermissionDenied) {
				return false, err
			}
			plan.Forbidden++
		}
		return true, nil
	})
	return plan, err
}

func authorizeObjectsUnder(ctx context.Context, engine *core.Engine, authorizer auth.Authorizer, userID, prefix string, boundary bool) error {
	return engine.WalkPrefix(ctx, prefix, boundary, func(page []*metadata.Metadata) (bool, error) {
		if err := authorizeObjects(ctx, authorizer, userID, page, auth.DeletePerm); err != nil {
			return false, err
		}
		return true, nil
	})
}

func authorizeObjects(ctx context.Context, authorizer auth.Authorizer, userID string, items []*metadata.Metadata, perm auth.PermissionType) error {
	for _, item := range items {
		if err := authorizer.AuthorizeWithMetadata(ctx, userID, item.Path, item, perm); err != nil {
			return fmt.Errorf("%w: %w", errObjectForbidden, err)
		}
	}
	return nil
}

func toObjectInfos(items []*metadata.Metadata) []ObjectInfo {
	infos := make([]ObjectInfo, 0, len(items))
	for _, item := range items {
		infos = append(infos, ObjectInfo{
			Path:  item.Path,
			Size:  item.Size,
			ETag:  item.ETag,
			MTime: item.MTime.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return infos
}
