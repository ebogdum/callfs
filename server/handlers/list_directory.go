package handlers

import (
	"cmp"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/core"
	"github.com/ebogdum/callfs/v2/core/log"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/server/middleware"
)

// DirectoryListingResponse represents the response for directory listing operations
type DirectoryListingResponse struct {
	Path      string     `json:"path"`
	Type      string     `json:"type"` // "directory"
	Recursive bool       `json:"recursive"`
	MaxDepth  int        `json:"max_depth,omitempty"`
	Count     int        `json:"count"`
	Truncated bool       `json:"truncated,omitempty"`
	Items     []FileInfo `json:"items"`
}

type DirectoryPageItem struct {
	FileInfo
	ETag         string `json:"etag"`
	Storage      string `json:"storage"`
	InstanceID   string `json:"instance_id"`
	ErasureCoded bool   `json:"erasure_coded"`
	Locked       bool   `json:"locked"`
}

type DirectoryPageResponse struct {
	Path       string              `json:"path"`
	Type       string              `json:"type"`
	Count      int                 `json:"count"`
	Total      int                 `json:"total"`
	Items      []DirectoryPageItem `json:"items"`
	NextCursor *string             `json:"next_cursor"`
	Partial    bool                `json:"partial"`
}

type directoryPageQuery struct {
	limit         int
	offset        int
	sortKey       string
	descending    bool
	entryType     string
	nameFilter    string
	includeHidden bool
}

const maxDirectoryListItems = 10000

const maxDirectoryPageLimit = 1000

var directorySortKeys = []string{"name", "size", "mtime", "type"}

// ListDirectory handles GET /api/directories/{path} requests
// @Summary List directory contents
// @Description Lists directory contents with optional recursive traversal
// @Tags directories
// @Security BearerAuth
// @Param path path string true "Directory path"
// @Param recursive query bool false "Recursively list subdirectories"
// @Param max_depth query int false "Maximum recursion depth (default: 100, max: 1000)"
// @Success 200 {object} DirectoryListingResponse "Directory listing"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Forbidden"
// @Failure 404 {object} ErrorResponse "Not Found"
// @Failure 400 {object} ErrorResponse "Bad Request"
// @Router /v1/directories/{path} [get]
func parseMaxDepth(r *http.Request) int {
	maxDepthStr := r.URL.Query().Get("max_depth")
	if maxDepthStr == "" {
		return 100
	}
	parsed, err := strconv.Atoi(maxDepthStr)
	if err != nil || parsed < 0 {
		return 100
	}
	if parsed > 1000 {
		return 1000
	}
	return parsed
}

