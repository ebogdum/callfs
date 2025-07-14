# API Reference

This document provides a comprehensive reference for all CallFS REST API endpoints, including request/response formats, authentication, error handling, and examples.

## Base URL and Protocol

- **Protocol**: HTTPS only (TLS 1.2+)
- **Base URL**: `https://your-callfs-server:8443/v1`
- **Content-Type**: `application/octet-stream` for file operations, `application/json` for metadata operations

## Authentication

All API endpoints (except health checks and metrics) require authentication via API key in the Authorization header:

```http
Authorization: Bearer your-api-key-here
```

### Example Authentication
```bash
curl -H "Authorization: Bearer your-api-key" \
  https://localhost:8443/v1/files/example.txt
```

## File Operations API

### GET /v1/files/{path}

Downloads file content or lists directory contents.

#### Parameters
- **path** (string, required): File or directory path

#### Response Headers for Files
- `Content-Type`: `application/octet-stream`
- `Content-Length`: File size in bytes
- `X-CallFS-Type`: `file`
- `X-CallFS-Size`: File size
- `X-CallFS-Mode`: File permissions (octal)
- `X-CallFS-UID`: Owner user ID
- `X-CallFS-GID`: Owner group ID
- `X-CallFS-MTime`: Last modification time (RFC3339)

#### Response for Directories
```json
[
  {
    "name": "subdirectory",
    "path": "/parent/subdirectory",
    "type": "directory",
    "size": 0,
    "mode": "0755",
    "uid": 1000,
    "gid": 1000,
    "mtime": "2025-07-13T12:00:00Z"
  },
  {
    "name": "file.txt",
    "path": "/parent/file.txt",
    "type": "file",
    "size": 1024,
    "mode": "0644",
    "uid": 1000,
    "gid": 1000,
    "mtime": "2025-07-13T12:00:00Z"
  }
]
```

#### Examples

**Download File:**
```bash
curl -H "Authorization: Bearer your-api-key" \
  https://localhost:8443/v1/files/documents/readme.txt
```

**List Directory:**
```bash
curl -H "Authorization: Bearer your-api-key" \
  https://localhost:8443/v1/files/documents/
```

#### Status Codes
- `200 OK`: Success
- `401 Unauthorized`: Invalid or missing API key
- `403 Forbidden`: Permission denied
- `404 Not Found`: File or directory not found
- `500 Internal Server Error`: Server error

### HEAD /v1/files/{path}

Retrieves file metadata without downloading content. **Enhanced with cross-server routing**.

#### Parameters
- **path** (string, required): File path

#### Response Headers
- `X-CallFS-Type`: `file` or `directory`
- `X-CallFS-Size`: File size in bytes
- `X-CallFS-Mode`: File permissions (octal)
- `X-CallFS-UID`: Owner user ID
- `X-CallFS-GID`: Owner group ID
- `X-CallFS-MTime`: Last modification time (RFC3339)
- `X-CallFS-Instance-ID`: Instance ID where file is stored
- `X-CallFS-Backend-Type`: Backend type (`localfs`, `s3`, `internalproxy`)

#### Cross-Server Behavior
If the file exists on another server instance, the request is automatically proxied to the correct server.

#### Example
```bash
curl -I -H "Authorization: Bearer your-api-key" \
  https://localhost:8443/v1/files/documents/readme.txt
```

### POST /v1/files/{path}

Creates a new file or directory. **Enhanced with cross-server conflict detection**.

#### Parameters
- **path** (string, required): File or directory path
- **Body**: File content (for files) or `{"type":"directory"}` (for directories)

#### Content Types
- **Files**: `application/octet-stream`
- **Directories**: `application/json`

#### Cross-Server Conflict Detection
If the resource already exists on another server, returns HTTP 409 with detailed conflict information:

```json
{
  "error": "Resource exists on another server",
  "existing_path": "/documents/readme.txt",
  "instance_id": "callfs-localfs-1",
  "backend_type": "localfs",
  "suggestion": "File already exists on another server. Use PUT to update it.",
  "update_url": "https://server-a:8443/v1/files/documents/readme.txt"
}
```

#### Examples

**Create File:**
```bash
curl -X POST -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @localfile.txt \
  https://localhost:8443/v1/files/documents/newfile.txt
```

**Create Directory:**
```bash
curl -X POST -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"type":"directory"}' \
  https://localhost:8443/v1/files/documents/newfolder/
```

#### Status Codes
- `201 Created`: File or directory created successfully
- `400 Bad Request`: Invalid request (e.g., missing trailing slash for directory)
- `401 Unauthorized`: Invalid or missing API key
- `403 Forbidden`: Permission denied
- `409 Conflict`: File or directory already exists (with cross-server info)
- `500 Internal Server Error`: Server error

