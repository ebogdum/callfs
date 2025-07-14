# Installation Guide

This guide covers all installation methods for CallFS, from source compilation to containerized deployments.

## Prerequisites

### System Requirements
- **Operating System**: Linux, macOS, or Windows
- **Go Version**: 1.21 or later (for source installation)
- **Memory**: Minimum 512MB RAM, recommended 2GB+ for production
- **Storage**: Depends on backend configuration
- **Network**: HTTPS/TLS capabilities required

### Dependencies
- **PostgreSQL**: 12+ for metadata storage
- **Redis**: 6+ for distributed locking
- **TLS Certificates**: Required for HTTPS operation

## Installation Methods

### 1. From Source (Recommended for Development)

#### Clone Repository
```bash
git clone https://github.com/ebogdum/callfs.git
cd callfs
```

#### Build Binary
```bash
# Build main server
go build -o bin/callfs ./cmd
```

#### Install Dependencies
```bash
go mod download
go mod verify
```

### 2. Using Docker

#### Quick Start with Docker Compose
```bash
# Clone repository
git clone https://github.com/ebogdum/callfs.git
cd callfs

# Start all services
docker-compose up -d

# View logs
docker-compose logs -f callfs
```

#### Manual Docker Setup
```bash
# Build image
docker build -t callfs:latest .

# Run container
docker run -d \
  --name callfs \
  -p 8443:8443 \
  -p 9090:9090 \
  -v /path/to/config.yaml:/app/config.yaml \
  -v /path/to/certs:/app/certs \
  -v /path/to/data:/var/lib/callfs \
  callfs:latest
```

### 3. Using Pre-built Binaries

#### Download Latest Release
```bash
# Linux AMD64
curl -L -o callfs https://github.com/ebogdum/callfs/releases/latest/download/callfs-linux-amd64
chmod +x callfs

# macOS ARM64
curl -L -o callfs https://github.com/ebogdum/callfs/releases/latest/download/callfs-darwin-arm64
chmod +x callfs

# Windows
curl -L -o callfs.exe https://github.com/ebogdum/callfs/releases/latest/download/callfs-windows-amd64.exe
```

## Database Setup

### PostgreSQL Configuration

#### 1. Install PostgreSQL
```bash
# Ubuntu/Debian
sudo apt update
sudo apt install postgresql postgresql-contrib

# CentOS/RHEL
sudo yum install postgresql postgresql-server

# macOS
brew install postgresql
```

#### 2. Create Database and User
```sql
-- Connect as postgres user
sudo -u postgres psql

-- Create database and user
CREATE DATABASE callfs;
CREATE USER callfs WITH ENCRYPTED PASSWORD 'secure_password';
GRANT ALL PRIVILEGES ON DATABASE callfs TO callfs;

-- Grant schema permissions
\c callfs
GRANT ALL ON SCHEMA public TO callfs;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO callfs;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO callfs;
```

#### 3. Configure Connection
```bash
# Edit postgresql.conf
sudo nano /etc/postgresql/14/main/postgresql.conf

# Enable connections
listen_addresses = 'localhost'
port = 5432

# Edit pg_hba.conf for authentication
sudo nano /etc/postgresql/14/main/pg_hba.conf

# Add line for callfs user
host    callfs          callfs          127.0.0.1/32            md5
```

### Redis Configuration

#### 1. Install Redis
```bash
# Ubuntu/Debian
sudo apt install redis-server

# CentOS/RHEL
sudo yum install redis

# macOS
brew install redis
```

#### 2. Configure Redis
```bash
# Edit redis configuration
sudo nano /etc/redis/redis.conf

# Security settings
requirepass your_redis_password
bind 127.0.0.1
port 6379

# Persistence settings
save 900 1
save 300 10
save 60 10000
```

#### 3. Start Redis
```bash
# Start and enable Redis
sudo systemctl start redis
sudo systemctl enable redis

# Test connection
redis-cli ping
```

## TLS Certificate Setup

### 1. Self-Signed Certificates (Development)
```bash
# Generate private key
openssl genrsa -out server.key 2048

# Generate certificate signing request
openssl req -new -key server.key -out server.csr

# Generate self-signed certificate
openssl x509 -req -days 365 -in server.csr -signkey server.key -out server.crt

# Set permissions
chmod 600 server.key
chmod 644 server.crt
```

### 2. Let's Encrypt (Production)
```bash
# Install certbot
sudo apt install certbot

# Generate certificate
sudo certbot certonly --standalone -d your-domain.com

# Copy certificates
sudo cp /etc/letsencrypt/live/your-domain.com/fullchain.pem /path/to/callfs/server.crt
sudo cp /etc/letsencrypt/live/your-domain.com/privkey.pem /path/to/callfs/server.key
sudo chown callfs:callfs /path/to/callfs/server.*
```

## Configuration

### 1. Create Configuration File
```bash
# Copy example configuration
cp config.yaml.example config.yaml

# Edit configuration
nano config.yaml
```

### 2. Basic Configuration Example
```yaml
server:
  listen_addr: ":8443"
  external_url: "https://your-domain.com:8443"
  cert_file: "server.crt"
  key_file: "server.key"

auth:
  api_keys:
    - "your-secure-api-key-here"
  internal_proxy_secret: "your-internal-secret"
  single_use_link_secret: "your-link-secret"

metadata_store:
  dsn: "postgres://callfs:secure_password@localhost/callfs?sslmode=require"

dlm:
  redis_addr: "localhost:6379"
  redis_password: "your_redis_password"

backend:
  localfs_root_path: "/var/lib/callfs"

log:
  level: "info"
  format: "json"
```

