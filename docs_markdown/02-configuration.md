# Configuration Reference

This document provides a comprehensive reference for all CallFS configuration options. CallFS can be configured via a YAML file, environment variables, or command-line flags, with environment variables taking the highest precedence.

## Configuration File

CallFS automatically looks for a `config.yaml` (or `.yml`, `.json`) file in the current directory. You can specify a different path using the `--config` flag.

### Complete Configuration Example

This example shows all available configuration options with typical values.

```yaml
# Server configuration
server:
  listen_addr: ":8443"
  protocol: "https" # "http", "https", or "auto"
  external_url: "https://callfs.example.com:8443"
  cert_file: "certs/server.crt"
  key_file: "certs/server.key"
  enable_quic: false
  quic_listen_addr: ":8443"
  read_timeout: 30s
  write_timeout: 30s
  file_op_timeout: 10s
  metadata_op_timeout: 5s

# Authentication and authorization
auth:
  api_keys:
    - "your-secure-api-key-1"
  internal_proxy_secret: "a-strong-secret-for-internal-traffic"
  single_use_link_secret: "another-strong-secret-for-links"

# Logging configuration
log:
  level: "info" # "debug", "info", "warn", or "error"
  format: "json" # "json" or "console"

# Metrics configuration
metrics:
  listen_addr: ":9090"

# Backend storage configuration
backend:
  default_backend: "localfs" # "localfs" or "s3"
  localfs_root_path: "/var/lib/callfs"
  
  s3:
    access_key: "YOUR_S3_ACCESS_KEY"
    secret_key: "YOUR_S3_SECRET_KEY"
    region: "us-east-1"
    bucket_name: "your-callfs-bucket"
    endpoint: "" # Optional: for S3-compatible services like MinIO
    server_side_encryption: "AES256"
    acl: "private"
    kms_key_id: "" # Optional: for SSE-KMS
  
  internal_proxy_skip_tls_verify: false

# Metadata store (postgres, sqlite, redis, or raft)
metadata_store:
  type: "postgres" # "postgres", "sqlite", "redis", or "raft"
  dsn: "postgres://callfs_user:your_password@localhost/callfs?sslmode=require"
  sqlite_path: "./callfs.sqlite3"
  redis_addr: "localhost:6379"
  redis_password: ""
  redis_db: 0
  redis_key_prefix: "callfs:"

# Raft metadata consensus (required when metadata_store.type=raft)
raft:
  enabled: true
  node_id: "callfs-node-1"
  bind_addr: "10.0.0.1:7000"
  data_dir: "/var/lib/callfs/raft"
  bootstrap: false # true only on initial bootstrap node
  peers:
    "callfs-node-2": "10.0.0.2:7000"
    "callfs-node-3": "10.0.0.3:7000"
  api_peer_endpoints:
    "callfs-node-1": "http://10.0.0.1:8443"
    "callfs-node-2": "http://10.0.0.2:8443"
    "callfs-node-3": "http://10.0.0.3:8443"
  apply_timeout: "10s"
  forward_timeout: "10s"
  snapshot_interval: "60s"
  snapshot_threshold: 256
  retain_snapshot_count: 2

# Distributed Lock Manager (Redis)
dlm:
  type: "redis" # "redis" or "local"
  redis_addr: "localhost:6379"
  redis_password: "your-redis-password"

# High availability content replication
ha:
  replication_enabled: false
  replica_backend: "s3" # "localfs" or "s3"
  require_replica_success: false

# Instance discovery for clustering
instance_discovery:
  instance_id: "callfs-node-1"
  peer_endpoints:
    "callfs-node-2": "https://callfs-node-2.internal:8443"
    "callfs-node-3": "https://callfs-node-3.internal:8443"
```

## Environment Variables

All YAML configuration keys can be set using environment variables. The format is `CALLFS_SECTION_KEY`. For nested keys, use an underscore (`_`).