### PUT /v1/files/{path}

Updates an existing file's content. **Enhanced with cross-server routing**.

#### Parameters
- **path** (string, required): File path (no trailing slash)
- **Body**: New file content

#### Content Type
- `application/octet-stream`

#### Cross-Server Behavior
If the file exists on another server instance, the request is automatically proxied to the correct server.

#### Example
```bash
curl -X PUT -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @updated-file.txt \
  https://localhost:8443/v1/files/documents/readme.txt
```

#### Status Codes
- `200 OK`: File updated successfully
- `201 Created`: File created (if it didn't exist)
- `400 Bad Request`: Invalid request (e.g., path with trailing slash)
- `401 Unauthorized`: Invalid or missing API key
- `403 Forbidden`: Permission denied
- `404 Not Found`: File not found
- `502 Bad Gateway`: Cross-server proxy error
- `500 Internal Server Error`: Server error

### DELETE /v1/files/{path}

Deletes a file or empty directory. **Enhanced with cross-server routing**.

#### Parameters
- **path** (string, required): File or directory path (use trailing slash for directories)

#### Cross-Server Behavior
If the file exists on another server instance, the request is automatically proxied to the correct server.

#### Examples

**Delete File:**
```bash
curl -X DELETE -H "Authorization: Bearer your-api-key" \
  https://localhost:8443/v1/files/documents/oldfile.txt
```

**Delete Directory:**
```bash
curl -X DELETE -H "Authorization: Bearer your-api-key" \
  https://localhost:8443/v1/files/documents/oldfolder/
```

#### Status Codes
- `204 No Content`: File or directory deleted successfully
- `400 Bad Request`: Directory not empty
- `401 Unauthorized`: Invalid or missing API key
- `403 Forbidden`: Permission denied
- `404 Not Found`: File or directory not found
- `502 Bad Gateway`: Cross-server proxy error
- `500 Internal Server Error`: Server error

## Directory Listing API

### GET /v1/directories/{path}

Enhanced directory listing with comprehensive metadata and recursive options.

#### Parameters
- **path** (string, required): Directory path
- **recursive** (boolean, optional): Enable recursive listing
- **max_depth** (integer, optional): Maximum recursion depth (1-1000, default: 100)

#### Response Format
```json
{
  "path": "/documents",
  "type": "directory",
  "recursive": false,
  "max_depth": 100,
  "count": 5,
  "items": [
    {
      "name": "subdirectory",
      "path": "/documents/subdirectory",
      "type": "directory",
      "size": 0,
      "mode": "0755",
      "uid": 1000,
      "gid": 1000,
      "mtime": "2025-07-13T12:00:00Z"
    },
    {
      "name": "document.pdf",
      "path": "/documents/document.pdf",
      "type": "file",
      "size": 102400,
      "mode": "0644",
      "uid": 1000,
      "gid": 1000,
      "mtime": "2025-07-13T11:30:00Z"
    }
  ]
}
```

#### Examples

**Basic Directory Listing:**
```bash
curl -H "Authorization: Bearer your-api-key" \
  https://localhost:8443/v1/directories/documents
```

**Recursive Listing:**
```bash
curl -H "Authorization: Bearer your-api-key" \
  "https://localhost:8443/v1/directories/documents?recursive=true"
```

**Depth-Limited Recursive Listing:**
```bash
curl -H "Authorization: Bearer your-api-key" \
  "https://localhost:8443/v1/directories/documents?recursive=true&max_depth=2"
```

#### Status Codes
- `200 OK`: Success
- `400 Bad Request`: Path is not a directory or invalid parameters
- `401 Unauthorized`: Invalid or missing API key
- `403 Forbidden`: Permission denied
- `404 Not Found`: Directory not found
- `500 Internal Server Error`: Server error

## Single-Use Links API

### POST /v1/links/generate

Generates a secure, time-limited, single-use download link for a file. **Rate limited to 100 requests per second**.

#### Request Format
```json
{
  "path": "/documents/sensitive-file.pdf",
  "expiry_seconds": 3600
}
```

#### Parameters
- **path** (string, required): File path
- **expiry_seconds** (integer, required): Link expiry time in seconds (1-86400)

#### Response Format
```json
{
  "url": "https://localhost:8443/download/eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": "2025-07-13T13:00:00Z",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

#### Security Features
- HMAC-SHA256 token signing
- Cryptographically secure token generation
- Automatic cleanup of expired links

#### Example
```bash
curl -X POST -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"path":"/documents/report.pdf","expiry_seconds":3600}' \
  https://localhost:8443/v1/links/generate
