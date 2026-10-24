# Automating Backups with CallFS: Scripts, Cron Jobs, and Retention

Backing up servers, databases, and configuration files is one of those tasks that is easy to understand and easy to neglect. The usual obstacle is friction: backup tooling often requires agents, daemons, or proprietary clients installed on every machine you want to protect. CallFS removes that friction. Because it exposes every file operation as a standard HTTP endpoint, any machine that can run `curl` can push backups to your **file backup server** without installing anything.

This guide walks through a complete, automated backup workflow on top of CallFS 2.0.0: writing a **backup script**, scheduling it with cron, listing and pruning old backups with a retention policy, verifying integrity with the file's SHA-256 tag, sharing a backup through an expiring link, keeping a second copy in S3, and monitoring the pipeline with Prometheus metrics and the web console.

---

## Prerequisites

- A running CallFS 2.0.0 server (see the [setup guide](01-linux-file-server-setup.md) or [Installation](../docs_markdown/01-installation.md))
- An access key for a user in `auth.api_key_users` (or `auth.api_keys`). Keys are at least 16 characters, and a key that contains `your-`, `change-me` or `a-strong-` is rejected at startup.
- `curl` and standard POSIX shell utilities on the machines you are backing up
- Optional: `jq` and `sha256sum` for retention and verification

All examples use `https://files.example.com:8443` as the CallFS URL. Replace it and `your-api-key` with your own values.

---

## Writing the Backup Script

The core of any automated backup workflow is a script that collects data and pushes it to the **file server**. CallFS accepts uploads as plain HTTP requests with an `application/octet-stream` body, so you can pipe data straight from any command with no temporary files.

Use `PUT` for backups. A `PUT` creates the file when the path is new (`201 Created`) and replaces it when it exists (`200 OK`), so a script that runs twice for the same date does not fail. A `POST` to an existing path returns `409` (`file already exists, use PUT to update`). Missing parent folders such as `backups/` are created for you.

```bash
#!/bin/bash
set -euo pipefail

CALLFS_URL="https://files.example.com:8443"
API_KEY="your-api-key"
DATE=$(date +%Y-%m-%d)

# Backup a database dump
pg_dump mydb | curl -sf -X PUT \
  -K <(printf 'header = "Authorization: Bearer %s"\n' "$API_KEY") \
  -H "Content-Type: application/octet-stream" \
  --data-binary @- \
  "$CALLFS_URL/v1/files/backups/db-$DATE.sql"

# Backup a directory (tar it first)
tar czf - /etc/nginx/ | curl -sf -X PUT \
  -K <(printf 'header = "Authorization: Bearer %s"\n' "$API_KEY") \
  -H "Content-Type: application/octet-stream" \
  --data-binary @- \
  "$CALLFS_URL/v1/files/backups/nginx-$DATE.tar.gz"
```

Save this to `/opt/scripts/backup.sh` and make it executable:

```bash
chmod 750 /opt/scripts/backup.sh
```

### How the script works

- `set -euo pipefail` makes the script stop if any command fails or any variable is unset, so you never get a silent partial backup.
- `pg_dump mydb` writes a SQL dump to stdout. The pipe sends that stream straight into `curl --data-binary @-`. The dump is never written to disk on the source machine.
- `tar czf -` streams a compressed archive of `/etc/nginx/` the same way.
- The `-K <(printf ...)` trick passes the key to `curl` through a file descriptor, so it does not show up in `ps` output.
- `curl -f` returns a non-zero exit code on HTTP errors (4xx, 5xx), so a failed upload fails the script.
- CallFS answers `201` or `200` only after the file is stored durably. This holds in both performance modes. `performance.mode: strict` additionally reads every saved file back and checks it before answering.

### Logging success and failure

```bash
upload() {
  local dest_path="$1"
  shift

  if ! "$@" | curl -sf -X PUT \
    -K <(printf 'header = "Authorization: Bearer %s"\n' "$API_KEY") \
    -H "Content-Type: application/octet-stream" \
    --data-binary @- \
    "$CALLFS_URL/v1/files/$dest_path"; then
    echo "[$(date -u +%FT%TZ)] ERROR: backup failed for $dest_path" >&2
    return 1
  fi

  echo "[$(date -u +%FT%TZ)] OK: $dest_path"
}

upload "backups/db-$DATE.sql" pg_dump mydb
upload "backups/nginx-$DATE.tar.gz" tar czf - /etc/nginx/
```

If the server is full, the upload returns `507` (`insufficient storage space`). CallFS checks free disk space on local storage before it accepts a file.

---

## Scheduling with Cron

Once the script works manually, schedule it. Open the system crontab:

