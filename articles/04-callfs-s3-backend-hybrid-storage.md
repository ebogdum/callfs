# Using S3 as a Storage Backend in CallFS

CallFS ships with two storage backends out of the box: local filesystem (`localfs`) and S3-compatible object storage (`s3`). Most deployments start with local disk, which is perfectly fine for a single server. As your needs grow (offsite durability, files that every server must reach, cost-effective long-term storage), switching to an S3-compatible backend, or running both at once, is a first-class feature of CallFS.

This guide walks through every S3 setting, covers when to choose S3 over local disk, explains the hybrid storage model and the backup copy feature, and finishes with a quick smoke test to verify your setup. The same settings are available in the web console under Settings, "Storage".

---

## Local Storage vs S3 Storage

Before touching any config, it helps to be deliberate about which backend fits your situation.

**Local filesystem is the right choice when:**

- You need the lowest possible read latency. Serving files off a locally attached NVMe drive will always beat a round-trip to an object store.
- Your workload is write-heavy and latency-sensitive.
- You are running a single-node instance and durability is handled at the infrastructure level (RAID, snapshot-based backups, etc.).
- You want operational simplicity with no external dependencies for file data.

**S3 is the right choice when:**

- You need files to survive a complete server loss without a separate backup pipeline.
- You are storing large amounts of data that would fill local disk.
- Several CallFS servers must read the same files without forwarding requests to the server that holds them. A file on local disk belongs to one server, and other servers fetch it from there; a file in S3 is reachable from every server directly.
- You have compliance requirements that mandate encryption at rest managed by your cloud provider's key infrastructure (SSE-KMS).
- You want to decouple file data from compute, so servers can be replaced or scaled without migrating data.

The two options are not mutually exclusive. You can configure both and choose where each file goes, or keep local disk as the primary storage and have CallFS keep a backup copy in S3.

---

## S3 Backend Configuration

CallFS turns S3 on whenever `backend.s3_bucket_name` is set. Which storage new files go to is a separate choice, `backend.default_backend`.

```yaml
backend:
  default_backend: "s3"
  s3_access_key: "REPLACE_WITH_ACCESS_KEY"
  s3_secret_key: "REPLACE_WITH_SECRET_KEY"
  s3_region: "us-east-1"
  s3_bucket_name: "my-callfs-bucket"
  s3_server_side_encryption: "AES256"
  s3_acl: "private"
```

**Field reference** (all of these need a restart to change):

| Key | Default | Allowed | What it does |
|---|---|---|---|
| `backend.default_backend` | `localfs` | `localfs`, `s3` | Where new files go. Existing files are always read from the storage recorded for them. |
| `backend.s3_bucket_name` | empty | text | The bucket to use. Empty turns S3 off. The bucket must exist: CallFS checks it at startup (15-second limit) and stops with `failed to access S3 bucket` if it cannot reach it. |
| `backend.s3_access_key` | empty | text | Access key ID. |
| `backend.s3_secret_key` | empty | text | Secret that pairs with the access key ID. |
| `backend.s3_region` | `us-east-1` | text | Region of the bucket. Most S3-compatible services ignore it, but keep a value set. |
| `backend.s3_endpoint` | empty | URL | Address of an S3-compatible service. Empty means Amazon S3. |
| `backend.s3_allow_insecure` | `false` | `true`, `false` | Allows a plain `http://` endpoint. Without it, such an endpoint stops CallFS at startup. |
| `backend.s3_server_side_encryption` | `AES256` | `AES256`, `aws:kms`, empty | How the bucket encrypts stored files. |
| `backend.s3_kms_key_id` | empty | text | Key to use with `aws:kms`. |
| `backend.s3_acl` | `private` | an S3 canned ACL, or empty | Access that new objects get. Empty sends no ACL. |

If `default_backend` is `s3` but no bucket is set, uploads fail with `backend not enabled`.

### Environment Variable Overrides

Every configuration field can be set via environment variable, which is preferable for secrets in production. The name is the prefix `CALLFS_`, then the key path in upper case with `__` between levels:

```sh
export CALLFS_BACKEND__S3_ACCESS_KEY="REPLACE_WITH_ACCESS_KEY"
export CALLFS_BACKEND__S3_SECRET_KEY="REPLACE_WITH_SECRET_KEY"
export CALLFS_BACKEND__S3_BUCKET_NAME="my-callfs-bucket"
```

Environment variables take priority over `config.yaml`. In the console, the access key and secret are never displayed after they are saved.

---

## Using a Custom S3 Endpoint

Any service that speaks the S3 API can be used as CallFS's storage backend: MinIO, Ceph, Cloudflare R2, DigitalOcean Spaces and others. When you set `s3_endpoint`, CallFS addresses the bucket in the URL path (`https://endpoint/bucket/key`), which is what self-hosted services expect.

