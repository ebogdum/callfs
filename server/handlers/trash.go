package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/core"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/server/middleware"
)

const (
	trashListDefaultLimit = 200
	trashListMaxLimit     = 1000
	trashRestoreBodyLimit = 4096
)

type trashListResponse struct {
	Items      []core.TrashItem `json:"items"`
	NextCursor *string          `json:"next_cursor"`
	TotalBytes int64            `json:"total_bytes"`
}

type trashRestoreRequest struct {
	To         string `json:"to"`
	OnConflict string `json:"on_conflict"`
}

type trashRestoreResponse struct {
	Path string `json:"path"`
}

type trashEmptyResponse struct {
	JobID string `json:"job_id"`
}

func V1ListTrash(trash *core.Trash) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := trash.Logger()
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		limit, err := parseTrashLimit(r.URL.Query().Get("limit"))
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusBadRequest)
			return
		}
		page, err := trash.List(r.Context(), userID, r.URL.Query().Get("deleted_by"), r.URL.Query().Get("cursor"), limit)
		if nil != err {
			sendTrashError(w, logger, err)
			return
		}
		response := trashListResponse{Items: page.Items, TotalBytes: page.TotalBytes}
		if "" != page.NextCursor {
			response.NextCursor = &page.NextCursor
		}
		SendJSONResponse(w, response)
	}
}

func V1RestoreTrash(trash *core.Trash) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := trash.Logger()
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		body, err := decodeTrashRestoreBody(http.MaxBytesReader(w, r.Body, trashRestoreBodyLimit))
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusBadRequest)
			return
		}
		restoredPath, err := trash.Restore(r.Context(), userID, chi.URLParam(r, "id"), body.To, body.OnConflict)
		if nil != err {
			sendTrashError(w, logger, err)
			return
		}
		SendJSONResponse(w, trashRestoreResponse{Path: restoredPath})
	}
}

func V1DeleteTrashItem(trash *core.Trash) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := trash.Logger()
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		if err := trash.Delete(r.Context(), userID, chi.URLParam(r, "id")); nil != err {
			sendTrashError(w, logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func V1EmptyTrash(trash *core.Trash) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := trash.Logger()
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		job, err := trash.Empty(r.Context(), userID, r.URL.Query().Get("deleted_by"))
		if nil != err {
			sendTrashError(w, logger, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(trashEmptyResponse{JobID: job.ID}); nil != err {
			logger.Error("Failed to encode job response", zap.Error(err))
		}
	}
}

func parseTrashLimit(raw string) (int, error) {
	if "" == raw {
		return trashListDefaultLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if nil != err || 1 > limit || trashListMaxLimit < limit {
		return 0, fmt.Errorf("limit must be between 1 and %d", trashListMaxLimit)
	}
	return limit, nil
}

func decodeTrashRestoreBody(body io.Reader) (trashRestoreRequest, error) {
	var request trashRestoreRequest
	if err := json.NewDecoder(body).Decode(&request); nil != err && !errors.Is(err, io.EOF) {
		return trashRestoreRequest{}, fmt.Errorf("invalid request body: %w", err)
	}
	return request, nil
}

func sendTrashError(w http.ResponseWriter, logger *zap.Logger, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, core.ErrTrashInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, core.ErrTrashConflict):
		status = http.StatusConflict
	case errors.Is(err, auth.ErrPermissionDenied):
		status = http.StatusForbidden
	case errors.Is(err, metadata.ErrNotFound):
		status = http.StatusNotFound
	}
	SendErrorResponse(w, logger, err, status)
}
