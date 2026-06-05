# Using S3 as a Storage Backend in CallFS

CallFS ships with two storage backends out of the box: local filesystem (`localfs`) and S3-compatible object storage (`s3`). Most deployments start with local disk, which is perfectly fine for a single server. As your needs grow — offsite durability, cross-region availability, or cost-effective long-term storage — switching to an S3-compatible file storage backend, or running both simultaneously, is a first-class feature built into CallFS's core engine.

This guide walks through every configuration option for the S3 backend, covers when to choose S3 over local disk, explains the hybrid storage model, and finishes with a quick smoke test to verify your setup.

---

## Local Storage vs S3 Storage

Before touching any config, it helps to be deliberate about which backend fits your situation.

**Local filesystem is the right choice when:**

- You need the lowest possible read latency. Serving files off a locally attached NVMe drive will always beat a round-trip to an object store.
- Your workload is write-heavy and latency-sensitive. Local writes are synchronous and bounded; S3 uploads add network overhead.
- You are running a single-node instance and durability is handled at the infrastructure level (RAID, snapshot-based backups, etc.).
- You want operational simplicity with no external dependencies for file data.

**S3 is the right choice when:**

- You need files to survive a complete server loss without a separate backup pipeline.
- You are storing large files that would saturate local disk capacity.
- Multiple CallFS instances need to read the same files without cross-server proxying.
- You have compliance requirements that mandate encryption at rest managed by your cloud provider's key infrastructure (SSE-KMS).
- You want to decouple file data from compute, so instances can be replaced or scaled without migrating data.

The two options are not mutually exclusive. The hybrid model described later in this guide lets you write to both simultaneously, giving you local read performance with S3 durability as a safety net.

---

## S3 Backend Configuration

The `backend` section of your `config.yaml` controls which storage backend CallFS uses by default and how it connects to S3.

```yaml
backend:
  default_backend: "s3"
  s3_access_key: "YOUR_ACCESS_KEY"
  s3_secret_key: "YOUR_SECRET_KEY"
  s3_region: "us-east-1"
  s3_bucket_name: "my-callfs-bucket"
  s3_endpoint: ""  # leave empty for AWS, or set for S3-compatible services
  s3_server_side_encryption: "AES256"  # AES256 | aws:kms | ""
  s3_acl: "private"
```

**Field reference:**

| Field | Description |
|---|---|
| `default_backend` | Sets whether new files go to `localfs` or `s3`. Existing files are always served from the backend recorded in their metadata. |
| `s3_access_key` | AWS access key ID or equivalent credential for S3-compatible services. |
| `s3_secret_key` | AWS secret access key or equivalent credential. |
| `s3_region` | AWS region for the bucket. Ignored by most S3-compatible services but still required to be set. |
| `s3_bucket_name` | The bucket CallFS will read from and write to. The bucket must exist before starting CallFS — the adapter calls `HeadBucket` at startup and fails fast if the bucket is unreachable. |
| `s3_endpoint` | Leave empty to use native AWS endpoints. Set to a URL for any S3-compatible service. |
| `s3_server_side_encryption` | See the encryption section below. |
| `s3_acl` | Object-level ACL applied on upload. `private` is the default and the recommended value for most deployments. |

### Environment Variable Overrides

Every configuration field can be set via environment variable, which is preferable for secrets in production. The naming convention is `CALLFS_` prefix followed by the key path with dots replaced by double underscores:

```sh
export CALLFS_BACKEND__S3_ACCESS_KEY="YOUR_ACCESS_KEY"
export CALLFS_BACKEND__S3_SECRET_KEY="YOUR_SECRET_KEY"
export CALLFS_BACKEND__S3_BUCKET_NAME="my-callfs-bucket"
```

Environment variables take highest priority and override anything in `config.yaml`.

---

## Using a Custom S3 Endpoint

Any service that speaks the S3 API can be used as CallFS's file storage backend. When you set `s3_endpoint`, CallFS automatically enables path-style addressing, which is required by most self-hosted S3-compatible services.

```yaml
backend:
  default_backend: "s3"
  s3_access_key: "YOUR_ACCESS_KEY"
  s3_secret_key: "YOUR_SECRET_KEY"
  s3_region: "us-east-1"
  s3_bucket_name: "my-callfs-bucket"
  s3_endpoint: "http://s3.internal:9000"
  s3_server_side_encryption: ""  # disable for most S3-compatible services
  s3_acl: "private"
```

**What changes when `s3_endpoint` is set:**

- Path-style addressing is forced (`S3ForcePathStyle: true`). Virtual-hosted-style URLs (`bucket.endpoint.com`) are not used.
- If the endpoint URL starts with `http://`, SSL is disabled automatically. HTTPS endpoints keep SSL enabled.
- MD5 content validation is disabled, which is necessary for compatibility with many S3-compatible services that do not implement that specific validation behavior.

If your endpoint requires HTTPS but uses a self-signed certificate, you will need to ensure the certificate is trusted by the system running CallFS, or place a reverse proxy with a valid certificate in front of your object store.

---

## Hybrid Storage with HA Replication

The most resilient configuration runs local filesystem as the primary backend and replicates every write to S3 asynchronously — or synchronously, if you need the guarantee. This is controlled through the `ha` section of the configuration.

