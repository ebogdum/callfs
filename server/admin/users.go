package admin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/config"
	"github.com/ebogdum/callfs/jobs"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/server/handlers"
	"github.com/ebogdum/callfs/server/middleware"
	"github.com/ebogdum/callfs/telemetry"
)

const (
	usersBodyLimit        = 64 << 10
	usersDefaultLimit     = 200
	usersMaxLimit         = telemetry.MaxPageLimit
	usersWindow           = 24 * time.Hour
	usersAPIKeyBytes      = 32
	usersMaxIDLength      = 64
	usersMaxOwnerLength   = 256
	usersMaxPrefixLength  = 4096
	usersMaxCursorLength  = 1024
	usersOwnerJobKind     = "owner_reassign"
	usersRootID           = "root"
	usersInternalProxyID  = "internal-proxy"
	usersPositionalPrefix = "api-user-"
	usersPathAPIKeys      = "auth.api_keys"
	usersPathAPIKeyUsers  = "auth.api_key_users"
	usersPathAdminUsers   = "auth.admin_users"
	usersIDRule           = "use 1 to 64 letters, digits, dots, dashes or underscores; root, internal-proxy and api-user-* names are reserved"
)

var usersMu sync.Mutex

type userItem struct {
	UserID          string     `json:"user_id"`
	Source          string     `json:"source"`
	Index           *int       `json:"index"`
	KeyHint         string     `json:"key_hint"`
	KeyFingerprint  string     `json:"key_fingerprint"`
	IsAdmin         bool       `json:"is_admin"`
	OwnedFiles      int64      `json:"owned_files"`
	OwnedBytes      int64      `json:"owned_bytes"`
	LastAuthAt      *time.Time `json:"last_auth_at"`
	Requests24h     int        `json:"requests_24h"`
	AuthFailures24h *int       `json:"auth_failures_24h"`
}

type usersResponse struct {
	Items           []userItem `json:"items"`
	PositionalCount int        `json:"positional_count"`
}

type userKeyResponse struct {
	UserID string `json:"user_id"`
	APIKey string `json:"api_key"`
}

type createUserRequest struct {
	UserID  string `json:"user_id"`
	IsAdmin bool   `json:"is_admin"`
}

type updateUserRequest struct {
	IsAdmin *bool `json:"is_admin"`
}

type positionalMapping struct {
	Index  int    `json:"index"`
	UserID string `json:"user_id"`
}

type migratePositionalRequest struct {
	Mapping       []positionalMapping `json:"mapping"`
	ReassignOwner *bool               `json:"reassign_owner"`
}

type reassignOwnerRequest struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Prefix string `json:"prefix"`
}

type ownersResponse struct {
	Items      []metadata.OwnerStat `json:"items"`
	NextCursor *string              `json:"next_cursor"`
}

type authFailuresResponse struct {
	Total24h int                     `json:"total_24h"`
	Items    []telemetry.AuthFailure `json:"items"`
}

type reassignResult struct {
	KeysMigrated    int `json:"keys_migrated,omitempty"`
	FilesReassigned int `json:"files_reassigned"`
}

type userValidationIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type userValidationDetails struct {
	Errors []userValidationIssue `json:"errors"`
}

type userErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type userRequestError struct {
	status  int
	code    string
	message string
	issues  []userValidationIssue
}

func (e *userRequestError) Error() string {
	return e.message
}

func userFail(status int, code, message string) error {
	return &userRequestError{status: status, code: code, message: message}
}

type userAuthState struct {
	apiKeys     []string
	apiKeyUsers map[string]string
	adminUsers  []string
}

type userActivity struct {
	requests int
	last     time.Time
}

func (a *API) Users(w http.ResponseWriter, r *http.Request) {
	items, err := a.userItems(r.Context())
	if nil != err {
		handlers.SendErrorResponse(w, a.deps.Logger, err, http.StatusInternalServerError)
		return
	}
	positional := 0
	for i := range items {
		if auth.SourceAPIKeys == items[i].Source {
			positional++
		}
	}
	handlers.SendJSONResponse(w, usersResponse{Items: items, PositionalCount: positional})
}

