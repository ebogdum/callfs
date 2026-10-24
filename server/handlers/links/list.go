package links

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/jobs"
	"github.com/ebogdum/callfs/v2/links"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/server/handlers"
	"github.com/ebogdum/callfs/v2/server/middleware"
	"github.com/ebogdum/callfs/v2/telemetry"
)

const (
	linkListDefaultLimit = 200
	linkListMaxLimit     = 1000
	linkDefaultWindow    = "24h"
	linkUsedRecentWindow = 24 * time.Hour
)

var (
	errLinkNotActive     = errors.New("link is not active")
	errInvalidLinkStatus = errors.New("status must be active, used, expired or revoked")
	errInvalidLinkWindow = errors.New("window must be 1h, 24h or 7d")
	errInvalidLinkLimit  = errors.New("limit must be between 1 and 1000")
	errInvalidLinkID     = errors.New("link id must be a positive integer")
	errInvalidLinkPath   = errors.New("invalid path")
)

var (
	linkStatusValues = map[string]struct{}{"active": {}, "used": {}, "expired": {}, "revoked": {}}
	linkStatsWindows = map[string]time.Duration{"1h": time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour}
)

type linkCounts struct {
	Active  int `json:"active"`
	Used    int `json:"used"`
	Expired int `json:"expired"`
	Revoked int `json:"revoked"`
}

