# Monitoring & Metrics

This document covers comprehensive monitoring, metrics collection, alerting, and observability for CallFS deployments.

## Overview

CallFS provides extensive observability through:
- **Prometheus Metrics**: Detailed performance and operational metrics
- **Structured Logging**: JSON-formatted logs with contextual information
- **Health Checks**: HTTP endpoints for service health verification
- **Distributed Tracing**: Request tracing across components (future)

## Prometheus Metrics

### Metrics Endpoint

CallFS exposes Prometheus metrics at `/metrics` endpoint (no authentication required):

```bash
curl https://localhost:8443/metrics
```

### HTTP Request Metrics

#### `callfs_http_requests_total`
Counter of total HTTP requests by method, path, and status code.

**Labels:**
- `method`: HTTP method (GET, POST, PUT, DELETE, HEAD)
- `path`: Request path pattern (/v1/files/*, /v1/directories/*, /v1/links/*, etc.)
- `status_code`: HTTP response status code

**Example:**
```prometheus
callfs_http_requests_total{method="GET",path="/v1/files/*",status_code="200"} 1542
callfs_http_requests_total{method="POST",path="/v1/files/*",status_code="201"} 328
callfs_http_requests_total{method="POST",path="/v1/files/*",status_code="409"} 45
callfs_http_requests_total{method="GET",path="/health",status_code="200"} 3600
callfs_http_requests_total{method="GET",path="/v1/directories/*",status_code="200"} 892
```

#### `callfs_http_request_duration_seconds`
Histogram of HTTP request duration in seconds.

**Labels:**
- `method`: HTTP method
- `path`: Request path pattern

**Buckets:** Default Prometheus buckets (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10)

### Backend Operation Metrics

#### `callfs_backend_ops_total`
Counter of backend operations by type and operation.

**Labels:**
- `backend_type`: Backend type (localfs, s3, internalproxy, noop)
- `operation`: Operation type (open, create, update, delete, list, stat, create_directory)

**Example:**
```prometheus
callfs_backend_ops_total{backend_type="localfs",operation="open"} 856
callfs_backend_ops_total{backend_type="s3",operation="create"} 245
callfs_backend_ops_total{backend_type="internalproxy",operation="open"} 123
callfs_backend_ops_total{backend_type="localfs",operation="list"} 342
```

#### `callfs_backend_op_duration_seconds`
Histogram of backend operation duration in seconds.

**Labels:**
- `backend_type`: Backend type
- `operation`: Operation type

### Metadata Database Metrics

#### `callfs_metadata_db_queries_total`
Counter of metadata database queries by operation.

**Labels:**
- `operation`: Database operation (get_metadata, create_metadata, update_metadata, delete_metadata, list_directory, create_link, etc.)

#### `callfs_metadata_db_query_duration_seconds`
Histogram of database query duration in seconds.

**Labels:**
- `operation`: Database operation type

### Single-Use Link Metrics

#### `callfs_single_use_link_generations_total`
Counter of single-use links generated (with rate limiting tracking).

#### `callfs_single_use_link_consumptions_total`
Counter of single-use link consumption attempts.

**Labels:**
- `status`: Consumption status (success, expired, invalid, not_found)

### Lock Manager Metrics

#### `callfs_lock_operations_total`
Counter of distributed lock operations.

**Labels:**
- `operation`: Lock operation (acquire, release)
- `status`: Operation status (success, failure)

#### `callfs_lock_operation_duration_seconds`
Histogram of lock operation duration in seconds.

**Labels:**
- `operation`: Lock operation type

#### `callfs_active_locks`
Gauge of currently active distributed locks.

### Cache Metrics

#### `callfs_cache_operations_total`
Counter of metadata cache operations.

**Labels:**
- `operation`: Cache operation (hit, miss, eviction, update)
- `cache_type`: Type of cache (metadata)

#### `callfs_cache_size`
Gauge of current cache size (number of entries).

**Labels:**
- `cache_type`: Type of cache (metadata)

### Cross-Server Metrics

#### `callfs_cross_server_operations_total`
Counter of cross-server operations (conflict detection, proxying).

**Labels:**
- `operation`: Operation type (conflict_detected, proxy_success, proxy_failure)
- `source_instance`: Source instance ID
- `target_instance`: Target instance ID (for proxying)

#### `callfs_cross_server_operation_duration_seconds`
Histogram of cross-server operation duration.

**Labels:**
- `operation`: Operation type
- `target_instance`: Target instance ID

## Monitoring Setup

### Prometheus Configuration

#### Basic Prometheus Config
```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  - "callfs_rules.yml"

scrape_configs:
  - job_name: 'callfs'
    static_configs:
      - targets: ['localhost:8443']
    metrics_path: '/metrics'
    scheme: 'https'
    tls_config:
      insecure_skip_verify: true
    scrape_interval: 10s
    scrape_timeout: 5s

alerting:
  alertmanagers:
    - static_configs:
        - targets:
          - alertmanager:9093
```

#### Service Discovery
```yaml
# For Kubernetes
- job_name: 'callfs-k8s'
  kubernetes_sd_configs:
    - role: pod
  relabel_configs:
    - source_labels: [__meta_kubernetes_pod_label_app]
      action: keep
      regex: callfs
    - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
      action: keep
      regex: true
    - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_port]
      action: replace
      target_label: __address__
      regex: ([^:]+)(?::\d+)?;(\d+)
      replacement: $1:$2

# For Docker Swarm
- job_name: 'callfs-swarm'
  dockerswarm_sd_configs:
    - host: unix:///var/run/docker.sock
      role: tasks
  relabel_configs:
    - source_labels: [__meta_dockerswarm_service_label_app]
      action: keep
      regex: callfs
```

### Grafana Dashboards

#### CallFS Overview Dashboard

**Dashboard JSON:**
```json
{
  "dashboard": {
    "title": "CallFS Overview",
    "panels": [
      {
        "title": "Request Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(callfs_http_requests_total[5m])",
            "legendFormat": "{{method}} {{path}} {{status_code}}"
          }
        ]
      },
      {
        "title": "Request Duration",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(callfs_http_request_duration_seconds_bucket[5m]))",
            "legendFormat": "95th percentile"
          },
          {
            "expr": "histogram_quantile(0.50, rate(callfs_http_request_duration_seconds_bucket[5m]))",
            "legendFormat": "50th percentile"
          }
        ]
      },
      {
        "title": "Backend Operations",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(callfs_backend_ops_total[5m])",
            "legendFormat": "{{backend_type}} {{operation}}"
          }
        ]
      },
      {
        "title": "Error Rate",
        "type": "singlestat",
        "targets": [
          {
            "expr": "rate(callfs_http_requests_total{status_code=~\"4..|5..\"}[5m]) / rate(callfs_http_requests_total[5m]) * 100"
          }
        ]
      }
    ]
  }
}
```

#### Backend Performance Dashboard
```json
{
  "dashboard": {
    "title": "CallFS Backend Performance",
    "panels": [
      {
        "title": "Backend Operation Duration",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(callfs_backend_op_duration_seconds_bucket[5m]))",
            "legendFormat": "{{backend_type}} {{operation}} 95th"
          }
        ]
      },
      {
        "title": "Database Query Performance",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(callfs_metadata_db_queries_total[5m])",
            "legendFormat": "{{operation}}"
          }
        ]
      },
      {
        "title": "Cache Hit Rate",
        "type": "singlestat",
        "targets": [
          {
            "expr": "rate(callfs_cache_operations_total{operation=\"hit\"}[5m]) / (rate(callfs_cache_operations_total{operation=\"hit\"}[5m]) + rate(callfs_cache_operations_total{operation=\"miss\"}[5m])) * 100"
          }
        ]
      }
    ]
  }
}
```

### Key Metrics Queries

#### Performance Monitoring
```prometheus
# Request rate
rate(callfs_http_requests_total[5m])

# Error rate
rate(callfs_http_requests_total{status_code=~"4..|5.."}[5m]) / rate(callfs_http_requests_total[5m])

# 95th percentile response time
histogram_quantile(0.95, rate(callfs_http_request_duration_seconds_bucket[5m]))

# Backend operation rate
rate(callfs_backend_ops_total[5m])

# Database query rate
rate(callfs_metadata_db_queries_total[5m])

# Cache hit rate
rate(callfs_cache_operations_total{operation="hit"}[5m]) / (
  rate(callfs_cache_operations_total{operation="hit"}[5m]) + 
  rate(callfs_cache_operations_total{operation="miss"}[5m])
)
```

#### Resource Utilization
```prometheus
# Active locks
callfs_active_locks

# Cache size
callfs_cache_size

# Lock operation failures
rate(callfs_lock_operations_total{status="failure"}[5m])

# Single-use link generation rate
rate(callfs_single_use_link_generations_total[5m])
```

## Alerting Rules

### Prometheus Alerting Rules

```yaml
# callfs_rules.yml
groups:
  - name: callfs
    rules:
      - alert: CallFSHighErrorRate
        expr: rate(callfs_http_requests_total{status_code=~"5.."}[5m]) / rate(callfs_http_requests_total[5m]) > 0.05
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "High error rate in CallFS"
          description: "CallFS error rate is {{ $value | humanizePercentage }} over the last 5 minutes"

      - alert: CallFSHighLatency
        expr: histogram_quantile(0.95, rate(callfs_http_request_duration_seconds_bucket[5m])) > 5
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High latency in CallFS"
          description: "CallFS 95th percentile latency is {{ $value }}s over the last 5 minutes"

      - alert: CallFSBackendFailures
        expr: rate(callfs_backend_ops_total{backend_type!="noop"}[5m]) == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "CallFS backend not responding"
          description: "No successful operations on {{ $labels.backend_type }} backend for 5 minutes"

      - alert: CallFSDatabaseSlowQueries
        expr: histogram_quantile(0.95, rate(callfs_metadata_db_query_duration_seconds_bucket[5m])) > 1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "Slow database queries in CallFS"
          description: "95th percentile database query time is {{ $value }}s"

      - alert: CallFSLockManagerFailures
        expr: rate(callfs_lock_operations_total{status="failure"}[5m]) > 0.1
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "CallFS lock manager failures"
          description: "High rate of lock operation failures: {{ $value }}/s"

      - alert: CallFSCacheLowHitRate
        expr: rate(callfs_cache_operations_total{operation="hit"}[10m]) / (rate(callfs_cache_operations_total{operation="hit"}[10m]) + rate(callfs_cache_operations_total{operation="miss"}[10m])) < 0.7
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Low cache hit rate in CallFS"
          description: "Cache hit rate is {{ $value | humanizePercentage }} over the last 10 minutes"

      - alert: CallFSHighMemoryUsage
        expr: process_resident_memory_bytes / 1024 / 1024 > 1000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High memory usage in CallFS"
          description: "CallFS process is using {{ $value }}MB of memory"

      - alert: CallFSServiceDown
        expr: up{job="callfs"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "CallFS service is down"
          description: "CallFS instance {{ $labels.instance }} is not responding"
```

### Alertmanager Configuration

```yaml
# alertmanager.yml
global:
  smtp_smarthost: 'localhost:587'
  smtp_from: 'alerts@yourdomain.com'

route:
  group_by: ['alertname']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 1h
  receiver: 'web.hook'

receivers:
  - name: 'web.hook'
    email_configs:
      - to: 'admin@yourdomain.com'
        subject: 'CallFS Alert: {{ .GroupLabels.alertname }}'
        body: |
          {{ range .Alerts }}
          Alert: {{ .Annotations.summary }}
          Description: {{ .Annotations.description }}
          {{ end }}
    
    slack_configs:
      - api_url: 'YOUR_SLACK_WEBHOOK_URL'
        channel: '#alerts'
        title: 'CallFS Alert'
        text: |
          {{ range .Alerts }}
          {{ .Annotations.summary }}
          {{ .Annotations.description }}
          {{ end }}
    
    pagerduty_configs:
      - routing_key: 'YOUR_PAGERDUTY_INTEGRATION_KEY'
        description: '{{ .GroupLabels.alertname }}'
```

## Logging

### Log Configuration

CallFS supports structured logging in JSON or console format:

```yaml
log:
  level: "info"    # debug, info, warn, error
  format: "json"   # json, console
```

### Log Levels

- **debug**: Detailed debugging information
- **info**: General operational messages
- **warn**: Warning conditions
- **error**: Error conditions requiring attention

### Log Format

#### JSON Format (Production)
```json
{
  "level": "info",
  "time": "2025-07-13T12:00:00Z",
  "msg": "HTTP request",
  "method": "GET",
  "path": "/files/document.pdf",
  "status": 200,
  "duration": 0.025,
  "user_agent": "curl/7.68.0",
  "remote_addr": "192.168.1.100",
  "trace_id": "abc123"
}
```

#### Console Format (Development)
```
2025-07-13T12:00:00Z    INFO    HTTP request    method=GET path=/files/document.pdf status=200 duration=25ms
```

### Log Categories

#### HTTP Access Logs
```json
{
  "level": "info",
  "time": "2025-07-13T12:00:00Z",
  "msg": "HTTP request",
  "method": "GET",
  "path": "/files/example.txt",
  "status": 200,
  "duration": 0.025,
  "user_agent": "curl/7.68.0",
  "remote_addr": "192.168.1.100"
}
```

#### Authentication Logs
```json
{
  "level": "warn",
  "time": "2025-07-13T12:00:00Z",
  "msg": "Authentication failed",
  "remote_addr": "192.168.1.100",
  "user_agent": "malicious-bot/1.0",
  "error": "invalid API key"
}
```

#### Backend Operation Logs
```json
{
  "level": "info",
  "time": "2025-07-13T12:00:00Z",
  "msg": "Backend operation",
  "backend_type": "s3",
  "operation": "write",
  "path": "/documents/report.pdf",
  "duration": 0.150,
  "bytes": 1048576
}
```

#### Error Logs
```json
{
  "level": "error",
  "time": "2025-07-13T12:00:00Z",
  "msg": "Database connection failed",
  "error": "connection timeout",
  "dsn": "postgres://callfs:***@localhost/callfs",
  "retry_count": 3
}
```

### Log Aggregation

#### ELK Stack (Elasticsearch, Logstash, Kibana)

**Logstash Configuration:**
```ruby
# callfs.conf
input {
  file {
    path => "/var/log/callfs/callfs.log"
    codec => "json"
  }
}

filter {
  if [level] == "error" {
    mutate {
      add_tag => ["error"]
    }
  }
  
  if [path] {
    grok {
      match => { "path" => "^/(?<path_category>[^/]+)" }
    }
  }
}

output {
  elasticsearch {
    hosts => ["elasticsearch:9200"]
    index => "callfs-logs-%{+YYYY.MM.dd}"
  }
}
```

#### Fluentd Configuration
```ruby
# fluent.conf
<source>
  @type tail
  path /var/log/callfs/callfs.log
  pos_file /var/log/fluentd/callfs.log.pos
  tag callfs
  format json
  time_key time
  time_format %Y-%m-%dT%H:%M:%S%z
</source>

<match callfs.**>
  @type elasticsearch
  host elasticsearch
  port 9200
  index_name callfs-logs
  type_name _doc
  flush_interval 10s
</match>
```

#### Vector Configuration
```toml
# vector.toml
[sources.callfs_logs]
type = "file"
includes = ["/var/log/callfs/callfs.log"]
read_from = "beginning"

[transforms.parse_json]
type = "json_parser"
inputs = ["callfs_logs"]

[sinks.elasticsearch]
type = "elasticsearch"
inputs = ["parse_json"]
endpoints = ["http://elasticsearch:9200"]
index = "callfs-logs-%Y.%m.%d"
```

## Health Checks

### HTTP Health Endpoint

CallFS provides a health check endpoint at `/health`:

```bash
curl https://localhost:8443/health
# Response: {"status":"ok"}
```

### Advanced Health Checks

#### Kubernetes Liveness Probe
```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8443
    scheme: HTTPS
  initialDelaySeconds: 30
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3
```

#### Kubernetes Readiness Probe
```yaml
readinessProbe:
  httpGet:
    path: /health
    port: 8443
    scheme: HTTPS
  initialDelaySeconds: 5
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 2
```

#### Docker Health Check
```dockerfile
HEALTHCHECK --interval=30s --timeout=3s --start-period=30s --retries=3 \
  CMD curl -f https://localhost:8443/health || exit 1
```

### Custom Health Checks

#### Database Connectivity Check
```bash
#!/bin/bash
# check-db.sh
psql -h localhost -U callfs -d callfs -c "SELECT 1;" > /dev/null 2>&1
echo $?
```

#### Redis Connectivity Check
```bash
#!/bin/bash
# check-redis.sh
redis-cli -h localhost -p 6379 ping > /dev/null 2>&1
echo $?
```

#### Backend Storage Check
```bash
#!/bin/bash
# check-storage.sh
# Check local filesystem
if [ -w "/var/lib/callfs" ]; then
  echo "LocalFS: OK"
else
  echo "LocalFS: FAIL"
  exit 1
fi

# Check S3 connectivity
aws s3 ls s3://callfs-storage > /dev/null 2>&1
if [ $? -eq 0 ]; then
  echo "S3: OK"
else
  echo "S3: FAIL"
  exit 1
fi
```

## Performance Monitoring

### System Metrics

#### CPU and Memory
```bash
# Monitor CPU usage
top -p $(pgrep callfs)

# Monitor memory usage
ps -o pid,vsz,rss,comm -p $(pgrep callfs)

# Continuous monitoring
while true; do
  ps -o pid,pcpu,pmem,vsz,rss,comm -p $(pgrep callfs)
  sleep 5
done
```

#### Disk I/O
```bash
# Monitor disk I/O
iostat -x 1

# Monitor specific device
iostat -x /dev/sda 1

# Monitor CallFS data directory
iotop -p $(pgrep callfs)
```

#### Network I/O
```bash
# Monitor network connections
netstat -tulpn | grep callfs

# Monitor network traffic
iftop -i eth0

# Monitor specific port
ss -tulpn | grep :8443
```

### Application Metrics

#### Go Runtime Metrics
```prometheus
# Goroutines
go_goroutines

# Memory usage
go_memstats_alloc_bytes
go_memstats_heap_inuse_bytes

# GC stats
go_gc_duration_seconds
go_memstats_gc_cpu_fraction
```

#### HTTP Connection Pool
```prometheus
# Active connections
promhttp_metric_handler_requests_in_flight

# Connection pool metrics (custom)
callfs_http_client_connections_active
callfs_http_client_connections_idle
```

### Performance Testing

#### Load Testing with wrk
```bash
# Basic load test
wrk -t4 -c100 -d30s --header "Authorization: Bearer api-key" \
  https://localhost:8443/v1/files/test.txt

# Upload test
wrk -t4 -c100 -d30s -s upload.lua \
  https://localhost:8443/v1/files/
```

#### Upload Test Script (upload.lua)
```lua
-- upload.lua
wrk.method = "POST"
wrk.headers["Authorization"] = "Bearer api-key"
wrk.headers["Content-Type"] = "application/octet-stream"
wrk.body = string.rep("test data ", 1000)

request = function()
  local path = "/files/test-" .. math.random(1, 10000) .. ".txt"
  return wrk.format(nil, path)
end
```

#### Load Testing with Artillery
```yaml
# artillery.yml
config:
  target: 'https://localhost:8443'
  phases:
    - duration: 60
      arrivalRate: 10
    - duration: 120
      arrivalRate: 50
  defaults:
    headers:
      Authorization: 'Bearer api-key'

scenarios:
  - name: "File operations"
    flow:
      - post:
          url: "/files/test-{{ $randomString() }}.txt"
          headers:
            Content-Type: "application/octet-stream"
          body: "test file content"
      - get:
          url: "/files/test.txt"
      - delete:
          url: "/files/test-{{ $randomString() }}.txt"
```

## Troubleshooting

### Common Issues

#### High Memory Usage
```bash
# Check memory profile
curl https://localhost:8443/debug/pprof/heap > heap.prof
go tool pprof heap.prof

# Monitor GC
GODEBUG=gctrace=1 ./callfs server
```

#### High CPU Usage
```bash
# Check CPU profile
curl https://localhost:8443/debug/pprof/profile > cpu.prof
go tool pprof cpu.prof

# Check goroutines
curl https://localhost:8443/debug/pprof/goroutine > goroutine.prof
go tool pprof goroutine.prof
```

#### Database Connection Issues
```bash
# Check connection pool
curl https://localhost:8443/debug/vars | jq '.database'

# Monitor database queries
tail -f /var/log/postgresql/postgresql.log

# Check slow queries
SELECT query, mean_time, calls 
FROM pg_stat_statements 
ORDER BY mean_time DESC 
LIMIT 10;
```

### Diagnostic Commands

#### Service Status
```bash
# Check service status
systemctl status callfs

# View recent logs
journalctl -u callfs -f

# Check process info
ps aux | grep callfs
```

#### Network Diagnostics
```bash
# Check listening ports
netstat -tulpn | grep callfs

# Test connectivity
curl -k https://localhost:8443/health

# Check TLS certificate
openssl s_client -connect localhost:8443 -servername localhost
```

#### Storage Diagnostics
```bash
# Check disk space
df -h /var/lib/callfs

# Check file permissions
ls -la /var/lib/callfs

# Test S3 connectivity
aws s3 ls s3://callfs-storage --debug
```

## Best Practices

### Monitoring Strategy
1. **Layer monitoring** from infrastructure to application
2. **Use SLIs/SLOs** to define service quality
3. **Implement alerting** for actionable issues
4. **Regular monitoring review** and tuning

### Metrics Collection
1. **Sample appropriately** to balance cost and granularity
2. **Tag consistently** for effective querying
3. **Monitor what matters** to avoid metric explosion
4. **Document metrics** and their meaning

### Alerting
1. **Alert on symptoms**, not causes
2. **Implement alert routing** based on severity
3. **Avoid alert fatigue** with proper thresholds
4. **Regular alert review** and tuning

### Performance Optimization
1. **Establish baselines** for normal operation
2. **Monitor trends** over time
3. **Regular performance testing** under load
4. **Optimize based on metrics** not assumptions
