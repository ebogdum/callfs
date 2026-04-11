# Backend Configuration

CallFS supports multiple storage backends, allowing you to choose the best storage strategy for your needs. You can use a local filesystem, an S3-compatible object store, or a combination of both in a distributed cluster.

## Backend Types

- **LocalFS**: Stores files directly on the local disk of a CallFS node. It's fast and simple, ideal for single-node deployments or for storing hot data in a cluster.
- **S3**: Uses Amazon S3 or any S3-compatible service (like MinIO, DigitalOcean Spaces, etc.) for object storage. This backend is highly scalable, durable, and perfect for large datasets.
- **Internal Proxy**: Not a storage backend itself, but a routing mechanism that enables cross-node operations in a distributed cluster.
- **No-Op**: A null backend that is used when a specific storage type is disabled.

## Local Filesystem Backend

This is the default backend. It's easy to set up and offers excellent performance.

**Configuration:**
```yaml
backend:
  localfs_root_path: "/var/lib/callfs"
```
Ensure the directory exists and the user running CallFS has read/write permissions.

**Security:**
- **Permissions**: Set restrictive permissions on the root path (e.g., `750`) and ensure it's owned by the `callfs` user.
- **Encryption**: For sensitive data, consider using filesystem-level encryption like LUKS on Linux.

## S3 Backend

The S3 backend allows CallFS to use scalable and durable object storage.

**Configuration:**
```yaml
backend:
  s3_access_key: "YOUR_S3_ACCESS_KEY"
  s3_secret_key: "YOUR_S3_SECRET_KEY"
  s3_region: "us-east-1"
  s3_bucket_name: "your-callfs-bucket"
  s3_endpoint: "" # Optional: for S3-compatible services like MinIO
  s3_server_side_encryption: "AES256" # "AES256", "aws:kms", or "" to disable
  s3_acl: "private"
  s3_kms_key_id: "" # Required if using aws:kms
```

> **Note:** For MinIO or S3-compatible services without KMS support, set `s3_server_side_encryption: ""` to disable server-side encryption.

**Security:**
- **IAM Best Practices**: Create a dedicated IAM user for CallFS with a least-privilege policy. The policy should only grant access to the specific S3 bucket and the necessary actions (`s3:GetObject`, `s3:PutObject`, `s3:DeleteObject`, `s3:ListBucket`).
- **Encryption at Rest**: Always enable server-side encryption on your S3 bucket. Use SSE-KMS for an additional layer of security with customer-managed keys.
- **Bucket Policy**: Use a bucket policy to enforce encryption and deny insecure (non-HTTPS) connections.

## Distributed Deployments and Backend Selection

In a clustered environment, CallFS intelligently manages file locations.

- **Default Backend**: You can configure a default backend (`localfs` or `s3`) for all new files.
  ```yaml
  backend:
    default_backend: "s3"
  ```
- **File Location**: The metadata store tracks which backend and which CallFS node a file resides on.
- **Automatic Routing**: When you request a file, CallFS uses the metadata to determine its location. If the file is on another node, the request is automatically proxied via the **Internal Proxy Backend**. This process is transparent to the end-user.

### Hybrid Configuration Example

This example shows a node configured to use both local storage and S3, with S3 as the default for new files.

```yaml
backend:
  default_backend: "s3"
  localfs_root_path: "/data/hot-storage"
  s3_bucket_name: "my-callfs-archive"
  # ... other s3_ config keys
```

In this setup:
- New files are written to the S3 bucket by default.
- The system can still read and serve files from its local filesystem if they exist there (e.g., for legacy data or hot-tier storage).
- It can also access files stored on the local filesystems of other nodes in the cluster.
