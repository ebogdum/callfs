# CallFS - Ultra-lightweight REST API Filesystem

CallFS is an ultra-lightweight, high-performance REST API filesystem that provides precise Linux filesystem semantics over various backends including local filesystem, Amazon S3, and distributed peer networks.

## 🚀 Quick Start

```bash
# Start CallFS server with default configuration
./callfs server

# Start with custom config file
./callfs server --config /path/to/config.yaml

# Validate configuration and display settings
./callfs config validate

# Show help
./callfs --help
```

## 📋 Command Line Flags and Options

### Main Commands

#### `callfs server`
Starts the CallFS server with configured backends and API endpoints.

**Usage:**
```bash
callfs server [--config|-c /path/to/config.yaml]
```

**Flags:**
- `--config`, `-c` - Path to configuration file (optional)

**Configuration Sources (in priority order):**
1. Environment variables (highest priority)
2. Configuration file (`config.yaml`, `config.yml`, or `config.json`)
3. Default values (lowest priority)

#### `callfs config validate`
Validates the CallFS configuration and displays loaded settings.

**Usage:**
```bash
callfs config validate
```

### Environment Variables

All configuration options can be set via environment variables with the `CALLFS_` prefix:

#### Server Configuration
- `CALLFS_SERVER_LISTEN_ADDR` - Server listen address (default: `:8443`)
- `CALLFS_SERVER_EXTERNAL_URL` - External URL for link generation (default: `localhost:8443`)
- `CALLFS_SERVER_CERT_FILE` - TLS certificate file path (default: `server.crt`)
- `CALLFS_SERVER_KEY_FILE` - TLS private key file path (default: `server.key`)
- `CALLFS_SERVER_READ_TIMEOUT` - HTTP read timeout (default: `30s`)
- `CALLFS_SERVER_WRITE_TIMEOUT` - HTTP write timeout (default: `30s`)
- `CALLFS_SERVER_FILE_OP_TIMEOUT` - File operation timeout (default: `10s`)
- `CALLFS_SERVER_METADATA_OP_TIMEOUT` - Metadata operation timeout (default: `5s`)

#### Authentication Configuration
- `CALLFS_AUTH_API_KEYS` - Comma-separated list of valid API keys (required)
- `CALLFS_AUTH_INTERNAL_PROXY_SECRET` - Secret for internal proxy authentication (required)
- `CALLFS_AUTH_SINGLE_USE_LINK_SECRET` - Secret for single-use link generation (required)

#### Logging Configuration
- `CALLFS_LOG_LEVEL` - Log level: `debug`, `info`, `warn`, `error` (default: `info`)
- `CALLFS_LOG_FORMAT` - Log format: `json`, `console` (default: `json`)

#### Metrics Configuration
- `CALLFS_METRICS_LISTEN_ADDR` - Metrics server address (default: `:9090`)

#### Backend Configuration
- `CALLFS_BACKEND_DEFAULT_BACKEND` - Default backend for new files: `localfs` or `s3` (default: `localfs`)
- `CALLFS_BACKEND_LOCALFS_ROOT_PATH` - Local filesystem root path (default: `/var/lib/callfs`)
- `CALLFS_BACKEND_S3_ACCESS_KEY` - AWS S3 access key
- `CALLFS_BACKEND_S3_SECRET_KEY` - AWS S3 secret key
- `CALLFS_BACKEND_S3_REGION` - AWS S3 region (default: `us-east-1`)
- `CALLFS_BACKEND_S3_BUCKET_NAME` - AWS S3 bucket name
- `CALLFS_BACKEND_S3_ENDPOINT` - Custom S3 endpoint (for MinIO, etc.)
- `CALLFS_BACKEND_S3_SERVER_SIDE_ENCRYPTION` - S3 server-side encryption: `AES256`, `aws:kms` (default: `AES256`)
- `CALLFS_BACKEND_S3_ACL` - S3 object ACL: `private`, `public-read`, etc. (default: `private`)
- `CALLFS_BACKEND_S3_KMS_KEY_ID` - KMS key ID for SSE-KMS encryption
- `CALLFS_BACKEND_INTERNAL_PROXY_SKIP_TLS_VERIFY` - Skip TLS verification for internal proxy requests (default: `false`)

