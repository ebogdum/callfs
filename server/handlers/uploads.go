package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/core"
	"github.com/ebogdum/callfs/v2/erasure"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/server/middleware"
	"github.com/ebogdum/callfs/v2/telemetry"
)

const uploadCreateBodyLimit = 64 << 10

var contentRangePattern = regexp.MustCompile(`^bytes (\d+)-(\d+)/(\d+)$`)

type uploadErasureRequest struct {
	DataShards   int      `json:"data_shards"`
	ParityShards int      `json:"parity_shards"`
	Instances    []string `json:"instances"`
}

type uploadCreateRequest struct {
	Path      string                `json:"path"`
	Size      int64                 `json:"size"`
	Overwrite bool                  `json:"overwrite"`
	IfMatch   string                `json:"if_match"`
	Mode      string                `json:"mode"`
	Storage   string                `json:"storage"`
	Erasure   *uploadErasureRequest `json:"erasure"`
}

type uploadCreatedBody struct {
	UploadID  string `json:"upload_id"`
	Offset    int64  `json:"offset"`
	PartBytes int64  `json:"part_bytes"`
	ExpiresAt string `json:"expires_at"`
}

type uploadOffsetBody struct {
	Offset int64 `json:"offset"`
}

type uploadStatusBody struct {
	UploadID  string `json:"upload_id"`
	Path      string `json:"path"`
	Offset    int64  `json:"offset"`
	Size      int64  `json:"size"`
	State     string `json:"state"`
	ExpiresAt string `json:"expires_at"`
}

type uploadCompletedBody struct {
	Path    string `json:"path"`
	ETag    string `json:"etag"`
	Created bool   `json:"created"`
}

type uploadErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func V1CreateUpload(uploads *core.Uploads) http.HandlerFunc {
	logger := uploads.Logger()
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := uploadUserID(w, r, logger)
		if !ok {
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, uploadCreateBodyLimit)
		var body uploadCreateRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); nil != err {
			sendWriteError(w, logger, err, http.StatusBadRequest)
			return
		}

		pathInfo := ParseFilePath(body.Path)
		if pathInfo.IsInvalid || pathInfo.IsDirectory {
			sendBadRequest(w, logger, "upload path must name a file")
			return
		}

		req := core.UploadRequest{
			Path:      pathInfo.FullPath,
			Size:      body.Size,
			Overwrite: body.Overwrite,
			IfMatch:   normalizeUploadIfMatch(body.IfMatch),
			Mode:      body.Mode,
			Storage:   body.Storage,
			NoErasure: "false" == erasureHeaderValue(r),
		}
		if nil != body.Erasure {
			req.Erasure = &erasure.StoreOptions{
				DataShards:   body.Erasure.DataShards,
				ParityShards: body.Erasure.ParityShards,
				Instances:    body.Erasure.Instances,
			}
		}

		session, err := uploads.Create(r.Context(), userID, req)
		if nil != err {
			sendUploadError(w, logger, err)
			return
		}
		writeUploadJSON(w, logger, http.StatusCreated, uploadCreatedBody{
			UploadID:  session.ID,
			Offset:    session.Offset,
			PartBytes: core.UploadPartBytes,
			ExpiresAt: session.ExpiresAt.UTC().Format(time.RFC3339),
		})
		logger.Info("Upload session created",
			zap.String("path", session.Path),
			zap.String("user_id", userID),
			zap.Int64("size", session.Size))
	}
}

func V1PutUploadPart(uploads *core.Uploads) http.HandlerFunc {
	logger := uploads.Logger()
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := uploadUserID(w, r, logger)
		if !ok {
			return
		}

		part, err := parseUploadContentRange(r.Header.Get("Content-Range"))
		if nil != err {
			sendBadRequest(w, logger, err.Error())
			return
		}
		length := part.End - part.Start + 1
		if r.ContentLength < 0 {
			sendBadRequest(w, logger, "Content-Length is required")
			return
		}
		if length != r.ContentLength {
			sendBadRequest(w, logger, "Content-Length must match the Content-Range length")
			return
		}

		id := chi.URLParam(r, "id")
		r.Body = http.MaxBytesReader(w, r.Body, length)
		transferPath := "/uploads/" + id
		if known, statusErr := uploads.Status(userID, id); nil == statusErr {
			transferPath = known.Path
		}
		transfer := telemetry.Transfers.Begin("upload_part", userID, transferPath, transferClient(r), length)
		defer transfer.Finish()
		r.Body = transfer.Body(r.Body)
		session, err := uploads.PutPart(userID, id, part, r.Body)
		if nil != err {
			sendUploadError(w, logger, err)
			return
		}
		writeUploadJSON(w, logger, http.StatusOK, uploadOffsetBody{Offset: session.Offset})
	}
}

