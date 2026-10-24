// Package config provides configuration management for CallFS.
// It handles loading and validating configuration from YAML files and environment variables.
package config

import "time"

// AppConfig represents the complete application configuration
type AppConfig struct {
	Server            ServerConfig            `koanf:"server"`
	Auth              AuthConfig              `koanf:"auth"`
	Log               LogConfig               `koanf:"log"`
	Metrics           MetricsConfig           `koanf:"metrics"`
	Backend           BackendConfig           `koanf:"backend"`
	MetadataStore     MetadataStoreConfig     `koanf:"metadata_store"`
	DLM               DLMConfig               `koanf:"dlm"`
	HA                HAConfig                `koanf:"ha"`
	InstanceDiscovery InstanceDiscoveryConfig `koanf:"instance_discovery"`
	Erasure           ErasureConfig           `koanf:"erasure"`
	UI                UIConfig                `koanf:"ui"`
	Trash             TrashConfig             `koanf:"trash"`
	NATS              NATSConfig              `koanf:"nats"`
	Performance       PerformanceConfig       `koanf:"performance"`
}

const (
	PerformanceFast   = "fast"
	PerformanceStrict = "strict"
)

type PerformanceConfig struct {
	Mode string `koanf:"mode"`
}

func (p PerformanceConfig) Strict() bool {
	return PerformanceStrict == p.Mode
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	ListenAddr        string          `koanf:"listen_addr"`
	Protocol          string          `koanf:"protocol"`
	ExternalURL       string          `koanf:"external_url"`
	CertFile          string          `koanf:"cert_file"`
	KeyFile           string          `koanf:"key_file"`
	EnableQUIC        bool            `koanf:"enable_quic"`
	QUICListenAddr    string          `koanf:"quic_listen_addr"`
	ReadHeaderTimeout time.Duration   `koanf:"read_header_timeout"`
	IdleTimeout       time.Duration   `koanf:"idle_timeout"`
	ReadTimeout       time.Duration   `koanf:"read_timeout"`
	WriteTimeout      time.Duration   `koanf:"write_timeout"`
	TransferTimeout   time.Duration   `koanf:"transfer_timeout"`
	FileOpTimeout     time.Duration   `koanf:"file_op_timeout"`
	MetadataOpTimeout time.Duration   `koanf:"metadata_op_timeout"`
	MetadataCacheTTL  time.Duration   `koanf:"metadata_cache_ttl"`
	MetadataCacheSize int             `koanf:"metadata_cache_size"`
	RateLimit         RateLimitConfig `koanf:"rate_limit"`
	TrustedProxies    []string        `koanf:"trusted_proxies"`
}

type RateLimitConfig struct {
	LinkGenerateRPS       float64 `koanf:"link_generate_rps"`
	LinkGenerateBurst     int     `koanf:"link_generate_burst"`
	MaxActiveLinksPerUser int     `koanf:"max_active_links_per_user"`
	DownloadRPS           float64 `koanf:"download_rps"`
	DownloadBurst         int     `koanf:"download_burst"`
	SessionRPS            float64 `koanf:"session_rps"`
	SessionBurst          int     `koanf:"session_burst"`
}

type UIConfig struct {
	Enabled               bool          `koanf:"enabled"`
	SessionTTL            time.Duration `koanf:"session_ttl"`
	IdleTimeout           time.Duration `koanf:"idle_timeout"`
	AllowRemoteRestart    bool          `koanf:"allow_remote_restart"`
	MetricsSampleInterval time.Duration `koanf:"metrics_sample_interval"`
	MetricsRetention      time.Duration `koanf:"metrics_retention"`
	LogBufferEntries      int           `koanf:"log_buffer_entries"`
	AuditBufferEntries    int           `koanf:"audit_buffer_entries"`
	EventBufferEntries    int           `koanf:"event_buffer_entries"`
}

