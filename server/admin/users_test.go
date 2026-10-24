package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/jobs"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/telemetry"
)

type usersTestPublisher struct{}

func (usersTestPublisher) Publish(string, any) {}

type usersTestStore struct {
	metadata.Store
	owners []metadata.OwnerStat
	files  []*metadata.Metadata
}

func (s *usersTestStore) OwnerStats(_ context.Context, cursor string, limit int) ([]metadata.OwnerStat, string, error) {
	start := 0
	if "" != cursor {
		parsed, err := strconv.Atoi(cursor)
		if nil != err {
			return nil, "", err
		}
		start = parsed
	}
	end := min(start+limit, len(s.owners))
	next := ""
	if end < len(s.owners) {
		next = strconv.Itoa(end)
	}
	return s.owners[start:end], next, nil
}

func (s *usersTestStore) ListByPrefix(_ context.Context, prefix, startAfter string, limit int) ([]*metadata.Metadata, error) {
	var out []*metadata.Metadata
	for _, md := range s.files {
		if strings.HasPrefix(md.Path, prefix) && md.Path > startAfter && len(out) < limit {
			out = append(out, md)
		}
	}
	return out, nil
}

func (s *usersTestStore) UpdateIfMatch(_ context.Context, md *metadata.Metadata, _ string) error {
	for _, existing := range s.files {
		if existing.Path == md.Path {
			existing.Owner = md.Owner
		}
	}
	return nil
}

func newUsersTestAPI(t *testing.T, store *usersTestStore, keys []string, users map[string]string, admins []string) *API {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("auth: {}\n"), 0o600); nil != err {
		t.Fatal(err)
	}
	hub, err := telemetry.NewHub(telemetry.Config{InstanceID: "test", LogEntries: 10, AuditEntries: 100, EventEntries: 10})
	if nil != err {
		t.Fatal(err)
	}
	cfg := &config.AppConfig{Auth: config.AuthConfig{
		APIKeys:             keys,
		APIKeyUsers:         users,
		AdminUsers:          admins,
		InternalProxySecret: "proxy-secret-value-0001",
	}}
	return &API{deps: Deps{
		Store:         store,
		Config:        cfg,
		ConfigPath:    configPath,
		Authenticator: auth.NewAPIKeyAuthenticator(keys, users, cfg.Auth.InternalProxySecret),
		Admins:        auth.NewAdminSet(admins),
		Telemetry:     hub,
		Jobs:          jobs.NewRunner(usersTestPublisher{}, zap.NewNop()),
		Logger:        zap.NewNop(),
	}}
}

func usersTestRequest(method, target, body, id string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if "" != id {
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", id)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	}
	return req
}

func TestUsersListsSourcesUsageAndOwnership(t *testing.T) {
	store := &usersTestStore{owners: []metadata.OwnerStat{{Owner: "alice", Files: 2, Bytes: 10}}}
	api := newUsersTestAPI(t, store, []string{"positional-key-0001"}, map[string]string{"alice": "alice-key-0001"}, []string{"alice"})
	api.deps.Telemetry.Audit.Add(telemetry.AuditRecord{At: time.Now().UTC(), UserID: "alice"})
	rec := httptest.NewRecorder()

	api.Users(rec, usersTestRequest(http.MethodGet, "/v1/admin/users", "", ""))

	if http.StatusOK != rec.Code {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "alice-key-0001") {
		t.Fatal("response leaks an API key")
	}
	var body usersResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); nil != err {
		t.Fatal(err)
	}
	if 1 != body.PositionalCount {
		t.Fatalf("positional_count = %d, want 1", body.PositionalCount)
	}
	for _, item := range body.Items {
		if "alice" == item.UserID {
			if !item.IsAdmin || 2 != item.OwnedFiles || 10 != item.OwnedBytes || 1 != item.Requests24h || nil == item.LastAuthAt {
				t.Fatalf("alice item = %+v", item)
			}
			return
		}
	}
	t.Fatal("alice missing from users")
}

