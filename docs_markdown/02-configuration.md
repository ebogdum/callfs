# Configuration Reference

This document provides a comprehensive reference for all CallFS configuration options, including YAML configuration files, environment variables, and command-line parameters.

## Configuration Hierarchy

CallFS loads configuration from multiple sources in the following priority order:

1. **Environment Variables** (highest priority)
2. **Configuration Files** (medium priority)
3. **Default Values** (lowest priority)

## Configuration Files

CallFS automatically searches for configuration files in the current directory:
- `config.yaml` (preferred)
- `config.yml`
- `config.json`

### Complete Configuration Example

```yaml
# Server configuration
server:
  listen_addr: ":8443"                    # Server bind address
  external_url: "https://localhost:8443"  # External URL for link generation
  cert_file: "server.crt"                 # TLS certificate file path
  key_file: "server.key"                  # TLS private key file path
  read_timeout: "30s"                     # HTTP read timeout
  write_timeout: "30s"                    # HTTP write timeout
  file_op_timeout: "10s"                  # File operation timeout
  metadata_op_timeout: "5s"               # Metadata operation timeout

# Authentication and authorization
auth:
  api_keys:                               # List of valid API keys
    - "api-key-1"
    - "api-key-2"
  internal_proxy_secret: "internal-secret" # Secret for internal proxy auth
  single_use_link_secret: "link-secret"   # Secret for single-use links

# Logging configuration
log:
  level: "info"                           # Log level: debug, info, warn, error
  format: "json"                          # Log format: json, console

# Metrics configuration
metrics:
  listen_addr: ":9090"                    # Metrics server address

# Backend storage configuration
backend:
  default_backend: "localfs"              # Default backend for new files: localfs or s3
  
  # Local filesystem backend
  localfs_root_path: "/var/lib/callfs"    # Local filesystem root path
  
  # S3 backend configuration
  s3_access_key: ""                       # AWS S3 access key
  s3_secret_key: ""                       # AWS S3 secret key
  s3_region: "us-east-1"                  # AWS S3 region
  s3_bucket_name: ""                      # S3 bucket name
  s3_endpoint: ""                         # Custom S3 endpoint (for MinIO)
  s3_server_side_encryption: "AES256"     # Server-side encryption
  s3_acl: "private"                       # Object ACL
  s3_kms_key_id: ""                       # KMS key ID for SSE-KMS
  
  # Internal proxy configuration
  internal_proxy_skip_tls_verify: false   # Skip TLS verification for internal proxy

# Metadata store configuration
metadata_store:
  dsn: "postgres://callfs:password@localhost/callfs?sslmode=disable"

# Distributed lock manager configuration
dlm:
  redis_addr: "localhost:6379"            # Redis server address
  redis_password: ""                      # Redis password

# Instance discovery for clustering
instance_discovery:
  instance_id: "callfs-instance-1"        # Unique instance identifier
  peer_endpoints:                         # Map of peer endpoints
    instance-2: "https://peer2:8443"
    instance-3: "https://peer3:8443"
```

## Environment Variables

All configuration options can be overridden using environment variables with the `CALLFS_` prefix. Nested configuration keys use dot notation with underscores.

### Server Configuration

| Environment Variable | YAML Path | Default | Description |
|---------------------|-----------|---------|-------------|
| `CALLFS_SERVER_LISTEN_ADDR` | `server.listen_addr` | `:8443` | Server bind address |
| `CALLFS_SERVER_EXTERNAL_URL` | `server.external_url` | `localhost:8443` | External URL for link generation |
| `CALLFS_SERVER_CERT_FILE` | `server.cert_file` | `server.crt` | TLS certificate file path |
| `CALLFS_SERVER_KEY_FILE` | `server.key_file` | `server.key` | TLS private key file path |
| `CALLFS_SERVER_READ_TIMEOUT` | `server.read_timeout` | `30s` | HTTP read timeout |
| `CALLFS_SERVER_WRITE_TIMEOUT` | `server.write_timeout` | `30s` | HTTP write timeout |
| `CALLFS_SERVER_FILE_OP_TIMEOUT` | `server.file_op_timeout` | `10s` | File operation timeout |
| `CALLFS_SERVER_METADATA_OP_TIMEOUT` | `server.metadata_op_timeout` | `5s` | Metadata operation timeout |

