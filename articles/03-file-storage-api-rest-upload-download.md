# File Storage API: Upload, Download, and Manage Files with CallFS

CallFS is a REST API filesystem that maps every standard file operation to a standard HTTP method. You get a file storage API with no SDK to install, no proprietary client library to learn, and no vendor-specific upload protocol to reverse-engineer. If you can run `curl`, you can store and retrieve files.

This tutorial walks through the operations the CallFS file storage API supports, with request examples, integration code in Python and JavaScript, and a breakdown of error handling. Everything you can do over the API you can also do in the web console at `/ui/`, which is noted where it helps.

---

## API Overview

The file storage API lives under the `/v1/files/` prefix. Every path beneath that prefix represents a file or directory in the filesystem. The HTTP method determines what happens to that resource.

| HTTP Method | Operation | Returns |
|-------------|-----------|---------|
| `POST` | Create a file or directory (fails if it exists) | `201 Created` |
| `GET` | Download a file (supports ranges) or list a directory | File bytes or JSON |
| `PUT` | Create or replace a file | `200 OK` or `201 Created` |
| `PATCH` | Rename or move; change mode or owner | `200 OK` with JSON |
| `DELETE` | Delete a file or directory | `204 No Content` |
| `HEAD` | Read metadata, no body transfer | Headers only |

All requests need a `Bearer` token in the `Authorization` header (or a console session). The server listens on port 8443 by default; the examples use `https://files.example.com`. For a local test server running with `server.protocol: http`, use `http://localhost:8443`.

The examples assume your key is in an environment variable:

```bash
export CALLFS_API_KEY=your-api-key
```

Paths are absolute from the root of the CallFS filesystem. Names with `..` segments or other path tricks are rejected with `400`. A directory path ends in `/`; a file path does not.

Who may do what is decided by ownership and the file's mode: the owner and administrators can do everything, other users follow the "other" read and write bits of the mode (`0644` files can be read by others, `0777` folders let others list and create), and only the owner can delete. See [Secure file sharing](02-secure-file-sharing-business.md).

---

## Operations

### Create a Directory

Directories are created by sending a `POST` to a path that ends with a trailing slash. No request body is required; the trailing slash is the signal.

```bash
curl -X POST \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  https://files.example.com/v1/files/uploads/
```

A successful response returns `HTTP 201 Created` with no body. If the directory already exists, CallFS returns `HTTP 200 OK` rather than an error, so the call is safe to repeat in setup scripts. If a file with that name exists, you get `409 Conflict`.

### Upload a File

Upload a file by sending a `POST` with the file bytes as the request body. Set `Content-Type: application/octet-stream` to indicate raw binary content.

```bash
curl -i -X POST \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @photo.jpg \
  https://files.example.com/v1/files/uploads/photo.jpg
```

The file goes to the configured default storage (`backend.default_backend`, local disk unless you changed it). When erasure coding is on and the body is at least `erasure.min_file_size`, it is split automatically; send `X-CallFS-Erasure: false` to opt out, or `X-CallFS-Erasure: true` to split a smaller file (see the erasure coding article). CallFS returns `HTTP 201 Created` and the new `ETag` header on success. If the file already exists, the API returns `HTTP 409 Conflict` with a message directing you to use `PUT` to update it.

The largest file you can send in one request is 10 GiB; a larger body returns `413 Payload Too Large`. Both standard `Content-Length` uploads and chunked transfer encoding are supported; CallFS stores the number of bytes it actually received.

To make a create fail rather than overwrite when another client may have created the same file in the meantime, add `If-None-Match: *`. If something already exists at that path, the answer is `412 Precondition Failed`.

By default CallFS answers only after the data is on disk. You can set the answer point for a single request with the `X-CallFS-Write-Ack` header: `disk` waits for the disk, `memory` answers sooner and can lose the file if the machine loses power right after. Any other value returns `400`. The server-wide default is `backend.localfs_ack`. See [Storage](../docs_markdown/05-storage.md).

### Download a File

Retrieve a file with a `GET` request. The response body is the raw file bytes, streamed from the storage backend.

```bash
curl -H "Authorization: Bearer $CALLFS_API_KEY" \
  https://files.example.com/v1/files/uploads/photo.jpg \
  -o photo.jpg
```

The response includes these headers alongside the file content:

