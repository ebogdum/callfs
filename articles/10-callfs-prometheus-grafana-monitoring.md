# Monitoring CallFS with Prometheus and Grafana

CallFS ships with first-class Prometheus support built into the binary — no plugins, no sidecars, no extra processes required. Every significant operation the server performs is tracked: HTTP requests, backend storage calls, metadata database queries, file operations, distributed locks, and single-use link lifecycles. This guide walks through enabling metric collection, understanding what each metric tells you, writing useful PromQL queries, and building Grafana dashboards that give your team real operational visibility into a running CallFS deployment.

---

## Why Instrument a File Server

A file server that only logs errors is a file server you react to after something breaks. Prometheus metrics let you track trends — rising p99 latency before it becomes an outage, a spike in error rate correlated with a backend change, or growing lock contention that explains why client uploads are slowing. The `prometheus monitoring setup` described below is designed to answer the questions an SRE actually has: is the server healthy right now, is it trending in the wrong direction, and where exactly is the bottleneck?

---

## Enabling Metrics

CallFS supports two modes for exposing its `/metrics` endpoint. Choose the one that fits your network topology and security model.

### Option 1: Dedicated Metrics Port (Recommended)

Add a `metrics` block to your `config.yaml`:

```yaml
metrics:
  listen_addr: ":9090"
```

With this configuration, CallFS binds a separate HTTP listener on port 9090 that serves only the Prometheus metrics endpoint — no authentication required. This is the standard approach for `prometheus monitoring setup` in a private network: your Prometheus scraper reaches the metrics port directly without needing API credentials, while the main API port (`:8443` by default) remains protected.

This is the preferred approach when your Prometheus instance runs inside the same network segment or Kubernetes cluster as CallFS and the metrics port is not reachable from the public internet.

### Option 2: Metrics via Main Port with Authentication