```

#### Status Codes
- `201 Created`: Link generated successfully
- `400 Bad Request`: Invalid request parameters
- `401 Unauthorized`: Invalid or missing API key
- `403 Forbidden`: Permission denied
- `404 Not Found`: File not found
- `429 Too Many Requests`: Rate limit exceeded
- `500 Internal Server Error`: Server error

### GET /download/{token}

Downloads a file using a single-use token. No authentication required.

#### Parameters
- **token** (string, required): Single-use download token

#### Response
Returns file content with appropriate headers. Token becomes invalid after one use.

#### Example
```bash
curl https://localhost:8443/download/eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

#### Status Codes
- `200 OK`: File downloaded successfully
- `400 Bad Request`: Invalid token format
- `404 Not Found`: Token not found
- `410 Gone`: Token expired or already used
- `500 Internal Server Error`: Server error

## System Endpoints

### GET /health

Health check endpoint. No authentication required.

#### Response
```json
{
  "status": "ok"
}
```

#### Example
```bash
curl https://localhost:8443/health
```

#### Status Codes
- `200 OK`: Service is healthy

### GET /metrics

Prometheus metrics endpoint. No authentication required.

#### Response
Returns metrics in Prometheus exposition format.

#### Example
```bash
curl https://localhost:8443/metrics
```

Sample metrics:
```
# HELP callfs_http_requests_total Total number of HTTP requests
# TYPE callfs_http_requests_total counter
callfs_http_requests_total{method="GET",path="/files/*",status_code="200"} 42

# HELP callfs_backend_ops_total Total number of backend operations
# TYPE callfs_backend_ops_total counter
callfs_backend_ops_total{backend_type="localfs",operation="read"} 35
```

## Error Response Format

All API errors return a consistent JSON format:

```json
{
  "code": "ERROR_CODE",
  "message": "Human readable error message"
}
```

### Error Codes

| Code | Description |
|------|-------------|
| `AUTHENTICATION_FAILED` | Invalid or missing API key |
| `AUTHORIZATION_FAILED` | Permission denied |
| `FILE_NOT_FOUND` | File or directory not found |
| `FILE_ALREADY_EXISTS` | File or directory already exists |
| `DIRECTORY_NOT_EMPTY` | Directory contains files and cannot be deleted |
| `INVALID_PATH` | Invalid file path |
| `INVALID_REQUEST` | Invalid request format or parameters |
| `INTERNAL_ERROR` | Internal server error |
| `RATE_LIMITED` | Too many requests |

## Rate Limiting

API endpoints have different rate limits:

- **File Operations**: 1000 requests per minute per API key
- **Link Generation**: 100 requests per second with burst of 1
- **Directory Listing**: 500 requests per minute per API key

Rate limit headers are included in responses:
- `X-RateLimit-Limit`: Request limit
- `X-RateLimit-Remaining`: Remaining requests
- `X-RateLimit-Reset`: Time when limit resets

## HTTP Headers

### Request Headers

| Header | Required | Description |
|--------|----------|-------------|
| `Authorization` | Yes* | Bearer token authentication |
| `Content-Type` | For POST/PUT | Content type of request body |
| `Content-Length` | For POST/PUT | Length of request body |

*Not required for health, metrics, and download endpoints

### Response Headers

| Header | Description |
|--------|-------------|
| `Content-Type` | Response content type |
| `Content-Length` | Response content length |
| `X-CallFS-Type` | Resource type (file/directory) |
| `X-CallFS-Size` | File size in bytes |
| `X-CallFS-Mode` | Unix file permissions |
| `X-CallFS-UID` | Owner user ID |
| `X-CallFS-GID` | Owner group ID |
| `X-CallFS-MTime` | Last modification time |

### Security Headers

All responses include security headers:
- `Strict-Transport-Security`: HSTS enforcement
- `X-Content-Type-Options`: nosniff
- `X-Frame-Options`: DENY
- `Content-Security-Policy`: Restrictive CSP
- `Referrer-Policy`: strict-origin-when-cross-origin

## API Usage Patterns

### Uploading Large Files

For large files, use streaming uploads:

```bash
# Stream large file upload
curl -X POST -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/octet-stream" \
  -T largefile.zip \
  https://localhost:8443/v1/files/uploads/largefile.zip
```

### Batch Operations

For multiple file operations, make concurrent requests:

```bash
# Upload multiple files concurrently
for file in *.txt; do
  curl -X POST -H "Authorization: Bearer your-api-key" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$file" \
    "https://localhost:8443/v1/files/batch/$file" &
done
wait
```

### Directory Synchronization

Use recursive directory listing to synchronize:

