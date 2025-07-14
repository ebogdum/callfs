package config

import "time"

// DefaultAppConfig returns an AppConfig struct with sensible default values
func DefaultAppConfig() AppConfig {
	return AppConfig{
		Server: ServerConfig{
			ListenAddr:        ":8443",
			ExternalURL:       "localhost:8443",
			CertFile:          "server.crt",
			KeyFile:           "server.key",
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
			DSN: "postgres://callfs:callfs@localhost/callfs?sslmode=disable",
		},
		DLM: DLMConfig{
			RedisAddr:     "localhost:6379",
			RedisPassword: "",
		},
		InstanceDiscovery: InstanceDiscoveryConfig{
			InstanceID:    "callfs-instance-1",
			PeerEndpoints: make(map[string]string),
		},
	}
}