If your deployment does not allow an additional open port, the `/metrics` endpoint is also available through the main server port. In this case you must supply a valid API key:

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" http://localhost:8443/metrics
```

This returns the standard Prometheus text exposition format. Configure your Prometheus scraper to include the Authorization header (shown in the scrape config section below).

---

## Available Metrics

CallFS registers the following metrics at startup. All are exported in the standard Prometheus text format.

### HTTP Layer

**`callfs_http_requests_total`** — Counter, labels: `method`, `path`, `status_code`

The total number of HTTP requests served, broken down by HTTP method, request path, and response status code. This is your primary signal for request volume and error rate. A sustained rise in `status_code="500"` demands immediate investigation.

**`callfs_http_request_duration_seconds`** — Histogram, labels: `method`, `path`

End-to-end HTTP request latency using Prometheus default buckets. Use this for latency percentile analysis across different endpoints. Upload (`PUT`) and download (`GET`) latencies will naturally differ and should be tracked separately.

### Backend Storage

**`callfs_backend_ops_total`** — Counter, labels: `backend_type`, `operation`

Total operations against each storage backend (for example, `localfs` or `s3`), broken down by operation type. Use this to understand the ratio of reads to writes and to spot anomalies like a sudden spike in delete operations.

**`callfs_backend_op_duration_seconds`** — Histogram, labels: `backend_type`, `operation`

Latency for each backend operation type. This is where you will see the difference between local disk I/O and object storage round-trip times. If `callfs_http_request_duration_seconds` spikes but `callfs_backend_op_duration_seconds` does not, the bottleneck is elsewhere (metadata, locking).

### File Operations

**`callfs_file_operations_total`** — Counter, labels: `operation`, `backend_type`

Counts completed file-level operations — `create`, `read`, `update`, and `delete` — attributed to the backend that handled them. This gives you the business-level view of what the server is actually doing with files, independent of raw HTTP mechanics.

### Distributed Locking

**`callfs_active_locks`** — Gauge

The current number of held locks across the system. A value that grows without bound indicates lock leaks or clients that are not releasing locks after failed operations.

**`callfs_lock_operations_total`** — Counter, labels: `operation`, `status`

Total lock acquire and release attempts, labeled by outcome (`success` or `failure`). High failure rates on `acquire` mean contention — multiple clients competing for the same file at the same time.

**`callfs_lock_operation_duration_seconds`** — Histogram, labels: `operation`

Time spent waiting to acquire or release a lock. Elevated acquisition latency is a leading indicator of write contention before it surfaces as client timeouts.

### Metadata Database

**`callfs_metadata_db_queries_total`** — Counter, labels: `operation`

Total queries issued to the metadata store, broken down by operation type. Useful for understanding metadata load and for correlating metadata query volume with storage backend load.

**`callfs_metadata_db_query_duration_seconds`** — Histogram, labels: `operation`

Latency of metadata database queries. If this climbs while backend latency stays flat, the bottleneck has shifted to metadata (common when SQLite is under heavy concurrent write load, or when a PostgreSQL connection pool is exhausted).

### Single-Use Links

**`callfs_single_use_link_generations_total`** — Counter

Total single-use download links generated. Track this to understand how heavily the link-sharing feature is used.

**`callfs_single_use_link_consumptions_total`** — Counter, labels: `status`

Total link consumption attempts, labeled by outcome: `success`, `expired`, `invalid`, or `not_found`. A rising `expired` rate suggests clients are receiving links but not using them within the configured TTL. A rising `invalid` rate may indicate a misconfigured client or an active token-probing attempt.

### Errors

**`callfs_errors_total`** — Counter, labels: `component`, `error_type`

Total errors by subsystem and type. This is the most direct signal for operational issues. The `component` label identifies which part of the system produced the error (for example, `backend`, `metadata`, `lock_manager`), and `error_type` describes what went wrong.

---

## Prometheus Scrape Configuration

### Scraping the Dedicated Metrics Port

```yaml
scrape_configs:
  - job_name: callfs
    static_configs:
      - targets:
          - callfs-host-1:9090
          - callfs-host-2:9090
          - callfs-host-3:9090
    scrape_interval: 15s
    scrape_timeout: 10s
```

Replace `callfs-host-N` with the hostname or IP of each CallFS node. In a Kubernetes deployment, use a ServiceMonitor (Prometheus Operator) or a pod annotation scrape configuration instead.

### Scraping via the Main Port with Authentication

```yaml
scrape_configs:
  - job_name: callfs
    static_configs:
      - targets:
          - callfs-host-1:8443
    scheme: https
    tls_config:
      insecure_skip_verify: false
      ca_file: /etc/prometheus/certs/callfs-ca.crt
    authorization:
      type: Bearer
      credentials: YOUR_API_KEY
    scrape_interval: 15s
