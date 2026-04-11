# File Storage API: Upload, Download, and Manage Files with CallFS

CallFS is a REST API filesystem that maps every standard file operation to a standard HTTP method. You get a file storage API with no SDK to install, no proprietary client library to learn, and no vendor-specific upload protocol to reverse-engineer. If you can run `curl`, you can store and retrieve files.

This tutorial walks through every operation the CallFS file storage API supports, with verified request examples, integration code in Python and JavaScript, and a breakdown of error handling.

---

## API Overview

The file storage API lives under the `/v1/files/` prefix. Every path beneath that prefix represents a file or directory in the filesystem. The HTTP method determines what happens to that resource.

| HTTP Method | Operation | Returns |
|-------------|-----------|---------|
| `POST` | Create file or directory | `201 Created` |
| `GET` | Download file or list directory | File bytes or JSON |
| `PUT` | Update (or create) a file | `200 OK` or `201 Created` |
| `DELETE` | Delete file or directory | `204 No Content` |
| `HEAD` | Read metadata, no body transfer | Headers only |

All requests require a `Bearer` token in the `Authorization` header. The server runs on `localhost:8443` by default.

---

## Operations

### Create a Directory

Directories are created by sending a `POST` to a path that ends with a trailing slash. No request body is required — the trailing slash is the signal.

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/files/uploads/
```

A successful response returns `HTTP 201 Created` with no body. If the directory already exists on the same server, CallFS returns `HTTP 200 OK` rather than an error, making this operation safe to call idempotently for setup scripts.

### Upload a File

Upload a file by sending a `POST` with the file bytes as the request body. Set `Content-Type: application/octet-stream` to indicate raw binary content.

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @photo.jpg \
  http://localhost:8443/v1/files/uploads/photo.jpg
```

The file is stored in the configured backend. CallFS returns `HTTP 201 Created` on success. If the file already exists, the API returns `HTTP 409 Conflict` with a message directing you to use `PUT` to update it.

Both standard `Content-Length` uploads and chunked transfer encoding are supported. For chunked uploads, CallFS counts the actual bytes received and corrects the stored size in metadata automatically.

### Download a File

Retrieve a file with a `GET` request. The response body is the raw file bytes, streamed directly from the storage backend.

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/files/uploads/photo.jpg \
  -o photo.jpg
```

The response includes these headers alongside the file content:

| Header | Description |
|--------|-------------|
| `X-CallFS-Type` | Always `file` for file resources |
| `X-CallFS-Size` | File size in bytes |
| `X-CallFS-Mode` | File permission mode (e.g. `0644`) |
| `X-CallFS-Owner` | Application user ID of the owner |
| `X-CallFS-MTime` | Last modified time in RFC 3339 format |
| `Content-Type` | Always `application/octet-stream` |
| `Content-Length` | File size in bytes |

### Update a File

To replace the content of an existing file, use `PUT`. The semantics differ from `POST`: `PUT` creates the file if it does not exist, and overwrites it if it does.

```bash
curl -X PUT \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @photo-v2.jpg \
  http://localhost:8443/v1/files/uploads/photo.jpg
```

Response is `HTTP 200 OK` when an existing file is updated, or `HTTP 201 Created` when the file is new. `PUT` does not accept paths with a trailing slash — directory creation is exclusively a `POST` operation.

### Delete a File or Directory

```bash
curl -X DELETE \
  -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/files/uploads/photo.jpg
```

Returns `HTTP 204 No Content` on success. The same endpoint handles both files and directories — no separate path is needed.

### Read Metadata Without Downloading

The `HEAD` method returns all metadata headers with no response body. This is the efficient way to check file existence, size, or last-modified time before deciding whether to download.

```bash
curl -I \
  -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/files/uploads/photo.jpg
```

Response headers:

| Header | Description |
|--------|-------------|
| `X-CallFS-Type` | `file` or `directory` |
| `X-CallFS-Size` | Size in bytes |
| `X-CallFS-Mode` | Permission mode |
| `X-CallFS-Owner` | Owning application user ID |
| `X-CallFS-MTime` | Last modified timestamp |
| `X-CallFS-Instance-ID` | Which cluster node holds the file (multi-node setups) |

### List a Directory

Sending a `GET` to a directory path returns a JSON document describing the directory and its immediate contents.

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/directories/uploads
```

Response:

