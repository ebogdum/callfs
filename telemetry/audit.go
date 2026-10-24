package telemetry

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	OpRead       = "read"
	OpList       = "list"
	OpStat       = "stat"
	OpUpload     = "upload"
	OpReplace    = "replace"
	OpRename     = "rename"
	OpMove       = "move"
	OpDelete     = "delete"
	OpTrash      = "trash"
	OpRestore    = "restore"
	OpLinkCreate = "link_create"
	OpLinkOpen   = "link_open"
	OpSession    = "session"
	OpAdmin      = "admin"
)

var auditOps = map[string]bool{
	OpRead: true, OpList: true, OpStat: true, OpUpload: true, OpReplace: true, OpRename: true,
	OpMove: true, OpDelete: true, OpTrash: true, OpRestore: true, OpLinkCreate: true,
	OpLinkOpen: true, OpSession: true, OpAdmin: true,
}

var listRoutes = map[string]bool{
	"/v1/links":         true,
	"/v1/trash":         true,
	"/v1/erasure/files": true,
	"/v1/jobs":          true,
}

type AuditRecord struct {
	ID         uint64    `json:"-"`
	At         time.Time `json:"at"`
	RequestID  string    `json:"request_id"`
	UserID     string    `json:"user_id"`
	Method     string    `json:"method"`
	Route      string    `json:"route"`
	Path       string    `json:"path"`
	Op         string    `json:"op"`
	Status     int       `json:"status"`
	BytesIn    int64     `json:"bytes_in"`
	BytesOut   int64     `json:"bytes_out"`
	DurationMS int64     `json:"duration_ms"`
	InstanceID string    `json:"instance_id"`
	Client     string    `json:"client"`
}

type AuditQuery struct {
	Since      time.Time
	Until      time.Time
	UserID     string
	Op         string
	PathPrefix string
	Status     string
	RequestID  string
	Limit      int
	Cursor     string
}

type AuditPage struct {
	Items      []AuditRecord `json:"items"`
	NextCursor *string       `json:"next_cursor"`
}

type auditSlot struct {
	mu     sync.Mutex
	record AuditRecord
}

type AuditRing struct {
	instanceID string
	head       atomic.Uint64
	slots      []auditSlot
}

func NewAuditRing(instanceID string, capacity int) (*AuditRing, error) {
	if 1 > capacity {
		return nil, ErrInvalidCapacity
	}
	return &AuditRing{instanceID: instanceID, slots: make([]auditSlot, capacity)}, nil
}

func (a *AuditRing) Add(record AuditRecord) {
	id := a.head.Add(1)
	record.ID = id
	record.InstanceID = a.instanceID
	slot := &a.slots[(id-1)%uint64(len(a.slots))]
	slot.mu.Lock()
	slot.record = record
	slot.mu.Unlock()
}

func (a *AuditRing) size() int {
	head := a.head.Load()
	if head > uint64(len(a.slots)) {
		return len(a.slots)
	}
	return int(head)
}

func (a *AuditRing) scanNewest(visit func(AuditRecord) bool) {
	head := a.head.Load()
	capacity := uint64(len(a.slots))
	for id := head; 0 < id; id-- {
		if head-id >= capacity {
			return
		}
		slot := &a.slots[(id-1)%capacity]
		slot.mu.Lock()
		record := slot.record
		slot.mu.Unlock()
		if record.ID != id {
			continue
		}
		if !visit(record) {
			return
		}
	}
}

func (a *AuditRing) Query(query AuditQuery) (AuditPage, error) {
	limit, err := pageLimit(query.Limit)
	if nil != err {
		return AuditPage{}, err
	}
	before, err := parseCursor(query.Cursor)
	if nil != err {
		return AuditPage{}, err
	}
	matches, err := query.compile()
	if nil != err {
		return AuditPage{}, err
	}
	items := make([]AuditRecord, 0, min(limit, a.size()))
	hasMore := false
	a.scanNewest(func(record AuditRecord) bool {
		if 0 < before && record.ID >= before {
			return true
		}
		if !matches(record) {
			return true
		}
		if len(items) == limit {
			hasMore = true
			return false
		}
		items = append(items, record)
		return true
	})
	var next *string
	if hasMore {
		cursor := encodeCursor(items[len(items)-1].ID)
		next = &cursor
	}
	return AuditPage{Items: items, NextCursor: next}, nil
}