```bash
sudo crontab -e
```

Run the **backup script** daily at 02:00:

```
0 2 * * * /opt/scripts/backup.sh >> /var/log/callfs-backup.log 2>&1
```

This appends stdout and stderr to a log file, so you have a record of every run.

### Verifying the cron environment

Cron runs jobs with a minimal environment. Keep secrets in a file that only root can read and source it at the top of each script:

```bash
# At the top of /opt/scripts/backup.sh
source /etc/callfs-backup.env
```

```bash
sudo install -m 600 /dev/null /etc/callfs-backup.env
sudo nano /etc/callfs-backup.env
```

Contents:

```bash
CALLFS_URL="https://files.example.com:8443"
API_KEY="your-api-key"
PGPASSWORD="your-postgres-password"
```

---

## Listing Backups

List everything under `backups/` with the directory API and `recursive=true`:

```bash
curl -s \
  -H "Authorization: Bearer $API_KEY" \
  "https://files.example.com:8443/v1/directories/backups/?recursive=true"
```

```json
{
  "path": "/backups",
  "type": "directory",
  "recursive": true,
  "max_depth": 100,
  "count": 2,
  "items": [
    {
      "name": "db-2026-04-05.sql",
      "path": "/backups/db-2026-04-05.sql",
      "type": "file",
      "size": 52428800,
      "mode": "0644",
      "owner": "alice",
      "mtime": "2026-04-05T02:03:11Z"
    }
  ]
}
```

Useful query parameters:

| Parameter | Effect |
|-----------|--------|
| `recursive=true` | Include subfolders. `max_depth` limits the depth (default 100, maximum 1000). |
| `limit=N` | Switch to paged output (1 to 1000 per page). The answer then has `items`, `count`, `total`, `next_cursor` and `partial`. Pass `next_cursor` as `cursor` for the next page. |
| `sort=name\|size\|mtime\|type`, `order=asc\|desc` | Sort a paged listing. |
| `type=file\|directory`, `q=text`, `include_hidden=false` | Filter a paged listing. |

Without `limit`, a listing returns at most 10000 entries and sets `"truncated": true` when there are more. For a large backup folder, use paging.

You can also browse and download backups in the web console under **Files** (see [Web console](../docs_markdown/08-web-console.md)). The console build is in the Docker image and in source builds; the release binaries contain the API only.

---

## Retention: Deleting Old Backups Automatically

Keeping every backup forever is impractical. This script lists the files in `backups/`, reads each modification time and deletes anything older than 30 days.

```bash
#!/bin/bash
set -euo pipefail

source /etc/callfs-backup.env

RETENTION_DAYS=30
CUTOFF=$(date -u -d "$RETENTION_DAYS days ago" +%s 2>/dev/null \
         || date -u -v-${RETENTION_DAYS}d +%s)   # macOS fallback

echo "[$(date -u +%FT%TZ)] Starting retention cleanup (keep=${RETENTION_DAYS}d)"

curl -sf \
  -H "Authorization: Bearer $API_KEY" \
  "$CALLFS_URL/v1/directories/backups/?recursive=true" \
| jq -r '.items[] | select(.type == "file") | "\(.mtime) \(.path)"' \
| while IFS=' ' read -r mtime path; do
    file_epoch=$(date -u -d "$mtime" +%s 2>/dev/null \
                 || date -u -j -f "%Y-%m-%dT%H:%M:%SZ" "$mtime" +%s)

    if [ "$file_epoch" -lt "$CUTOFF" ]; then
      echo "Deleting $path (mtime=$mtime)"
      curl -sf -X DELETE \
        -H "Authorization: Bearer $API_KEY" \
        "$CALLFS_URL/v1/files/${path#/}"
    fi
  done

echo "[$(date -u +%FT%TZ)] Retention cleanup complete"
```

Notes:

- `DELETE /v1/files/{path}` removes the file for good and returns `204`. It does not use the trash. To test the script first, add `?dry_run=true` to the delete URL; CallFS then returns a plan and removes nothing.
- If you prefer to keep deleted backups recoverable, send a batch `trash` operation instead. Trashed files stay for `trash.retention` (default `720h`, 30 days) and can be restored from **Files, Trash** in the console:

```bash
curl -sf -X POST "$CALLFS_URL/v1/batch" \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"ops":[{"op":"trash","path":"/backups/db-2026-03-01.sql"}]}'
```

- Run the cleanup after the new backup has uploaded and verified, never before.
- Users can only delete files they have delete permission on. A file created by one key is owned by that key's user.

---

## Verifying Backup Integrity

