package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/internal/pathutil"
	"github.com/ebogdum/callfs/server/middleware"
)

// renameMoveRequest is the JSON body of a PATCH /files/{path} request.
//
// Exactly one of Name (rename in place) or Destination (move) must be set.
type renameMoveRequest struct {
	Name          string `json:"name"`
	Destination   string `json:"destination"`
	Backend       string `json:"backend"`
	Instance      string `json:"instance"`
	Overwrite     bool   `json:"overwrite"`
	CreateParents bool   `json:"create_parents"`
}

// V1PatchFileEnhanced handles PATCH /files/{path} for first-class rename and move.
//
// @Summary Rename or move a file or directory
// @Description Renames a resource in place ({"name":"..."}) or moves it, optionally to a different folder, backend, or instance ({"destination":"...","backend":"...","instance":"..."}).
// @Tags files
// @Security BearerAuth
// @Param path path string true "Source file or directory path"
// @Param body body renameMoveRequest true "Rename or move request"
// @Success 200 {object} map[string]string "Renamed/moved; returns the new path"
// @Failure 400 {object} ErrorResponse "Bad Request"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Forbidden"
// @Failure 404 {object} ErrorResponse "Not Found"
// @Failure 409 {object} ErrorResponse "Conflict"
// @Failure 502 {object} ErrorResponse "Bad Gateway (cross-server proxy error)"
// @Router /v1/files/{path} [patch]
func V1PatchFileEnhanced(engine *core.Engine, authorizer auth.Authorizer, backendConfig *config.BackendConfig, cfg *config.ServerConfig, logger *zap.Logger) http.HandlerFunc {
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

		srcPath := pathInfo.FullPath
		if pathInfo.IsDirectory && srcPath != "/" {
			srcPath = strings.TrimSuffix(srcPath, "/")
		}

		var req renameMoveRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			sendBadRequest(w, logger, "invalid JSON body")
			return
		}

		// Query-param override for overwrite (body still wins if it set true).
		if r.URL.Query().Get("overwrite") == "true" {
			req.Overwrite = true
		}

		isRename := req.Name != ""
		isMove := req.Destination != ""
		if isRename == isMove {
			sendBadRequest(w, logger, "exactly one of \"name\" or \"destination\" must be provided")
			return
		}

		// Determine the destination path for authorization.
		dstPath, err := resolveDestination(srcPath, req)
		if err != nil {
			sendBadRequest(w, logger, err.Error())
			return
		}

		// A move is a delete at the source plus a create at the destination.
		if err := authorizer.Authorize(r.Context(), userID, srcPath, auth.DeletePerm); err != nil {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}
		if err := authorizer.Authorize(r.Context(), userID, dstPath, auth.WritePerm); err != nil {
			SendErrorResponse(w, logger, err, http.StatusForbidden)
			return
		}

		if isRename {
			err = engine.Rename(r.Context(), srcPath, req.Name)
		} else {
			err = engine.Move(r.Context(), srcPath, req.Destination, core.MoveOptions{
				Backend:       req.Backend,
				Instance:      req.Instance,
				Overwrite:     req.Overwrite,
				CreateParents: req.CreateParents,
			})
		}
		if err != nil {
			writeMoveError(w, logger, err)
			return
		}

		SendJSONResponse(w, map[string]string{"path": dstPath})
		logger.Info("Resource renamed/moved",
			zap.String("src", srcPath),
			zap.String("dst", dstPath),
			zap.String("user_id", userID))
	}
}

// resolveDestination computes the absolute destination path from the request.
//
// SECURITY: both branches must return a canonical path. These paths arrive in the
// request body, so they never pass through ParseFilePath. Returning a raw path
// here would reintroduce the metadata/backend split that canonicalization in
// ParseFilePath exists to prevent: the metadata stores key on this exact string
// while the backends normalize it before touching disk, so "/dir/../victim.txt"
// would authorize against a non-existent parent while overwriting victim.txt.
func resolveDestination(srcPath string, req renameMoveRequest) (string, error) {
	if req.Name != "" {
		if strings.Contains(req.Name, "/") || req.Name == "." || req.Name == ".." {
			return "", errors.New("name must be a single path segment")
		}
		if err := pathutil.ValidatePath(req.Name); err != nil {
			return "", errors.New("name contains invalid characters")
		}
		parent := srcPath[:strings.LastIndex(srcPath, "/")]
		if parent == "" {
			return canonicalDestination("/" + req.Name)
		}
		return canonicalDestination(parent + "/" + req.Name)
	}

	if err := pathutil.ValidatePath(req.Destination); err != nil {
		return "", errors.New("destination is not a valid path")
	}
	dst := req.Destination
	if !strings.HasPrefix(dst, "/") {
		dst = "/" + dst
	}
	return canonicalDestination(dst)
}

// canonicalDestination normalizes an absolute destination path, rejecting any
// path that escapes the root.
func canonicalDestination(dst string) (string, error) {
	canonical, err := pathutil.Clean(dst)
	if err != nil {
		return "", errors.New("destination escapes the filesystem root")
	}
	if canonical != "/" {
		canonical = strings.TrimSuffix(canonical, "/")
	}
	return canonical, nil
}

// writeMoveError maps engine errors to HTTP status codes.
func writeMoveError(w http.ResponseWriter, logger *zap.Logger, err error) {
	var invalid *core.ErrInvalidRename
	var unsupported *core.ErrUnsupportedMove
	switch {
	case errors.As(err, &invalid):
		sendBadRequest(w, logger, invalid.Error())
	case errors.As(err, &unsupported):
		sendBadRequest(w, logger, unsupported.Error())
	default:
		// SendErrorResponse maps ErrNotFound→404, ErrAlreadyExists→409,
		// auth errors→401/403; everything else falls back to 500.
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
	}
}

func sendBadRequest(w http.ResponseWriter, logger *zap.Logger, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	resp := ErrorResponse{Code: "BAD_REQUEST", Message: message}
	buf, err := json.Marshal(resp)
	if err != nil {
		logger.Error("Failed to marshal bad-request response", zap.Error(err))
		return
	}
	if _, err := w.Write(buf); err != nil {
		logger.Error("Failed to write bad-request response", zap.Error(err))
	}
}
