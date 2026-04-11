# Run CallFS on a Raspberry Pi as a Home File Server

CallFS ships as a single statically linked binary with no runtime dependencies. That makes it a natural fit for a Raspberry Pi home file server: download one file, write one config, and you have a fully operational REST API file server running on ARM64 hardware. This guide walks through the complete setup — from installing the binary to configuring a systemd service and running an automated backup cron job from other machines on your network.

---

## What You Need

- A Raspberry Pi 4 or Pi 5, or any other ARM64 Linux single-board computer
- A USB or NVMe storage drive for file storage (the Pi's SD card is not recommended for sustained write workloads)
- A wired or wireless network connection
- A second machine (laptop, desktop, or another server) for testing uploads and downloads
- `curl` installed on both machines — available by default on most Linux distributions and macOS

No Go runtime, no Python, no package manager setup. CallFS is a single binary.

---

## Installation

Download the ARM64 binary directly from the latest GitHub release, make it executable, and move it into your path:

```bash
curl -Lo callfs https://github.com/ebogdum/callfs/releases/latest/download/callfs-linux-arm64
chmod +x callfs
sudo mv callfs /usr/local/bin/
```

Verify the installation:

```bash
callfs version
```

You should see the version string printed to stdout. The binary is now available system-wide.

---

## Prepare Storage

Identify your external drive and mount it. The example below assumes your drive appears as `/dev/sda1`. Adjust the device path to match your system.

```bash
sudo mkdir -p /mnt/storage
sudo mount /dev/sda1 /mnt/storage
```

To mount the drive automatically on every boot, add an entry to `/etc/fstab`. Find the drive's UUID first:

```bash
sudo blkid /dev/sda1
```

Then add a line like the following to `/etc/fstab`, replacing the UUID with your own:

```
UUID=your-drive-uuid  /mnt/storage  ext4  defaults,nofail  0  2
```

Create the directories CallFS will use for file storage and its metadata database:

```bash
sudo mkdir -p /mnt/storage/callfs
sudo mkdir -p /opt/callfs
```

---

## Configuration

CallFS reads a YAML configuration file at startup. Create the config directory and write the file:

```bash
sudo mkdir -p /etc/callfs
sudo nano /etc/callfs/config.yaml
```

Paste the following configuration. This is a minimal home server setup using local disk storage and SQLite for metadata — no external services required:

```yaml
server:
  listen_addr: ":8443"
  protocol: "http"
  read_timeout: 30s
  write_timeout: 60s

auth:
  api_keys:
    - "my-home-server-key-change-this"
  internal_proxy_secret: "internal-secret-change-this"
  single_use_link_secret: "link-secret-change-this-too"

backend:
  default_backend: "localfs"
  localfs_root_path: "/mnt/storage/callfs"

metadata_store:
  type: "sqlite"
  sqlite_path: "/opt/callfs/metadata.sqlite3"

dlm:
  type: "local"

instance_discovery:
  instance_id: "pi-server"
```

### Configuration notes

- **`listen_addr`** — `:8443` binds to all network interfaces on port 8443. You can use any unprivileged port (1024 and above).
- **`protocol`** — `http` is fine on a trusted home network. If you later expose the server outside your LAN, switch this to `https` and provide `cert_file` and `key_file` paths.
- **`api_keys`** — replace the placeholder with a random string of at least 24 characters. This token must appear in the `Authorization: Bearer` header on every request. You can add multiple keys to support rotation without downtime.
- **`internal_proxy_secret`** and **`single_use_link_secret`** — both fields are required even on a single-node home server. Replace the placeholders with unique random strings.
- **`localfs_root_path`** — the directory on your storage drive where uploaded files land. Ensure it exists and is writable by the user running CallFS.
- **`sqlite_path`** — where CallFS writes its metadata database. SQLite works well for a home server with a single Pi.
- **`dlm.type: local`** — an in-process lock manager, appropriate for single-node deployments.
- **`instance_id`** — a human-readable identifier for this node. Any string works.

---

## Start the Server and Test It

Run CallFS directly to confirm it starts without errors:

```bash
callfs server --config /etc/callfs/config.yaml
```

You should see structured log output showing the server is listening. Leave this running and open a second terminal on the Pi to run a quick health check:

```bash
curl http://localhost:8443/health
```

Expected response:

```json
{"status":"ok"}
```

The health endpoint requires no authentication and always responds as long as the server is up.

---

## Upload and Download from Another Machine

From any other machine on your home network, replace `192.168.1.100` with the Pi's actual IP address, and `YOUR_API_KEY` with the key you set in `config.yaml`.

### Create a directory

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  http://192.168.1.100:8443/v1/files/photos/
```

### Upload a file

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @vacation.jpg \
  http://192.168.1.100:8443/v1/files/photos/vacation.jpg
```

### Download a file

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://192.168.1.100:8443/v1/files/photos/vacation.jpg \
  -o vacation.jpg
```

### List directory contents

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://192.168.1.100:8443/v1/directories/photos/
```

The response is JSON and includes each entry's name, type, size, and modification time. Append `?recursive=true` to list all subdirectories in one call.

---

## Run as a systemd Service

For a home file server setup that survives reboots, configure CallFS as a systemd service.

Create the unit file:

```bash
sudo tee /etc/systemd/system/callfs.service > /dev/null <<EOF
[Unit]
Description=CallFS REST API File Server
After=network.target local-fs.target

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

The `local-fs.target` dependency ensures the external storage drive is mounted before CallFS tries to use it.

Enable and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable callfs
sudo systemctl start callfs
```

Check the current status:

```bash
sudo systemctl status callfs
```

Follow logs in real time:

```bash
sudo journalctl -u callfs -f
```

CallFS will now start automatically on every boot and restart itself if it crashes.

---

## Resource Usage

CallFS is written in Go and compiles to a single statically linked binary. On a Raspberry Pi 4 or 5, expect the following:

- **Binary size** — approximately 25 MB on disk
- **RAM** — typically 20 to 40 MB at rest, scaling modestly with concurrent connections
- **CPU** — near-zero at idle; a brief spike during uploads, proportional to file size and connection count

The Pi 4 has 2 to 8 GB of RAM depending on the model. Even the 2 GB variant leaves ample headroom to run CallFS alongside other home server workloads.

---

## Optional: Add an S3 Backend for Cloud Backup

If you want files uploaded to your Pi to also go to cloud storage, CallFS supports an S3-compatible backend. Add the following block to your `config.yaml` and change `default_backend` to `s3`:

```yaml
backend:
  default_backend: "s3"
  localfs_root_path: "/mnt/storage/callfs"
  s3_bucket: "my-home-backup-bucket"
  s3_region: "us-east-1"
  s3_access_key: "YOUR_ACCESS_KEY"
  s3_secret_key: "YOUR_SECRET_KEY"
```

You can keep the `localfs_root_path` present in the config even when using S3 — it is used only when `default_backend` is set to `localfs`. Restart the service after changing the configuration:

```bash
sudo systemctl restart callfs
```

---

## Automated Backup from Other Machines

A common home server use case is having other machines on the network automatically push files to the Pi. The following shell script uploads every file in a directory to CallFS and can be run from any machine that has `curl` available.

### Backup script

Create `/usr/local/bin/callfs-backup.sh` on the machine you want to back up:

```bash
#!/usr/bin/env bash
set -euo pipefail

CALLFS_HOST="http://192.168.1.100:8443"
CALLFS_KEY="YOUR_API_KEY"
SOURCE_DIR="$HOME/Documents"
REMOTE_DIR="backups/documents"

# Ensure the remote directory exists
curl -sf -X POST \
  -H "Authorization: Bearer ${CALLFS_KEY}" \
  "${CALLFS_HOST}/v1/files/${REMOTE_DIR}/" || true

# Upload each file in the source directory
find "${SOURCE_DIR}" -maxdepth 1 -type f | while read -r file; do
  filename="$(basename "${file}")"
  curl -sf -X PUT \
    -H "Authorization: Bearer ${CALLFS_KEY}" \
    -H "Content-Type: application/octet-stream" \
    --data-binary "@${file}" \
    "${CALLFS_HOST}/v1/files/${REMOTE_DIR}/${filename}"
  echo "Uploaded: ${filename}"
done
```

Make it executable:

```bash
chmod +x /usr/local/bin/callfs-backup.sh
```

### Schedule with cron

Run the backup script every night at 2:00 AM by adding it to crontab:

```bash
crontab -e
```

Add the following line:

```
0 2 * * * /usr/local/bin/callfs-backup.sh >> /var/log/callfs-backup.log 2>&1
```

The script uses PUT so that re-running it overwrites existing files rather than creating duplicates. Logs accumulate in `/var/log/callfs-backup.log` for review.

---

## Next Steps

This guide covered a complete single-Pi home file server setup. CallFS supports considerably more for when your needs grow:

- **HTTPS** — add `cert_file` and `key_file` to the `server` block to enable TLS, useful if you want to reach the server from outside your home network
- **Clustering** — run a second Pi as a replica with Raft-based metadata consensus and automatic failover
- **Erasure coding** — distribute file shards across multiple nodes for fault tolerance without full replication overhead
- **Prometheus metrics** — a configurable metrics endpoint for tracking request rates, latency, and storage utilization
- **Single-use download links** — generate time-limited, unauthenticated URLs for sharing specific files without exposing your API key

Full documentation is available in the [CallFS repository](https://github.com/ebogdum/callfs) under the `docs/` directory.
