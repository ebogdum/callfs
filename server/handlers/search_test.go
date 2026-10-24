package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/auth"
	"github.com/ebogdum/callfs/core"
	"github.com/ebogdum/callfs/metadata"
	"github.com/ebogdum/callfs/server/middleware"
)

var searchTestTokens = map[string]string{
	"Bearer root-key":  "root",
	"Bearer alice-key": "alice",
	"Bearer bob-key":   "bob",
}

type fakeSearchAuthenticator struct{}

func (fakeSearchAuthenticator) Authenticate(_ context.Context, token string) (string, error) {
	userID, ok := searchTestTokens[token]
	if !ok {
		return "", auth.ErrAuthenticationFailed
	}
	return userID, nil
}

type fakeSearchStore struct {
	metadata.Store
	ordered []*metadata.Metadata
	byPath  map[string]*metadata.Metadata
}

func (s *fakeSearchStore) Get(_ context.Context, path string) (*metadata.Metadata, error) {
	md, ok := s.byPath[path]
	if !ok {
		return nil, metadata.ErrNotFound
	}
	return md, nil
}

func (s *fakeSearchStore) ListDescendants(_ context.Context, dirPath string, maxDepth, limit int) ([]*metadata.Metadata, error) {
	prefix := dirPath + "/"
	if "/" == dirPath {
		prefix = "/"
	}
	out := make([]*metadata.Metadata, 0)
	for _, md := range s.ordered {
		if md.Path == dirPath || !strings.HasPrefix(md.Path, prefix) {
			continue
		}
		if 0 < maxDepth && maxDepth < len(relativeSegments(dirPath, md.Path)) {
			continue
		}
		if 0 < limit && limit <= len(out) {
			break
		}
		out = append(out, md)
	}
	return out, nil
}

