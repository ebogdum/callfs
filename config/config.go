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
	InstanceDiscovery InstanceDiscoveryConfig `koanf:"instance_discovery"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	ListenAddr        string        `koanf:"listen_addr"`
	ExternalURL       string        `koanf:"external_url"`
	CertFile          string        `koanf:"cert_file"`
	KeyFile           string        `koanf:"key_file"`
	ReadTimeout       time.Duration `koanf:"read_timeout"`
	WriteTimeout      time.Duration `koanf:"write_timeout"`
	FileOpTimeout     time.Duration `koanf:"file_op_timeout"`
	MetadataOpTimeout time.Duration `koanf:"metadata_op_timeout"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	APIKeys             []string `koanf:"api_keys"`
	InternalProxySecret string   `koanf:"internal_proxy_secret"`
	SingleUseLinkSecret string   `koanf:"single_use_link_secret"`
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string `koanf:"level"`
	Format string `koanf:"format"`
}

// MetricsConfig holds metrics server configuration
type MetricsConfig struct {
	ListenAddr string `koanf:"listen_addr"`
}

// BackendConfig holds backend storage configuration
type BackendConfig struct {
	DefaultBackend             string `koanf:"default_backend"` // Default backend for new files: "localfs" or "s3"
	LocalFSRootPath            string `koanf:"localfs_root_path"`
	S3AccessKey                string `koanf:"s3_access_key"`
	S3SecretKey                string `koanf:"s3_secret_key"`
	S3Region                   string `koanf:"s3_region"`
	S3BucketName               string `koanf:"s3_bucket_name"`
	S3Endpoint                 string `koanf:"s3_endpoint"`                    // Custom S3 endpoint (e.g., for MinIO)
	S3ServerSideEncryption     string `koanf:"s3_server_side_encryption"`      // SSE algorithm (AES256, aws:kms)
	S3ACL                      string `koanf:"s3_acl"`                         // Object ACL (private, public-read, etc.)
	S3KMSKeyID                 string `koanf:"s3_kms_key_id"`                  // KMS key ID for SSE-KMS
	InternalProxySkipTLSVerify bool   `koanf:"internal_proxy_skip_tls_verify"` // Skip TLS certificate verification for internal proxy requests
}

// MetadataStoreConfig holds metadata store configuration
type MetadataStoreConfig struct {
	DSN string `koanf:"dsn"`
}

// DLMConfig holds distributed lock manager configuration
type DLMConfig struct {
	RedisAddr     string `koanf:"redis_addr"`
	RedisPassword string `koanf:"redis_password"`
}

// InstanceDiscoveryConfig holds instance discovery configuration
type InstanceDiscoveryConfig struct {
	InstanceID    string            `koanf:"instance_id"`
	PeerEndpoints map[string]string `koanf:"peer_endpoints"`
}
