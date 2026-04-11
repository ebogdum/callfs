# How to Set Up a Linux File Server with a REST API

Setting up a **file server on Linux** is one of those tasks that sounds straightforward until you actually do it. NFS requires kernel modules and portmapper daemons. Samba needs a working knowledge of Windows-style ACLs. FTP is largely abandoned but still shows up in legacy environments. Each of these protocols was designed for a different era, and none of them speak HTTP -- the language that modern applications actually use.

This tutorial walks through the traditional approaches to running a **linux file server**, explains where each falls short, and then shows you how to stand up a REST API-based file server using [CallFS](https://github.com/ebogdum/callfs) -- an open-source, single-binary server written in Go. By the end you will have a working file server that any HTTP client can talk to, secured with bearer token authentication and backed by SQLite for metadata.

---

## What You'll Build

A Linux file server that:

- Accepts file uploads, downloads, listings, and deletes over plain HTTP or HTTPS
- Authenticates requests with a static API key (no passwords, no OS user accounts)
- Stores files on the local filesystem and tracks metadata in a local SQLite database
- Runs as a single static binary with no runtime dependencies
- Exposes a `/health` endpoint suitable for load balancer or monitoring probes

---

## Prerequisites

- A Linux server (any distribution; commands shown work on Debian/Ubuntu and RHEL/Fedora alike)
- Go 1.24 or later installed, **or** the ability to download a prebuilt binary
- A user account with write access to the directory that will store files
- Basic familiarity with `curl` and YAML configuration files

---

## Traditional Linux File Server Approaches and Their Pain Points

Before jumping to the solution, it is worth understanding why the traditional tools still cause friction in 2024.

### NFS: The Unix Standard That Aged Poorly

NFS (Network File System) has been the default choice for sharing files between Linux machines for decades. It mounts remote directories as if they were local, which sounds convenient -- and it is, right up until something breaks.

The pain points are well-known:

- **Tight coupling to OS users.** NFS relies on UID/GID matching between client and server. If your application runs as UID 1001 on one machine and UID 1003 on another, you start chasing permission errors across nodes.
- **Firewall and NAT hostility.** NFS uses portmapper and dynamic ports. Getting it through a firewall or across a NAT boundary requires extra configuration that quickly becomes error-prone.
- **No built-in encryption.** Plain NFS sends data in the clear. Layering Kerberos on top works but adds significant operational complexity.
- **Not application-friendly.** There is no API. Your application has to treat remote files as local files, which breaks the moment you want to add a CDN, a presigned download link, or any HTTP-level feature.

### Samba: Windows SMB on Linux

Samba implements the SMB protocol so Linux servers can serve files to Windows clients (and to macOS, via Finder). It is mature and well-supported, but its configuration model is designed around Windows domain concepts -- workgroups, shares, user databases -- that do not map naturally onto a containerized or cloud-native environment.

Common complaints:

- Configuration files (`smb.conf`) are dense and counterintuitive for engineers who did not grow up managing Windows networks.
- Samba maintains its own user database separate from system users, adding an extra layer of account management.
- SMB is a stateful, session-oriented protocol. It does not compose well with REST APIs, message queues, or serverless functions.

### FTP: A Protocol from 1971

FTP is still running in plenty of production environments, usually because something was set up years ago and nobody has touched it since. Its problems are fundamental:

- **Plaintext credentials** by default. SFTP and FTPS exist but they are different protocols with different clients.
- **Active/passive mode complexity** causes constant firewall headaches.
- **No atomic operations.** Partial uploads are indistinguishable from complete ones without out-of-band coordination.
- Most modern programming languages treat FTP support as an afterthought; HTTP support is first-class everywhere.

### The Common Thread

All three protocols share a fundamental mismatch with how software is built today. Modern applications communicate over HTTP. They authenticate with bearer tokens or API keys. They expect JSON responses. They run in containers that do not have kernel modules, and they deploy to environments where OS-level user management is either impractical or forbidden.

A file server that speaks HTTP fixes all of this at the protocol level.

---

## A REST API Approach: CallFS

CallFS is an open-source file server that exposes a clean REST API over HTTP or HTTPS. Files are stored on local disk or in an S3-compatible object store. Metadata (file paths, ownership, sizes) is kept in a separate store -- PostgreSQL, SQLite, Redis, or a built-in Raft log.

For a single-node setup, the combination of local filesystem storage and SQLite metadata means there are zero external dependencies. One binary, one config file, one SQLite database file.

### Key design decisions worth noting

- **No OS user accounts.** Authentication is done with static API keys. There is no mapping between application users and Linux UIDs or GIDs.
- **Streaming I/O.** Files are never fully buffered in memory during transfer, which matters when you are moving large files.
- **Single static binary.** No runtime, no shared libraries, no package manager dependencies. Copy the binary and run it.

---

## Installation

### Option 1: Install with Go

If Go 1.24 or later is installed:

```bash
go install github.com/ebogdum/callfs/cmd@latest
```

This downloads, compiles, and places the `callfs` binary in `$GOPATH/bin` (typically `~/go/bin`). Make sure that directory is in your `PATH`.

### Option 2: Download a Prebuilt Binary

Prebuilt binaries for Linux (amd64, arm64) and macOS are available on the [GitHub releases page](https://github.com/ebogdum/callfs/releases). Download the binary for your platform, mark it executable, and move it somewhere on your `PATH`:

```bash
# Example for Linux amd64 -- check the releases page for the current version
curl -Lo callfs https://github.com/ebogdum/callfs/releases/latest/download/callfs-linux-amd64
chmod +x callfs
sudo mv callfs /usr/local/bin/callfs
```

Verify the installation:

```bash
callfs --help
```

---

## Configuration

CallFS is configured with a single YAML file. Below is a minimal configuration suitable for a single-node file server using local filesystem storage and SQLite metadata -- no external services required.

Create a file named `config.yaml`:

```yaml
server:
  listen_addr: ":8443"
  protocol: "http"
  external_url: "http://localhost:8443"

auth:
  api_keys:
    - "your-api-key-here-at-least-16-chars"
  internal_proxy_secret: "internal-secret-change-this-now"
  single_use_link_secret: "link-secret-change-this-too-now"

backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs/data"

metadata_store:
  type: "sqlite"
  sqlite_path: "/var/lib/callfs/callfs.sqlite3"

dlm:
  type: "local"

log:
  level: "info"
  format: "json"

instance_discovery:
  instance_id: "node-1"
```

A few things to note about this configuration:

- `api_keys` accepts a list of strings; each key must be at least 16 characters. Use a password manager or `openssl rand -hex 32` to generate a strong key.
- `internal_proxy_secret` and `single_use_link_secret` must be set to non-default values or the server will refuse to start.
- `dlm.type: "local"` uses an in-process lock manager. For multi-node deployments, switch this to `"redis"` and point it at a Redis instance.
- The `protocol: "http"` setting runs the server without TLS. For production, set this to `"https"` and add `cert_file` and `key_file` paths.

Create the data directory and validate the configuration:

```bash
sudo mkdir -p /var/lib/callfs/data
callfs config validate --config config.yaml
```

If validation passes, start the server:

```bash
callfs server --config config.yaml
```

You should see JSON log lines indicating the server is listening on `:8443`.

---

## Working with Files via the REST API

All endpoints live under `/v1` and require an `Authorization: Bearer <api-key>` header. The examples below use `your-api-key-here` as a placeholder -- substitute your actual key.

### Health Check

Before touching files, confirm the server is up:

```bash
curl -s http://localhost:8443/health
```

Expected response:

```json
{"status":"ok"}
```

### Uploading a File

Use HTTP `POST` to create a new file. The path in the URL becomes the file's path on the server:

```bash
echo "Hello, CallFS!" | curl -s \
  -X POST \
  -H "Authorization: Bearer your-api-key-here" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @- \
  http://localhost:8443/v1/files/docs/hello.txt
```

This creates a file at `docs/hello.txt` relative to the configured `localfs_root_path`. The server creates intermediate directories automatically.

To upload an existing file from disk:

```bash
curl -s \
  -X POST \
  -H "Authorization: Bearer your-api-key-here" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @/path/to/local/file.txt \
  http://localhost:8443/v1/files/docs/file.txt
```

### Downloading a File

Use HTTP `GET` on the same path:

```bash
curl -s \
  -H "Authorization: Bearer your-api-key-here" \
  http://localhost:8443/v1/files/docs/hello.txt
```

The response body is the raw file content. Add `-o output.txt` to save it to disk.

### Listing a Directory

The `/v1/directories/` prefix returns JSON listings of directory contents:

```bash
curl -s \
  -H "Authorization: Bearer your-api-key-here" \
  http://localhost:8443/v1/directories/docs/
```

Example response:

```json
{
  "path": "docs/",
  "entries": [
    {
      "name": "hello.txt",
      "path": "docs/hello.txt",
      "is_dir": false,
      "size": 14,
      "modified_at": "2024-11-01T12:00:00Z"
    }
  ]
}
```

Add `?recursive=true` to list subdirectories, or `?recursive=true&max_depth=2` to limit traversal depth.

### Updating a File

Use `PUT` to overwrite an existing file:

```bash
echo "Updated content" | curl -s \
  -X PUT \
  -H "Authorization: Bearer your-api-key-here" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @- \
  http://localhost:8443/v1/files/docs/hello.txt
```

### Deleting a File

```bash
curl -s \
  -X DELETE \
  -H "Authorization: Bearer your-api-key-here" \
  http://localhost:8443/v1/files/docs/hello.txt
```

A successful delete returns HTTP 204 with no body.

---

## Running CallFS as a systemd Service

For a production deployment, run CallFS under systemd so it starts on boot and restarts on failure.

Create a service file at `/etc/systemd/system/callfs.service`:

```ini
[Unit]
Description=CallFS File Server
After=network.target

[Service]
Type=simple
User=callfs
Group=callfs
ExecStart=/usr/local/bin/callfs server --config /etc/callfs/config.yaml
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

Create the system user, set up directories, and enable the service:

```bash
sudo useradd --system --no-create-home --shell /sbin/nologin callfs
sudo mkdir -p /etc/callfs /var/lib/callfs/data
sudo cp config.yaml /etc/callfs/config.yaml
sudo chown -R callfs:callfs /var/lib/callfs /etc/callfs
sudo systemctl daemon-reload
sudo systemctl enable --now callfs
```

Check the status:

```bash
sudo systemctl status callfs
sudo journalctl -u callfs -f
```

---

## Enabling HTTPS

For any network-accessible deployment, run CallFS with TLS. If you have a certificate from Let's Encrypt or another CA:

```yaml
server:
  listen_addr: ":8443"
  protocol: "https"
  external_url: "https://files.example.com:8443"
  cert_file: "/etc/ssl/certs/files.example.com.crt"
  key_file: "/etc/ssl/private/files.example.com.key"
```

For a quick self-signed certificate on a private network:

```bash
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout server.key -out server.crt \
  -days 365 -subj "/CN=localhost"
```

CallFS also supports HTTP/3 (QUIC) via the `enable_quic: true` config option, which can improve throughput on high-latency connections.

---

## How This Compares to NFS and Samba

| Concern | NFS / Samba / FTP | CallFS REST API |
|---|---|---|
| Protocol | OS-level mount / SMB / FTP | HTTP/HTTPS |
| Auth model | OS users / Samba accounts / passwords | Bearer token (API key) |
| Firewall-friendly | Often not | Yes -- single TCP port |
| Works from a container | Requires kernel modules | Any HTTP client |
| Encryption | Requires extra setup | Native TLS/HTTPS |
| Application integration | File system calls | Standard HTTP library |
| Monitoring | OS-level stats | Prometheus metrics at `/metrics` |

---

## Next Steps

A single-node CallFS setup with local storage and SQLite is a solid foundation. Once you are comfortable with the basics, several directions are worth exploring:

**Monitoring and alerting.** CallFS exposes Prometheus metrics on a dedicated port (`:9090` by default). Connect Grafana to scrape request latency histograms, operation counters, and backend durations. See the [Monitoring guide](../docs_markdown/06-monitoring-metrics.md) for dashboard examples.

**Switching to PostgreSQL.** Change `metadata_store.type` to `"postgres"` and provide a DSN. PostgreSQL gives you ACID guarantees and is the right choice for high-write workloads or environments where you already operate a database cluster.

**Adding an S3 backend.** Set `backend.default_backend` to `"s3"` and supply your credentials. CallFS will store file content in S3 while still serving the same REST API, giving you effectively unlimited storage capacity.

**Horizontal scaling.** Add a second node with the same metadata store configuration and list each node's API endpoint under `instance_discovery.peer_endpoints`. CallFS routes requests to the correct node transparently. Enable erasure coding to distribute shards across nodes for fault tolerance.

**Secure file sharing.** Use the `/v1/links/generate` endpoint to create time-limited, HMAC-signed download tokens. Share the token URL with clients who do not have an API key. Tokens expire, are rate-limited, and can only be used to download a specific file.

**Raft for self-contained clusters.** If you do not want to operate a separate PostgreSQL or Redis instance, enable the built-in Raft metadata store. Three nodes with `metadata_store.type: "raft"` give you a fully self-contained, strongly-consistent cluster with no external dependencies.

---

The core insight behind a REST API file server is simple: if your applications already speak HTTP, your file storage should too. Replacing a fragile NFS mount or an aging FTP server with a `curl`-compatible HTTP endpoint removes an entire class of infrastructure problems -- firewall rules, UID mapping, kernel module compatibility -- and replaces them with the same auth and networking model your other services already use.