| Environment Variable                          | YAML Path                                | Default Value         |
| --------------------------------------------- | ---------------------------------------- | --------------------- |
| `CALLFS_SERVER_LISTEN_ADDR`                   | `server.listen_addr`                     | `:8443`               |
| `CALLFS_SERVER_PROTOCOL`                      | `server.protocol`                        | `https`               |
| `CALLFS_SERVER_EXTERNAL_URL`                  | `server.external_url`                    | `localhost:8443`      |
| `CALLFS_SERVER_ENABLE_QUIC`                   | `server.enable_quic`                     | `false`               |
| `CALLFS_SERVER_QUIC_LISTEN_ADDR`              | `server.quic_listen_addr`                | `:8443`               |
| `CALLFS_AUTH_API_KEYS`                        | `auth.api_keys`                          | (none)                |
| `CALLFS_AUTH_INTERNAL_PROXY_SECRET`           | `auth.internal_proxy_secret`             | (none)                |
| `CALLFS_AUTH_SINGLE_USE_LINK_SECRET`          | `auth.single_use_link_secret`            | (none)                |
| `CALLFS_LOG_LEVEL`                            | `log.level`                              | `info`                |
| `CALLFS_LOG_FORMAT`                           | `log.format`                             | `json`                |
| `CALLFS_BACKEND_DEFAULT_BACKEND`              | `backend.default_backend`                | `localfs`             |
| `CALLFS_BACKEND_LOCALFS_ROOT_PATH`            | `backend.localfs_root_path`              | `/var/lib/callfs`     |
| `CALLFS_BACKEND_S3_ACCESS_KEY`                | `backend.s3.access_key`                  | (none)                |
| `CALLFS_BACKEND_S3_SECRET_KEY`                | `backend.s3.secret_key`                  | (none)                |
| `CALLFS_BACKEND_S3_REGION`                    | `backend.s3.region`                      | `us-east-1`           |
| `CALLFS_BACKEND_S3_BUCKET_NAME`               | `backend.s3.bucket_name`                 | (none)                |
| `CALLFS_METADATA_STORE_DSN`                   | `metadata_store.dsn`                     | (none)                |
| `CALLFS_METADATA_STORE_TYPE`                  | `metadata_store.type`                    | `postgres`            |
| `CALLFS_METADATA_STORE_SQLITE_PATH`           | `metadata_store.sqlite_path`             | `./callfs.sqlite3`    |
| `CALLFS_METADATA_STORE_REDIS_ADDR`            | `metadata_store.redis_addr`              | `localhost:6379`      |
| `CALLFS_METADATA_STORE_REDIS_PASSWORD`        | `metadata_store.redis_password`          | (none)                |
| `CALLFS_METADATA_STORE_REDIS_DB`              | `metadata_store.redis_db`                | `0`                   |
| `CALLFS_METADATA_STORE_REDIS_KEY_PREFIX`      | `metadata_store.redis_key_prefix`        | `callfs:`             |
| `CALLFS_RAFT_ENABLED`                         | `raft.enabled`                           | `false`               |
| `CALLFS_RAFT_NODE_ID`                         | `raft.node_id`                           | `callfs-node-1`       |
| `CALLFS_RAFT_BIND_ADDR`                       | `raft.bind_addr`                         | `127.0.0.1:7000`      |
| `CALLFS_RAFT_DATA_DIR`                        | `raft.data_dir`                          | `./raft`              |
| `CALLFS_RAFT_BOOTSTRAP`                       | `raft.bootstrap`                         | `false`               |
| `CALLFS_RAFT_PEERS`                           | `raft.peers`                             | (none)                |
| `CALLFS_RAFT_API_PEER_ENDPOINTS`              | `raft.api_peer_endpoints`                | (none)                |
| `CALLFS_RAFT_APPLY_TIMEOUT`                   | `raft.apply_timeout`                     | `10s`                 |
| `CALLFS_RAFT_FORWARD_TIMEOUT`                 | `raft.forward_timeout`                   | `10s`                 |
| `CALLFS_RAFT_SNAPSHOT_INTERVAL`               | `raft.snapshot_interval`                 | `60s`                 |
| `CALLFS_RAFT_SNAPSHOT_THRESHOLD`              | `raft.snapshot_threshold`                | `256`                 |
| `CALLFS_RAFT_RETAIN_SNAPSHOT_COUNT`           | `raft.retain_snapshot_count`             | `2`                   |
| `CALLFS_DLM_TYPE`                             | `dlm.type`                               | `redis`               |
| `CALLFS_DLM_REDIS_ADDR`                       | `dlm.redis_addr`                         | `localhost:6379`      |
| `CALLFS_DLM_REDIS_PASSWORD`                   | `dlm.redis_password`                     | (none)                |
| `CALLFS_HA_REPLICATION_ENABLED`               | `ha.replication_enabled`                 | `false`               |
| `CALLFS_HA_REPLICA_BACKEND`                   | `ha.replica_backend`                     | (none)                |
| `CALLFS_HA_REQUIRE_REPLICA_SUCCESS`           | `ha.require_replica_success`             | `false`               |
| `CALLFS_INSTANCE_DISCOVERY_INSTANCE_ID`       | `instance_discovery.instance_id`         | `callfs-instance-1`   |
| `CALLFS_INSTANCE_DISCOVERY_PEER_ENDPOINTS`    | `instance_discovery.peer_endpoints`      | (none)                |

