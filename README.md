# CallFS - Distributed REST API Filesystem in Go

**Open-source, high-performance file storage server with S3, local disk, and multi-node clustering over a simple REST API.**

CallFS turns any combination of local disks and S3-compatible object stores into a single, horizontally-scalable filesystem accessible via HTTP. Upload, download, list, delete, and share files across a cluster of nodes with automatic routing, erasure coding, and Unix-style permissions -- all through a clean REST API.

[![Go Report Card](https://goreportcard.com/badge/github.com/ebogdum/callfs)](https://goreportcard.com/report/github.com/ebogdum/callfs)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/github/go-mod/go-version/ebogdum/callfs)](https://go.dev/)

---

## Why CallFS?

Most file storage solutions force you to choose: local NAS simplicity, or cloud object-store scale. CallFS bridges the gap.

- **Replace fragile NFS mounts** with a REST API that works over any network
- **Add S3 as a storage tier** without rewriting your application
- **Scale horizontally** by adding nodes -- CallFS routes reads and writes to the right server automatically
- **Protect data with Reed-Solomon erasure coding** that distributes shards across nodes
- **Generate secure, expiring download links** for sharing files without exposing your API keys
- **Drop-in Docker deployment** or a single static binary for bare metal

If you're building a file management service, media pipeline, backup system, or any application that needs reliable distributed file storage over HTTP, CallFS gives you a production-ready foundation.

---

## Features

### Storage & Data
- **Multi-backend storage** -- local filesystem, Amazon S3, MinIO, and any S3-compatible object store
- **Reed-Solomon erasure coding** -- configurable data/parity shards distributed across nodes for fault tolerance
- **Streaming I/O** -- zero-copy file transfers without buffering entire files in memory
- **HA replication** -- optional synchronous or async replication between backends (configure via `ha` config section)

### Distributed Architecture
- **Horizontal scaling** -- add nodes and CallFS routes operations to the correct server
- **Cross-server file operations** -- create, update, delete, and move files across the cluster transparently
- **Raft consensus metadata** -- strongly consistent metadata coordination across nodes
- **Distributed locking** -- Redis-backed or local lock managers prevent concurrent write conflicts

### Security & Access Control
- **API key authentication** -- static bearer tokens with constant-time comparison
- **Unix-style permissions** -- UID/GID ownership and rwx permission bits on every file and directory
- **Single-use download links** -- time-limited, HMAC-signed tokens for secure file sharing
- **Rate limiting** -- configurable per-endpoint rate limits (link generation: 100 req/s, downloads: 10 req/s)
- **TLS / HTTPS / HTTP/3 (QUIC)** support out of the box

### Operations & Observability
- **Prometheus metrics** -- request latency histograms, operation counters, backend durations
- **Structured logging** -- JSON or console output with configurable log levels
- **Health endpoint** -- `/health` for load balancer and Kubernetes readiness probes
- **WebSocket file transfers** -- bidirectional streaming for large uploads and downloads

### Metadata Backends
Choose the metadata store that fits your deployment:

| Backend | Best for |
|---------|----------|
| **PostgreSQL** | Production clusters, ACID guarantees |
| **SQLite** | Single-node, edge, or embedded deployments |
| **Redis** | Low-latency metadata with existing Redis infrastructure |
| **Raft** | Fully self-contained clusters with no external dependencies |

---

## Quick Start

### Prerequisites

- **Go 1.24+** (or use a [prebuilt binary](builds/) or Docker)
- A metadata store: PostgreSQL, SQLite, Redis, or Raft (built-in)
- Optionally: Redis for distributed locking, S3 credentials for object storage

### Install

```bash
# From source
go build -o callfs ./cmd/main.go

# Or use Docker
docker build -t callfs .
```

### Configure

Copy the example config and edit it:

```bash
cp config.yaml.example config.yaml
```

Minimal `config.yaml` for local development:

```yaml
server:
  listen_addr: ":8443"
  protocol: "http"
  external_url: "http://localhost:8443"  # Used for single-use link URLs

auth:
  api_keys:
    - "my-secret-api-key-at-least-16-chars"
  internal_proxy_secret: "my-internal-proxy-secret-16ch"   # Must not use default value
  single_use_link_secret: "my-link-secret-at-least-16ch!"  # Must not use default value

backend:
  localfs_root_path: "./data"

metadata_store:
  type: "sqlite"
  sqlite_path: "./callfs.sqlite3"

dlm:
  type: "local"

log:
  level: "info"    # debug, info, warn, error
  format: "console" # json or console

instance_discovery:
  instance_id: "node-1"
```

> **Note:** All API keys must be at least 16 characters. The `internal_proxy_secret` and `single_use_link_secret` values are validated and must not use placeholder defaults. Run `callfs config validate` to check your configuration before starting the server.

### Run

```bash
./callfs server --config config.yaml
```

### Try It

```bash
# Validate your config
./callfs config validate --config config.yaml

# Upload a file
curl -X POST http://localhost:8443/v1/files/hello.txt \
  -H "Authorization: Bearer my-secret-api-key-at-least-16-chars" \
  -H "Content-Type: application/octet-stream" \
  -d "Hello, CallFS!"

# Download it
curl http://localhost:8443/v1/files/hello.txt \
  -H "Authorization: Bearer my-secret-api-key-at-least-16-chars"

# List the root directory
curl http://localhost:8443/v1/directories/ \
  -H "Authorization: Bearer my-secret-api-key-at-least-16-chars"

# Create a single-use download link (expires in 1 hour)
curl -X POST http://localhost:8443/v1/links/generate \
  -H "Authorization: Bearer my-secret-api-key-at-least-16-chars" \
  -H "Content-Type: application/json" \
  -d '{"path": "/hello.txt", "expiry_seconds": 3600}'

# Delete the file
curl -X DELETE http://localhost:8443/v1/files/hello.txt \
  -H "Authorization: Bearer my-secret-api-key-at-least-16-chars"
```

---

## API Reference

All endpoints are prefixed with `/v1` and require `Authorization: Bearer <api-key>` unless noted.

### Files & Directories

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/files/{path}` | Download a file |
| `HEAD` | `/v1/files/{path}` | Get file metadata headers |
| `POST` | `/v1/files/{path}` | Create a file or directory |
| `PUT` | `/v1/files/{path}` | Update (overwrite) a file |
| `DELETE` | `/v1/files/{path}` | Delete a file or directory |
| `GET` | `/v1/files/ws/{path}?mode=download\|upload` | WebSocket file transfer (64KB chunks, 100MB upload limit) |

### Directories

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/directories/{path}` | List directory contents (JSON) |
| `GET` | `/v1/directories/{path}?recursive=true` | Recursive listing |
| `GET` | `/v1/directories/{path}?recursive=true&max_depth=N` | Depth-limited recursive listing |

### Erasure Shards

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/shards/{path}/{index}` | Download individual erasure-coded shard |

### Single-Use Links

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `POST` | `/v1/links/generate` | Required | Generate a time-limited download token |
| `GET` | `/download/{token}` | None | Download a file via single-use token (rate-limited: 10 req/s) |

### Erasure-Coded Uploads

Erasure coding parameters can be set via headers or query parameters (headers take precedence):

```bash
# Upload with erasure coding via query parameters
curl -X POST "http://localhost:8443/v1/files/important.dat?erasure=true&data_shards=4&parity_shards=2" \
  -H "Authorization: Bearer <key>" \
  --data-binary @important.dat

# Or via headers
curl -X POST http://localhost:8443/v1/files/important.dat \
  -H "Authorization: Bearer <key>" \
  -H "X-CallFS-Erasure: true" \
  -H "X-CallFS-Erasure-Data-Shards: 4" \
  -H "X-CallFS-Erasure-Parity-Shards: 2" \
  --data-binary @important.dat
```

### System

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/health` | None | Health check (returns `{"status":"ok"}`) |
| `GET` | `/metrics` | Required | Prometheus metrics (also available on separate metrics port via config) |

---

## Architecture

```
                     +-------------------+
                     |   HTTP/HTTPS/H3   |
                     |    API Server     |
                     +--------+----------+
                              |
                     +--------v----------+
                     |    Core Engine    |
                     |  (routing, locks, |
                     |   cache, authz)   |
                     +--------+----------+
                              |
          +-------------------+-------------------+
          |                   |                   |
  +-------v-------+  +-------v-------+  +--------v--------+
  |   Local FS    |  |   Amazon S3   |  | Internal Proxy  |
  |   Backend     |  |   Backend     |  | (peer routing)  |
  +---------------+  +---------------+  +-----------------+
                              |
                     +--------v----------+
                     |  Metadata Store   |
                     | (Postgres/SQLite/ |
                     |  Redis/Raft)      |
                     +-------------------+
```

### How Cross-Server Routing Works

When a client sends a request to any node in the cluster:

1. The node checks its metadata store for the file's location
2. If the file lives on another node, the request is transparently proxied via the Internal Proxy backend
3. The response is streamed back to the client as if the file were local
4. For writes, distributed locks prevent conflicting concurrent modifications

---

## Clustering & Erasure Coding

### Multi-Node Cluster

```yaml
# Node 1 config
instance_discovery:
  instance_id: "node-1"
  peer_endpoints:
    node-2: "https://10.0.0.2:8443"
    node-3: "https://10.0.0.3:8443"

# Enable erasure coding
erasure:
  enabled: true
  data_shards: 4
  parity_shards: 2
```

With 4 data + 2 parity shards, CallFS can reconstruct any file even if 2 nodes go down.

### Raft Consensus (No External Dependencies)

For clusters that don't want to run PostgreSQL or Redis, use the built-in Raft metadata store:

```yaml
metadata_store:
  type: "raft"

raft:
  enabled: true
  node_id: "node-1"
  bind_addr: "10.0.0.1:7000"
  data_dir: "./raft-data"
  bootstrap: true  # Only on the first node
```

---

## CLI

```
callfs server              Start the API server
  --config, -c <path>      Path to config file

callfs config validate     Validate configuration
  --config, -c <path>      Path to config file

callfs cluster join        Join a Raft cluster
  --leader <url>           Leader API URL (required)
  --node-id <id>           This node's ID
  --raft-addr <addr>       This node's Raft address
  --api-endpoint <url>     This node's API endpoint
  --internal-secret <s>    Shared internal proxy secret
```

All configuration options can also be set via environment variables with the `CALLFS_` prefix (e.g., `CALLFS_SERVER__LISTEN_ADDR=:8443`).

---

## Documentation

| Guide | Description |
|-------|-------------|
| [Installation](docs_markdown/01-installation.md) | Binary, Docker, and source installation |
| [Configuration](docs_markdown/02-configuration.md) | Full config reference with examples |
| [API Reference](docs_markdown/03-api-reference.md) | Complete endpoint documentation |
| [Authentication & Security](docs_markdown/04-authentication-security.md) | API keys, permissions, TLS setup |
| [Backend Configuration](docs_markdown/05-backend-configuration.md) | LocalFS, S3, MinIO setup |
| [Monitoring & Metrics](docs_markdown/06-monitoring-metrics.md) | Prometheus, Grafana, alerting |
| [Clustering & Distribution](docs_markdown/07-clustering-distribution.md) | Multi-node setup, Raft, HA |
| [Developer Guide](docs_markdown/08-developer-guide.md) | Contributing, architecture deep-dive |
| [Troubleshooting](docs_markdown/09-troubleshooting.md) | Common issues and solutions |
| [Cross-Server Operations](docs_markdown/10-enhanced-cross-server.md) | Proxy routing, conflict resolution |

---

## Comparison

| Feature | CallFS | MinIO | SeaweedFS | Ceph RGW |
|---------|--------|-------|-----------|----------|
| Non-S3 REST API | Native | Limited | Partial (filer) | No |
| Single binary | Yes | Yes | No (master+volume+filer) | No |
| Local + S3 hybrid | Yes | No | Partial | No |
| Erasure coding | Yes | Yes | Yes | Yes |
| Unix permissions | Yes | No | No | POSIX via CephFS |
| Single-use links | Built-in | Presigned URLs | No | Presigned URLs |
| Raft metadata | Built-in | Built-in | Built-in | No (uses MON/Paxos) |
| Operational complexity | Low | Low | Medium | High |

---

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License. See [LICENSE](LICENSE).