```yaml
backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs/data"
  s3_bucket_name: "callfs-backup"
  s3_region: "us-east-1"
  s3_access_key: "KEY"
  s3_secret_key: "SECRET"

ha:
  replication_enabled: true
  replica_backend: "s3"
  require_replica_success: true
```

With `replication_enabled: true` and `replica_backend: "s3"`, every file write (both creates and updates) is followed by a replication step that copies the file to the S3 bucket. Deletes are also propagated — when a file is removed from local disk, CallFS attempts to remove the corresponding object from S3.

The `require_replica_success` flag controls the failure mode:

- **`true`**: The upload request fails if the S3 replication step fails. The client receives an error and the write is rolled back conceptually — the file exists on local disk but the upload is considered failed. Use this when you need a hard guarantee that every accepted write is durably backed up.
- **`false`**: S3 replication failures are logged as warnings but do not fail the request. The file is written to local disk and the client receives a success response. Use this when you prefer availability over strict durability.

---

## How It Works

Understanding the internal flow helps you reason about edge cases and failure modes.

**On upload (POST or PUT):**

1. CallFS acquires a distributed lock for the file path to prevent concurrent writes.
2. The file content is written to the primary backend (local disk, in the hybrid setup).
3. File metadata (path, size, owner, backend type, timestamps) is persisted to the configured metadata store.
4. If replication is enabled, the engine reads the file back from the primary backend and streams it to the replica backend (S3).
5. If `require_replica_success` is `true` and step 4 fails, the error is returned to the client.
6. The distributed lock is released and parent directory caches are invalidated.

**On download (GET):**

1. CallFS looks up the file's metadata from the metadata store (with an in-memory cache layer, TTL 5 minutes).
2. The `BackendType` field in the metadata record determines which backend to read from. A file written with `default_backend: "localfs"` will always be read from local disk, even when S3 replication is configured.
3. The file content is streamed from the backend directly to the HTTP response.

This design means downloads always read from the primary backend — there is no fallback read from S3 if the local file is missing. The S3 replica is a durability copy for disaster recovery, not a read fallback. If you need S3 as the read source, set `default_backend: "s3"` rather than using the replication feature.

---

## Server-Side Encryption Options

The `s3_server_side_encryption` field accepts three values:

### AES256 (S3-Managed Keys)

```yaml
s3_server_side_encryption: "AES256"
```

S3 manages the encryption keys. Every object is encrypted at rest using AES-256. This is the default and requires no additional configuration. It satisfies most compliance requirements without any AWS KMS dependency.

### aws:kms (KMS-Managed Keys)

```yaml
s3_server_side_encryption: "aws:kms"
s3_kms_key_id: "arn:aws:kms:us-east-1:123456789012:key/your-key-id"
```

AWS Key Management Service manages the encryption keys. This gives you key rotation control, cross-account access policies, and audit trails via CloudTrail. If `s3_kms_key_id` is left empty while using `aws:kms`, AWS uses the default S3 KMS key for your account.

### Disabled

```yaml
s3_server_side_encryption: ""
```

No server-side encryption is applied. Use this for S3-compatible services that do not support SSE, or when you are handling encryption at the infrastructure layer (encrypted volumes, etc.).

---

## Testing the Setup

Once your configuration is in place, use `curl` to verify that uploads flow to both local disk and S3.

**Step 1: Upload a test file**

```sh
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: text/plain" \
  --data-binary "hello from callfs" \
  https://your-callfs-host:8443/files/test/hello.txt
```

A `201 Created` response means the file was written to the primary backend and, if `require_replica_success: true`, confirmed in S3.

**Step 2: Read it back through CallFS**

```sh
curl -H "Authorization: Bearer YOUR_API_KEY" \
  https://your-callfs-host:8443/files/test/hello.txt
```

You should receive the file content. The `X-Backend-Type` response header (when present in debug builds) shows which backend served the file.

**Step 3: Verify the object in S3**

Use the AWS CLI or your S3-compatible service's console to check that the object exists:

```sh
aws s3 ls s3://my-callfs-bucket/test/
```

You should see `hello.txt` listed. The S3 key mirrors the CallFS path with the leading slash stripped — `/test/hello.txt` becomes `test/hello.txt` in the bucket.

**Step 4: Check local disk (hybrid setup only)**

If you are running the hybrid configuration, the file should also be present at the `localfs_root_path`:

```sh
ls -la /var/lib/callfs/data/test/
```

Both copies are independent. If you delete the file through CallFS, both the local copy and the S3 object are removed.

---

## Summary

CallFS's S3 backend support is built on the AWS SDK and works with any S3-compatible file storage backend. The key configuration decisions are:

- Set `default_backend: "s3"` for pure S3 storage, or keep `localfs` and enable HA replication for hybrid storage.
- Leave `s3_endpoint` empty for native AWS, or point it at your internal S3-compatible service for self-hosted deployments.
- Choose `AES256` for zero-configuration encryption, `aws:kms` when you need key management control, or an empty string to disable SSE.
- Set `require_replica_success: true` in the `ha` block when durability guarantees matter more than upload availability.

For most production workloads that need both performance and durability, the hybrid model — local primary with S3 replica and `require_replica_success: true` — is the recommended starting point.