type linkItem struct {
	ID          int64      `json:"id"`
	TokenPrefix string     `json:"token_prefix"`
	FilePath    string     `json:"file_path"`
	Status      string     `json:"status"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	UsedAt      *time.Time `json:"used_at"`
	UsedByIP    *string    `json:"used_by_ip"`
}

type linkListResponse struct {
	Items      []linkItem `json:"items"`
	NextCursor *string    `json:"next_cursor"`
	Counts     linkCounts `json:"counts"`
}

type linkAttempt struct {
	At     time.Time `json:"at"`
	Result string    `json:"result"`
	IP     string    `json:"ip"`
}

type linkDetail struct {
	linkItem
	UpdatedAt time.Time     `json:"updated_at"`
	Attempts  []linkAttempt `json:"attempts"`
}

type linkRevokeResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

type linkConsumed struct {
	Success  int `json:"success"`
	NotFound int `json:"not_found"`
	Expired  int `json:"expired"`
	Invalid  int `json:"invalid"`
}

type linkStatsResponse struct {
	Window      string       `json:"window"`
	Generated   int          `json:"generated"`
	Consumed    linkConsumed `json:"consumed"`
	Active      int          `json:"active"`
	UsedLast24h int          `json:"used_last_24h"`
	LastCleanup linkCleanup  `json:"last_cleanup"`
}

type linkCleanup struct {
	At             *time.Time `json:"at"`
	ExpiredRemoved int        `json:"expired_removed"`
	UsedRemoved    int        `json:"used_removed"`
	Error          *string    `json:"error"`
}

func lastLinkCleanup(runner *jobs.Runner) linkCleanup {
	var result linkCleanup
	if nil == runner {
		return result
	}
	for _, status := range runner.Workers() {
		if "link_cleanup" != status.Name {
			continue
		}
		result.At = status.LastRunAt
		result.Error = status.LastError
		if counts, ok := status.LastResult.(map[string]int); ok {
			result.ExpiredRemoved = counts["expired_removed"]
			result.UsedRemoved = counts["used_removed"]
		}
	}
	return result
}

func V1ListLinks(store metadata.Store, admins *auth.AdminSet) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := zap.L()
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			handlers.SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		query := r.URL.Query()
		status, err := parseLinkStatus(query.Get("status"))
		if nil != err {
			handlers.SendErrorResponse(w, logger, err, http.StatusBadRequest)
			return
		}
		limit, err := parseLinkLimit(query.Get("limit"))
		if nil != err {
			handlers.SendErrorResponse(w, logger, err, http.StatusBadRequest)
			return
		}
		filter := metadata.LinkFilter{
			Status:    status,
			CreatedBy: query.Get("created_by"),
			Now:       time.Now(),
			Limit:     limit,
			Cursor:    query.Get("cursor"),
		}
		if "" != query.Get("path") {
			filePath, pathErr := canonicalLinkPath(query.Get("path"))
			if nil != pathErr {
				handlers.SendErrorResponse(w, logger, pathErr, http.StatusBadRequest)
				return
			}
			filter.FilePath = filePath
		}
		if !admins.IsAdmin(userID) {
			filter.CreatedBy = userID
		}

		countFilter := filter
		countFilter.Status = ""
		countFilter.Cursor = ""
		counts, err := countLinks(r.Context(), store, countFilter)
		if nil != err {
			handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
		page, next, err := store.ListSingleUseLinks(r.Context(), filter)
		if nil != err {
			handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}

		items := make([]linkItem, 0, len(page))
		for _, link := range page {
			items = append(items, newLinkItem(link, filter.Now))
		}
		response := linkListResponse{Items: items, Counts: counts}
		if "" != next {
			response.NextCursor = &next
		}
		handlers.SendJSONResponse(w, response)
	}
}

func V1GetLink(store metadata.Store, admins *auth.AdminSet, hub *telemetry.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := zap.L()
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			handlers.SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		id, err := parseLinkID(chi.URLParam(r, "id"))
		if nil != err {
			handlers.SendErrorResponse(w, logger, err, http.StatusBadRequest)
			return
		}
		link, err := loadVisibleLink(r.Context(), store, admins, userID, id)
		if nil != err {
			sendLinkLookupError(w, logger, err)
			return
		}
		handlers.SendJSONResponse(w, linkDetail{
			linkItem:  newLinkItem(*link, time.Now()),
			UpdatedAt: link.UpdatedAt,
			Attempts:  linkAttempts(hub, id, logger),
		})
	}
}

func V1RevokeLink(store metadata.Store, admins *auth.AdminSet) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := zap.L()
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			handlers.SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		id, err := parseLinkID(chi.URLParam(r, "id"))
		if nil != err {
			handlers.SendErrorResponse(w, logger, err, http.StatusBadRequest)
			return
		}
		link, err := loadVisibleLink(r.Context(), store, admins, userID, id)
		if nil != err {
			sendLinkLookupError(w, logger, err)
			return
		}
		if "active" != effectiveLinkStatus(*link, time.Now()) {
			handlers.SendErrorResponse(w, logger, errLinkNotActive, http.StatusConflict)
			return
		}
		if err := store.RevokeSingleUseLink(r.Context(), id); nil != err {
			sendLinkLookupError(w, logger, err)
			return
		}
		handlers.SendJSONResponse(w, linkRevokeResponse{ID: id, Status: "revoked"})
		logger.Info("Revoked single-use link",
			zap.Int64("link_id", id),
			zap.String("user_id", userID))
	}
}

func V1LinkStats(store metadata.Store, admins *auth.AdminSet, hub *telemetry.Hub, runner *jobs.Runner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := zap.L()
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			handlers.SendErrorResponse(w, logger, auth.ErrAuthenticationFailed, http.StatusUnauthorized)
			return
		}
		window, duration, err := parseLinkStatsWindow(r.URL.Query().Get("window"))
		if nil != err {
			handlers.SendErrorResponse(w, logger, err, http.StatusBadRequest)
			return
		}
		now := time.Now()
		since := now.Add(-duration)
		admin := admins.IsAdmin(userID)
		var generated int
		var consumed linkConsumed
		if admin {
			generated, consumed, err = linkEventCounts(hub, since)
			if nil != err {
				handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
				return
			}
		}
		filter := metadata.LinkFilter{Now: now}
		if !admin {
			filter.CreatedBy = userID
		}
		var active, usedRecent int
		cutoff := now.Add(-linkUsedRecentWindow)
		err = scanLinks(r.Context(), store, filter, func(link metadata.SingleUseLink) {
			if !admin {
				if !link.CreatedAt.Before(since) {
					generated++
				}
				if nil != link.UsedAt && !link.UsedAt.Before(since) {
					consumed.Success++
				}
			}
			switch effectiveLinkStatus(link, now) {
			case "active":
				active++
			case "used":
				if nil != link.UsedAt && !link.UsedAt.Before(cutoff) {
					usedRecent++
				}
			}
		})
		if nil != err {
			handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
			return
		}
		handlers.SendJSONResponse(w, linkStatsResponse{
			Window:      window,
			Generated:   generated,
			Consumed:    consumed,
			Active:      active,
			UsedLast24h: usedRecent,
			LastCleanup: lastLinkCleanup(runner),
		})
	}
}

func parseLinkStatus(raw string) (string, error) {
	if "" == raw {
		return "", nil
	}
	if _, ok := linkStatusValues[raw]; !ok {
		return "", errInvalidLinkStatus
	}
	return raw, nil
}

func parseLinkLimit(raw string) (int, error) {
	if "" == raw {
		return linkListDefaultLimit, nil
	}
	limit, err := strconv.Atoi(raw)
	if nil != err || 1 > limit || linkListMaxLimit < limit {
		return 0, errInvalidLinkLimit
	}
	return limit, nil
}

func parseLinkStatsWindow(raw string) (string, time.Duration, error) {
	if "" == raw {
		raw = linkDefaultWindow
	}
	duration, ok := linkStatsWindows[raw]
	if !ok {
		return "", 0, errInvalidLinkWindow
	}
	return raw, duration, nil
}

func parseLinkID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if nil != err || 1 > id {
		return 0, errInvalidLinkID
	}
	return id, nil
}

func canonicalLinkPath(raw string) (string, error) {
	pathInfo := handlers.ParseFilePath(strings.TrimPrefix(raw, "/"))
	if pathInfo.IsInvalid {
		return "", errInvalidLinkPath
	}
	return pathInfo.FullPath, nil
}

func effectiveLinkStatus(link metadata.SingleUseLink, now time.Time) string {
	if "active" == link.Status && now.After(link.ExpiresAt) {
		return "expired"
	}
	return link.Status
}

func newLinkItem(link metadata.SingleUseLink, now time.Time) linkItem {
	return linkItem{
		ID:          link.ID,
		TokenPrefix: links.TokenPrefix(link.Token),
		FilePath:    link.FilePath,
		Status:      effectiveLinkStatus(link, now),
		CreatedBy:   link.Owner,
		CreatedAt:   link.CreatedAt,
		ExpiresAt:   link.ExpiresAt,
		UsedAt:      link.UsedAt,
		UsedByIP:    link.UsedByIP,
	}
}

func scanLinks(ctx context.Context, store metadata.Store, filter metadata.LinkFilter, visit func(metadata.SingleUseLink)) error {
	filter.Limit = linkListMaxLimit
	for {
		page, next, err := store.ListSingleUseLinks(ctx, filter)
		if nil != err {
			return err
		}
		for _, link := range page {
			visit(link)
		}
		if "" == next {
			return nil
		}
		filter.Cursor = next
	}
}

func countLinks(ctx context.Context, store metadata.Store, filter metadata.LinkFilter) (linkCounts, error) {
	var counts linkCounts
	err := scanLinks(ctx, store, filter, func(link metadata.SingleUseLink) {
		switch effectiveLinkStatus(link, filter.Now) {
		case "active":
			counts.Active++
		case "used":
			counts.Used++
		case "expired":
			counts.Expired++
		case "revoked":
			counts.Revoked++
		}
	})
	return counts, err
}

func loadVisibleLink(ctx context.Context, store metadata.Store, admins *auth.AdminSet, userID string, id int64) (*metadata.SingleUseLink, error) {
	link, err := store.GetSingleUseLinkByID(ctx, id)
	if nil != err {
		if errors.Is(err, metadata.ErrNotFound) {
			return nil, links.ErrLinkNotFound
		}
		return nil, err
	}
	if !admins.IsAdmin(userID) && userID != link.Owner {
		return nil, links.ErrLinkNotFound
	}
	return link, nil
}

func sendLinkLookupError(w http.ResponseWriter, logger *zap.Logger, err error) {
	if errors.Is(err, metadata.ErrNotFound) || errors.Is(err, links.ErrLinkNotFound) {
		handlers.SendErrorResponse(w, logger, links.ErrLinkNotFound, http.StatusNotFound)
		return
	}
	handlers.SendErrorResponse(w, logger, err, http.StatusInternalServerError)
}

func linkAttempts(hub *telemetry.Hub, id int64, logger *zap.Logger) []linkAttempt {
	attempts := make([]linkAttempt, 0)
	if nil == hub || nil == hub.Events {
		return attempts
	}
	events, err := hub.Events.Query(telemetry.EventQuery{
		Types: []string{"link.opened", "link.failed"},
		Limit: telemetry.MaxPageLimit,
	})
	if nil != err {
		logger.Warn("Failed to read link attempts from event history", zap.Error(err))
		return attempts
	}
	entityID := strconv.FormatInt(id, 10)
	for _, event := range events {
		if !eventHasLinkEntity(event, entityID) {
			continue
		}
		attempts = append(attempts, linkAttempt{
			At:     event.At,
			Result: linkAttemptResult(event),
			IP:     eventFieldString(event, "ip"),
		})
	}
	return attempts
}

func eventHasLinkEntity(event telemetry.Event, entityID string) bool {
	for _, entity := range event.Entities {
		if "link" == entity.Kind && entityID == entity.ID {
			return true
		}
	}
	return false
}

func linkAttemptResult(event telemetry.Event) string {
	if "link.opened" == event.Type {
		return "success"
	}
	return eventFieldString(event, "result")
}

func eventFieldString(event telemetry.Event, key string) string {
	value, _ := event.Fields[key].(string)
	return value
}

func linkEventCounts(hub *telemetry.Hub, since time.Time) (int, linkConsumed, error) {
	var generated int
	var consumed linkConsumed
	if nil == hub || nil == hub.Events {
		return generated, consumed, nil
	}
	events, err := hub.Events.Query(telemetry.EventQuery{
		Types: []string{"link.created", "link.opened", "link.failed"},
		Since: since,
		Limit: telemetry.MaxPageLimit,
	})
	if nil != err {
		return 0, linkConsumed{}, err
	}
	for _, event := range events {
		switch event.Type {
		case "link.created":
			generated++
		case "link.opened":
			consumed.Success++
		case "link.failed":
			switch eventFieldString(event, "result") {
			case "not_found":
				consumed.NotFound++
			case "expired":
				consumed.Expired++
			case "invalid":
				consumed.Invalid++
			}
		}
	}
	return generated, consumed, nil
}