```

Use `scheme: http` if the main server is running without TLS. Omit `tls_config` in that case.

---

## Useful PromQL Queries

### Request Rate

Total HTTP request rate across all methods and paths over a five-minute window:

```promql
rate(callfs_http_requests_total[5m])
```

Broken down by status code to separate successful requests from errors:

```promql
sum by (status_code) (rate(callfs_http_requests_total[5m]))
```

### Error Rate

Overall error rate from all subsystems:

```promql
sum(rate(callfs_errors_total[5m]))
```

Error rate broken down by component to identify which subsystem is misbehaving:

```promql
sum by (component) (rate(callfs_errors_total[5m]))
```

### P99 Latency

99th-percentile HTTP request latency across all endpoints:

```promql
histogram_quantile(
  0.99,
  rate(callfs_http_request_duration_seconds_bucket[5m])
)
```

P99 latency for upload operations specifically:

```promql
histogram_quantile(
  0.99,
  sum by (le) (
    rate(callfs_http_request_duration_seconds_bucket{method="PUT"}[5m])
  )
)
```

### File Operations by Type

Rate of each file operation type over the last five minutes:

```promql
sum by (operation) (rate(callfs_file_operations_total[5m]))
```

### Backend Latency by Storage Type

P95 latency per backend and operation:

```promql
histogram_quantile(
  0.95,
  sum by (backend_type, operation, le) (
    rate(callfs_backend_op_duration_seconds_bucket[5m])
  )
)
```

### Lock Contention

Lock acquire failure rate — a non-zero value sustained over time warrants investigation:

```promql
rate(callfs_lock_operations_total{operation="acquire",status="failure"}[5m])
```

---

## Grafana Dashboard Panels

The following panels form a practical `grafana dashboard` for day-to-day CallFS operations. Import them into a dashboard with a `job` variable set to `callfs`.

### Panel 1: Request Rate (Time Series)

**PromQL:** `sum by (status_code) (rate(callfs_http_requests_total[5m]))`

A stacked time series with one series per HTTP status code. At a glance this shows total throughput and the proportion of requests that are returning errors. Color the `2xx` band green, `4xx` orange, and `5xx` red. Set the Y-axis unit to `requests/s`.

### Panel 2: P50 / P95 / P99 HTTP Latency (Time Series)

**PromQL (three queries):**

```promql
histogram_quantile(0.50, rate(callfs_http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(callfs_http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.99, rate(callfs_http_request_duration_seconds_bucket[5m]))
```

Three overlapping lines on the same panel. P99 diverging sharply from P50 indicates that a subset of requests — typically large uploads or cold cache reads — are experiencing extreme latency while the median looks fine. Set the Y-axis unit to `seconds` and enable a log scale if values span multiple orders of magnitude.

### Panel 3: File Operations by Type (Time Series)

**PromQL:** `sum by (operation) (rate(callfs_file_operations_total[5m]))`

Tracks create, read, update, and delete rates. A healthy server typically shows reads dominating. An unusual spike in deletes or a sudden drop in all operations can indicate automated client behavior changes or a connectivity issue.

### Panel 4: Active Locks (Stat or Time Series)

**PromQL:** `callfs_active_locks`

A single stat panel showing the current lock gauge value is useful for at-a-glance health. Add a time series panel next to it to show how the value evolves over time. Alert thresholds: yellow above a baseline appropriate for your workload, red if the value grows monotonically across multiple scrape intervals.

### Panel 5: Error Rate by Component (Time Series)

**PromQL:** `sum by (component) (rate(callfs_errors_total[5m]))`

One series per component (`backend`, `metadata`, `lock_manager`, etc.). This panel is the first place to look during an incident — it immediately narrows down whether the problem is in storage, metadata, or the locking subsystem.

### Panel 6: Backend Operation Latency Heatmap (Heatmap)

**PromQL:** `sum by (le) (rate(callfs_backend_op_duration_seconds_bucket[5m]))`

A Grafana heatmap visualization using the histogram bucket data. This shows where latency is distributed, not just the percentiles. Useful for detecting bimodal distributions — for example, most requests completing in under 50ms but a long tail of requests taking several seconds, which often indicates intermittent network issues to object storage.

---

## Alerting Rules

Alerting on the right signals prevents alert fatigue while still catching real problems early. The rules below cover the most critical failure modes for a `file server monitoring` setup.

### High HTTP Error Rate

Fire when more than 5% of requests over the last five minutes are returning server errors:

```yaml
- alert: CallFSHighErrorRate
  expr: |
    (
      sum(rate(callfs_http_requests_total{status_code=~"5.."}[5m]))
      /
      sum(rate(callfs_http_requests_total[5m]))
    ) > 0.05
  for: 2m
  labels:
    severity: critical
  annotations:
    summary: "CallFS HTTP error rate above 5%"
    description: >
      The ratio of 5xx responses to total requests has exceeded 5% for
      more than two minutes. Current value: {{ $value | humanizePercentage }}.
```

### High P99 Latency

Fire when p99 request latency exceeds two seconds:

```yaml
- alert: CallFSHighLatency
  expr: |
    histogram_quantile(
      0.99,
      rate(callfs_http_request_duration_seconds_bucket[5m])
    ) > 2.0
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "CallFS p99 latency above 2 seconds"
    description: >
      The 99th-percentile HTTP request duration has been above 2 seconds
      for more than five minutes. Investigate backend and metadata latency.
```

### Backend Operation Errors

Fire when any backend is returning errors at a sustained rate:

```yaml
- alert: CallFSBackendErrors
  expr: |
    sum by (backend_type) (rate(callfs_errors_total{component="backend"}[5m])) > 0.1
  for: 3m
  labels:
    severity: warning
  annotations:
    summary: "CallFS backend errors on {{ $labels.backend_type }}"
    description: >
      The {{ $labels.backend_type }} backend has been producing errors
      at more than 0.1/s for three minutes. Check storage connectivity.
```

### Lock Contention

Fire when lock acquisitions are frequently failing:

```yaml
- alert: CallFSLockContention
  expr: |
    rate(callfs_lock_operations_total{operation="acquire",status="failure"}[5m]) > 0.5
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "CallFS lock acquisition failures"
    description: >
      Lock acquisition is failing at more than 0.5/s. This indicates
      write contention. Check for clients holding locks longer than expected.
```

### Example: Complete Alerting Rules File

```yaml
groups:
  - name: callfs
    interval: 30s
    rules:
      - alert: CallFSHighErrorRate
        expr: |
          (
            sum(rate(callfs_http_requests_total{status_code=~"5.."}[5m]))
            /
            sum(rate(callfs_http_requests_total[5m]))
          ) > 0.05
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "CallFS HTTP error rate above 5%"
          description: >
            5xx response ratio has exceeded 5% for more than two minutes.
            Current value: {{ $value | humanizePercentage }}.

      - alert: CallFSHighLatency
        expr: |
          histogram_quantile(
            0.99,
            rate(callfs_http_request_duration_seconds_bucket[5m])
          ) > 2.0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "CallFS p99 latency above 2 seconds"
          description: >
            P99 HTTP request latency has been above 2 seconds for five minutes.

      - alert: CallFSBackendErrors
        expr: |
          sum by (backend_type) (
            rate(callfs_errors_total{component="backend"}[5m])
          ) > 0.1
        for: 3m
        labels:
          severity: warning
        annotations:
          summary: "CallFS backend errors on {{ $labels.backend_type }}"
          description: >
            The {{ $labels.backend_type }} backend is producing errors
            at more than 0.1/s. Check storage connectivity and permissions.

      - alert: CallFSLockContention
        expr: |
          rate(
            callfs_lock_operations_total{operation="acquire",status="failure"}[5m]
          ) > 0.5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "CallFS lock acquisition failures"
          description: >
            Lock acquisitions are failing at more than 0.5/s, indicating
            write contention. Review client lock usage patterns.

      - alert: CallFSActiveLockGrowth
        expr: |
          deriv(callfs_active_locks[10m]) > 0.5
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "CallFS active locks growing steadily"
          description: >
            The active lock count has been growing at more than 0.5/min
            for ten minutes, which may indicate a lock leak.
```

---

## Putting It All Together

A complete `file server monitoring` setup for CallFS involves four components working together: CallFS exposing metrics on a dedicated port, Prometheus scraping those metrics every 15 seconds, alerting rules firing before users feel the impact, and Grafana dashboards giving your team the context to act quickly.

The metrics described in this guide cover every layer of the system: HTTP handling, file operations, backend storage, metadata queries, and distributed locking. Start with the six Grafana panels and five alerting rules above. As you learn your deployment's normal behavior, refine the alert thresholds to reduce false positives without missing real incidents.

For multi-node CallFS deployments, add a `instance` label to your scrape targets and use `sum by (instance)` in your queries to track per-node health alongside cluster-wide aggregates.