type TrashConfig struct {
	Enabled   bool          `koanf:"enabled"`
	Retention time.Duration `koanf:"retention"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	// APIKeys is the legacy positional form. Each key is assigned the user ID
	// "api-user-<index>", derived from its position in this list.
	//
	// That identity is NOT stable: removing or reordering an entry shifts every
	// later key's user ID, which silently transfers ownership of the removed
	// user's existing files to whoever holds the next key. Prefer APIKeyUsers,
	// which binds each key to an explicit, position-independent identity.
	APIKeys []string `koanf:"api_keys"`

	// APIKeyUsers maps an explicit app user ID to its API key. Identity is bound
	// to the name, not to ordering, so keys can be revoked, rotated, or reordered
	// without reassigning ownership of existing files.
	APIKeyUsers map[string]string `koanf:"api_key_users"`

	InternalProxySecret string   `koanf:"internal_proxy_secret"`
	SingleUseLinkSecret string   `koanf:"single_use_link_secret"`
	AdminUsers          []string `koanf:"admin_users"`
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string `koanf:"level"`
	Format string `koanf:"format"`
}

// MetricsConfig holds metrics server configuration
type MetricsConfig struct {
	ListenAddr string `koanf:"listen_addr"`
	Profiling  bool   `koanf:"profiling"`
}

// BackendConfig holds backend storage configuration
type BackendConfig struct {
	DefaultBackend             string        `koanf:"default_backend"` // Default backend for new files: "localfs" or "s3"
	LocalFSRootPath            string        `koanf:"localfs_root_path"`
	LocalFSSync                string        `koanf:"localfs_sync"`
	LocalFSAck                 string        `koanf:"localfs_ack"`
	FlushWorkers               int           `koanf:"flush_workers"`
	FlushMaxPendingBytes       int64         `koanf:"flush_max_pending_bytes"`
	FlushMaxDelay              time.Duration `koanf:"flush_max_delay"`
	S3AccessKey                string        `koanf:"s3_access_key"`
	S3SecretKey                string        `koanf:"s3_secret_key"`
	S3Region                   string        `koanf:"s3_region"`
	S3BucketName               string        `koanf:"s3_bucket_name"`
	S3Endpoint                 string        `koanf:"s3_endpoint"`                    // Custom S3 endpoint (e.g., for MinIO)
	S3AllowInsecure            bool          `koanf:"s3_allow_insecure"`              // Permit a plaintext http:// S3 endpoint
	S3ServerSideEncryption     string        `koanf:"s3_server_side_encryption"`      // SSE algorithm (AES256, aws:kms)
	S3ACL                      string        `koanf:"s3_acl"`                         // Object ACL (private, public-read, etc.)
	S3KMSKeyID                 string        `koanf:"s3_kms_key_id"`                  // KMS key ID for SSE-KMS
	InternalProxySkipTLSVerify bool          `koanf:"internal_proxy_skip_tls_verify"` // Skip TLS certificate verification for internal proxy requests
}

// MetadataStoreConfig holds metadata store configuration
type MetadataStoreConfig struct {
	Type               string         `koanf:"type"` // postgres | sqlite | redis | nats
	DSN                string         `koanf:"dsn"`
	MigrationDSN       string         `koanf:"migration_dsn"`
	SQLitePath         string         `koanf:"sqlite_path"`
	RedisAddr          string         `koanf:"redis_addr"`
	RedisUsername      string         `koanf:"redis_username"`
	RedisPassword      string         `koanf:"redis_password"`
	RedisDB            int            `koanf:"redis_db"`
	RedisKeyPrefix     string         `koanf:"redis_key_prefix"`
	RedisTLS           bool           `koanf:"redis_tls"`
	RedisTLSCAFile     string         `koanf:"redis_tls_ca_file"`
	RedisTLSSkipVerify bool           `koanf:"redis_tls_skip_verify"`
	Postgres           PostgresConfig `koanf:"postgres"`
}

type PostgresConfig struct {
	MaxOpenConns    int           `koanf:"max_open_conns"`
	MaxIdleConns    int           `koanf:"max_idle_conns"`
	ConnMaxLifetime time.Duration `koanf:"conn_max_lifetime"`
}

// DLMConfig holds distributed lock manager configuration
type DLMConfig struct {
	Type               string        `koanf:"type"` // redis | local | nats
	RedisAddr          string        `koanf:"redis_addr"`
	RedisPassword      string        `koanf:"redis_password"`
	WaitTimeout        time.Duration `koanf:"wait_timeout"`
	RedisPoolSize      int           `koanf:"redis_pool_size"`
	RedisMinIdleConns  int           `koanf:"redis_min_idle_conns"`
	RedisTLS           bool          `koanf:"redis_tls"`
	RedisTLSCAFile     string        `koanf:"redis_tls_ca_file"`
	RedisTLSSkipVerify bool          `koanf:"redis_tls_skip_verify"`
}

// HAConfig controls optional high-availability replication behavior
type HAConfig struct {
	ReplicationEnabled    bool   `koanf:"replication_enabled"`
	ReplicaBackend        string `koanf:"replica_backend"` // localfs | s3
	RequireReplicaSuccess bool   `koanf:"require_replica_success"`
}

// ErasureConfig holds erasure coding configuration
type ErasureConfig struct {
	Enabled      bool   `koanf:"enabled"`
	DataShards   int    `koanf:"data_shards"`   // default: 4
	ParityShards int    `koanf:"parity_shards"` // default: 2
	ShardBackend string `koanf:"shard_backend"` // "localfs" or "s3"
}

// InstanceDiscoveryConfig holds instance discovery configuration
type InstanceDiscoveryConfig struct {
	InstanceID    string            `koanf:"instance_id"`
	PeerEndpoints map[string]string `koanf:"peer_endpoints"`
}

type NATSConfig struct {
	Enabled         bool                `koanf:"enabled"`
	Mode            string              `koanf:"mode"`
	ServerName      string              `koanf:"server_name"`
	ClusterName     string              `koanf:"cluster_name"`
	ClientListen    string              `koanf:"client_listen"`
	ClusterListen   string              `koanf:"cluster_listen"`
	Routes          []string            `koanf:"routes"`
	RoutesFromPeers bool                `koanf:"routes_from_peers"`
	MonitorListen   string              `koanf:"monitor_listen"`
	ServerTags      map[string]string   `koanf:"server_tags"`
	TLS             NATSTLSConfig       `koanf:"tls"`
	Auth            NATSAuthConfig      `koanf:"auth"`
	JetStream       NATSJetStreamConfig `koanf:"jetstream"`
	External        NATSExternalConfig  `koanf:"external"`
	RPCTimeout      time.Duration       `koanf:"rpc_timeout"`
	Metadata        NATSMetadataConfig  `koanf:"metadata"`
	Locks           NATSLocksConfig     `koanf:"locks"`
	Streams         NATSStreamsConfig   `koanf:"streams"`
	Bytes           string              `koanf:"bytes"`
	Counters        bool                `koanf:"counters"`
}

type NATSTLSConfig struct {
	CertFile       string `koanf:"cert_file"`
	KeyFile        string `koanf:"key_file"`
	CAFile         string `koanf:"ca_file"`
	ClientCertFile string `koanf:"client_cert_file"`
	ClientKeyFile  string `koanf:"client_key_file"`
}

type NATSAuthConfig struct {
	Mode           string `koanf:"mode"`
	NKeySeedFile   string `koanf:"nkey_seed_file"`
	CredsFile      string `koanf:"creds_file"`
	PreviousSecret string `koanf:"previous_secret"`
}

type NATSJetStreamConfig struct {
	StoreDir     string        `koanf:"store_dir"`
	MaxMemory    ByteSize      `koanf:"max_memory"`
	MaxStore     ByteSize      `koanf:"max_store"`
	Sync         string        `koanf:"sync"`
	SyncInterval time.Duration `koanf:"sync_interval"`
	UniqueTag    string        `koanf:"unique_tag"`
}

type NATSExternalConfig struct {
	URLs      []string `koanf:"urls"`
	JSDomain  string   `koanf:"js_domain"`
	TLSCAFile string   `koanf:"tls_ca_file"`
}

type NATSMetadataConfig struct {
	Replicas         int           `koanf:"replicas"`
	ApplyTimeout     time.Duration `koanf:"apply_timeout"`
	ReadBarrier      string        `koanf:"read_barrier"`
	MissBarrier      bool          `koanf:"miss_barrier"`
	SnapshotInterval time.Duration `koanf:"snapshot_interval"`
	SnapshotEvery    uint64        `koanf:"snapshot_every"`
	RetainCommands   uint64        `koanf:"retain_commands"`
	RetainSnapshots  int           `koanf:"retain_snapshots"`
	MaxMsgSize       ByteSize      `koanf:"max_msg_size"`
}

type NATSLocksConfig struct {
	Replicas   int           `koanf:"replicas"`
	TTL        time.Duration `koanf:"ttl"`
	QueueStale time.Duration `koanf:"queue_stale"`
}

type NATSStreamsConfig struct {
	Replicas        int             `koanf:"replicas"`
	EventsMaxAge    time.Duration   `koanf:"events_max_age"`
	EventsMaxBytes  ByteSize        `koanf:"events_max_bytes"`
	AuditMaxAge     time.Duration   `koanf:"audit_max_age"`
	LogsLevel       string          `koanf:"logs_level"`
	LogsMaxAge      time.Duration   `koanf:"logs_max_age"`
	TasksMaxDeliver int             `koanf:"tasks_max_deliver"`
	TasksBackoff    []time.Duration `koanf:"tasks_backoff"`
	DLQMaxAge       time.Duration   `koanf:"dlq_max_age"`
}

const (
	NATSModeEmbedded = "embedded"
	NATSModeExternal = "external"

	NATSAuthDerived = "derived"
	NATSAuthNKey    = "nkey"
	NATSAuthCreds   = "creds"

	NATSSyncAlways   = "always"
	NATSSyncInterval = "interval"

	NATSReadBarrierInfo    = "info"
	NATSReadBarrierPublish = "publish"

	NATSBytesHTTP    = "http"
	NATSBytesChunked = "chunked"
)
