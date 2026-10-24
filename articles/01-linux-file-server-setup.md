# How to Set Up a REST API File Server on Linux

Setting up a file server on Linux no longer requires complex daemon configurations or proprietary protocols. CallFS is an open-source, REST API-native file server that runs as a single binary on any Linux system. It exposes every file operation (upload, download, list, rename, delete) as a standard HTTP endpoint, making it scriptable with `curl`, easy to call from any HTTP client library, and straightforward to monitor and secure. It also ships a web console at `/ui/` for people who prefer a browser. This tutorial walks you through installing CallFS, configuring it for a single-node deployment, and performing common file operations using plain HTTP requests.

---

## What You'll Build

By the end of this guide you will have a working Linux file server that accepts authenticated HTTP requests to upload, download, list, move, and delete files. The server stores files on local disk and tracks metadata in a local SQLite database, so no external services are required to get started. You will also run it as a systemd service so it survives reboots, and learn how to switch it to HTTPS when you are ready to expose it beyond a local network.

---

## Prerequisites

- A Linux server (any modern distribution: Debian, Ubuntu, RHEL, Rocky, Fedora, or similar)
- `curl` and `openssl` installed (both are available by default on most distributions)
- A user account with `sudo` privileges

No programming language runtime or package manager is needed. CallFS ships as a statically linked binary.

---

## Installation

Download the release binary from GitHub, check it against the published checksums, and install it system-wide. Pin the version you want instead of using a "latest" link:

```bash
VERSION=v2.0.0
curl -LO https://github.com/ebogdum/callfs/releases/download/$VERSION/callfs-linux-amd64
curl -LO https://github.com/ebogdum/callfs/releases/download/$VERSION/SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
sudo install -m 755 callfs-linux-amd64 /usr/local/bin/callfs
callfs --help
```

`sha256sum` must print `OK` for `callfs-linux-amd64`. Stop if it prints `FAILED`. On ARM64 machines use `callfs-linux-arm64`.

The prebuilt binaries include the web console at `/ui/`. Everything in this tutorial also works from the command line.

---

## Configuration

CallFS reads a YAML configuration file at startup. Create a service user, the directories CallFS will use, and the secrets it needs:

```bash
sudo useradd --system --shell /usr/sbin/nologin --home /var/lib/callfs callfs
sudo mkdir -p /var/lib/callfs/data /etc/callfs
sudo chown -R callfs:callfs /var/lib/callfs

API_KEY=$(openssl rand -hex 32)
SECRET_1=$(openssl rand -hex 32)
SECRET_2=$(openssl rand -hex 32)
echo "Your API key: $API_KEY"
```

Save the API key. You need it for every request and to sign in to the console.

Create `/etc/callfs/config.yaml`. This is a minimal single-node configuration using local disk storage and SQLite for metadata, the simplest possible setup for a Linux file server:

```bash
sudo tee /etc/callfs/config.yaml > /dev/null <<EOF
server:
  listen_addr: ":8443"
  protocol: "http"
  external_url: "http://localhost:8443"

auth:
  api_key_users:
    admin: "$API_KEY"
  admin_users: ["admin"]
  internal_proxy_secret: "$SECRET_1"
  single_use_link_secret: "$SECRET_2"

backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs/data"

metadata_store:
  type: "sqlite"
  sqlite_path: "/var/lib/callfs/metadata.sqlite3"

dlm:
  type: "local"

instance_discovery:
  instance_id: "node-1"

metrics:
  listen_addr: "127.0.0.1:9090"
EOF
sudo chown callfs:callfs /etc/callfs/config.yaml
sudo chmod 600 /etc/callfs/config.yaml
```

### Configuration notes

