# File Storage API: How to Upload, Download, and Manage Files Over REST

Every application that handles user data eventually needs somewhere to put files. Profile photos, document attachments, generated reports, audio recordings — at some point your backend needs a file storage layer, and the question is how to build it.

This guide covers the practical reality of that choice and walks through every file operation you will actually need: upload, download, update, delete, metadata inspection, and directory management — all using real HTTP calls you can run today.

---

## The File Storage Problem

When you start building a backend that needs to store files, you have roughly three paths:

**Build your own storage layer.** You write code that saves files to disk, tracks metadata in a database, handles concurrent writes, implements access control, and deals with cleanup. This works, but it is not your core product and it takes weeks to get right. Edge cases — partial uploads, concurrent modifications, permission enforcement — pile up quickly.

**Use S3 directly.** AWS S3 is reliable and scalable, but it introduces AWS dependency into every service that touches files. You are also dealing with presigned URLs, IAM policies, and the SDK surface area in every language you use. For some applications this is exactly right. For others, it adds friction that is not worth it.

**Use a self-hosted REST API file server.** A dedicated file storage service with a clean HTTP API sits between your application and wherever files actually live. Your application code speaks simple REST. The file server handles the storage details. You can run it on your own infrastructure, point it at local disk or S3, and keep the same API either way.

