# API Reference

This document provides a comprehensive reference for the CallFS REST API. All endpoints are versioned under `/v1`. Deployments can run with HTTP, HTTPS, or auto mode.

## Authentication

All API endpoints, except for `/health` and `/download/{token}`, require bearer token authentication. The `/metrics` endpoint also requires authentication.

**Header:**
```
Authorization: Bearer <your-api-key>
```

## File and Directory Operations

These endpoints form the core of the filesystem API. They are designed to handle files and directories seamlessly, with intelligent routing in a clustered environment.

### `GET /v1/files/{path}`

Downloads a file or lists the contents of a directory.

- **If `{path}` is a file**: The response body will contain the raw file data.
  - **Headers**: `Content-Type: application/octet-stream`, `Content-Length`, `ETag`, and custom metadata headers (`X-CallFS-Mode`, `X-CallFS-	MTime`, etc.).
  - **Range requests**: Send `Range: bytes=start-end` to receive `206 Partial Content` with a `Content-Range` header. An unsatisfiable range returns `416 Range Not Satisfiable`.
- **If `{path}` is a directory**: The response body will be a JSON array of file and directory metadata objects.

Examples pass the server's CA certificate with `--cacert` so bearer keys are only sent over verified TLS. For a self-signed development certificate only, replace `--cacert` with `-k`; never skip verification in production.

**Example: Download a file**
```bash
curl --cacert /etc/callfs/tls/ca.pem -H "Authorization: Bearer <api-key>" \
  https://localhost:8443/v1/files/documents/report.pdf
```

**Example: List a directory**
```bash
curl --cacert /etc/callfs/tls/ca.pem -H "Authorization: Bearer <api-key>" \
  https://localhost:8443/v1/files/documents/
```

### `HEAD /v1/files/{path}`

Retrieves metadata for a file or directory without transferring its content. This is an **enhanced** operation.

- **Cross-Server Routing**: If the resource is located on another node in the cluster, this request will be automatically proxied to the correct node.
- **Response Headers**: Includes detailed metadata such as `X-CallFS-Type`, `X-CallFS-Size`, `X-CallFS-Mode`, `X-CallFS-Owner`, `X-CallFS-MTime`, `X-CallFS-Instance-ID`, and `ETag`.

**Example: Get file metadata**
```bash
curl --cacert /etc/callfs/tls/ca.pem -I -H "Authorization: Bearer <api-key>" \
  https://localhost:8443/v1/files/archive/dataset.zip
```

### `POST /v1/files/{path}`

Creates a new file or directory. This is an **enhanced** operation.

- **To create a file**: `POST` the raw file data with `Content-Type: application/octet-stream`.
- **To create a directory**: `POST` to a path ending with `/`. No request body is needed; the trailing slash signals directory creation.
- **Cross-Server Conflict Detection**: Before creating, CallFS checks if the resource already exists anywhere in the cluster. If a conflict is found, it returns a `409 Conflict` error with details about the existing resource.
- **Create-only guard**: Send `If-None-Match: *` to create the file only if nothing exists at `{path}`. If it exists, the request fails with `412 Precondition Failed`.
- **Response**: A successful create returns the new `ETag`.

**Example: Create a directory**
```bash
curl --cacert /etc/callfs/tls/ca.pem -X POST -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/octet-stream" \
  https://localhost:8443/v1/files/new-folder/
```

### `PUT /v1/files/{path}`

Uploads or updates a file's content. This is an **enhanced** operation.

- **Cross-Server Routing**: Automatically proxies the request to the node where the file is stored. If the file does not exist, it will be created on the default backend.
- **Streaming Support**: Efficiently handles large files by streaming data directly to the backend without buffering.
- **Conditional writes**: Send `If-Match: <etag>` to update only if the current `ETag` matches, or `If-None-Match: *` to create only if the file does not exist. A failed condition returns `412 Precondition Failed`.
- **Response**: A successful upload returns the new `ETag`.

**Example: Upload a file**
```bash
curl --cacert /etc/callfs/tls/ca.pem -X PUT -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/octet-stream" --data-binary @local-file.txt \
  https://localhost:8443/v1/files/documents/remote-file.txt
```

### `PATCH /v1/files/{path}`