```yaml
backend:
  default_backend: "s3"
  s3_access_key: "REPLACE_WITH_ACCESS_KEY"
  s3_secret_key: "REPLACE_WITH_SECRET_KEY"
  s3_region: "us-east-1"
  s3_bucket_name: "my-callfs-bucket"
  s3_endpoint: "https://s3.example.com:9000"
  s3_server_side_encryption: ""
  s3_acl: "private"
```

Notes for S3-compatible services:

- Set `s3_server_side_encryption` to an empty string for servers that do not support encryption headers, such as a basic MinIO.
- Some services reject any ACL. If uploads fail with an ACL error, set `s3_acl` to an empty string.
- An endpoint that starts with `http://` is refused unless `s3_allow_insecure: true` is set. With it, CallFS logs a warning on every start because file data and credentials then travel unencrypted. Use it only on a private network.
- If your endpoint uses HTTPS with a self-signed certificate, make sure the certificate is trusted by the operating system of the machine running CallFS, or put a proxy with a valid certificate in front of the object store.

---

## Hybrid Storage: Both Backends at Once

Set both `localfs_root_path` and the S3 settings, and CallFS can hold files in both. Each file remembers where it lives:

```yaml
backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs/data"
  s3_bucket_name: "my-callfs-archive"
  s3_region: "us-east-1"
  s3_access_key: "REPLACE_WITH_ACCESS_KEY"
  s3_secret_key: "REPLACE_WITH_SECRET_KEY"
```

- Uploads with `POST` and `PUT` go to `default_backend`.
- Resumable uploads can pick a storage per file: `{"path": "/archive/2025.tar", "size": 1073741824, "storage": "s3"}` on `POST /v1/uploads`.
- Moving an existing file to the other storage (or to another server with `instance`) is an admin operation: `PATCH` with a `backend` or `instance` field moves the file for an admin, and returns `403` for ordinary users. In the console, admins use the move-to-another-server action on the Files page. Folders cannot be moved this way; move their files one by one.
- Downloads read from the storage recorded for the file, whichever it is. In listings and search, the `storage` field (and the `storage=s3` search filter) tells you where each file is. The Files page of the console shows it per item.

The console's Cluster page has a Storage section that lists each storage, marks the default for new files, shows disk use and when the local disk will fill, and has a "Check storage" button. The check writes, reads and deletes a small probe file in each storage and reports the result. The same check is `POST /v1/admin/backends/<name>/probe`.

---

## Backup Copy: Local Primary, S3 Replica

The most resilient configuration keeps local disk as the primary storage and writes a second copy of every file to S3. This is controlled by the `ha` section:

```yaml
backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs/data"
  s3_bucket_name: "callfs-backup"
  s3_region: "us-east-1"
  s3_access_key: "REPLACE_WITH_ACCESS_KEY"
  s3_secret_key: "REPLACE_WITH_SECRET_KEY"

ha:
  replication_enabled: true
  replica_backend: "s3"
  require_replica_success: true
```

| Key | Default | What it does |
|---|---|---|
| `ha.replication_enabled` | `false` | Turns the backup copy on. |
| `ha.replica_backend` | empty | `localfs` or `s3`. Required when replication is on; startup fails with `ha.replica_backend must be one of: localfs, s3` otherwise. A file whose primary storage is already the replica backend is not copied. |
| `ha.require_replica_success` | `false` | Whether a failed backup copy fails the upload. |

Both new files and updates are copied, and deleting a file also removes its copy. The two modes differ in when the copy happens:

- **`require_replica_success: true`**: the file is written to the primary and to the replica at the same time. If the replica write fails, the upload fails, nothing is kept, and the client gets an error. If the last step of making the copy fails after the file was saved, the file stays saved on the primary and the request still returns an error; send the upload again with `PUT`. A delete also fails if the copy cannot be removed. Use this when every accepted file must have a backup. Uploads fail while the backup storage is down.
- **`require_replica_success: false`**: the upload succeeds as soon as the primary write is done, and the copy is made in the background. A failure is logged and counted, and does not affect the client. When clustering is on, each copy is a durable task that is retried several times with growing delays (`nats.streams.tasks_max_deliver` and `tasks_backoff`); tasks that still fail are kept for review. A copy that cannot be queued is counted as dropped, so the file stays without a copy until you retry. A copy for a file that was deleted or changed since it was queued is counted as skipped. Use this when availability matters more than a guaranteed second copy.

Reads always come from the file's primary storage. The copy is for recovery, not a fallback: if you want S3 as the read source, set `default_backend: "s3"` instead.

Watch the copies in Prometheus with `callfs_replication_total{result="succeeded"|"failed"|"dropped"|"skipped"}`. In the console, Cluster, "Backup" shows how many copies were made in the last 24 hours with the counts of ok, skipped and failed, lists each failed copy with its error, and has a "Retry failed copies" button (`GET /v1/admin/replication` and `POST /v1/admin/replication/retry` in the API).

---

## How It Works

**On upload (POST or PUT):**

1. CallFS locks the file path so two writers cannot collide.
2. The file is written to a temporary name on the chosen storage, then moved into place.
3. File metadata (path, size, owner, storage, timestamps, ETag) is saved to the metadata store.
4. If a backup copy is on, it is made as described above.
5. The lock is released and the client receives `201 Created` or `200 OK` with the `ETag`.

