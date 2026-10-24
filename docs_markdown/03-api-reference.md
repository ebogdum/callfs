# API reference

This page covers the public HTTP API of CallFS 2.0.0: files, directories, objects, batch, stat, search, trash, resumable uploads, single-use links, your account, permissions, capabilities, usage, jobs, erasure-coded files, the websocket transfer endpoint and the health endpoints.

Admin routes (`/v1/admin/*` and the node gateway `/v1/nodes/*`) are in [10-admin-api.md](10-admin-api.md).

Node-to-node routes under `/internal/` and `/v1/internal/` exist only for traffic between cluster nodes. They are protected by the shared internal secret and are not part of the public API.

Examples use `https://files.example.com` and the key `your-api-key`.

## Conventions

### Authentication

Every `/v1` route except `POST /v1/session` needs one of:

| Method | How |
|---|---|
| API key | Header `Authorization: Bearer your-api-key`. The key on its own (without `Bearer `) is also accepted. |
| Console session | The `callfs_session` cookie from `POST /v1/session`. Every request that is not GET, HEAD or OPTIONS must also send the header `X-CallFS-CSRF` with the `csrf_token` value from the session. |

Details on keys, users and sessions are in [04-authentication-security.md](04-authentication-security.md).

After 10 failed key attempts from one client address, further attempts are answered with 429 until the allowance refills (one attempt every 6 seconds).

A failed login returns 401 with the header `WWW-Authenticate: Bearer`. `POST /v1/session` with a wrong key also returns 401.

### Permissions

Access is decided by the owner of each file or directory and by its mode bits. CallFS users are not system users; only the "other" bits of the mode apply to people who do not own the item.