- **`listen_addr`**: the address and port the server binds to. `:8443` means all interfaces on port 8443. Any port of 1024 and above works without elevated privileges.
- **`protocol`**: `http`, `https` or `auto`. The default is `https`, which requires `cert_file` and `key_file`. This tutorial uses `http`; see the HTTPS section below for production.
- **`external_url`**: the public address CallFS puts into the download links it creates. Without a `http://` prefix it assumes `https://`.
- **`api_key_users`**: maps a user name to an API key. The user name is recorded as the owner of everything that key creates, and it stays the same when other keys are added, rotated or removed. Each key must be at least 16 characters, must be different from every other key and secret, and must not contain placeholder text such as `your-`, `change-me` or `a-strong-`. The names `root`, `internal-proxy` and `api-user-N` are reserved.
- **`admin_users`**: users who can see the administration pages of the web console and change settings. Every name here must also exist in `api_key_users`.
- **`internal_proxy_secret`**: the shared secret that CallFS nodes use to trust each other. It is required even on a single node, must be at least 16 characters, and must differ from every API key and from `single_use_link_secret`.
- **`single_use_link_secret`**: the signing secret for one-time download links. It is required even if you do not plan to use the feature, and it has the same rules as `internal_proxy_secret`.
- **`localfs_root_path`**: the directory where uploaded files are stored. It must exist and be writable by the user running CallFS. Changing it later does not move existing files.
- **`sqlite_path`**: where CallFS writes its metadata database. SQLite is a good choice for a single node. For several nodes use the embedded NATS metadata store, or PostgreSQL or Redis.
- **`dlm.type: local`**: in-process locking, correct for one node. Use `nats` or `redis` when you run several nodes.
- **`instance_id`**: a name for this server. It is required and has no default.
- **`metrics.listen_addr`**: a separate, unauthenticated port for Prometheus metrics. The default `:9090` listens on every interface; binding it to `127.0.0.1` keeps it private.

You can also supply any setting as an environment variable. The name is `CALLFS_` followed by the key path in upper case with `__` between levels, for example `CALLFS_AUTH__INTERNAL_PROXY_SECRET`. Environment variables win over the file. Every setting is listed in [Configuration](../docs_markdown/02-configuration.md).

### Validate the file

```bash
sudo -u callfs callfs config validate --config /etc/callfs/config.yaml
```

It prints `Configuration is valid` and exits with 0, or prints the first problem and exits with 1. It checks the settings only; it does not connect to any database or remote service.

---

## Starting the Server

Run CallFS directly from the command line to confirm it starts correctly:

```bash
sudo -u callfs callfs server --config /etc/callfs/config.yaml
```

The server logs to standard error as structured JSON. For human-readable output during development, add this block to your config:

```yaml
log:
  level: "info"
  format: "text"
```

`level` is `debug`, `info`, `warn` or `error`. Leave the server running and open a second terminal to test file operations with `curl`.

---

## Working with Files

All file operations use standard HTTP verbs against the `/v1/files/` and `/v1/directories/` endpoint prefixes. POST creates, GET reads, PUT creates or replaces, PATCH renames or moves, and DELETE removes. Authentication is a bearer token in the `Authorization` header on every request, except for the health endpoints and one-time download links.

In the examples below, set the key once:

```bash
export CALLFS_API_KEY=your-api-key
```

Use the key printed during configuration instead of `your-api-key`.

### Health check

Confirm the server is up and responding:

```bash
curl http://localhost:8443/health
```

Expected response:

```json
{"status":"ok"}
```

The health endpoint requires no authentication and is suitable for load balancers and monitoring probes. `GET /health/ready` is a deeper check of this server: it returns `{"status":"ok"}` or `{"status":"degraded"}` with HTTP 200 while the server can work, and `{"status":"down"}` with HTTP 503 when it cannot. Problems with other servers are listed under `peers` and never fail readiness. The command `callfs healthcheck --config /etc/callfs/config.yaml` calls `/health` and reports the result through its exit code, which is handy for container health checks.

### Create a directory

CallFS uses a trailing slash to tell directories from files. Create a directory with a POST to a path that ends in a slash:

```bash
curl -X POST \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  http://localhost:8443/v1/files/projects/
```

A new directory returns `201 Created` and gets mode `0777`; new files get `0644`. The owner and administrators can do everything; other users follow the "other" bits of the mode (read lets them read or list, write lets them overwrite or create items, delete stays with the owner). Creating one that already exists returns `200 OK`, so the call is safe to repeat in setup scripts.

### Upload a file

Send the file content as the request body. CallFS stores the data and records its metadata (path, size, owner, modification time and a content tag called the ETag) in the metadata store. Create the parent directory first, as in the previous step:

```bash
curl -i -X POST \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @readme.txt \
  http://localhost:8443/v1/files/projects/readme.txt
```

A successful create returns `201 Created` and an `ETag` header. POST creates only: if the file already exists you get `409 Conflict`, and you use PUT to replace it. The largest upload is 10 GiB. For big files or unreliable networks, resumable uploads let you continue after a break (see [File Storage API](03-file-storage-api-rest-upload-download.md)).

