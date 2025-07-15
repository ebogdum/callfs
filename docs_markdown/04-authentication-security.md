# Authentication & Security

CallFS is designed with security as a first-class citizen. This document outlines the authentication mechanisms, security features, and best practices for deploying and operating CallFS securely.

## Authentication

### API Key Authentication

All protected API endpoints use bearer token authentication.

**Configuration:**
API keys are defined in your `config.yaml` or via environment variables. It is recommended to use a secrets management system in production.

```yaml
auth:
  api_keys:
    - "your-strong-api-key-1"
    - "your-strong-api-key-2"
```
**Usage:**
Provide the key in the `Authorization` header of your HTTP requests.
```http
Authorization: Bearer your-strong-api-key-1
```

### Internal Proxy Authentication

In a clustered setup, CallFS instances authenticate with each other using a shared secret. This ensures that only trusted nodes can participate in cross-server operations.

**Configuration:**
```yaml
auth:
  internal_proxy_secret: "a-very-strong-and-long-shared-secret"
```
This secret must be identical across all nodes in the cluster.

## Authorization: Unix Permission Model

CallFS enforces a standard Unix-style permission model for all file and directory operations. Each file and directory has an owner, a group, and a set of permissions (read, write, execute) for the owner, group, and others.

- **Ownership**: When a file is created, its ownership is assigned based on the authenticated user.
- **Permission Checks**: Every API operation that accesses a file or directory is checked against these permissions. For example, a `PUT` request to update a file requires write permission.

This model provides a familiar and powerful way to control access to your data.

## TLS/SSL Encryption

All communication with the CallFS API is encrypted using TLS 1.2 or higher.

**Configuration:**
You must provide a valid TLS certificate and private key.
```yaml
server:
  cert_file: "/path/to/your/fullchain.pem"
  key_file: "/path/to/your/privkey.pem"
```
For production, it is highly recommended to use certificates from a trusted Certificate Authority (CA) like Let's Encrypt.

## Secure Single-Use Links

Single-use links provide a secure way to grant temporary, one-time access to files without exposing your API keys.

**Security Features:**
- **HMAC-Signed Tokens**: Links are protected with an HMAC-SHA256 signature, making them tamper-proof. The signature is generated using the `single_use_link_secret` from your configuration.
- **Time-Limited**: Each link has a configurable expiration time (from seconds to hours).
- **One-Time Use**: A token is automatically invalidated after the first successful download.
- **Path-Bound**: A token is valid only for the specific file path it was generated for.

## Security Headers

CallFS automatically includes a comprehensive set of HTTP security headers in all responses to protect against common web vulnerabilities:
- `Content-Security-Policy`
- `Strict-Transport-Security` (HSTS)
- `X-Content-Type-Options`
- `X-Frame-Options`
- and more, to enforce best security practices on the client-side.

## Rate Limiting

To prevent abuse and ensure service stability, CallFS implements rate limiting on its API endpoints.
- **Link Generation**: Has a stricter rate limit to prevent token generation abuse.
- **File Operations**: Have more lenient limits suitable for normal application usage.

Rate limit status is communicated via standard HTTP headers (`X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`).

## Backend Storage Security

### Local Filesystem
- **Permissions**: Ensure the `localfs_root_path` directory has appropriate file permissions, restricting access to the user running the CallFS process.
- **Mount Options**: When possible, mount the filesystem with security-enhancing options like `nodev`, `nosuid`, and `noexec`.

### S3 Backend
- **IAM Policies**: Use IAM roles with least-privilege policies that grant CallFS only the necessary permissions (`s3:GetObject`, `s3:PutObject`, `s3:DeleteObject`, `s3:ListBucket`).
- **Encryption**: Enforce server-side encryption (SSE-S3, SSE-KMS) on your S3 bucket to protect data at rest.
- **Bucket Policies**: Use bucket policies to enforce SSL/TLS for all connections to your bucket.

## Best Practices for Secure Deployment

- **Secrets Management**: Never hardcode API keys or secrets in your configuration files. Use environment variables or a dedicated secrets management service (e.g., HashiCorp Vault, AWS Secrets Manager).
- **Firewall**: Configure your firewall to only allow traffic on the necessary ports (e.g., 8443 for the API). If possible, the database and Redis should not be exposed to the public internet.
- **Regular Updates**: Keep CallFS and its dependencies (Go, PostgreSQL, Redis) up to date with the latest security patches.
- **Monitoring**: Actively monitor logs for suspicious activity, such as failed authentication attempts or authorization failures.