func TestCreateUserReturnsKeyOnceAndReloads(t *testing.T) {
	api := newUsersTestAPI(t, &usersTestStore{}, nil, nil, nil)
	rec := httptest.NewRecorder()

	api.CreateUser(rec, usersTestRequest(http.MethodPost, "/v1/admin/users", `{"user_id":"bob","is_admin":true}`, ""))

	if http.StatusCreated != rec.Code {
		t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body.String())
	}
	var created userKeyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); nil != err {
		t.Fatal(err)
	}
	userID, err := api.deps.Authenticator.Authenticate(context.Background(), created.APIKey)
	if nil != err || "bob" != userID {
		t.Fatalf("new key authenticates as %q, err %v", userID, err)
	}
	if !api.deps.Admins.IsAdmin("bob") {
		t.Fatal("bob is not admin after is_admin=true")
	}
}

func TestCreateUserRejectsReservedID(t *testing.T) {
	api := newUsersTestAPI(t, &usersTestStore{}, nil, nil, nil)
	rec := httptest.NewRecorder()

	api.CreateUser(rec, usersTestRequest(http.MethodPost, "/v1/admin/users", `{"user_id":"root"}`, ""))

	if http.StatusUnprocessableEntity != rec.Code {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestRotateUserKeyInvalidatesOldKey(t *testing.T) {
	oldKey := "old-key-0000001"
	api := newUsersTestAPI(t, &usersTestStore{}, nil, map[string]string{"alice": oldKey}, nil)
	rec := httptest.NewRecorder()

	api.RotateUserKey(rec, usersTestRequest(http.MethodPost, "/v1/admin/users/alice/rotate", "", "alice"))

	if http.StatusOK != rec.Code {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var rotated userKeyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &rotated); nil != err {
		t.Fatal(err)
	}
	if _, err := api.deps.Authenticator.Authenticate(context.Background(), oldKey); nil == err {
		t.Fatal("old key still authenticates")
	}
	if userID, err := api.deps.Authenticator.Authenticate(context.Background(), rotated.APIKey); nil != err || "alice" != userID {
		t.Fatalf("rotated key authenticates as %q, err %v", userID, err)
	}
}

func TestUpdateUserRefusesLastAdmin(t *testing.T) {
	api := newUsersTestAPI(t, &usersTestStore{}, nil, map[string]string{"alice": "alice-key-0001"}, []string{"alice"})
	rec := httptest.NewRecorder()

	api.UpdateUser(rec, usersTestRequest(http.MethodPatch, "/v1/admin/users/alice", `{"is_admin":false}`, "alice"))

	if http.StatusConflict != rec.Code {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if !api.deps.Admins.IsAdmin("alice") {
		t.Fatal("alice lost admin despite refusal")
	}
}

func TestDeleteUserRefusesLastAdmin(t *testing.T) {
	api := newUsersTestAPI(t, &usersTestStore{}, nil, map[string]string{"alice": "alice-key-0001"}, []string{"alice"})
	rec := httptest.NewRecorder()

	api.DeleteUser(rec, usersTestRequest(http.MethodDelete, "/v1/admin/users/alice", "", "alice"))

	if http.StatusConflict != rec.Code {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestDeleteUserRemovesKeyAndRefusesNumberedKey(t *testing.T) {
	api := newUsersTestAPI(t, &usersTestStore{}, []string{"numbered-key-0001"}, map[string]string{"bob": "bob-key-000001", "carol": "carol-key-000001"}, []string{"bob", "carol"})
	numbered := ""
	for _, info := range api.deps.Authenticator.Users() {
		if auth.SourceAPIKeys == info.Source {
			numbered = info.UserID
		}
	}
	refused := httptest.NewRecorder()
	api.DeleteUser(refused, usersTestRequest(http.MethodDelete, "/v1/admin/users/"+numbered, "", numbered))
	if http.StatusConflict != refused.Code {
		t.Fatalf("numbered delete status = %d, want 409", refused.Code)
	}

	rec := httptest.NewRecorder()
	api.DeleteUser(rec, usersTestRequest(http.MethodDelete, "/v1/admin/users/bob", "", "bob"))
	if http.StatusNoContent != rec.Code {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if _, err := api.deps.Authenticator.Authenticate(context.Background(), "bob-key-000001"); nil == err {
		t.Fatal("deleted user key still authenticates")
	}
}

func TestMigratePositionalRejectsBadIndex(t *testing.T) {
	api := newUsersTestAPI(t, &usersTestStore{}, []string{"numbered-key-0001"}, nil, nil)
	rec := httptest.NewRecorder()

	api.MigratePositional(rec, usersTestRequest(http.MethodPost, "/v1/admin/users/migrate-positional", `{"mapping":[{"index":3,"user_id":"carol"}]}`, ""))

	if http.StatusUnprocessableEntity != rec.Code {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestOwnersPaginatesWithCursor(t *testing.T) {
	store := &usersTestStore{owners: []metadata.OwnerStat{
		{Owner: "", Files: 1, Bytes: 1},
		{Owner: "alice", Files: 2, Bytes: 2},
		{Owner: "bob", Files: 3, Bytes: 3},
	}}
	api := newUsersTestAPI(t, store, nil, nil, nil)

	first := httptest.NewRecorder()
	api.Owners(first, usersTestRequest(http.MethodGet, "/v1/admin/owners?limit=2", "", ""))
	var page ownersResponse
	if err := json.Unmarshal(first.Body.Bytes(), &page); nil != err {
		t.Fatal(err)
	}
	if 2 != len(page.Items) || nil == page.NextCursor || "2" != *page.NextCursor {
		t.Fatalf("first page = %+v", page)
	}

	second := httptest.NewRecorder()
	api.Owners(second, usersTestRequest(http.MethodGet, "/v1/admin/owners?limit=2&cursor=2", "", ""))
	var last ownersResponse
	if err := json.Unmarshal(second.Body.Bytes(), &last); nil != err {
		t.Fatal(err)
	}
	if 1 != len(last.Items) || "bob" != last.Items[0].Owner || nil != last.NextCursor {
		t.Fatalf("last page = %+v", last)
	}
}

func TestReassignOwnerRequiresDifferentTarget(t *testing.T) {
	api := newUsersTestAPI(t, &usersTestStore{}, nil, nil, nil)
	rec := httptest.NewRecorder()

	api.ReassignOwner(rec, usersTestRequest(http.MethodPost, "/v1/admin/owners/reassign", `{"from":"alice","to":"alice"}`, ""))

	if http.StatusUnprocessableEntity != rec.Code {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestOwnerReassignMovesMatchingFiles(t *testing.T) {
	store := &usersTestStore{files: []*metadata.Metadata{
		{Path: "/a/one.txt", Owner: "alice"},
		{Path: "/b/two.txt", Owner: "bob"},
	}}
	api := newUsersTestAPI(t, store, nil, nil, nil)

	moved, err := api.reassignOwners(context.Background(), func(int) {}, "/", map[string]string{"alice": "carol"})

	if nil != err || 1 != moved {
		t.Fatalf("moved = %d, err %v", moved, err)
	}
	if "carol" != store.files[0].Owner || "bob" != store.files[1].Owner {
		t.Fatalf("owners after reassign: %q, %q", store.files[0].Owner, store.files[1].Owner)
	}
}

func TestAuthFailuresCountsLastDay(t *testing.T) {
	api := newUsersTestAPI(t, &usersTestStore{}, nil, nil, nil)
	now := time.Now().UTC()
	api.deps.Telemetry.AuthFailures.Record(telemetry.AuthFailure{At: now, Client: "10.0.0.5", Reason: "wrong_key", Path: "/v1/me"})
	api.deps.Telemetry.AuthFailures.Record(telemetry.AuthFailure{At: now.Add(-48 * time.Hour), Client: "10.0.0.6", Reason: "missing_key", Path: "/v1/me"})
	rec := httptest.NewRecorder()

	api.AuthFailures(rec, usersTestRequest(http.MethodGet, "/v1/admin/auth/failures", "", ""))

	if http.StatusOK != rec.Code {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body authFailuresResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); nil != err {
		t.Fatal(err)
	}
	if 1 != body.Total24h || 1 != len(body.Items) || "wrong_key" != body.Items[0].Reason {
		t.Fatalf("auth failures = %+v", body)
	}
}