By default CallFS tells you the file is saved only after it is on disk. A client can ask for a faster answer on a single request with the `X-CallFS-Write-Ack` header (`disk` or `memory`); see [Storage](../docs_markdown/05-storage.md) for the trade-off.

### Download a file

A GET request to any file path streams the content back. CallFS sets `Content-Length`, an `ETag`, and `Accept-Ranges: bytes`, so clients can show progress, resume, and fetch partial ranges:

```bash
curl -H "Authorization: Bearer $CALLFS_API_KEY" \
  http://localhost:8443/v1/files/projects/readme.txt \
  -o readme-copy.txt
```

To fetch only the first kilobyte, send a `Range` header; the server answers `206 Partial Content`:

```bash
curl -H "Authorization: Bearer $CALLFS_API_KEY" \
  -H "Range: bytes=0-1023" \
  http://localhost:8443/v1/files/projects/readme.txt
```

### List directory contents

Use the `/v1/directories/` prefix to list a directory. The response is JSON with each entry's name, path, type, size, mode, owner and modification time:

```bash
curl -H "Authorization: Bearer $CALLFS_API_KEY" \
  http://localhost:8443/v1/directories/projects/
```

Example response:

```json
{
  "path": "/projects",
  "type": "directory",
  "recursive": false,
  "count": 1,
  "items": [
    {
      "name": "readme.txt",
      "path": "/projects/readme.txt",
      "type": "file",
      "size": 1024,
      "mode": "0644",
      "owner": "admin",
      "mtime": "2026-04-11T10:00:00Z"
    }
  ]
}
```

Add `?recursive=true` (and optionally `&max_depth=3`) to list the whole tree. A listing returns at most 10,000 entries; when it is cut short the response carries `"truncated": true`.

For big directories, add `limit` to get pages with sorting and filtering:

```bash
curl -H "Authorization: Bearer $CALLFS_API_KEY" \
  "http://localhost:8443/v1/directories/projects/?limit=50&sort=mtime&order=desc&type=file&q=readme"
```

`limit` is 1 to 1000, `sort` is `name`, `size`, `mtime` or `type`, `order` is `asc` or `desc`, `type` is `file` or `directory`, `q` filters by name, and `include_hidden=false` hides names that start with a dot. When more entries remain, the response has a `next_cursor` that you pass back as `cursor`.

### Read file metadata

A HEAD request returns file metadata in response headers without transferring the body. It is a cheap way to check whether a file exists and how big it is:

```bash
curl -I \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  http://localhost:8443/v1/files/projects/readme.txt
```

CallFS returns these headers:

| Header | Description |
|---|---|
| `X-CallFS-Size` | File size in bytes |
| `X-CallFS-Mode` | Permission mode string, for example `0644` |
| `X-CallFS-Owner` | The user name that owns the file |
| `X-CallFS-MTime` | Last modification time |
| `X-CallFS-Type` | `file` or `directory` |
| `X-CallFS-Instance-ID` | The server that holds the file |
| `ETag` | Content tag for conditional requests |

### Update a file

Replace an existing file's content with a PUT request. PUT also creates the file when it does not exist:

```bash
curl -i -X PUT \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @readme-v2.txt \
  http://localhost:8443/v1/files/projects/readme.txt
```

The answer is `200 OK` for a replaced file and `201 Created` for a new one, with the new `ETag`. To avoid overwriting someone else's change, send `If-Match: "<etag>"` with the ETag you last saw; if the file changed, the request fails with `412 Precondition Failed`. Send `If-None-Match: *` to create only when nothing exists yet.

### Rename or move a file

A PATCH with a JSON body renames or moves a file or a whole directory:

```bash
curl -X PATCH \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"destination": "/archive/readme.txt", "create_parents": true}' \
  http://localhost:8443/v1/files/projects/readme.txt
```

Use `{"name": "new-name.txt"}` instead to rename in place. Send exactly one of `name` or `destination`. Add `"overwrite": true` to replace an existing target. The answer is `200 OK` with `{"path": "<new path>"}`.

### Delete a file

```bash
curl -X DELETE \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  http://localhost:8443/v1/files/projects/readme.txt
```