Every file CallFS stores gets an `ETag` that is the SHA-256 hash of its content, in hex. Compare it with a hash you calculate while you upload and you can verify a backup without downloading it again. Files saved by earlier releases may have an empty tag until an admin runs `POST /v1/admin/retag`.

```bash
#!/bin/bash
set -euo pipefail

source /etc/callfs-backup.env

DATE=$(date +%Y-%m-%d)
BACKUP_PATH="backups/db-$DATE.sql"
HASH_FILE=$(mktemp)

# Upload and hash the same stream
pg_dump mydb \
  | tee >(sha256sum | cut -d' ' -f1 > "$HASH_FILE") \
  | curl -sf -X PUT \
      -K <(printf 'header = "Authorization: Bearer %s"\n' "$API_KEY") \
      -H "Content-Type: application/octet-stream" \
      --data-binary @- \
      "$CALLFS_URL/v1/files/$BACKUP_PATH"

sleep 1
LOCAL_HASH=$(cat "$HASH_FILE")
rm -f "$HASH_FILE"

# Ask CallFS for the stored tag and size
REMOTE_HASH=$(curl -sfI \
  -K <(printf 'header = "Authorization: Bearer %s"\n' "$API_KEY") \
  "$CALLFS_URL/v1/files/$BACKUP_PATH" \
  | awk -F': ' 'tolower($1)=="etag" {gsub(/[\r"]/,"",$2); print $2}')

if [ "$LOCAL_HASH" = "$REMOTE_HASH" ]; then
  echo "[$(date -u +%FT%TZ)] Verification PASSED: $BACKUP_PATH ($REMOTE_HASH)"
else
  echo "[$(date -u +%FT%TZ)] Verification FAILED: local=$LOCAL_HASH remote=$REMOTE_HASH" >&2
  exit 1
fi
```

A `HEAD` request returns the file's details in headers: `ETag`, `X-CallFS-Type`, `X-CallFS-Size`, `X-CallFS-Mode`, `X-CallFS-Owner`, `X-CallFS-MTime` and, in a cluster, `X-CallFS-Instance-ID`.

```bash
curl -sI -H "Authorization: Bearer $API_KEY" \
  "$CALLFS_URL/v1/files/backups/db-2026-04-05.sql"
```

To restore, download the file with `GET` and, for a large file, use a `Range` header to fetch only part of it:

```bash
curl -sf -H "Authorization: Bearer $API_KEY" \
  "$CALLFS_URL/v1/files/backups/db-2026-04-05.sql" | psql mydb
```

---

## Sharing a Backup with a Single-Use Link

Sometimes you need to hand a backup to a colleague or an auditor without giving them an API key. CallFS creates single-use, time-limited download links.

```bash
curl -s -X POST \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"path": "/backups/db-2026-04-11.sql", "expiry_seconds": 3600}' \
  "https://files.example.com:8443/v1/links/generate"
```

CallFS answers `201`:

```json
{
  "id": 42,
  "url": "https://files.example.com:8443/download/TOKEN",
  "token": "TOKEN",
  "expires": "2026-04-11T15:30:00Z"
}
```

Send the `url` to the recipient. They open it in a browser, see a confirmation page and press **Download file**. With curl, the download is a `POST`:

```bash
curl -X POST -OJ "https://files.example.com:8443/download/TOKEN"
```

The link works once. A second attempt returns `410`. `expiry_seconds` must be between 1 and 86400 (24 hours), and a link is for a file, not a folder. You can see and revoke your links in the console under **Sharing** or with `GET /v1/links` and `DELETE /v1/links/{id}`. The full walkthrough is in [Expiring download links](08-expiring-download-links.md).

---

## Keeping a Second Copy in S3

A **file server** storing backups only on local disk has an obvious weakness: a disk failure takes the backups with it. CallFS can write a second copy of each new file to another backend. Keep local disk as the main storage and make S3 the backup copy:

```yaml
backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs"
  s3_bucket_name: "my-callfs-backups"
  s3_region: "us-east-1"
  s3_access_key: "your-s3-access-key"
  s3_secret_key: "your-s3-secret-key"
  s3_server_side_encryption: "AES256"
  s3_acl: "private"

ha:
  replication_enabled: true
  replica_backend: "s3"
  require_replica_success: true
```

The S3 settings live under `backend:`. The `ha:` block only says whether to copy, where to, and what to do when the copy fails. A file that already lives on the replica backend is not copied again.

| `require_replica_success` | Behavior |
|---------------------------|----------|
| `true` (required) | The copy is written while the file is uploaded. If it fails, the upload fails, so your script's `curl -f` reports it. Uploads fail while S3 is down. |
| `false` (best effort, the default) | The upload is confirmed as soon as the local copy is stored. The S3 copy is made in the background. A failure is listed but the upload still succeeds. |