func (a *AuditRing) Scan(query AuditQuery, visit func(*AuditRecord) bool) error {
	before, err := parseCursor(query.Cursor)
	if nil != err {
		return err
	}
	matches, err := query.compile()
	if nil != err {
		return err
	}
	a.scanNewest(func(record AuditRecord) bool {
		if 0 < before && record.ID >= before {
			return true
		}
		if !matches(record) {
			return true
		}
		return visit(&record)
	})
	return nil
}

func (q AuditQuery) compile() (func(AuditRecord) bool, error) {
	if "" != q.Op && !auditOps[q.Op] {
		return nil, fmt.Errorf("%w: op", ErrInvalidQuery)
	}
	status, err := parseStatusFilter(q.Status)
	if nil != err {
		return nil, err
	}
	prefix := strings.TrimPrefix(q.PathPrefix, "/")
	return func(record AuditRecord) bool {
		if !q.Since.IsZero() && record.At.Before(q.Since) {
			return false
		}
		if !q.Until.IsZero() && record.At.After(q.Until) {
			return false
		}
		if "" != q.UserID && record.UserID != q.UserID {
			return false
		}
		if "" != q.Op && record.Op != q.Op {
			return false
		}
		if "" != prefix && !strings.HasPrefix(strings.TrimPrefix(record.Path, "/"), prefix) {
			return false
		}
		if nil != status && !status(record.Status) {
			return false
		}
		return "" == q.RequestID || record.RequestID == q.RequestID
	}, nil
}

func parseStatusFilter(expression string) (func(int) bool, error) {
	if "" == expression {
		return nil, nil
	}
	operator := "="
	for _, candidate := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(expression, candidate) {
			operator = candidate
			expression = strings.TrimPrefix(expression, candidate)
			break
		}
	}
	value, err := strconv.Atoi(strings.TrimSpace(expression))
	if nil != err {
		return nil, fmt.Errorf("%w: status", ErrInvalidQuery)
	}
	switch operator {
	case ">=":
		return func(status int) bool { return status >= value }, nil
	case "<=":
		return func(status int) bool { return status <= value }, nil
	case ">":
		return func(status int) bool { return status > value }, nil
	case "<":
		return func(status int) bool { return status < value }, nil
	default:
		return func(status int) bool { return status == value }, nil
	}
}

type auditStateKey struct{}

type auditState struct {
	userID string
	path   string
}

func SetAuditPath(ctx context.Context, path string) {
	if state, ok := ctx.Value(auditStateKey{}).(*auditState); ok {
		state.path = path
	}
}

func SetAuditUser(ctx context.Context, userID string) {
	if state, ok := ctx.Value(auditStateKey{}).(*auditState); ok {
		state.userID = userID
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (s *statusRecorder) WriteHeader(code int) {
	if 0 == s.status && http.StatusOK <= code {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(payload []byte) (int, error) {
	if 0 == s.status {
		s.status = http.StatusOK
	}
	written, err := s.ResponseWriter.Write(payload)
	s.bytes += int64(written)
	return written, err
}

func (s *statusRecorder) ReadFrom(src io.Reader) (int64, error) {
	if 0 == s.status {
		s.status = http.StatusOK
	}
	var copied int64
	var err error
	if readerFrom, ok := s.ResponseWriter.(io.ReaderFrom); ok {
		copied, err = readerFrom.ReadFrom(src)
	} else {
		copied, err = io.Copy(struct{ io.Writer }{s.ResponseWriter}, src)
	}
	s.bytes += copied
	return copied, err
}

func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(s.ResponseWriter).Hijack()
	if nil == err && 0 == s.status {
		s.status = http.StatusSwitchingProtocols
	}
	return conn, rw, err
}

func (s *statusRecorder) Flush() {
	_ = http.NewResponseController(s.ResponseWriter).Flush()
}

func AuditMiddleware(hub *Hub) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			state := &auditState{}
			recorder := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(recorder, r.WithContext(context.WithValue(r.Context(), auditStateKey{}, state)))
			route := routePattern(r)
			status := recorder.status
			if 0 == status {
				status = http.StatusOK
			}
			record := AuditRecord{
				At:         started.UTC(),
				RequestID:  recorder.Header().Get("X-Request-ID"),
				UserID:     state.userID,
				Method:     r.Method,
				Route:      route,
				Path:       auditPathOf(r, route, state),
				Op:         classifyOp(r, route),
				Status:     status,
				BytesIn:    max(r.ContentLength, 0),
				BytesOut:   recorder.bytes,
				DurationMS: time.Since(started).Milliseconds(),
				Client:     clientAddress(r.RemoteAddr),
			}
			hub.Audit.Add(record)
			if http.StatusUnauthorized == record.Status && !presentedCredentials(r) {
				return
			}
			if event, found := eventOfAudit(record); found {
				hub.Events.record(event)
			}
		})
	}
}