| Header | Description |
|--------|-------------|
| `Content-Type` | Always `application/octet-stream` |
| `Content-Length` | Bytes in this response |
| `ETag` | Content tag, a quoted hex string |
| `Accept-Ranges` | `bytes` |
| `X-CallFS-Type` | `file` |
| `X-CallFS-Size` | Full file size in bytes |
| `X-CallFS-Mode` | File permission mode (for example `0644`) |
| `X-CallFS-Owner` | The user name of the owner |
| `X-CallFS-MTime` | Last modified time in RFC 3339 format |

Downloads support conditional and partial requests:

- **Ranges**: send `Range: bytes=0-1023` (or `bytes=1024-`, or `bytes=-512` for the last 512 bytes) and the answer is `206 Partial Content` with a `Content-Range` header. A start beyond the end of the file returns `416 Range Not Satisfiable`. Only one range per request is served; a request with several ranges receives the whole file.
- **Caching**: send `If-None-Match: "<etag>"` and an unchanged file answers `304 Not Modified` with no body.
- **Resuming**: send `If-Range: "<etag>"` with a `Range` to continue a download only if the file has not changed; otherwise you get the full file.

A GET on a directory path returns a JSON array of its entries; the `/v1/directories/` endpoint below is the richer way to list.

### Update a File

To replace the content of an existing file, use `PUT`. The semantics differ from `POST`: `PUT` creates the file if it does not exist, and overwrites it if it does.

```bash
curl -i -X PUT \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @photo-v2.jpg \
  https://files.example.com/v1/files/uploads/photo.jpg
```

Response is `HTTP 200 OK` when an existing file is updated, or `HTTP 201 Created` when the file is new, each with the new `ETag`. `PUT` does not accept paths with a trailing slash; directory creation is exclusively a `POST` operation. Replacing a directory with file content returns `400`.

Use the ETag for safe concurrent edits: `If-Match: "<etag>"` makes the update apply only if the file is still the version you read, and fails with `412 Precondition Failed` otherwise. Files that were stored with erasure coding cannot be overwritten in place; replace them by deleting and creating again. `PUT` with `X-CallFS-Erasure: true` returns `501` when erasure coding is off.

### Rename or Move

`PATCH` renames or moves a file, or a directory with everything in it:

```bash
curl -X PATCH \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name": "photo-final.jpg"}' \
  https://files.example.com/v1/files/uploads/photo.jpg
```

```bash
curl -X PATCH \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"destination": "/archive/2026/photo.jpg", "create_parents": true}' \
  https://files.example.com/v1/files/uploads/photo-final.jpg
```

| Field | Meaning |
|-------|---------|
| `name` | New name within the same folder. |
| `destination` | New absolute path. Send exactly one of `name` or `destination`. |
| `overwrite` | `true` replaces an existing destination (also accepted as `?overwrite=true`). Default `false`. |
| `create_parents` | `true` creates missing folders in the destination. Default `false`. |
| `backend`, `instance` | Move a file to another storage or server. Administrators can; other users get `403`. |

The answer is `200 OK` with `{"path": "<new path>"}`. A destination that exists without `overwrite` returns `409`; a missing source returns `404`.

`PATCH` can also change a file's mode or its owner. The mode controls what other users may do (read bit 4 and write bit 2 of the last digit). Send `{"mode": "0640"}` or (administrators only) `{"owner": "bob"}`, without `name` or `destination`; the answer is `{"path","mode","owner"}`.

### Delete a File or Directory

```bash
curl -X DELETE \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  https://files.example.com/v1/files/uploads/photo.jpg
```

Returns `HTTP 204 No Content` on success, and the file is removed for good. The same endpoint handles files and directories. An empty directory is deleted directly; a directory with content returns `409 Conflict` unless you add `?recursive=true`, which removes everything under it and answers `200` with `{"deleted","deleted_files","deleted_directories"}`. Add `&dry_run=true` to see what a delete would remove without removing anything.

For a delete you can undo, use "Move to trash" in the console or a `trash` operation in a batch request (below). Trash is on by default (`trash.enabled`) and keeps items for 30 days (`trash.retention`).

### Read Metadata Without Downloading

The `HEAD` method returns all metadata headers with no response body. This is the efficient way to check file existence, size, or last-modified time before deciding whether to download.

```bash
curl -I \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  https://files.example.com/v1/files/uploads/photo.jpg
```

Response headers:

| Header | Description |
|--------|-------------|
| `X-CallFS-Type` | `file` or `directory` |
| `X-CallFS-Size` | Size in bytes |
| `X-CallFS-Mode` | Permission mode |
| `X-CallFS-Owner` | Owning user name |
| `X-CallFS-MTime` | Last modified timestamp |
| `X-CallFS-Instance-ID` | The server that holds the file |
| `ETag` | Content tag |

`GET /v1/stat/<path>` returns the same facts, plus lock and storage details, as JSON.

### List a Directory

Send a `GET` to `/v1/directories/` to get a JSON document describing the directory and its immediate contents.

```bash
curl -H "Authorization: Bearer $CALLFS_API_KEY" \
  https://files.example.com/v1/directories/uploads
```

Response:

```json
{
  "path": "/uploads",
  "type": "directory",
  "recursive": false,
  "count": 1,
  "items": [
    {
      "name": "photo.jpg",
      "path": "/uploads/photo.jpg",
      "type": "file",
      "size": 204800,
      "mode": "0644",
      "owner": "alice",
      "mtime": "2026-04-11T10:00:00Z"
    }
  ]
}
```

A listing returns at most 10,000 entries; if there are more, the response has `"truncated": true`. A path that is a file returns `400`, and a missing path returns `404`.

### Recursive Directory Listing

Add `?recursive=true` to traverse all subdirectories in a single request. An optional `max_depth` parameter limits traversal depth (default 100, maximum 1000). Both also work together with the paged listing below.

```bash
curl -H "Authorization: Bearer $CALLFS_API_KEY" \
  "https://files.example.com/v1/directories/uploads?recursive=true&max_depth=5"
```

The response shape is identical to a flat listing, with all descendant items in the `items` array, `"recursive": true` and `"max_depth"` in the envelope.

### Page, Sort and Filter a Listing

When you add `limit`, the listing becomes a page you can sort and filter, and each item gains `etag`, `storage`, `instance_id`, `erasure_coded` and `locked` fields:

```bash
curl -H "Authorization: Bearer $CALLFS_API_KEY" \
  "https://files.example.com/v1/directories/uploads?limit=100&sort=mtime&order=desc&type=file&q=photo"
```

| Parameter | Values |
|-----------|--------|
| `limit` | 1 to 1000 |
| `cursor` | The `next_cursor` of the previous page |
| `sort` | `name` (default; folders first), `size`, `mtime`, `type` |
| `order` | `asc` (default) or `desc` |
| `type` | `file` or `directory` |
| `q` | Name contains this text (case-insensitive) |
| `include_hidden` | `false` hides names starting with a dot |

The response has `count`, `total`, `items`, `next_cursor` (null on the last page) and `partial`, which is `true` when the folder holds more than 10,000 entries and only the first 10,000 were considered.

### Search

`GET /v1/search` finds files and folders by metadata anywhere under a path:

```bash
curl -H "Authorization: Bearer $CALLFS_API_KEY" \
  "https://files.example.com/v1/search?path=/uploads&q=*.jpg&min_size=1048576&owner=alice&limit=50"
```

Parameters: `path` (scope), `q` (text, or a glob such as `*.jpg`), `type`, `min_size`, `max_size`, `modified_after`, `modified_before` (RFC 3339), `owner`, `storage`, `instance`, `erasure` (`true` or `false`), `locked` (`true` or `false`, files that are locked now or not), `limit` (default 100, maximum 1000), `cursor`, and `facets=true` for counts by type, storage and owner. A search in a place you may not read returns `403`. The console exposes the same search at Files, "Search".

### List by Prefix and Delete by Prefix

For object-store style access, `GET /v1/objects?prefix=/uploads/&limit=1000&start_after=<path>` lists files in path order and returns `objects`, `truncated` and `next_start_after`. Add `count_only=true` for a count and byte total. `DELETE /v1/objects?prefix=/tmp/` removes every file under a prefix and returns `{"deleted": N}`; add `dry_run=true` to preview. A prefix of `/` is refused.

### Batch Operations

`POST /v1/batch` runs many operations in one request: `delete`, `trash`, `move`, `rename`, `mode` and `restore`.

```bash
curl -X POST \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"ops": [
        {"op": "trash", "path": "/uploads/old.jpg"},
        {"op": "move", "path": "/uploads/a.jpg", "destination": "/archive/a.jpg", "create_parents": true}
      ], "mode": "sync", "stop_on_error": false}' \
  https://files.example.com/v1/batch
```