A successful delete returns `204 No Content`, and the file's data and metadata are removed for good. Deleting a path that does not exist returns `404`. Deleting a directory that still has content returns `409`; add `?recursive=true` to remove the directory with everything in it, or `?dry_run=true` to see what would be removed first.

If you want a delete you can undo, use the web console's "Move to trash" action (or a `trash` operation in a batch request). Trashed items stay for 30 days by default (`trash.retention`) and can be restored from the Trash page.

---

## Using the Web Console Instead

Everything above is also available in a browser. With a build that includes the console, open `http://localhost:8443/ui/` and paste your API key into the "Access key" field. The Files page lets you browse folders, upload files or whole folders, create folders, rename, move, download, create one-time download links and move items to trash. Users listed in `admin_users` also see Cluster, Insights, Activity and Settings pages for health, charts, logs and configuration.

If you started with the prebuilt binary, which has no console, nothing changes for the API. Switch to the Docker image or a `make build` binary when you want the console. See [Web console](../docs_markdown/08-web-console.md) for every page.

---

## Running as a systemd Service

For a production Linux file server, you want CallFS to start automatically on boot and restart on failure. Create a systemd unit file:

```bash
sudo tee /etc/systemd/system/callfs.service > /dev/null <<'EOF'
[Unit]
Description=CallFS REST API File Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=callfs
Group=callfs
WorkingDirectory=/var/lib/callfs
ExecStart=/usr/local/bin/callfs server --config /etc/callfs/config.yaml
Restart=always
RestartSec=5s
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/callfs
SyslogIdentifier=callfs

[Install]
WantedBy=multi-user.target
EOF
```

Use `Restart=always`, not `on-failure`. When an administrator restarts CallFS from the console, the process exits with code 75 and relies on systemd to start it again. Stop the foreground server from the previous step, then enable and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now callfs
sudo systemctl status callfs
```

View logs at any time with:

```bash
sudo journalctl -u callfs -f
```

Stopping the service (`SIGTERM`) lets CallFS finish open requests for up to 30 seconds before it exits.

---

## Enabling HTTPS

The HTTP configuration above is convenient for a local network or a trusted internal environment. For a publicly reachable file server, enable TLS by providing a certificate and key. If you have a certificate (for example from Let's Encrypt), update the `server` block:

```yaml
server:
  listen_addr: ":8443"
  protocol: "https"
  external_url: "https://files.example.com"
  cert_file: "/etc/callfs/server.crt"
  key_file: "/etc/callfs/server.key"
```

CallFS refuses to start with `protocol: https` unless both files are set. The `callfs` user must be able to read them. Then use `https://` in your `curl` commands. For a self-signed test certificate, add `-k` to `curl` only while testing; never skip certificate verification in production.

If a reverse proxy in front of CallFS ends TLS, keep `protocol: http` on the private side and list the proxy's address in `server.trusted_proxies` (for example `["10.0.0.0/24"]`) so CallFS logs and rate-limits the real client address from `X-Forwarded-For`.

Restart the service after any configuration change:

```bash
sudo systemctl restart callfs
```

---

## Next Steps

This tutorial covered a minimal single-node setup. CallFS supports considerably more:

- **Monitoring**: a Prometheus metrics endpoint on its own port, `/health/ready`, and Insights and Activity pages in the console for charts, logs and audit records. See [Monitoring and metrics](../docs_markdown/06-monitoring-metrics.md).
- **Clustering**: multi-node deployments on an embedded NATS server for metadata, locks and node messaging, with requests routed to the node that holds a file. See [Clustering](../docs_markdown/07-clustering.md).
- **S3 backend**: store files in any S3-compatible object store instead of local disk, with optional server-side encryption. See [Storage](../docs_markdown/05-storage.md).
- **Erasure coding**: split files into data and parity pieces spread across servers for fault tolerance.
- **One-time download links**: generate expiring, single-use download URLs for sharing a file without exposing an API key. See [Secure file sharing](02-secure-file-sharing-business.md).

Full documentation is in the [CallFS repository](https://github.com/ebogdum/callfs) under `docs_markdown/`: [Installation](../docs_markdown/01-installation.md), [Configuration](../docs_markdown/02-configuration.md), [API reference](../docs_markdown/03-api-reference.md), [Authentication and security](../docs_markdown/04-authentication-security.md), [Troubleshooting](../docs_markdown/09-troubleshooting.md).
