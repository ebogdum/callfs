package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/erasure"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/server/middleware"
)

func parseErasureOptions(r *http.Request) *erasure.StoreOptions {
	opts := &erasure.StoreOptions{}

	if v := r.Header.Get("X-CallFS-Erasure-Data-Shards"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			opts.DataShards = n
		}
	} else if v := r.URL.Query().Get("data_shards"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			opts.DataShards = n
		}
	}

	if v := r.Header.Get("X-CallFS-Erasure-Parity-Shards"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			opts.ParityShards = n
		}
	} else if v := r.URL.Query().Get("parity_shards"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			opts.ParityShards = n
		}
	}

	if v := r.Header.Get("X-CallFS-Erasure-Instances"); v != "" {
		opts.Instances = strings.Split(v, ",")
	} else if v := r.URL.Query().Get("instances"); v != "" {
		opts.Instances = strings.Split(v, ",")
	}

	return opts
}

// CrossServerConflictResponse represents a response when a file exists on another server
type CrossServerConflictResponse struct {
	Error        string `json:"error"`
	ExistingPath string `json:"existing_path"`
	InstanceID   string `json:"instance_id"`
	BackendType  string `json:"backend_type"`
	Suggestion   string `json:"suggestion"`
	UpdateURL    string `json:"update_url,omitempty"`
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
		} else if peerEndpoint := engine.GetPeerEndpoint(*existingMd.CallFSInstanceID); peerEndpoint != "" {
			conflictResponse.UpdateURL = peerEndpoint + "/v1/files" + enginePath
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	buf, _ := json.Marshal(conflictResponse)
	_, _ = w.Write(buf)
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
	data, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		SendErrorResponse(w, logger, readErr, http.StatusInternalServerError)
		return
	}
	actualSize := int64(len(data))
	opts := parseErasureOptions(r)

	em := engine.GetErasureManager()
	if _, storeErr := em.StoreFile(r.Context(), enginePath, data, actualSize, opts); storeErr != nil {
		SendErrorResponse(w, logger, storeErr, http.StatusInternalServerError)
		return
	}

	md := &metadata.Metadata{
		Name:         pathInfo.Name,
		Type:         "file",
		Size:         actualSize,
		Mode:         "0644",
		Owner:        userID,
		BackendType:  "erasure",
		ErasureCoded: true,
		ATime:        time.Now(),
		MTime:        time.Now(),
		CTime:        time.Now(),
	}

	if err := engine.CreateErasureMetadata(r.Context(), enginePath, md); err != nil {
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	logger.Info("Erasure-coded file created",
		zap.String("path", pathInfo.FullPath),
		zap.String("user_id", userID),
		zap.Int64("size", actualSize))
}

func handleRegularFileCreate(w http.ResponseWriter, r *http.Request, engine *core.Engine, enginePath string, pathInfo PathInfo, userID string, backendConfig *config.BackendConfig, logger *zap.Logger) {
	size := r.ContentLength
	isChunked := size < 0
	if isChunked {
		size = 0
	}

	const maxUploadBytes = 10 << 30
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	var countReader *CountingReader
	if isChunked {
		countReader = NewCountingReader(r.Body)
		r.Body = io.NopCloser(countReader)
	}

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

	if err := engine.CreateFile(r.Context(), enginePath, r.Body, size, md); err != nil {
		if errors.Is(err, metadata.ErrAlreadyExists) {
			SendErrorResponse(w, logger, &customError{message: "file already exists, use PUT to update"}, http.StatusConflict)
			return
		}
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}

	if countReader != nil {
		actualSize := countReader.BytesRead()
		if actualSize != size {
			md.Size = actualSize
			md.MTime = time.Now()
			md.UpdatedAt = time.Now()
			if updateErr := engine.UpdateMetadataOnly(r.Context(), md); updateErr != nil {
				logger.Warn("Failed to correct metadata size after chunked upload",
					zap.String("path", enginePath),
					zap.Int64("actual_size", actualSize),
					zap.Error(updateErr))
			}
			size = actualSize
		}
	}

	w.WriteHeader(http.StatusCreated)
	logger.Info("File created",
		zap.String("path", pathInfo.FullPath),
		zap.String("user_id", userID),
		zap.Int64("size", size))
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

		if err := authorizer.Authorize(r.Context(), userID, enginePath, auth.WritePerm); err != nil {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		existingMd, err := engine.GetMetadata(r.Context(), enginePath)
		if err == nil {
			currentInstanceID := engine.GetCurrentInstanceID()
			if existingMd.CallFSInstanceID != nil && *existingMd.CallFSInstanceID != currentInstanceID {
				handleCrossServerConflict(w, engine, existingMd, pathInfo, enginePath)
				return
			}
			handleLocalConflict(w, logger, existingMd, pathInfo)
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

		erasureRequested := r.Header.Get("X-CallFS-Erasure") == "true" || r.URL.Query().Get("erasure") == "true"
		if erasureRequested && engine.GetErasureManager() != nil {
			handleErasureUpload(w, r, engine, enginePath, pathInfo, userID, logger)
			return
		}

		handleRegularFileCreate(w, r, engine, enginePath, pathInfo, userID, backendConfig, logger)
	}
}