- The owner and the `root` user can read, write and delete.
- Everyone else (including other admins) is limited by the "other" bits of the mode. The read bit (`4`) allows reading a file or listing a directory. The write bit (`2`) allows replacing a file or creating items inside a directory.
- Deleting (and renaming or moving) an existing item is for the owner and `root` only.
- Admins (`root` and the users in `auth.admin_users`) bypass file and folder permissions (read, write, delete) and can also change owners and modes, move data between storages and servers, and see everyone's jobs, links and trash.
- Creating something new needs write access to its parent directory.
- New directories get mode `0777` and new files `0644`, so by default everyone can read and create in directories, and read (but not change) files. Change a mode with [`PATCH`](#patch-v1filespath).
- Requests without the needed access return 403 `PERMISSION_DENIED`.

Use [`GET /v1/permissions/check`](#get-v1permissionscheck) to ask what is allowed.

### Paths

- A path is everything after `/v1/files/`. A leading slash is optional in bodies and query parameters.
- A trailing slash means directory (`/v1/files/docs/`). No trailing slash means file.
- `..` segments are resolved before use. Paths that escape the root, contain a backslash or contain invalid characters return 400.

### Request headers (all authenticated routes)

| Header | Value | What it does |
|---|---|---|
| `Authorization` | `Bearer your-api-key` | Authenticates the request. |
| `X-CallFS-Write-Ack` | `memory` or `disk` | Chooses, for this request, when a write is acknowledged. Any other value returns 400 with a plain-text message. See [05-storage.md](05-storage.md). |
| `X-CallFS-CSRF` | session CSRF token | Required with session cookies on non-safe methods. |

### Response headers (all routes)

| Header | What it is |
|---|---|
| `X-Request-ID` | Unique ID of the request. Quote it when reporting a problem. |
| `ETag` | Quoted content hash of a file (`"3f2a..."`). Returned by GET, HEAD, PUT, POST and cross-server writes. |
| `X-CallFS-Type` | `file` or `directory`. |
| `X-CallFS-Size` | Size in bytes (`0` for directories). |
| `X-CallFS-Mode` | Octal mode, for example `0644`. |
| `X-CallFS-Owner` | Owner user ID. |
| `X-CallFS-MTime` | Last modified time, RFC 3339. |
| `X-CallFS-Instance-ID` | HEAD only. ID of the server that stores the file. |
| `X-CallFS-Count`, `X-CallFS-Recursive` | Directory listing at `/v1/directories/`. |
| `Retry-After` | Seconds to wait, on rate-limit 429 responses. |

Security headers (CSP, `X-Content-Type-Options`, `X-Frame-Options`, HSTS on TLS and others) are added to every response.

### Error body

Errors are JSON:

```json
{
  "code": "FILE_NOT_FOUND",
  "message": "file not found",
  "details": {}
}
```

`details` appears only on some errors. Responses to HEAD requests have no body.

| Status | `code` | When |
|---|---|---|
| 400 | `BAD_REQUEST` | Bad path, parameter, header or body. `message` says what. |
| 400 | `NAME_TOO_LONG` | A file or directory name is too long. |
| 401 | `AUTHENTICATION_FAILED` | Missing, wrong or expired key or session. |
| 403 | `PERMISSION_DENIED` | You may not do this. |
| 403 | `ADMIN_REQUIRED` | The action needs an admin. |
| 403 | `CSRF_FAILED` | Session request without a valid `X-CallFS-CSRF`. |
| 404 | `FILE_NOT_FOUND` | Path does not exist. |
| 404 | `NOT_FOUND` | Other resource (upload) does not exist. |
| 404 | `SHARD_NOT_FOUND` | Shard does not exist. |
| 404 | `FEATURE_DISABLED` | Erasure coding is off. `details.feature` is `erasure`. |
| 404 | `FILE_NOT_FOUND` | Also returned by the erasure routes when the file has no erasure record. |
| 404 | `LINK_NOT_FOUND` | Link or token does not exist. |
| 409 | `FILE_ALREADY_EXISTS` | Target already exists. |
| 409 | `CONFLICT` | State conflict, for example a non-empty directory. |
| 409 | `RESOURCE_LOCKED` | Another operation holds the path. Retry. |
| 410 | `LINK_EXPIRED`, `LINK_INVALID` | Download link expired, used or revoked. |
| 412 | `PRECONDITION_FAILED` | `If-Match` or `If-None-Match` condition failed. |
| 413 | `PAYLOAD_TOO_LARGE` | Body too big. |
| 413 | `SUBTREE_TOO_LARGE` | Directory tree too large for the operation. |
| 416 | `RANGE_NOT_SATISFIABLE` | Range outside the file. |
| 429 | `RATE_LIMIT_EXCEEDED` | Rate limit hit. See `Retry-After`. |
| 500 | `INTERNAL_ERROR` | Server fault. The message is generic. |
| 501 | `NOT_IMPLEMENTED` | Message `this server is not set up for this request`. Feature not enabled, for example `X-CallFS-Erasure: true` while erasure coding is off. |
| 503 | `INTERNAL_ERROR` | No free websocket upload slot. |
| 502 | `BAD_GATEWAY` | The server that owns the file failed. |
| 507 | `INSUFFICIENT_STORAGE` | Storage is full. |

Other status codes use the upper-case status text as `code`.

Unknown routes return the plain-text body `404 page not found`.

## Files: `/v1/files/{path}`

### GET /v1/files/{path}

Downloads a file, or lists a directory when the path ends in `/`.

Auth: read access.

**Query parameters**

| Name | Values | What it does |
|---|---|---|
| `manifest` | `true` | For erasure-coded files, returns the shard manifest instead of the content. |

**Request headers**

| Header | What it does |
|---|---|
| `Range` | `bytes=0-99`, `bytes=100-` or `bytes=-500` (last 500 bytes). One range only. A malformed or multiple range is ignored and the whole file is sent. |
| `If-Range` | Quoted ETag. If it does not match the current ETag the range is ignored and the whole file is sent. |
| `If-None-Match` | One or more ETags, or `*`. A match returns 304 with no body. |

**File response** (200, or 206 for a range): body is the raw bytes, `Content-Type: application/octet-stream`.

Headers: `Content-Length`, `Accept-Ranges: bytes`, `ETag`, `X-CallFS-Type`, `X-CallFS-Size`, `X-CallFS-Mode`, `X-CallFS-Owner`, `X-CallFS-MTime`, and for 206 `Content-Range: bytes 0-99/1024`.

**Directory response** (200): JSON array of the directory's direct children.

```json
[
  {
    "name": "report.pdf",
    "path": "/docs/report.pdf",
    "type": "file",
    "size": 20480,
    "mode": "0644",
    "owner": "alice",
    "mtime": "2026-01-15T10:30:00Z"
  }
]
```

For paging, sorting and filtering use [`/v1/directories/`](#get-v1directoriespath).

**Status codes:** 200, 206, 304, 400, 401, 403, 404, 416 (`Content-Range: bytes */SIZE`, no body), 500.

```bash
curl -H "Authorization: Bearer your-api-key" \
  -o report.pdf https://files.example.com/v1/files/docs/report.pdf

curl -H "Authorization: Bearer your-api-key" -H "Range: bytes=0-1023" \
  https://files.example.com/v1/files/docs/report.pdf

curl -H "Authorization: Bearer your-api-key" -H 'If-None-Match: "3f2a9c"' -i \
  https://files.example.com/v1/files/docs/report.pdf
```

### HEAD /v1/files/{path}

Returns the metadata headers (`X-CallFS-Type`, `X-CallFS-Size`, `X-CallFS-Mode`, `X-CallFS-Owner`, `X-CallFS-MTime`, `X-CallFS-Instance-ID` when known, `ETag` when known) with no body.

Auth: read access. Status codes: 200, 400, 401, 403, 404, 500.

```bash
curl -I -H "Authorization: Bearer your-api-key" \
  https://files.example.com/v1/files/docs/report.pdf
```

### PUT /v1/files/{path}

Creates a file, or replaces its content. No trailing slash.

Auth: write access (to the file, or to the parent directory when the file is new).

**Body:** the raw file bytes. Chunked transfer encoding is accepted. Maximum 10 GiB.

**Erasure-coded files:** a new file (no `If-Match`) is stored as shards when you send `X-CallFS-Erasure: true`, or automatically when `Content-Length` is at least `erasure.min_file_size` and at most 1 GiB. Send `X-CallFS-Erasure: false` to store the file whole instead. The body must not be empty. The shard options of [POST](#post-v1filespath) apply. Replacing an existing file keeps its current storage. `X-CallFS-Erasure: true` while erasure coding is off returns 501.

**Request headers**

| Header | What it does |
|---|---|
| `If-Match` | ETag of the version you expect (quotes optional). The write is refused with 412 if the file has changed. `*` means "the file must exist". |
| `If-None-Match` | Only `*` is accepted: create only, refuse with 412 if the file exists. Any other value returns 400. |
| `X-CallFS-Write-Ack` | `memory` or `disk`. |
| `X-CallFS-True-Overwrite` | `true` or `false`. Picks the overwrite way for this request and wins over `backend.localfs_true_overwrite` and `backend.localfs_tmp_save_max_size`. See [Overwrites](05-storage.md#overwrites). Any other value returns 400. |
| `X-CallFS-Erasure` | `true` stores a new file as shards, `false` stores it whole even above `erasure.min_file_size`. |

**Response:** no body. 201 when the file was created, 200 when it was replaced. Header `ETag` carries the new content hash. A file stored on another cluster node is updated on that node.

**Status codes:** 200, 201, 400 (directory path, directory target, bad condition header, bad `X-CallFS-True-Overwrite` value, empty erasure body), 401, 403, 409, 412, 413, 501 (erasure requested but off), 502, 507, 500.

```bash
curl -X PUT -H "Authorization: Bearer your-api-key" \
  --data-binary @report.pdf -i \
  https://files.example.com/v1/files/docs/report.pdf

curl -X PUT -H "Authorization: Bearer your-api-key" -H "If-None-Match: *" \
  --data-binary @new.txt https://files.example.com/v1/files/docs/new.txt

curl -X PUT -H "Authorization: Bearer your-api-key" -H 'If-Match: "3f2a9c"' \
  --data-binary @report.pdf https://files.example.com/v1/files/docs/report.pdf
```

### POST /v1/files/{path}

Creates something new. Fails if the path already exists.

Auth: write access to the parent directory.

- Path ending in `/`: creates a directory (mode `0777`). 201. If the directory already exists, 200.
- Path without trailing slash: creates a file from the body (mode `0644`). 201 with `ETag`. If the path exists, 409.

**Body:** the file bytes (files), nothing for directories. Maximum 10 GiB for plain files.

**Erasure-coded files**

Send `X-CallFS-Erasure: true` (or `?erasure=true`) to store the file as shards (see [05-storage.md](05-storage.md)). Files whose `Content-Length` is at least `erasure.min_file_size` (and at most 1 GiB) are split automatically; send `X-CallFS-Erasure: false` to store such a file whole. Maximum 1 GiB for split files, and the body must not be empty.

| Header | Query | What it does |
|---|---|---|
| `X-CallFS-True-Overwrite` | | `true` or `false`. Same as on PUT. A bad value returns 400. |
| `X-CallFS-Erasure-Data-Shards` | `data_shards` | Data shards, integer 1 or more. Server default when omitted. |
| `X-CallFS-Erasure-Parity-Shards` | `parity_shards` | Parity shards, integer 1 or more. |
| `X-CallFS-Erasure-Instances` | `instances` | Comma-separated server IDs to place shards on. At most 64. |

Data plus parity shards must not exceed 255. If erasure coding is not enabled the request returns 501.

**Conflict on another server:** when the existing path lives on another cluster node, the 409 body is:

```json
{
  "error": "Resource exists on another server",
  "existing_path": "/docs/report.pdf",
  "instance_id": "node-2",
  "backend_type": "localfs",
  "suggestion": "File already exists on another server. Use PUT to update it."
}
```

**Status codes:** 200 (directory exists), 201, 400, 401, 403, 409, 413, 501, 507, 500.

```bash
curl -X POST -H "Authorization: Bearer your-api-key" \
  https://files.example.com/v1/files/projects/

curl -X POST -H "Authorization: Bearer your-api-key" --data-binary @a.txt \
  https://files.example.com/v1/files/projects/a.txt

curl -X POST -H "Authorization: Bearer your-api-key" \
  -H "X-CallFS-Erasure: true" -H "X-CallFS-Erasure-Data-Shards: 4" \
  -H "X-CallFS-Erasure-Parity-Shards: 2" --data-binary @big.bin \
  https://files.example.com/v1/files/archive/big.bin
```

### PATCH /v1/files/{path}

Renames, moves, or changes the mode or owner of a file or directory. Body is JSON, at most 1 MiB.

| Field | Type | What it does |
|---|---|---|
| `name` | string | Rename in place. A single name, no slashes. |
| `destination` | string | Move to this absolute path. |
| `overwrite` | bool | Replace an existing destination. Also settable with `?overwrite=true`. |
| `create_parents` | bool | Create missing parent directories of the destination. |
| `backend` | string | Move the data to another storage (`localfs` or `s3`). Admin only. |
| `instance` | string | Move the data to another cluster node. Admin only. |
| `mode` | string | Octal permission such as `0640`. |
| `owner` | string | New owner user ID, up to 255 characters, no surrounding spaces. Admin only. |

Use exactly one of `name` or `destination`, or use `mode` and/or `owner` alone. Mixing them returns 400.

Moving data to another storage or server (`backend`, `instance`) is for admins. Other users get 403.

Auth: rename and move need delete access to the source and write access to the destination. `mode` and `owner` need ownership or admin.

**Response (rename, move):**

```json
{ "path": "/docs/final.pdf" }
```

**Response (mode, owner):**

```json
{ "path": "/docs/report.pdf", "mode": "0640", "owner": "bob" }
```

**Status codes:** 200, 400, 401, 403 (`PERMISSION_DENIED`, or `ADMIN_REQUIRED` for `owner`), 404, 409 (destination exists without `overwrite`), 502, 500.

```bash
curl -X PATCH -H "Authorization: Bearer your-api-key" \
  -d '{"name":"final.pdf"}' https://files.example.com/v1/files/docs/report.pdf

curl -X PATCH -H "Authorization: Bearer your-api-key" \
  -d '{"destination":"/archive/2026/report.pdf","create_parents":true}' \
  https://files.example.com/v1/files/docs/report.pdf

curl -X PATCH -H "Authorization: Bearer your-api-key" \
  -d '{"mode":"0600"}' https://files.example.com/v1/files/docs/report.pdf
```

### DELETE /v1/files/{path}

Deletes a file or directory permanently. It does not use the trash; to delete into the trash use the `trash` operation of [`/v1/batch`](#post-v1batch).

Auth: delete access.

**Query parameters**

| Name | Values | What it does |
|---|---|---|
| `recursive` | `true` | Delete a directory with everything inside. Without it, a non-empty directory returns 409. |
| `dry_run` | `true` | Delete nothing. Returns what would be deleted. |

**Response:** 204 with no body. With `recursive=true` on a local directory: 200 and

```json
{ "deleted": 12, "deleted_files": 12, "deleted_directories": 3 }
```

**Dry run response (200):**

```json
{
  "files": 12,
  "directories": 3,
  "bytes": 1048576,
  "erasure_files": 1,
  "remote_instances": ["node-2"]
}
```

**Status codes:** 200, 204, 400, 401, 403, 404, 409, 502, 500.

```bash
curl -X DELETE -H "Authorization: Bearer your-api-key" \
  https://files.example.com/v1/files/docs/old.txt

curl -X DELETE -H "Authorization: Bearer your-api-key" \
  "https://files.example.com/v1/files/docs/?recursive=true&dry_run=true"
```

## Directories

### GET /v1/directories/{path}

Lists a directory. The path is a directory; the trailing slash is optional. `/v1/directories/` lists the root.

Auth: read access. A path that is a file returns 400.

**Simple listing (no `limit`)**

| Query | Default | What it does |
|---|---|---|
| `recursive` | `false` | `true` lists all descendants. Also works with the paged listing below. |
| `max_depth` | 100 | Recursion depth, 0 to 1000. Invalid values use 100. Also works with the paged listing. |

The listing is capped at 10000 entries; then `truncated` is `true`.

```json
{
  "path": "/docs",
  "type": "directory",
  "recursive": true,
  "max_depth": 100,
  "count": 2,
  "truncated": false,
  "items": [
    {"name": "a.txt", "path": "/docs/a.txt", "type": "file", "size": 5, "mode": "0644", "owner": "alice", "mtime": "2026-01-15T10:30:00Z"}
  ]
}
```

Headers: `X-CallFS-Type: directory`, `X-CallFS-Count`, `X-CallFS-Recursive`.

**Paged listing (send `limit`)**

| Query | Default | What it does |
|---|---|---|
| `limit` | required | 1 to 1000. Larger values are lowered to 1000. |
| `cursor` | | `next_cursor` from the previous page. |
| `sort` | `name` | `name`, `size`, `mtime` or `type`. Sorting by `name` puts directories first. |
| `order` | `asc` | `asc` or `desc`. |
| `type` | | `file` or `directory`. |
| `q` | | Case-insensitive name contains. |
| `include_hidden` | `true` | `false` hides names starting with a dot. |
| `recursive`, `max_depth` | `false`, 100 | As above: page through all descendants instead of direct children only. |

```json
{
  "path": "/docs",
  "type": "directory",
  "count": 50,
  "total": 120,
  "items": [
    {
      "name": "report.pdf", "path": "/docs/report.pdf", "type": "file",
      "size": 20480, "mode": "0644", "owner": "alice",
      "mtime": "2026-01-15T10:30:00Z",
      "etag": "\"3f2a9c\"", "storage": "localfs", "instance_id": "node-1",
      "erasure_coded": false, "locked": false
    }
  ],
  "next_cursor": "NTA",
  "partial": false
}
```

`next_cursor` is `null` on the last page. `partial` is `true` when the directory holds more than 10000 entries and only the first 10000 were considered.

**Status codes:** 200, 400 (bad parameter), 401, 403, 404, 500.

```bash
curl -H "Authorization: Bearer your-api-key" \
  "https://files.example.com/v1/directories/docs?limit=50&sort=mtime&order=desc&type=file"
```

## Objects

Flat, prefix-based access to all stored items, like an object store.

### GET /v1/objects

| Query | Default | What it does |
|---|---|---|
| `prefix` | empty | Only paths that start with this prefix. |
| `limit` | 1000 | 1 to 1000. Larger values are lowered; 0 or empty means 1000. |
| `start_after` | | Return paths after this one. Use `next_start_after`. |
| `count_only` | | `true` returns totals instead of a list. |

Auth: read access to every returned item (a single forbidden item returns 403).

```json
{
  "objects": [
    {"path": "/docs/a.txt", "size": 5, "etag": "\"3f2a9c\"", "mtime": "2026-01-15T10:30:00Z"}
  ],
  "truncated": true,
  "next_start_after": "/docs/a.txt"
}
```

`count_only=true` returns `{"count": 120, "bytes": 5242880, "truncated": false}`; counting stops at 1,000,000 items and sets `truncated`.

Status codes: 200, 400 (`limit` not a number), 401, 403, 500.

```bash
curl -H "Authorization: Bearer your-api-key" \
  "https://files.example.com/v1/objects?prefix=/logs/2026/&limit=500"
```

### DELETE /v1/objects

Deletes everything under a prefix, permanently.

| Query | What it does |
|---|---|
| `prefix` | Required. `/` is refused with 400. |
| `boundary` | `true` matches only whole path segments (`/logs` matches `/logs/a` but not `/logs2`). |
| `dry_run` | `true` deletes nothing and returns the plan. |

Auth: delete access to every item.

Response: `{"deleted": 42}`. Dry run: `{"would_delete": 42, "bytes": 1048576, "forbidden": 0}`.

If the delete fails part way, the response is 500 and the body reports progress:

```json
{
  "code": "INTERNAL_ERROR",
  "message": "an internal error occurred",
  "details": {"deleted": 10, "failed_path": "/logs/2026/x.log"}
}
```

Status codes: 200, 400, 401, 403, 500.

```bash
curl -X DELETE -H "Authorization: Bearer your-api-key" \
  "https://files.example.com/v1/objects?prefix=/tmp/&boundary=true&dry_run=true"
```

## Batch: `/v1/batch`

### POST /v1/batch

Runs many operations in one request. Needs a signed-in user; size limit 4 MiB.

| Field | Type | What it does |
|---|---|---|
| `ops` | array | 1 to 10000 operations. |
| `mode` | string | `sync` (default, at most 100 ops, results in the response) or `job` (runs in the background). |
| `stop_on_error` | bool | Stop after the first failed operation. |

Each operation:

| Field | Used by | What it does |
|---|---|---|
| `op` | all | `delete`, `trash`, `move`, `rename`, `mode` or `restore`. |
| `path` | all | Target path. For `restore` it is the trash item ID. |
| `destination` | `move`, `restore` | Target path. For `restore` it is optional (default: original path). |
| `name` | `rename` | New name. |
| `mode` | `mode` | Octal such as `0644`. |
| `recursive` | `delete` | Delete a directory with its contents. |
| `overwrite` | `move`, `restore` | Replace an existing target. |
| `create_parents` | `move` | Create missing parents. |
| `storage`, `instance` | `move` | Move to another storage or node. Admin only. |

Permissions are checked per operation as for the matching single call. `trash` moves into the trash (see [Trash](#trash)); the root cannot be changed.

**Sync response (200):**

```json
{
  "results": [
    {"index": 0, "status": "ok", "code": "OK", "message": "", "path": "/docs/a.txt"},
    {"index": 1, "status": "failed", "code": "FILE_NOT_FOUND", "message": "file not found", "path": "/docs/b.txt"}
  ]
}
```

Result codes: `OK`, `BAD_REQUEST`, `PERMISSION_DENIED`, `FILE_NOT_FOUND`, `FILE_ALREADY_EXISTS`, `CONFLICT`, `RESOURCE_LOCKED`, `BAD_GATEWAY`, `INTERNAL_ERROR`.

**Job response (202):** `{"job_id": "..."}`. Follow it with [`/v1/jobs/{id}`](#jobs). The job result is `{"processed": 500, "failed": 2}`.

Status codes: 200, 202, 400 (bad body, too many ops, unknown `op`), 401.

```bash
curl -X POST -H "Authorization: Bearer your-api-key" -d '{
  "ops": [
    {"op": "trash", "path": "/docs/old.txt"},
    {"op": "move", "path": "/docs/a.txt", "destination": "/archive/a.txt", "create_parents": true},
    {"op": "mode", "path": "/docs/b.txt", "mode": "0600"}
  ],
  "stop_on_error": true
}' https://files.example.com/v1/batch
```

## Stat: `/v1/stat/{path}`

### GET /v1/stat/{path}

Full metadata for one file or directory, including a lock flag and child count.

Auth: read access.

```json
{
  "path": "/docs/report.pdf",
  "name": "report.pdf",
  "type": "file",
  "size": 20480,
  "mode": "0644",
  "owner": "alice",
  "etag": "\"3f2a9c\"",
  "storage": "localfs",
  "instance_id": "node-1",
  "is_local": true,
  "erasure_coded": false,
  "symlink_target": null,
  "mtime": "2026-01-15T10:30:00Z",
  "atime": "2026-01-15T10:30:00Z",
  "ctime": "2026-01-15T10:30:00Z",
  "created_at": "2026-01-15T10:30:00Z",
  "updated_at": "2026-01-15T10:30:00Z",
  "locked": false,
  "children_count": null
}
```

`children_count` is a number for directories. `locked` is `true` while a write holds the path.

Status codes: 200, 400, 401, 403, 404, 500.

```bash
curl -H "Authorization: Bearer your-api-key" \
  https://files.example.com/v1/stat/docs/report.pdf
```

## Search: `/v1/search`

### GET /v1/search

Searches names under a directory, recursively. Matches are limited to items you may read.

| Query | Default | What it does |
|---|---|---|
| `path` | `/` | Directory to search in. Must exist and be a directory. |
| `q` | | Name filter, up to 256 characters. Case-insensitive. Plain text means "contains". `*`, `?` and `[...]` make it a glob over the whole name. |
| `type` | | `file` or `directory`. |
| `min_size`, `max_size` | | Size bounds in bytes, 0 or more. `min_size` must not exceed `max_size`. |
| `modified_after`, `modified_before` | | RFC 3339 times. |
| `owner` | | Exact owner. |
| `storage` | | Exact storage name (`localfs`, `s3`, `erasure`). |
| `instance` | | Exact server ID. |
| `erasure` | | `true` or `false`. |
| `locked` | | `true` returns only items an editor currently holds locked, `false` only items that are not locked. |
| `limit` | 100 | 1 to 1000. |
| `cursor` | | `next_cursor` from the previous response. |
| `facets` | `false` | `true` adds counts by type, storage, owner and instance. |

At most 50000 entries are scanned per request; `scan_truncated` tells you when that limit was hit.

```json
{
  "items": [
    {
      "path": "/docs/report.pdf", "name": "report.pdf", "type": "file",
      "size": 20480, "mode": "0644", "owner": "alice",
      "etag": "\"3f2a9c\"", "storage": "localfs",
      "instance_id": "node-1", "is_local": true, "erasure_coded": false,
      "mtime": "2026-01-15T10:30:00Z", "updated_at": "2026-01-15T10:30:00Z"
    }
  ],
  "next_cursor": "",
  "took_ms": 12,
  "scan_truncated": false,
  "scanned": 340,
  "facets": {
    "type": {"file": 1},
    "storage": {"localfs": 1},
    "owner": {"alice": 1},
    "instance": {"node-1": 1}
  }
}
```

`next_cursor` is an empty string on the last page. `facets` appears only when requested.

Status codes: 200, 400 (bad parameter, including a `locked` value that is not `true` or `false`), 401, 403 (you may not read the search path), 404 (search path missing), 500.

```bash
curl -H "Authorization: Bearer your-api-key" \
  "https://files.example.com/v1/search?path=/docs&q=*.pdf&min_size=1048576&facets=true"
```

## Trash

Trash routes exist when `trash.enabled` is on. Files get into the trash with the batch `trash` operation or from the console. Items are removed for good after `trash.retention`; the cleanup runs on one server of the cluster. You see only your own items; admins see all.

### GET /v1/trash

| Query | Default | What it does |
|---|---|---|
| `limit` | 200 | 1 to 1000. |
| `cursor` | | From `next_cursor`. |
| `deleted_by` | | Admins: filter by user, or `*` for everyone. Others may only use their own ID. |

```json
{
  "items": [
    {
      "id": "9f1c2b",
      "original_path": "/docs/old.txt",
      "name": "old.txt",
      "type": "file",
      "size": 512,
      "deleted_by": "alice",
      "deleted_at": "2026-01-15T10:30:00Z",
      "expires_at": "2026-02-14T10:30:00Z"
    }
  ],
  "next_cursor": null,
  "total_bytes": 512
}
```

Status codes: 200, 400, 401, 403.

### POST /v1/trash/{id}/restore

Optional JSON body (4 KiB max):

| Field | What it does |
|---|---|
| `to` | Restore to this path instead of the original. |
| `on_conflict` | If the target exists: `fail` (default, returns 409), `keep_both` (restores under a new name) or `replace`. |

Response: `{"path": "/docs/old.txt"}`. Needs write access at the target and ownership of the trash item (or admin).

Status codes: 200, 400, 401, 403, 404, 409.

### DELETE /v1/trash/{id}

Deletes one item for good. Response 204. Status codes: 204, 401, 403, 404.

### DELETE /v1/trash

Empties the trash in the background (`deleted_by` works as for listing). Response 202: `{"job_id": "..."}`. Follow it with [`/v1/jobs/{id}`](#jobs).

```bash
curl -H "Authorization: Bearer your-api-key" "https://files.example.com/v1/trash?limit=50"

curl -X POST -H "Authorization: Bearer your-api-key" \
  -d '{"on_conflict":"keep_both"}' \
  https://files.example.com/v1/trash/9f1c2b/restore

curl -X DELETE -H "Authorization: Bearer your-api-key" https://files.example.com/v1/trash
```

## Resumable uploads

Upload large files in parts that you can resume. Routes exist on every server. A session lasts 24 hours. Parts are exactly 8 MiB (the last one may be smaller). A file may be up to 10 GiB (1 GiB for erasure-coded). Sessions belong to the user who created them.

The flow: create a session, send each part with `PUT`, then complete.

### POST /v1/uploads

JSON body (64 KiB max; unknown fields return 400):

| Field | Type | What it does |
|---|---|---|
| `path` | string | Required. The target file path (no trailing slash). |
| `size` | int | Required. Total size in bytes. |
| `overwrite` | bool | Allow replacing an existing file. |
| `if_match` | string | Replace only if the file has this ETag. |
| `mode` | string | Octal mode. Default `0644`. |
| `storage` | string | `localfs` or `s3`. Default is the configured default. |
| `erasure` | object | `data_shards`, `parity_shards`, `instances` as in [POST /v1/files](#post-v1filespath). |

Without `overwrite` or `if_match`, an existing file returns 409.

Response 201:

```json
{
  "upload_id": "b7d0f4...",
  "offset": 0,
  "part_bytes": 8388608,
  "expires_at": "2026-01-16T10:30:00Z"
}
```

Status codes: 201, 400, 401, 403, 409 (`FILE_ALREADY_EXISTS`, or the path is a directory), 412, 413, 500.

### PUT /v1/uploads/{id}

Sends one part. Required headers:

- `Content-Range: bytes START-END/TOTAL` where `TOTAL` equals the session `size` and `START` equals the current `offset`.
- `Content-Length` equal to `END - START + 1`, at most 8 MiB.

Body: the part bytes. Response 200: `{"offset": 8388608}` (the next `START`).

Status codes: 200, 400 (bad range or length), 404 (unknown or expired), 409 (`START` is not the current offset, or the upload is not open; `details.offset` holds the correct offset so you can resume), 413, 401.

### GET /v1/uploads/{id}

Shows progress.

```json
{
  "upload_id": "b7d0f4...",
  "path": "/big/archive.tar",
  "offset": 16777216,
  "size": 52428800,
  "state": "open",
  "expires_at": "2026-01-16T10:30:00Z"
}
```

`state` is `open`, `completing`, `done`, `failed` or `expired`. Status codes: 200, 401, 404.

### POST /v1/uploads/{id}/complete

Publishes the file once `offset` equals `size`. Response 201 when the file was created, 200 when it replaced one:

```json
{ "path": "/big/archive.tar", "etag": "3f2a9c...", "created": true }
```

Status codes: 200, 201, 401, 404, 409 (not all bytes received, or not open), 412.

### DELETE /v1/uploads/{id}

Cancels the session and discards the parts. Response 204. Status codes: 204, 401, 404.

```bash
curl -X POST -H "Authorization: Bearer your-api-key" \
  -d '{"path":"/big/archive.tar","size":52428800}' https://files.example.com/v1/uploads

curl -X PUT -H "Authorization: Bearer your-api-key" \
  -H "Content-Range: bytes 0-8388607/52428800" \
  --data-binary @part1 https://files.example.com/v1/uploads/b7d0f4

curl -X POST -H "Authorization: Bearer your-api-key" \
  https://files.example.com/v1/uploads/b7d0f4/complete
```

## Single-use links

A link lets someone without an account download one file once. The link works for 1 second to 24 hours. Each user can hold up to `server.rate_limit.max_active_links_per_user` active links (default 1000; 0 also means 1000).

### POST /v1/links/generate

JSON body (4 KiB max):

| Field | What it does |
|---|---|
| `path` | Required. A file you can read. Directories return 400. |
| `expiry_seconds` | 1 to 86400. |

Response 201:

```json
{
  "id": 42,
  "url": "https://files.example.com/download/token123",
  "token": "token123",
  "expires": "2026-01-15T11:30:00Z"
}
```

The URL is built from `server.external_url`. Rate limited per client address (`server.rate_limit.link_generate_rps`, `..._burst`).

Status codes: 201, 400, 401, 403, 404, 429 (rate limit, or too many active links), 500.

```bash
curl -X POST -H "Authorization: Bearer your-api-key" \
  -d '{"path":"/docs/report.pdf","expiry_seconds":3600}' \
  https://files.example.com/v1/links/generate
```

### GET /v1/links

Lists links. You see your own; admins see all.

| Query | Default | What it does |
|---|---|---|
| `status` | all | `active`, `used`, `expired` or `revoked`. |
| `path` | | Only links for this file. |
| `created_by` | | Admins only: filter by creator. |
| `limit` | 200 | 1 to 1000. |
| `cursor` | | From `next_cursor`. |

```json
{
  "items": [
    {
      "id": 42,
      "token_prefix": "token12",
      "file_path": "/docs/report.pdf",
      "status": "active",
      "created_by": "alice",
      "created_at": "2026-01-15T10:30:00Z",
      "expires_at": "2026-01-15T11:30:00Z",
      "used_at": null,
      "used_by_ip": null
    }
  ],
  "next_cursor": null,
  "counts": {"active": 1, "used": 0, "expired": 0, "revoked": 0}
}
```

`counts` covers all statuses for the same filters. Status codes: 200, 400, 401, 500.

### GET /v1/links/{id}

One link, with the same fields plus `updated_at` and `attempts` (recent download attempts, each `{"at", "result", "ip"}`). `result` is `success`, `not_found`, `expired` or `invalid`. Status codes: 200, 400, 401, 404.

### GET /v1/links/stats

Counts for the dashboard. Admins get counts for all links in the cluster. Other users get counts for their own links only.

| Query | Default | What it does |
|---|---|---|
| `window` | `24h` | `1h`, `24h` or `7d`. |

```json
{
  "window": "24h",
  "generated": 10,
  "consumed": {"success": 7, "not_found": 1, "expired": 1, "invalid": 0},
  "active": 3,
  "used_last_24h": 7,
  "last_cleanup": {"at": "2026-01-15T10:00:00Z", "expired_removed": 4, "used_removed": 2, "error": null}
}
```

`generated` and `consumed` cover the `window`. `active` is the number of links that can still be used and `used_last_24h` the links used in the last 24 hours. `last_cleanup` describes the latest run of the link cleanup (every 5 minutes, on one server of the cluster): when it ran (`at`, `null` before the first run), how many expired and used links it removed, and `error` (`null` when it worked).

Status codes: 200, 400, 401.

### DELETE /v1/links/{id}

Revokes an active link. Response `{"id": 42, "status": "revoked"}`. Status codes: 200, 400, 401, 404, 409 (link is not active).

### GET and POST /download/{token}

No authentication. The token is the secret. Rate limited per client address (`server.rate_limit.download_rps`, `..._burst`).

- `GET` returns a small HTML page with a "Download file" button. It does not use the link.
- `POST` downloads the file once and uses the link up. Headers: `Content-Type: application/octet-stream`, `Content-Disposition: attachment; filename*=UTF-8''report.pdf`.

Status codes: 200, 400, 404 (`LINK_NOT_FOUND`), 410 (`LINK_EXPIRED` or `LINK_INVALID` when already used or revoked), 429, 500.

```bash
curl -X POST -o report.pdf https://files.example.com/download/token123
```

## Account

### POST /v1/session

Starts a console session. No authentication. Body (4 KiB max):

```json
{ "api_key": "your-api-key" }
```

Response 200 with the `Set-Cookie: callfs_session=...; HttpOnly; SameSite=Strict` cookie (`Secure` over TLS, lifetime `ui.session_ttl`):

```json
{
  "user_id": "alice",
  "is_admin": false,
  "expires_at": "2026-01-15T18:30:00Z",
  "csrf_token": "c1f0...",
  "idle_timeout_seconds": 1800
}
```

`idle_timeout_seconds` is `ui.idle_timeout` in seconds; the console signs you out after that long without activity.

Rate limited per client address (`server.rate_limit.session_rps`, `..._burst`). Status codes: 200, 400, 401 (wrong key), 413, 429.

```bash
curl -c cookies.txt -d '{"api_key":"your-api-key"}' https://files.example.com/v1/session
```

### DELETE /v1/session

Ends the session: the cookie is cleared and the session is revoked on the server, so the same cookie no longer works on any server of the cluster. Needs authentication (with a session, send `X-CallFS-CSRF`). Response 204.

### GET /v1/me

Who you are.

```json
{
  "user_id": "alice",
  "is_admin": false,
  "auth_method": "api_key",
  "session_expires_at": null,
  "csrf_token": null,
  "permissions_model": "owner",
  "idle_timeout_seconds": 1800
}
```

With a session, `auth_method` is `session` and `session_expires_at` and `csrf_token` are filled. `idle_timeout_seconds` is `ui.idle_timeout` in seconds. Status codes: 200, 401.

### GET /v1/me/events

Recent events about your own actions (file created, link opened and so on).

| Query | Default | What it does |
|---|---|---|
| `since` | | RFC 3339 time; only newer events. |
| `limit` | 200 | 1 to 1000. |

```json
{
  "items": [
    {
      "id": 981,
      "at": "2026-01-15T10:30:00Z",
      "type": "file.created",
      "level": "info",
      "actor": "alice",
      "instance_id": "node-1",
      "message": "File created",
      "entities": [{"kind": "file", "id": "/docs/a.txt"}],
      "fields": {}
    }
  ]
}
```

Status codes: 200, 400, 401.

### GET /v1/permissions/check

Asks whether a user may read, write or delete a path.

| Query | What it does |
|---|---|
| `perm` | Required. `read`, `write` or `delete`. |
| `path` | Path to check. Default `/`. |
| `user` | User to check, 1 to 256 characters. Default is you. Other users: admin only (403 `ADMIN_REQUIRED`). |

```json
{
  "path": "/docs/report.pdf",
  "perm": "write",
  "user": "bob",
  "allowed": false,
  "reason": "not_owner",
  "owner": "alice",
  "type": "file"
}
```

`reason` is one of `admin`, `owner`, `open_directory`, `parent_open` (path is new and the parent allows creating), `not_found` and `not_owner`. Status codes: 200, 400, 401, 403.

```bash
curl -H "Authorization: Bearer your-api-key" \
  "https://files.example.com/v1/permissions/check?perm=write&path=/docs/report.pdf"
```

### GET /v1/capabilities

What this server supports and its limits.

```json
{
  "version": "2.0.0",
  "commit": "abc1234",
  "build_date": "2026-01-10",
  "go_version": "go1.24",
  "instance_id": "node-1",
  "node_id": "node-1",
  "features": {
    "erasure": true, "cluster": true, "nats": true, "cross_server": false,
    "websocket": true, "replication": false, "quic": false,
    "metrics_listener": false, "trash": true,
    "resumable_uploads": true, "search": true, "ui": true
  },
  "storages": ["localfs", "s3"],
  "default_storage": "localfs",
  "file_index": "nats",
  "lock_type": "nats",
  "external_url": "https://files.example.com",
  "limits": {
    "max_upload_bytes": 10737418240,
    "max_split_upload_bytes": 1073741824,
    "max_ws_upload_bytes": 100000000,
    "ws_frame_bytes": 1048576,
    "max_patch_body_bytes": 1048576,
    "link_expiry_min_s": 1,
    "link_expiry_max_s": 86400,
    "list_page_max": 1000,
    "upload_part_bytes": 8388608
  },
  "erasure_defaults": {"data_shards": 4, "parity_shards": 2, "max_total_shards": 64, "min_file_size": 104857600},
  "trash_retention_s": 2592000
}
```

`node_id` is `null` when no node name is set. `features.cluster` and `features.nats` are both `true` when `nats.enabled` is on. `erasure_defaults.min_file_size` is `erasure.min_file_size` (0 means files are never split automatically). Status codes: 200, 401.

### GET /v1/usage

Storage totals under a directory.

| Query | Default | What it does |
|---|---|---|
| `path` | `/` | Directory to measure. |
| `depth` | 0 | 0 to 3. Adds per-subdirectory totals in `children`. |
| `owner` | | Count only this owner's files. |
| `histograms` | `false` | `true` adds `size_histogram` and `age_histogram`. |

```json
{
  "path": "/",
  "files": 1200,
  "directories": 80,
  "bytes": 52428800,
  "by_storage": {"localfs": {"files": 1200, "bytes": 52428800}},
  "by_instance": {"node-1": {"files": 1200, "bytes": 52428800}},
  "by_owner": {"alice": {"files": 1200, "bytes": 52428800}},
  "erasure_files": 0,
  "erasure_raw_bytes": 0,
  "children": [{"path": "/docs", "files": 300, "bytes": 1048576}],
  "children_truncated": false,
  "size_histogram": [{"le": 1024, "count": 10}, {"le": null, "count": 2}],
  "age_histogram": [{"le_days": 1, "count": 4}, {"le_days": null, "count": 50}],
  "computed_at": "2026-01-15T10:30:00Z",
  "scan_truncated": false
}
```

`by_instance` counts only files stored on a server's own disk (erasure-coded files are counted under `erasure_files`). Results are cached for one minute. Trees of more than 200000 entries are measured in the background: the response is 202 with `{"job_id": "...", "result": {...}}`, where `result` appears only if an earlier result is cached. Fetch the final numbers from [`/v1/jobs/{id}`](#jobs) (its `result`) or repeat the request after the job ends.

Status codes: 200, 202, 400, 401, 404, 500.

## Jobs

Background work (large batches, trash emptying, usage scans, erasure repair) runs as jobs. You see your own jobs; admins see all. Job routes exist whenever the batch route does.

Job states: `queued`, `running`, `cancelling`, `succeeded`, `partial`, `failed`, `cancelled`.

### GET /v1/jobs

| Query | Default | What it does |
|---|---|---|
| `state` | all | Comma-separated states to include, for example `running,queued`. An unknown state returns 400. |
| `limit` | 50 | 1 to 200. |
| `cursor` | | From `next_cursor`. |

Response: `{"items": [job, ...], "next_cursor": null}`.

### GET /v1/jobs/{id}

```json
{
  "id": "j-4c1e",
  "kind": "batch",
  "node": "node-1",
  "state": "partial",
  "created_by": "alice",
  "created_at": "2026-01-15T10:30:00Z",
  "started_at": "2026-01-15T10:30:01Z",
  "finished_at": "2026-01-15T10:30:09Z",
  "progress": {"done": 500, "total": 500, "failed": 2},
  "failed_items": [{"path": "/docs/b.txt", "code": "FILE_NOT_FOUND", "reason": "file not found"}],
  "result": {"processed": 500, "failed": 2},
  "error": null
}
```

`kind` is `batch`, `trash_empty`, `usage_scan` or `erasure_repair`. `node` is the server that runs the job. A retry job also has `retry_of`.

### POST /v1/jobs/{id}/cancel

Asks a running job to stop. Response: the job. Status 409 if the job already ended.

### POST /v1/jobs/{id}/retry-failed

Starts a new job that repeats only the failed items. Response 202: the new job. Status 409 if there is nothing to retry or the job is still running.

Status codes for all job routes: 200, 202, 400 (bad `state` or `limit`), 401, 404 (unknown, or not yours), 409.

```bash
curl -H "Authorization: Bearer your-api-key" "https://files.example.com/v1/jobs?state=running,queued"
curl -X POST -H "Authorization: Bearer your-api-key" https://files.example.com/v1/jobs/j-4c1e/cancel
```

## Erasure-coded files

These routes exist when erasure coding is enabled; otherwise they return 404 `FEATURE_DISABLED`. All paths are file paths. See [05-storage.md](05-storage.md) for what erasure coding is.

### GET /v1/erasure/files

| Query | Default | What it does |
|---|---|---|
| `prefix` | | Path prefix, up to 4096 bytes. |
| `health` | | `ok`, `reduced`, `critical` or `unreadable`. |
| `limit` | 200 | 1 to 1000. |
| `cursor` | | From `next_cursor`. |

```json
{
  "items": [
    {
      "path": "/archive/big.bin",
      "original_size": 104857600,
      "data_shards": 4,
      "parity_shards": 2,
      "shard_size": 26214400,
      "shards_total": 6,
      "healthy_shards": 6,
      "last_checked_at": "2026-01-15T10:30:00Z",
      "health": "ok"
    }
  ],
  "next_cursor": null
}
```

`health` is `unknown` and `healthy_shards` and `last_checked_at` are `null` until the file has been checked. Only files you can read are listed.

### GET /v1/erasure/files/{path}

Details of one file with a fresh check of every shard.

```json
{
  "path": "/archive/big.bin",
  "original_size": 104857600,
  "profile": {"data_shards": 4, "parity_shards": 2, "shard_size": 26214400},
  "shards": [
    {"index": 0, "kind": "data", "instance_id": "node-1", "storage": "localfs", "path": "...", "size": 26214400, "checksum": "ab12...", "state": "ok"}
  ],
  "tolerance": 2,
  "last_checked_at": "2026-01-15T10:30:00Z"
}
```

Shard `state` is `ok`, `missing`, `corrupt`, `unreachable` or `unknown`. `tolerance` is how many shards can be lost.

### GET /v1/erasure/manifest/{path}

Shard locations for parallel download.

```json
{
  "path": "/archive/big.bin",
  "original_size": 104857600,
  "erasure_profile": {"data_shards": 4, "parity_shards": 2},
  "shards": [{"index": 0, "endpoint": "https://node-1.example.com", "size": 26214400, "checksum": "ab12..."}]
}
```

### POST /v1/erasure/verify/{path}

Checks every shard now.

```json
{
  "path": "/archive/big.bin",
  "readable": true,
  "healthy": 6,
  "required": 4,
  "shards": [{"index": 0, "state": "ok", "error": null}]
}
```

### POST /v1/erasure/repair/{path}

Rebuilds damaged shards in the background. Needs write access. Response 202: `{"job_id": "..."}`. The finished job `result` is `{"repaired": [2], "failed": []}`.

### GET /v1/shards/{path}/{index}

Downloads one raw shard (`application/octet-stream`). Needs read access to the file. Status codes: 200, 400, 401, 403, 404 (`SHARD_NOT_FOUND`), 500.

Status codes for the other erasure routes: 200, 202, 400 (bad path, `health`, `limit` or `cursor`), 401, 403, 404, 500.

```bash
curl -X POST -H "Authorization: Bearer your-api-key" \
  https://files.example.com/v1/erasure/verify/archive/big.bin
```

## WebSocket transfer

### GET /v1/files/ws/{path}

Streams one file over a websocket. Connect with the usual upgrade handshake and your `Authorization` header (or the session cookie). Browsers must connect from the same host as the server.

| Query | Default | What it does |
|---|---|---|
| `mode` | `download` | `download` or `upload`. |
| `size` | | Upload only. The exact byte count, up to 100 MiB. If given, data streams straight to storage. |

Rules:

- The path must be a file path. Directories return 400.
- A message is at most 1 MiB. Data travels in binary messages; other frames are ignored (too many close the socket).
- A user can run 4 uploads at once, the server 64. More returns 503. With `nats.counters` on, these limits count across the whole cluster.
- Without `size`, uploads are limited to 100 MiB total. With `size`, the received bytes must match exactly.
- An upload replaces an existing file or creates a new one.

**Download:** the server sends the text message `size:BYTES`, then binary messages with the content, text messages `progress:BYTES` about every 1 MiB, then closes with code 1000 "download complete".

**Upload:** send binary messages, then close the socket with code 1000. The server sends `ack:BYTES` about every 8 MiB and finally `ok:BYTES:ETAG`, then closes with code 1000 "upload complete".

| Close code | Meaning |
|---|---|
| 1000 | Done. |
| 1003 | Path is not a file, or invalid `size`. |
| 1008 | No access, file not found, or too many non-binary frames. |
| 1009 | Upload too large. |
| 1011 | Server error. |
| 4409 | Upload target already exists. |

Before the upgrade, errors are normal JSON responses: 400, 401, 503.

```bash
websocat -H "Authorization: Bearer your-api-key" \
  "wss://files.example.com/v1/files/ws/docs/report.pdf?mode=download" > report.pdf
```

## Health and metrics

### GET /health

No authentication. Liveness check.

```json
{ "status": "ok" }
```

Always 200 while the process runs.

### GET /health/ready

No authentication. Readiness check, based on the health checks in [06-monitoring-metrics.md](06-monitoring-metrics.md). The result is cached for 2 seconds.

| Status | `status` | Meaning |
|---|---|---|
| 200 | `ok` | All checks pass. |
| 200 | `degraded` | At least one warning. The server still works. |
| 503 | `down` | A check is failing. Take the server out of rotation. |

Readiness reflects this server only. Problems with other servers are listed under `peers` and never change the status or the code:

```json
{ "status": "ok", "peers": [ { "name": "storage.internal_proxy", "status": "warn", "detail": "Not reachable" } ] }
```

`peers` is left out when there is nothing to report. Entry `status` is `ok`, `warn`, `down` or `disabled`.

```bash
curl -i https://files.example.com/health/ready
```

### GET /metrics

Prometheus metrics. Needs authentication (any valid key). Metrics are described in [06-monitoring-metrics.md](06-monitoring-metrics.md).

## Rate limits

| Route | Setting | Default (requests per second / burst) |
|---|---|---|
| `POST /v1/session` | `server.rate_limit.session_rps`, `session_burst` | 1 / 5 |
| `POST /v1/links/generate` | `server.rate_limit.link_generate_rps`, `link_generate_burst` | 100 / 100 |
| `/download/{token}` | `server.rate_limit.download_rps`, `download_burst` | 10 / 5 |

Limits apply per client address (IPv6 clients are grouped by /64). With `nats.counters` on, the allowance is shared by all servers in the cluster. Rejected requests get 429, `RATE_LIMIT_EXCEEDED` and `Retry-After`. See [02-configuration.md](02-configuration.md).