**Note:** For `auth.api_keys`, provide a comma-separated string: `export CALLFS_AUTH_API_KEYS="key1,key2"`. For `instance_discovery.peer_endpoints`, provide a JSON string: `export CALLFS_INSTANCE_DISCOVERY_PEER_ENDPOINTS='{"node2":"https://node2.local:8443"}'`.

## Configuration Validation

CallFS validates the configuration on startup and will exit if critical values are missing or invalid. You can also validate your configuration manually.

**Command:**
```bash
./callfs config validate --config /path/to/config.yaml
```

## Cluster Join Command (Raft)

To add a new node to an existing Raft metadata cluster:

```bash
./callfs cluster join --config /path/to/config.yaml --leader http://raft-leader:8443
```

Override auto-detected values if needed:

```bash
./callfs cluster join \
  --leader http://raft-leader:8443 \
  --node-id callfs-node-4 \
  --raft-addr 10.0.0.14:7000 \
  --api-endpoint http://10.0.0.14:8443 \
  --internal-secret your-internal-secret
```

**Required Fields:**
- `server.listen_addr`
- `server.protocol` (`http`, `https`, or `auto`)
- `metadata_store.type`
- `instance_discovery.instance_id`
- `auth.api_keys` (must not be empty)
- `auth.internal_proxy_secret`
- `auth.single_use_link_secret`

Type-specific requirements:
- `metadata_store.type=postgres` requires `metadata_store.dsn`
- `metadata_store.type=sqlite` requires `metadata_store.sqlite_path`
- `metadata_store.type=redis` requires `metadata_store.redis_addr`
- `metadata_store.type=raft` requires `raft.node_id`, `raft.bind_addr`, `raft.data_dir`, and valid raft timing settings
- `dlm.type=redis` requires `dlm.redis_addr`

If `server.protocol=https` or `server.enable_quic=true`, both `server.cert_file` and `server.key_file` are required.

## Production Best Practices

- **Security**: Never use default secrets. Store secrets in environment variables or a dedicated secrets management tool (like HashiCorp Vault or AWS Secrets Manager).
- **Database**: Use a strong, unique password for the PostgreSQL user. Enable `sslmode=require` or higher for encrypted database connections.
- **Redis**: Secure your Redis instance with a password.
- **Logging**: Use `json` format in production and ship logs to a centralized logging platform (e.g., ELK Stack, Splunk, Grafana Loki).
- **Clustering**: In a multi-node setup, ensure `instance_id` is unique for each node and that `peer_endpoints` are correctly configured for internal communication.
- **Backups**: Regularly back up your PostgreSQL database and any data stored on the `localfs` backend.