A `sync` batch (the default) holds up to 100 operations and answers `{"results": [{"index", "status", "code", "message", "path"}]}`, one entry per operation, with `status` `ok` or `failed`. Use `"mode": "job"` for up to 10,000 operations; the answer is `202 Accepted` with `{"job_id": "..."}`, and `GET /v1/jobs/<job_id>` reports progress and failures. `GET /v1/jobs` lists jobs and accepts a `state` filter. The request body may be at most 4 MiB.

### Resumable Uploads

For large files or unreliable networks, upload in parts that you can resume. A session lasts 24 hours.

1. Start a session. `size` is the total byte count:

```bash
curl -X POST \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"path": "/uploads/big.bin", "size": 20971520}' \
  https://files.example.com/v1/uploads
```

```json
{"upload_id": "3f9a0c1d5b7e4a2c8d6f1e0b9a8c7d6e", "offset": 0, "part_bytes": 8388608, "expires_at": "2026-04-12T10:00:00Z"}
```

2. Send parts in order. Each part is at most `part_bytes` (8 MiB), must start at the current offset, and needs a matching `Content-Range` and `Content-Length`:

```bash
head -c 8388608 big.bin | curl -X PUT \
  -H "Authorization: Bearer $CALLFS_API_KEY" \
  -H "Content-Range: bytes 0-8388607/20971520" \
  --data-binary @- \
  https://files.example.com/v1/uploads/3f9a0c1d5b7e4a2c8d6f1e0b9a8c7d6e
```

The answer is `{"offset": 8388608}`. A part that does not start at the current offset returns `409` with `details.offset` telling you where to continue.

3. After a break, `GET /v1/uploads/<id>` returns `upload_id`, `path`, `offset`, `size`, `state` (`open`, `completing`, `done`, `failed` or `expired`) and `expires_at`.
4. When the offset equals the size, finish with `POST /v1/uploads/<id>/complete`. The answer is `201` (new file) or `200` (replaced) with `{"path","etag","created"}`. An incomplete upload returns `409`.
5. Abort with `DELETE /v1/uploads/<id>` (`204`).

The session body also takes `overwrite` (default `false`; without it an existing file returns `409`), `if_match` (an ETag), `mode` (default `0644`), `storage` (`localfs` or `s3`, to pick where this file goes) and `erasure` (shard options, see the erasure coding article). The maximum size is 10 GiB. The console uses these uploads for files and folders and shows them under Files, "Transfers".

### Streaming Over WebSocket

`GET /v1/files/ws/<path>?mode=download` and `?mode=upload` transfer a file as binary WebSocket messages, using the same bearer token during the handshake. Use `wss://` on HTTPS.

---

## Python Integration

The `requests` library maps cleanly to CallFS operations.

