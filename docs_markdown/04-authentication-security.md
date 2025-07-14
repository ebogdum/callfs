# Authentication & Security

This document covers CallFS authentication mechanisms, security features, and best practices for secure deployment and operation.

## Authentication

### API Key Authentication

CallFS uses API key-based authentication with Bearer tokens for all protected endpoints.

#### Configuration

API keys are configured in the `auth.api_keys` configuration:

```yaml
auth:
  api_keys:
    - "api-key-1"
    - "api-key-2"
    - "api-key-3"
```

Or via environment variable:
```bash
export CALLFS_AUTH_API_KEYS="api-key-1,api-key-2,api-key-3"
```

#### Usage

Include the API key in the Authorization header:
```http
Authorization: Bearer your-api-key-here
```

#### Examples

```bash
# Valid request
curl -H "Authorization: Bearer api-key-1" \
  https://localhost:8443/v1/files/example.txt

# Invalid request (missing token)
curl https://localhost:8443/v1/files/example.txt
# Response: 401 Unauthorized

# Invalid request (wrong token)
curl -H "Authorization: Bearer invalid-key" \
  https://localhost:8443/v1/files/example.txt
# Response: 401 Unauthorized
```

#### API Key Best Practices

1. **Generate Strong Keys**
   ```bash
   # Generate 32-byte random key
   openssl rand -hex 32
   
   # Generate base64 encoded key
   openssl rand -base64 32
   
   # Generate UUID-based key
   uuidgen
   ```

2. **Key Rotation**
   - Support multiple keys during rotation
   - Remove old keys after migration
   - Monitor key usage through logs

3. **Key Storage**
   - Store in environment variables
   - Use secret management systems (Vault, Kubernetes secrets)
   - Never commit keys to version control

### Internal Proxy Authentication

For distributed deployments, CallFS instances authenticate with each other using internal proxy secrets.

#### Configuration

```yaml
auth:
  internal_proxy_secret: "secure-internal-secret"
```

#### Security Requirements

- Must be different from default value
- Should be at least 32 characters
- Must be shared across all cluster instances
- Should be rotated regularly

## Authorization

### Unix Permission Model

CallFS implements Unix-style permissions for file and directory access.

#### Permission Structure

- **Owner (User)**: File owner permissions (rwx)
- **Group**: Group permissions (rwx)
- **Other**: World permissions (rwx)

#### Permission Checks

1. **User Identification**: API key maps to system user (currently "root" for all keys)
2. **Permission Evaluation**: Standard Unix permission logic
3. **Access Decision**: Allow/deny based on permissions

#### Examples

```bash
# File with 644 permissions (rw-r--r--)
# Owner: read/write, Group: read, Other: read

# Directory with 755 permissions (rwxr-xr-x)
# Owner: read/write/execute, Group: read/execute, Other: read/execute
```

### Access Control Lists (Future)

Future versions will support extended ACLs for fine-grained access control.

## TLS/SSL Security

### TLS Configuration

CallFS requires TLS for all communications:

```yaml
server:
  cert_file: "/path/to/certificate.pem"
  key_file: "/path/to/private-key.pem"
```

### Certificate Requirements

- **TLS Version**: 1.2 or higher
- **Key Length**: Minimum 2048-bit RSA or 256-bit ECDSA
- **Certificate Chain**: Include intermediate certificates
- **SAN Extension**: Include all domains/IPs

### Certificate Generation

#### Self-Signed (Development)
```bash
# Generate private key
openssl genrsa -out server.key 2048

# Generate certificate
openssl req -new -x509 -key server.key -out server.crt -days 365 \
  -subj "/C=US/ST=CA/L=SF/O=CallFS/CN=localhost" \
  -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
```

#### Let's Encrypt (Production)
```bash
# Install certbot
sudo apt install certbot

# Generate certificate
sudo certbot certonly --standalone -d callfs.yourdomain.com

# Set up auto-renewal
sudo crontab -e
# Add: 0 12 * * * /usr/bin/certbot renew --quiet
```

#### Commercial CA
```bash
# Generate private key
openssl genrsa -out server.key 2048

# Generate CSR
openssl req -new -key server.key -out server.csr \
  -subj "/C=US/ST=CA/L=SF/O=YourOrg/CN=callfs.yourdomain.com"

# Submit CSR to CA and install signed certificate
```

## Security Headers

CallFS automatically adds comprehensive security headers to all responses:

### Content Security Policy (CSP)
```http
Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'self'; form-action 'self'
```

### HTTP Strict Transport Security (HSTS)
```http
Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
```

