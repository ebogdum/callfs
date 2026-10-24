package config

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

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
func parserForFile(path string) koanf.Parser {
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		return yaml.Parser()
	}
	if strings.HasSuffix(path, ".json") {
		return json.Parser()
	}
	return nil
}

func LoadConfigFromFile(configFilePath string) (AppConfig, error) {
	k := koanf.New(".")

	// Load default configuration first
	defaultCfg := DefaultAppConfig()
	if err := k.Load(structs.Provider(defaultCfg, "koanf"), nil); err != nil {
		return AppConfig{}, fmt.Errorf("failed to load default config: %w", err)
	}

	// Load from config file
	if configFilePath != "" {
		if _, err := os.Stat(configFilePath); err != nil {
			return AppConfig{}, fmt.Errorf("specified config file %s not found: %w", configFilePath, err)
		}
		if err := k.Load(file.Provider(configFilePath), parserForFile(configFilePath)); err != nil {
			return AppConfig{}, fmt.Errorf("failed to load config file %s: %w", configFilePath, err)
		}
	} else {
		configFiles := []string{"config.yaml", "config.yml", "config.json"}
		for _, configFile := range configFiles {
			_, err := os.Stat(configFile)
			if nil != err {
				if os.IsNotExist(err) {
					continue
				}
				return AppConfig{}, fmt.Errorf("failed to access config file %s: %w", configFile, err)
			}
			if err := k.Load(file.Provider(configFile), parserForFile(configFile)); err != nil {
				return AppConfig{}, fmt.Errorf("failed to load config file %s: %w", configFile, err)
			}
			break
		}
	}

	// Load environment variables with CALLFS_ prefix
	if err := k.Load(env.ProviderWithValue("CALLFS_", ".", func(s string, value string) (string, interface{}) {
		key := strings.TrimPrefix(s, "CALLFS_")
		const apiKeyUsersPrefix = "AUTH__API_KEY_USERS__"
		if strings.HasPrefix(key, apiKeyUsersPrefix) {
			return "auth.api_key_users." + strings.ToLower(strings.TrimPrefix(key, apiKeyUsersPrefix)), value
		}
		key = strings.ToLower(key)
		key = strings.ReplaceAll(key, "__", ".")
		if commaListKeys[key] {
			return key, splitCommaList(value)
		}
		return key, value
	}), nil); err != nil {
		return AppConfig{}, fmt.Errorf("failed to load environment variables: %w", err)
	}

	if err := rejectLegacyRaft(k); nil != err {
		return AppConfig{}, err
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

func validateServerConfig(cfg *AppConfig) error {
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

	if cfg.Server.ReadHeaderTimeout <= 0 {
		return fmt.Errorf("server.read_header_timeout must be > 0")
	}
	if cfg.Server.IdleTimeout <= 0 {
		return fmt.Errorf("server.idle_timeout must be > 0")
	}
	if cfg.Server.TransferTimeout <= 0 {
		return fmt.Errorf("server.transfer_timeout must be > 0")
	}
	if 0 >= cfg.Server.ReadTimeout {
		return fmt.Errorf("server.read_timeout must be > 0")
	}
	if 0 >= cfg.Server.WriteTimeout {
		return fmt.Errorf("server.write_timeout must be > 0")
	}
	if 0 >= cfg.Server.FileOpTimeout {
		return fmt.Errorf("server.file_op_timeout must be > 0")
	}
	if 0 >= cfg.Server.MetadataOpTimeout {
		return fmt.Errorf("server.metadata_op_timeout must be > 0")
	}
	if cfg.Server.MetadataCacheTTL < 0 {
		return fmt.Errorf("server.metadata_cache_ttl must be >= 0")
	}
	if 0 >= cfg.Server.MetadataCacheSize {
		return fmt.Errorf("server.metadata_cache_size must be > 0")
	}
	if cfg.Server.RateLimit.LinkGenerateRPS <= 0 {
		return fmt.Errorf("server.rate_limit.link_generate_rps must be > 0")
	}
	if cfg.Server.RateLimit.LinkGenerateBurst < 1 {
		return fmt.Errorf("server.rate_limit.link_generate_burst must be >= 1")
	}
	if cfg.Server.RateLimit.DownloadRPS <= 0 {
		return fmt.Errorf("server.rate_limit.download_rps must be > 0")
	}
	if cfg.Server.RateLimit.DownloadBurst < 1 {
		return fmt.Errorf("server.rate_limit.download_burst must be >= 1")
	}
	if cfg.Server.RateLimit.SessionRPS <= 0 {
		return fmt.Errorf("server.rate_limit.session_rps must be > 0")
	}
	if cfg.Server.RateLimit.SessionBurst < 1 {
		return fmt.Errorf("server.rate_limit.session_burst must be >= 1")
	}
	for _, cidr := range cfg.Server.TrustedProxies {
		if _, err := netip.ParsePrefix(cidr); nil != err {
			return fmt.Errorf("server.trusted_proxies contains invalid CIDR %q: %w", cidr, err)
		}
	}

	return nil
}

func validateMetadataStoreConfig(cfg *AppConfig) error {
	if cfg.MetadataStore.Type == "" {
		cfg.MetadataStore.Type = "postgres"
	}

	switch strings.ToLower(cfg.MetadataStore.Type) {
	case "postgres":
		if cfg.MetadataStore.DSN == "" {
			return fmt.Errorf("metadata_store.dsn is required when metadata_store.type=postgres")
		}
		if 0 >= cfg.MetadataStore.Postgres.MaxOpenConns {
			return fmt.Errorf("metadata_store.postgres.max_open_conns must be > 0 when metadata_store.type=postgres")
		}
		if 0 > cfg.MetadataStore.Postgres.MaxIdleConns {
			return fmt.Errorf("metadata_store.postgres.max_idle_conns must be >= 0 when metadata_store.type=postgres")
		}
		if 0 > cfg.MetadataStore.Postgres.ConnMaxLifetime {
			return fmt.Errorf("metadata_store.postgres.conn_max_lifetime must be >= 0 when metadata_store.type=postgres")
		}
	case "sqlite":
		if cfg.MetadataStore.SQLitePath == "" {
			return fmt.Errorf("metadata_store.sqlite_path is required when metadata_store.type=sqlite")
		}
	case "redis":
		if cfg.MetadataStore.RedisAddr == "" {
			return fmt.Errorf("metadata_store.redis_addr is required when metadata_store.type=redis")
		}
	case "nats":
		if !cfg.NATS.Enabled {
			return fmt.Errorf("metadata_store.type=nats requires nats.enabled=true")
		}
	case "raft":
		return ErrRaftRemoved
	default:
		return fmt.Errorf("metadata_store.type must be one of: postgres, sqlite, redis, nats")
	}

	return nil
}

func isLoopbackBindAddr(bindAddr string) bool {
	host, _, err := net.SplitHostPort(bindAddr)
	if nil != err {
		return false
	}
	if "localhost" == host {
		return true
	}
	addr, err := netip.ParseAddr(host)
	if nil != err {
		return false
	}
	return addr.IsLoopback()
}

func validatePeerEndpoints(field string, endpoints map[string]string) error {
	for nodeID, endpoint := range endpoints {
		parsed, err := url.Parse(endpoint)
		if nil != err {
			return fmt.Errorf("%s: endpoint for %q is not a valid URL: %w", field, nodeID, err)
		}
		if "https" != strings.ToLower(parsed.Scheme) && !isLoopbackEndpoint(parsed) {
			return fmt.Errorf("%s: endpoint for %q must use https because the internal secret is sent to it", field, nodeID)
		}
	}
	return nil
}

func validateDLMConfig(cfg *AppConfig) error {
	if cfg.DLM.Type == "" {
		cfg.DLM.Type = "redis"
	}

	switch strings.ToLower(cfg.DLM.Type) {
	case "redis":
		if cfg.DLM.RedisAddr == "" {
			return fmt.Errorf("dlm.redis_addr is required when dlm.type=redis")
		}
		if 0 >= cfg.DLM.RedisPoolSize {
			return fmt.Errorf("dlm.redis_pool_size must be > 0 when dlm.type=redis")
		}
		if 0 > cfg.DLM.RedisMinIdleConns {
			return fmt.Errorf("dlm.redis_min_idle_conns must be >= 0 when dlm.type=redis")
		}
	case "local":
		if cfg.NATS.Enabled && 0 < len(remotePeerEndpoints(cfg)) {
			return fmt.Errorf("dlm.type=local cannot coordinate several nodes; set dlm.type to nats or redis")
		}
	case "nats":
		if !cfg.NATS.Enabled {
			return fmt.Errorf("dlm.type=nats requires nats.enabled=true")
		}
	case "raft":
		return ErrRaftRemoved
	default:
		return fmt.Errorf("dlm.type must be one of: redis, local, nats")
	}

	return nil
}

// reservedUserIDPattern matches the identities CallFS assigns itself: the
// positional "api-user-N" form and the built-in admin accounts. An operator must
// not be able to claim one of these via api_key_users, because "root" and
// "internal-proxy" bypass every permission check and an "api-user-N" collision
// would let two different keys resolve to the same owner.
var reservedUserIDPattern = regexp.MustCompile(`^api-user-\d+$`)

var placeholderSecretMarkers = []string{"your-", "change-me", "a-strong-"}

func isPlaceholderSecret(secret string) bool {
	lowered := strings.ToLower(secret)
	for _, marker := range placeholderSecretMarkers {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}

func validateAPIKey(source, key string) error {
	if key == "default-api-key" {
		return fmt.Errorf("auth.%s must not use default value 'default-api-key'", source)
	}
	if isPlaceholderSecret(key) {
		return fmt.Errorf("auth.%s must not use an example placeholder value", source)
	}
	if len(key) < 16 {
		return fmt.Errorf("auth.%s: each key must be at least 16 characters", source)
	}
	return nil
}

func validateAuthConfig(cfg *AppConfig) error {
	if len(cfg.Auth.APIKeys) == 0 && len(cfg.Auth.APIKeyUsers) == 0 {
		return fmt.Errorf("auth.api_keys or auth.api_key_users must contain at least one key")
	}

	// A key must resolve to exactly one identity. If the same secret appeared in
	// both forms, the authenticated user would depend on map iteration order.
	seenKeys := make(map[string]string, len(cfg.Auth.APIKeys)+len(cfg.Auth.APIKeyUsers))

	for i, key := range cfg.Auth.APIKeys {
		if err := validateAPIKey("api_keys", key); err != nil {
			return err
		}
		if owner, dup := seenKeys[key]; dup {
			return fmt.Errorf("auth.api_keys: key at index %d is already configured for %s", i, owner)
		}
		seenKeys[key] = fmt.Sprintf("api_keys index %d", i)
	}

	for userID, key := range cfg.Auth.APIKeyUsers {
		if strings.TrimSpace(userID) == "" {
			return fmt.Errorf("auth.api_key_users: user ID must not be empty")
		}
		if userID == "root" || userID == "internal-proxy" || reservedUserIDPattern.MatchString(userID) {
			return fmt.Errorf("auth.api_key_users: user ID %q is reserved", userID)
		}
		if err := validateAPIKey("api_key_users", key); err != nil {
			return err
		}
		if owner, dup := seenKeys[key]; dup {
			return fmt.Errorf("auth.api_key_users: key for %q is already configured for %s", userID, owner)
		}
		seenKeys[key] = fmt.Sprintf("api_key_users %q", userID)
	}

	if _, dup := seenKeys[cfg.Auth.InternalProxySecret]; dup && cfg.Auth.InternalProxySecret != "" {
		return fmt.Errorf("auth.internal_proxy_secret must not be reused as an API key")
	}

	if "" == cfg.Auth.InternalProxySecret || "change-me-internal-secret" == cfg.Auth.InternalProxySecret || isPlaceholderSecret(cfg.Auth.InternalProxySecret) {
		return fmt.Errorf("auth.internal_proxy_secret must be set and not use default value")
	}
	if len(cfg.Auth.InternalProxySecret) < 16 {
		return fmt.Errorf("auth.internal_proxy_secret must be at least 16 characters")
	}

	if "" == cfg.Auth.SingleUseLinkSecret || "change-me-link-secret" == cfg.Auth.SingleUseLinkSecret || isPlaceholderSecret(cfg.Auth.SingleUseLinkSecret) {
		return fmt.Errorf("auth.single_use_link_secret must be set and not use default value")
	}
	if len(cfg.Auth.SingleUseLinkSecret) < 16 {
		return fmt.Errorf("auth.single_use_link_secret must be at least 16 characters")
	}

	if cfg.Auth.InternalProxySecret == cfg.Auth.SingleUseLinkSecret {
		return fmt.Errorf("auth.single_use_link_secret must not be the same as auth.internal_proxy_secret")
	}
	if _, dup := seenKeys[cfg.Auth.SingleUseLinkSecret]; dup {
		return fmt.Errorf("auth.single_use_link_secret must not be reused as an API key")
	}

	return nil
}

// validateConfig validates that required configuration fields are set
func validateConfig(cfg *AppConfig) error {
	if err := validateServerConfig(cfg); err != nil {
		return err
	}

	if err := validateMetadataStoreConfig(cfg); err != nil {
		return err
	}

	if err := validateDLMConfig(cfg); err != nil {
		return err
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

	if err := validatePeerEndpoints("instance_discovery.peer_endpoints", cfg.InstanceDiscovery.PeerEndpoints); err != nil {
		return err
	}

	if err := validateAuthConfig(cfg); err != nil {
		return err
	}

	if err := validateNATSConfig(cfg); err != nil {
		return err
	}

	if err := validateAdminUsers(cfg); err != nil {
		return err
	}

	if err := validateUIConfig(cfg); err != nil {
		return err
	}

	if err := validateTrashConfig(cfg); err != nil {
		return err
	}

	if cfg.Erasure.Enabled {
		if cfg.Erasure.DataShards < 2 {
			return fmt.Errorf("erasure.data_shards must be >= 2 (got %d)", cfg.Erasure.DataShards)
		}
		if cfg.Erasure.ParityShards < 1 {
			return fmt.Errorf("erasure.parity_shards must be >= 1 (got %d)", cfg.Erasure.ParityShards)
		}
		if cfg.Erasure.DataShards+cfg.Erasure.ParityShards > 256 {
			return fmt.Errorf("erasure.data_shards + erasure.parity_shards must not exceed 256")
		}
	}

	cfg.Backend.DefaultBackend = strings.ToLower(strings.TrimSpace(cfg.Backend.DefaultBackend))
	switch cfg.Backend.DefaultBackend {
	case "localfs", "s3", "":
	default:
		return fmt.Errorf("backend.default_backend must be one of: localfs, s3 (got %q)", cfg.Backend.DefaultBackend)
	}

	cfg.Backend.LocalFSSync = strings.ToLower(strings.TrimSpace(cfg.Backend.LocalFSSync))
	switch cfg.Backend.LocalFSSync {
	case "full", "barrier", "none", "":
	default:
		return fmt.Errorf("backend.localfs_sync must be one of: full, barrier, none (got %q)", cfg.Backend.LocalFSSync)
	}
	cfg.Performance.Mode = strings.ToLower(strings.TrimSpace(cfg.Performance.Mode))
	switch cfg.Performance.Mode {
	case PerformanceFast, PerformanceStrict:
	case "":
		cfg.Performance.Mode = PerformanceFast
	default:
		return fmt.Errorf("performance.mode must be one of: fast, strict (got %q)", cfg.Performance.Mode)
	}
	cfg.Backend.LocalFSAck = strings.ToLower(strings.TrimSpace(cfg.Backend.LocalFSAck))
	switch cfg.Backend.LocalFSAck {
	case "memory", "disk", "":
	default:
		return fmt.Errorf("backend.localfs_ack must be one of: memory, disk (got %q)", cfg.Backend.LocalFSAck)
	}
	if 0 > cfg.Backend.FlushWorkers {
		return fmt.Errorf("backend.flush_workers must be >= 0 (0 means one per CPU)")
	}
	if 0 > cfg.Backend.FlushMaxPendingBytes {
		return fmt.Errorf("backend.flush_max_pending_bytes must be >= 0")
	}
	if 0 > cfg.Backend.FlushMaxDelay {
		return fmt.Errorf("backend.flush_max_delay must be >= 0")
	}

	return nil
}

func isConfiguredUserID(cfg *AppConfig, userID string) bool {
	if "root" == userID {
		return true
	}
	if _, ok := cfg.Auth.APIKeyUsers[userID]; ok {
		return true
	}
	for i := range cfg.Auth.APIKeys {
		if fmt.Sprintf("api-user-%d", i) == userID {
			return true
		}
	}
	return false
}

func validateAdminUsers(cfg *AppConfig) error {
	for _, userID := range cfg.Auth.AdminUsers {
		if !isConfiguredUserID(cfg, userID) {
			return fmt.Errorf("auth.admin_users: %q is not a configured user", userID)
		}
	}
	return nil
}

func validateUIConfig(cfg *AppConfig) error {
	ui := cfg.UI
	if ui.SessionTTL < 5*time.Minute || 720*time.Hour < ui.SessionTTL {
		return fmt.Errorf("ui.session_ttl must be between 5m and 720h")
	}
	if ui.IdleTimeout < time.Minute || ui.SessionTTL < ui.IdleTimeout {
		return fmt.Errorf("ui.idle_timeout must be between 1m and ui.session_ttl")
	}
	if ui.MetricsSampleInterval < time.Second || 60*time.Second < ui.MetricsSampleInterval {
		return fmt.Errorf("ui.metrics_sample_interval must be between 1s and 60s")
	}
	if ui.MetricsRetention < time.Hour || 168*time.Hour < ui.MetricsRetention {
		return fmt.Errorf("ui.metrics_retention must be between 1h and 168h")
	}
	if 100000 < ui.MetricsRetention/ui.MetricsSampleInterval {
		return fmt.Errorf("ui.metrics_retention / ui.metrics_sample_interval must not exceed 100000 samples per series")
	}
	buffers := []struct {
		field string
		value int
	}{
		{"ui.log_buffer_entries", ui.LogBufferEntries},
		{"ui.audit_buffer_entries", ui.AuditBufferEntries},
		{"ui.event_buffer_entries", ui.EventBufferEntries},
	}
	for _, buffer := range buffers {
		if buffer.value < 100 || 1000000 < buffer.value {
			return fmt.Errorf("%s must be between 100 and 1000000", buffer.field)
		}
	}
	return nil
}

func validateTrashConfig(cfg *AppConfig) error {
	if cfg.Trash.Retention < time.Hour || 8760*time.Hour < cfg.Trash.Retention {
		return fmt.Errorf("trash.retention must be between 1h and 8760h")
	}
	return nil
}

func isLoopbackEndpoint(u *url.URL) bool {
	if "http" != strings.ToLower(u.Scheme) {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold("localhost", host) {
		return true
	}
	ip := net.ParseIP(host)
	return nil != ip && ip.IsLoopback()
}