func searchTestEntry(path, name, entryType, owner string, size int64) *metadata.Metadata {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return &metadata.Metadata{
		Path:        path,
		Name:        name,
		Type:        entryType,
		Owner:       owner,
		Size:        size,
		Mode:        "0644",
		BackendType: "localfs",
		MTime:       now,
		ATime:       now,
		CTime:       now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func newSearchTestRouter() http.Handler {
	entries := []*metadata.Metadata{
		searchTestEntry("/", "", "directory", "root", 0),
		searchTestEntry("/docs", "docs", "directory", "alice", 0),
		searchTestEntry("/docs/a.txt", "a.txt", "file", "alice", 5),
		searchTestEntry("/docs/big.log", "big.log", "file", "alice", 2048),
		searchTestEntry("/docs/sub", "sub", "directory", "alice", 0),
		searchTestEntry("/docs/sub/c.txt", "c.txt", "file", "bob", 10),
	}
	store := &fakeSearchStore{ordered: entries, byPath: make(map[string]*metadata.Metadata)}
	for _, md := range entries {
		store.byPath[md.Path] = md
	}
	engine := core.NewEngine(store, nil, nil, nil, nil, nil, "node-1", nil, false, "", false, zap.NewNop())
	authorizer := auth.NewUnixAuthorizer(store, nil)

	router := chi.NewRouter()
	router.Use(middleware.V1AuthMiddleware(fakeSearchAuthenticator{}, nil, nil, zap.NewNop(), nil))
	router.Get("/v1/search", V1Search(engine, authorizer))
	router.Get("/v1/usage", V1Usage(engine, NewUsageCache(nil)))
	router.Get("/v1/permissions/check", V1PermissionCheck(authorizer, engine, auth.NewAdminSet(nil)))
	return router
}

func serveSearchTest(handler http.Handler, target, authorization string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Authorization", authorization)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestV1SearchFiltersByGlobAndPages(t *testing.T) {
	router := newSearchTestRouter()

	rec := serveSearchTest(router, "/v1/search?path=/docs&q=*.txt&facets=true", "Bearer alice-key")
	if http.StatusOK != rec.Code {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if 2 != len(body.Items) {
		t.Fatalf("items = %d, want 2", len(body.Items))
	}
	if 4 != body.Scanned {
		t.Fatalf("scanned = %d, want 4", body.Scanned)
	}
	if 1 != body.Facets["owner"]["bob"] {
		t.Fatalf("owner facet bob = %d, want 1", body.Facets["owner"]["bob"])
	}

	rec = serveSearchTest(router, "/v1/search?path=/docs&q=*.txt&limit=1", "Bearer alice-key")
	if http.StatusOK != rec.Code {
		t.Fatalf("paged status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode paged search response: %v", err)
	}
	if 1 != len(body.Items) || "/docs/a.txt" != body.NextCursor {
		t.Fatalf("paged items = %d next_cursor = %q, want 1 and /docs/a.txt", len(body.Items), body.NextCursor)
	}

	rec = serveSearchTest(router, "/v1/search?path=/docs&limit=0", "Bearer alice-key")
	if http.StatusBadRequest != rec.Code {
		t.Fatalf("invalid limit status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestV1UsageAggregatesChildrenAndHistograms(t *testing.T) {
	rec := serveSearchTest(newSearchTestRouter(), "/v1/usage?path=/docs&depth=1&histograms=true", "Bearer alice-key")
	if http.StatusOK != rec.Code {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var usage Usage
	if err := json.Unmarshal(rec.Body.Bytes(), &usage); err != nil {
		t.Fatalf("decode usage response: %v", err)
	}
	if 3 != usage.Files || 1 != usage.Directories || 2063 != usage.Bytes {
		t.Fatalf("files=%d directories=%d bytes=%d, want 3 1 2063", usage.Files, usage.Directories, usage.Bytes)
	}
	if 1 != len(usage.Children) || "/docs/sub" != usage.Children[0].Path || 1 != usage.Children[0].Files || 10 != usage.Children[0].Bytes {
		t.Fatalf("children = %+v, want one /docs/sub with 1 file and 10 bytes", usage.Children)
	}
	if 8 != len(usage.SizeHistogram) || 6 != len(usage.AgeHistogram) {
		t.Fatalf("histogram lengths = %d, %d, want 8, 6", len(usage.SizeHistogram), len(usage.AgeHistogram))
	}

	rec = serveSearchTest(newSearchTestRouter(), "/v1/usage?path=/docs/a.txt", "Bearer alice-key")
	if http.StatusBadRequest != rec.Code {
		t.Fatalf("file path status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestV1PermissionCheckReportsDecisionsAndAdminOverride(t *testing.T) {
	router := newSearchTestRouter()

	rec := serveSearchTest(router, "/v1/permissions/check?path=/docs/a.txt&perm=delete", "Bearer bob-key")
	if http.StatusOK != rec.Code {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var decision PermissionCheckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &decision); err != nil {
		t.Fatalf("decode permission response: %v", err)
	}
	if decision.Allowed || "not_owner" != decision.Reason || "alice" != decision.Owner || "file" != decision.Type {
		t.Fatalf("decision = %+v, want denied not_owner owned by alice", decision)
	}

	rec = serveSearchTest(router, "/v1/permissions/check?path=/docs/a.txt&perm=read&user=alice", "Bearer bob-key")
	if http.StatusForbidden != rec.Code || !strings.Contains(rec.Body.String(), "ADMIN_REQUIRED") {
		t.Fatalf("non-admin override status = %d, want 403 ADMIN_REQUIRED: %s", rec.Code, rec.Body.String())
	}

	rec = serveSearchTest(router, "/v1/permissions/check?path=/docs/a.txt&perm=read&user=bob", "Bearer root-key")
	if http.StatusOK != rec.Code {
		t.Fatalf("admin override status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &decision); err != nil {
		t.Fatalf("decode admin permission response: %v", err)
	}
	if !decision.Allowed || "bob" != decision.User || "not_owner" != decision.Reason {
		t.Fatalf("admin decision = %+v, want allowed read for bob with reason not_owner", decision)
	}
}