```json
{
  "path": "/uploads",
  "type": "directory",
  "recursive": false,
  "count": 3,
  "items": [
    {
      "name": "photo.jpg",
      "path": "/uploads/photo.jpg",
      "type": "file",
      "size": 204800,
      "mode": "0644",
      "owner": "user-123",
      "mtime": "2026-04-11T10:00:00Z"
    }
  ]
}
```

### Recursive Directory Listing

Add `?recursive=true` to traverse all subdirectories in a single request. An optional `max_depth` parameter limits traversal depth (default: 100, maximum: 1000).

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
  "http://localhost:8443/v1/directories/uploads?recursive=true&max_depth=5"
```

The response shape is identical to a flat listing, with all descendant items in the `items` array and `"recursive": true` in the envelope.

---

## Python Integration

The `requests` library maps cleanly to CallFS operations.

```python
import requests

BASE_URL = "http://localhost:8443/v1"
API_KEY = "YOUR_API_KEY"
HEADERS = {"Authorization": f"Bearer {API_KEY}"}


def create_directory(path: str) -> None:
    url = f"{BASE_URL}/files{path}/"
    response = requests.post(url, headers=HEADERS)
    response.raise_for_status()


def upload_file(remote_path: str, local_path: str) -> None:
    url = f"{BASE_URL}/files{remote_path}"
    with open(local_path, "rb") as f:
        response = requests.post(
            url,
            headers={**HEADERS, "Content-Type": "application/octet-stream"},
            data=f,
        )
    response.raise_for_status()


def download_file(remote_path: str, local_path: str) -> None:
    url = f"{BASE_URL}/files{remote_path}"
    response = requests.get(url, headers=HEADERS, stream=True)
    response.raise_for_status()
    with open(local_path, "wb") as f:
        for chunk in response.iter_content(chunk_size=65536):
            f.write(chunk)


def update_file(remote_path: str, local_path: str) -> None:
    url = f"{BASE_URL}/files{remote_path}"
    with open(local_path, "rb") as f:
        response = requests.put(
            url,
            headers={**HEADERS, "Content-Type": "application/octet-stream"},
            data=f,
        )
    response.raise_for_status()


def delete_file(remote_path: str) -> None:
    url = f"{BASE_URL}/files{remote_path}"
    response = requests.delete(url, headers=HEADERS)
    response.raise_for_status()


def get_metadata(remote_path: str) -> dict:
    url = f"{BASE_URL}/files{remote_path}"
    response = requests.head(url, headers=HEADERS)
    response.raise_for_status()
    return {
        "type": response.headers.get("X-CallFS-Type"),
        "size": int(response.headers.get("X-CallFS-Size", 0)),
        "mode": response.headers.get("X-CallFS-Mode"),
        "owner": response.headers.get("X-CallFS-Owner"),
        "mtime": response.headers.get("X-CallFS-MTime"),
    }


def list_directory(path: str, recursive: bool = False) -> dict:
    url = f"{BASE_URL}/directories{path}"
    params = {"recursive": "true"} if recursive else {}
    response = requests.get(url, headers=HEADERS, params=params)
    response.raise_for_status()
    return response.json()


# Example usage
create_directory("/uploads")
upload_file("/uploads/photo.jpg", "photo.jpg")
meta = get_metadata("/uploads/photo.jpg")
print(f"Stored {meta['size']} bytes, mode {meta['mode']}")
download_file("/uploads/photo.jpg", "photo-downloaded.jpg")
update_file("/uploads/photo.jpg", "photo-v2.jpg")
delete_file("/uploads/photo.jpg")
```

For large uploads, passing the file object directly to `data=` streams it without loading the entire file into memory.

---

## Node.js / JavaScript Integration

The native `fetch` API handles all CallFS operations without additional dependencies.

```javascript
const BASE_URL = "http://localhost:8443/v1";
const API_KEY = "YOUR_API_KEY";
const AUTH_HEADER = { Authorization: `Bearer ${API_KEY}` };

