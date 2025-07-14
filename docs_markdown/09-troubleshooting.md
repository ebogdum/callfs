# Troubleshooting Guide

This guide helps diagnose and resolve common issues with CallFS deployment, configuration, and operation.

## Quick Diagnostic Checklist

### Health Check Commands
```bash
# Basic connectivity test
curl -k https://localhost:8443/health

# API authentication test
curl -k -H "Authorization: Bearer your-api-key" \
  https://localhost:8443/api/v1/files/

# Configuration validation
callfs config validate --config config.yaml

# Database connectivity test
psql "postgres://callfs:password@localhost:5432/callfs" -c "SELECT 1;"

# Redis connectivity test
redis-cli -h localhost -p 6379 ping
```

### Log Analysis
```bash
# View recent logs
journalctl -u callfs -f

# Search for errors
journalctl -u callfs | grep -i error

# Filter by time range
journalctl -u callfs --since "2024-01-01 00:00:00" --until "2024-01-01 23:59:59"

# JSON log parsing
journalctl -u callfs -o json | jq '.MESSAGE'
```

## Common Issues and Solutions

### 1. Service Won't Start

#### Symptoms
- Service fails to start
- Immediate exit after startup
- "Connection refused" errors

#### Diagnostic Commands
```bash
# Check service status
systemctl status callfs

# View startup logs
journalctl -u callfs --no-pager

# Test configuration
callfs config validate --config /etc/callfs/config.yaml

# Check port availability
netstat -tlnp | grep :8443
lsof -i :8443
```

#### Common Causes and Solutions

**Invalid Configuration:**
```bash
# Problem: YAML syntax error
# Solution: Validate YAML syntax
yamllint /etc/callfs/config.yaml

# Problem: Missing required fields
# Solution: Check required configuration
callfs config validate --config /etc/callfs/config.yaml
```

**Port Already in Use:**
```bash
# Problem: Port 8443 already bound
# Solution: Find and kill conflicting process
sudo lsof -i :8443
sudo kill -9 <PID>

# Or change port in configuration
server:
  listen_addr: ":8444"  # Use different port
```

**Certificate Issues:**
```bash
# Problem: TLS certificate not found
# Solution: Check certificate paths
ls -la /etc/callfs/certs/server.crt
ls -la /etc/callfs/certs/server.key

# Verify certificate validity
openssl x509 -in /etc/callfs/certs/server.crt -text -noout

# Generate new self-signed certificate
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt \
  -days 365 -nodes -subj "/CN=localhost"
```

**Database Connection Issues:**
```bash
# Problem: Cannot connect to PostgreSQL
# Solution: Test database connection
psql "postgres://callfs:password@localhost:5432/callfs" -c "SELECT version();"

# Check database service
systemctl status postgresql

# Verify connection parameters
PGPASSWORD=password psql -h localhost -p 5432 -U callfs -d callfs -c "\l"
```

### 2. Authentication Failures

#### Symptoms
- HTTP 401 Unauthorized responses
- "Invalid API key" errors
- Authentication middleware rejecting requests

#### Diagnostic Commands
```bash
# Test API key authentication
curl -k -v -H "Authorization: Bearer your-api-key" \
  https://localhost:8443/api/v1/files/

# Check configured API keys (be careful with logs)
grep -A5 "api_keys" /etc/callfs/config.yaml

# Test without authentication (should fail)
curl -k -v https://localhost:8443/api/v1/files/
```

#### Common Causes and Solutions

**Incorrect API Key Format:**
```bash
# Problem: Missing "Bearer " prefix
# Wrong:
curl -H "Authorization: your-api-key" ...

# Correct:
curl -H "Authorization: Bearer your-api-key" ...
```

**API Key Not Configured:**
```yaml
# Problem: Empty or missing API keys
auth:
  api_keys: []  # Empty list

# Solution: Add valid API keys
auth:
  api_keys:
    - "your-secure-api-key-here"
    - "another-api-key"
```

