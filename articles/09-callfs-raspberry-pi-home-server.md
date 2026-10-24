# Run CallFS on a Raspberry Pi as a Home File Server

CallFS ships as a single statically linked binary with no runtime dependencies. That makes it a natural fit for a Raspberry Pi home file server: download one file, write one config, and you have a fully operational file server with a REST API on ARM64 hardware. This guide walks through the complete setup for CallFS 2.0.0: installing the binary, preparing a drive, writing a config, creating the TLS certificate, running CallFS as a systemd service, sharing files with expiring links, and backing up other machines to the Pi.

---

## What You Need

- A Raspberry Pi 4 or Pi 5, or any other ARM64 Linux single-board computer
- A USB or NVMe drive for file storage (the Pi's SD card is not recommended for sustained write workloads)
- A wired or wireless network connection
- A second machine (laptop, desktop, or another server) for testing uploads and downloads
- `curl` and `openssl` on the Pi, and `curl` on the second machine

No Go runtime, no database server, no package manager setup. CallFS keeps its file index in a single SQLite file.

---

## Installation

Download the ARM64 binary from a pinned release, check its checksum and install it:

```bash
VERSION=v2.0.0
curl -LO https://github.com/ebogdum/callfs/releases/download/$VERSION/callfs-linux-arm64
curl -LO https://github.com/ebogdum/callfs/releases/download/$VERSION/SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
sudo install -m 755 callfs-linux-arm64 /usr/local/bin/callfs
callfs --help
```

`sha256sum` must print `OK`. Stop if it prints `FAILED`.

The release binaries include the web console (a browser app for browsing files, creating share links and checking server health) at `/ui/`. Everything in this guide also works from the command line.

---

## Prepare Storage

Identify your external drive and mount it. The example assumes it appears as `/dev/sda1`. Adjust the device path to match your system.

```bash
sudo mkdir -p /mnt/storage
sudo mount /dev/sda1 /mnt/storage
```

To mount the drive on every boot, add an entry to `/etc/fstab`. Find the drive's UUID first:

```bash
sudo blkid /dev/sda1
```

Then add a line like this, with your own UUID:

```
UUID=your-drive-uuid  /mnt/storage  ext4  defaults,nofail  0  2
```

Create a user for CallFS and the folders it needs. Keep the file index outside the folder that holds your files:

```bash
sudo useradd --system --shell /usr/sbin/nologin --home /var/lib/callfs callfs
sudo mkdir -p /mnt/storage/callfs-files /mnt/storage/callfs-index /etc/callfs/tls
sudo chown callfs:callfs /mnt/storage/callfs-files /mnt/storage/callfs-index
sudo chmod 750 /mnt/storage/callfs-files /mnt/storage/callfs-index
```

---

## Create the TLS Certificate and the Secrets

HTTPS keeps your access key encrypted on your home network. Create a self-signed certificate once. Replace `192.168.1.100` with the Pi's address:

```bash
sudo openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
  -keyout /etc/callfs/tls/server.key -out /etc/callfs/tls/server.crt \
  -subj "/CN=192.168.1.100" -addext "subjectAltName=IP:192.168.1.100"
sudo chown callfs:callfs /etc/callfs/tls/server.key /etc/callfs/tls/server.crt
sudo chmod 600 /etc/callfs/tls/server.key
```

Generate three random secrets: one access key and two server secrets. Run the command three times and keep the output:

```bash
openssl rand -hex 32
```

CallFS refuses to start with a key that is shorter than 16 characters or that contains `your-`, `change-me` or `a-strong-`, and the three values must all be different.

---

## Configuration

Write this file as `config.yaml` in your current folder:

```yaml
server:
  listen_addr: "192.168.1.100:8443"
  protocol: "https"
  external_url: "https://192.168.1.100:8443"
  cert_file: "/etc/callfs/tls/server.crt"
  key_file: "/etc/callfs/tls/server.key"

auth:
  api_key_users:
    pi-admin: "REPLACE_WITH_RANDOM_ACCESS_KEY"
  admin_users: ["pi-admin"]
  internal_proxy_secret: "REPLACE_WITH_RANDOM_INTERNAL_SECRET"
  single_use_link_secret: "REPLACE_WITH_RANDOM_LINK_SECRET"

backend:
  default_backend: "localfs"
  localfs_root_path: "/mnt/storage/callfs-files"

metadata_store:
  type: "sqlite"
  sqlite_path: "/mnt/storage/callfs-index/callfs.sqlite3"

dlm:
  type: "local"

metrics:
  listen_addr: "127.0.0.1:9090"

instance_discovery:
  instance_id: "pi-server"
```

Install it where the service will read it and check it:

```bash
sudo install -m 600 -o callfs -g callfs config.yaml /etc/callfs/config.yaml
callfs config validate --config /etc/callfs/config.yaml
```

`callfs config validate` prints the loaded settings or the first error, such as `auth.internal_proxy_secret must be at least 16 characters`.

### Configuration notes

- **`listen_addr`**: `192.168.1.100:8443` binds only to the Pi's LAN address. Use `:8443` to listen on every address, or `127.0.0.1:8443` to allow only the Pi itself. Any unprivileged port (1024 and above) works.
- **`external_url`**: the address CallFS puts inside the download links it creates. Use the address your other devices use to reach the Pi.
- **`api_key_users`**: maps a user name to a secret key. The name is stored as the owner of every file that key uploads. The key goes in the `Authorization: Bearer` header on every request. You can add more users, for example one key per family member or per backup job.
- **`admin_users`**: users who may use the admin API and the administration pages of the console.
- **`internal_proxy_secret`** and **`single_use_link_secret`**: both are required even on a single machine. The first is for server-to-server traffic, the second signs download links. They must be at least 16 characters and different from each other and from every key.
- **`localfs_root_path`**: the folder on your drive where uploaded files are stored. It must exist and be writable by the user that runs CallFS.
- **`sqlite_path`**: where CallFS keeps its file index. SQLite suits a single Pi well. CallFS creates the file and restricts its permissions.
- **`dlm.type: local`**: in-process locks, correct for a single machine.
- **`metrics.listen_addr`**: the metrics port is unauthenticated and defaults to `:9090` on every address. Binding it to `127.0.0.1` keeps it private to the Pi. Set it to an empty string to turn it off.
- **`instance_id`**: a name for this server. Letters, digits, dot, dash and underscore.

You can set any value from environment variables instead. The prefix is `CALLFS_` and nesting uses a double underscore, for example `CALLFS_SERVER__LISTEN_ADDR` or `CALLFS_AUTH__API_KEY_USERS__PI_ADMIN`. See [Configuration](../docs_markdown/02-configuration.md).

### Protection against power loss

A home server loses power more often than a data center. The defaults already protect you: `backend.localfs_sync: full` forces every saved file to the physical disk, and `backend.localfs_ack: disk` only tells the client "saved" after that. A file that CallFS confirmed survives a power cut. Changing these to `none` or `memory` is faster but can lose the last files saved before a power cut. For bulk copies you can repeat, a single request can ask for the faster behavior with the header `X-CallFS-Write-Ack: memory`.

---

## Start the Server and Test It

Run CallFS in the foreground to confirm it starts:

```bash
sudo -u callfs callfs server --config /etc/callfs/config.yaml
```

In a second terminal on the Pi:

```bash
curl --cacert /etc/callfs/tls/server.crt https://192.168.1.100:8443/health
```

```json
{"status":"ok"}
```

`/health` needs no key and answers as long as the process is up. `/health/ready` tests this server's file index, locks, disk space and certificate, and answers `ok`, `degraded` or `down` (HTTP 503). Problems with other servers never fail it; they are listed under `peers`. You can run `callfs healthcheck --config /etc/callfs/config.yaml` for a quick local check.

Copy `/etc/callfs/tls/server.crt` to your other machine so it can trust the Pi, and use it as `--cacert server.crt` in the examples below. Adding `-k` also works but skips verification.

---

## Upload and Download from Another Machine

Replace `192.168.1.100` with the Pi's address and `your-api-key` with the key you put in `config.yaml`.

### Upload a file

```bash
curl --cacert server.crt -X PUT \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @vacation.jpg \
  https://192.168.1.100:8443/v1/files/photos/vacation.jpg
```

`PUT` creates the file (`201 Created`) or replaces it (`200 OK`). The `photos/` folder is created automatically. `POST` also creates a file, but it returns `409` if the file already exists.

### Create an empty folder

End the path with a slash:

```bash
curl --cacert server.crt -X POST \
  -H "Authorization: Bearer your-api-key" \
  https://192.168.1.100:8443/v1/files/photos/2026/
```

A new folder returns `201`, and a folder that already exists returns `200`.

### Download a file

```bash
curl --cacert server.crt -H "Authorization: Bearer your-api-key" \
  https://192.168.1.100:8443/v1/files/photos/vacation.jpg \
  -o vacation.jpg
```

Add a `Range` header (for example `-H "Range: bytes=0-1048575"`) to fetch part of a large file.

### List a folder

```bash
curl --cacert server.crt -H "Authorization: Bearer your-api-key" \
  https://192.168.1.100:8443/v1/directories/photos/
```

The JSON answer has `items`, each with `name`, `path`, `type`, `size`, `mode`, `owner` and `mtime`. Add `?recursive=true` to include subfolders, or `?limit=100` for a paged listing with a `next_cursor`.

### Delete a file

```bash
curl --cacert server.crt -X DELETE -H "Authorization: Bearer your-api-key" \
  https://192.168.1.100:8443/v1/files/photos/vacation.jpg
```

This answers `204` and removes the file for good. Deleting from the console moves the file to the trash for 30 days (`trash.retention`) so you can restore it.

### Share a file with a link

To give a friend one file without giving them your key, create a single-use link that expires in a day:

```bash
curl --cacert server.crt -X POST \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"path":"/photos/vacation.jpg","expiry_seconds":86400}' \
  https://192.168.1.100:8443/v1/links/generate
```

Send the `url` from the answer. Whoever opens it presses **Download file** once. Details are in [Expiring download links](08-expiring-download-links.md). Links only work for people who can reach the Pi's address, so for sharing outside your home you need a reachable address (a VPN or a reverse proxy with its own certificate) and `server.external_url` set to it.

---

## Run as a systemd Service

Create the unit file:

```bash
sudo tee /etc/systemd/system/callfs.service > /dev/null <<'EOF'
[Unit]
Description=CallFS file server
After=network-online.target
Wants=network-online.target
RequiresMountsFor=/mnt/storage

[Service]
Type=simple
User=callfs
Group=callfs
ExecStart=/usr/local/bin/callfs server --config /etc/callfs/config.yaml
Restart=always
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/mnt/storage/callfs-files /mnt/storage/callfs-index /etc/callfs
StandardOutput=journal
StandardError=journal
SyslogIdentifier=callfs

[Install]
WantedBy=multi-user.target
EOF
```

`/etc/callfs` is writable so settings you save from the console can be written to the config file. `RequiresMountsFor=/mnt/storage` makes systemd wait for the external drive, so CallFS never starts against an empty mount point. Use `Restart=always`: when you restart CallFS from the console (`ui.allow_remote_restart`), it exits with code 75 and expects the service manager to start it again.

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now callfs
sudo systemctl status callfs
sudo journalctl -u callfs -f
```

CallFS now starts on every boot and restarts if it stops. With the default `log.format: json`, the journal shows one structured entry per event. Use `log.format: text` and `log.level: debug` while you troubleshoot.

---

## Resource Usage

CallFS is a single Go process with no other service to run beside it. Memory depends mostly on three settings:

| Setting | Default | Effect |
|---------|---------|--------|
| `server.metadata_cache_size` | `1000` | Number of file index answers kept in memory. |
| `backend.flush_max_pending_bytes` | `256 MiB` | Cap on data that is saved but not yet flushed to disk. Writes wait when the cap is reached. Lower it on a Pi with 2 GB of RAM, for example `67108864` (64 MiB). |
| `server.transfer_timeout` | `1h` | Longest time for one upload or download. Raise it if you move very large files over a slow link. |

At idle CallFS does almost nothing. CPU use rises during uploads in proportion to file size and the number of simultaneous transfers.

---

## Optional: Keep a Second Copy in the Cloud

If you want every file uploaded to your Pi to also be copied to an S3-compatible bucket, keep local disk as the main storage and turn on the backup copy. The S3 settings go under `backend:`:

```yaml
backend:
  default_backend: "localfs"
  localfs_root_path: "/mnt/storage/callfs-files"
  s3_bucket_name: "my-home-backup-bucket"
  s3_region: "us-east-1"
  s3_access_key: "your-s3-access-key"
  s3_secret_key: "your-s3-secret-key"

ha:
  replication_enabled: true
  replica_backend: "s3"
  require_replica_success: false
```

With `require_replica_success: false` an upload is confirmed once it is on the Pi, and the cloud copy follows in the background. Set it to `true` if an upload must fail when the cloud copy fails. Deletes remove the cloud copy too. Restart the service after changing the config:

```bash
sudo systemctl restart callfs
```

See [Storage](../docs_markdown/05-storage.md#backup-copy-replication) for the details, including how to retry failed copies.

---

## Automated Backup from Other Machines

A common home server use case is having other machines push files to the Pi on a schedule. This script uploads the files in a folder and works on any machine that has `curl` and `jq`.

### Backup script

Create `/usr/local/bin/callfs-backup.sh` on the machine you want to back up:

```bash
#!/usr/bin/env bash
set -euo pipefail

CALLFS_HOST="https://192.168.1.100:8443"
CALLFS_KEY="your-api-key"
CA_FILE="/etc/callfs-server.crt"
SOURCE_DIR="$HOME/Documents"
REMOTE_DIR="backups/documents"

find "${SOURCE_DIR}" -maxdepth 1 -type f -print0 | while IFS= read -r -d '' file; do
  filename="$(basename "${file}")"
  encoded="$(jq -rn --arg name "${filename}" '$name|@uri')"
  curl -sf --cacert "${CA_FILE}" -X PUT \
    -H "Authorization: Bearer ${CALLFS_KEY}" \
    -H "Content-Type: application/octet-stream" \
    --data-binary "@${file}" \
    "${CALLFS_HOST}/v1/files/${REMOTE_DIR}/${encoded}"
  echo "Uploaded: ${filename}"
done
```

The `backups/documents/` folders are created on the first upload. Make the script executable:

```bash
chmod 700 /usr/local/bin/callfs-backup.sh
```

### Schedule with cron

Run it every night at 2:00 AM:

```bash
crontab -e
```

```
0 2 * * * /usr/local/bin/callfs-backup.sh >> "$HOME/callfs-backup.log" 2>&1
```

The script uses `PUT`, so re-running it overwrites existing files instead of failing with `409`. `curl -f` makes the script stop with an error if the Pi rejects an upload. For retention, verification and monitoring of backups, see [Automating backups with CallFS](07-automated-backups-callfs.md).

---

## Next Steps

- **Web console**: build CallFS with `make build` to browse files, create share links, see health findings and change settings from a browser at `https://192.168.1.100:8443/ui/`. You can also drag files and whole folders onto the Files page, and set the sign-out time for an idle browser with `ui.idle_timeout`. See [Web console](../docs_markdown/08-web-console.md).
- **More users**: add one entry per person or job to `auth.api_key_users`. Changes apply without a restart.
- **Clustering**: run three Pis as one cluster with embedded NATS. Any Pi accepts any request, and the cluster survives the loss of one. If your Pis use certificates from your own CA, set `backend.internal_proxy_ca_file` so they trust each other. See [Setting up a CallFS cluster](06-callfs-clustering-nats.md).
- **Erasure coding**: spread files across several disks or nodes so a lost drive does not lose data. Set `erasure.min_file_size` to split large files automatically. See [Erasure coding](05-callfs-erasure-coding.md).
- **Monitoring**: Prometheus metrics on the metrics port show request rates, latency and storage use. See [Prometheus and Grafana](10-callfs-prometheus-grafana-monitoring.md).

Full documentation is in the [CallFS repository](https://github.com/ebogdum/callfs) under `docs_markdown/`.
