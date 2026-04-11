# Automating Backups with CallFS: Scripts, Cron Jobs, and Retention

Backing up servers, databases, and configuration files is one of those tasks that is easy to understand and easy to neglect. The usual obstacle is friction: backup tooling often requires agents, daemons, or proprietary clients installed on every machine you want to protect. CallFS removes that friction. Because it exposes every file operation as a standard HTTP endpoint, any machine that can run `curl` can push backups to your **file backup server** without installing anything beyond the binary already on the system.

This guide walks through building a complete, automated backup workflow on top of CallFS: writing a **backup script**, scheduling it with cron, listing and pruning old backups with a retention policy, verifying integrity, sharing specific backups via expiring links, and monitoring the whole pipeline with Prometheus metrics.

---

## Prerequisites

- A running CallFS instance (see the [setup guide](01-linux-file-server-setup.md) if you need to deploy one)
- An API key configured in `auth.api_keys`
- `curl` and standard POSIX shell utilities on the machines you are backing up
- Optional: `sha256sum` or `md5sum` for integrity verification

All examples use `https://fileserver:8443` as the CallFS URL. Replace that and `your-api-key` with your actual values.

---

## Writing the Backup Script

The core of any automated backup workflow is a script that collects data and pushes it to the **file server**. CallFS accepts uploads as plain HTTP POST requests with an `application/octet-stream` body, which means you can pipe data directly from any command — no temporary files required.

```bash
#!/bin/bash
set -euo pipefail

CALLFS_URL="https://fileserver:8443"
API_KEY="your-api-key"
DATE=$(date +%Y-%m-%d)

# Backup a database dump
pg_dump mydb | curl -s -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @- \
  "$CALLFS_URL/v1/files/backups/db-$DATE.sql"

# Backup a directory (tar it first)
tar czf - /etc/nginx/ | curl -s -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @- \
  "$CALLFS_URL/v1/files/backups/nginx-$DATE.tar.gz"
```

Save this to `/opt/scripts/backup.sh` and make it executable:

```bash
chmod 750 /opt/scripts/backup.sh
```

### How the script works

- `set -euo pipefail` causes the script to exit immediately if any command fails or any variable is unset, preventing silent partial backups.
- `pg_dump mydb` writes a SQL dump to stdout. The pipe sends that stream directly into `curl --data-binary @-`, which reads from stdin. The database dump is never written to disk on the backup source machine.
- `tar czf -` similarly streams a compressed archive of `/etc/nginx/` directly into `curl`.
- CallFS stores each upload atomically: the file either lands completely or not at all.

### Handling upload failures

To capture curl exit codes and log failures, extend the script with explicit error checking:

```bash
upload() {
  local src_cmd="$1"
  local dest_path="$2"

  if ! eval "$src_cmd" | curl -sf -X POST \
    -H "Authorization: Bearer $API_KEY" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @- \
    "$CALLFS_URL/v1/files/$dest_path"; then
    echo "[$(date -u +%FT%TZ)] ERROR: backup failed for $dest_path" >&2
    return 1
  fi

  echo "[$(date -u +%FT%TZ)] OK: $dest_path"
}

upload "pg_dump mydb"    "backups/db-$DATE.sql"
upload "tar czf - /etc/nginx/" "backups/nginx-$DATE.tar.gz"
```

The `-f` flag on `curl` causes it to return a non-zero exit code on HTTP errors (4xx, 5xx), so upload failures surface as script errors rather than being swallowed silently.

---

## Scheduling with Cron

Once the script is working manually, schedule it with a cron job. Open the system crontab:

```bash
sudo crontab -e
```

Add the following line to run the **backup script** daily at 02:00:

```
0 2 * * * /opt/scripts/backup.sh >> /var/log/callfs-backup.log 2>&1
```

This appends both stdout and stderr to a log file, giving you a record of every backup run including any error messages.

### Verifying the cron environment

Cron runs jobs with a minimal environment. If your script depends on environment variables (database passwords, the API key), source them explicitly at the top of the script or pass them through a credentials file:

```bash
# At the top of /opt/scripts/backup.sh
source /etc/callfs-backup.env
```

Create `/etc/callfs-backup.env` with mode `600` so only root can read it:

```bash
sudo install -m 600 /dev/null /etc/callfs-backup.env
# Then edit it:
sudo nano /etc/callfs-backup.env
```

Contents:

```bash
CALLFS_URL="https://fileserver:8443"
API_KEY="your-api-key"
PGPASSWORD="your-postgres-password"
```

---

## Listing Backups

To see all files currently stored under the `backups/` directory, use the directory listing API with the `recursive=true` query parameter:

```bash
curl -s \
  -H "Authorization: Bearer $API_KEY" \
  "https://fileserver:8443/v1/directories/backups/?recursive=true"
```

CallFS returns a JSON document listing every file with its name, path, size, and modification time:

```json
{
  "path": "backups",
  "type": "directory",
  "recursive": true,
  "count": 6,
  "items": [
    {
      "name": "db-2026-04-05.sql",
      "path": "backups/db-2026-04-05.sql",
      "type": "file",
      "size": 52428800,
      "mode": "-rw-r--r--",
      "owner": "",
      "mtime": "2026-04-05T02:03:11Z"
    }
  ]
}
```

You can pipe the output through `jq` to filter by type or sort by modification time for further processing in scripts.

---

## Retention: Deleting Old Backups Automatically

Keeping every backup indefinitely is impractical. The following script lists all files in the `backups/` directory, parses the modification time from each entry, and deletes anything older than 30 days using the CallFS DELETE API.

```bash
#!/bin/bash
set -euo pipefail

source /etc/callfs-backup.env

RETENTION_DAYS=30
CUTOFF=$(date -u -d "$RETENTION_DAYS days ago" +%s 2>/dev/null \
         || date -u -v-${RETENTION_DAYS}d +%s)   # macOS fallback

echo "[$(date -u +%FT%TZ)] Starting retention cleanup (keep=${RETENTION_DAYS}d)"

# Fetch directory listing as JSON, extract paths and mtimes
curl -sf \
  -H "Authorization: Bearer $API_KEY" \
  "$CALLFS_URL/v1/directories/backups/?recursive=true" \
| jq -r '.items[] | select(.type == "file") | "\(.mtime) \(.path)"' \
| while IFS=' ' read -r mtime path; do
    # Parse ISO 8601 mtime to epoch seconds
    file_epoch=$(date -u -d "$mtime" +%s 2>/dev/null \
                 || date -u -j -f "%Y-%m-%dT%H:%M:%SZ" "$mtime" +%s)

    if [ "$file_epoch" -lt "$CUTOFF" ]; then
      echo "Deleting $path (mtime=$mtime)"
      curl -sf -X DELETE \
        -H "Authorization: Bearer $API_KEY" \
        "$CALLFS_URL/v1/files/$path"
    fi
  done

echo "[$(date -u +%FT%TZ)] Retention cleanup complete"
```

Schedule this separately or append it to the main backup script after uploads finish. A common pattern is to run cleanup after the new backup succeeds, ensuring you never delete old backups before the new one is confirmed uploaded.

---

## Verifying Backup Integrity

Uploading a file is not the same as verifying the file is intact and retrievable. Add a verification step that downloads the most recent backup and checks its size or checksum against the source.

```bash
#!/bin/bash
set -euo pipefail

source /etc/callfs-backup.env

DATE=$(date +%Y-%m-%d)
BACKUP_PATH="backups/db-$DATE.sql"
LOCAL_DUMP=$(mktemp)

# Re-generate the dump locally for comparison
pg_dump mydb > "$LOCAL_DUMP"
LOCAL_HASH=$(sha256sum "$LOCAL_DUMP" | awk '{print $1}')
LOCAL_SIZE=$(wc -c < "$LOCAL_DUMP")

# Download the backup from CallFS
REMOTE_DUMP=$(mktemp)
curl -sf \
  -H "Authorization: Bearer $API_KEY" \
  "$CALLFS_URL/v1/files/$BACKUP_PATH" \
  -o "$REMOTE_DUMP"

REMOTE_HASH=$(sha256sum "$REMOTE_DUMP" | awk '{print $1}')
REMOTE_SIZE=$(wc -c < "$REMOTE_DUMP")

rm -f "$LOCAL_DUMP" "$REMOTE_DUMP"

if [ "$LOCAL_HASH" = "$REMOTE_HASH" ]; then
  echo "[$(date -u +%FT%TZ)] Verification PASSED: $BACKUP_PATH (${REMOTE_SIZE} bytes)"
else
  echo "[$(date -u +%FT%TZ)] Verification FAILED: hash mismatch" >&2
  echo "  local:  $LOCAL_HASH ($LOCAL_SIZE bytes)"
  echo "  remote: $REMOTE_HASH ($REMOTE_SIZE bytes)"
  exit 1
fi
```

For large backups where regenerating the source is expensive, you can instead use a HEAD request to check that the file's recorded size matches what you uploaded, without downloading the full content:

```bash
curl -sI \
  -H "Authorization: Bearer $API_KEY" \
  "$CALLFS_URL/v1/files/$BACKUP_PATH" \
| grep -i "x-callfs-size:"
```

The `X-CallFS-Size` header returns the stored file size in bytes. If it matches the size reported by your upload source, the file is almost certainly intact.

