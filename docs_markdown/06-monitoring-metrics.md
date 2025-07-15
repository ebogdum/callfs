# Monitoring & Metrics

CallFS provides comprehensive observability through structured logging, detailed Prometheus metrics, and health check endpoints. This allows you to monitor the performance, health, and behavior of your CallFS deployment in real-time.

## Prometheus Metrics

CallFS exposes a rich set of metrics in Prometheus format at the `/metrics` endpoint. This endpoint does not require authentication.

**Endpoint:** `GET /metrics`

**Example:**
```bash
curl -k https://localhost:8443/metrics
```

### Key Metrics

- **`callfs_http_requests_total` (Counter)**: Tracks the total number of HTTP requests, labeled by `method`, `path`, and `status_code`. Useful for monitoring request rates and error rates.
- **`callfs_http_request_duration_seconds` (Histogram)**: Measures the latency of HTTP requests, labeled by `method` and `path`. Essential for tracking API performance and identifying slow endpoints.
- **`callfs_backend_ops_total` (Counter)**: Counts operations performed on storage backends (`localfs`, `s3`, `internalproxy`), labeled by `backend_type` and `operation`. Helps in understanding backend usage patterns.
- **`callfs_backend_op_duration_seconds` (Histogram)**: Measures the duration of backend operations, providing insight into storage performance.
- **`callfs_metadata_db_queries_total` (Counter)**: Counts queries to the PostgreSQL metadata store, labeled by `operation`.
- **`callfs_lock_operations_total` (Counter)**: Tracks distributed lock acquisitions and releases, labeled by `operation` and `status`. Critical for diagnosing concurrency issues.
- **`callfs_active_locks` (Gauge)**: Shows the number of currently active distributed locks.
- **`callfs_cross_server_operations_total` (Counter)**: Counts cross-server operations like proxying and conflict detection in a clustered setup.

### Monitoring Setup

1.  **Configure Prometheus**: Add a scrape job to your `prometheus.yml` to collect metrics from your CallFS instance(s).
    ```yaml
    scrape_configs:
      - job_name: 'callfs'
        metrics_path: '/metrics'
        scheme: 'https'
        tls_config:
          insecure_skip_verify: true # Use proper certs in production
        static_configs:
          - targets: ['your-callfs-host:8443']
    ```
2.  **Build Grafana Dashboards**: Use the exported metrics to build dashboards in Grafana for visualizing performance, error rates, backend activity, and more.

## Structured Logging

CallFS produces structured logs in either `json` (recommended for production) or `console` format.

**Configuration:**
```yaml
log:
  level: "info"   # debug, info, warn, error
  format: "json"  # json or console
```

**Log Fields:**
Logs include contextual information such as `trace_id`, `request_id`, `method`, `path`, `status`, `duration`, and `error` messages, making them easy to parse, search, and analyze in log aggregation platforms like the ELK Stack, Splunk, or Grafana Loki.

**Example JSON Log Entry:**
```json
{
  "level": "info",
  "ts": "2025-07-15T10:30:00Z",
  "caller": "server/router.go:80",
  "msg": "HTTP request",
  "method": "PUT",
  "path": "/v1/files/reports/quarterly.pdf",
  "status": 201,
  "duration": "52.3ms",
  "user_agent": "curl/7.81.0",
  "remote_addr": "192.168.1.10"
}
```

## Health Checks

CallFS provides an HTTP health check endpoint for monitoring its operational status.

**Endpoint:** `GET /health`

This endpoint is unauthenticated and returns a `200 OK` with a simple JSON body if the service is healthy.
```json
{"status":"ok"}
```

It can be used for:
- **Load Balancer Health Checks**: To ensure traffic is only routed to healthy instances.
- **Container Orchestration Probes**: For Kubernetes liveness and readiness probes or Docker health checks.

## Alerting

By combining Prometheus metrics with Alertmanager, you can create a powerful alerting strategy.

### Example Alerting Rules

- **High Error Rate**: Alert when the rate of 5xx server errors exceeds a certain threshold.
  ```yaml
  - alert: CallFSHighErrorRate
    expr: sum(rate(callfs_http_requests_total{status_code=~"5.."}[5m])) / sum(rate(callfs_http_requests_total[5m])) > 0.05
  ```
- **High Latency**: Alert when the 95th percentile request latency is too high.
  ```yaml
  - alert: CallFSHighLatency
    expr: histogram_quantile(0.95, sum(rate(callfs_http_request_duration_seconds_bucket[5m])) by (le)) > 2.0
  ```
- **Backend Errors**: Alert if a storage backend is consistently failing.
  ```yaml
  - alert: CallFSBackendFailure
    expr: rate(callfs_backend_ops_total{status="failure"}[5m]) > 0
  ```
- **Service Down**: Alert if the `up` metric for the CallFS job is 0.
  ```yaml
  - alert: CallFSServiceDown
    expr: up{job="callfs"} == 0
  ```
