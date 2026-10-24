package config

import (
	"testing"
	"time"
)

func TestUIAndTrashDefaultsAreValid(t *testing.T) {
	cfg := DefaultAppConfig()
	if err := validateUIConfig(&cfg); nil != err {
		t.Errorf("ui defaults rejected: %v", err)
	}
	if err := validateTrashConfig(&cfg); nil != err {
		t.Errorf("trash defaults rejected: %v", err)
	}
	if err := validateServerConfig(&cfg); nil != err {
		t.Errorf("session rate-limit defaults rejected: %v", err)
	}
}

func TestValidateUIConfigRejects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AppConfig)
	}{
		{"session_ttl below 5m", func(c *AppConfig) { c.UI.SessionTTL = time.Minute }},
		{"session_ttl above 720h", func(c *AppConfig) { c.UI.SessionTTL = 721 * time.Hour }},
		{"idle_timeout below 1m", func(c *AppConfig) { c.UI.IdleTimeout = 30 * time.Second }},
		{"idle_timeout above session_ttl", func(c *AppConfig) {
			c.UI.SessionTTL = 10 * time.Minute
			c.UI.IdleTimeout = 20 * time.Minute
		}},
		{"sample interval below 1s", func(c *AppConfig) { c.UI.MetricsSampleInterval = 500 * time.Millisecond }},
		{"sample interval above 60s", func(c *AppConfig) { c.UI.MetricsSampleInterval = 2 * time.Minute }},
		{"retention below 1h", func(c *AppConfig) { c.UI.MetricsRetention = 30 * time.Minute }},
		{"retention above 168h", func(c *AppConfig) { c.UI.MetricsRetention = 169 * time.Hour }},
		{"too many samples per series", func(c *AppConfig) {
			c.UI.MetricsRetention = 168 * time.Hour
			c.UI.MetricsSampleInterval = time.Second
		}},
		{"log buffer below 100", func(c *AppConfig) { c.UI.LogBufferEntries = 99 }},
		{"audit buffer above 1000000", func(c *AppConfig) { c.UI.AuditBufferEntries = 1000001 }},
		{"event buffer below 100", func(c *AppConfig) { c.UI.EventBufferEntries = 0 }},
		{"trash retention below 1h", func(c *AppConfig) { c.Trash.Retention = 59 * time.Minute }},
		{"trash retention above 8760h", func(c *AppConfig) { c.Trash.Retention = 8761 * time.Hour }},
		{"session_rps zero", func(c *AppConfig) { c.Server.RateLimit.SessionRPS = 0 }},
		{"session_burst zero", func(c *AppConfig) { c.Server.RateLimit.SessionBurst = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultAppConfig()
			tt.mutate(&cfg)
			uiErr := validateUIConfig(&cfg)
			trashErr := validateTrashConfig(&cfg)
			serverErr := validateServerConfig(&cfg)
			if nil == uiErr && nil == trashErr && nil == serverErr {
				t.Errorf("expected a validation error")
			}
		})
	}
}

func TestValidateAdminUsers(t *testing.T) {
	t.Run("root and configured users are accepted", func(t *testing.T) {
		cfg := baseAuthConfig()
		cfg.Auth.APIKeys = []string{"legacy-key-at-least-16-chars"}
		cfg.Auth.APIKeyUsers = map[string]string{"alice": "alice-key-at-least-16-chars"}
		cfg.Auth.AdminUsers = []string{"root", "alice", "api-user-0"}
		if err := validateAdminUsers(cfg); nil != err {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("unknown and out-of-range users are rejected", func(t *testing.T) {
		for _, userID := range []string{"ghost", "api-user-1", "api-user-01", "internal-proxy"} {
			cfg := baseAuthConfig()
			cfg.Auth.APIKeys = []string{"legacy-key-at-least-16-chars"}
			cfg.Auth.AdminUsers = []string{userID}
			if err := validateAdminUsers(cfg); nil == err {
				t.Errorf("admin user %q accepted", userID)
			}
		}
	})
}

func TestUIEnvOverrides(t *testing.T) {
	t.Setenv("CALLFS_AUTH__API_KEY_USERS__ALICE", "alice-key-at-least-16-chars")
	t.Setenv("CALLFS_UI__SESSION_TTL", "2h")
	t.Setenv("CALLFS_TRASH__ENABLED", "false")
	cfg, err := LoadConfigFromFile(writeMinimalConfig(t))
	if nil != err {
		t.Fatalf("load config: %v", err)
	}
	if 2*time.Hour != cfg.UI.SessionTTL {
		t.Errorf("ui.session_ttl = %v, want 2h", cfg.UI.SessionTTL)
	}
	if cfg.Trash.Enabled {
		t.Errorf("trash.enabled = true, want false")
	}
	if time.Hour != cfg.UI.IdleTimeout {
		t.Errorf("ui.idle_timeout = %v, want 1h", cfg.UI.IdleTimeout)
	}
}