func V1GetUpload(uploads *core.Uploads) http.HandlerFunc {
	logger := uploads.Logger()
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := uploadUserID(w, r, logger)
		if !ok {
			return
		}

		session, err := uploads.Status(userID, chi.URLParam(r, "id"))
		if nil != err {
			sendUploadError(w, logger, err)
			return
		}
		writeUploadJSON(w, logger, http.StatusOK, uploadStatusBody{
			UploadID:  session.ID,
			Path:      session.Path,
			Offset:    session.Offset,
			Size:      session.Size,
			State:     string(session.State),
			ExpiresAt: session.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
}

func V1CompleteUpload(uploads *core.Uploads) http.HandlerFunc {
	logger := uploads.Logger()
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := uploadUserID(w, r, logger)
		if !ok {
			return
		}

		id := chi.URLParam(r, "id")
		result, err := uploads.Complete(r.Context(), userID, id)
		if nil != err && !errors.Is(err, core.ErrReplicaCommitPending) {
			sendUploadError(w, logger, err)
			return
		}

		status := http.StatusOK
		if result.Created {
			status = http.StatusCreated
		}
		writeUploadJSON(w, logger, status, uploadCompletedBody{
			Path:    result.Path,
			ETag:    result.ETag,
			Created: result.Created,
		})
		if nil != err {
			logger.Warn("Upload committed with replica commit pending",
				zap.String("upload_id", id),
				zap.String("path", result.Path),
				zap.Error(err))
			return
		}
		logger.Info("Upload completed",
			zap.String("upload_id", id),
			zap.String("path", result.Path),
			zap.String("user_id", userID))
	}
}

func V1AbortUpload(uploads *core.Uploads) http.HandlerFunc {
	logger := uploads.Logger()
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := uploadUserID(w, r, logger)
		if !ok {
			return
		}

		id := chi.URLParam(r, "id")
		if err := uploads.Abort(userID, id); nil != err {
			sendUploadError(w, logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		logger.Info("Upload aborted", zap.String("upload_id", id), zap.String("user_id", userID))
	}
}

func uploadUserID(w http.ResponseWriter, r *http.Request, logger *zap.Logger) (string, bool) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
	}
	return userID, ok
}

func normalizeUploadIfMatch(raw string) string {
	trimmed := strings.TrimSpace(raw)
	return strings.Trim(strings.TrimPrefix(trimmed, "W/"), `"`)
}

func parseUploadContentRange(raw string) (core.UploadPart, error) {
	match := contentRangePattern.FindStringSubmatch(strings.TrimSpace(raw))
	if nil == match {
		return core.UploadPart{}, fmt.Errorf("%w: Content-Range must be bytes start-end/total", core.ErrUploadRange)
	}
	start, startErr := strconv.ParseInt(match[1], 10, 64)
	end, endErr := strconv.ParseInt(match[2], 10, 64)
	total, totalErr := strconv.ParseInt(match[3], 10, 64)
	if nil != errors.Join(startErr, endErr, totalErr) {
		return core.UploadPart{}, fmt.Errorf("%w: Content-Range values are out of range", core.ErrUploadRange)
	}
	return core.UploadPart{Start: start, End: end, Total: total}, nil
}

func sendUploadError(w http.ResponseWriter, logger *zap.Logger, err error) {
	var offsetErr *core.UploadOffsetError
	switch {
	case errors.As(err, &offsetErr):
		writeUploadError(w, logger, http.StatusConflict, "CONFLICT", offsetErr.Error(),
			map[string]any{"offset": offsetErr.Offset})
	case errors.Is(err, core.ErrUploadNotFound):
		writeUploadError(w, logger, http.StatusNotFound, "NOT_FOUND", "upload not found", nil)
	case errors.Is(err, core.ErrUploadExpired):
		writeUploadError(w, logger, http.StatusNotFound, "NOT_FOUND", "upload expired, start a new upload", nil)
	case errors.Is(err, core.ErrUploadNotOpen), errors.Is(err, core.ErrUploadIncomplete):
		writeUploadError(w, logger, http.StatusConflict, "CONFLICT", err.Error(), nil)
	case errors.Is(err, core.ErrUploadExists):
		writeUploadError(w, logger, http.StatusConflict, "FILE_ALREADY_EXISTS", "file already exists", nil)
	case errors.Is(err, core.ErrUploadPathIsDirectory):
		writeUploadError(w, logger, http.StatusConflict, "CONFLICT", "path is a directory", nil)
	case errors.Is(err, core.ErrUploadTooLarge):
		writeUploadError(w, logger, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "upload exceeds size limit", nil)
	case errors.Is(err, core.ErrUploadRange), errors.Is(err, core.ErrUploadInvalid):
		writeUploadError(w, logger, http.StatusBadRequest, "BAD_REQUEST", err.Error(), nil)
	case errors.Is(err, metadata.ErrPrecondition):
		writeUploadError(w, logger, http.StatusPreconditionFailed, "PRECONDITION_FAILED", "precondition failed", nil)
	default:
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
	}
}

func writeUploadError(w http.ResponseWriter, logger *zap.Logger, status int, code, message string, details map[string]any) {
	writeUploadJSON(w, logger, status, uploadErrorBody{Code: code, Message: message, Details: details})
	logLevel := logger.Debug
	if status >= 500 {
		logLevel = logger.Error
	}
	logLevel("Error response sent",
		zap.String("error_code", code),
		zap.Int("status_code", status))
}

func writeUploadJSON(w http.ResponseWriter, logger *zap.Logger, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); nil != err {
		logger.Error("Failed to write upload response", zap.Error(err))
	}
}
