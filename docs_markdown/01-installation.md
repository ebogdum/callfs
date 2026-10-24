# Installation

This guide covers how to install CallFS, start it for the first time and use every command of the `callfs` program.

CallFS is one static binary. A single node needs no other service: it can keep its metadata in a local SQLite file or in its own embedded NATS server. PostgreSQL and Redis are optional.

## System requirements

| Item | Requirement |
|---|---|
| Operating system | Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64), FreeBSD and OpenBSD (amd64, arm64) |
| Memory and CPU | Runs on a small VM. Memory use grows with `server.metadata_cache_size`, `nats.jetstream.max_memory` and the write buffer set by `backend.flush_max_pending_bytes` |
| Disk | A directory for file data (`backend.localfs_root_path`, default `/var/lib/callfs`). With the embedded NATS server, also a directory for its data (`nats.jetstream.store_dir`, default `./data/nats`) |
| Network | One TCP port for the API and the console (`server.listen_addr`, default `:8443`). Optional: a metrics port (`metrics.listen_addr`, default `:9090`), a UDP port for HTTP/3 (`server.quic_listen_addr`), and a cluster port between nodes (`nats.cluster_listen`, default `127.0.0.1:6222`) |
| TLS | By default `server.protocol` is `https` and needs `server.cert_file` and `server.key_file`. Set `server.protocol: http` for local testing or when a proxy ends TLS |
| Optional services | PostgreSQL 12 or later (`metadata_store.type: postgres`), Redis 6 or later (`metadata_store.type: redis` or `dlm.type: redis`), an S3-compatible bucket (`backend.s3_bucket_name`) |
| To build from source | Go 1.26 or later; Node.js 22 or later and npm to build the web console |

## Prebuilt binaries