### Authentication Configuration

| Environment Variable | YAML Path | Default | Description |
|---------------------|-----------|---------|-------------|
| `CALLFS_AUTH_API_KEYS` | `auth.api_keys` | `["default-api-key"]` | Comma-separated list of API keys |
| `CALLFS_AUTH_INTERNAL_PROXY_SECRET` | `auth.internal_proxy_secret` | `change-me-internal-secret` | Internal proxy authentication secret |
| `CALLFS_AUTH_SINGLE_USE_LINK_SECRET` | `auth.single_use_link_secret` | `change-me-link-secret` | Single-use link generation secret |

### Logging Configuration

| Environment Variable | YAML Path | Default | Description |
|---------------------|-----------|---------|-------------|
| `CALLFS_LOG_LEVEL` | `log.level` | `info` | Log level (debug, info, warn, error) |
| `CALLFS_LOG_FORMAT` | `log.format` | `json` | Log format (json, console) |

### Metrics Configuration

| Environment Variable | YAML Path | Default | Description |
|---------------------|-----------|---------|-------------|
| `CALLFS_METRICS_LISTEN_ADDR` | `metrics.listen_addr` | `:9090` | Metrics server bind address |

### Backend Configuration

| Environment Variable | YAML Path | Default | Description |
|---------------------|-----------|---------|-------------|
| `CALLFS_BACKEND_DEFAULT_BACKEND` | `backend.default_backend` | `localfs` | Default backend for new files |
| `CALLFS_BACKEND_LOCALFS_ROOT_PATH` | `backend.localfs_root_path` | `/var/lib/callfs` | Local filesystem root path |
| `CALLFS_BACKEND_S3_ACCESS_KEY` | `backend.s3_access_key` | `""` | AWS S3 access key |
| `CALLFS_BACKEND_S3_SECRET_KEY` | `backend.s3_secret_key` | `""` | AWS S3 secret key |
| `CALLFS_BACKEND_S3_REGION` | `backend.s3_region` | `us-east-1` | AWS S3 region |
| `CALLFS_BACKEND_S3_BUCKET_NAME` | `backend.s3_bucket_name` | `""` | S3 bucket name |
| `CALLFS_BACKEND_S3_ENDPOINT` | `backend.s3_endpoint` | `""` | Custom S3 endpoint |
| `CALLFS_BACKEND_S3_SERVER_SIDE_ENCRYPTION` | `backend.s3_server_side_encryption` | `AES256` | S3 server-side encryption |
| `CALLFS_BACKEND_S3_ACL` | `backend.s3_acl` | `private` | S3 object ACL |
| `CALLFS_BACKEND_S3_KMS_KEY_ID` | `backend.s3_kms_key_id` | `""` | KMS key ID for SSE-KMS |
| `CALLFS_BACKEND_INTERNAL_PROXY_SKIP_TLS_VERIFY` | `backend.internal_proxy_skip_tls_verify` | `false` | Skip TLS verification for internal proxy |

### Metadata Store Configuration

| Environment Variable | YAML Path | Default | Description |
|---------------------|-----------|---------|-------------|
| `CALLFS_METADATA_STORE_DSN` | `metadata_store.dsn` | `postgres://callfs:callfs@localhost/callfs?sslmode=disable` | PostgreSQL connection string |

### Distributed Lock Manager Configuration

| Environment Variable | YAML Path | Default | Description |
|---------------------|-----------|---------|-------------|
| `CALLFS_DLM_REDIS_ADDR` | `dlm.redis_addr` | `localhost:6379` | Redis server address |
| `CALLFS_DLM_REDIS_PASSWORD` | `dlm.redis_password` | `""` | Redis password |

### Instance Discovery Configuration

| Environment Variable | YAML Path | Default | Description |
|---------------------|-----------|---------|-------------|
| `CALLFS_INSTANCE_DISCOVERY_INSTANCE_ID` | `instance_discovery.instance_id` | `callfs-instance-1` | Unique instance identifier |
| `CALLFS_INSTANCE_DISCOVERY_PEER_ENDPOINTS` | `instance_discovery.peer_endpoints` | `{}` | JSON map of peer endpoints |

## Configuration Validation

CallFS performs comprehensive configuration validation at startup. The following fields are required:

### Required Configuration