**Environment Variable Override:**
```bash
# Problem: Environment variable overriding config
export CALLFS_AUTH_API_KEYS="wrong-key"

# Solution: Check environment variables
env | grep CALLFS_AUTH

# Unset problematic variables
unset CALLFS_AUTH_API_KEYS
```

### 3. File Upload/Download Issues

#### Symptoms
- Upload failures or timeouts
- Download corruption or incomplete transfers
- "File not found" errors for existing files

#### Diagnostic Commands
```bash
# Test small file upload
echo "test content" | curl -k -X PUT \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: text/plain" \
  --data-binary @- \
  https://localhost:8443/api/v1/files/test.txt

# Test file download
curl -k -H "Authorization: Bearer your-api-key" \
  https://localhost:8443/api/v1/files/test.txt

# Check file permissions (LocalFS backend)
ls -la /var/lib/callfs/

# Check disk space
df -h /var/lib/callfs/
```

#### Common Causes and Solutions

**Insufficient Disk Space:**
```bash
# Problem: No space left on device
# Solution: Clean up disk space
df -h
du -sh /var/lib/callfs/*

# Clean temporary files
find /tmp -name "callfs-*" -mtime +1 -delete

# Rotate logs if needed
journalctl --vacuum-time=7d
```

**File Permission Issues:**
```bash
# Problem: Permission denied accessing files
# Solution: Fix file permissions
sudo chown -R callfs:callfs /var/lib/callfs/
sudo chmod -R 755 /var/lib/callfs/

# Check SELinux context (if applicable)
ls -Z /var/lib/callfs/
sudo restorecon -R /var/lib/callfs/
```

**Large File Upload Issues:**
```yaml
# Problem: Large uploads failing
# Solution: Adjust client timeout and server limits
server:
  read_timeout: 300s   # 5 minutes for large uploads
  write_timeout: 300s
  
# Check reverse proxy limits (nginx example)
client_max_body_size 10G;
proxy_read_timeout 300s;
proxy_send_timeout 300s;
```

**Backend Storage Issues:**
```bash
# S3 Backend troubleshooting
aws s3 ls s3://your-bucket-name/

# Check S3 credentials
aws sts get-caller-identity

# Test S3 connectivity
aws s3api head-bucket --bucket your-bucket-name
```

### 4. Database Connection Issues

#### Symptoms
- "Connection refused" to PostgreSQL
- "Too many connections" errors
- Slow query performance

#### Diagnostic Commands
```bash
# Test database connection
psql "postgres://callfs:password@localhost:5432/callfs" -c "SELECT 1;"

# Check active connections
psql "postgres://callfs:password@localhost:5432/callfs" -c \
  "SELECT count(*) FROM pg_stat_activity WHERE datname='callfs';"

# Monitor database performance
psql "postgres://callfs:password@localhost:5432/callfs" -c \
  "SELECT query, query_start, state FROM pg_stat_activity WHERE datname='callfs';"

# Check database size
psql "postgres://callfs:password@localhost:5432/callfs" -c \
  "SELECT pg_size_pretty(pg_database_size('callfs'));"
```

#### Common Causes and Solutions

**Connection Pool Exhausted:**
```yaml
# Problem: Too many connections
metadata_store:
  max_open_conns: 100   # Reduce if needed
  max_idle_conns: 10
  conn_max_lifetime: 5m

# PostgreSQL configuration
max_connections = 200
```

**Database Migration Issues:**
```bash
# Problem: Schema out of date
# Solution: Check migration status
migrate -database "postgres://callfs:password@localhost:5432/callfs?sslmode=disable" \
  -path metadata/schema version

# Apply missing migrations
migrate -database "postgres://callfs:password@localhost:5432/callfs?sslmode=disable" \
  -path metadata/schema up
```

**Slow Query Performance:**
```sql
-- Problem: Missing indexes
-- Solution: Check for missing indexes
EXPLAIN ANALYZE SELECT * FROM files WHERE path = '/some/path';

-- Create index if needed
CREATE INDEX idx_files_path ON files(path);

-- Update table statistics
ANALYZE files;
```

### 5. Redis/Distributed Locking Issues

