# Enhanced Cross-Server Functionality

This document describes the enhanced cross-server functionality for CallFS that addresses the key requirements for proper distributed file system behavior.

## Overview

The enhanced cross-server functionality provides:

1. **Conflict Detection with Suggestions**: When attempting to create files/directories that exist on other servers
2. **Cross-Server Operation Routing**: Automatic routing of operations to the correct server based on file location
3. **Comprehensive HTTP Method Support**: Full support for GET, HEAD, POST, PUT, DELETE across servers

## Key Features

### 1. POST Conflict Detection and Resolution

When you try to create a file or directory that already exists on another server:

**Scenario**: File exists on Server A, trying to create on Server B

**Response**: HTTP 409 Conflict with detailed information:
```json
{
  "error": "Resource exists on another server",
  "existing_path": "/path/to/file.txt",
  "instance_id": "callfs-localfs-1", 
  "backend_type": "localfs",
  "suggestion": "File already exists on another server. Use PUT to update it.",
  "update_url": "https://server-a:8443/v1/files/path/to/file.txt"
}
```

**Benefits**:
- Clear indication that the resource exists elsewhere
- Specific server information for debugging
- Actionable suggestions (use PUT for updates)
- Direct URL for cross-server operations

### 2. Enhanced PUT with Cross-Server Routing

**Automatic Proxying**: PUT requests are automatically routed to the server containing the file

```bash
# File created on LocalFS server
curl -X PUT -H "Authorization: Bearer key" \
  --data "content" \
  https://localfs-server:8443/v1/files/myfile.txt

# Update via S3 server - automatically proxied to LocalFS server
curl -X PUT -H "Authorization: Bearer key" \
  --data "updated content" \
  https://s3-server:8444/v1/files/myfile.txt
```

**Features**:
- Transparent proxying to the correct server
- Proper error handling and status codes
- Request/response header preservation
- Authentication forwarding

### 3. Enhanced DELETE with Cross-Server Routing

**Automatic Proxying**: DELETE requests are routed to the server containing the file/directory

```bash
# Delete file regardless of which server you connect to
curl -X DELETE -H "Authorization: Bearer key" \
  https://any-server:8443/v1/files/myfile.txt
```

**Features**:
- Works for both files and directories
- Directory emptiness checking before deletion
- Proper metadata cleanup across servers

### 4. Enhanced HEAD with Cross-Server Metadata

**Cross-Server Metadata Retrieval**: HEAD requests automatically fetch metadata from the correct server

```bash
curl -I -H "Authorization: Bearer key" \
  https://any-server:8443/v1/files/myfile.txt
```

**Response Headers Include**:
```
X-CallFS-Type: file
X-CallFS-Size: 1024
X-CallFS-Mode: 0644
X-CallFS-UID: 1000
X-CallFS-GID: 1000
X-CallFS-MTime: 2025-01-14T10:30:00Z
X-CallFS-Instance-ID: callfs-localfs-1
```

## Implementation Architecture

### Core Components

1. **Enhanced Engine Methods**:
   - `GetCurrentInstanceID()`: Returns current instance identifier
   - `GetPeerEndpoint(instanceID)`: Returns endpoint URL for peer instance

2. **Enhanced Handlers**:
   - `V1PostFileEnhanced`: POST with conflict detection
   - `V1PutFileEnhanced`: PUT with cross-server routing
   - `V1DeleteFileEnhanced`: DELETE with cross-server routing
   - `V1HeadFileEnhanced`: HEAD with cross-server metadata

3. **Proxy Functions**:
   - `proxyPutRequest()`: Proxies PUT operations
   - `proxyDeleteRequest()`: Proxies DELETE operations
   - `proxyHeadRequest()`: Proxies HEAD operations

### Routing Logic

```go
// Metadata lookup determines file location
md, err := engine.GetMetadata(ctx, path)

// Check if file is on current instance
if md.CallFSInstanceID != nil && *md.CallFSInstanceID != currentInstanceID {
    // File is on another server - proxy the request
    return proxyRequest(targetInstanceID, path, request)
}

// File is on this instance - handle locally
return handleLocally(path, request)
```