```python
import requests

BASE_URL = "https://files.example.com/v1"
API_KEY = "your-api-key"
HEADERS = {"Authorization": f"Bearer {API_KEY}"}


class CallFSError(Exception):
    def __init__(self, status: int, code: str, message: str):
        super().__init__(f"{status} {code}: {message}")
        self.status = status
        self.code = code


def check(response: requests.Response) -> requests.Response:
    if response.ok:
        return response
    try:
        body = response.json()
        raise CallFSError(response.status_code, body.get("code", ""), body.get("message", ""))
    except ValueError:
        raise CallFSError(response.status_code, "", response.text)


def create_directory(path: str) -> None:
    check(requests.post(f"{BASE_URL}/files{path}/", headers=HEADERS))


def upload_file(remote_path: str, local_path: str) -> str:
    with open(local_path, "rb") as f:
        response = check(requests.put(
            f"{BASE_URL}/files{remote_path}",
            headers={**HEADERS, "Content-Type": "application/octet-stream"},
            data=f,
        ))
    return response.headers.get("ETag", "")


def download_file(remote_path: str, local_path: str) -> None:
    response = check(requests.get(f"{BASE_URL}/files{remote_path}", headers=HEADERS, stream=True))
    with open(local_path, "wb") as f:
        for chunk in response.iter_content(chunk_size=65536):
            f.write(chunk)


def update_file_if_unchanged(remote_path: str, local_path: str, etag: str) -> str:
    with open(local_path, "rb") as f:
        response = check(requests.put(
            f"{BASE_URL}/files{remote_path}",
            headers={**HEADERS, "Content-Type": "application/octet-stream", "If-Match": etag},
            data=f,
        ))
    return response.headers.get("ETag", "")


def delete_file(remote_path: str) -> None:
    check(requests.delete(f"{BASE_URL}/files{remote_path}", headers=HEADERS))


def get_metadata(remote_path: str) -> dict:
    response = check(requests.head(f"{BASE_URL}/files{remote_path}", headers=HEADERS))
    return {
        "type": response.headers.get("X-CallFS-Type"),
        "size": int(response.headers.get("X-CallFS-Size", 0)),
        "mode": response.headers.get("X-CallFS-Mode"),
        "owner": response.headers.get("X-CallFS-Owner"),
        "mtime": response.headers.get("X-CallFS-MTime"),
        "etag": response.headers.get("ETag"),
    }


def list_directory(path: str, recursive: bool = False) -> dict:
    params = {"recursive": "true"} if recursive else {}
    return check(requests.get(f"{BASE_URL}/directories{path}", headers=HEADERS, params=params)).json()


create_directory("/uploads")
etag = upload_file("/uploads/photo.jpg", "photo.jpg")
meta = get_metadata("/uploads/photo.jpg")
print(f"Stored {meta['size']} bytes, mode {meta['mode']}, owner {meta['owner']}")
download_file("/uploads/photo.jpg", "photo-downloaded.jpg")
etag = update_file_if_unchanged("/uploads/photo.jpg", "photo-v2.jpg", etag)
delete_file("/uploads/photo.jpg")
```

For large uploads, passing the file object directly to `data=` streams it without loading the entire file into memory.

---

## Node.js / JavaScript Integration

The native `fetch` API handles all CallFS operations without additional dependencies on Node.js 18 and later.

```javascript
import fs from "node:fs";

const BASE_URL = "https://files.example.com/v1";
const API_KEY = "your-api-key";
const AUTH_HEADER = { Authorization: `Bearer ${API_KEY}` };

async function fail(res) {
  const err = await res.json().catch(() => ({ code: "", message: res.statusText }));
  throw new Error(`${res.status} ${err.code}: ${err.message}`);
}

async function createDirectory(path) {
  const res = await fetch(`${BASE_URL}/files${path}/`, {
    method: "POST",
    headers: AUTH_HEADER,
  });
  if (!res.ok) await fail(res);
}

async function uploadFile(remotePath, fileBuffer) {
  const res = await fetch(`${BASE_URL}/files${remotePath}`, {
    method: "PUT",
    headers: { ...AUTH_HEADER, "Content-Type": "application/octet-stream" },
    body: fileBuffer,
  });
  if (!res.ok) await fail(res);
  return res.headers.get("ETag");
}

async function downloadFile(remotePath) {
  const res = await fetch(`${BASE_URL}/files${remotePath}`, { headers: AUTH_HEADER });
  if (!res.ok) await fail(res);
  return res.arrayBuffer();
}

async function deleteFile(remotePath) {
  const res = await fetch(`${BASE_URL}/files${remotePath}`, {
    method: "DELETE",
    headers: AUTH_HEADER,
  });
  if (!res.ok) await fail(res);
}

async function getMetadata(remotePath) {
  const res = await fetch(`${BASE_URL}/files${remotePath}`, {
    method: "HEAD",
    headers: AUTH_HEADER,
  });
  if (!res.ok) throw new Error(`HEAD request failed: ${res.status}`);
  return {
    type: res.headers.get("X-CallFS-Type"),
    size: parseInt(res.headers.get("X-CallFS-Size") ?? "0", 10),
    mode: res.headers.get("X-CallFS-Mode"),
    owner: res.headers.get("X-CallFS-Owner"),
    mtime: res.headers.get("X-CallFS-MTime"),
    etag: res.headers.get("ETag"),
  };
}

async function listDirectory(path, recursive = false) {
  const url = new URL(`${BASE_URL}/directories${path}`);
  if (recursive) url.searchParams.set("recursive", "true");
  const res = await fetch(url, { headers: AUTH_HEADER });
  if (!res.ok) await fail(res);
  return res.json();
}

await createDirectory("/uploads");
const buffer = await fs.promises.readFile("photo.jpg");
await uploadFile("/uploads/photo.jpg", buffer);
const meta = await getMetadata("/uploads/photo.jpg");
console.log(`Stored ${meta.size} bytes`);
const data = await downloadFile("/uploads/photo.jpg");
await fs.promises.writeFile("photo-downloaded.jpg", Buffer.from(data));
await deleteFile("/uploads/photo.jpg");
```