#### Symptoms
- "Failed to acquire lock" errors
- Deadlock situations
- Redis connection timeouts

#### Diagnostic Commands
```bash
# Test Redis connectivity
redis-cli -h localhost -p 6379 ping

# Check Redis memory usage
redis-cli -h localhost -p 6379 info memory

# Monitor Redis commands
redis-cli -h localhost -p 6379 monitor

# Check active locks
redis-cli -h localhost -p 6379 keys "callfs:lock:*"
```

#### Common Causes and Solutions

**Redis Connection Issues:**
```yaml
# Problem: Redis connection timeout
dlm:
  redis_addr: "localhost:6379"
  redis_password: "your-password"
  dial_timeout: 10s
  read_timeout: 5s
  write_timeout: 5s
```

**Lock Timeout Issues:**
```yaml
# Problem: Lock acquisition timeout
dlm:
  lock_timeout: 60s     # Increase if operations take longer
  retry_delay: 100ms
  max_retries: 10
```

**Orphaned Locks:**
```bash
# Problem: Stale locks blocking operations
# Solution: Clear specific locks
redis-cli -h localhost -p 6379 del "callfs:lock:/path/to/file"

# Clear all locks (emergency only)
redis-cli -h localhost -p 6379 eval "return redis.call('del', unpack(redis.call('keys', 'callfs:lock:*')))" 0
```

### 6. Performance Issues

#### Symptoms
- Slow response times
- High CPU or memory usage
- Request timeouts

#### Diagnostic Commands
```bash
# Monitor system resources
htop
iostat 1
sar 1

# Check CallFS metrics
curl -k https://localhost:8443/metrics

# Profile CPU usage
go tool pprof http://localhost:8443/debug/pprof/profile

# Profile memory usage
go tool pprof http://localhost:8443/debug/pprof/heap

# Check goroutines
go tool pprof http://localhost:8443/debug/pprof/goroutine
```

#### Common Causes and Solutions

**High Memory Usage:**
```bash
# Problem: Memory leaks or excessive allocation
# Solution: Analyze memory profile
go tool pprof -http=:8080 http://localhost:8443/debug/pprof/heap

# Reduce memory usage with caching configuration
cache:
  ttl: 5m              # Reduce cache TTL
  cleanup_interval: 2m # More frequent cleanup
```

**Database Performance:**
```sql
-- Problem: Slow queries
-- Solution: Identify slow queries
SELECT query, mean_time, calls 
FROM pg_stat_statements 
ORDER BY mean_time DESC 
LIMIT 10;

-- Add appropriate indexes
CREATE INDEX idx_files_created_at ON files(created_at);
CREATE INDEX idx_files_size ON files(size);
```

**File System Performance:**
```bash
# Problem: Slow file I/O
# Solution: Check disk performance
iostat -x 1

# Check for disk errors
dmesg | grep -i error

# Consider using faster storage or SSD
# Mount options for better performance
mount -o noatime,nodiratime /dev/sdb1 /var/lib/callfs
```

## Advanced Debugging Techniques

### 1. Tracing and Profiling

#### Enable Debug Logging
```yaml
# Configuration for detailed logging
log:
  level: debug
  format: json
  file: "/var/log/callfs/debug.log"
```

#### HTTP Request Tracing
```bash
# Use verbose curl for request analysis
curl -k -v -H "Authorization: Bearer your-api-key" \
  -H "X-Trace-Id: debug-$(date +%s)" \
  https://localhost:8443/api/v1/files/

# Capture network traffic
sudo tcpdump -i any port 8443 -w callfs-traffic.pcap

# Analyze with tshark
tshark -r callfs-traffic.pcap -Y "http"
```

#### Application Profiling
```bash
# CPU profiling
go tool pprof -http=:8080 http://localhost:8443/debug/pprof/profile?seconds=30

# Memory profiling
go tool pprof -http=:8080 http://localhost:8443/debug/pprof/heap

# Goroutine analysis
go tool pprof -http=:8080 http://localhost:8443/debug/pprof/goroutine

# Blocking operations
go tool pprof -http=:8080 http://localhost:8443/debug/pprof/block
```