Local disk writes are confirmed to the client according to `backend.localfs_ack` (`disk` by default: after the data is on disk). With `performance.mode: strict`, CallFS also reads every saved file back from the storage and checks it before it answers, which adds a read from S3 to each upload. See [Storage](../docs_markdown/05-storage.md).

**On download (GET):**

1. CallFS looks up the file's metadata.
2. The storage recorded in the metadata decides where the bytes come from.
3. The file is streamed to the client. Range requests read only the requested bytes from S3.

**What you will see in the bucket.** The object key is the CallFS path without the leading slash: `/test/hello.txt` is stored as `test/hello.txt`. CallFS also uses a few reserved names in the bucket that you should leave alone: `.callfs-staging/` for uploads in progress, `.callfs/trash/` for items in trash, `.callfs/shards/` (the `erasure.shard_path` setting) for erasure-coded pieces, and `.callfs-probe/` for storage checks. Leftover `.callfs-staging` objects from stopped uploads can be listed and removed in the console under Cluster, "Hygiene".

---

## Server-Side Encryption Options

The `s3_server_side_encryption` field accepts three values.

### AES256 (S3-Managed Keys)

```yaml
s3_server_side_encryption: "AES256"
```

S3 manages the encryption keys and encrypts every object at rest. This is the default and requires nothing else.

### aws:kms (KMS-Managed Keys)

```yaml
s3_server_side_encryption: "aws:kms"
s3_kms_key_id: "arn:aws:kms:us-east-1:123456789012:key/your-key-id"
```

AWS Key Management Service manages the keys, which gives you key rotation control, cross-account policies and an audit trail. If `s3_kms_key_id` is empty, AWS uses the default S3 key for your account.

### Disabled

```yaml
s3_server_side_encryption: ""
```

No encryption header is sent. Use this for S3-compatible services that do not support it, or when encryption is handled at another layer.

---

## Testing the Setup

Once your configuration is in place, check the file first and start the server:

```bash
callfs config validate --config /etc/callfs/config.yaml
callfs server --config /etc/callfs/config.yaml
```

In the log you should see `Initializing S3 backend`. Then use `curl` to verify that uploads reach S3.

**Step 1: Upload a test file**

```sh
curl -X POST \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: text/plain" \
  --data-binary "hello from callfs" \
  https://files.example.com/v1/files/test/hello.txt
```

Create the `test/` folder first with `curl -X POST -H "Authorization: Bearer your-api-key" https://files.example.com/v1/files/test/`. A `201 Created` response means the file was written to the default storage and, with `require_replica_success: true`, also to the replica.

**Step 2: Read it back through CallFS**

```sh
curl -H "Authorization: Bearer your-api-key" \
  https://files.example.com/v1/files/test/hello.txt
```

You should receive the file content. `curl -I` on the same URL shows `X-CallFS-Size` and the `ETag`.

**Step 3: Verify the object in S3**

```sh
aws s3 ls s3://my-callfs-bucket/test/
```

You should see `hello.txt` listed. For a custom endpoint, add `--endpoint-url https://s3.example.com:9000`.

**Step 4: Check the other copy (backup setup only)**

With local disk as the primary and S3 as the replica, the file also exists under the data directory:

```sh
ls -la /var/lib/callfs/data/test/
```

With `require_replica_success: false`, allow a moment for the background copy.

**Step 5: Look at it in the console**

Open `https://files.example.com/ui/`, sign in, and go to Cluster, "Storage": the S3 card shows the bucket, region, endpoint and encryption, and "Check storage" confirms CallFS can write to the bucket. Files shows `test/hello.txt` with its storage.

If you delete the file through CallFS, the primary object and the backup copy are both removed. Removing the file this way is permanent; the console's "Move to trash" keeps it recoverable for the `trash.retention` period (30 days by default).

---

## Summary

CallFS's S3 support works with Amazon S3 and any S3-compatible service. The key decisions are:

- Set `backend.s3_bucket_name` to turn S3 on, and `default_backend: "s3"` for pure S3 storage, or keep `localfs` as the default and choose S3 per file.
- Leave `s3_endpoint` empty for Amazon S3, or point it at your own service; plain `http://` endpoints need `s3_allow_insecure: true`.
- Choose `AES256` for zero-configuration encryption, `aws:kms` when you need key control, or an empty string to disable it.
- Use `ha.replication_enabled` with `replica_backend: "s3"` for a backup copy, and set `require_replica_success: true` when a guaranteed second copy matters more than upload availability.

For most production workloads that need both performance and durability, local primary storage with an S3 backup copy and `require_replica_success: true` is a solid starting point. See [Storage](../docs_markdown/05-storage.md) for the full reference, [Configuration](../docs_markdown/02-configuration.md) for every setting, and [Clustering](../docs_markdown/07-clustering.md) for running several servers on shared S3 storage.