Renames or moves a file or directory as a first-class operation. The request body (JSON) selects the operation; exactly one of `name` or `destination` must be present. Directories are renamed/moved together with their entire subtree. Both the metadata store and the underlying bytes are updated together (bytes first, then metadata), so a failed byte move leaves the source intact.

- **Cross-Server Routing**: a request that lands on a node which does not own the resource is automatically proxied to the owning (or, for relocations, the target) node.
- **Authorization**: requires ownership of the source and write access to the destination's parent directory.

**Request fields**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | — | New name (single path segment). Renames in place — same folder, backend, instance. |
| `destination` | string | — | Absolute destination path. Moves the resource. |
| `backend` | string | source's | Target backend (`localfs`, `s3`). |
| `instance` | string | source's | Target instance ID. |
| `overwrite` | bool | `false` | Replace an existing destination (also accepted as `?overwrite=true`). |
| `create_parents` | bool | `false` | Create missing destination parent directories. |

**Example: Rename in place**
```bash
curl --cacert /etc/callfs/tls/ca.pem -X PATCH -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" -d '{"name": "renamed.txt"}' \
  https://localhost:8443/v1/files/documents/report.txt
```

**Example: Move to another folder**
```bash
curl --cacert /etc/callfs/tls/ca.pem -X PATCH -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" \
  -d '{"destination": "/archive/2026/report.txt", "create_parents": true}' \
  https://localhost:8443/v1/files/documents/renamed.txt
```

**Example: Move a file to the S3 backend**
```bash
curl --cacert /etc/callfs/tls/ca.pem -X PATCH -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" \
  -d '{"destination": "/cold/report.txt", "backend": "s3"}' \
  https://localhost:8443/v1/files/archive/2026/report.txt
```

**Responses**: `200 OK` with body `{"path": "<new path>"}`; `400` for an invalid request (bad name, both/neither of `name`/`destination`, moving a directory into itself, or a missing destination parent without `create_parents`); `403` if the caller is not the source owner or lacks write access to the destination parent; `404` if the source does not exist; `409` if the destination exists and `overwrite` is false.

Moving a file to or from the `erasure` backend, or a directory across backends or instances, returns `400`.

### `DELETE /v1/files/{path}`

Deletes a file or an empty directory. This is an **enhanced** operation.

- **Cross-Server Routing**: Automatically proxies the delete request to the correct node in the cluster.
- **Recursive directory delete**: `DELETE /v1/files/<dir>?recursive=true` deletes a directory together with every file and subdirectory under it.

**Example: Delete a file**
```bash
curl --cacert /etc/callfs/tls/ca.pem -X DELETE -H "Authorization: Bearer <api-key>" \
  https://localhost:8443/v1/files/documents/obsolete-file.txt
```

**Example: Delete a directory tree**
```bash
curl --cacert /etc/callfs/tls/ca.pem -X DELETE -H "Authorization: Bearer <api-key>" \
  "https://localhost:8443/v1/files/documents/old-project?recursive=true"
```

### `GET /v1/files/ws/{path}?mode=download|upload`

Transfers files over WebSocket. Use `ws://` when running HTTP and `wss://` when running HTTPS.

- `mode=download`: Streams file bytes from server to client as binary websocket messages.
- `mode=upload`: Client sends binary websocket messages, server writes them as file content.
- Authentication is required via standard bearer token header during websocket handshake.

## Object Listing and Bulk Delete

### `GET /v1/objects?prefix=&start_after=&limit=`

Lists every file whose path starts with `prefix`, in path order, one page at a time.

- `prefix`: path prefix to match (optional).
- `start_after`: return only paths after this value; pass the previous page's `next_start_after` to continue.
- `limit`: page size, default `1000`, maximum `1000`.

Response:

```json
{
  "objects": [{"path": "/docs/a.txt", "size": 12, "etag": "\"3f2a...\"", "mtime": "2026-10-01T12:00:00Z"}],
  "truncated": true,
  "next_start_after": "/docs/a.txt"
}
```

### `DELETE /v1/objects?prefix=`

Deletes every file under `prefix`. Returns the number of files removed:

```json
{"deleted": 42}
```

**Example: Delete all files under a prefix**
```bash
curl --cacert /etc/callfs/tls/ca.pem -X DELETE -H "Authorization: Bearer <api-key>" \
  "https://localhost:8443/v1/objects?prefix=/tmp-uploads/"
```

## Enhanced Directory Listing

### `GET /v1/directories/{path}`

