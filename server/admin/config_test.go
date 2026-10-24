package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/config"
)

const configTestYAML = `server:
  protocol: http
auth:
  api_key_users:
    alice: k3y-alpha-0123456789abcdefghijklmnopqrstuvwxyz
  internal_proxy_secret: internal-secret-0123456789abcdefghijklmnop
  single_use_link_secret: link-secret-0123456789abcdefghijklmnopqrs
metadata_store:
  type: sqlite
instance_discovery:
  instance_id: test-1
`

type configTestKey struct {
	Path  string `json:"path"`
	Value any    `json:"value"`
	Set   bool   `json:"set"`
}

type configTestBody struct {
	Revision string          `json:"revision"`
	Keys     []configTestKey `json:"keys"`
}

func newConfigTestAPI(t *testing.T, sources config.SourceMap) (*API, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(configTestYAML), 0o600); nil != err {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.LoadConfigFromFile(path)
	if nil != err {
		t.Fatalf("load config: %v", err)
	}
	api := &API{deps: Deps{Config: &cfg, ConfigPath: path, ConfigSources: sources, Logger: zap.NewNop()}}
	return api, path
}

func configTestRequest(t *testing.T, method, target string, body any) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	payload, err := json.Marshal(body)
	if nil != err {
		t.Fatalf("marshal body: %v", err)
	}
	return httptest.NewRecorder(), httptest.NewRequest(method, target, bytes.NewReader(payload))
}

func configTestRevision(t *testing.T, api *API) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	api.GetConfig(recorder, httptest.NewRequest(http.MethodGet, "/v1/admin/config", nil))
	var current configTestBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &current); nil != err {
		t.Fatalf("decode: %v", err)
	}
	return current.Revision
}

func TestConfigGetMasksSecrets(t *testing.T) {
	api, _ := newConfigTestAPI(t, nil)
	recorder := httptest.NewRecorder()
	api.GetConfig(recorder, httptest.NewRequest(http.MethodGet, "/v1/admin/config", nil))

	if http.StatusOK != recorder.Code {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body configTestBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); nil != err {
		t.Fatalf("decode: %v", err)
	}
	if "" == body.Revision {
		t.Fatal("revision is empty")
	}
	for _, key := range body.Keys {
		if "auth.internal_proxy_secret" == key.Path {
			if nil != key.Value || !key.Set {
				t.Fatalf("secret exposed or unset: value=%v set=%v", key.Value, key.Set)
			}
			return
		}
	}
	t.Fatal("auth.internal_proxy_secret missing from config view")
}

func TestConfigSaveWritesBackupAndNewRevision(t *testing.T) {
	api, path := newConfigTestAPI(t, nil)
	revision := configTestRevision(t, api)

	recorder, request := configTestRequest(t, http.MethodPut, "/v1/admin/config", map[string]any{
		"changes":           map[string]any{"server.read_timeout": "45s"},
		"expected_revision": revision,
	})
	api.SaveConfig(recorder, request)

	if http.StatusOK != recorder.Code {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var saved struct {
		BackupPath string `json:"backup_path"`
		Revision   string `json:"revision"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &saved); nil != err {
		t.Fatalf("decode: %v", err)
	}
	if _, err := os.Stat(saved.BackupPath); nil != err {
		t.Fatalf("backup missing: %v", err)
	}
	data, err := os.ReadFile(path)
	if nil != err {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), "45s") {
		t.Fatalf("saved file lacks new value: %s", data)
	}
	if revision == saved.Revision {
		t.Fatal("revision did not change after save")
	}
}

func TestConfigSaveRejectsStaleRevision(t *testing.T) {
	api, _ := newConfigTestAPI(t, nil)
	recorder, request := configTestRequest(t, http.MethodPut, "/v1/admin/config", map[string]any{
		"changes":           map[string]any{"server.read_timeout": "45s"},
		"expected_revision": "stale",
	})
	api.SaveConfig(recorder, request)

	if http.StatusConflict != recorder.Code {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
}

func TestConfigSaveRejectsEnvironmentSourcedKey(t *testing.T) {
	sources := config.SourceMap{"server.read_timeout": config.SourceEnv}
	api, _ := newConfigTestAPI(t, sources)
	recorder, request := configTestRequest(t, http.MethodPut, "/v1/admin/config", map[string]any{
		"changes":           map[string]any{"server.read_timeout": "45s"},
		"expected_revision": configTestRevision(t, api),
	})
	api.SaveConfig(recorder, request)

	if http.StatusUnprocessableEntity != recorder.Code {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}
}

func TestConfigValidateReportsUnknownKey(t *testing.T) {
	api, _ := newConfigTestAPI(t, nil)
	recorder, request := configTestRequest(t, http.MethodPost, "/v1/admin/config/validate", map[string]any{
		"changes": map[string]any{"server.nope": 1},
	})
	api.ValidateConfig(recorder, request)

	var body struct {
		Valid  bool `json:"valid"`
		Errors []struct {
			Path string `json:"path"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); nil != err {
		t.Fatalf("decode: %v", err)
	}
	if body.Valid || 0 == len(body.Errors) || "server.nope" != body.Errors[0].Path {
		t.Fatalf("unexpected validation result: %s", recorder.Body.String())
	}
}

func TestConfigPatchRuntimeRejectsRestartKey(t *testing.T) {
	api, _ := newConfigTestAPI(t, nil)
	recorder, request := configTestRequest(t, http.MethodPatch, "/v1/admin/config/runtime", map[string]any{
		"server.listen_addr": ":9000",
	})
	api.PatchRuntimeConfig(recorder, request)

	if http.StatusUnprocessableEntity != recorder.Code {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}
}

func TestConfigExportRedactsSecrets(t *testing.T) {
	api, _ := newConfigTestAPI(t, nil)
	recorder := httptest.NewRecorder()
	api.ExportConfig(recorder, httptest.NewRequest(http.MethodGet, "/v1/admin/config/export?format=yaml&effective=true", nil))

	if http.StatusOK != recorder.Code {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "<set>") || strings.Contains(body, "internal-secret-0123456789abcdefghijklmnop") {
		t.Fatalf("export does not redact secrets: %s", body)
	}
}