- `server.listen_addr` - Must be a valid bind address
- `metadata_store.dsn` - Must be a valid PostgreSQL connection string
- `instance_discovery.instance_id` - Must be a non-empty unique identifier
- `auth.api_keys` - Must contain at least one valid API key
- `auth.internal_proxy_secret` - Must be set and not use default value
- `auth.single_use_link_secret` - Must be set and not use default value

### Configuration Examples by Environment

#### Development Environment

```yaml
server:
  listen_addr: ":8443"
  external_url: "https://localhost:8443"
  cert_file: "certs/server.crt"
  key_file: "certs/server.key"

auth:
  api_keys:
    - "dev-api-key-123"
  internal_proxy_secret: "dev-internal-secret"
  single_use_link_secret: "dev-link-secret"

log:
  level: "debug"
  format: "console"

backend:
  default_backend: "localfs"
  localfs_root_path: "./data"

metadata_store:
  dsn: "postgres://callfs:callfs@localhost/callfs_dev?sslmode=disable"

dlm:
  redis_addr: "localhost:6379"
```

#### Production Environment

```yaml
server:
  listen_addr: ":8443"
  external_url: "https://callfs.yourdomain.com:8443"
  cert_file: "/etc/callfs/certs/fullchain.pem"
  key_file: "/etc/callfs/certs/privkey.pem"
  read_timeout: "60s"
  write_timeout: "60s"

auth:
  api_keys:
    - "prod-api-key-1"
    - "prod-api-key-2"
  internal_proxy_secret: "secure-internal-secret-production"
  single_use_link_secret: "secure-link-secret-production"

log:
  level: "info"
  format: "json"

backend:
  default_backend: "s3"
  localfs_root_path: "/var/lib/callfs"
  s3_bucket_name: "callfs-production"
  s3_region: "us-west-2"
  s3_server_side_encryption: "aws:kms"
  s3_kms_key_id: "arn:aws:kms:us-west-2:123456789:key/12345678-1234-1234-1234-123456789012"

metadata_store:
  dsn: "postgres://callfs:secure_password@db.internal:5432/callfs?sslmode=require"

dlm:
  redis_addr: "redis.internal:6379"
  redis_password: "secure_redis_password"

instance_discovery:
  instance_id: "callfs-prod-01"
  peer_endpoints:
    callfs-prod-02: "https://callfs-02.internal:8443"
    callfs-prod-03: "https://callfs-03.internal:8443"
```

#### Clustered Environment

```yaml
server:
  listen_addr: ":8443"
  external_url: "https://callfs-cluster.yourdomain.com:8443"

auth:
  api_keys:
    - "cluster-api-key-1"
  internal_proxy_secret: "cluster-internal-secret"
  single_use_link_secret: "cluster-link-secret"

backend:
  default_backend: "s3"
  s3_bucket_name: "callfs-cluster-storage"
  s3_region: "us-east-1"

metadata_store:
  dsn: "postgres://callfs:password@postgres-cluster:5432/callfs?sslmode=require"

dlm:
  redis_addr: "redis-cluster:6379"
  redis_password: "cluster_redis_password"

instance_discovery:
  instance_id: "callfs-node-1"
  peer_endpoints:
    callfs-node-2: "https://callfs-node-2:8443"
    callfs-node-3: "https://callfs-node-3:8443"
    callfs-node-4: "https://callfs-node-4:8443"
```

## Advanced Configuration

### Database Connection String Options

The `metadata_store.dsn` supports various PostgreSQL connection parameters:

```
postgres://username:password@host:port/database?param1=value1&param2=value2
```

Common parameters:
- `sslmode` - SSL mode (disable, require, verify-ca, verify-full)
- `connect_timeout` - Connection timeout in seconds
- `statement_timeout` - Statement timeout in milliseconds
- `lock_timeout` - Lock timeout in milliseconds
- `application_name` - Application name for logging

Example:
```yaml
metadata_store:
  dsn: "postgres://callfs:password@localhost:5432/callfs?sslmode=require&connect_timeout=10&application_name=callfs"
```

### Redis Configuration Options

For clustered Redis setups, use Redis Cluster or Sentinel configurations:

```yaml
dlm:
  redis_addr: "redis-cluster-node1:7000,redis-cluster-node2:7000,redis-cluster-node3:7000"
  redis_password: "cluster_password"
```

