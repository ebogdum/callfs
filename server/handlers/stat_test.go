package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/core"
	"github.com/ebogdum/callfs/v2/locks"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/server/middleware"
)

const statTestToken = "Bearer test-key"

type fakeStatAuthenticator struct{}

func (fakeStatAuthenticator) Authenticate(_ context.Context, token string) (string, error) {
	if statTestToken != token {
		return "", auth.ErrAuthenticationFailed
	}
	return "alice", nil
}

type fakeStatAuthorizer struct{}

func (fakeStatAuthorizer) Authorize(context.Context, string, string, auth.PermissionType) error {
	return nil
}

func (fakeStatAuthorizer) AuthorizeWithMetadata(context.Context, string, string, *metadata.Metadata, auth.PermissionType) error {
	return nil
}

type fakeStatMetadataStore struct {
	metadata.Store
	entries  map[string]*metadata.Metadata
	children map[string][]*metadata.Metadata
}

func (s *fakeStatMetadataStore) Get(_ context.Context, path string) (*metadata.Metadata, error) {
	md, ok := s.entries[path]
	if !ok {
		return nil, metadata.ErrNotFound
	}
	return md, nil
}

func (s *fakeStatMetadataStore) ListChildren(_ context.Context, parentPath string) ([]*metadata.Metadata, error) {
	return s.children[parentPath], nil
}

type fakeStatLockInspector struct {
	held []locks.HeldLock
}

func (f fakeStatLockInspector) Snapshot(context.Context) ([]locks.HeldLock, error) {
	return f.held, nil
}

func (f fakeStatLockInspector) Stats() locks.LockStats {
	return locks.LockStats{}
}

func (f fakeStatLockInspector) ForceRelease(context.Context, string) (bool, error) {
	return false, nil
}

