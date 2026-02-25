package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/server/middleware"
)

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
func V1PostFileEnhanced(engine *core.Engine, authorizer auth.Authorizer, backendConfig *config.BackendConfig, cfg *config.ServerConfig, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract and parse path from URL
		urlPath := chi.URLParam(r, "*")
		pathInfo := ParseFilePath(urlPath)
		if pathInfo.IsInvalid {
			SendErrorResponse(w, logger, &customError{message: "invalid path"}, http.StatusBadRequest)
			return
		}

		// Get user ID from context
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}

		// Normalize path for engine calls
		enginePath := pathInfo.FullPath
		if pathInfo.IsDirectory && enginePath != "/" {
			enginePath = strings.TrimSuffix(enginePath, "/")
		}

		// Authorize write access FIRST
		if err := authorizer.Authorize(r.Context(), userID, enginePath, auth.WritePerm); err != nil {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		// Check if file/directory already exists (with cross-server detection)
		existingMd, err := engine.GetMetadata(r.Context(), enginePath)
		fileExists := (err == nil)

		if fileExists {
			// Check if the existing resource is on this instance
			currentInstanceID := engine.GetCurrentInstanceID()

			if existingMd.CallFSInstanceID != nil && *existingMd.CallFSInstanceID != currentInstanceID {
				// Resource exists on another server - provide conflict response
				conflictResponse := CrossServerConflictResponse{
					Error:        "Resource exists on another server",
					ExistingPath: existingMd.Path,
					InstanceID:   *existingMd.CallFSInstanceID,
					BackendType:  existingMd.BackendType,
				}

				if pathInfo.IsDirectory {
					if existingMd.Type != "directory" {
						conflictResponse.Suggestion = "Path exists as file on another server, cannot create directory"
					} else {
						conflictResponse.Suggestion = "Directory already exists on another server. Use GET to access it."
					}
				} else {
					if existingMd.Type != "file" {
						conflictResponse.Suggestion = "Path exists as directory on another server, cannot create file"
					} else {
						conflictResponse.Suggestion = "File already exists on another server. Use PUT to update it."
						// Provide the update URL for cross-server PUT
						if peerEndpoint := engine.GetPeerEndpoint(*existingMd.CallFSInstanceID); peerEndpoint != "" {
							conflictResponse.UpdateURL = peerEndpoint + "/v1/files" + enginePath
						}
					}
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				SendJSONResponse(w, conflictResponse)
				return
			}

			// Resource exists on this instance - handle normally
			if pathInfo.IsDirectory {
				if existingMd.Type != "directory" {
					SendErrorResponse(w, logger, &customError{message: "path exists as file, cannot create directory"}, http.StatusConflict)
					return
				}
				// Directory already exists - return OK
				w.WriteHeader(http.StatusOK)
				return
			} else {
				if existingMd.Type != "file" {
					SendErrorResponse(w, logger, &customError{message: "path exists as directory, cannot create file"}, http.StatusConflict)
					return
				}
				// File already exists on this instance - return conflict for POST
				SendErrorResponse(w, logger, &customError{message: "file already exists, use PUT to update"}, http.StatusConflict)
				return
			}
		}

		if pathInfo.IsDirectory {
			// Create new directory
			md := &metadata.Metadata{
				Name:        pathInfo.Name,
				Type:        "directory",
				Mode:        "0755",
				UID:         1000,
				GID:         1000,
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

		} else {
			// File creation (fileExists is false at this point)
			size := r.ContentLength
			if size < 0 {
				// Chunked uploads don't provide a trusted content length here.
				size = 0
			}

			md := &metadata.Metadata{
				Name:        pathInfo.Name,
				Type:        "file",
				Mode:        "0644",
				UID:         1000,
				GID:         1000,
				BackendType: backendConfig.DefaultBackend,
				ATime:       time.Now(),
				MTime:       time.Now(),
				CTime:       time.Now(),
			}

			// Create new file
			if err := engine.CreateFile(r.Context(), enginePath, r.Body, size, md); err != nil {
				SendErrorResponse(w, logger, err, http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusCreated)
			logger.Info("File created",
				zap.String("path", pathInfo.FullPath),
				zap.String("user_id", userID),
				zap.Int64("size", size))
		}
	}
}