type auditEventKind struct {
	eventType string
	verb      string
	entity    string
}

var auditEventKinds = map[string]auditEventKind{
	OpUpload:     {"file.created", "uploaded", "file"},
	OpReplace:    {"file.updated", "replaced", "file"},
	OpRename:     {"file.renamed", "renamed", "file"},
	OpMove:       {"file.moved", "moved", "file"},
	OpDelete:     {"file.deleted", "deleted", "file"},
	OpTrash:      {"file.trashed", "moved to trash", "file"},
	OpRestore:    {"file.restored", "restored", "file"},
	OpLinkCreate: {"link.created", "created a link for", "link"},
	OpLinkOpen:   {"link.opened", "opened a link to", "link"},
}

func eventOfAudit(record AuditRecord) (Event, bool) {
	var actor *string
	if "" != record.UserID {
		user := record.UserID
		actor = &user
	}
	who := record.UserID
	if "" == who {
		who = "Someone"
	}
	fields := map[string]any{"route": record.Route, "status": record.Status, "request_id": record.RequestID}
	switch {
	case http.StatusUnauthorized == record.Status:
		return Event{At: record.At, Type: "security.auth_failed", Level: LevelWarn, Message: "Sign-in failed from " + record.Client, Fields: fields}, true
	case http.StatusTooManyRequests == record.Status:
		return Event{At: record.At, Type: "security.rate_limited", Level: LevelWarn, Actor: actor, Message: who + " was slowed down by a rate limit", Fields: fields}, true
	}
	kind, tracked := auditEventKinds[record.Op]
	if !tracked {
		return Event{}, false
	}
	if http.StatusBadRequest <= record.Status {
		if OpLinkOpen != record.Op {
			return Event{}, false
		}
		return Event{At: record.At, Type: "link.failed", Level: LevelWarn, Actor: actor, Message: "A link could not be opened", Fields: fields}, true
	}
	entities := []Entity{}
	message := who + " " + kind.verb
	if "" != record.Path {
		entities = append(entities, Entity{Kind: kind.entity, ID: record.Path})
		message += " " + record.Path
	}
	return Event{At: record.At, Type: kind.eventType, Level: LevelInfo, Actor: actor, Message: message, Entities: entities, Fields: fields}, true
}

func routePattern(r *http.Request) string {
	routeContext := chi.RouteContext(r.Context())
	if nil == routeContext {
		return ""
	}
	return routeContext.RoutePattern()
}

func auditPathOf(r *http.Request, route string, state *auditState) string {
	if "" != state.path {
		return state.path
	}
	return auditPath(r, route)
}

func auditPath(r *http.Request, route string) string {
	if hasPathPrefix(r.URL.Path, "/download") {
		return ""
	}
	if routeContext := chi.RouteContext(r.Context()); nil != routeContext {
		if wildcard := routeContext.URLParam("*"); "" != wildcard {
			return wildcard
		}
	}
	return r.URL.Path
}