func fileInfoOf(child *metadata.Metadata) FileInfo {
	return FileInfo{
		Name:  child.Name,
		Path:  child.Path,
		Type:  child.Type,
		Size:  child.Size,
		Mode:  child.Mode,
		Owner: child.Owner,
		MTime: child.MTime.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func metadataToFileInfos(children []*metadata.Metadata) []FileInfo {
	fileInfos := make([]FileInfo, 0, len(children))
	for _, child := range children {
		fileInfos = append(fileInfos, fileInfoOf(child))
	}
	return fileInfos
}

func parseDirectoryPageQuery(values url.Values) (directoryPageQuery, error) {
	query := directoryPageQuery{
		sortKey:       "name",
		includeHidden: true,
	}

	limit, err := strconv.Atoi(values.Get("limit"))
	if nil != err || 1 > limit {
		return query, fmt.Errorf("invalid limit")
	}
	query.limit = min(limit, maxDirectoryPageLimit)

	if cursor := values.Get("cursor"); "" != cursor {
		offset, cursorErr := decodeDirectoryCursor(cursor)
		if nil != cursorErr {
			return query, cursorErr
		}
		query.offset = offset
	}

	if sortKey := values.Get("sort"); "" != sortKey {
		if !slices.Contains(directorySortKeys, sortKey) {
			return query, fmt.Errorf("invalid sort")
		}
		query.sortKey = sortKey
	}

	switch values.Get("order") {
	case "", "asc":
	case "desc":
		query.descending = true
	default:
		return query, fmt.Errorf("invalid order")
	}

	switch entryType := values.Get("type"); entryType {
	case "", "file", "directory":
		query.entryType = entryType
	default:
		return query, fmt.Errorf("invalid type")
	}

	query.nameFilter = strings.ToLower(values.Get("q"))

	switch values.Get("include_hidden") {
	case "", "true":
	case "false":
		query.includeHidden = false
	default:
		return query, fmt.Errorf("invalid include_hidden")
	}

	return query, nil
}

func decodeDirectoryCursor(cursor string) (int, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if nil != err {
		return 0, fmt.Errorf("invalid cursor")
	}
	offset, err := strconv.Atoi(string(raw))
	if nil != err || 0 > offset {
		return 0, fmt.Errorf("invalid cursor")
	}
	return offset, nil
}

func encodeDirectoryCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

func filterDirectoryChildren(children []*metadata.Metadata, query directoryPageQuery) []*metadata.Metadata {
	matched := make([]*metadata.Metadata, 0, len(children))
	for _, child := range children {
		if "" != query.entryType && child.Type != query.entryType {
			continue
		}
		if !query.includeHidden && strings.HasPrefix(child.Name, ".") {
			continue
		}
		if "" != query.nameFilter && !strings.Contains(strings.ToLower(child.Name), query.nameFilter) {
			continue
		}
		matched = append(matched, child)
	}
	return matched
}

func compareDirectoryChildren(a, b *metadata.Metadata, query directoryPageQuery) int {
	if "name" == query.sortKey && a.Type != b.Type {
		if "directory" == a.Type {
			return -1
		}
		return 1
	}

	result := 0
	switch query.sortKey {
	case "size":
		result = cmp.Compare(a.Size, b.Size)
	case "mtime":
		result = a.MTime.Compare(b.MTime)
	case "type":
		result = strings.Compare(a.Type, b.Type)
	}
	if 0 == result {
		result = strings.Compare(a.Name, b.Name)
	}
	if query.descending {
		return -result
	}
	return result
}

func serveDirectoryPage(ctx context.Context, w http.ResponseWriter, values url.Values, engine *core.Engine, path string, recursive bool, maxDepth int, logger *zap.Logger) {
	query, err := parseDirectoryPageQuery(values)
	if nil != err {
		SendErrorResponse(w, logger, err, http.StatusBadRequest)
		return
	}

	var children []*metadata.Metadata
	if recursive {
		children, err = engine.ListDirectoryRecursiveLimited(ctx, path, maxDepth, maxDirectoryListItems+1)
	} else {
		children, err = engine.ListDirectory(ctx, path)
	}
	if nil != err {
		SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	partial := maxDirectoryListItems < len(children)
	if partial {
		children = children[:maxDirectoryListItems]
	}

	matched := filterDirectoryChildren(children, query)
	slices.SortStableFunc(matched, func(a, b *metadata.Metadata) int {
		return compareDirectoryChildren(a, b, query)
	})

	total := len(matched)
	start := min(query.offset, total)
	end := min(start+query.limit, total)
	var nextCursor *string
	if end < total {
		encoded := encodeDirectoryCursor(end)
		nextCursor = &encoded
	}

	currentInstanceID := engine.GetCurrentInstanceID()
	page := matched[start:end]
	items := make([]DirectoryPageItem, 0, len(page))
	for _, child := range page {
		items = append(items, DirectoryPageItem{
			FileInfo:     fileInfoOf(child),
			ETag:         etagField(child.ETag),
			Storage:      child.BackendType,
			InstanceID:   owningInstanceID(child, currentInstanceID),
			ErasureCoded: child.ErasureCoded,
		})
	}

	SendJSONResponse(w, DirectoryPageResponse{
		Path:       path,
		Type:       "directory",
		Count:      len(items),
		Total:      total,
		Items:      items,
		NextCursor: nextCursor,
		Partial:    partial,
	})
}

func V1ListDirectory(engine *core.Engine, authorizer auth.Authorizer, logger *zap.Logger) http.HandlerFunc {
	return V1ListDirectoryWithConfig(engine, authorizer, &config.ServerConfig{MetadataOpTimeout: 30 * time.Second}, logger)
}

func V1ListDirectoryWithConfig(engine *core.Engine, authorizer auth.Authorizer, cfg *config.ServerConfig, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		metadataCtx, cancel := context.WithTimeout(r.Context(), cfg.MetadataOpTimeout)
		defer cancel()

		pathParam := chi.URLParam(r, "*")
		if pathParam == "" {
			pathParam = "/"
		} else {
			pathParam = "/" + pathParam
		}

		pathInfo := ParseFilePath(pathParam)
		if pathInfo.IsInvalid {
			SendErrorResponse(w, logger, fmt.Errorf("invalid path"), http.StatusBadRequest)
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

		md, mdErr := engine.GetMetadata(metadataCtx, enginePath)
		if nil != mdErr && !errors.Is(mdErr, metadata.ErrNotFound) {
			SendErrorResponse(w, logger, mdErr, http.StatusInternalServerError)
			return
		}

		if err := authorizer.AuthorizeWithMetadata(metadataCtx, userID, enginePath, md, auth.ReadPerm); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, auth.ErrPermissionDenied) {
				status = http.StatusForbidden
			}
			SendErrorResponse(w, logger, err, status)
			return
		}

		if nil == md {
			SendErrorResponse(w, logger, mdErr, http.StatusNotFound)
			return
		}

		if md.Type != "directory" {
			SendErrorResponse(w, logger, fmt.Errorf("path is not a directory"), http.StatusBadRequest)
			return
		}

		recursive := r.URL.Query().Get("recursive") == "true"
		maxDepth := parseMaxDepth(r)

		if values := r.URL.Query(); values.Has("limit") {
			serveDirectoryPage(metadataCtx, w, values, engine, enginePath, recursive, maxDepth, logger)
			return
		}

		var children []*metadata.Metadata
		var err error
		if recursive {
			children, err = engine.ListDirectoryRecursiveLimited(metadataCtx, enginePath, maxDepth, maxDirectoryListItems+1)
		} else {
			children, err = engine.ListDirectory(metadataCtx, enginePath)
		}
		if err != nil {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}

		truncated := maxDirectoryListItems < len(children)
		if truncated {
			children = children[:maxDirectoryListItems]
		}

		fileInfos := metadataToFileInfos(children)
		response := DirectoryListingResponse{
			Path:      enginePath,
			Type:      "directory",
			Recursive: recursive,
			Count:     len(fileInfos),
			Truncated: truncated,
			Items:     fileInfos,
		}
		if recursive {
			response.MaxDepth = maxDepth
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-CallFS-Type", "directory")
		w.Header().Set("X-CallFS-Count", fmt.Sprintf("%d", len(fileInfos)))
		w.Header().Set("X-CallFS-Recursive", fmt.Sprintf("%t", recursive))

		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Error("Failed to write directory listing response", zap.Error(err))
		}

		if ce := logger.Check(zap.InfoLevel, "Directory listed via API"); nil != ce {
			logFields := log.LogFields{
				Path:      pathInfo.FullPath,
				UserID:    userID,
				Operation: "list_directory",
			}.Sanitize()

			ce.Write(
				zap.String("path", logFields.Path),
				zap.String("user_id", logFields.UserID),
				zap.Bool("recursive", recursive),
				zap.Int("max_depth", maxDepth),
				zap.Int("items_count", len(children)))
		}
	}
}