func (a *API) CreateUser(w http.ResponseWriter, r *http.Request) {
	logger := a.deps.Logger
	var req createUserRequest
	if !decodeUserBody(w, r, logger, &req) {
		return
	}
	if !validUserID(req.UserID) {
		sendUserValidation(w, logger, []userValidationIssue{{Path: "user_id", Message: usersIDRule}})
		return
	}
	var key string
	err := a.changeAuth(func(next *userAuthState) error {
		if a.userKnown(req.UserID) {
			return userFail(http.StatusConflict, "CONFLICT", "user already exists")
		}
		generated, err := newUserAPIKey()
		if nil != err {
			return err
		}
		key = generated
		next.apiKeyUsers[req.UserID] = key
		if req.IsAdmin {
			next.adminUsers = appendUniqueID(next.adminUsers, req.UserID)
		}
		return nil
	})
	if nil != err {
		sendUserFailure(w, logger, err)
		return
	}
	writeUserJSON(w, logger, http.StatusCreated, userKeyResponse{UserID: req.UserID, APIKey: key})
	logger.Info("User created", zap.String("user_id", req.UserID), zap.Bool("is_admin", req.IsAdmin), zap.String("actor", userActor(r)))
}

func (a *API) RotateUserKey(w http.ResponseWriter, r *http.Request) {
	logger := a.deps.Logger
	id := chi.URLParam(r, "id")
	var key string
	err := a.changeAuth(func(next *userAuthState) error {
		if _, ok := next.apiKeyUsers[id]; !ok {
			return a.missingUserError(id)
		}
		generated, err := newUserAPIKey()
		if nil != err {
			return err
		}
		key = generated
		next.apiKeyUsers[id] = key
		return nil
	})
	if nil != err {
		sendUserFailure(w, logger, err)
		return
	}
	handlers.SendJSONResponse(w, userKeyResponse{UserID: id, APIKey: key})
	logger.Info("API key rotated", zap.String("user_id", id), zap.String("actor", userActor(r)))
}

func (a *API) UpdateUser(w http.ResponseWriter, r *http.Request) {
	logger := a.deps.Logger
	id := chi.URLParam(r, "id")
	var req updateUserRequest
	if !decodeUserBody(w, r, logger, &req) {
		return
	}
	if nil == req.IsAdmin {
		sendUserValidation(w, logger, []userValidationIssue{{Path: "is_admin", Message: "is_admin is required"}})
		return
	}
	if usersRootID == id {
		sendUserFailure(w, logger, userFail(http.StatusConflict, "CONFLICT", "root is always an admin"))
		return
	}
	info, ok := a.findUser(id)
	if !ok {
		sendUserFailure(w, logger, userFail(http.StatusNotFound, "NOT_FOUND", "user not found"))
		return
	}
	if auth.SourceAPIKeys == info.Source || auth.SourceReserved == info.Source {
		sendUserFailure(w, logger, userFail(http.StatusConflict, "CONFLICT", "only users with their own key can be admins; migrate numbered keys first"))
		return
	}
	err := a.changeAuth(func(next *userAuthState) error {
		if *req.IsAdmin {
			next.adminUsers = appendUniqueID(next.adminUsers, id)
			return nil
		}
		remaining := withoutID(next.adminUsers, id)
		if slices.Contains(next.adminUsers, id) && 0 == len(remaining) {
			return userFail(http.StatusConflict, "CONFLICT", "cannot remove the last admin")
		}
		next.adminUsers = remaining
		return nil
	})
	if nil != err {
		sendUserFailure(w, logger, err)
		return
	}
	items, err := a.userItems(r.Context())
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	for i := range items {
		if id == items[i].UserID {
			handlers.SendJSONResponse(w, items[i])
			break
		}
	}
	logger.Info("User admin status changed", zap.String("user_id", id), zap.Bool("is_admin", *req.IsAdmin), zap.String("actor", userActor(r)))
}

