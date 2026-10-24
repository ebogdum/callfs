package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/core"
	"github.com/ebogdum/callfs/v2/erasure"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/server/middleware"
)

func erasureOptionValue(r *http.Request, header, query string) string {
	if v := r.Header.Get(header); "" != v {
		return v
	}
	return r.URL.Query().Get(query)
}

func parseErasureShardCount(name, raw string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if nil != err || 1 > n {
		return 0, &customError{message: name + " must be an integer of at least 1"}
	}
	return n, nil
}

func parseErasureInstances(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	instances := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if "" == trimmed {
			return nil, &customError{message: "erasure instance list must not contain empty entries"}
		}
		instances = append(instances, trimmed)
	}
	return instances, nil
}

func parseErasureOptions(r *http.Request) (*erasure.StoreOptions, error) {
	opts := &erasure.StoreOptions{}

	if v := erasureOptionValue(r, "X-CallFS-Erasure-Data-Shards", "data_shards"); "" != v {
		n, err := parseErasureShardCount("erasure data shard count", v)
		if nil != err {
			return nil, err
		}
		opts.DataShards = n
	}

	if v := erasureOptionValue(r, "X-CallFS-Erasure-Parity-Shards", "parity_shards"); "" != v {
		n, err := parseErasureShardCount("erasure parity shard count", v)
		if nil != err {
			return nil, err
		}
		opts.ParityShards = n
	}

	if v := erasureOptionValue(r, "X-CallFS-Erasure-Instances", "instances"); "" != v {
		instances, err := parseErasureInstances(v)
		if nil != err {
			return nil, err
		}
		opts.Instances = instances
	}

	return opts, nil
}

const (
	maxErasureShards    = 255
	maxErasureInstances = 64
)

func validateErasureOptions(opts *erasure.StoreOptions) error {
	if maxErasureShards < opts.DataShards || maxErasureShards < opts.ParityShards {
		return &customError{message: "erasure data and parity shard counts must not exceed 255"}
	}
	if maxErasureShards < opts.DataShards+opts.ParityShards {
		return &customError{message: "erasure total shard count must not exceed 255"}
	}
	if maxErasureInstances < len(opts.Instances) {
		return &customError{message: "erasure instance list must not exceed 64 entries"}
	}
	return nil
}

// CrossServerConflictResponse represents a response when a file exists on another server
type CrossServerConflictResponse struct {
	Error        string `json:"error"`
	ExistingPath string `json:"existing_path"`
	InstanceID   string `json:"instance_id"`
	BackendType  string `json:"backend_type"`
	Suggestion   string `json:"suggestion"`
}