### Additional Headers
```http
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: strict-origin-when-cross-origin
Permissions-Policy: geolocation=(), microphone=(), camera=(), payment=(), usb=()
Cross-Origin-Opener-Policy: same-origin
Cross-Origin-Embedder-Policy: require-corp
```

## Single-Use Link Security

### Link Generation

Single-use links provide secure, time-limited access to files without requiring API keys.

#### HMAC Signing
- Links are signed with HMAC-SHA256
- Secret key configured in `auth.single_use_link_secret`
- Prevents tampering and unauthorized generation

#### Token Structure
```json
{
  "path": "/path/to/file",
  "expires_at": "2025-07-13T15:00:00Z",
  "instance_id": "callfs-instance-1"
}
```

#### Security Features

1. **Time-Limited**: Configurable expiry (1 second to 24 hours)
2. **Single-Use**: Token becomes invalid after one download
3. **Tamper-Proof**: HMAC signature prevents modification
4. **Path-Bound**: Token valid only for specific file
5. **Instance-Bound**: Prevents cross-instance replay

### Link Usage Best Practices

```bash
# Generate short-lived link (5 minutes)
curl -X POST -H "Authorization: Bearer api-key" \
  -H "Content-Type: application/json" \
  -d '{"path":"/sensitive/document.pdf","expiry_seconds":300}' \
  https://localhost:8443/v1/links/generate

# Use link immediately
curl "https://localhost:8443/download/token..."

# Link is now invalid for subsequent requests
```

## Rate Limiting

### Endpoint-Specific Limits

Different endpoints have different rate limits to prevent abuse:

#### Link Generation
- **Limit**: 100 requests per second
- **Burst**: 1 request
- **Window**: Rolling window

#### File Operations
- **Limit**: 1000 requests per minute per API key
- **Enforcement**: Per-API-key tracking

#### Directory Listing
- **Limit**: 500 requests per minute per API key
- **Enforcement**: Per-API-key tracking

### Rate Limit Headers

Responses include rate limit information:
```http
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1626181200
```

### Rate Limit Configuration

Rate limits are configurable in the router setup:
```go
linkRateLimiter := rate.NewLimiter(100, 1)
r.With(authMiddleware.RateLimitMiddleware(linkRateLimiter, logger))
```

## Network Security

### Firewall Configuration

Recommended firewall rules:

```bash
# Allow HTTPS traffic
sudo ufw allow 8443/tcp

# Allow metrics (internal only)
sudo ufw allow from 10.0.0.0/8 to any port 9090

# Deny all other traffic
sudo ufw default deny incoming
sudo ufw default allow outgoing
```

### Network Isolation

- **DMZ Deployment**: Place CallFS in DMZ network
- **Internal Networks**: Use internal networks for database/Redis
- **Load Balancer**: Use load balancer for SSL termination and DDoS protection

## Database Security

### PostgreSQL Security

#### Connection Security
```yaml
metadata_store:
  dsn: "postgres://callfs:password@localhost:5432/callfs?sslmode=require"
```

#### User Permissions
```sql
-- Create restricted user
CREATE USER callfs WITH ENCRYPTED PASSWORD 'secure_password';

-- Grant minimal permissions
GRANT CONNECT ON DATABASE callfs TO callfs;
GRANT USAGE ON SCHEMA public TO callfs;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO callfs;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO callfs;
```

#### Network Security
```conf
# postgresql.conf
listen_addresses = 'localhost'
ssl = on
ssl_cert_file = '/path/to/server.crt'
ssl_key_file = '/path/to/server.key'

# pg_hba.conf
hostssl callfs callfs 127.0.0.1/32 md5
hostssl callfs callfs ::1/128 md5
```

### Redis Security

#### Authentication
```conf
# redis.conf
requirepass secure_redis_password
```

#### Network Binding
```conf
# redis.conf
bind 127.0.0.1
port 6379
protected-mode yes
```

#### ACL Configuration (Redis 6+)
```conf
# Create user for CallFS
ACL SETUSER callfs on >password +@all -@dangerous
```

## Storage Backend Security

### Local Filesystem

#### File Permissions
```bash
# Secure data directory
sudo mkdir -p /var/lib/callfs
sudo chown callfs:callfs /var/lib/callfs
sudo chmod 750 /var/lib/callfs
```

#### Mount Options
```bash
# Mount with security options
mount -o nodev,nosuid,noexec /dev/disk /var/lib/callfs
```

### S3 Backend