func (a *API) DeleteUser(w http.ResponseWriter, r *http.Request) {
	logger := a.deps.Logger
	id := chi.URLParam(r, "id")
	err := a.changeAuth(func(next *userAuthState) error {
		if _, ok := next.apiKeyUsers[id]; !ok {
			return a.missingUserError(id)
		}
		remaining := withoutID(next.adminUsers, id)
		if slices.Contains(next.adminUsers, id) && 0 == len(remaining) {
			return userFail(http.StatusConflict, "CONFLICT", "cannot remove the last admin")
		}
		delete(next.apiKeyUsers, id)
		next.adminUsers = remaining
		return nil
	})
	if nil != err {
		sendUserFailure(w, logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	logger.Info("User removed", zap.String("user_id", id), zap.String("actor", userActor(r)))
}

func (a *API) MigratePositional(w http.ResponseWriter, r *http.Request) {
	logger := a.deps.Logger
	var req migratePositionalRequest
	if !decodeUserBody(w, r, logger, &req) {
		return
	}
	if 0 == len(req.Mapping) {
		sendUserValidation(w, logger, []userValidationIssue{{Path: "mapping", Message: "list at least one numbered key to migrate"}})
		return
	}
	reassign := nil == req.ReassignOwner || *req.ReassignOwner
	targets := make(map[string]string)
	err := a.changeAuth(func(next *userAuthState) error {
		slots := positionalSlots(next.apiKeys)
		var issues []userValidationIssue
		indexSeen := make(map[int]struct{}, len(req.Mapping))
		userSeen := make(map[string]struct{}, len(req.Mapping))
		for i, item := range req.Mapping {
			path := "mapping[" + strconv.Itoa(i) + "]"
			if 0 > item.Index || len(slots) <= item.Index {
				issues = append(issues, userValidationIssue{Path: path + ".index", Message: "no numbered key has this index"})
			} else if _, dup := indexSeen[item.Index]; dup {
				issues = append(issues, userValidationIssue{Path: path + ".index", Message: "this index is listed twice"})
			}
			indexSeen[item.Index] = struct{}{}
			if !validUserID(item.UserID) {
				issues = append(issues, userValidationIssue{Path: path + ".user_id", Message: usersIDRule})
			} else if _, dup := userSeen[item.UserID]; dup {
				issues = append(issues, userValidationIssue{Path: path + ".user_id", Message: "this user_id is listed twice"})
			} else if _, taken := next.apiKeyUsers[item.UserID]; taken || a.userKnown(item.UserID) {
				issues = append(issues, userValidationIssue{Path: path + ".user_id", Message: "user already exists"})
			}
			userSeen[item.UserID] = struct{}{}
		}
		if 0 < len(issues) {
			return &userRequestError{status: http.StatusUnprocessableEntity, code: "VALIDATION_FAILED", message: "request failed validation", issues: issues}
		}
		removed := make(map[int]struct{}, len(req.Mapping))
		for _, item := range req.Mapping {
			slot := slots[item.Index]
			removed[slot] = struct{}{}
			next.apiKeyUsers[item.UserID] = next.apiKeys[slot]
			if reassign {
				a.collectPositionalOwners(targets, item.Index, item.UserID)
			}
		}
		kept := make([]string, 0, len(next.apiKeys))
		for slot, key := range next.apiKeys {
			if _, gone := removed[slot]; !gone {
				kept = append(kept, key)
			}
		}
		next.apiKeys = kept
		return nil
	})
	if nil != err {
		sendUserFailure(w, logger, err)
		return
	}
	job := a.submitOwnerJob(r, func(ctx context.Context, progress func(int)) (reassignResult, error) {
		files, err := a.reassignOwners(ctx, progress, "/", targets)
		return reassignResult{KeysMigrated: len(req.Mapping), FilesReassigned: files}, err
	})
	writeUserJSON(w, logger, http.StatusAccepted, job)
}

func (a *API) Owners(w http.ResponseWriter, r *http.Request) {
	logger := a.deps.Logger
	limit, err := parseUserLimit(r.URL.Query().Get("limit"))
	if nil != err {
		sendUserBadRequest(w, logger, err.Error())
		return
	}
	cursor := r.URL.Query().Get("cursor")
	if usersMaxCursorLength < len(cursor) {
		sendUserBadRequest(w, logger, "cursor is too long")
		return
	}
	stats, next, err := a.deps.Store.OwnerStats(r.Context(), cursor, limit)
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	if nil == stats {
		stats = []metadata.OwnerStat{}
	}
	response := ownersResponse{Items: stats}
	if "" != next {
		response.NextCursor = &next
	}
	handlers.SendJSONResponse(w, response)
}

func (a *API) ReassignOwner(w http.ResponseWriter, r *http.Request) {
	logger := a.deps.Logger
	var req reassignOwnerRequest
	if !decodeUserBody(w, r, logger, &req) {
		return
	}
	if "" == req.Prefix {
		req.Prefix = "/"
	}
	var issues []userValidationIssue
	if !validUserID(req.To) {
		issues = append(issues, userValidationIssue{Path: "to", Message: usersIDRule})
	}
	if usersMaxOwnerLength < len(req.From) {
		issues = append(issues, userValidationIssue{Path: "from", Message: "from is too long"})
	}
	if req.From == req.To {
		issues = append(issues, userValidationIssue{Path: "to", Message: "to must differ from from"})
	}
	if !validOwnerPrefix(req.Prefix) {
		issues = append(issues, userValidationIssue{Path: "prefix", Message: "prefix must start with / and must not contain .."})
	}
	if 0 < len(issues) {
		sendUserValidation(w, logger, issues)
		return
	}
	targets := map[string]string{req.From: req.To}
	job := a.submitOwnerJob(r, func(ctx context.Context, progress func(int)) (reassignResult, error) {
		files, err := a.reassignOwners(ctx, progress, req.Prefix, targets)
		return reassignResult{FilesReassigned: files}, err
	})
	writeUserJSON(w, logger, http.StatusAccepted, job)
}

func (a *API) AuthFailures(w http.ResponseWriter, r *http.Request) {
	logger := a.deps.Logger
	now := time.Now().UTC()
	since := now.Add(-usersWindow)
	if raw := r.URL.Query().Get("since"); "" != raw {
		parsed, err := time.Parse(time.RFC3339, raw)
		if nil != err {
			sendUserBadRequest(w, logger, "since must be an RFC 3339 time")
			return
		}
		since = parsed
	}
	limit, err := parseUserLimit(r.URL.Query().Get("limit"))
	if nil != err {
		sendUserBadRequest(w, logger, err.Error())
		return
	}
	window, err := a.deps.Telemetry.AuthFailures.List(now.Add(-usersWindow), 1)
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	page, err := a.deps.Telemetry.AuthFailures.List(since, limit)
	if nil != err {
		handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
		return
	}
	handlers.SendJSONResponse(w, authFailuresResponse{Total24h: window.Total, Items: page.Items})
}

func (a *API) changeAuth(mutate func(next *userAuthState) error) error {
	usersMu.Lock()
	defer usersMu.Unlock()
	next := a.currentAuthState()
	if err := mutate(&next); nil != err {
		return err
	}
	return a.saveAuthState(next)
}

func (a *API) currentAuthState() userAuthState {
	current := a.deps.Config.Auth
	apiKeyUsers := make(map[string]string, len(current.APIKeyUsers))
	for userID, key := range current.APIKeyUsers {
		apiKeyUsers[userID] = key
	}
	return userAuthState{
		apiKeys:     append([]string{}, current.APIKeys...),
		apiKeyUsers: apiKeyUsers,
		adminUsers:  append([]string{}, current.AdminUsers...),
	}
}

func (a *API) saveAuthState(next userAuthState) error {
	original, doc, err := config.ReadConfigDocument(a.deps.ConfigPath)
	if nil != err {
		return err
	}
	changes := []struct {
		path  string
		value any
	}{
		{usersPathAPIKeys, next.apiKeys},
		{usersPathAPIKeyUsers, next.apiKeyUsers},
		{usersPathAdminUsers, next.adminUsers},
	}
	for _, change := range changes {
		if err := config.SetDocumentValue(doc, change.path, change.value); nil != err {
			return err
		}
	}
	if _, _, err := config.WriteConfigDocument(a.deps.ConfigPath, original, doc, time.Now()); nil != err {
		return err
	}
	a.deps.Config.Auth.APIKeys = next.apiKeys
	a.deps.Config.Auth.APIKeyUsers = next.apiKeyUsers
	a.deps.Config.Auth.AdminUsers = next.adminUsers
	a.deps.Authenticator.Reload(next.apiKeys, next.apiKeyUsers, a.deps.Config.Auth.InternalProxySecret)
	a.deps.Admins.Replace(next.adminUsers)
	return nil
}

func (a *API) findUser(id string) (auth.UserInfo, bool) {
	for _, info := range a.deps.Authenticator.Users() {
		if id == info.UserID {
			return info, true
		}
	}
	return auth.UserInfo{}, false
}

func (a *API) userKnown(id string) bool {
	_, ok := a.findUser(id)
	return ok
}

func (a *API) missingUserError(id string) error {
	info, ok := a.findUser(id)
	if !ok {
		return userFail(http.StatusNotFound, "NOT_FOUND", "user not found")
	}
	if auth.SourceAPIKeys == info.Source {
		return userFail(http.StatusConflict, "CONFLICT", "this user has a numbered key; migrate it first")
	}
	return userFail(http.StatusConflict, "CONFLICT", "reserved users have no editable key")
}

func (a *API) collectPositionalOwners(targets map[string]string, index int, userID string) {
	for _, info := range a.deps.Authenticator.Users() {
		if auth.SourceAPIKeys == info.Source && nil != info.Index && index == *info.Index {
			targets[info.UserID] = userID
			for _, alias := range a.deps.Authenticator.LegacyAliases(info.UserID) {
				targets[alias] = userID
			}
		}
	}
}

func (a *API) userItems(ctx context.Context) ([]userItem, error) {
	infos := a.deps.Authenticator.Users()
	owners, err := a.ownerTotals(ctx)
	if nil != err {
		return nil, err
	}
	activity, err := a.recentActivity(time.Now().UTC())
	if nil != err {
		return nil, err
	}
	items := make([]userItem, 0, len(infos))
	for _, info := range infos {
		item := userItem{
			UserID:         info.UserID,
			Source:         info.Source,
			Index:          info.Index,
			KeyFingerprint: info.KeyFingerprint,
			IsAdmin:        a.deps.Admins.IsAdmin(info.UserID),
		}
		if "" != info.KeyHint {
			item.KeyHint = "…" + info.KeyHint
		}
		for _, id := range a.identitiesOf(info.UserID) {
			stat := owners[id]
			item.OwnedFiles += stat.Files
			item.OwnedBytes += stat.Bytes
			seen := activity[id]
			item.Requests24h += seen.requests
			if !seen.last.IsZero() && (nil == item.LastAuthAt || seen.last.After(*item.LastAuthAt)) {
				last := seen.last
				item.LastAuthAt = &last
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func (a *API) identitiesOf(userID string) []string {
	return append([]string{userID}, a.deps.Authenticator.LegacyAliases(userID)...)
}

func (a *API) ownerTotals(ctx context.Context) (map[string]metadata.OwnerStat, error) {
	totals := make(map[string]metadata.OwnerStat)
	cursor := ""
	for {
		stats, next, err := a.deps.Store.OwnerStats(ctx, cursor, usersMaxLimit)
		if nil != err {
			return nil, err
		}
		for _, stat := range stats {
			totals[stat.Owner] = stat
		}
		if "" == next || next == cursor {
			return totals, nil
		}
		cursor = next
	}
}

func (a *API) recentActivity(now time.Time) (map[string]userActivity, error) {
	windowStart := now.Add(-usersWindow)
	activity := make(map[string]userActivity)
	cursor := ""
	for {
		page, err := a.deps.Telemetry.Audit.Query(telemetry.AuditQuery{Limit: usersMaxLimit, Cursor: cursor})
		if nil != err {
			return nil, err
		}
		for i := range page.Items {
			record := &page.Items[i]
			if "" == record.UserID {
				continue
			}
			seen := activity[record.UserID]
			if seen.last.IsZero() || record.At.After(seen.last) {
				seen.last = record.At
			}
			if !record.At.Before(windowStart) {
				seen.requests++
			}
			activity[record.UserID] = seen
		}
		if nil == page.NextCursor {
			return activity, nil
		}
		cursor = *page.NextCursor
	}
}

func (a *API) reassignOwners(ctx context.Context, progress func(int), prefix string, targets map[string]string) (int, error) {
	if 0 == len(targets) {
		return 0, nil
	}
	reassigned := 0
	startAfter := ""
	for {
		files, err := a.deps.Store.ListByPrefix(ctx, prefix, startAfter, usersMaxLimit)
		if nil != err {
			return reassigned, err
		}
		for _, md := range files {
			startAfter = md.Path
			to, ok := targets[md.Owner]
			if !ok {
				continue
			}
			md.Owner = to
			if err := a.deps.Store.UpdateIfMatch(ctx, md, md.ETag); nil != err {
				return reassigned, err
			}
			reassigned++
		}
		progress(len(files))
		if usersMaxLimit > len(files) {
			return reassigned, nil
		}
	}
}

func (a *API) submitOwnerJob(r *http.Request, run func(ctx context.Context, progress func(int)) (reassignResult, error)) jobs.Job {
	logger := a.deps.Logger
	actor := userActor(r)
	return a.deps.Jobs.Submit(usersOwnerJobKind, actor, func(ctx context.Context, p *jobs.Progress) (any, error) {
		logger.Info("Owner reassign started", zap.String("actor", actor))
		result, err := run(ctx, p.Advance)
		if nil != err {
			logger.Error("Owner reassign failed", zap.String("actor", actor), zap.Error(err))
			return nil, err
		}
		logger.Info("Owner reassign finished", zap.String("actor", actor), zap.Int("files_reassigned", result.FilesReassigned))
		return result, nil
	})
}

func decodeUserBody(w http.ResponseWriter, r *http.Request, logger *zap.Logger, target any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, usersBodyLimit)).Decode(target); nil != err {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeUserJSON(w, logger, http.StatusRequestEntityTooLarge, userErrorBody{Code: "PAYLOAD_TOO_LARGE", Message: "request body is too large"})
			return false
		}
		sendUserBadRequest(w, logger, "request body must be valid JSON")
		return false
	}
	return true
}

func sendUserBadRequest(w http.ResponseWriter, logger *zap.Logger, message string) {
	writeUserJSON(w, logger, http.StatusBadRequest, userErrorBody{Code: "BAD_REQUEST", Message: message})
}

func sendUserValidation(w http.ResponseWriter, logger *zap.Logger, issues []userValidationIssue) {
	writeUserJSON(w, logger, http.StatusUnprocessableEntity, userErrorBody{
		Code:    "VALIDATION_FAILED",
		Message: "request failed validation",
		Details: userValidationDetails{Errors: issues},
	})
}

func sendUserFailure(w http.ResponseWriter, logger *zap.Logger, err error) {
	var requestErr *userRequestError
	if errors.As(err, &requestErr) {
		if 0 < len(requestErr.issues) {
			sendUserValidation(w, logger, requestErr.issues)
			return
		}
		writeUserJSON(w, logger, requestErr.status, userErrorBody{Code: requestErr.code, Message: requestErr.message})
		return
	}
	handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
}

func writeUserJSON(w http.ResponseWriter, logger *zap.Logger, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); nil != err {
		logger.Error("Failed to encode admin response", zap.Error(err))
	}
}

func parseUserLimit(raw string) (int, error) {
	if "" == raw {
		return usersDefaultLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if nil != err || 1 > limit || usersMaxLimit < limit {
		return 0, errors.New("limit must be a whole number from 1 to 1000")
	}
	return limit, nil
}

func validUserID(id string) bool {
	if 0 == len(id) || usersMaxIDLength < len(id) {
		return false
	}
	if usersRootID == id || usersInternalProxyID == id || strings.HasPrefix(id, usersPositionalPrefix) {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !('a' <= c && 'z' >= c || 'A' <= c && 'Z' >= c || '0' <= c && '9' >= c || '-' == c || '_' == c || '.' == c) {
			return false
		}
	}
	return true
}

func validOwnerPrefix(prefix string) bool {
	return usersMaxPrefixLength >= len(prefix) &&
		strings.HasPrefix(prefix, "/") &&
		!strings.Contains(prefix, "..") &&
		!strings.ContainsRune(prefix, 0)
}

func newUserAPIKey() (string, error) {
	buf := make([]byte, usersAPIKeyBytes)
	if _, err := rand.Read(buf); nil != err {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func positionalSlots(keys []string) []int {
	slots := make([]int, 0, len(keys))
	for slot, key := range keys {
		if "" != key {
			slots = append(slots, slot)
		}
	}
	return slots
}

func appendUniqueID(ids []string, id string) []string {
	if slices.Contains(ids, id) {
		return ids
	}
	return append(ids, id)
}

func withoutID(ids []string, id string) []string {
	kept := make([]string, 0, len(ids))
	for _, existing := range ids {
		if id != existing {
			kept = append(kept, existing)
		}
	}
	return kept
}

func userActor(r *http.Request) string {
	actor, _ := middleware.GetUserID(r.Context())
	return actor
}
