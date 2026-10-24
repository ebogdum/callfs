package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/server/middleware"
)

const (
	searchScanBudget     = 50000
	searchDefaultLimit   = 100
	searchMaxLimit       = 1000
	searchMaxQueryLength = 256
	searchMaxCursorBytes = 4096
)

type SearchItem struct {
	Path         string `json:"path"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Size         int64  `json:"size"`
	Mode         string `json:"mode"`
	Owner        string `json:"owner"`
	ETag         string `json:"etag"`
	Storage      string `json:"storage"`
	InstanceID   string `json:"instance_id"`
	IsLocal      bool   `json:"is_local"`
	ErasureCoded bool   `json:"erasure_coded"`
	MTime        string `json:"mtime"`
	UpdatedAt    string `json:"updated_at"`
}

type SearchResponse struct {
	Items         []SearchItem              `json:"items"`
	NextCursor    string                    `json:"next_cursor"`
	TookMs        int64                     `json:"took_ms"`
	ScanTruncated bool                      `json:"scan_truncated"`
	Scanned       int                       `json:"scanned"`
	Facets        map[string]map[string]int `json:"facets,omitempty"`
}

type searchFilter struct {
	query          string
	entryType      string
	minSize        *int64
	maxSize        *int64
	modifiedAfter  *time.Time
	modifiedBefore *time.Time
	owner          string
	storage        string
	instance       string
	erasure        *bool
}

type searchRequest struct {
	scope  string
	filter searchFilter
	limit  int
	cursor string
	facets bool
}

func V1Search(engine *core.Engine, authorizer auth.Authorizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := zap.L()
		started := time.Now()

		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}

		req, err := parseSearchRequest(r.URL.Query())
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusBadRequest)
			return
		}

		scopeMD, err := engine.GetMetadata(r.Context(), req.scope)
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
		if "directory" != scopeMD.Type {
			SendErrorResponse(w, logger, &customError{message: "path is not a directory"}, http.StatusBadRequest)
			return
		}
		if err := authorizer.AuthorizeWithMetadata(r.Context(), userID, req.scope, scopeMD, auth.ReadPerm); nil != err {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}

		currentInstanceID := engine.GetCurrentInstanceID()
		candidates, scanned, err := engine.SearchDirectoryRecursiveLimited(r.Context(), req.scope, -1, searchScanBudget, func(md *metadata.Metadata) bool {
			return req.filter.matches(md, currentInstanceID)
		})
		if nil != err {
			SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}

		matched := make([]*metadata.Metadata, 0, len(candidates))
		for _, md := range candidates {
			if err := authorizer.AuthorizeWithMetadata(r.Context(), userID, md.Path, md, auth.ReadPerm); nil != err {
				if errors.Is(err, auth.ErrPermissionDenied) {
					continue
				}
				SendErrorResponse(w, logger, err, http.StatusInternalServerError)
				return
			}
			matched = append(matched, md)
		}

		items := make([]SearchItem, 0, req.limit)
		nextCursor := ""
		for _, md := range matched {
			if "" != req.cursor && md.Path <= req.cursor {
				continue
			}
			if len(items) == req.limit {
				nextCursor = items[len(items)-1].Path
				break
			}
			items = append(items, searchItemOf(md, currentInstanceID))
		}

		response := SearchResponse{
			Items:         items,
			NextCursor:    nextCursor,
			TookMs:        time.Since(started).Milliseconds(),
			ScanTruncated: searchScanBudget == scanned,
			Scanned:       scanned,
		}
		if req.facets {
			response.Facets = buildSearchFacets(matched, currentInstanceID)
		}
		SendJSONResponse(w, response)
	}
}

func parseSearchRequest(values url.Values) (searchRequest, error) {
	var req searchRequest

	scope, ok := normalizeQueryPath(values.Get("path"))
	if !ok {
		return req, &customError{message: "invalid path"}
	}
	req.scope = scope

	if values.Has("locked") {
		return req, &customError{message: "locked filter is not available"}
	}

	entryType := values.Get("type")
	if "" != entryType && "file" != entryType && "directory" != entryType {
		return req, &customError{message: "type must be file or directory"}
	}
	req.filter.entryType = entryType

	query, err := boundedQueryValue(values, "q", searchMaxQueryLength)
	if nil != err {
		return req, err
	}
	if strings.ContainsAny(query, "*?[") {
		if _, matchErr := path.Match(query, ""); nil != matchErr {
			return req, &customError{message: "q has an invalid glob pattern"}
		}
	}
	req.filter.query = query

	if req.filter.minSize, err = parseOptionalInt64(values, "min_size"); nil != err {
		return req, err
	}
	if req.filter.maxSize, err = parseOptionalInt64(values, "max_size"); nil != err {
		return req, err
	}
	if nil != req.filter.minSize && nil != req.filter.maxSize && *req.filter.minSize > *req.filter.maxSize {
		return req, &customError{message: "min_size must not be greater than max_size"}
	}
	if req.filter.modifiedAfter, err = parseOptionalTime(values, "modified_after"); nil != err {
		return req, err
	}
	if req.filter.modifiedBefore, err = parseOptionalTime(values, "modified_before"); nil != err {
		return req, err
	}
	if req.filter.owner, err = boundedQueryValue(values, "owner", searchMaxQueryLength); nil != err {
		return req, err
	}
	if req.filter.storage, err = boundedQueryValue(values, "storage", searchMaxQueryLength); nil != err {
		return req, err
	}
	if req.filter.instance, err = boundedQueryValue(values, "instance", searchMaxQueryLength); nil != err {
		return req, err
	}
	if req.filter.erasure, err = parseOptionalBool(values, "erasure"); nil != err {
		return req, err
	}

	req.limit = searchDefaultLimit
	if raw := values.Get("limit"); "" != raw {
		limit, parseErr := strconv.Atoi(raw)
		if nil != parseErr || 1 > limit || searchMaxLimit < limit {
			return req, &customError{message: "limit must be 1 to 1000"}
		}
		req.limit = limit
	}

	if req.cursor, err = boundedQueryValue(values, "cursor", searchMaxCursorBytes); nil != err {
		return req, err
	}

	facets, err := parseOptionalBool(values, "facets")
	if nil != err {
		return req, err
	}
	req.facets = nil != facets && *facets

	return req, nil
}

func (f searchFilter) matches(md *metadata.Metadata, currentInstanceID string) bool {
	if "" != f.entryType && f.entryType != md.Type {
		return false
	}
	if "" != f.query && !nameMatches(md.Name, f.query) {
		return false
	}
	if nil != f.minSize && md.Size < *f.minSize {
		return false
	}
	if nil != f.maxSize && md.Size > *f.maxSize {
		return false
	}
	if nil != f.modifiedAfter && !md.MTime.After(*f.modifiedAfter) {
		return false
	}
	if nil != f.modifiedBefore && !md.MTime.Before(*f.modifiedBefore) {
		return false
	}
	if "" != f.owner && f.owner != md.Owner {
		return false
	}
	if "" != f.storage && f.storage != md.BackendType {
		return false
	}
	if "" != f.instance && f.instance != owningInstanceID(md, currentInstanceID) {
		return false
	}
	if nil != f.erasure && *f.erasure != md.ErasureCoded {
		return false
	}
	return true
}

func nameMatches(name, query string) bool {
	lowerName := strings.ToLower(name)
	lowerQuery := strings.ToLower(query)
	if strings.ContainsAny(lowerQuery, "*?[") {
		matched, err := path.Match(lowerQuery, lowerName)
		return nil == err && matched
	}
	return strings.Contains(lowerName, lowerQuery)
}

func searchItemOf(md *metadata.Metadata, currentInstanceID string) SearchItem {
	instanceID := owningInstanceID(md, currentInstanceID)
	return SearchItem{
		Path:         md.Path,
		Name:         md.Name,
		Type:         md.Type,
		Size:         md.Size,
		Mode:         md.Mode,
		Owner:        md.Owner,
		ETag:         etagField(md.ETag),
		Storage:      md.BackendType,
		InstanceID:   instanceID,
		IsLocal:      instanceID == currentInstanceID,
		ErasureCoded: md.ErasureCoded,
		MTime:        rfc3339UTC(md.MTime),
		UpdatedAt:    rfc3339UTC(md.UpdatedAt),
	}
}

func buildSearchFacets(matched []*metadata.Metadata, currentInstanceID string) map[string]map[string]int {
	facets := map[string]map[string]int{
		"type":     {},
		"storage":  {},
		"owner":    {},
		"instance": {},
	}
	for _, md := range matched {
		facets["type"][md.Type]++
		facets["storage"][md.BackendType]++
		facets["owner"][md.Owner]++
		facets["instance"][owningInstanceID(md, currentInstanceID)]++
	}
	return facets
}

func normalizeQueryPath(raw string) (string, bool) {
	pathInfo := ParseFilePath(raw)
	if pathInfo.IsInvalid {
		return "", false
	}
	enginePath := pathInfo.FullPath
	if pathInfo.IsDirectory && "/" != enginePath {
		enginePath = strings.TrimSuffix(enginePath, "/")
	}
	return enginePath, true
}

func boundedQueryValue(values url.Values, key string, maxBytes int) (string, error) {
	value := values.Get(key)
	if maxBytes < len(value) {
		return "", &customError{message: key + " is too long"}
	}
	return value, nil
}

func parseOptionalInt64(values url.Values, key string) (*int64, error) {
	raw := values.Get(key)
	if "" == raw {
		return nil, nil
	}
	number, err := strconv.ParseInt(raw, 10, 64)
	if nil != err || 0 > number {
		return nil, &customError{message: key + " must be a whole number of bytes, 0 or more"}
	}
	return &number, nil
}

func parseOptionalTime(values url.Values, key string) (*time.Time, error) {
	raw := values.Get(key)
	if "" == raw {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if nil != err {
		return nil, &customError{message: key + " must be an RFC 3339 time"}
	}
	return &parsed, nil
}

func parseOptionalBool(values url.Values, key string) (*bool, error) {
	raw := values.Get(key)
	if "" == raw {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(raw)
	if nil != err {
		return nil, &customError{message: key + " must be true or false"}
	}
	return &parsed, nil
}