### S3 Endpoint Configuration

For S3-compatible storage providers:

```yaml
backend:
  s3_endpoint: "https://minio.yourdomain.com"
  s3_region: "us-east-1"
  s3_bucket_name: "callfs-storage"
  s3_access_key: "minio_access_key"
  s3_secret_key: "minio_secret_key"
```

### Peer Endpoints Configuration

For distributed deployments, configure peer endpoints:

```yaml
instance_discovery:
  instance_id: "callfs-us-west-1a"
  peer_endpoints:
    callfs-us-west-1b: "https://callfs-us-west-1b.internal:8443"
    callfs-us-east-1a: "https://callfs-us-east-1a.internal:8443"
    callfs-eu-west-1a: "https://callfs-eu-west-1a.internal:8443"
```

## Configuration Validation Command

Use the validation command to check your configuration:

```bash
# Validate current configuration
./callfs config validate

# Example output:
# ✅ Configuration is valid
# Instance ID: callfs-instance-1
# Listen Address: :8443
# Metadata Store DSN: postgres://callfs:***@localhost/callfs
# Redis Address: localhost:6379
# Local FS Root: /var/lib/callfs
# S3 Bucket: my-callfs-bucket
# S3 Region: us-west-2
```

## Environment-Specific Configurations

### Docker Environment Variables

```dockerfile
ENV CALLFS_SERVER_LISTEN_ADDR=":8443"
ENV CALLFS_SERVER_EXTERNAL_URL="https://callfs.yourdomain.com:8443"
ENV CALLFS_AUTH_API_KEYS="docker-api-key-1,docker-api-key-2"
ENV CALLFS_METADATA_STORE_DSN="postgres://callfs:password@postgres:5432/callfs?sslmode=disable"
ENV CALLFS_DLM_REDIS_ADDR="redis:6379"
ENV CALLFS_BACKEND_LOCALFS_ROOT_PATH="/data"
```

### Kubernetes ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: callfs-config
data:
  config.yaml: |
    server:
      listen_addr: ":8443"
      external_url: "https://callfs.k8s.local:8443"
    auth:
      api_keys:
        - "k8s-api-key-1"
      internal_proxy_secret: "k8s-internal-secret"
      single_use_link_secret: "k8s-link-secret"
    metadata_store:
      dsn: "postgres://callfs:password@postgres-service:5432/callfs?sslmode=require"
    dlm:
      redis_addr: "redis-service:6379"
    backend:
      s3_bucket_name: "callfs-k8s-storage"
      s3_region: "us-west-2"
```

### Systemd Environment File

```bash
# /etc/callfs/environment
CALLFS_SERVER_LISTEN_ADDR=":8443"
CALLFS_SERVER_EXTERNAL_URL="https://callfs.yourdomain.com:8443"
CALLFS_AUTH_API_KEYS="system-api-key-1,system-api-key-2"
CALLFS_METADATA_STORE_DSN="postgres://callfs:password@localhost:5432/callfs?sslmode=require"
CALLFS_DLM_REDIS_ADDR="localhost:6379"
CALLFS_DLM_REDIS_PASSWORD="redis_password"
CALLFS_BACKEND_LOCALFS_ROOT_PATH="/var/lib/callfs"
CALLFS_LOG_LEVEL="info"
CALLFS_LOG_FORMAT="json"
```

## Best Practices

### Security
1. **Never use default secrets in production**
2. **Store sensitive configuration in environment variables or secret management systems**
3. **Use strong, randomly generated API keys**
4. **Enable TLS/SSL for all database connections**
5. **Regularly rotate secrets and API keys**

### Performance
1. **Adjust timeouts based on your storage backend performance**
2. **Configure appropriate connection pooling for database and Redis**
3. **Use local filesystem for hot data, S3 for cold storage**
4. **Monitor metrics to optimize configuration**

### Reliability
1. **Use clustered databases and Redis in production**
2. **Configure multiple peer endpoints for redundancy**
3. **Set up proper health checks and monitoring**
4. **Use persistent storage for local filesystem backend**

### Monitoring
1. **Enable structured logging (JSON format)**
2. **Configure metrics collection**
3. **Set up log aggregation and analysis**
4. **Monitor configuration validation errors**
