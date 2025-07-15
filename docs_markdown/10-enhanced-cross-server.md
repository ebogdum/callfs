# Enhanced Cross-Server Operations

In a distributed CallFS cluster, "enhanced" API operations provide intelligent routing and conflict management, making the cluster behave like a single, unified filesystem. This document details the functionality of these crucial features.

## Core Concepts

- **Automatic Routing**: When you make a request to one node for a file that is stored on another node's local filesystem, CallFS automatically proxies the request to the correct node. This is transparent to the client.
- **Conflict Detection**: When you try to create a file or directory, CallFS checks the entire cluster to prevent accidental overwrites of data that exists on other nodes.

These features apply to the `HEAD`, `POST`, `PUT`, and `DELETE` methods on the `/v1/files/{path}` endpoint.

## Enhanced Operations

### `POST /v1/files/{path}` - Create with Conflict Detection

When you `POST` to create a new file or directory, CallFS performs a cluster-wide check.

- **If the path is available everywhere**: The resource is created on the default backend of the node that received the request.
- **If the path already exists on another node**: The API returns a `409 Conflict` error. The response body provides details about the conflict, including which node holds the resource and a suggestion to use `PUT` for updates.

**Example `409 Conflict` Response:**
```json
{
  "error": "Resource exists on another server",
  "existing_path": "/shared/report.docx",
  "instance_id": "callfs-node-east",
  "backend_type": "localfs",
  "suggestion": "File already exists on another server. Use PUT to update it."
}
```

### `PUT /v1/files/{path}` - Update with Automatic Routing

When you `PUT` to update a file, CallFS first looks up the file's location in the metadata store.

- **If the file is on the current node**: The operation proceeds locally.
- **If the file is on a different node**: The request (including the data payload) is automatically proxied to the correct node.
- **If the file does not exist**: It is created on the default backend of the current node.

This ensures that you can update any file by connecting to any node in the cluster, without needing to know where the file is physically stored.

### `DELETE /v1/files/{path}` - Delete with Automatic Routing

Similar to `PUT`, a `DELETE` request is automatically routed to the node that holds the file or directory, ensuring the correct resource is removed.

### `HEAD /v1/files/{path}` - Get Metadata with Automatic Routing

A `HEAD` request will also be proxied to the correct node to fetch the resource's metadata. The response headers will include the `X-CallFS-Instance-ID` and `X-CallFS-Backend-Type`, telling you exactly where the file is located.

## Use Cases

- **Unified Namespace**: Present a single, consistent filesystem view to your applications, even when data is physically distributed across many servers and storage backends.
- **Data Locality**: Allow applications to write data to a local CallFS node for low latency, while still being able to access that data from any other node in the cluster.
- **Simplified Clients**: Client applications don't need complex logic to track file locations; they can simply interact with any node in the cluster.