func statTestMetadata(path, name, entryType string, size int64, etag string) *metadata.Metadata {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return &metadata.Metadata{
		Path:        path,
		Name:        name,
		Type:        entryType,
		Size:        size,
		Mode:        "0644",
		Owner:       "alice",
		BackendType: "localfs",
		ETag:        etag,
		MTime:       now,
		ATime:       now,
		CTime:       now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func newStatTestRouter(inspector locks.Inspector) http.Handler {
	root := statTestMetadata("/docs", "docs", "directory", 0, "")
	file := statTestMetadata("/docs/a.txt", "a.txt", "file", 5, "abc")
	hidden := statTestMetadata("/docs/.hidden", ".hidden", "file", 1, "def")
	sub := statTestMetadata("/docs/sub", "sub", "directory", 0, "")
	store := &fakeStatMetadataStore{
		entries: map[string]*metadata.Metadata{
			"/docs":         root,
			"/docs/a.txt":   file,
			"/docs/.hidden": hidden,
			"/docs/sub":     sub,
		},
		children: map[string][]*metadata.Metadata{
			"/docs": {file, hidden, sub},
		},
	}
	engine := core.NewEngine(store, nil, nil, nil, nil, nil, "node-1", nil, false, "", false, zap.NewNop())

	router := chi.NewRouter()
	router.Use(middleware.V1AuthMiddleware(fakeStatAuthenticator{}, nil, nil, zap.NewNop(), nil))
	router.Get("/v1/stat/*", V1Stat(engine, fakeStatAuthorizer{}, inspector))
	router.Get("/v1/directories/*", V1ListDirectoryWithConfig(engine, fakeStatAuthorizer{}, &config.ServerConfig{MetadataOpTimeout: time.Second}, zap.NewNop()))
	return router
}

func serveStatTest(handler http.Handler, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Authorization", statTestToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestV1StatReturnsFileFields(t *testing.T) {
	inspector := fakeStatLockInspector{held: []locks.HeldLock{{Key: "path:/docs/a.txt"}}}
	rec := serveStatTest(newStatTestRouter(inspector), "/v1/stat/docs/a.txt")
	if http.StatusOK != rec.Code {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode stat response: %v", err)
	}
	if "file" != body["type"] {
		t.Errorf("type = %v, want file", body["type"])
	}
	if "\"abc\"" != body["etag"] {
		t.Errorf("etag = %v, want quoted etag", body["etag"])
	}
	if true != body["locked"] {
		t.Errorf("locked = %v, want true", body["locked"])
	}
	if true != body["is_local"] {
		t.Errorf("is_local = %v, want true", body["is_local"])
	}
	if nil != body["children_count"] {
		t.Errorf("children_count = %v, want null for a file", body["children_count"])
	}
	if "2026-01-02T03:04:05Z" != body["mtime"] {
		t.Errorf("mtime = %v, want RFC 3339 UTC", body["mtime"])
	}
}

func TestV1StatMissingPathReturnsNotFound(t *testing.T) {
	rec := serveStatTest(newStatTestRouter(fakeStatLockInspector{}), "/v1/stat/docs/missing.txt")
	if http.StatusNotFound != rec.Code {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestV1ListDirectoryPagedFoldersFirstWithCursor(t *testing.T) {
	router := newStatTestRouter(fakeStatLockInspector{})

	first := serveStatTest(router, "/v1/directories/docs/?limit=2")
	if http.StatusOK != first.Code {
		t.Fatalf("first page status = %d, want 200: %s", first.Code, first.Body.String())
	}
	var firstPage DirectoryPageResponse
	if err := json.Unmarshal(first.Body.Bytes(), &firstPage); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	if 3 != firstPage.Total || 2 != firstPage.Count || firstPage.NextCursor == nil {
		t.Fatalf("first page total=%d count=%d next=%v", firstPage.Total, firstPage.Count, firstPage.NextCursor)
	}
	if "sub" != firstPage.Items[0].Name || ".hidden" != firstPage.Items[1].Name {
		t.Errorf("first page names = %q, %q; want sub, .hidden", firstPage.Items[0].Name, firstPage.Items[1].Name)
	}

	second := serveStatTest(router, "/v1/directories/docs/?limit=2&cursor="+*firstPage.NextCursor)
	var secondPage DirectoryPageResponse
	if err := json.Unmarshal(second.Body.Bytes(), &secondPage); err != nil {
		t.Fatalf("decode second page: %v", err)
	}
	if 1 != secondPage.Count || secondPage.NextCursor != nil {
		t.Fatalf("second page count=%d next=%v, want 1 item and no cursor", secondPage.Count, secondPage.NextCursor)
	}
	if "a.txt" != secondPage.Items[0].Name || "\"abc\"" != secondPage.Items[0].ETag {
		t.Errorf("second page item = %+v, want a.txt with quoted etag", secondPage.Items[0])
	}
}

func TestV1ListDirectoryPagedFiltersHiddenAndType(t *testing.T) {
	rec := serveStatTest(newStatTestRouter(fakeStatLockInspector{}), "/v1/directories/docs/?limit=10&include_hidden=false&type=file")
	var page DirectoryPageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	if 1 != page.Total || 1 != len(page.Items) || "a.txt" != page.Items[0].Name {
		t.Errorf("filtered page total=%d items=%+v, want only a.txt", page.Total, page.Items)
	}
}

func TestV1ListDirectoryPagedRejectsInvalidParameters(t *testing.T) {
	router := newStatTestRouter(fakeStatLockInspector{})
	for _, target := range []string{
		"/v1/directories/docs/?limit=0",
		"/v1/directories/docs/?limit=5&sort=bogus",
	} {
		t.Run(target, func(t *testing.T) {
			rec := serveStatTest(router, target)
			if http.StatusBadRequest != rec.Code {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestSendErrorResponseMapsStatusCodes(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		err         error
		wantCode    string
		wantMessage string
	}{
		{"not found keeps message", http.StatusNotFound, errors.New("missing thing"), "NOT_FOUND", "missing thing"},
		{"conflict", http.StatusConflict, errors.New("busy"), "CONFLICT", "busy"},
		{"precondition", http.StatusPreconditionFailed, errors.New("etag changed"), "PRECONDITION_FAILED", "etag changed"},
		{"payload too large", http.StatusRequestEntityTooLarge, errors.New("too big"), "PAYLOAD_TOO_LARGE", "too big"},
		{"range", http.StatusRequestedRangeNotSatisfiable, errors.New("bad range"), "RANGE_NOT_SATISFIABLE", "bad range"},
		{"rate limit", http.StatusTooManyRequests, errors.New("slow down"), "RATE_LIMIT_EXCEEDED", "slow down"},
		{"server error is generic", http.StatusInternalServerError, errors.New("db password leaked"), "INTERNAL_ERROR", "an internal error occurred"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			SendErrorResponse(rec, zap.NewNop(), tc.err, tc.status)
			if tc.status != rec.Code {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
			var body ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if tc.wantCode != body.Code || tc.wantMessage != body.Message {
				t.Errorf("body = %+v, want code %q message %q", body, tc.wantCode, tc.wantMessage)
			}
		})
	}
}