Provides a rich, JSON-formatted listing of a directory's contents with support for recursive traversal.

**Query Parameters:**
- `recursive` (boolean, optional): If `true`, lists contents of all subdirectories.
- `max_depth` (integer, optional): Limits the recursion depth when `recursive=true`.

**Example: Recursive listing with limited depth**
```bash
curl --cacert /etc/callfs/tls/ca.pem -H "Authorization: Bearer <api-key>" \
  "https://localhost:8443/v1/directories/projects/?recursive=true&max_depth=3"
```

**Response Format:**
```json
{
  "path": "/projects/",
  "type": "directory",
  "recursive": true,
  "max_depth": 3,
  "count": 15,
  "items": [
    { "name": "project-a", "path": "/projects/project-a", "type": "directory", ... },
    { "name": "file.go", "path": "/projects/project-a/file.go", "type": "file", ... }
  ]
}
```

## Single-Use Download Links

### `POST /v1/links/generate`

Generates a secure, time-limited, single-use download link for a file. This endpoint is rate-limited to prevent abuse.

**Request Body:**
```json
{
  "path": "/path/to/your/file.zip",
  "expiry_seconds": 3600
}
```

**Response Body:**
```json
{
  "url": "https://callfs.example.com/download/some-secure-token",
  "expires": "2025-07-15T18:00:00Z",
  "token": "some-secure-token"
}
```

### `GET /download/{token}`

Downloads a file using a single-use token. This endpoint **does not require authentication**. The token is invalidated immediately after the first successful download attempt or upon expiration.

**Example:**
```bash
curl -L https://callfs.example.com/download/some-secure-token -o downloaded-file.zip
```

## System Endpoints

These endpoints provide insight into the health and performance of the CallFS server.

### `GET /health`

A simple health check endpoint. Returns a `200 OK` with `{"status":"ok"}` if the service is running. **No authentication required.**

### `GET /metrics`

Exposes a wide range of performance metrics in Prometheus format for monitoring and alerting. **Authentication required.** A separate unauthenticated metrics endpoint can be configured on a dedicated port via `metrics.listen_addr`.

## Error Responses

Errors are returned with a standard JSON structure:
```json
{
  "code": "ERROR_CODE",
  "message": "A human-readable description of the error."
}
```

Error codes:

| Code | Status | Meaning |
|------|--------|---------|
| `BAD_REQUEST` | 400 | Malformed or invalid request (e.g. PATCH body fails validation). |
| `AUTHENTICATION_FAILED` | 401 | Missing or invalid credentials. |
| `PERMISSION_DENIED` | 403 | Caller is not allowed to perform the operation. |
| `FILE_NOT_FOUND` | 404 | File or directory does not exist. |
| `SHARD_NOT_FOUND` | 404 | Erasure-coded shard is missing. |
| `LINK_NOT_FOUND` | Per endpoint | Download link does not exist. |
| `LINK_EXPIRED` | Per endpoint | Download link has expired. |
| `LINK_INVALID` | Per endpoint | Download link is invalid or malformed. |
| `FILE_ALREADY_EXISTS` | 409 | Destination already exists. |
| `RESOURCE_LOCKED` | 409 | Resource is locked by another operation; retry. |
| `PRECONDITION_FAILED` | 412 | Conditional request precondition (e.g. ETag) did not match. |
| `PAYLOAD_TOO_LARGE` | 413 | Request body exceeds the size limit. |
| `INTERNAL_ERROR` | 500 | Unexpected server error. |

**Example: File Not Found**
```json
{
  "code": "FILE_NOT_FOUND",
  "message": "metadata not found"
}
```

## Console and Admin Endpoints

Access: `public` needs no credentials, `user` accepts an API key or a console session cookie with `X-CallFS-CSRF` on non-GET requests, `admin` also requires an admin user.