```bash
# Get complete directory structure
curl -H "Authorization: Bearer your-api-key" \
  "https://localhost:8443/v1/directories/sync-folder?recursive=true" \
  | jq '.items[] | select(.type == "file") | {path: .path, mtime: .mtime}'
```

### Secure File Sharing

Generate single-use links for secure sharing:

```bash
# Generate 1-hour download link
LINK=$(curl -X POST -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"path":"/shared/document.pdf","expiry_seconds":3600}' \
  https://localhost:8443/v1/links/generate | jq -r '.url')

echo "Share this link: $LINK"
```

## SDK Examples

### cURL Examples

Complete file operation workflow:

```bash
# Set API key
API_KEY="your-api-key"
BASE_URL="https://localhost:8443"

# Create directory
curl -X POST -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"type":"directory"}' \
  "$BASE_URL/v1/files/projects/new-project/"

# Upload file
curl -X POST -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @README.md \
  "$BASE_URL/v1/files/projects/new-project/README.md"

# List directory
curl -H "Authorization: Bearer $API_KEY" \
  "$BASE_URL/v1/directories/projects/new-project?recursive=true"

# Generate share link
curl -X POST -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"path":"/projects/new-project/README.md","expiry_seconds":86400}' \
  "$BASE_URL/v1/links/generate"

# Download file
curl -H "Authorization: Bearer $API_KEY" \
  -o local-readme.md \
  "$BASE_URL/v1/files/projects/new-project/README.md"

# Delete file
curl -X DELETE -H "Authorization: Bearer $API_KEY" \
  "$BASE_URL/v1/files/projects/new-project/README.md"

# Delete directory
curl -X DELETE -H "Authorization: Bearer $API_KEY" \
  "$BASE_URL/v1/files/projects/new-project/"
```

### Python Example

```python
import requests
import json

class CallFSClient:
    def __init__(self, base_url, api_key):
        self.base_url = base_url
        self.headers = {'Authorization': f'Bearer {api_key}'}
    
    def upload_file(self, local_path, remote_path):
        with open(local_path, 'rb') as f:
            response = requests.post(
                f"{self.base_url}/v1/files{remote_path}",
                headers={**self.headers, 'Content-Type': 'application/octet-stream'},
                data=f
            )
        return response.status_code == 201
    
    def download_file(self, remote_path, local_path):
        response = requests.get(
            f"{self.base_url}/v1/files{remote_path}",
            headers=self.headers
        )
        if response.status_code == 200:
            with open(local_path, 'wb') as f:
                f.write(response.content)
            return True
        return False
    
    def list_directory(self, path, recursive=False):
        params = {'recursive': 'true'} if recursive else {}
        response = requests.get(
            f"{self.base_url}/v1/directories{path}",
            headers=self.headers,
            params=params
        )
        return response.json() if response.status_code == 200 else None

# Usage
client = CallFSClient('https://localhost:8443', 'your-api-key')
client.upload_file('local-file.txt', '/remote/file.txt')
files = client.list_directory('/remote', recursive=True)
```

### JavaScript Example

```javascript
class CallFSClient {
    constructor(baseUrl, apiKey) {
        this.baseUrl = baseUrl;
        this.headers = {
            'Authorization': `Bearer ${apiKey}`
        };
    }

    async uploadFile(file, remotePath) {
        const response = await fetch(`${this.baseUrl}/v1/files${remotePath}`, {
            method: 'POST',
            headers: {
                ...this.headers,
                'Content-Type': 'application/octet-stream'
            },
            body: file
        });
        return response.status === 201;
    }

    async listDirectory(path, recursive = false) {
        const params = recursive ? '?recursive=true' : '';
        const response = await fetch(`${this.baseUrl}/v1/directories${path}${params}`, {
            headers: this.headers
        });
        return response.ok ? await response.json() : null;
    }

    async generateLink(path, expirySeconds) {
        const response = await fetch(`${this.baseUrl}/v1/links/generate`, {
            method: 'POST',
            headers: {
                ...this.headers,
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                path: path,
                expiry_seconds: expirySeconds
            })
        });
        return response.ok ? await response.json() : null;
    }
}

// Usage
const client = new CallFSClient('https://localhost:8443', 'your-api-key');
const files = await client.listDirectory('/documents', true);
const link = await client.generateLink('/documents/report.pdf', 3600);
```

## Best Practices

1. **Use appropriate HTTP methods** for each operation type
2. **Include trailing slashes** for directory operations where required
3. **Stream large files** instead of loading into memory
4. **Handle rate limits** with exponential backoff
5. **Validate responses** and handle errors appropriately
6. **Use single-use links** for secure file sharing
7. **Monitor API usage** through metrics endpoint
8. **Cache directory listings** when appropriate to reduce API calls
