# API Reference

This document provides a comprehensive reference for the CallFS REST API. All endpoints are versioned under `/v1` and require TLS (HTTPS).

## Authentication

All API endpoints, except for `/health`, `/metrics`, and `/download/{token}`, require bearer token authentication.

**Header:**
```
Authorization: Bearer <your-api-key>
```

## File and Directory Operations

These endpoints form the core of the filesystem API. They are designed to handle files and directories seamlessly, with intelligent routing in a clustered environment.

### `GET /v1/files/{path}`

Downloads a file or lists the contents of a directory.

- **If `{path}` is a file**: The response body will contain the raw file data.
  - **Headers**: `Content-Type: application/octet-stream`, `Content-Length`, and custom metadata headers (`X-CallFS-Mode`, `X-CallFS-	MTime`, etc.).
- **If `{path}` is a directory**: The response body will be a JSON array of file and directory metadata objects.

**Example: Download a file**
```bash
curl -k -H "Authorization: Bearer <api-key>" \
  https://localhost:8443/v1/files/documents/report.pdf
```

**Example: List a directory**
```bash
curl -k -H "Authorization: Bearer <api-key>" \
  https://localhost:8443/v1/files/documents/
```

### `HEAD /v1/files/{path}`

Retrieves metadata for a file or directory without transferring its content. This is an **enhanced** operation.

- **Cross-Server Routing**: If the resource is located on another node in the cluster, this request will be automatically proxied to the correct node.
- **Response Headers**: Includes detailed metadata such as `X-CallFS-Type`, `X-CallFS-Size`, `X-CallFS-Mode`, `X-CallFS-MTime`, `X-CallFS-Instance-ID`, and `X-CallFS-Backend-Type`.

**Example: Get file metadata**
```bash
curl -k -I -H "Authorization: Bearer <api-key>" \
  https://localhost:8443/v1/files/archive/dataset.zip
```

### `POST /v1/files/{path}`

Creates a new file or directory. This is an **enhanced** operation.

- **To create a file**: `POST` the raw file data with `Content-Type: application/octet-stream`.
- **To create a directory**: `POST` a JSON body `{"type":"directory"}` with `Content-Type: application/json`. The path must end with a `/`.
- **Cross-Server Conflict Detection**: Before creating, CallFS checks if the resource already exists anywhere in the cluster. If a conflict is found, it returns a `409 Conflict` error with details about the existing resource.

**Example: Create a directory**
```bash
curl -k -X POST -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" -d '{"type":"directory"}' \
  https://localhost:8443/v1/files/new-folder/
```

### `PUT /v1/files/{path}`

Uploads or updates a file's content. This is an **enhanced** operation.

- **Cross-Server Routing**: Automatically proxies the request to the node where the file is stored. If the file does not exist, it will be created on the default backend.
- **Streaming Support**: Efficiently handles large files by streaming data directly to the backend without buffering.

**Example: Upload a file**
```bash
curl -k -X PUT -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/octet-stream" --data-binary @local-file.txt \
  https://localhost:8443/v1/files/documents/remote-file.txt
```

### `DELETE /v1/files/{path}`

Deletes a file or an empty directory. This is an **enhanced** operation.

- **Cross-Server Routing**: Automatically proxies the delete request to the correct node in the cluster.

**Example: Delete a file**
```bash
curl -k -X DELETE -H "Authorization: Bearer <api-key>" \
  https://localhost:8443/v1/files/documents/obsolete-file.txt
```

## Enhanced Directory Listing

### `GET /v1/directories/{path}`

Provides a rich, JSON-formatted listing of a directory's contents with support for recursive traversal.

**Query Parameters:**
- `recursive` (boolean, optional): If `true`, lists contents of all subdirectories.
- `max_depth` (integer, optional): Limits the recursion depth when `recursive=true`.

**Example: Recursive listing with limited depth**
```bash
curl -k -H "Authorization: Bearer <api-key>" \
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
  "expires_at": "2025-07-15T18:00:00Z",
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

Exposes a wide range of performance metrics in Prometheus format for monitoring and alerting. **No authentication required.**

## Error Responses

Errors are returned with a standard JSON structure:
```json
{
  "error": "A brief, machine-readable error code",
  "message": "A human-readable description of the error."
}
```

**Example: File Not Found**
```json
{
  "error": "not_found",
  "message": "File or directory not found at path: /non/existent/file.txt"
}
```
