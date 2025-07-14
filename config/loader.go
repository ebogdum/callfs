package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

// LoadConfig loads configuration from multiple sources with strict priority:
// 1. Environment variables (highest priority)
// 2. Config file (config.yaml or config.json)
// 3. Defaults (lowest priority)
func LoadConfig() (AppConfig, error) {
	return LoadConfigFromFile("")
}

// LoadConfigFromFile loads configuration from multiple sources with a specific config file:
// 1. Environment variables (highest priority)
// 2. Specified config file or default config files
// 3. Defaults (lowest priority)
func LoadConfigFromFile(configFilePath string) (AppConfig, error) {
	k := koanf.New(".")

	// Load default configuration first
	defaultCfg := DefaultAppConfig()
	if err := k.Load(structs.Provider(defaultCfg, "koanf"), nil); err != nil {
		return AppConfig{}, fmt.Errorf("failed to load default config: %w", err)
	}

	// Load from config file
	if configFilePath != "" {
		// Use specified config file
		if _, err := os.Stat(configFilePath); err != nil {
			return AppConfig{}, fmt.Errorf("specified config file %s not found: %w", configFilePath, err)
		}

		var parser koanf.Parser
		if strings.HasSuffix(configFilePath, ".yaml") || strings.HasSuffix(configFilePath, ".yml") {
			parser = yaml.Parser()
		}

		if err := k.Load(file.Provider(configFilePath), parser); err != nil {
			return AppConfig{}, fmt.Errorf("failed to load config file %s: %w", configFilePath, err)
		}
	} else {
		// Load from default config files if they exist
		configFiles := []string{"config.yaml", "config.yml", "config.json"}
		for _, configFile := range configFiles {
			if _, err := os.Stat(configFile); err == nil {
				var parser koanf.Parser
				if strings.HasSuffix(configFile, ".yaml") || strings.HasSuffix(configFile, ".yml") {
					parser = yaml.Parser()
				}

				if err := k.Load(file.Provider(configFile), parser); err != nil {
					return AppConfig{}, fmt.Errorf("failed to load config file %s: %w", configFile, err)
				}
				break
			}
		}
	}

	// Load environment variables with CALLFS_ prefix
	if err := k.Load(env.Provider("CALLFS_", ".", func(s string) string {
		return strings.Replace(strings.ToLower(
			strings.TrimPrefix(s, "CALLFS_")), "_", ".", -1)
	}), nil); err != nil {
		return AppConfig{}, fmt.Errorf("failed to load environment variables: %w", err)
	}

	// Unmarshal into config struct
	var cfg AppConfig
	if err := k.Unmarshal("", &cfg); err != nil {
		return AppConfig{}, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate required fields
	if err := validateConfig(&cfg); err != nil {
		return AppConfig{}, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// validateConfig validates that required configuration fields are set
func validateConfig(cfg *AppConfig) error {
	if cfg.Server.ListenAddr == "" {
		return fmt.Errorf("server.listen_addr is required")
	}

	if cfg.MetadataStore.DSN == "" {
		return fmt.Errorf("metadata_store.dsn is required")
	}

	if cfg.InstanceDiscovery.InstanceID == "" {
		return fmt.Errorf("instance_discovery.instance_id is required")
	}

	if len(cfg.Auth.APIKeys) == 0 {
		return fmt.Errorf("auth.api_keys must contain at least one key")
	}

	if cfg.Auth.InternalProxySecret == "" || cfg.Auth.InternalProxySecret == "change-me-internal-secret" {
		return fmt.Errorf("auth.internal_proxy_secret must be set and not use default value")
	}

	if cfg.Auth.SingleUseLinkSecret == "" || cfg.Auth.SingleUseLinkSecret == "change-me-link-secret" {
		return fmt.Errorf("auth.single_use_link_secret must be set and not use default value")
	}

	return nil
}