#### Metadata Store Configuration
- `CALLFS_METADATA_STORE_DSN` - PostgreSQL connection string (required)

#### Distributed Lock Manager Configuration
- `CALLFS_DLM_REDIS_ADDR` - Redis server address (default: `localhost:6379`)
- `CALLFS_DLM_REDIS_PASSWORD` - Redis password

#### Instance Discovery Configuration
- `CALLFS_INSTANCE_DISCOVERY_INSTANCE_ID` - Unique instance identifier (default: `callfs-instance-1`)
- `CALLFS_INSTANCE_DISCOVERY_PEER_ENDPOINTS` - JSON map of peer endpoints for clustering

## 🏗️ Architecture

CallFS provides a REST API that abstracts filesystem operations across multiple storage backends:

- **LocalFS Backend**: Direct local filesystem access with full Unix semantics
- **S3 Backend**: Amazon S3 or S3-compatible storage (MinIO, etc.)
- **Internal Proxy Backend**: Distributed peer-to-peer file sharing with cross-server operation routing
- **NoOp Backend**: Placeholder for disabled backends

### Core Components

- **Engine**: Central orchestrator for file operations and backend selection
- **Metadata Store**: PostgreSQL-based metadata management with caching
- **Link Manager**: Secure single-use download link generation and validation
- **Lock Manager**: Redis-based distributed locking for concurrent operations
- **Metrics**: Prometheus-compatible metrics collection
- **Authentication**: API key-based authentication with Unix authorization

## 🔑 Key Features

- **Multi-Backend Support**: Local filesystem, S3, and distributed peer networks
- **Cross-Server Operations**: Automatic conflict detection and operation routing across servers
- **Enhanced REST API**: Standard HTTP methods with cross-server proxy support
- **Single-Use Links**: Secure, time-limited download links with HMAC validation
- **Distributed Locking**: Redis-based locking for concurrent operations across instances
- **Metadata Caching**: High-performance in-memory metadata operations with TTL
- **Authentication & Authorization**: API key-based authentication with Unix permission model
- **Unix Permissions**: Full Unix filesystem semantics and permission enforcement
- **Monitoring**: Comprehensive Prometheus metrics and structured logging
- **TLS Security**: HTTPS-only with comprehensive security headers and middleware

## 📊 Monitoring

CallFS exposes Prometheus metrics at `/metrics` endpoint:

- **HTTP Request Metrics**: Duration, status codes, request paths, method-specific timing
- **Backend Operation Metrics**: Duration and operation counts by backend type
- **Metadata Database Metrics**: Query performance and operation counts
- **Single-Use Link Metrics**: Generation/consumption rates and status tracking
- **Distributed Lock Metrics**: Lock acquisition/release duration and success rates
- **Active Locks Gauge**: Real-time count of active distributed locks
- **Cross-Server Metrics**: Proxy operation success rates and routing statistics

## 🔗 API Endpoints

### File Operations
- `GET /v1/files/{path}` - Download file or list directory
- `HEAD /v1/files/{path}` - Get file metadata with cross-server routing
- `POST /v1/files/{path}` - Create file or directory with conflict detection
- `PUT /v1/files/{path}` - Update file content with cross-server proxy support
- `DELETE /v1/files/{path}` - Delete file or directory with cross-server routing

### Directory Listing API
- `GET /v1/directories/{path}` - List directory contents with metadata
- `GET /v1/directories/{path}?recursive=true` - Recursive directory listing
- `GET /v1/directories/{path}?recursive=true&max_depth=N` - Depth-limited recursive listing

### Single-Use Links
- `POST /v1/links/generate` - Generate single-use download link with rate limiting
- `GET /download/{token}` - Download file via single-use link (no auth required)

### System Endpoints
- `GET /health` - Health check (no authentication required)
- `GET /metrics` - Prometheus metrics (no authentication required)

## 🔧 Configuration File

Create a `config.yaml` file for persistent configuration:

```yaml
server:
  listen_addr: ":8443"
  external_url: "https://your-domain.com:8443"
  cert_file: "/path/to/cert.pem"
  key_file: "/path/to/key.pem"

auth:
  api_keys:
    - "your-secure-api-key-1"
    - "your-secure-api-key-2"
  internal_proxy_secret: "your-internal-secret"
  single_use_link_secret: "your-link-secret"

backend:
  default_backend: "localfs"  # Default backend for new files
  localfs_root_path: "/var/lib/callfs"
  s3_bucket_name: "your-s3-bucket"
  s3_region: "us-west-2"
  s3_access_key: "your-access-key"
  s3_secret_key: "your-secret-key"
  s3_endpoint: "https://s3.amazonaws.com"  # Custom for MinIO
  s3_server_side_encryption: "AES256"
  s3_acl: "private"
  internal_proxy_skip_tls_verify: false

metadata_store:
  dsn: "postgres://user:pass@localhost/callfs?sslmode=require"

dlm:
  redis_addr: "localhost:6379"
  redis_password: "your-redis-password"

instance_discovery:
  instance_id: "callfs-instance-1"
  peer_endpoints:
    "callfs-instance-2": "https://peer2.example.com:8443"
    "callfs-instance-3": "https://peer3.example.com:8443"

log:
  level: "info"
  format: "json"
```

## 🚀 Example Usage

```bash
# Upload a file
curl -X PUT -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @localfile.txt \
  https://localhost:8443/v1/files/documents/myfile.txt

# Download a file
curl -H "Authorization: Bearer your-api-key" \
  https://localhost:8443/v1/files/documents/myfile.txt

# List directory with enhanced API
curl -H "Authorization: Bearer your-api-key" \
  https://localhost:8443/v1/directories/documents/

# Recursive directory listing
curl -H "Authorization: Bearer your-api-key" \
  "https://localhost:8443/v1/directories/documents/?recursive=true&max_depth=3"

# Generate single-use link
curl -X POST -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"path":"/documents/myfile.txt","expiry_seconds":3600}' \
  https://localhost:8443/v1/links/generate

# Cross-server file operations (automatic conflict detection)
curl -X POST -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"type":"file"}' \
  https://localhost:8443/v1/files/shared/newfile.txt
```

## 📖 Documentation

Comprehensive documentation is available in the `docs_markdown/` directory:

- [Installation Guide](docs_markdown/01-installation.md)
- [Configuration Reference](docs_markdown/02-configuration.md)
- [API Reference](docs_markdown/03-api-reference.md)
- [Authentication & Security](docs_markdown/04-authentication-security.md)
- [Backend Configuration](docs_markdown/05-backend-configuration.md)
- [Monitoring & Metrics](docs_markdown/06-monitoring-metrics.md)
- [Clustering & Distribution](docs_markdown/07-clustering-distribution.md)
- [Developer Guide](docs_markdown/08-developer-guide.md)
- [Enhanced Cross-Server Operations](docs_markdown/10-enhanced-cross-server.md)
- [Troubleshooting](docs_markdown/09-troubleshooting.md)

## ⚡ Performance Features

- **Zero-Copy I/O**: Efficient streaming with `io.Reader`/`io.Writer` interfaces
- **Connection Pooling**: Optimized database and HTTP client connections
- **Metadata Caching**: In-memory cache with configurable TTL for hot paths (5min TTL, 1000 entries)
- **Concurrent Operations**: Safe concurrent file operations with distributed locking
- **Streaming Uploads/Downloads**: No memory buffering for large files
- **Background Processing**: Async cleanup workers for expired links and metadata
- **Smart Backend Selection**: Configurable default backend with automatic routing

## 🔒 Security Features

- **TLS/HTTPS Only**: All communications encrypted with configurable certificates
- **API Key Authentication**: Bearer token authentication with internal proxy secrets
- **Unix Permissions**: Full filesystem permission enforcement with user/group support
- **Security Headers**: Comprehensive HTTP security headers and middleware
- **Rate Limiting**: Configurable rate limiting for sensitive endpoints (link generation)
- **Single-Use Links**: Time-limited, cryptographically secure one-time download links
- **Cross-Server Security**: Secure internal proxy communication with TLS verification
- **Request Validation**: Path sanitization and input validation throughout the stack

## 📝 License

MIT License - see [LICENSE](LICENSE) file for details.

## 🤝 Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines.