### 2. Database Query Analysis

#### Slow Query Investigation
```sql
-- Enable query logging
SET log_statement = 'all';
SET log_duration = on;
SET log_min_duration_statement = 100; -- Log queries > 100ms

-- Analyze query plans
EXPLAIN (ANALYZE, BUFFERS) 
SELECT * FROM files 
WHERE path LIKE '/uploads/%' 
ORDER BY created_at DESC 
LIMIT 100;

-- Check for missing indexes
SELECT schemaname, tablename, attname, n_distinct, correlation 
FROM pg_stats 
WHERE tablename = 'files';
```

#### Connection Pool Monitoring
```sql
-- Monitor connection usage
SELECT 
    application_name,
    state,
    count(*) as connection_count
FROM pg_stat_activity 
WHERE datname = 'callfs'
GROUP BY application_name, state;

-- Check for long-running transactions
SELECT 
    pid,
    now() - pg_stat_activity.query_start AS duration,
    query 
FROM pg_stat_activity 
WHERE (now() - pg_stat_activity.query_start) > interval '5 minutes';
```

### 3. Network Troubleshooting

#### TLS/SSL Issues
```bash
# Test TLS connection
openssl s_client -connect localhost:8443 -servername localhost

# Check certificate chain
openssl x509 -in /etc/callfs/certs/server.crt -text -noout

# Verify private key matches certificate
openssl x509 -noout -modulus -in server.crt | openssl md5
openssl rsa -noout -modulus -in server.key | openssl md5
```

#### Load Balancer Debugging
```bash
# Test direct backend connection
curl -k -H "Host: your-domain.com" \
  -H "Authorization: Bearer your-api-key" \
  https://backend-server:8443/health

# Check load balancer logs
tail -f /var/log/haproxy.log
tail -f /var/log/nginx/access.log
tail -f /var/log/nginx/error.log
```

### 4. Container and Orchestration Issues

#### Docker Troubleshooting
```bash
# Check container logs
docker logs callfs-container

# Execute shell in container
docker exec -it callfs-container /bin/sh

# Check container resource usage
docker stats callfs-container

# Inspect container configuration
docker inspect callfs-container
```

#### Kubernetes Debugging
```bash
# Check pod status
kubectl get pods -n callfs

# View pod logs
kubectl logs -n callfs deployment/callfs -f

# Describe pod for events
kubectl describe pod -n callfs <pod-name>

# Execute shell in pod
kubectl exec -n callfs -it <pod-name> -- /bin/sh

# Check service endpoints
kubectl get endpoints -n callfs

# Test service connectivity
kubectl port-forward -n callfs service/callfs-service 8443:8443
```

## Monitoring and Alerting

### 1. Key Metrics to Monitor

#### Application Metrics
```promql
# Request rate
rate(callfs_http_requests_total[5m])

# Error rate
rate(callfs_http_requests_total{status=~"5.."}[5m]) / 
rate(callfs_http_requests_total[5m])

# Response time percentiles
histogram_quantile(0.95, 
  rate(callfs_http_request_duration_seconds_bucket[5m]))

# File operation metrics
rate(callfs_file_operations_total[5m])

# Backend availability
callfs_backend_availability
```

#### System Metrics
```promql
# CPU usage
100 - (avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)

# Memory usage
(1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)) * 100

# Disk usage
(1 - (node_filesystem_avail_bytes / node_filesystem_size_bytes)) * 100

# Network I/O
rate(node_network_receive_bytes_total[5m])
rate(node_network_transmit_bytes_total[5m])
```

### 2. Alert Rules

#### Critical Alerts
```yaml
# alerts.yml
groups:
- name: callfs-critical
  rules:
  - alert: CallFSDown
    expr: up{job="callfs"} == 0
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "CallFS instance is down"
      description: "CallFS instance {{ $labels.instance }} has been down for more than 1 minute"

  - alert: HighErrorRate
    expr: rate(callfs_http_requests_total{status=~"5.."}[5m]) / rate(callfs_http_requests_total[5m]) > 0.1
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "High error rate detected"
      description: "Error rate is {{ $value | humanizePercentage }} for instance {{ $labels.instance }}"

  - alert: DatabaseConnectionFailed
    expr: callfs_database_connections_active == 0
    for: 2m
    labels:
      severity: critical
    annotations:
      summary: "Database connection failed"
      description: "No active database connections for instance {{ $labels.instance }}"
```

