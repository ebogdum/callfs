package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/json"
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
		} else if strings.HasSuffix(configFilePath, ".json") {
			parser = json.Parser()
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
				} else if strings.HasSuffix(configFile, ".json") {
					parser = json.Parser()
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
		key := strings.TrimPrefix(s, "CALLFS_")
		key = strings.ToLower(key)
		key = strings.ReplaceAll(key, "__", ".")
		return key
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

	if cfg.Server.Protocol == "" {
		cfg.Server.Protocol = "https"
	}

	switch strings.ToLower(cfg.Server.Protocol) {
	case "http", "https", "auto":
	default:
		return fmt.Errorf("server.protocol must be one of: http, https, auto")
	}

	if strings.ToLower(cfg.Server.Protocol) == "https" {
		if cfg.Server.CertFile == "" || cfg.Server.KeyFile == "" {
			return fmt.Errorf("server.cert_file and server.key_file are required when server.protocol=https")
		}
	}

	if cfg.Server.EnableQUIC {
		if cfg.Server.CertFile == "" || cfg.Server.KeyFile == "" {
			return fmt.Errorf("server.cert_file and server.key_file are required when server.enable_quic=true")
		}
		if cfg.Server.QUICListenAddr == "" {
			return fmt.Errorf("server.quic_listen_addr is required when server.enable_quic=true")
		}
	}

	if cfg.MetadataStore.Type == "" {
		cfg.MetadataStore.Type = "postgres"
	}

	switch strings.ToLower(cfg.MetadataStore.Type) {
	case "postgres":
		if cfg.MetadataStore.DSN == "" {
			return fmt.Errorf("metadata_store.dsn is required when metadata_store.type=postgres")
		}
	case "sqlite":
		if cfg.MetadataStore.SQLitePath == "" {
			return fmt.Errorf("metadata_store.sqlite_path is required when metadata_store.type=sqlite")
		}
	case "redis":
		if cfg.MetadataStore.RedisAddr == "" {
			return fmt.Errorf("metadata_store.redis_addr is required when metadata_store.type=redis")
		}
	case "raft":
		if !cfg.Raft.Enabled {
			cfg.Raft.Enabled = true
		}
		if cfg.Raft.NodeID == "" {
			return fmt.Errorf("raft.node_id is required when metadata_store.type=raft")
		}
		if cfg.Raft.BindAddr == "" {
			return fmt.Errorf("raft.bind_addr is required when metadata_store.type=raft")
		}
		if cfg.Raft.DataDir == "" {
			return fmt.Errorf("raft.data_dir is required when metadata_store.type=raft")
		}
		if cfg.Raft.ApplyTimeout <= 0 {
			return fmt.Errorf("raft.apply_timeout must be > 0 when metadata_store.type=raft")
		}
		if cfg.Raft.ForwardTimeout <= 0 {
			return fmt.Errorf("raft.forward_timeout must be > 0 when metadata_store.type=raft")
		}
		if cfg.Raft.SnapshotInterval <= 0 {
			return fmt.Errorf("raft.snapshot_interval must be > 0 when metadata_store.type=raft")
		}
		if cfg.Raft.SnapshotThreshold == 0 {
			return fmt.Errorf("raft.snapshot_threshold must be > 0 when metadata_store.type=raft")
		}
		if cfg.Raft.RetainSnapshotCount <= 0 {
			return fmt.Errorf("raft.retain_snapshot_count must be > 0 when metadata_store.type=raft")
		}
	default:
		return fmt.Errorf("metadata_store.type must be one of: postgres, sqlite, redis, raft")
	}

	if cfg.DLM.Type == "" {
		cfg.DLM.Type = "redis"
	}

	switch strings.ToLower(cfg.DLM.Type) {
	case "redis":
		if cfg.DLM.RedisAddr == "" {
			return fmt.Errorf("dlm.redis_addr is required when dlm.type=redis")
		}
	case "local":
	default:
		return fmt.Errorf("dlm.type must be one of: redis, local")
	}

	if cfg.HA.ReplicationEnabled {
		replicaBackend := strings.ToLower(strings.TrimSpace(cfg.HA.ReplicaBackend))
		if replicaBackend != "localfs" && replicaBackend != "s3" {
			return fmt.Errorf("ha.replica_backend must be one of: localfs, s3 when ha.replication_enabled=true")
		}
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