Every release at [github.com/ebogdum/callfs/releases](https://github.com/ebogdum/callfs/releases) has one binary per platform and a `SHA256SUMS` file.

| Asset | Platform |
|---|---|
| `callfs-linux-amd64` | Linux, x86-64 |
| `callfs-linux-arm64` | Linux, ARM64 |
| `callfs-darwin-amd64` | macOS, Intel |
| `callfs-darwin-arm64` | macOS, Apple silicon |
| `callfs-windows-amd64.exe` | Windows, x86-64 |
| `callfs-freebsd-amd64`, `callfs-freebsd-arm64` | FreeBSD |
| `callfs-openbsd-amd64`, `callfs-openbsd-arm64` | OpenBSD |
| `SHA256SUMS` | SHA-256 checksum of every binary above |

The binaries are static (`CGO_ENABLED=0`) and include the web console at `/ui/`. Release builds are published with build provenance attestations.

Install one:

```bash
VERSION=v2.0.0
curl -LO https://github.com/ebogdum/callfs/releases/download/$VERSION/callfs-linux-amd64
curl -LO https://github.com/ebogdum/callfs/releases/download/$VERSION/SHA256SUMS
shasum -a 256 -c SHA256SUMS --ignore-missing
chmod +x callfs-linux-amd64
sudo install -m 755 callfs-linux-amd64 /usr/local/bin/callfs
callfs --help
```

`shasum` must print `OK` for your file. Stop if it prints `FAILED`. Always download from a pinned release tag, not from a "latest" link.

## Docker

### The image

The `Dockerfile` in the repository builds a small image that contains only the `callfs` binary (with the web console embedded) and CA certificates.

```bash
docker build -t callfs:2.0.0 \
  --build-arg VERSION=2.0.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) .
```

| Property | Value |
|---|---|
| Entry point | `/callfs` (no arguments means `server`) |
| User | `65532:65532` (non-root) |
| Exposed port | `8443` |
| Data directory | `/var/lib/callfs`, owned by the runtime user |
| Health check | `/callfs healthcheck` every 30 s, 5 s timeout, 10 s start period, 3 retries |
| Config | No config file inside. Pass settings as `CALLFS_*` environment variables or mount a file and pass `--config` |

Run one node with a local SQLite database and no TLS:

```bash
docker volume create callfs-data
docker run -d --name callfs -p 8443:8443 \
  -v callfs-data:/var/lib/callfs \
  -e CALLFS_SERVER__PROTOCOL=http \
  -e CALLFS_METADATA_STORE__TYPE=sqlite \
  -e CALLFS_METADATA_STORE__SQLITE_PATH=/var/lib/callfs/callfs.sqlite3 \
  -e CALLFS_DLM__TYPE=local \
  -e CALLFS_INSTANCE_DISCOVERY__INSTANCE_ID=node-1 \
  -e CALLFS_AUTH__API_KEY_USERS__ADMIN=your-api-key-0123456789 \
  -e CALLFS_AUTH__INTERNAL_PROXY_SECRET=$(openssl rand -hex 32) \
  -e CALLFS_AUTH__SINGLE_USE_LINK_SECRET=$(openssl rand -hex 32) \
  callfs:2.0.0
```

To use a config file instead, mount it and pass the path:

```bash
docker run -d -p 8443:8443 -v $PWD/config.yaml:/etc/callfs/config.yaml:ro \
  -v callfs-data:/var/lib/callfs callfs:2.0.0 server --config /etc/callfs/config.yaml
```

Because the container runs as user 65532, every directory CallFS writes to (file data, the SQLite file, `nats.jetstream.store_dir`) must be writable by that user. A named volume mounted at `/var/lib/callfs` already is.

The health check runs `callfs healthcheck` inside the container. It reads the same environment variables and config file as the server, so it follows your `server.listen_addr` and protocol.

### docker-compose.yml

The `docker-compose.yml` in the repository starts three services on a network named `callfs-network`:

| Service | Image | What it is | Ports |
|---|---|---|---|
| `postgres` | `postgres:17` | Metadata database `callfs`, with the runtime role `callfs_app` created by the scripts in `deploy/postgres/initdb`. Data is kept in memory (tmpfs) and is lost when the container stops. Health check: `pg_isready` every 10 s | `127.0.0.1:5432` |
| `redis` | `redis:7` | Lock manager, password protected, persistence off. Health check: `redis-cli ping` every 10 s | `127.0.0.1:6379` |
| `callfs` | built from the `Dockerfile` | The CallFS server. Starts only after `postgres` and `redis` are healthy | `8443` |

The `callfs` service runs with `server.protocol: http` on `:8443`, `metadata_store.type: postgres` (the app connects as `callfs_app`, migrations run as `callfs`) and `dlm.type: redis`. It mounts no volume, so file data lives inside the container; add a volume for `/var/lib/callfs` to keep it.

Compose refuses to start until these environment variables are set (in your shell or in a `.env` file next to `docker-compose.yml`):

| Variable | Used for |
|---|---|
| `CALLFS_POSTGRES_PASSWORD` | Password of the `callfs` database owner (runs migrations) |
| `CALLFS_POSTGRES_RUNTIME_PASSWORD` | Password of the `callfs_app` role the server uses |
| `CALLFS_REDIS_PASSWORD` | Redis password |
| `CALLFS_INTERNAL_PROXY_SECRET` | `auth.internal_proxy_secret` |
| `CALLFS_SINGLE_USE_LINK_SECRET` | `auth.single_use_link_secret` |
| `CALLFS_ADMIN_API_KEY` | API key of the user `admin` (`auth.api_key_users.admin`) |

Every secret and the API key must be at least 16 characters and must not contain placeholder text such as `your-` or `change-me`. The three secrets must all differ.

The server also requires `instance_discovery.instance_id`. Add it to the `callfs` service:

```yaml
    environment:
      CALLFS_INSTANCE_DISCOVERY__INSTANCE_ID: "node-1"
```

Start it:

```bash
export CALLFS_POSTGRES_PASSWORD=$(openssl rand -hex 24)
export CALLFS_POSTGRES_RUNTIME_PASSWORD=$(openssl rand -hex 24)
export CALLFS_REDIS_PASSWORD=$(openssl rand -hex 24)
export CALLFS_INTERNAL_PROXY_SECRET=$(openssl rand -hex 32)
export CALLFS_SINGLE_USE_LINK_SECRET=$(openssl rand -hex 32)
export CALLFS_ADMIN_API_KEY=$(openssl rand -hex 32)
docker compose up -d --build
curl http://localhost:8443/health
```

Open `http://localhost:8443/ui/` and sign in with the value of `CALLFS_ADMIN_API_KEY`. Keep these values: you need them again on every start.

## Build from source

Requirements: Go 1.26 or later, `make`, git. To include the web console: Node.js 22 or later with npm.

```bash
git clone https://github.com/ebogdum/callfs.git
cd callfs
make web-install   # npm ci in web/
make build-ui      # builds the console into web/build
make build         # builds ./callfs with the console embedded
```

### Makefile targets

| Target | What it does |
|---|---|
| `make web-install` | Installs the console's npm dependencies (`npm ci` in `web/`) |
| `make build-ui` | Builds the console into `web/build` (`npm run build` in `web/`) |
| `make build` | Builds `./callfs` with the console embedded (`go build -tags ui`) and the version, commit and build date stamped in |
| `make build-noui` | Builds `./callfs` without the console (`go build`, no `ui` tag) |
| `make test` | Runs the Go tests |
| `make test-ui` | Runs the Go tests with the `ui` tag |

`make build` embeds whatever is in `web/build`, and fails if that directory is missing. Run `make build-ui` first, and again whenever the console source changes.

`make build` takes the version from `git describe --tags --always --dirty`, the commit from `git rev-parse --short HEAD` and the date from the current UTC time. Override them: `make build VERSION=2.0.0`. The version, commit and build date are written to the log at startup.

### Building without the console

`make build-noui` produces a smaller binary that serves the API only. With `ui.enabled: true` (the default), the address `/ui/` then shows a "Console not available" page. Set `ui.enabled: false` to remove the console routes entirely. All API features work the same.

### Cross-compiling every release binary

`./builds.sh` builds all platforms of the table above in parallel into `builds/` and writes `builds/SHA256SUMS`. These are API-only binaries (no `ui` tag).

## First run

### 1. Write a minimal config

This config runs a single node with a local SQLite metadata file, local locks, files on disk and no TLS. It is a good start for a test machine.

```yaml
server:
  listen_addr: ":8443"
  protocol: "http"

instance_discovery:
  instance_id: "node-1"

metadata_store:
  type: "sqlite"
  sqlite_path: "/var/lib/callfs/callfs.sqlite3"

dlm:
  type: "local"

backend:
  localfs_root_path: "/var/lib/callfs"

auth:
  api_key_users:
    admin: "REPLACE_WITH_API_KEY"
  admin_users: ["admin"]
  internal_proxy_secret: "REPLACE_WITH_SECRET_1"
  single_use_link_secret: "REPLACE_WITH_SECRET_2"
```

The server refuses to start unless all of these are true:

- `instance_discovery.instance_id` is set (there is no default).
- At least one API key exists in `auth.api_key_users` or `auth.api_keys`.
- Each API key, `auth.internal_proxy_secret` and `auth.single_use_link_secret` is at least 16 characters, contains no placeholder text (`your-`, `change-me`, `a-strong-`), and is different from the others.
- User IDs in `auth.api_key_users` are not `root`, `internal-proxy` or `api-user-N`.
- Every ID in `auth.admin_users` is a configured user.
- With `server.protocol: https` (the default), `server.cert_file` and `server.key_file` are set.
- With the default `metadata_store.type: postgres`, `metadata_store.dsn` is set. With the default `dlm.type: redis`, a Redis server is reachable at `dlm.redis_addr`. The config above avoids both by choosing `sqlite` and `local`.

### 2. Generate the secrets and keys

Create the three values with any random generator:

```bash
openssl rand -hex 32   # run three times: API key, internal proxy secret, link secret
```

Put them in the config, or keep them out of the file and pass them as environment variables:

```bash
export CALLFS_AUTH__API_KEY_USERS__ADMIN=$(openssl rand -hex 32)
export CALLFS_AUTH__INTERNAL_PROXY_SECRET=$(openssl rand -hex 32)
export CALLFS_AUTH__SINGLE_USE_LINK_SECRET=$(openssl rand -hex 32)
```

Save the API key: it is what you type to sign in. Restrict the config file so only the CallFS user can read it: `chmod 600 config.yaml`.

Settings from environment variables always win over the file. The variable name is `CALLFS_` plus the key path in upper case with `__` between levels (for example `CALLFS_NATS__ROUTES`). Details are in [Configuration](02-configuration.md).

For HTTPS, create or obtain a certificate and key and set `server.protocol: https`, `server.cert_file` and `server.key_file`. For a quick self-signed test certificate:

```bash
openssl req -x509 -newkey rsa:4096 -nodes -days 365 \
  -keyout server.key -out server.crt -subj "/CN=files.example.com"
```

### 3. Check the config

```bash
callfs config validate --config ./config.yaml
```

It prints `Configuration is valid` and exits with 0, or prints the first problem and exits with 1.

### 4. Start the server

```bash
sudo mkdir -p /var/lib/callfs
callfs server --config ./config.yaml
```

Running `callfs` with no arguments is the same as `callfs server`. Without `--config`, CallFS reads `config.yaml`, then `config.yml`, then `config.json` from the current directory (the first one found), and falls back to defaults plus environment variables.

Check that it is up:

```bash
curl http://localhost:8443/health
callfs healthcheck --config ./config.yaml && echo healthy
```

Stop it with `Ctrl+C` or `SIGTERM`. CallFS then finishes open requests (up to 30 seconds) and exits with code 0.

### 5. Open the console and sign in

Go to `http://localhost:8443/ui/` (or `https://files.example.com/ui/`). The root address `/` redirects there. On the sign-in page, paste an API key from `auth.api_key_users` or `auth.api_keys` into the "Access key" field and select "Sign in". Users listed in `auth.admin_users` see the administration pages. The pages are described in [Web console](08-web-console.md), and sign-in and sessions in [Authentication and security](04-authentication-security.md).

### 6. Store a first file

```bash
curl -X PUT -H "Authorization: Bearer $CALLFS_AUTH__API_KEY_USERS__ADMIN" \
  --data-binary @report.pdf http://localhost:8443/v1/files/docs/report.pdf
curl -H "Authorization: Bearer $CALLFS_AUTH__API_KEY_USERS__ADMIN" \
  http://localhost:8443/v1/files/docs/report.pdf -o copy.pdf
```

The full API is in [API reference](03-api-reference.md).

## Single node or cluster

By default `nats.enabled` is `false` and CallFS runs as one node, using the metadata store and lock manager you choose (`sqlite`, `postgres`, `redis` or `nats` for metadata; `local`, `redis` or `nats` for locks).

You can also run a single node on the embedded NATS server, which gives you the same storage as a cluster and lets you add nodes later:

```yaml
nats:
  enabled: true
  jetstream:
    store_dir: "/var/lib/callfs/nats"

metadata_store:
  type: "nats"

dlm:
  type: "nats"
```

`nats.jetstream.store_dir` holds your metadata. Back it up together with the file data directory.

To run several nodes that share files, metadata and locks, set `nats.enabled: true`, `metadata_store.type: nats`, `dlm.type: nats`, give each node its own `instance_discovery.instance_id`, list the other nodes in `instance_discovery.peer_endpoints`, and use the same three secrets on every node, and give the cluster port TLS files (`nats.tls.*`; add `backend.internal_proxy_ca_file` when your certificates come from a private CA). See [Clustering](07-clustering.md) for the full procedure.

## Run as a systemd service

```bash
sudo useradd --system --shell /usr/sbin/nologin --home /var/lib/callfs callfs
sudo mkdir -p /var/lib/callfs /etc/callfs /opt/callfs
sudo chown callfs:callfs /var/lib/callfs
sudo install -m 755 callfs /opt/callfs/callfs
sudo install -m 600 -o callfs -g callfs config.yaml /etc/callfs/config.yaml
```

`/etc/systemd/system/callfs.service`:

```ini
[Unit]
Description=CallFS file server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=callfs
Group=callfs
WorkingDirectory=/var/lib/callfs
ExecStart=/opt/callfs/callfs server --config /etc/callfs/config.yaml
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/callfs

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now callfs
sudo systemctl status callfs
sudo journalctl -u callfs -f
```

Use `Restart=always`, not `on-failure`: when you restart CallFS from the console (see `ui.allow_remote_restart`), the process exits with code 75 and expects its supervisor to start it again. If you use PostgreSQL or Redis, add them to `After=` and `Wants=`.

## Command-line reference

```
callfs [command] [flags]
```

| Command | What it does |
|---|---|
| `callfs server` | Starts the server |
| `callfs config validate` | Checks the configuration and prints the main settings |
| `callfs healthcheck` | Asks the local server for `/health` and reports the result through the exit code |
| `callfs help [command]` | Shows help for a command |
| `callfs completion bash\|zsh\|fish\|powershell` | Prints a shell completion script |
| `callfs` (no arguments) | Same as `callfs server` |

Every command accepts `-h` / `--help`.

### callfs server

Starts the API, the web console, the metrics port, background jobs and, if enabled, the embedded NATS server and the HTTP/3 listener.

| Flag | Short | Default | What it does |
|---|---|---|---|
| `--config` | `-c` | none | Path of the config file (`.yaml`, `.yml` or `.json`). A missing file is an error. When omitted, `config.yaml`, `config.yml` or `config.json` in the current directory is used if present |

```bash
callfs server --config /etc/callfs/config.yaml
```

Exit codes:

| Code | Meaning |
|---|---|
| 0 | Stopped cleanly after `SIGINT` or `SIGTERM` |
| 1 | Failed to start or failed while running. The reason is printed as `Error: ...` |
| 75 | Restart requested from the console or the admin API. Your service manager should start CallFS again |

### callfs config validate

Loads the config file, environment variables and defaults, applies every validation rule and prints:

```
Validating configuration...
Configuration is valid
Instance ID: node-1
Listen Address: :8443
Metadata Store DSN: postgres://db.example.com:5432
Redis Address: localhost:6379
Local FS Root: /var/lib/callfs
```

The metadata DSN is shown with credentials and path removed. `S3 Bucket` and `S3 Region` lines are added when an S3 bucket is configured.

| Flag | Short | Default | What it does |
|---|---|---|---|
| `--config` | `-c` | none | Path of the config file. Same lookup as `server` |

| Exit code | Meaning |
|---|---|
| 0 | The configuration is valid |
| 1 | The configuration is invalid. The line `Configuration validation failed: <reason>` is printed. Configs that use the removed Raft mode fail here too |

`validate` only checks the settings. It does not connect to PostgreSQL, Redis, S3 or other nodes.

### callfs healthcheck

Sends `GET /health` to the local server and exits. It builds the address from `server.listen_addr`: an empty host, `0.0.0.0` or `::` becomes `localhost`. It uses HTTPS when both `server.cert_file` and `server.key_file` are set, otherwise HTTP. For `localhost` it does not verify the certificate. It waits up to 5 seconds.

| Flag | Short | Default | What it does |
|---|---|---|---|
| `--config` | `-c` | none | Path of the config file. Same lookup as `server` |

| Exit code | Meaning |
|---|---|
| 0 | `/health` answered `200 OK` |
| 1 | The config could not be loaded, `server.listen_addr` is invalid, the request failed (message `health check failed: ...`), or `/health` answered another status (message `health check returned status N`) |

```bash
callfs healthcheck --config /etc/callfs/config.yaml || echo "not healthy"
```

### Removed commands

`callfs cluster join` and `callfs cluster remove` no longer exist. Nodes join a cluster through the `nats` and `instance_discovery` settings; see [Clustering](07-clustering.md).

## Upgrading to 2.0.0

- Raft cluster mode was removed in 2.0.0. Configs with `metadata_store.type: raft` or `dlm.type: raft` are rejected at startup. Use `metadata_store.type: nats` and `dlm.type: nats`, and add the `nats` section with `nats.enabled: true`.
- The `callfs cluster join` and `callfs cluster remove` commands and the old Raft endpoints are gone.
- The Go module path is `github.com/ebogdum/callfs/v2`. Update imports and `go install` paths.
- `instance_discovery.instance_id` must be set on every node.
- Keep the same `auth.api_key_users` entries (or the same order in `auth.api_keys`) so file owners stay the same.
- Run `callfs config validate --config ...` with the new binary before you restart the service.
- The web console at `/ui/` is part of the Docker image and of `make build`. Prebuilt release binaries and `make build-noui` do not include it.

## Next steps

- [Configuration](02-configuration.md): every setting, its default and its environment variable
- [Authentication and security](04-authentication-security.md): users, API keys and TLS
- [Storage](05-storage.md): local disk, S3, erasure coding and write safety
- [Clustering](07-clustering.md): run several nodes
- [Troubleshooting](09-troubleshooting.md)