async function createDirectory(path) {
  const res = await fetch(`${BASE_URL}/files${path}/`, {
    method: "POST",
    headers: AUTH_HEADER,
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(`${err.code}: ${err.message}`);
  }
}

async function uploadFile(remotePath, fileBuffer) {
  const res = await fetch(`${BASE_URL}/files${remotePath}`, {
    method: "POST",
    headers: {
      ...AUTH_HEADER,
      "Content-Type": "application/octet-stream",
    },
    body: fileBuffer,
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(`${err.code}: ${err.message}`);
  }
}

async function downloadFile(remotePath) {
  const res = await fetch(`${BASE_URL}/files${remotePath}`, {
    headers: AUTH_HEADER,
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(`${err.code}: ${err.message}`);
  }
  return res.arrayBuffer();
}

async function updateFile(remotePath, fileBuffer) {
  const res = await fetch(`${BASE_URL}/files${remotePath}`, {
    method: "PUT",
    headers: {
      ...AUTH_HEADER,
      "Content-Type": "application/octet-stream",
    },
    body: fileBuffer,
  });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(`${err.code}: ${err.message}`);
  }
}

async function deleteFile(remotePath) {
  const res = await fetch(`${BASE_URL}/files${remotePath}`, {
    method: "DELETE",
    headers: AUTH_HEADER,
  });
  if (!res.ok && res.status !== 204) {
    const err = await res.json();
    throw new Error(`${err.code}: ${err.message}`);
  }
}

async function getMetadata(remotePath) {
  const res = await fetch(`${BASE_URL}/files${remotePath}`, {
    method: "HEAD",
    headers: AUTH_HEADER,
  });
  if (!res.ok) {
    throw new Error(`HEAD request failed: ${res.status}`);
  }
  return {
    type: res.headers.get("X-CallFS-Type"),
    size: parseInt(res.headers.get("X-CallFS-Size") ?? "0", 10),
    mode: res.headers.get("X-CallFS-Mode"),
    owner: res.headers.get("X-CallFS-Owner"),
    mtime: res.headers.get("X-CallFS-MTime"),
  };
}

async function listDirectory(path, recursive = false) {
  const url = new URL(`${BASE_URL}/directories${path}`);
  if (recursive) url.searchParams.set("recursive", "true");
  const res = await fetch(url, { headers: AUTH_HEADER });
  if (!res.ok) {
    const err = await res.json();
    throw new Error(`${err.code}: ${err.message}`);
  }
  return res.json();
}

// Example usage
await createDirectory("/uploads");
const buffer = await fs.promises.readFile("photo.jpg");
await uploadFile("/uploads/photo.jpg", buffer);
const meta = await getMetadata("/uploads/photo.jpg");
console.log(`Stored ${meta.size} bytes`);
const data = await downloadFile("/uploads/photo.jpg");
await fs.promises.writeFile("photo-downloaded.jpg", Buffer.from(data));
await deleteFile("/uploads/photo.jpg");
```

In a browser context, pass a `File` or `Blob` object directly as the `body` for uploads. In Node.js 18+, `fetch` is available globally and `Buffer` objects work as the `body`.

---

## Error Handling

Every error response from the file storage API uses the same JSON structure:

```json
{
  "code": "FILE_NOT_FOUND",
  "message": "the requested path does not exist"
}
```

The `code` field is machine-readable and stable across versions. The `message` field is human-readable and meant for logging.

| Code | HTTP Status | When it occurs |
|------|-------------|----------------|
| `FILE_NOT_FOUND` | 404 | Path does not exist |
| `FILE_ALREADY_EXISTS` | 409 | `POST` to a path that already holds a file |
| `AUTHENTICATION_FAILED` | 401 | Missing or invalid `Authorization` header |
| `PERMISSION_DENIED` | 403 | Token does not have access to the requested path |
| `INTERNAL_ERROR` | 500 | Storage backend or server-side failure |

For `409 Conflict` on cross-server clusters, the response body extends this structure with additional fields: `existing_path`, `instance_id`, `backend_type`, `suggestion`, and optionally `update_url` pointing to the node where the file lives.

When building retry logic, treat `401` and `403` as permanent failures (fix credentials or permissions before retrying), treat `404` as a permanent failure for downloads but recoverable for uploads (the file may not exist yet), and treat `500` as a transient failure eligible for exponential backoff.

---

## Next Steps

- Read the [Authentication and Security guide](../docs_markdown/04-authentication-security.md) to understand how API keys are issued and scoped.
- See the [Backend Configuration reference](../docs_markdown/05-backend-configuration.md) to understand where files are physically stored.
- Explore the [Clustering and Distribution guide](../docs_markdown/07-clustering-distribution.md) if you are running more than one CallFS instance.
