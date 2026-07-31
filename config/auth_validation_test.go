package config

import "testing"

func baseAuthConfig() *AppConfig {
	return &AppConfig{
		Auth: AuthConfig{
			InternalProxySecret: "internal-secret-at-least-16",
			SingleUseLinkSecret: "link-secret-at-least-16-ch",
		},
	}
}

func TestValidateAuthConfigAcceptsEitherForm(t *testing.T) {
	t.Run("positional only", func(t *testing.T) {
		cfg := baseAuthConfig()
		cfg.Auth.APIKeys = []string{"legacy-key-at-least-16-chars"}
		if err := validateAuthConfig(cfg); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("explicit only", func(t *testing.T) {
		cfg := baseAuthConfig()
		cfg.Auth.APIKeyUsers = map[string]string{"alice": "alice-key-at-least-16-chars"}
		if err := validateAuthConfig(cfg); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("both forms together", func(t *testing.T) {
		cfg := baseAuthConfig()
		cfg.Auth.APIKeys = []string{"legacy-key-at-least-16-chars"}
		cfg.Auth.APIKeyUsers = map[string]string{"alice": "alice-key-at-least-16-chars"}
		if err := validateAuthConfig(cfg); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestValidateAuthConfigRejects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AppConfig)
	}{
		{"no keys at all", func(c *AppConfig) {}},
		{"short positional key", func(c *AppConfig) {
			c.Auth.APIKeys = []string{"short"}
		}},
		{"placeholder positional key", func(c *AppConfig) {
			c.Auth.APIKeys = []string{"default-api-key"}
		}},
		{"short explicit key", func(c *AppConfig) {
			c.Auth.APIKeyUsers = map[string]string{"alice": "short"}
		}},
		{"placeholder explicit key", func(c *AppConfig) {
			c.Auth.APIKeyUsers = map[string]string{"alice": "default-api-key"}
		}},
		{"empty user id", func(c *AppConfig) {
			c.Auth.APIKeyUsers = map[string]string{"": "alice-key-at-least-16-chars"}
		}},
		{"blank user id", func(c *AppConfig) {
			c.Auth.APIKeyUsers = map[string]string{"   ": "alice-key-at-least-16-chars"}
		}},
		{"reserved root", func(c *AppConfig) {
			c.Auth.APIKeyUsers = map[string]string{"root": "root-key-at-least-16-chars"}
		}},
		{"reserved internal-proxy", func(c *AppConfig) {
			c.Auth.APIKeyUsers = map[string]string{"internal-proxy": "proxy-key-at-least-16-chars"}
		}},
		{"reserved positional pattern", func(c *AppConfig) {
			c.Auth.APIKeyUsers = map[string]string{"api-user-0": "some-key-at-least-16-chars"}
		}},
		{"duplicate key across forms", func(c *AppConfig) {
			c.Auth.APIKeys = []string{"shared-key-at-least-16-chars"}
			c.Auth.APIKeyUsers = map[string]string{"alice": "shared-key-at-least-16-chars"}
		}},
		{"duplicate key within positional list", func(c *AppConfig) {
			c.Auth.APIKeys = []string{"same-key-at-least-16-chars", "same-key-at-least-16-chars"}
		}},
		{"api key reuses internal proxy secret", func(c *AppConfig) {
			c.Auth.APIKeys = []string{"internal-secret-at-least-16"}
		}},
		{"missing internal proxy secret", func(c *AppConfig) {
			c.Auth.APIKeys = []string{"legacy-key-at-least-16-chars"}
			c.Auth.InternalProxySecret = ""
		}},
		{"default internal proxy secret", func(c *AppConfig) {
			c.Auth.APIKeys = []string{"legacy-key-at-least-16-chars"}
			c.Auth.InternalProxySecret = "change-me-internal-secret"
		}},
		{"default link secret", func(c *AppConfig) {
			c.Auth.APIKeys = []string{"legacy-key-at-least-16-chars"}
			c.Auth.SingleUseLinkSecret = "change-me-link-secret"
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseAuthConfig()
			tt.mutate(cfg)
			if err := validateAuthConfig(cfg); err == nil {
				t.Errorf("validateAuthConfig accepted invalid config %q", tt.name)
			}
		})
	}
}

// TestDefaultsRequireOperatorKeys confirms the shipped defaults never constitute
// a usable credential set: a server started with no auth configuration must fail
// validation rather than come up with a well-known key.
func TestDefaultsRequireOperatorKeys(t *testing.T) {
	cfg := DefaultAppConfig()
	if err := validateAuthConfig(&cfg); err == nil {
		t.Error("default config passed auth validation; it must require operator-supplied keys")
	}
}
