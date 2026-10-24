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
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			TransferTimeout:   time.Hour,
			FileOpTimeout:     10 * time.Second,
			MetadataOpTimeout: 5 * time.Second,
			MetadataCacheTTL:  2 * time.Second,
			MetadataCacheSize: 1000,
			RateLimit: RateLimitConfig{
				LinkGenerateRPS:       100,
				LinkGenerateBurst:     1,
				MaxActiveLinksPerUser: 1000,
				DownloadRPS:           10,
				DownloadBurst:         5,
				SessionRPS:            1,
				SessionBurst:          5,
			},
		},
		Auth: AuthConfig{
			// Deliberately empty: validateAuthConfig requires the operator to
			// supply keys via api_keys or api_key_users. Pre-seeding a placeholder
			// here would make a config that sets only api_key_users fail the
			// "must not use default value" check on a key it never asked for.
			APIKeys:             nil,
			APIKeyUsers:         nil,
			InternalProxySecret: "change-me-internal-secret",
			SingleUseLinkSecret: "change-me-link-secret",
			AdminUsers:          []string{},
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
			LocalFSSync:                "full",
			LocalFSAck:                 "disk",
			FlushMaxPendingBytes:       256 << 20,
			FlushMaxDelay:              0,
			S3AccessKey:                "",
			S3SecretKey:                "",
			S3Region:                   "us-east-1",
			S3BucketName:               "",
			S3ServerSideEncryption:     "AES256",  // Default to AES256 for security
			S3ACL:                      "private", // Default to private ACL for security
			S3KMSKeyID:                 "",        // Empty by default, set when using SSE-KMS
			InternalProxySkipTLSVerify: false,     // Default to strict TLS verification
		},
		Erasure: ErasureConfig{
			DataShards:   4,
			ParityShards: 2,
		},
		MetadataStore: MetadataStoreConfig{
			Type:           "postgres",
			DSN:            "",
			SQLitePath:     "./callfs.sqlite3",
			RedisAddr:      "localhost:6379",
			RedisPassword:  "",
			RedisDB:        0,
			RedisKeyPrefix: "callfs:",
			Postgres: PostgresConfig{
				MaxOpenConns:    25,
				MaxIdleConns:    10,
				ConnMaxLifetime: 30 * time.Minute,
			},
		},
		DLM: DLMConfig{
			Type:              "redis",
			RedisAddr:         "localhost:6379",
			RedisPassword:     "",
			WaitTimeout:       30 * time.Second,
			RedisPoolSize:     10,
			RedisMinIdleConns: 5,
		},
		HA: HAConfig{
			ReplicationEnabled:    false,
			ReplicaBackend:        "",
			RequireReplicaSuccess: false,
		},
		InstanceDiscovery: InstanceDiscoveryConfig{
			InstanceID:    "",
			PeerEndpoints: make(map[string]string),
		},
		UI: UIConfig{
			Enabled:               true,
			SessionTTL:            8 * time.Hour,
			IdleTimeout:           time.Hour,
			AllowRemoteRestart:    false,
			MetricsSampleInterval: 10 * time.Second,
			MetricsRetention:      24 * time.Hour,
			LogBufferEntries:      5000,
			AuditBufferEntries:    20000,
			EventBufferEntries:    5000,
		},
		Performance: PerformanceConfig{Mode: PerformanceFast},
		Trash: TrashConfig{
			Enabled:   true,
			Retention: 720 * time.Hour,
		},
		NATS: NATSConfig{
			Enabled:         false,
			Mode:            NATSModeEmbedded,
			ServerName:      "",
			ClusterName:     "callfs",
			ClientListen:    "",
			ClusterListen:   "127.0.0.1:6222",
			Routes:          []string{},
			RoutesFromPeers: true,
			MonitorListen:   "",
			ServerTags:      make(map[string]string),
			Auth: NATSAuthConfig{
				Mode: NATSAuthDerived,
			},
			JetStream: NATSJetStreamConfig{
				StoreDir:     "./data/nats",
				MaxMemory:    256 * MiB,
				MaxStore:     0,
				Sync:         NATSSyncAlways,
				SyncInterval: 2 * time.Minute,
				UniqueTag:    "",
			},
			External: NATSExternalConfig{
				URLs: []string{},
			},
			RPCTimeout: 10 * time.Second,
			Metadata: NATSMetadataConfig{
				Replicas:         3,
				ApplyTimeout:     10 * time.Second,
				ReadBarrier:      NATSReadBarrierInfo,
				MissBarrier:      false,
				SnapshotInterval: 5 * time.Minute,
				SnapshotEvery:    50000,
				RetainCommands:   100000,
				RetainSnapshots:  3,
				MaxMsgSize:       8 * MiB,
			},
			Locks: NATSLocksConfig{
				Replicas:   3,
				TTL:        15 * time.Second,
				QueueStale: 3 * time.Second,
			},
			Streams: NATSStreamsConfig{
				Replicas:        3,
				EventsMaxAge:    168 * time.Hour,
				EventsMaxBytes:  GiB,
				AuditMaxAge:     2160 * time.Hour,
				LogsLevel:       "warn",
				LogsMaxAge:      72 * time.Hour,
				TasksMaxDeliver: 8,
				TasksBackoff:    []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute},
				DLQMaxAge:       720 * time.Hour,
			},
			Bytes:    NATSBytesHTTP,
			Counters: false,
		},
	}
}