func classifyOp(r *http.Request, route string) string {
	switch {
	case hasPathPrefix(route, "/v1/admin") && http.MethodGet != r.Method && http.MethodHead != r.Method:
		return OpAdmin
	case hasPathPrefix(route, "/v1/session"):
		return OpSession
	case hasPathPrefix(route, "/download"):
		return OpLinkOpen
	case "/v1/links/generate" == route:
		return OpLinkCreate
	case hasPathPrefix(route, "/v1/trash") && strings.HasSuffix(route, "/restore"):
		return OpRestore
	case hasPathPrefix(route, "/v1/stat"):
		return OpStat
	case hasPathPrefix(route, "/v1/erasure/verify"):
		return OpRead
	case hasPathPrefix(route, "/v1/erasure/repair"):
		return OpReplace
	case http.MethodDelete == r.Method && hasPathPrefix(route, "/v1/files") && "true" == r.URL.Query().Get("trash"):
		return OpTrash
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		if hasPathPrefix(route, "/v1/directories") || hasPathPrefix(route, "/v1/search") || hasPathPrefix(route, "/v1/objects") || listRoutes[route] {
			return OpList
		}
		return OpRead
	case http.MethodPut:
		if hasPathPrefix(route, "/v1/files") && "" != r.Header.Get("If-Match") {
			return OpReplace
		}
		return OpUpload
	case http.MethodPost:
		return OpUpload
	case http.MethodPatch:
		return OpRename
	case http.MethodDelete:
		return OpDelete
	default:
		return OpRead
	}
}

func hasPathPrefix(route, prefix string) bool {
	if !strings.HasPrefix(route, prefix) {
		return false
	}
	return len(route) == len(prefix) || '/' == route[len(prefix)]
}

func clientAddress(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if nil != err {
		return remoteAddr
	}
	return host
}

type AuthFailure struct {
	At     time.Time `json:"at"`
	Client string    `json:"client"`
	Reason string    `json:"reason"`
	Path   string    `json:"path"`
}

type AuthFailurePage struct {
	Total int           `json:"total"`
	Items []AuthFailure `json:"items"`
}

type AuthFailureRing struct {
	entries *ring[AuthFailure]
}

func NewAuthFailureRing(capacity int) (*AuthFailureRing, error) {
	entries, err := newRing[AuthFailure](capacity)
	if nil != err {
		return nil, err
	}
	return &AuthFailureRing{entries: entries}, nil
}

func (r *AuthFailureRing) Record(failure AuthFailure) {
	if failure.At.IsZero() {
		failure.At = time.Now().UTC()
	}
	r.entries.add(failure)
}

func (r *AuthFailureRing) List(since time.Time, limit int) (AuthFailurePage, error) {
	pageSize, err := pageLimit(limit)
	if nil != err {
		return AuthFailurePage{}, err
	}
	page := AuthFailurePage{Items: make([]AuthFailure, 0, pageSize)}
	r.entries.scan(func(failure AuthFailure) bool {
		if !since.IsZero() && failure.At.Before(since) {
			return true
		}
		page.Total++
		if len(page.Items) < pageSize {
			page.Items = append(page.Items, failure)
		}
		return true
	})
	return page, nil
}

type ReplicationFailure struct {
	At    time.Time `json:"at"`
	Path  string    `json:"path"`
	Error string    `json:"error"`
}

type ReplicationFailureRing struct {
	entries *ring[ReplicationFailure]
}

func NewReplicationFailureRing(capacity int) (*ReplicationFailureRing, error) {
	entries, err := newRing[ReplicationFailure](capacity)
	if nil != err {
		return nil, err
	}
	return &ReplicationFailureRing{entries: entries}, nil
}

func (r *ReplicationFailureRing) Record(path, message string) {
	r.entries.add(ReplicationFailure{At: time.Now().UTC(), Path: path, Error: message})
}

func (r *ReplicationFailureRing) Recent(limit int) ([]ReplicationFailure, error) {
	pageSize, err := pageLimit(limit)
	if nil != err {
		return nil, err
	}
	failures := make([]ReplicationFailure, 0, pageSize)
	r.entries.scan(func(failure ReplicationFailure) bool {
		failures = append(failures, failure)
		return len(failures) < pageSize
	})
	return failures, nil
}

func (r *ReplicationFailureRing) CountSince(since time.Time) int {
	count := 0
	r.entries.scan(func(failure ReplicationFailure) bool {
		if !failure.At.Before(since) {
			count++
		}
		return true
	})
	return count
}

func presentedCredentials(r *http.Request) bool {
	if "" != r.Header.Get("Authorization") {
		return true
	}
	if cookie, err := r.Cookie("callfs_session"); nil == err && "" != cookie.Value {
		return true
	}
	return nil != r.Body && http.MethodPost == r.Method && strings.HasSuffix(r.URL.Path, "/session")
}