// V1PostFileEnhanced handles POST /files/{path} requests with enhanced cross-server conflict detection
// @Summary Create or update file or directory with cross-server conflict detection
// @Description Creates a new file or directory. If the resource exists on another server, provides proper conflict resolution
// @Tags files
// @Security BearerAuth
// @Param path path string true "File or directory path"
// @Param file body string false "File content (for files) or directory creation request"
// @Success 201 "Created"
// @Success 200 "OK (directory already exists)"
// @Failure 400 {object} ErrorResponse "Bad Request"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Forbidden"
// @Failure 409 {object} CrossServerConflictResponse "Conflict - resource exists on another server"
// @Failure 500 {object} ErrorResponse "Internal Server Error"
// @Router /v1/files/{path} [post]
func handleCrossServerConflict(w http.ResponseWriter, engine *core.Engine, existingMd *metadata.Metadata, pathInfo PathInfo, enginePath string) {
	conflictResponse := CrossServerConflictResponse{
		Error:        "Resource exists on another server",
		ExistingPath: existingMd.Path,
		InstanceID:   *existingMd.CallFSInstanceID,
		BackendType:  existingMd.BackendType,
	}

	if pathInfo.IsDirectory {
		conflictResponse.Suggestion = "Directory already exists on another server. Use GET to access it."
		if existingMd.Type != "directory" {
			conflictResponse.Suggestion = "Path exists as file on another server, cannot create directory"
		}
	} else {
		conflictResponse.Suggestion = "File already exists on another server. Use PUT to update it."
		if existingMd.Type != "file" {
			conflictResponse.Suggestion = "Path exists as directory on another server, cannot create file"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	buf, _ := json.Marshal(conflictResponse)
	_, _ = w.Write(buf)
}

func respondExistingConflict(w http.ResponseWriter, engine *core.Engine, existingMd *metadata.Metadata, pathInfo PathInfo, enginePath string, logger *zap.Logger) {
	if existingMd.CallFSInstanceID != nil && *existingMd.CallFSInstanceID != engine.GetCurrentInstanceID() {
		handleCrossServerConflict(w, engine, existingMd, pathInfo, enginePath)
		return
	}
	handleLocalConflict(w, logger, existingMd, pathInfo)
}

func handleLocalConflict(w http.ResponseWriter, logger *zap.Logger, existingMd *metadata.Metadata, pathInfo PathInfo) {
	if pathInfo.IsDirectory {
		if existingMd.Type != "directory" {
			SendErrorResponse(w, logger, &customError{message: "path exists as file, cannot create directory"}, http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	if existingMd.Type != "file" {
		SendErrorResponse(w, logger, &customError{message: "path exists as directory, cannot create file"}, http.StatusConflict)
		return
	}
	SendErrorResponse(w, logger, &customError{message: "file already exists, use PUT to update"}, http.StatusConflict)
}

func handleErasureUpload(w http.ResponseWriter, r *http.Request, engine *core.Engine, enginePath string, pathInfo PathInfo, userID string, logger *zap.Logger) {
	const maxErasureUpload = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxErasureUpload)
	hasher := sha256.New()
	opts, parseErr := parseErasureOptions(r)
	if nil != parseErr {
		SendErrorResponse(w, logger, parseErr, http.StatusBadRequest)
		return
	}
	if validErr := validateErasureOptions(opts); nil != validErr {
		SendErrorResponse(w, logger, validErr, http.StatusBadRequest)
		return
	}

	committedMd, err := engine.CreateErasureFile(r.Context(), enginePath, io.TeeReader(r.Body, hasher), opts, func(info *erasure.ErasureFileInfo) *metadata.Metadata {
		return &metadata.Metadata{
			Name:         pathInfo.Name,
			Type:         "file",
			Size:         info.OriginalSize,
			Mode:         "0644",
			Owner:        userID,
			BackendType:  "erasure",
			ErasureCoded: true,
			ATime:        time.Now(),
			MTime:        time.Now(),
			CTime:        time.Now(),
			ETag:         hex.EncodeToString(hasher.Sum(nil)),
		}
	})
	if err != nil {
		if errors.Is(err, metadata.ErrPrecondition) {
			if existingMd, lookupErr := engine.GetMetadata(r.Context(), enginePath); lookupErr == nil {
				respondExistingConflict(w, engine, existingMd, pathInfo, enginePath, logger)
				return
			}
			SendErrorResponse(w, logger, &customError{message: "file already exists, use PUT to update"}, http.StatusConflict)
			return
		}
		if errors.Is(err, erasure.ErrEmptyUpload) {
			SendErrorResponse(w, logger, err, http.StatusBadRequest)
			return
		}
		sendWriteError(w, logger, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("ETag", quotedETag(committedMd.ETag))
	w.WriteHeader(http.StatusCreated)
	logger.Info("Erasure-coded file created",
		zap.String("path", pathInfo.FullPath),
		zap.String("user_id", userID),
		zap.Int64("size", committedMd.Size))
}

func handleRegularFileCreate(w http.ResponseWriter, r *http.Request, engine *core.Engine, enginePath string, pathInfo PathInfo, userID string, backendConfig *config.BackendConfig, logger *zap.Logger) {
	size := r.ContentLength
	if size < 0 {
		size = 0
	}

	const maxUploadBytes = 10 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	md := &metadata.Metadata{
		Name:        pathInfo.Name,
		Type:        "file",
		Mode:        "0644",
		Owner:       userID,
		BackendType: backendConfig.DefaultBackend,
		ATime:       time.Now(),
		MTime:       time.Now(),
		CTime:       time.Now(),
	}

	committedMd, _, err := engine.PutFile(r.Context(), enginePath, r.Body, size, md, "", true)
	if err != nil {
		if errors.Is(err, metadata.ErrPrecondition) {
			if existingMd, lookupErr := engine.GetMetadata(r.Context(), enginePath); lookupErr == nil {
				respondExistingConflict(w, engine, existingMd, pathInfo, enginePath, logger)
				return
			}
			SendErrorResponse(w, logger, &customError{message: "file already exists, use PUT to update"}, http.StatusConflict)
			return
		}
		sendWriteError(w, logger, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("ETag", quotedETag(committedMd.ETag))
	w.WriteHeader(http.StatusCreated)
	logger.Info("File created",
		zap.String("path", pathInfo.FullPath),
		zap.String("user_id", userID),
		zap.Int64("size", committedMd.Size))
}

func V1PostFileEnhanced(engine *core.Engine, authorizer auth.Authorizer, backendConfig *config.BackendConfig, cfg *config.ServerConfig, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		urlPath := chi.URLParam(r, "*")
		pathInfo := ParseFilePath(urlPath)
		if pathInfo.IsInvalid {
			SendErrorResponse(w, logger, &customError{message: "invalid path"}, http.StatusBadRequest)
			return
		}

		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}

		enginePath := pathInfo.FullPath
		if pathInfo.IsDirectory && enginePath != "/" {
			enginePath = strings.TrimSuffix(enginePath, "/")
		}

		existingMd, mdErr := engine.GetMetadata(r.Context(), enginePath)
		if nil != mdErr && !errors.Is(mdErr, metadata.ErrNotFound) {
			SendErrorResponse(w, logger, mdErr, http.StatusInternalServerError)
			return
		}

		if err := authorizer.AuthorizeWithMetadata(r.Context(), userID, enginePath, existingMd, auth.WritePerm); err != nil {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		erasureRequested := r.Header.Get("X-CallFS-Erasure") == "true" || r.URL.Query().Get("erasure") == "true"
		if erasureRequested && nil == engine.GetErasureManager() {
			SendErrorResponse(w, logger, &customError{message: "erasure coding is not enabled on this server"}, http.StatusNotImplemented)
			return
		}
		erasureEnabled := erasureRequested

		if (pathInfo.IsDirectory || erasureEnabled) && nil != existingMd {
			respondExistingConflict(w, engine, existingMd, pathInfo, enginePath, logger)
			return
		}

		if pathInfo.IsDirectory {
			md := &metadata.Metadata{
				Name:        pathInfo.Name,
				Type:        "directory",
				Mode:        "0755",
				Owner:       userID,
				BackendType: backendConfig.DefaultBackend,
				ATime:       time.Now(),
				MTime:       time.Now(),
				CTime:       time.Now(),
			}
			if err := engine.CreateDirectory(r.Context(), enginePath, md); err != nil {
				SendErrorResponse(w, logger, err, http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			logger.Info("Directory created",
				zap.String("path", pathInfo.FullPath),
				zap.String("user_id", userID))
			return
		}

		if erasureEnabled {
			handleErasureUpload(w, r, engine, enginePath, pathInfo, userID, logger)
			return
		}

		handleRegularFileCreate(w, r, engine, enginePath, pathInfo, userID, backendConfig, logger)
	}
}