| Method | Route | Access | Purpose |
|---|---|---|---|
| POST | `/v1/session` | public | Sign in with an API key and start a console session |
| DELETE | `/v1/session` | user | End the console session |
| GET | `/health/ready` | public | Readiness: `ok`, `degraded` (200) or `down` (503) |
| GET | `/v1/me` | user | Caller identity, admin flag and session expiry |
| GET | `/v1/me/events` | user | Events caused by the caller |
| GET | `/v1/capabilities` | user | Build, enabled features, storages and limits |
| GET | `/v1/stat/{path}` | user | File or folder details |
| GET | `/v1/directories/{path}` | user | Folder listing with `limit`, `cursor`, `sort`, `order`, `type`, `q`, `include_hidden` |
| GET | `/v1/search` | user | Search metadata by name, size, time, owner, storage and more |
| GET | `/v1/usage` | user | Files, folders and bytes under a path |
| GET | `/v1/permissions/check` | user | Whether a user may read, write or delete a path, and why |
| GET | `/v1/objects?count_only=true` | user | Count objects under a prefix |
| DELETE | `/v1/objects?dry_run=true` | user | Preview or run a prefix delete |
| DELETE | `/v1/files/{path}?recursive=true&dry_run=true` | user | Preview or run a recursive delete |
| DELETE | `/v1/files/{path}?trash=true` | user | Move a file or folder to trash |
| PATCH | `/v1/files/{path}` | user | Change `mode`, or `owner` (admin only) |
| GET | `/v1/links` | user | List single-use links with `status`, `path`, `created_by` filters |
| GET | `/v1/links/{id}` | user | Link detail with open attempts |
| DELETE | `/v1/links/{id}` | user | Revoke a link |
| GET | `/v1/links/stats` | user | Link counts and open results for a window (`1h`, `24h`, `7d`) |
| POST | `/v1/links/generate` | user | Create a single-use link (returns `id`) |
| GET | `/v1/erasure/files` | user | Split files with last known health |
| GET | `/v1/erasure/files/{path}` | user | Shard states and fault tolerance of a split file |
| GET | `/v1/erasure/manifest/{path}` | user | Chunk manifest of a split file |
| POST | `/v1/erasure/verify/{path}` | user | Read every shard and report health, no writes |
| POST | `/v1/erasure/repair/{path}` | user | Start a repair job (owner or admin) |
| GET | `/v1/trash` | user | List trash items |
| POST | `/v1/trash/{id}/restore` | user | Restore a trash item, with `on_conflict` |
| DELETE | `/v1/trash/{id}` | user | Delete a trash item permanently |
| DELETE | `/v1/trash` | user | Empty trash as a job |
| POST | `/v1/batch` | user | Run up to 10,000 file operations, sync or as a job |
| POST | `/v1/uploads` | user | Start a resumable upload |
| PUT | `/v1/uploads/{id}` | user | Send the next part (`Content-Range`) |
| GET | `/v1/uploads/{id}` | user | Upload offset and state |
| POST | `/v1/uploads/{id}/complete` | user | Finish an upload |
| DELETE | `/v1/uploads/{id}` | user | Abort an upload |
| GET | `/v1/jobs` | user | Own jobs |
| GET | `/v1/jobs/{id}` | user | Job detail (own, or any for admin) |
| POST | `/v1/jobs/{id}/cancel` | user | Cancel a job |
| POST | `/v1/jobs/{id}/retry-failed` | user | Retry the failed items of a job |
| GET | `/v1/admin/status` | admin | Version, listeners, storages and runtime |
| POST | `/v1/admin/restart` | admin | Restart the process (needs `ui.allow_remote_restart`) |
| GET | `/v1/admin/migrations` | admin | Schema migration version and state |
| GET | `/v1/admin/health` | admin | Component health (`fresh=true` skips the 5 s cache) |
| GET | `/v1/admin/findings` | admin | Problems found in the current setup |
| POST | `/v1/admin/findings/{id}/mute` | admin | Mute a finding for `duration_s` |
| DELETE | `/v1/admin/findings/{id}/mute` | admin | Unmute a finding |
| GET | `/v1/admin/tls` | admin | Certificate, key and protocol details |
| GET | `/v1/admin/config` | admin | Config values with source and restart requirement |
| GET | `/v1/admin/config/schema` | admin | Config key schema |
| POST | `/v1/admin/config/validate` | admin | Validate proposed changes |
| PUT | `/v1/admin/config` | admin | Save changes to the config file (`expected_revision`) |
| PATCH | `/v1/admin/config/runtime` | admin | Apply runtime-safe changes |
| GET | `/v1/admin/config/export` | admin | Export config as `yaml`, `env` or `json` |
| GET | `/v1/admin/cluster` | admin | Raft membership, leader, quorum |
| POST | `/v1/admin/cluster/preflight` | admin | Check a node before joining |
| POST | `/v1/admin/cluster/nodes` | admin | Join a node |
| DELETE | `/v1/admin/cluster/nodes/{node_id}` | admin | Remove a node (`force=true` overrides quorum checks) |
| POST | `/v1/admin/cluster/leadership-transfer` | admin | Move leadership to a node |
| POST | `/v1/admin/cluster/snapshot` | admin | Take a Raft snapshot |
| GET | `/v1/admin/cluster/snapshots` | admin | List Raft snapshots |
| GET | `/v1/admin/cluster/state-stats` | admin | Counts of files, folders, links and index entries |
| GET | `/v1/admin/instances` | admin | Known servers and reachability |
| PUT | `/v1/admin/instances/{id}/endpoint` | admin | Set a server endpoint (`persist` saves it) |
| DELETE | `/v1/admin/instances/{id}/endpoint` | admin | Remove a server endpoint |
| POST | `/v1/admin/instances/{id}/probe` | admin | Probe a server now |
| GET | `/v1/admin/backends` | admin | Storage backends, space and last probe |
| POST | `/v1/admin/backends/{name}/probe` | admin | Write, read and delete a probe file |
| GET | `/v1/admin/erasure` | admin | Erasure coding summary and health counts |
| POST | `/v1/admin/erasure/plan` | admin | Plan shard placement and fault tolerance |
| POST | `/v1/admin/erasure/verify-all` | admin | Verify split files under a prefix as a job |
| GET | `/v1/admin/replication` | admin | Backup copy counts and recent failures |
| POST | `/v1/admin/replication/retry` | admin | Retry failed backup copies as a job |
| GET | `/v1/admin/staging` | admin | Leftover temporary files |
| DELETE | `/v1/admin/staging` | admin | Remove temporary files older than `older_than_s` as a job |
| GET | `/v1/admin/hygiene` | admin | Files without owner or version tag, leftovers |
| POST | `/v1/admin/retag` | admin | Compute missing version tags as a job |
| GET | `/v1/admin/locks` | admin | Held edit locks and lock statistics |
| DELETE | `/v1/admin/locks/{key}` | admin | Force release a lock (`reason` required) |
| GET | `/v1/admin/cache` | admin | Metadata cache statistics |
| POST | `/v1/admin/cache/invalidate` | admin | Invalidate a prefix or the whole cache |
| GET | `/v1/admin/ratelimits` | admin | Rate limits, rejections and top clients |
| GET | `/v1/admin/users` | admin | Users, key hints and activity |
| POST | `/v1/admin/users` | admin | Create a user and return its key once |
| POST | `/v1/admin/users/{id}/rotate` | admin | Rotate a user key |
| PATCH | `/v1/admin/users/{id}` | admin | Change admin flag |
| DELETE | `/v1/admin/users/{id}` | admin | Delete a user |
| POST | `/v1/admin/users/migrate-positional` | admin | Move positional keys to named users as a job |
| GET | `/v1/admin/owners` | admin | Owners with file and byte totals |
| POST | `/v1/admin/owners/reassign` | admin | Move ownership from one user to another as a job |
| GET | `/v1/admin/auth/failures` | admin | Recent failed sign-ins and requests |
| GET | `/v1/admin/metrics` | admin | Prometheus families and values |
| GET | `/v1/admin/metrics/series` | admin | Sampled series with rate, total, quantiles |
| GET | `/v1/admin/logs` | admin | Recent log entries with filters |
| GET | `/v1/admin/logs/stream` | admin | Live log entries (SSE) |
| GET | `/v1/admin/audit` | admin | Audit records with filters, `format=csv` available |
| GET | `/v1/admin/events` | admin | Live events (SSE, `Last-Event-ID` replay) |
| GET | `/v1/admin/events/history` | admin | Recent events |
| GET | `/v1/admin/transfers` | admin | Transfers in progress |
| GET | `/v1/admin/jobs` | admin | Jobs with state, kind and creator filters |
| GET | `/v1/admin/jobs/{id}` | admin | Job detail |
| POST | `/v1/admin/jobs/{id}/cancel` | admin | Cancel a job |
| POST | `/v1/admin/jobs/{id}/retry-failed` | admin | Retry the failed items of a job |
| GET | `/v1/admin/workers` | admin | Background workers and last results |
| POST | `/v1/admin/workers/{name}/run` | admin | Run a worker now as a job |