## Configuration

### Enabling Enhanced Cross-Server Support

Add to your configuration:

```yaml
handlers:
  enable_cross_server_support: true

instance_discovery:
  instance_id: "callfs-instance-1"
  peer_endpoints:
    callfs-instance-2: "https://server2:8443"
    callfs-instance-3: "https://server3:8443"
```

### Handler Selection

The system automatically chooses handlers based on configuration:

- **Enhanced Handlers**: When `enable_cross_server_support: true`
- **Standard Handlers**: When `enable_cross_server_support: false` (default)

## Error Handling

### Cross-Server Proxy Errors

**HTTP 502 Bad Gateway**: When proxy requests fail
```json
{
  "code": "PROXY_ERROR",
  "message": "failed to proxy request to owning server: connection refused"
}
```

**Common Causes**:
- Target server is down
- Network connectivity issues
- Authentication token expired
- Mismatched peer endpoint configuration

### Conflict Resolution Errors

**HTTP 409 Conflict**: For resource conflicts
```json
{
  "error": "Resource exists on another server",
  "instance_id": "target-server-id",
  "suggestion": "Use PUT to update the existing resource"
}
```

## Testing

### Test Script

Use the enhanced test script to validate functionality:

```bash
./scripts/05-test-enhanced-cross-server.sh
```

**Test Coverage**:
- POST conflict detection (both directions)
- PUT cross-server routing (both directions)  
- DELETE cross-server routing (both directions)
- HEAD cross-server metadata (both directions)

### Manual Testing

```bash
# Test conflict detection
curl -X POST -H "Authorization: Bearer key" \
  --data "content" \
  https://server1:8443/v1/files/test.txt

curl -X POST -H "Authorization: Bearer key" \
  --data "different content" \
  https://server2:8444/v1/files/test.txt
# Should return HTTP 409 with conflict information

# Test cross-server PUT
curl -X PUT -H "Authorization: Bearer key" \
  --data "updated content" \
  https://server2:8444/v1/files/test.txt
# Should successfully update file on server1
```

## Performance Considerations

### Latency
- Cross-server operations add network round-trip latency
- Metadata lookups are cached to minimize database queries
- Local operations remain fast (no proxy overhead)

### Scalability
- Each proxy request uses one connection from the connection pool
- Consider connection limits and timeouts in high-traffic scenarios
- Monitor proxy request success rates

### Optimization Tips
1. **Sticky Sessions**: Route clients to servers containing their files when possible
2. **Local Affinity**: Create files on the local server by default
3. **Monitoring**: Track cross-server request patterns and optimize placement

## Security

### Authentication
- Original authentication tokens are forwarded to target servers
- Internal proxy uses shared secrets for server-to-server communication
- No credential exposure in cross-server requests

### Authorization
- Authorization is checked on the receiving server (not just the proxy)
- Permissions are enforced based on the actual file location
- No privilege escalation through proxying

## Monitoring

### Metrics
- Cross-server request counts by operation type
- Proxy request latency and success rates
- Conflict detection frequency
- Instance-specific file distribution

### Logging
- All cross-server operations are logged with instance information
- Proxy failures include detailed error context
- Conflict detection events are tracked for analysis

## Migration from Standard to Enhanced

### Backwards Compatibility
- Enhanced handlers are fully backwards compatible
- Existing files work without modification
- Standard API behavior is preserved for local operations

### Gradual Rollout
1. Deploy enhanced handlers with feature flag disabled
2. Enable enhanced support on test instances
3. Monitor performance and error rates
4. Gradually enable on production instances

### Configuration Changes
```yaml
# Before
handlers:
  # No cross-server configuration

# After  
handlers:
  enable_cross_server_support: true
```

This enhanced functionality transforms CallFS from a collection of independent instances into a truly distributed filesystem with intelligent operation routing and conflict resolution.