---

## Sharing a Backup with a Single-Use Link

Sometimes you need to hand a specific backup to a colleague or an auditor without giving them an API key. CallFS supports single-use, time-limited download links that expire automatically and become invalid after one download.

Generate a link for a specific backup file:

```bash
curl -s -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"path": "backups/db-2026-04-11.sql", "expiry_seconds": 3600}' \
  "https://fileserver:8443/v1/links/generate"
```

CallFS responds with a URL, token, and expiry timestamp:

```json
{
  "url": "https://fileserver:8443/download/a3f8c1d2e9b047f6...",
  "token": "a3f8c1d2e9b047f6...",
  "expires": "2026-04-11T15:30:00Z"
}
```

Send the `url` to the recipient. They can download the file directly in a browser or with curl — no API key required. The link is consumed on first use and cannot be replayed. The maximum expiry is 86400 seconds (24 hours).

---

## Scaling: Adding S3 Replication

A single-node **file server** storing backups only on local disk has an obvious weakness: a disk failure takes the backups with it. CallFS's HA replication feature lets you mirror every write to an S3-compatible object store in addition to local storage, so your backups exist in two independent locations simultaneously.

Update `/etc/callfs/config.yaml` on your CallFS node to enable replication:

```yaml
backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs/data"

ha:
  replication_enabled: true
  replica_backend: "s3"
  require_replica_success: true

  s3_access_key: "AKIAIOSFODNN7EXAMPLE"
  s3_secret_key: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
  s3_region: "us-east-1"
  s3_bucket_name: "my-callfs-backups"
  s3_server_side_encryption: "AES256"
  s3_acl: "private"
```

With `require_replica_success: true`, CallFS only acknowledges a backup upload as successful after confirming it has been written to both local disk and S3. Set this to `false` if you prefer best-effort replication that does not block the upload response.

For a custom S3-compatible endpoint (such as a self-hosted object store), add:

```yaml
  s3_endpoint: "https://s3.internal.example.com"
  s3_server_side_encryption: ""   # disable SSE for MinIO
```

Restart CallFS after any configuration change:

```bash
sudo systemctl restart callfs
```

From this point, every backup your script uploads is stored in both places without any changes to the **backup script** itself.

---

## Monitoring Backups

Knowing that your cron job is scheduled is not the same as knowing your backups are actually running. Use the `callfs_file_operations_total` Prometheus metric to verify that uploads are happening.

CallFS exposes metrics on a separate port (default `:9090`):

```bash
curl -s http://fileserver:9090/metrics | grep callfs_file_operations_total
```

Example output:

```
# HELP callfs_file_operations_total Total number of file operations
# TYPE callfs_file_operations_total counter
callfs_file_operations_total{backend_type="localfs",operation="create"} 42
callfs_file_operations_total{backend_type="localfs",operation="read"} 138
callfs_file_operations_total{backend_type="localfs",operation="delete"} 12
```

The `operation="create"` counter increments each time a file is uploaded. If you configure your cron to run daily and each run uploads two files (database dump + nginx config), you should see this counter increase by 2 each day.

### Prometheus alerting rule

Add a Prometheus alerting rule to fire if no backup uploads are detected within a 26-hour window (which covers the daily cron plus a two-hour grace period):

```yaml
groups:
  - name: callfs_backup
    rules:
      - alert: BackupMissing
        expr: |
          increase(callfs_file_operations_total{operation="create",backend_type="localfs"}[26h]) == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "No backup uploads detected in the last 26 hours"
```

For more targeted monitoring, track `callfs_http_requests_total` filtered by the `POST` method and `/v1/files/*` path to count only upload requests, or monitor `callfs_errors_total` to detect upload failures before they accumulate.

---

## Putting It All Together

A production-grade automated backup setup on CallFS looks like this:

1. **Backup script** at `/opt/scripts/backup.sh` — pipes database dumps and directory archives directly into CallFS via HTTP POST.
2. **Cron job** at `0 2 * * *` — runs the backup script nightly and logs output.
3. **Retention script** at `/opt/scripts/cleanup.sh` — lists the `backups/` directory, identifies files older than 30 days by their `mtime` field, and deletes them via the DELETE API.
4. **Verification step** — downloads the most recent backup and compares its SHA-256 hash (or size via the `X-CallFS-Size` header) against the source.
5. **S3 replication** in `config.yaml` — every upload is mirrored to object storage without touching the backup scripts.
6. **Prometheus monitoring** — `callfs_file_operations_total{operation="create"}` confirms uploads are running; an alert fires if the counter stops increasing.

The entire workflow requires no agents on the machines being backed up, no proprietary clients, and no changes when you add new servers — just a URL, an API key, and a shell script.