#### IAM Policy
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::callfs-bucket",
        "arn:aws:s3:::callfs-bucket/*"
      ]
    }
  ]
}
```

#### Encryption Configuration
```yaml
backend:
  s3_server_side_encryption: "aws:kms"
  s3_kms_key_id: "arn:aws:kms:us-west-2:123456789:key/12345678-1234-1234-1234-123456789012"
  s3_acl: "private"
```

#### Bucket Policy
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "DenyInsecureConnections",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "s3:*",
      "Resource": [
        "arn:aws:s3:::callfs-bucket",
        "arn:aws:s3:::callfs-bucket/*"
      ],
      "Condition": {
        "Bool": {
          "aws:SecureTransport": "false"
        }
      }
    }
  ]
}
```

## Monitoring & Auditing

### Security Logging

CallFS logs all security-relevant events:

```json
{
  "level": "warn",
  "time": "2025-07-13T12:00:00Z",
  "msg": "Authentication failed",
  "remote_addr": "192.168.1.100",
  "user_agent": "curl/7.68.0",
  "error": "invalid API key"
}
```

### Audit Trail

Track security events:
- Authentication attempts (success/failure)
- Authorization failures
- File access events
- Administrative actions
- Rate limit violations

### Metrics Monitoring

Monitor security metrics:
```prometheus
# Authentication failures
increase(callfs_http_requests_total{status_code="401"}[5m])

# Rate limit violations
increase(callfs_http_requests_total{status_code="429"}[5m])

# Error rates
rate(callfs_http_requests_total{status_code=~"4.."}[5m])
```

## Security Scanning

### Vulnerability Assessment

Regular security assessments:

1. **Dependency Scanning**
   ```bash
   # Go vulnerability check
   go list -json -m all | nancy sleuth
   
   # CVE scanning
   govulncheck ./...
   ```

2. **Static Analysis**
   ```bash
   # Security-focused linting
   gosec ./...
   
   # General security analysis
   staticcheck ./...
   ```

3. **Container Scanning**
   ```bash
   # Scan Docker image
   docker scan callfs:latest
   
   # Trivy scanning
   trivy image callfs:latest
   ```

### Penetration Testing

Regular penetration testing should include:
- API authentication bypass attempts
- Authorization escalation tests
- Input validation testing
- TLS configuration assessment
- Rate limiting bypass attempts

## Security Incident Response

### Incident Categories

1. **Authentication Compromise**
   - Rotate affected API keys immediately
   - Review access logs
   - Notify affected users

2. **Data Breach**
   - Identify scope of breach
   - Preserve forensic evidence
   - Notify relevant authorities

3. **Service Disruption**
   - Implement rate limiting
   - Block malicious IPs
   - Scale infrastructure

### Response Procedures

1. **Immediate Response**
   - Isolate affected systems
   - Preserve logs and evidence
   - Implement containment measures

2. **Investigation**
   - Analyze logs and metrics
   - Identify attack vectors
   - Assess impact

3. **Recovery**
   - Patch vulnerabilities
   - Restore services
   - Monitor for recurrence

4. **Post-Incident**
   - Document lessons learned
   - Update security procedures
   - Implement additional controls

## Security Best Practices

### Deployment Security

1. **Environment Isolation**
   - Separate development/staging/production
   - Use different secrets for each environment
   - Implement network segmentation

2. **Secrets Management**
   - Use dedicated secret management systems
   - Rotate secrets regularly
   - Monitor secret usage

3. **Access Control**
   - Implement least privilege principle
   - Use role-based access control
   - Regular access reviews

### Operational Security

1. **Regular Updates**
   - Keep CallFS updated
   - Update dependencies regularly
   - Apply security patches promptly

2. **Monitoring**
   - Implement comprehensive logging
   - Set up security alerts
   - Regular security reviews

3. **Backup Security**
   - Encrypt backups
   - Secure backup storage
   - Test backup restoration

### Development Security

1. **Secure Coding**
   - Follow security guidelines
   - Regular code reviews
   - Security testing

2. **Dependency Management**
   - Audit dependencies regularly
   - Use dependency scanning tools
   - Keep dependencies updated

3. **Configuration Management**
   - Use infrastructure as code
   - Version control configurations
   - Implement change management

## Compliance Considerations

### Data Protection

- **GDPR**: Implement data protection measures
- **CCPA**: Provide data access and deletion capabilities
- **HIPAA**: Ensure healthcare data protection

### Industry Standards

- **SOC 2**: Implement security controls
- **ISO 27001**: Information security management
- **PCI DSS**: Payment card data protection

### Regulatory Requirements

- Document security procedures
- Implement audit trails
- Regular compliance assessments
- Security training programs
