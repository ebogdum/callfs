# How to Set Up a REST API File Server on Linux

Setting up a file server on Linux no longer requires complex daemon configurations or proprietary protocols. CallFS is an open-source, REST API-native file server that runs as a single binary on any Linux system. It exposes every file operation — upload, download, list, update, delete — as a standard HTTP endpoint, making it scriptable with `curl`, integratable with any HTTP client library, and straightforward to monitor and secure. This tutorial walks you through installing CallFS, configuring it for a single-node deployment, and performing common file operations using plain HTTP requests.

---

## What You'll Build

By the end of this guide you will have a fully operational Linux file server that accepts authenticated HTTP requests to upload, download, list, and delete files. The server stores files on local disk and tracks metadata in a local SQLite database — no external services required to get started. You will also configure it as a systemd service so it survives reboots, and learn how to switch it to HTTPS when you are ready to expose it beyond a local network.

---

## Prerequisites

- A Linux server (any modern distribution: Debian, Ubuntu, RHEL, Rocky, Fedora, or similar)
- `curl` installed (available by default on most distributions)
- A user account with `sudo` privileges

No programming language runtime or package manager is needed. CallFS ships as a statically linked binary.

---

## Installation

Download the latest release binary directly from GitHub, make it executable, and move it to a system-wide location:

```bash
curl -Lo callfs https://github.com/ebogdum/callfs/releases/latest/download/callfs-linux-amd64 \
  && chmod +x callfs \
  && sudo mv callfs /usr/local/bin/
```

Verify the installation:

```bash
callfs version
```

You should see the version string printed to stdout. CallFS is now available system-wide.

---

## Configuration

CallFS reads a YAML configuration file at startup. Create the directories it will use, then write the configuration:

```bash
sudo mkdir -p /var/lib/callfs/data
sudo mkdir -p /etc/callfs
```

Create `/etc/callfs/config.yaml` with the following content. This is a minimal single-node configuration using local disk storage and SQLite for metadata — the simplest possible setup for a Linux file server:

```yaml
server:
  listen_addr: ":8443"
  protocol: "http"
  read_timeout: 30s
  write_timeout: 60s

auth:
  api_keys:
    - "your-api-key-here-change-me"
  internal_proxy_secret: "internal-secret-change-me"
  single_use_link_secret: "link-secret-change-this"

backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs/data"

metadata_store:
  type: "sqlite"
  sqlite_path: "/var/lib/callfs/metadata.sqlite3"

dlm:
  type: "local"

instance_discovery:
  instance_id: "server-1"
```

### Configuration notes

- **`listen_addr`** — the address and port the server binds to. `:8443` means all interfaces on port 8443. You can change the port to any unprivileged port (1024 and above) without requiring elevated privileges.
- **`protocol`** — set to `http` for this tutorial. See the HTTPS section below for production hardening.
- **`api_keys`** — a list of bearer tokens clients must present in the `Authorization: Bearer` header. Replace the placeholder with a random string of at least 16 characters. You can list multiple keys to support key rotation without downtime.
- **`internal_proxy_secret`** — used for authenticated communication between CallFS nodes in a cluster. Even on a single node, this field is required and must be set to a non-empty value.
- **`single_use_link_secret`** — the signing secret for time-limited download links. Required even if you do not plan to use the feature.
- **`localfs_root_path`** — the directory where uploaded files are stored on disk. Ensure this path exists and is writable by the user running CallFS.
- **`sqlite_path`** — where CallFS writes its metadata database. SQLite is a good choice for single-node deployments and development. For multi-node setups, CallFS also supports PostgreSQL and Redis.
- **`dlm.type: local`** — uses an in-process distributed lock manager, appropriate for single-node deployments. Switch to `redis` when running multiple instances to coordinate concurrent writes across nodes.

---

## Starting the Server

Run CallFS directly from the command line to confirm it starts correctly:

```bash
callfs server --config /etc/callfs/config.yaml
```

You should see structured JSON log output indicating the server is listening on the configured address. The log format and level are configurable; for human-readable output during development, add the following block to your config:

```yaml
log:
  level: "info"
  format: "console"
```

Leave the server running and open a second terminal to test file operations with `curl`.

---

## Working with Files

All file operations use standard HTTP verbs against the `/v1/files/` and `/v1/directories/` endpoint prefixes. The API follows REST conventions: POST creates, GET reads, PUT replaces, DELETE removes. Authentication is handled through a bearer token in the `Authorization` header on every request (except the health endpoint).

Replace `YOUR_API_KEY` in each example with the key you set in `config.yaml`.

### Health check

Confirm the server is up and responding:

```bash
curl http://localhost:8443/health
```