#### Warning Alerts
```yaml
- name: callfs-warning
  rules:
  - alert: HighResponseTime
    expr: histogram_quantile(0.95, rate(callfs_http_request_duration_seconds_bucket[5m])) > 2
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "High response times detected"
      description: "95th percentile response time is {{ $value }}s for instance {{ $labels.instance }}"

  - alert: HighDiskUsage
    expr: (1 - (node_filesystem_avail_bytes{mountpoint="/var/lib/callfs"} / node_filesystem_size_bytes{mountpoint="/var/lib/callfs"})) * 100 > 80
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "High disk usage"
      description: "Disk usage is {{ $value | humanizePercentage }} on {{ $labels.instance }}"
```

### 3. Health Check Endpoints

#### Comprehensive Health Check
```go
// Custom health check implementation
type HealthChecker struct {
    db    *sql.DB
    redis *redis.Client
    backends []Backend
}

func (h *HealthChecker) Check(ctx context.Context) map[string]interface{} {
    result := map[string]interface{}{
        "status": "ok",
        "timestamp": time.Now().UTC(),
        "checks": map[string]interface{}{},
    }
    
    // Database check
    if err := h.db.PingContext(ctx); err != nil {
        result["checks"]["database"] = map[string]interface{}{
            "status": "error",
            "error": err.Error(),
        }
        result["status"] = "error"
    } else {
        result["checks"]["database"] = map[string]interface{}{"status": "ok"}
    }
    
    // Redis check
    if err := h.redis.Ping(ctx).Err(); err != nil {
        result["checks"]["redis"] = map[string]interface{}{
            "status": "error",
            "error": err.Error(),
        }
        result["status"] = "error"
    } else {
        result["checks"]["redis"] = map[string]interface{}{"status": "ok"}
    }
    
    // Backend checks
    for _, backend := range h.backends {
        if err := backend.HealthCheck(ctx); err != nil {
            result["checks"][backend.Name()] = map[string]interface{}{
                "status": "error",
                "error": err.Error(),
            }
            result["status"] = "error"
        } else {
            result["checks"][backend.Name()] = map[string]interface{}{"status": "ok"}
        }
    }
    
    return result
}
```

## Recovery Procedures

### 1. Service Recovery

#### Automatic Recovery
```bash
#!/bin/bash
# recovery.sh - Automatic service recovery script

SERVICE="callfs"
MAX_ATTEMPTS=3
ATTEMPT=0

while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
    if systemctl is-active --quiet $SERVICE; then
        echo "Service $SERVICE is running"
        exit 0
    fi
    
    echo "Attempt $((ATTEMPT + 1)): Starting $SERVICE"
    systemctl start $SERVICE
    sleep 10
    
    ATTEMPT=$((ATTEMPT + 1))
done

echo "Failed to start $SERVICE after $MAX_ATTEMPTS attempts"
exit 1
```

#### Manual Recovery Steps
```bash
# 1. Stop the service
systemctl stop callfs

# 2. Check for stale processes
ps aux | grep callfs
pkill -f callfs

# 3. Clear any stale locks
redis-cli eval "return redis.call('del', unpack(redis.call('keys', 'callfs:lock:*')))" 0

# 4. Validate configuration
callfs config validate --config /etc/callfs/config.yaml

# 5. Start the service
systemctl start callfs

# 6. Verify service is healthy
curl -k https://localhost:8443/health
```

### 2. Database Recovery

#### Backup and Restore
```bash
# Create backup
pg_dump "postgres://callfs:password@localhost:5432/callfs" > callfs_backup.sql

# Restore from backup
psql "postgres://callfs:password@localhost:5432/callfs" < callfs_backup.sql

# Point-in-time recovery (if WAL archiving enabled)
pg_basebackup -h localhost -D /var/lib/postgresql/backup -U callfs -P -W
```