For a custom S3-compatible endpoint (such as MinIO), add `s3_endpoint: "https://s3.internal.example.com"` and set `s3_server_side_encryption: ""`. A plain `http://` endpoint also needs `s3_allow_insecure: true`.

Restart CallFS after any config change to `backend` or `ha`:

```bash
sudo systemctl restart callfs
```

Two things to remember:

- **Deletes remove the backup copy too.** The retention script above deletes both copies. If you want S3 to keep history, turn on bucket versioning in your S3 service.
- **Failed copies are listed and can be retried.** In the console, **Cluster, Backup copy** shows the mode, the ok, skipped and failed copy counts of the last 24 hours, and a list of failed copies with a **Retry** button. The API equivalents are `GET /v1/admin/replication` and `POST /v1/admin/replication/retry` (send `{"paths": ["/backups/db-2026-04-05.sql"]}` for chosen files, or nothing to retry every failed copy). The health finding `backup.failing` warns about recent failures. In a cluster, background copies are retried automatically with growing delays (`nats.streams.tasks_max_deliver`, `tasks_backoff`) before they show up as failed. Admins also see a backup status column on the Files page, so you can tell which files already have a copy.

Also back up CallFS itself: the file index (for example `metadata_store.sqlite_path`, or `nats.jetstream.store_dir` in a cluster) and your config file. See [Storage](../docs_markdown/05-storage.md#back-up).

---

## Monitoring Backups

Knowing that cron is scheduled is not the same as knowing your backups run. CallFS exposes Prometheus metrics on a separate port (default `:9090`, set with `metrics.listen_addr`) with no sign-in, and on the main port at `/metrics` with an API key.

```bash
curl -s http://files.example.com:9090/metrics | grep callfs_file_operations_total
```

```
# HELP callfs_file_operations_total Total number of file operations
# TYPE callfs_file_operations_total counter
callfs_file_operations_total{backend_type="localfs",operation="create"} 42
callfs_file_operations_total{backend_type="localfs",operation="read"} 138
callfs_file_operations_total{backend_type="localfs",operation="update"} 6
```

Because the backup script uses `PUT`, a new file counts as `create` and a re-run that replaces a file counts as `update`. Watch both.

### Prometheus alerting rules

Fire if no backup upload is seen in 26 hours (a daily cron plus two hours of grace), and if backup copies to S3 fail:

```yaml
groups:
  - name: callfs_backup
    rules:
      - alert: BackupMissing
        expr: |
          (sum(increase(callfs_file_operations_total{operation=~"create|update"}[26h])) or vector(0)) == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "No backup uploads detected in the last 26 hours"

      - alert: BackupCopyFailing
        expr: increase(callfs_replication_total{result=~"failed|dropped"}[1h]) > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Copies to the backup storage are failing"
```

`callfs_replication_total` has the result label `succeeded`, `failed`, `dropped` or `skipped`. `dropped` means a copy could not be queued and the new file was left uncopied. `skipped` means the file was deleted or changed before its copy ran, so there was nothing to copy.

To count only uploads to the backup folder, use `callfs_http_requests_total{method="PUT",path="/v1/files/*"}`. The `path` label is the route pattern, not the real file path, so it cannot tell folders apart. For per-folder checks, use the verification script's exit code and your cron mail.

`/health/ready` also reports a `replication` component that turns to `warn` when backup copies failed in the last hour. See [Monitoring](../docs_markdown/06-monitoring-metrics.md) and the [Prometheus and Grafana guide](10-callfs-prometheus-grafana-monitoring.md).

---

## Putting It All Together

A production-grade automated backup setup on CallFS looks like this:

1. **Backup script** at `/opt/scripts/backup.sh` pipes database dumps and directory archives into CallFS with `PUT`.
2. **Cron job** at `0 2 * * *` runs it nightly and logs output.
3. **Verification** compares the SHA-256 `ETag` with the hash calculated during the upload.
4. **Retention script** at `/opt/scripts/cleanup.sh` lists `backups/`, finds files older than 30 days by `mtime` and deletes them (or moves them to the trash).
5. **Backup copy** in `config.yaml` mirrors every upload to S3 (`ha.replication_enabled`, `ha.replica_backend: s3`).
6. **Monitoring** uses `callfs_file_operations_total` and `callfs_replication_total`, plus the **Cluster, Backup copy** page in the console.

The entire workflow needs no agents on the machines being backed up, no proprietary clients, and no changes when you add new servers: just a URL, an access key, and a shell script.