Expected response:

```json
{"status":"ok"}
```

The health endpoint requires no authentication and is suitable for use with load balancers and monitoring probes.

### Create a directory

CallFS uses a trailing slash convention to distinguish directory operations from file operations. Directories are created with a POST request to a path ending in a trailing slash:

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/files/projects/
```

### Upload a file

Send file content as an octet-stream body. CallFS writes the file to disk and records its metadata — path, size, owner, and modification time — atomically in the metadata store. The parent directory must exist before uploading:

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @readme.txt \
  http://localhost:8443/v1/files/projects/readme.txt
```

### Download a file

A GET request to any file path streams the content back. CallFS sets the `Content-Length` header so clients can display download progress:

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/files/projects/readme.txt
```

To save the downloaded content to a local file, add `-o output.txt` to the command. For large files, `curl` will display transfer progress automatically when writing to a file.

### List directory contents

Use the `/v1/directories/` prefix to list a directory. The response is JSON and includes each entry's name, type, size, and modification time:

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/directories/projects/
```

Example response:

```json
{
  "path": "projects/",
  "type": "directory",
  "recursive": false,
  "count": 1,
  "items": [
    {
      "name": "readme.txt",
      "path": "projects/readme.txt",
      "type": "file",
      "size": 1024,
      "mode": "-rw-r--r--",
      "owner": "",
      "mtime": "2026-04-11T10:00:00Z"
    }
  ]
}
```

To list recursively, append `?recursive=true` to the URL.

### Read file metadata

A HEAD request returns file metadata in response headers without transferring the body. This is useful for checking whether a file exists and what its current size is before deciding to download it:

```bash
curl -I \
  -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/files/projects/readme.txt
```

CallFS returns the following custom headers:

| Header | Description |
|---|---|
| `X-CallFS-Size` | File size in bytes |
| `X-CallFS-Mode` | File permission mode string |
| `X-CallFS-Owner` | Owner identifier |
| `X-CallFS-MTime` | Last modification timestamp |
| `X-CallFS-Type` | Resource type (`file` or `directory`) |

### Update a file

Replace an existing file's content with a PUT request:

```bash
curl -X PUT \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @readme-v2.txt \
  http://localhost:8443/v1/files/projects/readme.txt
```

PUT is idempotent: running it multiple times with the same content produces the same result.

### Delete a file

```bash
curl -X DELETE \
  -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/files/projects/readme.txt
```

A successful delete returns HTTP 204 No Content. Both the file data and its metadata record are removed. Attempting to delete a path that does not exist returns HTTP 404.

---

## Running as a systemd Service

For a production Linux file server, you want CallFS to start automatically on boot and restart on failure. Create a systemd unit file:

```bash
sudo tee /etc/systemd/system/callfs.service > /dev/null <<EOF
[Unit]
Description=CallFS REST API File Server
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/callfs server --config /etc/callfs/config.yaml
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal
SyslogIdentifier=callfs

[Install]
WantedBy=multi-user.target
EOF
```

Enable and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable callfs
sudo systemctl start callfs
```

Check that it is running:

```bash
sudo systemctl status callfs
```

View logs at any time with:

```bash
sudo journalctl -u callfs -f
```

---

## Enabling HTTPS

The HTTP configuration above is convenient for a local network or a trusted internal environment. For a publicly accessible file server on Linux, enable TLS by providing a certificate and key.

If you have a certificate (for example from Let's Encrypt), update the `server` block in your configuration:

```yaml
server:
  listen_addr: ":8443"
  protocol: "https"
  cert_file: "/etc/callfs/server.crt"
  key_file: "/etc/callfs/server.key"
  read_timeout: 30s
  write_timeout: 60s
```

Then update any `curl` commands to use `https://` instead of `http://`. If you are using a self-signed certificate during testing, add the `-k` flag to `curl` to skip certificate verification — do not do this in production.

Restart the service after any configuration change:

```bash
sudo systemctl restart callfs
```

---

## Next Steps

This tutorial covered a minimal single-node setup. CallFS supports considerably more:

- **Monitoring** — a Prometheus-compatible metrics endpoint on a configurable port for tracking request rates, latency, and storage utilization.
- **Clustering** — multi-node deployments with Raft-based metadata consensus and peer-aware request routing.
- **S3 backend** — store files in any S3-compatible object store instead of local disk, with optional server-side encryption.
- **Erasure coding** — distribute file shards across nodes for fault tolerance with configurable data and parity shard counts.
- **Single-use download links** — generate time-limited, unauthenticated download URLs for sharing files without exposing API keys.

Full documentation is available in the [CallFS repository](https://github.com/ebogdum/callfs) under the `docs/` directory.