This guide uses [CallFS](https://github.com/orellazri/callfs) for examples. CallFS is an open-source self-hosted file storage server with a REST API, Bearer token authentication, and support for both local disk and S3 backends. The examples below are verified against CallFS v1.4.0.

---

## Authentication

Every request to the file storage API requires a Bearer token in the `Authorization` header. You configure API keys when you set up the server. All examples below use `YOUR_API_KEY` as a placeholder — substitute your actual key.

```
Authorization: Bearer YOUR_API_KEY
```

The base URL in all examples is `http://localhost:8443`. In production, this would be your server's address with TLS.

---

## File Operations

### Upload a File (POST)

To create a new file, send a POST request with the file contents as the request body. Set `Content-Type: application/octet-stream` for binary files. The URL path determines where the file is stored on the server.

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @photo.jpg \
  http://localhost:8443/v1/files/uploads/photo.jpg
```

The path `/uploads/photo.jpg` is the full path within the file store. If the `uploads` directory does not exist, create it first (see the directory section below). A successful upload returns HTTP 201.

For text files, you can also use `Content-Type: text/plain` or the appropriate MIME type. The server stores the content as-is.

### Download a File (GET)

To retrieve a file, send a GET request to the same path:

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/files/uploads/photo.jpg \
  -o photo.jpg
```

The `-o photo.jpg` flag writes the response body to a local file. Without it, curl writes to stdout. The server returns the raw file bytes with the appropriate content headers.

### Update a File (PUT)

To replace an existing file's contents, use PUT:

```bash
curl -X PUT \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @photo-v2.jpg \
  http://localhost:8443/v1/files/uploads/photo.jpg
```

PUT replaces the entire file. If the file does not exist at the specified path, the server returns an error. Use POST for initial creation, PUT for updates.

### Delete a File (DELETE)

```bash
curl -X DELETE \
  -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/files/uploads/photo.jpg
```

A successful delete returns HTTP 204 with no response body. Deleting a path that does not exist returns HTTP 404.

### Inspect File Metadata (HEAD)

The HEAD method returns file metadata without downloading the file body. This is useful for checking whether a file exists, when it was last modified, or how large it is before committing to a full download.

```bash
curl -I \
  -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/files/uploads/photo.jpg
```

The response includes these custom headers:

| Header | Description |
|--------|-------------|
| `X-CallFS-Size` | File size in bytes |
| `X-CallFS-Mode` | Unix file mode (e.g., `0644`) |
| `X-CallFS-Owner` | Owning user identifier |
| `X-CallFS-MTime` | Last modification time (RFC 3339) |
| `X-CallFS-Type` | Entry type: `file` or `directory` |

HEAD requests are cheap — they touch metadata only and do not read file data from disk or object storage.

---

## Directory Operations

Files live inside directories. The API distinguishes file paths from directory paths by the presence of a trailing slash.

### Create a Directory

Send a POST request to a path that ends with `/`:

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/files/uploads/
```

This creates the `uploads` directory. Directories can be nested — create parent directories before creating children.

### List a Directory

Directory listings use the `/v1/directories/` endpoint instead of `/v1/files/`:

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/directories/uploads/
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
      "mtime": "2025-11-14T09:32:11Z"
    }
  ]
}
```

The `count` field tells you how many items are in the listing. Each item in `items` includes its name, full path, type, size, and modification time.

### Recursive Directory Listing

To list a directory and all its subdirectories in a single request, add the `recursive=true` query parameter:

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
  "http://localhost:8443/v1/directories/uploads/?recursive=true"
```

The response structure is identical, but `recursive` will be `true` and `items` will include entries from all nested directories. Paths in the response reflect the full hierarchy.

---

## Integration Examples

### Python with requests

```python
import requests
from pathlib import Path

BASE_URL = "http://localhost:8443"
API_KEY = "YOUR_API_KEY"

session = requests.Session()
session.headers["Authorization"] = f"Bearer {API_KEY}"


def upload_file(local_path: str, remote_path: str) -> None:
    data = Path(local_path).read_bytes()
    response = session.post(
        f"{BASE_URL}/v1/files{remote_path}",
        data=data,
        headers={"Content-Type": "application/octet-stream"},
    )
    response.raise_for_status()


def download_file(remote_path: str, local_path: str) -> None:
    response = session.get(f"{BASE_URL}/v1/files{remote_path}")
    response.raise_for_status()
    Path(local_path).write_bytes(response.content)


def update_file(local_path: str, remote_path: str) -> None:
    data = Path(local_path).read_bytes()
    response = session.put(
        f"{BASE_URL}/v1/files{remote_path}",
        data=data,
        headers={"Content-Type": "application/octet-stream"},
    )
    response.raise_for_status()


def delete_file(remote_path: str) -> None:
    response = session.delete(f"{BASE_URL}/v1/files{remote_path}")
    response.raise_for_status()


def get_metadata(remote_path: str) -> dict:
    response = session.head(f"{BASE_URL}/v1/files{remote_path}")
    response.raise_for_status()
    return {
        "size": response.headers.get("X-CallFS-Size"),
        "mode": response.headers.get("X-CallFS-Mode"),
        "owner": response.headers.get("X-CallFS-Owner"),
        "mtime": response.headers.get("X-CallFS-MTime"),
        "type": response.headers.get("X-CallFS-Type"),
    }


def list_directory(remote_path: str, recursive: bool = False) -> dict:
    params = {"recursive": "true"} if recursive else {}
    response = session.get(
        f"{BASE_URL}/v1/directories{remote_path}",
        params=params,
    )
    response.raise_for_status()
    return response.json()


# Usage
upload_file("photo.jpg", "/uploads/photo.jpg")
meta = get_metadata("/uploads/photo.jpg")
print(f"Uploaded {meta['size']} bytes")

listing = list_directory("/uploads/")
for item in listing["items"]:
    print(item["name"], item["size"])
```

The `Session` object reuses the TCP connection across requests and automatically includes the `Authorization` header on every call. `raise_for_status()` converts 4xx and 5xx responses into exceptions so errors surface immediately.

### Node.js with fetch

```javascript
const BASE_URL = "http://localhost:8443";
const API_KEY = "YOUR_API_KEY";

const authHeaders = {
  Authorization: `Bearer ${API_KEY}`,
};

async function uploadFile(buffer, remotePath) {
  const response = await fetch(`${BASE_URL}/v1/files${remotePath}`, {
    method: "POST",
    headers: {
      ...authHeaders,
      "Content-Type": "application/octet-stream",
    },
    body: buffer,
  });
  if (!response.ok) {
    throw new Error(`Upload failed: ${response.status} ${response.statusText}`);
  }
}

async function downloadFile(remotePath) {
  const response = await fetch(`${BASE_URL}/v1/files${remotePath}`, {
    headers: authHeaders,
  });
  if (!response.ok) {
    throw new Error(`Download failed: ${response.status} ${response.statusText}`);
  }
  return response.arrayBuffer();
}

async function deleteFile(remotePath) {
  const response = await fetch(`${BASE_URL}/v1/files${remotePath}`, {
    method: "DELETE",
    headers: authHeaders,
  });
  if (!response.ok) {
    throw new Error(`Delete failed: ${response.status} ${response.statusText}`);
  }
}

async function getMetadata(remotePath) {
  const response = await fetch(`${BASE_URL}/v1/files${remotePath}`, {
    method: "HEAD",
    headers: authHeaders,
  });
  if (!response.ok) {
    throw new Error(`Metadata fetch failed: ${response.status}`);
  }
  return {
    size: response.headers.get("X-CallFS-Size"),
    mode: response.headers.get("X-CallFS-Mode"),
    owner: response.headers.get("X-CallFS-Owner"),
    mtime: response.headers.get("X-CallFS-MTime"),
    type: response.headers.get("X-CallFS-Type"),
  };
}

async function listDirectory(remotePath, recursive = false) {
  const url = new URL(`${BASE_URL}/v1/directories${remotePath}`);
  if (recursive) {
    url.searchParams.set("recursive", "true");
  }
  const response = await fetch(url.toString(), { headers: authHeaders });
  if (!response.ok) {
    throw new Error(`List failed: ${response.status} ${response.statusText}`);
  }
  return response.json();
}

// Usage example
import { readFile } from "fs/promises";

const buffer = await readFile("photo.jpg");
await uploadFile(buffer, "/uploads/photo.jpg");

const meta = await getMetadata("/uploads/photo.jpg");
console.log(`Size: ${meta.size} bytes, modified: ${meta.mtime}`);

const listing = await listDirectory("/uploads/");
listing.items.forEach((item) => console.log(item.name, item.size));
```

---

## Storage Backends: Local Disk and S3

CallFS supports two storage backends: local disk and S3-compatible object storage. The API your application calls is identical in both cases — you configure the backend on the server side.

Local disk storage writes files to a directory on the server's filesystem. This works well for single-server deployments and development environments. It is the simplest option to set up and reason about.

S3 backend mode stores files in an S3 bucket (AWS S3, MinIO, Cloudflare R2, or any S3-compatible service). Files appear to your application exactly as they do with local storage — same API, same paths, same responses. The server handles the translation between REST requests and S3 object operations.

You can also run CallFS in a hybrid configuration: keep recently accessed files on local disk and archive older files to S3. This provides local-speed access for hot data without indefinite disk growth.

The practical benefit of this architecture is that you can start with local disk storage during development, switch to S3 for production, and never change a line of application code. The file storage API contract stays constant.

---

## Common Patterns

### Checking Before Uploading

Use HEAD to check whether a file exists before deciding whether to POST or PUT:

```bash
# Returns 200 if file exists, 404 if not
curl -I -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/files/uploads/photo.jpg
```

If the response is 200, use PUT to update. If 404, use POST to create.

### Organizing Files with Directories

Create a directory structure that mirrors your data model. For example:

```
/uploads/users/42/avatar.jpg
/uploads/users/42/documents/report.pdf
/uploads/posts/187/attachments/screenshot.png
```

Create the full directory path before uploading. List `/uploads/users/42/` to retrieve all files for a specific user, or use `recursive=true` on `/uploads/users/` to get everything across all users.

### Large File Uploads

For large files, `--data-binary` reads the entire file into memory before sending. If you are scripting uploads of multi-gigabyte files, consider using curl's `--data-binary @-` with a pipe, or use a library that supports streaming request bodies. The Python `requests` library accepts file objects directly, which avoids loading the full content into memory:

```python
with open("large-file.bin", "rb") as f:
    session.post(
        f"{BASE_URL}/v1/files/uploads/large-file.bin",
        data=f,
        headers={"Content-Type": "application/octet-stream"},
    )
```

---

## Summary

A file storage API built around standard HTTP verbs is straightforward to work with from any language or tool. The operations map cleanly: POST to create, GET to retrieve, PUT to replace, DELETE to remove, HEAD to inspect. Directories use trailing slashes and a separate listing endpoint.

Running your own file storage service gives you control over data locality, cost, and integration with your existing infrastructure. With S3 backend support, you are not locked into local disk — you can move to cloud object storage without rewriting client code.

The curl examples in this guide are all you need to verify connectivity and test your setup before writing application code. From there, the Python and Node.js examples provide a starting point for integrating file storage into your application.
