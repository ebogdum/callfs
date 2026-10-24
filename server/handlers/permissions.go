package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/server/middleware"
)

const permissionUserMaxBytes = 256

var permissionTypesByName = map[string]auth.PermissionType{
	"read":   auth.ReadPerm,
	"write":  auth.WritePerm,
	"delete": auth.DeletePerm,
}

type decisionExplainer interface {
	Explain(ctx context.Context, userID string, path string, perm auth.PermissionType) (auth.Decision, error)
}

type PermissionCheckResponse struct {
	Path    string `json:"path"`
	Perm    string `json:"perm"`
	User    string `json:"user"`
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
	Owner   string `json:"owner"`
	Type    string `json:"type"`
}

func V1PermissionCheck(authorizer auth.Authorizer, engine *core.Engine, admins *auth.AdminSet) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := zap.L()

		callerID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}

		query := r.URL.Query()
		permName := query.Get("perm")
		perm, ok := permissionTypesByName[permName]
		if !ok {
			SendErrorResponse(w, logger, &customError{message: "perm must be read, write or delete"}, http.StatusBadRequest)
			return
		}

		enginePath, ok := normalizeQueryPath(query.Get("path"))
		if !ok {
			SendErrorResponse(w, logger, &customError{message: "invalid path"}, http.StatusBadRequest)
			return
		}

		explainer, ok := authorizer.(decisionExplainer)
		if !ok {
			SendErrorResponse(w, logger, &customError{message: "authorizer cannot explain decisions"}, http.StatusInternalServerError)
			return
		}

		targetID := callerID
		if query.Has("user") {
			targetID = query.Get("user")
			if "" == targetID || permissionUserMaxBytes < len(targetID) {
				SendErrorResponse(w, logger, &customError{message: "user must be 1 to 256 characters"}, http.StatusBadRequest)
				return
			}
			if targetID != callerID && !admins.IsAdmin(callerID) {
				writeAdminRequiredResponse(w, logger)
				return
			}
		}

		decision, err := explainer.Explain(r.Context(), targetID, enginePath, perm)
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}

		SendJSONResponse(w, PermissionCheckResponse{
			Path:    enginePath,
			Perm:    permName,
			User:    targetID,
			Allowed: decision.Allowed,
			Reason:  decision.Reason,
			Owner:   decision.Owner,
			Type:    decision.Type,
		})
	}
}

func writeAdminRequiredResponse(w http.ResponseWriter, logger *zap.Logger) {
	body, err := json.Marshal(ErrorResponse{Code: "ADMIN_REQUIRED", Message: "Admin access required"})
	if nil != err {
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	if _, writeErr := w.Write(append(body, '\n')); nil != writeErr {
		logger.Warn("failed to write admin required response", zap.Error(writeErr))
	}
}