In a browser, pass a `File` or `Blob` object directly as the `body` for uploads. Cookie sessions are for the console; scripts should send the bearer key.

---

## Error Handling

Every error response from the file storage API uses the same JSON structure:

```json
{
  "code": "FILE_NOT_FOUND",
  "message": "file not found"
}
```

The `code` field is machine-readable and stable across versions. The `message` field is human-readable and meant for logging. Every response carries an `X-Request-ID` header; quote it when you look something up in the logs.

| Code | HTTP Status | When it occurs |
|------|-------------|----------------|
| `BAD_REQUEST` | 400 | Invalid path, JSON, parameter or header value |
| `AUTHENTICATION_FAILED` | 401 | Missing or invalid key or session (including a wrong key on `POST /v1/session`) |
| `PERMISSION_DENIED` | 403 | You are not allowed to change or delete that path |
| `FILE_NOT_FOUND` | 404 | Path does not exist, or an erasure request for a file that has no erasure record |
| `CONFLICT` | 409 | `POST` to a path that already exists, non-empty folder delete, destination exists |
| `FILE_ALREADY_EXISTS` | 409 | A target already exists (move, resumable upload) |
| `RESOURCE_LOCKED` | 409 | Another operation holds the path; retry shortly |
| `PRECONDITION_FAILED` | 412 | `If-Match` or `If-None-Match` did not hold |
| `PAYLOAD_TOO_LARGE` | 413 | Body over the limit (10 GiB per upload) |
| `RANGE_NOT_SATISFIABLE` | 416 | `Range` starts past the end of the file |
| `RATE_LIMIT_EXCEEDED` | 429 | Too many requests or failed sign-ins from one address |
| `INTERNAL_ERROR` | 500 | Storage or server-side failure |
| `BAD_GATEWAY` | 502 | The server that holds the file did not answer |
| `INSUFFICIENT_STORAGE` | 507 | The disk is full |

For `409 Conflict` when the file lives on another cluster node, the body has `error`, `existing_path`, `instance_id`, `backend_type` and `suggestion`, so you can see which server holds the file. The `HEAD` response header `X-CallFS-Instance-ID` shows the same.

When building retry logic, treat `400`, `401`, `403` and `412` as permanent failures (fix the request, credentials or version first), treat `404` as permanent for downloads, treat `409 RESOURCE_LOCKED`, `429`, `502` and `5xx` as transient and retry with exponential backoff, and honor the `Retry-After` header on `429`.

---

## The Same Tasks in the Web Console

Open `https://files.example.com/ui/` and sign in with an API key. On the Files page you can create folders, upload files or whole folders (also by dragging them onto Browse), download one file or several, rename, move (Cut with Ctrl/Cmd+X and Paste with Ctrl/Cmd+V, or drag), filter by name, switch between list and grid, open a right-click or long-press menu on any row, edit the mode (including symbolic input like `u=rw,go=r`), delete for good, and see owner and storage for each item. Admins also see a backup status column and can move a file to another server. "Search" runs the metadata search, "Transfers" shows uploads in progress (pick what to do on a name conflict, resume interrupted resumable uploads after a reload, and watch the "Verifying" step that compares the SHA-256 of the upload with the server's ETag for files up to 64 MiB), and "Trash" lists deleted items you can restore, choosing Replace, Keep both, Replace if changed or Skip when the original name is taken. Select a file and choose "Create download link..." to share it, or "Download range..." to save part of it. The page [Web console](../docs_markdown/08-web-console.md) describes every screen.

---

## Next Steps

- Read the [Authentication and Security guide](../docs_markdown/04-authentication-security.md) to understand how API keys, users and sessions work.
- See the [API reference](../docs_markdown/03-api-reference.md) for every endpoint and parameter.
- See the [Storage guide](../docs_markdown/05-storage.md) to understand where files are physically stored and how writes are made safe.
- Explore the [Clustering guide](../docs_markdown/07-clustering.md) if you are running more than one CallFS server.
