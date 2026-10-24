# CallFS

[![Go Report Card](https://goreportcard.com/badge/github.com/ebogdum/callfs/v2)](https://goreportcard.com/report/github.com/ebogdum/callfs/v2)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/ebogdum/callfs)](https://go.dev/)

CallFS is a file server with a REST API and a built-in web console. It stores files on local disks, S3-compatible object stores, or both, and presents them as one filesystem over HTTP. You can run it as a single binary on one machine, or join several nodes into a cluster that needs no external coordination service: clustering runs on an embedded NATS server. Files are protected with ownership rules, optional erasure coding and replication, and can be shared through expiring single-use links.

![CallFS console](docs/images/console/home-dark.png)

## What's new in 2.0

### A web console

CallFS now ships with a full web console at `/ui/`. Browse, upload (including whole folders by drag and drop), move, share and restore files. Watch every server in the cluster from one place, manage users and keys, change settings, follow logs and audit events, and act on health findings. See the [web console guide](docs_markdown/08-web-console.md) and the [screenshots](#screenshots) below.

### Much faster writes

Writes were reworked end to end: locks are taken only where they are needed, saves are flushed to disk in batches instead of one at a time, the file index commits many changes in one step, and overwrites save the new copy first and swap it in with two renames. Uploads and overwrites are now about five times faster under load, with far lower tail latency and no failed requests.

Both versions were run on the same machine with the same single-node setup: local disk storage, SQLite file index, default write safety (each save is on disk before it is confirmed), plain HTTP, keep-alive connections, 1 KB files, a fresh server for every test.

| Test (64 clients) | 1.6.0 | 2.0.0 | Change |
|---|---|---|---|
| Upload new files | 159 files/s, p99 1.02 s, 44 failed | 773 files/s, p99 343 ms, 0 failed | 4.9× faster |
| Overwrite files | 154 saves/s, p99 1.24 s, 56 failed | 778 saves/s, p99 328 ms, 0 failed | 5.1× faster |

## Features

### Files API
- Upload, download, overwrite, rename, move and delete files and folders under `/v1/files/{path}`.
- Range downloads, `HEAD` for metadata, and a WebSocket transfer mode for large files.
- Directory listing with sorting, filtering, paging, hidden-file control and recursive listing with a depth limit.
- Search across your files by name, size, date, owner, storage, server, split state and lock state, plus bulk listing and bulk delete by prefix.
- Resumable multi-part uploads.
- Batch operations that run as background jobs you can follow, cancel and retry.
- Trash with restore, permanent delete and a configurable retention period.
- Per-user storage usage report.
- Folder and file mode bits control access for other users: the "other" read bit allows reading a file or listing a folder, the "other" write bit allows changing a file or creating items in a folder. Deleting stays with the owner. New folders start at `0777`, new files at `0644`.

### Sharing and links
- Single-use download links with an expiry you choose.
- List, inspect and revoke the links you created; see link statistics (admins see the whole cluster, other users see their own links, including the last cleanup result).
- A per-user limit on active links (`server.rate_limit.max_active_links_per_user`, default 1000).
- Links open a confirmation step before the download starts, and are rate limited.

### Storage backends
- Local disk and any S3-compatible store (Amazon S3, MinIO and others), with server-side encryption and ACL settings for S3.
- Choose the default backend, and move files between backends and servers.
- Metadata index in PostgreSQL, SQLite, Redis or NATS.

### Erasure coding
- Split files into data and parity shards so they survive the loss of nodes or disks.
- Set shard counts globally or per upload, split every upload at or above `erasure.min_file_size` automatically (opt out with `X-CallFS-Erasure: false`), and choose the folder for pieces with `erasure.shard_path`.
- Verify, repair and plan erasure-coded files from the API and the console.

### Replication
- Copy every file to a second backend, optionally requiring the copy to succeed before a write is acknowledged.
- Copies run as durable background tasks with retries; failed ones are listed and you can retry them.

### Clustering on embedded NATS
- Nodes find each other through peer endpoints and form a cluster with no external services.
- Metadata replication, distributed locks and node messaging all run on the embedded NATS server.
- The file index log is snapshotted and compacted on a schedule, and nodes restore from the newest snapshot at start.
- TLS for client and cluster traffic with mutual verification, key-based sign-in between nodes and rolling secret changes.
- Fair first-come lock queue, cluster-wide request limits and shared logs so the console shows every node.
- File bytes between nodes travel over HTTP or in chunks over NATS (`nats.bytes`).
- Every node announces itself; `/v1/nodes` shows its role, version and uptime, and background jobs that must run once per cluster run on one node.
- Any node serves any file: requests are routed to the node that holds the data.
- Manage every node from one console: members (add, make leader, remove an offline member), replication, snapshots and servers.
- Connect to your own NATS servers instead with `nats.mode: external`.
- Admin cluster API for state, snapshots, leadership transfer, member removal and join configs.
- Raft cluster mode was removed in 2.0.0; configs that use it are rejected at startup. Use `metadata_store.type: nats` and `dlm.type: nats`.

### Performance modes and write safety
- `performance.mode: fast` (default) or `strict`. Both only acknowledge a write once it is durable. Strict also reads back every saved file and checks it, locks every change, and makes every read wait for the latest cluster state.
- Choose how local writes are acknowledged with `backend.localfs_ack` (`memory` or `disk`, default `disk`), how they are synced with `backend.localfs_sync` (`full`, `barrier` or `none`, default `full`), and override the acknowledgement per request with the `X-CallFS-Write-Ack` header.
- Tune background flushing with `backend.flush_workers`, `backend.flush_max_pending_bytes` and `backend.flush_max_delay`.

### Web console
- Served from the same port at `/ui/`; sign in with an API key.
- Overview with health checks, findings and key numbers.
- Files: browse with a right-click menu, drag and drop of files and whole folders, cut and paste, rename, move (also between storages and servers for admins), mode input as boxes, numbers or letters, preview, inspector, search, flat list, trash with restore choices, and transfers with conflict choices, resume after reload and checksum verification.
- Sharing: create, inspect and revoke single-use links.
- Cluster: topology, members (add, make leader, remove offline), replication and snapshots, servers, storage, durability, backup copy, hygiene, edit locks, index cache and request limits.
- Insights (dashboards, explore, storage, metrics) and Activity (events, audit, jobs, logs, sign-ins).
- Settings for every option in plain language, users and keys, config file export and appearance; change runtime settings live and restart from the console.
- Command palette, editable keyboard shortcuts, light and dark themes, printable report and a wall display.

### Monitoring and health findings
- Prometheus metrics on the API port and on a separate metrics port.
- `/health` and `/health/ready` probes (readiness reflects this node only; peer problems are listed but never fail it) and a `callfs healthcheck` command for container health checks.
- A findings list that flags problems such as unencrypted connections, default or short secrets, expiring certificates, a nearly full disk, split files that need repair, failing backup copies and failed sign-ins. You can mute or ignore findings.
- Live logs, audit trail, event stream and metric charts in the console.

### Security
- API keys bound to named users; every file has an owner and access follows ownership.
- Admin users listed in `auth.admin_users` manage users, keys, settings and the cluster.
- Console sessions with expiry, idle timeout and CSRF protection; signing out ends the session on the server and across the cluster.
- TLS, HTTP/3 (QUIC), configurable rate limits and trusted-proxy handling.
- HMAC-signed single-use links; startup rejects sample or weak secrets.
- Encrypted, authenticated traffic between cluster nodes, with an extra CA setting (`backend.internal_proxy_ca_file`) for checking other servers' certificates.

## Screenshots

![Files](docs/images/console/files.png)
Browse, upload, rename, move, share and delete files.

![Transfers](docs/images/console/files-transfers.png)
Uploads and downloads with progress, conflict choices and resume.

![Sharing](docs/images/console/sharing.png)
Create single-use links and see which ones are active, used or expired.

![Cluster](docs/images/console/cluster.png)
Cluster health and what needs attention, at a glance.

![Cluster members](docs/images/console/cluster-nodes.png)
Every member with its state, how far behind it is, and the Add member form.

![Replication](docs/images/console/cluster-replication.png)
The file index log, the copies on each server, snapshots, and a button to move the leader.

![Insights](docs/images/console/insights.png)
Traffic, errors, latency and storage trends.

![Settings](docs/images/console/settings-group.png)
Every setting in plain language, with a marker for those that need a restart.

![Cluster and locks settings](docs/images/console/settings-coordination.png)
Every NATS and edit lock setting in one place.

![Activity](docs/images/console/activity.png)
Recent events, jobs, sign-ins and the audit trail.

## Quick start

### Install

Binary: download the build for your platform (Linux, macOS or Windows) and make it executable, or build from source:

```bash
make web-install build-ui build
```

This produces `./callfs` with the console included. Use `make build-noui` for a binary without the console. See [Installation](docs_markdown/01-installation.md).

Docker:

```bash
docker build -t callfs .
```

### Configure

Create `config.yaml`:

```yaml
server:
  listen_addr: ":8443"
  protocol: "http"
  external_url: "http://localhost:8443"

auth:
  api_key_users:
    alice: "your-api-key-at-least-16-chars"
  admin_users: ["alice"]
  internal_proxy_secret: "replace-with-openssl-rand-hex-32"
  single_use_link_secret: "replace-with-another-random-value"

backend:
  localfs_root_path: "/var/lib/callfs"

metadata_store:
  type: "sqlite"
  sqlite_path: "/var/lib/callfs/callfs.sqlite3"

dlm:
  type: "local"

instance_discovery:
  instance_id: "node-1"
```

API keys must be at least 16 characters. Generate the two secrets with `openssl rand -hex 32`; sample values are rejected at startup. Check the file before you start:

```bash
./callfs config validate --config config.yaml
```

Every setting can also be set by environment variable with the `CALLFS_` prefix and a double underscore for nesting, for example `CALLFS_SERVER__LISTEN_ADDR=:8443`.

### Start

```bash
./callfs server --config config.yaml
```

With Docker:

```bash
docker run -d --name callfs -p 8443:8443 \
  -v /path/to/config.yaml:/config.yaml:ro \
  -v callfs-data:/var/lib/callfs \
  callfs server --config /config.yaml
```

Open `http://localhost:8443/ui/` and sign in with your API key.

### First upload and download

```bash
curl -X POST http://localhost:8443/v1/files/hello.txt \
  -H "Authorization: Bearer your-api-key-at-least-16-chars" \
  -H "Content-Type: application/octet-stream" \
  -d "Hello, CallFS!"

curl http://localhost:8443/v1/files/hello.txt \
  -H "Authorization: Bearer your-api-key-at-least-16-chars"
```

Create a one-hour single-use link:

```bash
curl -X POST http://localhost:8443/v1/links/generate \
  -H "Authorization: Bearer your-api-key-at-least-16-chars" \
  -H "Content-Type: application/json" \
  -d '{"path": "/hello.txt", "expiry_seconds": 3600}'
```

## Cluster quick start

Give each node its own `instance_id`, list the others under `instance_discovery.peer_endpoints`, and turn on NATS:

```yaml
instance_discovery:
  instance_id: "node-1"
  peer_endpoints:
    node-2: "https://node-2.example.com:8443"
    node-3: "https://node-3.example.com:8443"

metadata_store:
  type: "nats"

dlm:
  type: "nats"

nats:
  enabled: true
```

Use the same `auth.internal_proxy_secret` on every node. Non-loopback cluster traffic needs TLS certificates. The full walkthrough, including routes, TLS and node management, is in [Clustering](docs_markdown/07-clustering.md).

## Documentation

| Guide | What it covers |
|-------|----------------|
| [Installation](docs_markdown/01-installation.md) | Binary, Docker, source build, first run, CLI |
| [Configuration](docs_markdown/02-configuration.md) | Every setting, environment variables, validation |
| [API reference](docs_markdown/03-api-reference.md) | Files, directories, links, uploads, search, trash, batch, health |
| [Authentication and security](docs_markdown/04-authentication-security.md) | API keys, users, sessions, TLS, rate limits |
| [Storage](docs_markdown/05-storage.md) | Local disk, S3, erasure coding, replication, trash, write safety |
| [Monitoring and metrics](docs_markdown/06-monitoring-metrics.md) | Prometheus, health checks, findings, logs, audit |
| [Clustering](docs_markdown/07-clustering.md) | NATS cluster, nodes, cross-server operations, locks |
| [Web console](docs_markdown/08-web-console.md) | Every console page |
| [Troubleshooting](docs_markdown/09-troubleshooting.md) | Problems and fixes |
| [Admin API](docs_markdown/10-admin-api.md) | `/v1/admin` and node gateway |
| [Articles](articles/) | Step-by-step guides: file server setup, secure sharing, S3 hybrid storage, erasure coding, NATS clustering, backups, expiring links, Raspberry Pi, Grafana monitoring |

## Contributing, security, license

- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)
- [License](LICENSE) (MIT)
