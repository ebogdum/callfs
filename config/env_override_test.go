package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeMinimalConfig writes a config file that satisfies every non-auth required
// field, so a test can vary only the auth settings.
func writeMinimalConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := `
server:
  listen_addr: ":8443"
  protocol: "http"
  external_url: "localhost:8443"
auth:
  internal_proxy_secret: "internal-secret-at-least-16"
  single_use_link_secret: "link-secret-at-least-16-ch"
metadata_store:
  type: "sqlite"
  dsn: "file:test.db"
instance_discovery:
  instance_id: "test-instance"
dlm:
  type: "local"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// TestAPIKeyUsersFromEnv pins the documented environment-variable form for the
// api_key_users map. The env provider maps "__" to the koanf key delimiter, so a
// map entry is addressed as CALLFS_AUTH__API_KEY_USERS__<ID>. This test exists so
// docs_markdown/02-configuration.md cannot drift from what actually loads.
func TestAPIKeyUsersFromEnv(t *testing.T) {
	configPath := writeMinimalConfig(t)
	t.Setenv("CALLFS_AUTH__API_KEY_USERS__ALICE", "alice-key-at-least-16-chars")

	cfg, err := LoadConfigFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFromFile: %v", err)
	}

	// Env keys are lowercased by the provider, so the user ID arrives as "alice".
	got, ok := cfg.Auth.APIKeyUsers["alice"]
	if !ok {
		t.Fatalf("api_key_users from env missing; got map %v", cfg.Auth.APIKeyUsers)
	}
	if got != "alice-key-at-least-16-chars" {
		t.Errorf("api_key_users[alice] = %q, want the env-supplied key", got)
	}
}

// TestAPIKeyUsersFromFile confirms the YAML form loads and validates end to end,
// including that a config with no api_keys list at all is accepted.
func TestAPIKeyUsersFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := `
server:
  listen_addr: ":8443"
  protocol: "http"
  external_url: "localhost:8443"
auth:
  api_key_users:
    alice: "alice-key-at-least-16-chars"
    bob: "bob-key-at-least-16-chars"
  internal_proxy_secret: "internal-secret-at-least-16"
  single_use_link_secret: "link-secret-at-least-16-ch"
metadata_store:
  type: "sqlite"
  dsn: "file:test.db"
instance_discovery:
  instance_id: "test-instance"
dlm:
  type: "local"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfigFromFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFromFile: %v", err)
	}
	if len(cfg.Auth.APIKeys) != 0 {
		t.Errorf("api_keys = %v, want empty when only api_key_users is configured", cfg.Auth.APIKeys)
	}
	if cfg.Auth.APIKeyUsers["alice"] != "alice-key-at-least-16-chars" {
		t.Errorf("api_key_users[alice] = %q", cfg.Auth.APIKeyUsers["alice"])
	}
	if cfg.Auth.APIKeyUsers["bob"] != "bob-key-at-least-16-chars" {
		t.Errorf("api_key_users[bob] = %q", cfg.Auth.APIKeyUsers["bob"])
	}
}

// TestLegacyAPIKeysStillLoad confirms the positional form is untouched, so
// existing deployments keep working exactly as before.
func TestLegacyAPIKeysStillLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := `
server:
  listen_addr: ":8443"
  protocol: "http"
  external_url: "localhost:8443"
auth:
  api_keys:
    - "legacy-key-at-least-16-chars"
    - "second-key-at-least-16-chars"
  internal_proxy_secret: "internal-secret-at-least-16"
  single_use_link_secret: "link-secret-at-least-16-ch"
metadata_store:
  type: "sqlite"
  dsn: "file:test.db"
instance_discovery:
  instance_id: "test-instance"
dlm:
  type: "local"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfigFromFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFromFile: %v", err)
	}
	if len(cfg.Auth.APIKeys) != 2 {
		t.Fatalf("api_keys = %v, want 2 entries", cfg.Auth.APIKeys)
	}
	if cfg.Auth.APIKeys[0] != "legacy-key-at-least-16-chars" {
		t.Errorf("api_keys[0] = %q, order must be preserved", cfg.Auth.APIKeys[0])
	}
}