#### Migration Recovery
```bash
# Check current migration version
migrate -database "postgres://callfs:password@localhost:5432/callfs?sslmode=disable" \
  -path metadata/schema version

# Rollback to previous version
migrate -database "postgres://callfs:password@localhost:5432/callfs?sslmode=disable" \
  -path metadata/schema down 1

# Force migration version (emergency only)
migrate -database "postgres://callfs:password@localhost:5432/callfs?sslmode=disable" \
  -path metadata/schema force 1
```

### 3. Data Recovery

#### File System Recovery
```bash
# Check file system integrity
fsck -y /dev/sdb1

# Recover deleted files (ext4)
extundelete /dev/sdb1 --restore-directory /var/lib/callfs

# Restore from backup
rsync -av /backup/callfs/ /var/lib/callfs/
```

#### S3 Recovery
```bash
# Enable versioning (preventive)
aws s3api put-bucket-versioning \
  --bucket your-bucket \
  --versioning-configuration Status=Enabled

# Restore deleted objects
aws s3api list-object-versions --bucket your-bucket --prefix path/
aws s3api restore-object \
  --bucket your-bucket \
  --key path/file.txt \
  --version-id version-id
```

## Getting Help

### 1. Log Collection Script
```bash
#!/bin/bash
# collect-logs.sh - Collect diagnostic information

OUTPUT_DIR="callfs-diagnostics-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$OUTPUT_DIR"

echo "Collecting CallFS diagnostic information..."

# System information
uname -a > "$OUTPUT_DIR/system-info.txt"
free -h > "$OUTPUT_DIR/memory-info.txt"
df -h > "$OUTPUT_DIR/disk-info.txt"

# Service status
systemctl status callfs > "$OUTPUT_DIR/service-status.txt" 2>&1

# Configuration (sensitive data redacted)
cp /etc/callfs/config.yaml "$OUTPUT_DIR/config.yaml"
sed -i 's/password: .*/password: [REDACTED]/g' "$OUTPUT_DIR/config.yaml"
sed -i 's/api_keys:.*/api_keys: [REDACTED]/g' "$OUTPUT_DIR/config.yaml"

# Recent logs
journalctl -u callfs --since "24 hours ago" > "$OUTPUT_DIR/service-logs.txt"

# Database connection test
psql "postgres://callfs:password@localhost:5432/callfs" \
  -c "SELECT version();" > "$OUTPUT_DIR/db-connection.txt" 2>&1

# Redis connection test
redis-cli ping > "$OUTPUT_DIR/redis-connection.txt" 2>&1

# Network connectivity
curl -k -I https://localhost:8443/health > "$OUTPUT_DIR/health-check.txt" 2>&1

# Create archive
tar -czf "$OUTPUT_DIR.tar.gz" "$OUTPUT_DIR"
rm -rf "$OUTPUT_DIR"

echo "Diagnostic information collected in $OUTPUT_DIR.tar.gz"
```

### 2. Support Information

When reporting issues, please include:

1. **Environment details:**
   - CallFS version
   - Operating system and version
   - Database version (PostgreSQL)
   - Redis version
   - Deployment method (Docker, Kubernetes, systemd)

2. **Configuration:**
   - Anonymized configuration file
   - Environment variables (non-sensitive)

3. **Symptoms:**
   - Error messages
   - Log entries
   - Screenshots if applicable

4. **Reproduction steps:**
   - Steps to reproduce the issue
   - Expected vs actual behavior

5. **Diagnostic output:**
   - Output from diagnostic commands
   - Log collection script results

### 3. Community Resources

- **GitHub Issues**: https://github.com/yourusername/callfs/issues
- **Documentation**: https://callfs.readthedocs.io
- **Community Forum**: https://community.callfs.io
- **Security Issues**: security@callfs.io

This troubleshooting guide covers the most common issues and provides systematic approaches to diagnosis and resolution. For complex issues, the diagnostic tools and recovery procedures ensure minimal downtime and data loss.
