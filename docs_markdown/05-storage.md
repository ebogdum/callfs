# Storage

This page covers where CallFS keeps your files and how it keeps them safe: local disk, S3, write safety, performance mode, erasure coding, backup copies, trash, resumable uploads, the file index (metadata store) and backups.

Settings are listed with their YAML key. Every key also works as an environment variable: prefix `CALLFS_`, and use a double underscore for each nesting level (`backend.localfs_root_path` becomes `CALLFS_BACKEND__LOCALFS_ROOT_PATH`). See [Configuration](02-configuration.md). All settings on this page need a restart unless the table says "no".

## How storage fits together

CallFS keeps two kinds of data:

- File content. It lives on a storage backend: local disk, an S3 bucket, or split across servers by erasure coding.
- The file index. It lists every file and folder with owner, size, mode, time stamps and the backend that holds the content. It lives in the metadata store (SQLite, Postgres, Redis or the built-in NATS store).

You can switch on local disk and S3 at the same time. Each file is stored on exactly one of them (plus an optional backup copy and optional erasure coding). New files go to the default backend.

## Local disk

Local disk is the default backend. It is on whenever `backend.localfs_root_path` is set.

```yaml
backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs"
```

| Key | Type | Default | Allowed | What it does |
|---|---|---|---|---|
| `backend.localfs_root_path` | path | `/var/lib/callfs` | any folder | Folder where files are kept. CallFS creates it if it is missing. Changing it does not move existing files. An empty value turns local disk off. |
| `backend.localfs_sync` | string | `full` | `full`, `barrier`, `none` | How hard the disk is asked to confirm each saved file. |
| `backend.localfs_ack` | string | `disk` | `memory`, `disk` | When a client is told its file is saved. |
| `backend.localfs_true_overwrite` | bool | `false` | `true`, `false` | `true` makes every overwrite a true overwrite. See [Overwrites](#overwrites). |
| `backend.localfs_tmp_save_max_size` | size | not set | 1 byte or more | Largest file that uses the temp save. Not set means no limit. |
| `backend.flush_workers` | int | `0` | 0 to 1024 | How many files are flushed to disk at the same time. `0` means one per CPU. |
| `backend.flush_max_pending_bytes` | int (bytes) | `268435456` (256 MiB) | 0 or more | Writers slow down when this much saved data is not on disk yet. `0` uses the default. |
| `backend.flush_max_delay` | duration | `0` | 0 or more, such as `5ms` | How long a flush waits to group more files into one disk flush. `0` flushes at once. |

### File and folder modes

Every file and folder has a mode such as `0644`. New files get `0644` and new folders get `0777`. A resumable upload can set its own `mode` (default `0644`). Change a mode in the console (octal such as `0640`, or symbolic such as `u=rw,go=r`) or through the API.

Modes decide what users other than the owner may do. They are CallFS permissions only: app users are not operating system users, and the bits never touch the OS accounts that run CallFS.

| Mode bit for "other" | Effect for a user who does not own the item |
|---|---|
| Read | Read the file, list the folder |
| Write | Write the file, create items in the folder |

Deleting stays with the owner. Owners are not limited by the mode. Admins (`root` and the users in `auth.admin_users`) bypass file and folder permissions (read, write, delete). With the defaults, every user can read and write new folders and read new files.

Run CallFS as a dedicated user and set restrictive permissions on the root folder (for example `750`). For sensitive data, use disk-level encryption such as LUKS.

### Free space checks

Before it accepts a file of known size, CallFS checks that the root folder has that much free space. If not, the upload fails with status 507 and the error code `INSUFFICIENT_STORAGE`. The Health findings page warns when free disk space falls below 10 percent (warning) and 5 percent (critical).

### Write safety (`backend.localfs_sync`)

This sets how hard the disk is asked to make a file permanent.

| Value | Meaning |
|---|---|
| `full` | Every saved file is forced to the physical disk. Survives a power loss. This is the default and the safest choice. |
| `barrier` | Faster on macOS and the same as `full` on other systems. |
| `none` | No forced flush. Fastest. Files saved shortly before a power loss or system crash can be lost. A normal CallFS restart loses nothing. |

### Write confirmation (`backend.localfs_ack`)

This sets the moment CallFS answers "saved" to the client.

| Value | The client is told "saved" when | Effect |
|---|---|---|
| `disk` | The file and its folder entry are on the disk (flushed according to `localfs_sync`). | Slower. Survives a power cut right after the answer. |
| `memory` | The file is written and indexed. The flush to disk follows in the background. | Faster. A power cut right after the answer can lose that file. |

With `localfs_sync: none` there is no flush, so both values behave the same.

### Choose the confirmation per request (`X-CallFS-Write-Ack`)

A client can override `backend.localfs_ack` for one request with the header `X-CallFS-Write-Ack`. Values are `memory` and `disk` (case does not matter). Any other value returns 400 with the text `write ack must be one of: memory, disk`. The header only affects files stored on local disk.

```bash
curl -X PUT "https://files.example.com:8443/v1/files/reports/q1.pdf" \
  -H "Authorization: Bearer your-api-key" \
  -H "X-CallFS-Write-Ack: disk" \
  --data-binary @q1.pdf
```

Use `disk` for files you cannot lose and `memory` for bulk loads that you can repeat.

### Flush tuning

CallFS flushes saved files to disk in the background with a pool of workers. These settings apply when `localfs_sync` is not `none`.

- `flush_workers` sets how many flushes run in parallel. Raise it on fast NVMe disks with many small files. Leave `0` for one worker per CPU.
- `flush_max_delay` lets flushes wait a short time so several files share one flush. Values from 1ms to 10ms can raise throughput on busy servers and add that much to each save when `localfs_ack` is `disk`.
- `flush_max_pending_bytes` caps the data that is saved but not yet on disk. When the cap is reached, new writes wait until flushes catch up. This keeps memory use bounded.

```yaml
backend:
  localfs_sync: "full"
  localfs_ack: "disk"
  flush_workers: 8
  flush_max_pending_bytes: 536870912
  flush_max_delay: "2ms"
```

## Overwrites

When you replace a file on local disk, CallFS uses a temp save by default. It is fast and safe: a confirmed overwrite survives a power cut.

How the temp save works:

1. The new upload is written to a hidden temp file next to the target and flushed.
2. The old file is renamed aside to a hidden name.
3. The temp file is renamed into place and the folder is flushed.
4. The index is committed and the client is told the save succeeded.
5. The old copy is deleted in the background.

If the index commit fails, the old file is put back and the client gets an error. Hidden temp and old names are never listed, searched or counted.

### True overwrite

A true overwrite is the older way: the new copy is written in the staging area, the index is committed, and then the file is moved into place before the reply. It is slower under load. Choose it with:

| Setting | Type | Default | What it does |
|---|---|---|---|
| `backend.localfs_true_overwrite` | bool | `false` | `true`: every overwrite is a true overwrite. Needs a restart. |
| `backend.localfs_tmp_save_max_size` | size | not set | Used only while `localfs_true_overwrite` is `false`. Files up to this size use the temp save; bigger files use a true overwrite. Leave it out (or empty) for no limit. When set it must be at least 1 byte. `0` is rejected at startup with `must be at least 1 byte; leave it out for no size limit`. Needs a restart. |

```yaml
backend:
  localfs_true_overwrite: false
  localfs_tmp_save_max_size: "64MiB"
```

In the console these are on Settings > Storage. The size row sits behind a **Limit file size** switch and is hidden while true overwrite is on. Turning the switch off removes the key from the config file.

### Choose per request (`X-CallFS-True-Overwrite`)

Send `X-CallFS-True-Overwrite: true` or `false` on `PUT` or `POST` to pick the way for that one request. The header wins over both settings. Any other value returns 400 with `X-CallFS-True-Overwrite must be true or false`.

```bash
curl -X PUT -H "Authorization: Bearer your-api-key" -H "X-CallFS-True-Overwrite: true" \
  --data-binary @report.pdf https://files.example.com/v1/files/docs/report.pdf
```

### Strict mode

`performance.mode: strict` always uses a true overwrite.

### After a crash

If the server stops in the middle of an overwrite, it fixes this at the next start. The old copy is restored when the index still points at it, otherwise it is deleted. Leftover temp files are deleted. The metric `callfs_overwrite_restores_total` counts the old copies that were put back.

### Speed

Single node, same machine, SQLite index, 1 KB files, a fresh server per test:

| Test | 1.6.0 | 2.0 temp save | 2.0 true overwrite |
|---|---|---|---|
| Overwrite, 64 clients | 154/s (56 failed) | 778/s (0 failed) | 108/s |
| New files, 64 clients | 159/s | 773/s | |
| One client overwriting one file | 108/s | 65/s | 33/s |

## Performance mode

```yaml
performance:
  mode: "fast"
```

| Value | Default | What it does |
|---|---|---|
| `fast` | yes | Normal operation. |
| `strict` | no | The same durability as `fast` plus extra checks. Slower. |

Changing `performance.mode` needs a restart.

Both modes only confirm a save once the file is durable, and both keep every confirmed save. `strict` adds three checks:

- After each save, CallFS reads the stored file back and compares its size and checksum with what you uploaded. A mismatch fails the save with an error that starts with `stored file does not match the uploaded bytes`.
- It locks every change to the file tree, so two changes at the same time are always handled one after the other.
- Every read waits until it sees the latest state of the whole cluster, so a read after a write on another node always shows that write.

Choose `strict` when correctness matters more than speed, for example for audit data or when you test a new setup. Any other value is rejected at startup with `performance.mode must be one of: fast, strict`.

## S3 and compatible stores

The S3 backend is on whenever `backend.s3_bucket_name` is set. CallFS checks that it can reach the bucket at startup and refuses to start when it cannot.

```yaml
backend:
  default_backend: "s3"
  s3_bucket_name: "your-callfs-bucket"
  s3_region: "us-east-1"
  s3_access_key: "your-s3-access-key"
  s3_secret_key: "your-s3-secret-key"
  s3_server_side_encryption: "AES256"
  s3_acl: "private"
```

| Key | Type | Default | Allowed | What it does |
|---|---|---|---|---|
| `backend.s3_bucket_name` | string | empty | bucket name | Bucket that stores files. Empty turns S3 off. |
| `backend.s3_region` | string | `us-east-1` | region name | Region of the bucket. |
| `backend.s3_access_key` | secret | empty | text | Access key ID. |
| `backend.s3_secret_key` | secret | empty | text | Secret access key. |
| `backend.s3_endpoint` | URL | empty | `https://...` or `http://...` | Address of an S3-compatible service that is not Amazon. Empty uses Amazon S3. |
| `backend.s3_allow_insecure` | bool | `false` | `true`, `false` | Lets `s3_endpoint` start with `http://`. Without it, a plain HTTP endpoint is refused at startup. |
| `backend.s3_server_side_encryption` | string | `AES256` | `AES256`, `aws:kms`, empty | How the bucket encrypts stored files. Empty turns it off. |
| `backend.s3_kms_key_id` | string | empty | KMS key ID or ARN | Key used when encryption is `aws:kms`. |
| `backend.s3_acl` | string | `private` | any S3 canned ACL, such as `private` or `public-read`; empty sends no ACL | Access that new files get in the bucket. |

### MinIO and other S3-compatible stores

Set `s3_endpoint` to the service address. When an endpoint is set, CallFS uses path-style addresses (`https://host/bucket/key`), which MinIO, Ceph and most other stores expect.

```yaml
backend:
  default_backend: "s3"
  s3_endpoint: "https://minio.example.com:9000"
  s3_bucket_name: "callfs"
  s3_region: "us-east-1"
  s3_access_key: "your-s3-access-key"
  s3_secret_key: "your-s3-secret-key"
  s3_server_side_encryption: ""
```

- MinIO servers without encryption set up need `s3_server_side_encryption: ""`.
- For a local test server on plain HTTP, add `s3_allow_insecure: true`. CallFS logs a warning because file data and keys then travel unencrypted. Do not use this over a network you do not control.
- Never set `s3_acl: public-read` unless you want anyone on the internet to read new files.

### S3 security

- Create a dedicated user for CallFS that can use only this bucket: `s3:GetObject`, `s3:PutObject`, `s3:DeleteObject` and `s3:ListBucket`.
- Keep server-side encryption on. Use `aws:kms` with `s3_kms_key_id` for keys you control.
- Use a bucket policy that denies non-HTTPS access.

## Choose where files go

### Default backend

`backend.default_backend` (`localfs` or `s3`, default `localfs`) is where new files are written. Existing files stay where they are. Any other value is rejected at startup.

```yaml
backend:
  default_backend: "s3"
  localfs_root_path: "/var/lib/callfs"
  s3_bucket_name: "your-callfs-bucket"
```

With this setup new files go to S3. Files already on local disk are still served from there.

Every file shows its backend in listings, in stat results and in the console (the Storage column). The `GET /v1/capabilities` endpoint lists the backends that are on (`storages`) and the default (`default_storage`).

### A different backend for one file

- Resumable upload: set `"storage": "localfs"` or `"storage": "s3"` when you create the upload (see [Resumable uploads](#resumable-uploads)). Any other value returns 400 with `storage must be localfs or s3`.
- Move an existing file: send `{"destination": "...", "backend": "s3"}` in a move request (see [Move files between backends](#move-files-between-backends)).
- Erasure coding: see the next section.

In a cluster each file also belongs to one server. A request that reaches another server is passed on to the owning server without any change for the client. See [Clustering](07-clustering.md).

## Erasure coding

Erasure coding (also called splitting) cuts a file into pieces (shards) and adds spare pieces. The file stays readable when some pieces are lost. You choose how many pieces hold data and how many are spares. With 4 data pieces and 2 spare pieces, a file survives the loss of any 2 pieces and uses 1.5 times its size on disk (6 divided by 4).

### Settings

```yaml
erasure:
  enabled: true
  data_shards: 4
  parity_shards: 2
  shard_backend: "localfs"
  min_file_size: 104857600
  shard_path: ".callfs/shards"
```

| Key | Type | Default | Allowed | What it does |
|---|---|---|---|---|
| `erasure.enabled` | bool | `false` | `true`, `false` | Turns erasure coding on. Files saved before stay as they are. |
| `erasure.data_shards` | int | `4` | 2 to 32 | Pieces that hold the file data. Changing it affects new files only. |
| `erasure.parity_shards` | int | `2` | 1 to 32 | Spare pieces. A file survives the loss of this many pieces. |
| `erasure.shard_backend` | string | `localfs` | `localfs`, `s3` | Where pieces are stored. `localfs` uses the local disk folder of each server. `s3` uses the bucket. |
| `erasure.min_file_size` | bytes | `0` | 0 or more | New files at or above this size are split automatically, without being asked. `0` means only uploads that ask are split. |
| `erasure.shard_path` | relative path | `.callfs/shards` | clean relative folder inside the storage, no `..` | Folder inside each storage (local root or bucket) where pieces are kept. An empty value uses the default. |

Data plus spare pieces must not exceed 256. Startup fails with `erasure.data_shards must be >= 2`, `erasure.parity_shards must be >= 1`, `erasure.data_shards + erasure.parity_shards must not exceed 256`, `erasure.min_file_size must be >= 0 (0 means only when an upload asks)` or `erasure.shard_path must be a clean relative path inside the storage` otherwise. Changing `min_file_size` or `shard_path` needs a restart. Pieces written under an older `.erasure` folder stay readable. All four metadata store types support erasure coding.

Pieces are kept in the `shard_path` folder (default `.callfs/shards`) under the local root folder, or under the same prefix in the S3 bucket. Do not edit or move this folder.

### Automatic splitting by size

When `erasure.min_file_size` is above 0, every new file whose size is known and is at or above that value is split without any header. This covers `POST` and `PUT` uploads (a `PUT` with `If-Match` is not split) and resumable uploads that state their `size`. Files above 1 GiB are never split automatically and are stored whole. Existing files are not changed.

A client opts out for one upload with `X-CallFS-Erasure: false` (or `?erasure=false`), and a resumable upload accepts the same header on `POST /v1/uploads`. `GET /v1/capabilities` shows the threshold as `min_file_size` under `erasure_defaults`.

### Store a file erasure-coded on request

Below the threshold, or when `min_file_size` is 0, files are split only when you ask. Send `POST` to the file path with `X-CallFS-Erasure: true` (or the query `?erasure=true`).

```bash
curl -X POST "https://files.example.com:8443/v1/files/archive/video.mp4" \
  -H "Authorization: Bearer your-api-key" \
  -H "X-CallFS-Erasure: true" \
  --data-binary @video.mp4
```

Optional per-file overrides (header or query):

| Header | Query | What it does |
|---|---|---|
| `X-CallFS-Erasure-Data-Shards` | `data_shards` | Data pieces for this file. Whole number, 1 or more. Default: `erasure.data_shards`. |
| `X-CallFS-Erasure-Parity-Shards` | `parity_shards` | Spare pieces for this file. Whole number, 1 or more. Default: `erasure.parity_shards`. |
| `X-CallFS-Erasure-Instances` | `instances` | Comma-separated list of server IDs that should hold the pieces. Default: all servers. |

```bash
curl -X POST "https://files.example.com:8443/v1/files/archive/video.mp4?erasure=true&data_shards=6&parity_shards=3&instances=node-1,node-2,node-3" \
  -H "Authorization: Bearer your-api-key" \
  --data-binary @video.mp4
```

Rules and answers:

- The maximum size of an erasure-coded file is 1 GiB.
- Data pieces, spare pieces and their sum must each be at most 255, and the instance list at most 64 entries. Otherwise the answer is 400.
- A bad number or an empty entry in the list returns 400.
- If the path already exists, the answer is 409. Delete the file first or choose another path.
- An empty upload returns 400.
- If erasure coding is off on the server, `X-CallFS-Erasure: true` on `POST` or `PUT` gets a 501 with `erasure coding is not enabled on this server`.
- A piece that is sent to another server must be at most 256 MiB. Pick enough data pieces so that the file size divided by the data pieces stays under that size.
- A resumable upload can be erasure-coded too, with an `erasure` object in the upload request (up to 1 GiB). See [Resumable uploads](#resumable-uploads).

You read an erasure-coded file like any other file, including range requests. The result is the same bytes. Listings and stat results show the storage as `erasure`.

For parallel downloads, add `?manifest=true` to the file `GET`. The answer is a JSON list of pieces with their sizes and checksums. Only admins see the direct server address of each piece. Each piece can be downloaded with `GET /v1/shards/{path}/{index}` (needs read permission on the file).

### Where the pieces go

Pieces are spread round-robin over the servers. The server that receives the upload gets piece 0, then the pieces rotate through all servers in `instance_discovery` (or the list you give in `X-CallFS-Erasure-Instances`). On a single server, every piece sits on that server and a lost disk loses the file.

A file survives the loss of a whole server only if that server holds no more pieces than the number of spare pieces. With 6 pieces (4 + 2) on 3 servers, each server holds 2 pieces and one server can fail. With 2 servers, 3 pieces each, one failure breaks the file. Plan the layout before you store files:

```bash
curl -X POST "https://files.example.com:8443/v1/admin/erasure/plan" \
  -H "Authorization: Bearer your-admin-api-key" \
  -H "Content-Type: application/json" \
  -d '{"data_shards": 4, "parity_shards": 2, "instances": ["node-1", "node-2", "node-3"]}'
```

The answer lists each piece and its server, the pieces per server, how many whole-server failures it survives (`tolerated_instance_failures`), how many pieces can be lost (`tolerated_piece_losses`), the storage overhead (`overhead_ratio`) and warnings such as `One server failure can break split files` and `All pieces would be on one server`.

Pieces on other servers travel over HTTPS. A plain `http://` peer address is accepted only for loopback addresses such as `localhost`.

### Verify and repair

Each piece has a checksum. A check reads every piece and gives it one of these states:

| State | Meaning |
|---|---|
| `ok` | The piece is there and its checksum matches. |
| `missing` | The piece does not exist. |
| `corrupt` | The piece exists but its content is wrong. |
| `unreachable` | The server or bucket that holds the piece did not answer. |

A file has one of these health levels:

| Health | Meaning |
|---|---|
| `ok` | All pieces are healthy. |
| `reduced` | Some pieces are lost, but more than the minimum are left. |
| `critical` | Exactly the minimum number of pieces is left. One more loss makes the file unreadable. |
| `unreadable` | Fewer than the minimum number of pieces are left. The file cannot be read. |
| `unknown` | The file was not checked since the server started. |

Endpoints (all under `/v1`, API key or console session; when erasure coding is off they answer 404 with the code `FEATURE_DISABLED`):

| Method and path | Needs | What it does |
|---|---|---|
| `GET /v1/erasure/files?prefix=&health=&limit=&cursor=` | read on each file | Lists erasure-coded files with sizes, piece counts, `healthy_shards`, `health` and `last_checked_at`. `health` filters by `ok`, `reduced`, `critical` or `unreadable`. `limit` is 1 to 1000 (default 200). Use `next_cursor` for the next page. |
| `GET /v1/erasure/files/{path}` | read | Shows every piece: index, `kind` (`data` or `parity`), server, storage, size, checksum and current state. Runs a check first. |
| `GET /v1/erasure/manifest/{path}` | read | The same download manifest as `?manifest=true`. |
| `POST /v1/erasure/verify/{path}` | read | Checks every piece now. Answers `readable`, `healthy`, `required` and the state of each piece. |
| `POST /v1/erasure/repair/{path}` | write | Starts a repair job and answers 202 with a `job_id`. The job rebuilds damaged pieces from the healthy ones and answers `repaired` and `failed` lists. Follow it under Activity, Jobs. |
| `GET /v1/admin/erasure` | admin | Summary: settings, number of files, bytes stored, raw bytes used, files per health level, pieces per server. |
| `POST /v1/admin/erasure/plan` | admin | Plans piece placement (see above). |
| `POST /v1/admin/erasure/verify-all` | admin | Starts a job that checks every file under `prefix` (default `/`). Answers 202 with a `job_id`. |

```bash
curl -X POST "https://files.example.com:8443/v1/erasure/verify/archive/video.mp4" \
  -H "Authorization: Bearer your-api-key"

curl -X POST "https://files.example.com:8443/v1/erasure/repair/archive/video.mp4" \
  -H "Authorization: Bearer your-api-key"
```

CallFS also heals files on its own:

- When a download finds a damaged piece, it rebuilds the file from the healthy pieces and repairs the damaged ones in the background.
- After a full-file download it checks the spare pieces in the background and repairs them if needed.

The Health findings raise a warning for files at `reduced` or `critical` health (`erasure.damaged`) and when one server holds more pieces than there are spare pieces (`erasure.tolerance`). The console shows the same on Cluster, then Durability. See [Monitoring](06-monitoring-metrics.md) and [Web console](08-web-console.md).

### What happens when a node is lost

- Files with enough healthy pieces stay readable. The loss shows as `reduced` or `critical` health.
- Reads of those files still work, and CallFS repairs pieces that are reachable but damaged.
- When the node comes back, its pieces are `ok` again after the next check.
- Repair rewrites a damaged piece on the server that holds it. It cannot move pieces to other servers, so a node that is gone for good keeps its files at `reduced` or `critical` health. Bring the node back, or restore its disk from backup, and then run a repair.
- If more pieces than the spare count are lost, the file is `unreadable` and reads fail with an error that mentions code 3050 (insufficient shards).
- Files that are not erasure-coded and live on the lost node are not covered by erasure coding. Use a backup copy or a backup for them.

Run a check after any outage (`POST /v1/admin/erasure/verify-all` or Cluster, Durability in the console) and repair every file that is not `ok`.

## Backup copy (replication)

A backup copy writes a second copy of each new file to another backend. In a cluster, copy work is shared by all servers. The copy goes to the other backend only: a file on local disk is copied to the replica backend, and a file that is already on the replica backend is not copied again.

```yaml
ha:
  replication_enabled: true
  replica_backend: "s3"
  require_replica_success: false
```

| Key | Type | Default | Allowed | What it does |
|---|---|---|---|---|
| `ha.replication_enabled` | bool | `false` | `true`, `false` | Turns the backup copy on. |
| `ha.replica_backend` | string | empty | `localfs`, `s3` | Where the backup copy goes. Required when replication is on; startup fails with `ha.replica_backend must be one of: localfs, s3 when ha.replication_enabled=true` otherwise. |
| `ha.require_replica_success` | bool | `false` | `true`, `false` | What happens when the backup copy fails. |

Both backends must be set up: for `replica_backend: s3`, set `backend.s3_bucket_name` and the S3 keys.

| `require_replica_success` | Behavior |
|---|---|
| `false` (best effort) | The upload is confirmed as soon as the main copy is stored. The copy is queued as a durable background task that survives restarts. If a copy fails, it is retried automatically and the upload still succeeds. A copy that cannot be queued is listed as failed and counted as `dropped`. |
| `true` (required) | The backup copy is written while the file is uploaded. If the copy fails, the upload fails, and a delete also fails when the backup copy cannot be deleted. Uploads fail while the backup storage is down. |

Deletes remove the backup copy too.

### Retries and dead letters

With NATS on, each background copy is a durable task. A failed task is tried again after a wait, up to a fixed number of attempts, then kept as a failed background task so it is not lost. Admins list failed tasks, retry them (the task goes back to the same server queue it came from) or drop them in the console (Activity > Jobs > Background) or with `GET /v1/admin/tasks/dead`, `POST /v1/admin/tasks/dead/{id}/retry` and `DELETE /v1/admin/tasks/dead/{id}` (see the admin API). With NATS off the list is empty. A task whose file was deleted or changed since it was queued is skipped and counted as `skipped`.

| Key | Type | Default | Allowed | What it does |
|---|---|---|---|---|
| `nats.streams.tasks_max_deliver` | int | `8` | 1 or more | How many times a task is tried before it moves to failed tasks. Must be greater than the number of wait steps. |
| `nats.streams.tasks_backoff` | list of durations | `1s, 5s, 30s, 2m, 10m` | each above zero | Wait before each retry. |

Both apply without a restart. The failed copies list and `POST /v1/admin/replication/retry` start a copy again by hand.

Check and fix backup copies:

- Console: Cluster, then Backup copy, shows the mode, where copies go, the copies made, skipped and failed in the last 24 hours, and a list of failed copies with a Retry button. Admins also see a Backup column in the file list.
- `GET /v1/admin/replication` returns the settings, `counts_24h` (`ok`, `skipped`, `failed`) and the recent failures.
- `POST /v1/admin/replication/retry` starts a retry job (202 with `job_id`). Send `{"paths": ["/reports/q1.pdf"]}` for chosen files (up to 1000), or no paths to retry every failed copy.
- Prometheus metric: `callfs_replication_total` with the result label `succeeded`, `failed`, `dropped` or `skipped`. See [Monitoring](06-monitoring-metrics.md).
- The finding `backup.failing` warns about recent failures.

## Trash

With trash on, files deleted from the console or through a batch `trash` operation move to a hidden trash folder. You can restore them until they expire.

```yaml
trash:
  enabled: true
  retention: "720h"
```

| Key | Type | Default | Allowed | Restart needed | What it does |
|---|---|---|---|---|---|
| `trash.enabled` | bool | `true` | `true`, `false` | no | Keeps deleted files in trash. When off, the trash endpoints are not available. |
| `trash.retention` | duration | `720h` (30 days) | `1h` to `8760h` (1 year) | no | How long files stay in trash before they are deleted for good. Outside the range, startup fails with `trash.retention must be between 1h and 8760h`. |

A plain `DELETE /v1/files/{path}` deletes for good and does not use trash. To move a file to trash through the API, use a batch operation:

```bash
curl -X POST "https://files.example.com:8443/v1/batch" \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"ops": [{"op": "trash", "path": "/reports/old.pdf"}]}'
```

Trash endpoints:

| Method and path | What it does |
|---|---|
| `GET /v1/trash?limit=&cursor=&deleted_by=` | Lists trash items: `id`, `original_path`, `name`, `type`, `size`, `deleted_by`, `deleted_at`, `expires_at`. Also gives `total_bytes` and `next_cursor`. `limit` is 1 to 1000 (default 200). |
| `POST /v1/trash/{id}/restore` | Restores an item. Optional body: `{"to": "/new/path", "on_conflict": "fail"}`. Answers `{"path": "..."}`. |
| `DELETE /v1/trash/{id}` | Deletes one item for good. Answers 204. |
| `DELETE /v1/trash?deleted_by=` | Empties the trash. Answers 202 with a `job_id`. |
| Batch operation `restore` | Restores items in a batch (`path` is the trash id, `destination` the target). |

`on_conflict` says what to do when the restore path already exists:

| Value | Behavior |
|---|---|
| `fail` (default) | Answer 409. |
| `keep_both` | Restore under a new name. |
| `replace` | Replace the existing file. |

Other answers: 400 for a bad request (unknown `on_conflict`, restoring to `/` or into the trash folder), 403 for another user's item, 404 for an unknown id.

Who sees what: a user sees and manages only their own trash. An admin sees everyone's trash, can filter with `deleted_by=user-id`, and can pass `deleted_by=*` for all users. A restore needs write permission on the target path.

Expired items are deleted for good by a worker that runs every 10 minutes. Trash counts toward disk usage until then. The console page is Files, then Trash. The wording there says 30 days, which is the default retention.

## Resumable uploads

Resumable uploads let a client send a large file in parts and continue after a broken connection.

| Method and path | What it does |
|---|---|
| `POST /v1/uploads` | Starts an upload. Body (JSON): `path` (required), `size` (bytes), `overwrite` (bool), `if_match` (ETag), `mode` (octal like `0644`, default `0644`), `storage` (`localfs` or `s3`), `erasure` (`{"data_shards", "parity_shards", "instances"}`). Answers 201 with `upload_id`, `offset`, `part_bytes` and `expires_at`. |
| `PUT /v1/uploads/{id}` | Sends one part with `Content-Range: bytes start-end/total`. The part must start at the current offset. |
| `GET /v1/uploads/{id}` | Shows `offset`, `size`, `state` and `expires_at`. Use it to resume. |
| `POST /v1/uploads/{id}/complete` | Stores the file. Answers the final `path`, `etag` and `created`. |
| `DELETE /v1/uploads/{id}` | Cancels the upload and removes its data. |

Limits:

- A file can be up to 10 GiB (`limits.max_upload_bytes` in `GET /v1/capabilities`). With `erasure`, up to 1 GiB. Larger sizes return 413 `PAYLOAD_TOO_LARGE`.
- The suggested part size is 8 MiB (`part_bytes`).
- An upload expires 24 hours after it starts. An expired upload returns 404 with `upload expired, start a new upload`.
- An existing file needs `overwrite: true` or `if_match`, otherwise the answer is 409 `FILE_ALREADY_EXISTS`. A mismatching `if_match` returns 412.
- A part at the wrong offset returns 409 and the correct `offset` in the answer.

Open uploads keep their parts in a hidden `.callfs-uploads` folder inside `backend.localfs_root_path`. This is the folder to leave alone and to size your disk for: free space must hold the uploads in progress and the finished file. A worker removes expired uploads every 10 minutes. Uploads need `backend.localfs_root_path`, even when the final file goes to S3.

Single `POST` and `PUT` uploads write to a hidden `.callfs-staging` folder first and move the file into place when it is complete. Leftovers from broken uploads show under Cluster, then Hygiene, and can be cleaned there or with `DELETE /v1/admin/staging?older_than_s=3600`.

## Metadata store (the file index)

`metadata_store.type` selects where the index lives. The default is `postgres`.

| Type | Use it when | Notes |
|---|---|---|
| `sqlite` | One server, small to medium setups, tests. No extra service. | One file on local disk. Not shared between servers. |
| `postgres` | One or more servers that need a strong, shared database. | Needs a Postgres server. CallFS creates and updates its tables at startup. |
| `redis` | Fast shared index where Redis is already run and managed. | Keep Redis persistence on (AOF) or the index can be lost. |
| `nats` | Clusters. Replicated across the servers of the cluster. No extra database. | Needs `nats.enabled: true`. Also set `dlm.type: nats`. See [Clustering](07-clustering.md). |

Raft cluster mode was removed in 2.0.0; configs that use it are rejected at startup. Use `metadata_store.type: nats` and `dlm.type: nats`.

Switching types does not copy the index. CallFS starts with an empty file list, and the files on the backends are no longer listed.

### Common keys

| Key | Type | Default | What it does |
|---|---|---|---|
| `metadata_store.type` | string | `postgres` | `postgres`, `sqlite`, `redis` or `nats`. |

### SQLite

```yaml
metadata_store:
  type: "sqlite"
  sqlite_path: "/var/lib/callfs/index.sqlite3"
```

| Key | Type | Default | What it does |
|---|---|---|---|
| `metadata_store.sqlite_path` | path | `./callfs.sqlite3` | File that holds the index. Required. A relative path starts in the folder where CallFS runs. CallFS restricts the file permissions to its own user. |

Keep the index file outside `backend.localfs_root_path`.

### Postgres

```yaml
metadata_store:
  type: "postgres"
  dsn: "postgres://callfs:your-db-password@db.example.com:5432/callfs?sslmode=require"
  postgres:
    max_open_conns: 25
    max_idle_conns: 10
    conn_max_lifetime: "30m"
```

| Key | Type | Default | What it does |
|---|---|---|---|
| `metadata_store.dsn` | secret | empty | Connection string. Required for Postgres. Use `sslmode=require` or stronger. |
| `metadata_store.migration_dsn` | secret | empty | Connection string used only to create and update tables, for example a login with more rights. Empty uses `dsn`. |
| `metadata_store.postgres.max_open_conns` | int | `25` | Most connections open at once. Must be above 0. |
| `metadata_store.postgres.max_idle_conns` | int | `10` | Connections kept ready. 0 or more. |
| `metadata_store.postgres.conn_max_lifetime` | duration | `30m` | How long a connection is reused. 0 or more. |

### Redis

```yaml
metadata_store:
  type: "redis"
  redis_addr: "10.0.0.5:6379"
  redis_password: "your-redis-password"
  redis_db: 0
  redis_key_prefix: "callfs:"
  redis_tls: true
```

| Key | Type | Default | What it does |
|---|---|---|---|
| `metadata_store.redis_addr` | host:port | `localhost:6379` | Redis address. Required. |
| `metadata_store.redis_username` | string | empty | Redis user name (Redis 6 ACL). |
| `metadata_store.redis_password` | secret | empty | Redis password. Without one, anyone who reaches Redis can change the index. |
| `metadata_store.redis_db` | int | `0` | Database number, 0 to 15. |
| `metadata_store.redis_key_prefix` | string | `callfs:` | Text put before each key. Changing it hides the current files. |
| `metadata_store.redis_tls` | bool | `false` | Connects over TLS. |
| `metadata_store.redis_tls_ca_file` | path | empty | CA file that signed the Redis certificate. |
| `metadata_store.redis_tls_skip_verify` | bool | `false` | Skips the certificate check. Use for tests only. |

### NATS

```yaml
nats:
  enabled: true
metadata_store:
  type: "nats"
dlm:
  type: "nats"
```

The index is kept by the built-in NATS server on each CallFS node and copied between the nodes. Tuning keys (`nats.metadata.*`) are in [Configuration](02-configuration.md) and [Clustering](07-clustering.md). `nats.metadata.replicas` (1 to 5, default 3) sets how many nodes hold a copy of the index. A single node works the same way with one copy.

## Back up

A full backup needs three things: the file content, the file index and your configuration. Take them close in time. Restore them together.

### What to back up

| Item | Where | Notes |
|---|---|---|
| Local disk files | `backend.localfs_root_path` | Includes the pieces folder (`erasure.shard_path`, default `.callfs/shards`, and an older `.erasure` if present) and `.callfs-uploads`. You can skip `.callfs-staging`. |
| S3 files | The bucket | Turn on bucket versioning or replication in your S3 service. |
| Configuration | The file passed with `--config`, plus the certificate and key files | Contains secrets. Store it encrypted. |
| SQLite index | `metadata_store.sqlite_path` and the `-wal` and `-shm` files next to it | Stop CallFS first, or use the SQLite online backup (`sqlite3 index.sqlite3 ".backup backup.sqlite3"`). |
| Postgres index | The database | Use `pg_dump` or continuous archiving. |
| Redis index | The Redis database | Keep AOF on. Copy the RDB or AOF files, or use your managed service backup. |
| NATS index | `nats.jetstream.store_dir` on each node (default `./data/nats`) | The data is copied between nodes. Back up at least one healthy node. For a consistent copy, stop the node first. |

Each server in a cluster keeps its own local files, so back up the local root folder of every server.

### Restore

1. Stop CallFS.
2. Restore the configuration, the file content and the index.
3. Start CallFS.
4. Run `POST /v1/admin/erasure/verify-all` if you use erasure coding, and check the Health findings.

An index that is older than the file content is safe but may not list the newest files. An index that is newer than the file content lists files whose content is missing.

## Move files between backends

Admins can move files between storages and servers. Send a move request. A move can change the name, the folder, the backend and the server in one step.

```bash
curl -X PATCH "https://files.example.com:8443/v1/files/reports/q1.pdf" \
  -H "Authorization: Bearer your-admin-api-key" \
  -H "Content-Type: application/json" \
  -d '{"destination": "/archive/q1.pdf", "backend": "s3"}'
```

Body fields: `destination`, `backend` (`localfs`, `s3`, `erasure`), `instance` (server ID), `overwrite`, `create_parents`. Use `destination` for a move, or `name` for a rename, not both. The batch operation `move` accepts `storage` and `instance` for the same purpose.

| Case | Result |
|---|---|
| Move or rename on the same backend and server | Allowed. Files and folders. |
| Move one file to another backend (`localfs` to `s3` or back) | Allowed. The content is copied to the new backend and the old copy is removed. |
| Move one file to another server | Allowed. The content is copied to the target server. |
| Move a folder to another backend or server | 400: `moving a directory across backends or instances is not supported; move its files individually or keep it on the same backend/instance` |
| Move a file to or from `erasure` | 400: `moving a file to or from the erasure backend (re-tiering) is not supported in this release` |
| Rename or move of an erasure-coded file on its own backend | Allowed. |
| Unknown `backend` value | 400: `unknown backend "..."` |
| Unknown `instance` | 400: `unknown instance "..."` |
| Move into itself, move of `/`, same source and destination | 400 |
| Destination exists and `overwrite` is not true | 409 |
| Changing the backend or server as a non-admin user | 403: `moving to a different backend or instance requires admin` |
| Other server does not answer | 502 |

To move a folder's files to another backend, move each file. To change an erasure-coded file to a plain file, download it and upload it again.
