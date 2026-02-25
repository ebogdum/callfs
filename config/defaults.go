package config

import "time"

// DefaultAppConfig returns an AppConfig struct with sensible default values
func DefaultAppConfig() AppConfig {
	return AppConfig{
		Server: ServerConfig{
			ListenAddr:        ":8443",
			Protocol:          "https",
			ExternalURL:       "localhost:8443",
			CertFile:          "server.crt",
			KeyFile:           "server.key",
			EnableQUIC:        false,
			QUICListenAddr:    ":8443",
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			FileOpTimeout:     10 * time.Second,
			MetadataOpTimeout: 5 * time.Second,
		},
		Auth: AuthConfig{
			APIKeys:             []string{"default-api-key"},
			InternalProxySecret: "change-me-internal-secret",
			SingleUseLinkSecret: "change-me-link-secret",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
		Metrics: MetricsConfig{
			ListenAddr: ":9090",
		},
		Backend: BackendConfig{
			DefaultBackend:             "localfs", // Default to local filesystem
			LocalFSRootPath:            "/var/lib/callfs",
			S3AccessKey:                "",
			S3SecretKey:                "",
			S3Region:                   "us-east-1",
			S3BucketName:               "",
			S3ServerSideEncryption:     "AES256",  // Default to AES256 for security
			S3ACL:                      "private", // Default to private ACL for security
			S3KMSKeyID:                 "",        // Empty by default, set when using SSE-KMS
			InternalProxySkipTLSVerify: false,     // Default to strict TLS verification
		},
		MetadataStore: MetadataStoreConfig{
			Type:           "postgres",
			DSN:            "postgres://callfs:callfs@localhost/callfs?sslmode=disable",
			SQLitePath:     "./callfs.sqlite3",
			RedisAddr:      "localhost:6379",
			RedisPassword:  "",
			RedisDB:        0,
			RedisKeyPrefix: "callfs:",
		},
		Raft: RaftConfig{
			Enabled:             false,
			NodeID:              "callfs-node-1",
			BindAddr:            "127.0.0.1:7000",
			DataDir:             "./raft",
			Bootstrap:           false,
			Peers:               make(map[string]string),
			APIPeerEndpoints:    make(map[string]string),
			ApplyTimeout:        10 * time.Second,
			ForwardTimeout:      10 * time.Second,
			SnapshotInterval:    60 * time.Second,
			SnapshotThreshold:   256,
			RetainSnapshotCount: 2,
		},
		DLM: DLMConfig{
			Type:          "redis",
			RedisAddr:     "localhost:6379",
			RedisPassword: "",
		},
		HA: HAConfig{
			ReplicationEnabled:    false,
			ReplicaBackend:        "",
			RequireReplicaSuccess: false,
		},
		InstanceDiscovery: InstanceDiscoveryConfig{
			InstanceID:    "callfs-instance-1",
			PeerEndpoints: make(map[string]string),
		},
	}
}
