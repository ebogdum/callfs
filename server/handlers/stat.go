package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/locks"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/server/middleware"
)

const pathLockKeyPrefix = "path:"

type StatResponse struct {
	Path          string  `json:"path"`
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	Size          int64   `json:"size"`
	Mode          string  `json:"mode"`
	Owner         string  `json:"owner"`
	ETag          string  `json:"etag"`
	Storage       string  `json:"storage"`
	InstanceID    string  `json:"instance_id"`
	IsLocal       bool    `json:"is_local"`
	ErasureCoded  bool    `json:"erasure_coded"`
	SymlinkTarget *string `json:"symlink_target"`
	MTime         string  `json:"mtime"`
	ATime         string  `json:"atime"`
	CTime         string  `json:"ctime"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
	Locked        bool    `json:"locked"`
	ChildrenCount *int    `json:"children_count"`
}

func V1Stat(engine *core.Engine, authorizer auth.Authorizer, lockInspector locks.Inspector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := zap.L()

		pathInfo := ParseFilePath(chi.URLParam(r, "*"))
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
		if pathInfo.IsDirectory && "/" != enginePath {
			enginePath = strings.TrimSuffix(enginePath, "/")
		}

		md, mdErr := engine.GetMetadata(r.Context(), enginePath)
		if nil != mdErr && !errors.Is(mdErr, metadata.ErrNotFound) {
			SendErrorResponse(w, logger, mdErr, http.StatusInternalServerError)
			return
		}

		if err := authorizer.AuthorizeWithMetadata(r.Context(), userID, enginePath, md, auth.ReadPerm); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, auth.ErrPermissionDenied) {
				status = http.StatusForbidden
			}
			SendErrorResponse(w, logger, err, status)
			return
		}

		if nil == md {
			SendErrorResponse(w, logger, metadata.ErrNotFound, http.StatusNotFound)
			return
		}

		lockedPaths, err := heldPathLocks(r.Context(), lockInspector)
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}

		childrenCount, err := directoryChildrenCount(r.Context(), engine, md)
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}

		currentInstanceID := engine.GetCurrentInstanceID()
		instanceID := owningInstanceID(md, currentInstanceID)
		SendJSONResponse(w, StatResponse{
			Path:          enginePath,
			Name:          md.Name,
			Type:          md.Type,
			Size:          md.Size,
			Mode:          md.Mode,
			Owner:         md.Owner,
			ETag:          etagField(md.ETag),
			Storage:       md.BackendType,
			InstanceID:    instanceID,
			IsLocal:       instanceID == currentInstanceID,
			ErasureCoded:  md.ErasureCoded,
			SymlinkTarget: md.SymlinkTarget,
			MTime:         rfc3339UTC(md.MTime),
			ATime:         rfc3339UTC(md.ATime),
			CTime:         rfc3339UTC(md.CTime),
			CreatedAt:     rfc3339UTC(md.CreatedAt),
			UpdatedAt:     rfc3339UTC(md.UpdatedAt),
			Locked:        lockedPaths[enginePath],
			ChildrenCount: childrenCount,
		})
	}
}

func etagField(etag string) string {
	if "" == etag {
		return ""
	}
	return quotedETag(etag)
}

func rfc3339UTC(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func owningInstanceID(md *metadata.Metadata, currentInstanceID string) string {
	if nil != md.CallFSInstanceID {
		return *md.CallFSInstanceID
	}
	return currentInstanceID
}

func heldPathLocks(ctx context.Context, inspector locks.Inspector) (map[string]bool, error) {
	held := make(map[string]bool)
	if nil == inspector {
		return held, nil
	}
	heldLocks, err := inspector.Snapshot(ctx)
	if nil != err {
		return nil, err
	}
	for _, lock := range heldLocks {
		if strings.HasPrefix(lock.Key, pathLockKeyPrefix) {
			held[strings.TrimPrefix(lock.Key, pathLockKeyPrefix)] = true
		}
	}
	return held, nil
}

func directoryChildrenCount(ctx context.Context, engine *core.Engine, md *metadata.Metadata) (*int, error) {
	if "directory" != md.Type {
		return nil, nil
	}
	count, err := engine.CountDirectoryChildren(ctx, md.Path)
	if nil != err {
		return nil, err
	}
	return &count, nil
}
