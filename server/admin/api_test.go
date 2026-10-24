package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/auth"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/findings"
)

func TestMountRejectsAnonymousRequests(t *testing.T) {
	api := NewAPI(Deps{Admins: auth.NewAdminSet(nil), Logger: zap.NewNop()})
	router := chi.NewRouter()
	router.Route("/v1", func(v1 chi.Router) {
		Mount(v1, api)
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/status", nil))
	if http.StatusUnauthorized != rec.Code {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHealthWithoutCheckerIsUnavailable(t *testing.T) {
	api := NewAPI(Deps{Logger: zap.NewNop()})
	rec := httptest.NewRecorder()
	api.Health(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/health", nil))
	if http.StatusServiceUnavailable != rec.Code {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestRestartRefusedUnlessEnabled(t *testing.T) {
	api := NewAPI(Deps{
		Config:  &config.AppConfig{},
		Restart: func(string) error { return nil },
		Logger:  zap.NewNop(),
	})
	rec := httptest.NewRecorder()
	api.Restart(rec, httptest.NewRequest(http.MethodPost, "/v1/admin/restart", strings.NewReader(`{"reason":"test"}`)))
	if http.StatusForbidden != rec.Code {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); nil != err {
		t.Fatalf("decode body: %v", err)
	}
	if "FEATURE_DISABLED" != body.Code {
		t.Fatalf("code = %q, want FEATURE_DISABLED", body.Code)
	}
}

func TestFindingsRejectsUnknownArea(t *testing.T) {
	cfg := config.DefaultAppConfig()
	engine := findings.NewEngine(
		func() *config.AppConfig { return &cfg },
		func(context.Context) (findings.Facts, error) { return findings.Facts{}, nil },
		nil,
		nil,
	)
	api := NewAPI(Deps{Config: &cfg, Findings: engine, Logger: zap.NewNop()})
	rec := httptest.NewRecorder()
	api.Findings(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/findings?area=bogus", nil))
	if http.StatusUnprocessableEntity != rec.Code {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestMigrationsListsEmbeddedVersions(t *testing.T) {
	cfg := config.AppConfig{MetadataStore: config.MetadataStoreConfig{Type: "sqlite"}}
	api := NewAPI(Deps{Config: &cfg, Logger: zap.NewNop()})
	rec := httptest.NewRecorder()
	api.Migrations(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/migrations", nil))
	if http.StatusOK != rec.Code {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body struct {
		Driver         string `json:"driver"`
		CurrentVersion *int64 `json:"current_version"`
		Available      []struct {
			Version int64  `json:"version"`
			Name    string `json:"name"`
		} `json:"available"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); nil != err {
		t.Fatalf("decode body: %v", err)
	}
	if "none" != body.Driver || nil != body.CurrentVersion {
		t.Fatalf("driver = %q, current_version = %v", body.Driver, body.CurrentVersion)
	}
	if 1 > len(body.Available) || 1 != body.Available[0].Version || "initial_schema" != body.Available[0].Name {
		t.Fatalf("available = %+v, want first 1 initial_schema", body.Available)
	}
}
