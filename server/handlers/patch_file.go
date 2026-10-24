package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/core"
	"github.com/ebogdum/callfs/v2/internal/pathutil"
	"github.com/ebogdum/callfs/v2/server/middleware"
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
	Mode          string `json:"mode"`
	Owner         string `json:"owner"`
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
func V1PatchFileEnhanced(engine *core.Engine, authorizer auth.Authorizer, admins *auth.AdminSet, backendConfig *config.BackendConfig, cfg *config.ServerConfig, logger *zap.Logger) http.HandlerFunc {
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

		if "" != req.Mode || "" != req.Owner {
			if "" != req.Name || "" != req.Destination {
				sendBadRequest(w, logger, "mode or owner cannot be combined with name or destination")
				return
			}
			patchFileAttributes(w, r, engine, admins, userID, srcPath, req, logger)
			return
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
		if isMove && !isAdminAppUser(userID) && ("" != req.Backend || "" != req.Instance) {
			SendErrorResponse(w, logger, &customError{message: "moving to a different backend or instance requires admin"}, http.StatusForbidden)
			return
		}
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
			err = engine.Move(r.Context(), srcPath, dstPath, core.MoveOptions{
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
	var proxyFailed *core.ErrProxyFailed
	switch {
	case errors.As(err, &invalid):
		sendBadRequest(w, logger, invalid.Error())
	case errors.As(err, &unsupported):
		sendBadRequest(w, logger, unsupported.Error())
	case errors.As(err, &proxyFailed):
		SendErrorResponse(w, logger, proxyFailed, http.StatusBadGateway)
	default:
		// SendErrorResponse maps ErrNotFound→404, ErrAlreadyExists→409,
		// auth errors→401/403; everything else falls back to 500.
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
	}
}

const maxOwnerLength = 255

type fileAttributesResponse struct {
	Path  string `json:"path"`
	Mode  string `json:"mode"`
	Owner string `json:"owner"`
}

func patchFileAttributes(w http.ResponseWriter, r *http.Request, engine *core.Engine, admins *auth.AdminSet, userID, path string, req renameMoveRequest, logger *zap.Logger) {
	isAdmin := isAdminAppUser(userID) || admins.IsAdmin(userID)
	if "" != req.Owner && !isAdmin {
		sendCodedError(w, logger, http.StatusForbidden, "ADMIN_REQUIRED", "Only admins can do this.")
		return
	}
	if "" != req.Owner && (maxOwnerLength < len(req.Owner) || req.Owner != strings.TrimSpace(req.Owner)) {
		sendBadRequest(w, logger, "owner must be a user ID of at most 255 characters without surrounding spaces")
		return
	}

	mode := ""
	if "" != req.Mode {
		normalized, ok := normalizeFileMode(req.Mode)
		if !ok {
			sendBadRequest(w, logger, "mode must be an octal permission such as 0640")
			return
		}
		mode = normalized
	}

	md, err := engine.GetMetadata(r.Context(), path)
	if nil != err {
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	if !isAdmin && md.Owner != userID {
		SendErrorResponse(w, logger, auth.ErrPermissionDenied, http.StatusForbidden)
		return
	}

	updated := *md
	if "" != mode {
		updated.Mode = mode
	}
	if "" != req.Owner {
		updated.Owner = req.Owner
	}
	if err := engine.UpdateMetadataOnly(r.Context(), &updated); nil != err {
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}

	SendJSONResponse(w, fileAttributesResponse{Path: path, Mode: updated.Mode, Owner: updated.Owner})
	logger.Info("File mode or owner updated",
		zap.String("path", path),
		zap.String("user_id", userID),
		zap.String("mode", updated.Mode),
		zap.String("owner", updated.Owner))
}

func normalizeFileMode(raw string) (string, bool) {
	digits := raw
	if 4 == len(raw) && '0' == raw[0] {
		digits = raw[1:]
	}
	if 3 != len(digits) {
		return "", false
	}
	for i := 0; i < len(digits); i++ {
		if '0' > digits[i] || '7' < digits[i] {
			return "", false
		}
	}
	return "0" + digits, true
}

func sendCodedError(w http.ResponseWriter, logger *zap.Logger, status int, code, message string) {
	buf, err := json.Marshal(ErrorResponse{Code: code, Message: message})
	if err != nil {
		logger.Error("Failed to marshal error response", zap.Error(err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(buf); err != nil {
		logger.Error("Failed to write error response", zap.Error(err))
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