### 3. Environment Variables (Alternative)
```bash
export CALLFS_SERVER_LISTEN_ADDR=":8443"
export CALLFS_SERVER_EXTERNAL_URL="https://your-domain.com:8443"
export CALLFS_AUTH_API_KEYS="your-secure-api-key-here"
export CALLFS_METADATA_STORE_DSN="postgres://callfs:secure_password@localhost/callfs?sslmode=require"
export CALLFS_DLM_REDIS_ADDR="localhost:6379"
export CALLFS_BACKEND_LOCALFS_ROOT_PATH="/var/lib/callfs"
```

## System Service Setup

### 1. Create System User
```bash
sudo useradd --system --shell /bin/false --home /var/lib/callfs callfs
sudo mkdir -p /var/lib/callfs
sudo chown callfs:callfs /var/lib/callfs
```

### 2. Create Systemd Service
```bash
sudo nano /etc/systemd/system/callfs.service
```

```ini
[Unit]
Description=CallFS REST API Filesystem
After=network.target postgresql.service redis.service
Requires=postgresql.service redis.service

[Service]
Type=simple
User=callfs
Group=callfs
WorkingDirectory=/opt/callfs
ExecStart=/opt/callfs/callfs server
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=callfs

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/callfs
CapabilityBoundingSet=

# Environment
Environment=CALLFS_BACKEND_LOCALFS_ROOT_PATH=/var/lib/callfs

[Install]
WantedBy=multi-user.target
```

### 3. Install and Start Service
```bash
# Copy binary and configuration
sudo mkdir -p /opt/callfs
sudo cp bin/callfs /opt/callfs/
sudo cp config.yaml /opt/callfs/
sudo cp server.crt server.key /opt/callfs/
sudo chown -R callfs:callfs /opt/callfs

# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable callfs
sudo systemctl start callfs

# Check status
sudo systemctl status callfs
sudo journalctl -u callfs -f
```

## Verification

### 1. Health Check
```bash
curl -k https://localhost:8443/health
# Expected: {"status":"ok"}
```

### 2. API Test
```bash
# Test authentication
curl -k -H "Authorization: Bearer your-api-key" \
  https://localhost:8443/v1/files/

# Upload test file
echo "Hello CallFS" | curl -k -X POST \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @- \
  https://localhost:8443/v1/files/test.txt

# Download test file
curl -k -H "Authorization: Bearer your-api-key" \
  https://localhost:8443/v1/files/test.txt
```

### 3. Metrics Check
```bash
curl -k https://localhost:8443/metrics | grep callfs
```

## Troubleshooting

### Common Issues

#### 1. Permission Denied Errors
```bash
# Check file permissions
ls -la /var/lib/callfs
sudo chown -R callfs:callfs /var/lib/callfs

# Check certificate permissions
ls -la server.crt server.key
sudo chmod 600 server.key
sudo chmod 644 server.crt
```

#### 2. Database Connection Issues
```bash
# Test PostgreSQL connection
psql -h localhost -U callfs -d callfs

# Check PostgreSQL status
sudo systemctl status postgresql
sudo journalctl -u postgresql -f
```

#### 3. Redis Connection Issues
```bash
# Test Redis connection
redis-cli -h localhost -p 6379 ping

# Check Redis status
sudo systemctl status redis
sudo journalctl -u redis -f
```

#### 4. TLS Certificate Issues
```bash
# Verify certificate
openssl x509 -in server.crt -text -noout

# Test TLS connection
openssl s_client -connect localhost:8443 -servername localhost
```

### Log Analysis
```bash
# View application logs
sudo journalctl -u callfs -f

# Check for specific errors
sudo journalctl -u callfs | grep ERROR

# View startup logs
sudo journalctl -u callfs --since "10 minutes ago"
```

## Security Considerations

### 1. Firewall Configuration
```bash
# Allow HTTPS traffic
sudo ufw allow 8443/tcp
sudo ufw allow 9090/tcp  # For metrics

# Deny direct database access from external
sudo ufw deny 5432/tcp
sudo ufw deny 6379/tcp
```

### 2. API Key Management
- Use strong, randomly generated API keys
- Rotate API keys regularly
- Store API keys securely (environment variables or secrets management)
- Monitor API key usage through logs and metrics

### 3. Database Security
- Use strong passwords
- Enable SSL/TLS for database connections
- Regular backups
- Monitor database access logs

### 4. File Permissions
```bash
# Secure configuration files
sudo chmod 600 /opt/callfs/config.yaml
sudo chmod 600 /opt/callfs/server.key
sudo chmod 644 /opt/callfs/server.crt
```

## Next Steps

1. Review [Configuration Reference](02-configuration.md) for advanced settings
2. Read [API Reference](03-api-reference.md) for usage details
3. Check [Authentication & Security](04-authentication-security.md) for security best practices
4. Configure [Backend Storage](05-backend-configuration.md) for your storage needs
5. Set up [Monitoring & Metrics](06-monitoring-metrics.md) for observability
