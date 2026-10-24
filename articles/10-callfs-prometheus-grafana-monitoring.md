# Monitoring CallFS with Prometheus and Grafana

CallFS has Prometheus support built into the binary: no plugins, no sidecars, no extra processes. Every significant operation is counted or timed: HTTP requests, storage calls, file index queries, file operations, edit locks, download links, backup copies, background jobs and, in a cluster, the embedded NATS service. This guide for CallFS 2.0.0 shows how to turn metrics on, what each metric tells you, which PromQL queries are useful, how to build Grafana panels, and which alert rules to start with.

---

## Why Instrument a File Server

A file server that only logs errors is a file server you react to after something breaks. Metrics let you see trends: p99 latency that rises before an outage, an error rate that jumps after a storage change, lock waits that explain why uploads slow down. The setup below answers the three questions an operator asks: is the server healthy now, is it trending the wrong way, and where is the bottleneck?

CallFS also has a built-in view of the same numbers. The web console (**Insights** and **Home**) charts them without Prometheus; see [Web console](../docs_markdown/08-web-console.md). Use Prometheus and Grafana when you want longer history, several servers on one dashboard, or alerts that reach your pager.

---

## Enabling Metrics

CallFS serves the Prometheus text format at `/metrics` in two places. Choose the one that fits your network.

### Option 1: Dedicated metrics port (recommended)

The metrics port is on by default:

```yaml
metrics:
  listen_addr: "10.0.0.5:9090"
```

| Key | Default | Meaning |
|-----|---------|---------|
| `metrics.listen_addr` | `:9090` | Address of the metrics listener. An empty value turns it off. Needs a restart. |
| `metrics.profiling` | `false` | Also serves Go profiling pages on this port. Keep it off unless you are debugging. |

This listener serves only metrics, and it has no sign-in. Anyone who can reach the port can read every metric, so bind it to a private address (`127.0.0.1:9090` or a private interface) or firewall it. If you set a non-loopback address, the health finding `metrics.public_port` appears as a reminder. Environment variable: `CALLFS_METRICS__LISTEN_ADDR`.

```bash
curl -s http://10.0.0.5:9090/metrics | head
```

### Option 2: Metrics on the main port, with a key

If you cannot open another port, `/metrics` is also on the main HTTPS port. It needs an access key (or a console session):

```bash
curl -s -H "Authorization: Bearer your-api-key" \
  https://files.example.com:8443/metrics | head
```

---

## Available Metrics

Every metric starts with `callfs_`. Go runtime and process metrics (`go_*`, `process_*`) are exposed as well. The full reference with every label value is in [Monitoring](../docs_markdown/06-monitoring-metrics.md#metric-reference). The ones you will use most:

### HTTP layer

| Metric | Type | Labels | Notes |
|--------|------|--------|-------|
| `callfs_http_requests_total` | counter | `method`, `path`, `status_code` | `path` is the route pattern such as `/v1/files/*` or `/v1/links/generate`, never the real file path. Requests that match no route use `unmatched`. |
| `callfs_http_request_duration_seconds` | histogram | `method`, `path` | Default Prometheus buckets (5 ms to 10 s). Live streams and WebSocket upgrades are not timed. Uploads and downloads that take more than 10 seconds land in the last bucket. |
| `callfs_errors_total` | counter | `component`, `error_type` | Every response with status 400 or above. `component` is the first part of the route (`files`, `links`, `admin`, `download`, `trash`, `uploads`, and so on). `error_type` names the status: `bad_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `gone`, `rate_limited`, `internal`, `bad_gateway` and more. |
| `callfs_rate_limit_rejections_total` | counter | `endpoint` | Requests rejected by a rate limit: `download`, `link_generate` or `session`. |
| `callfs_auth_failures_total` | counter | `reason` | Failed authentications: `missing_key`, `wrong_key`, `expired_session`, `csrf`, `blocked`, `bad_delegation`. |
| `callfs_transfers_active` | gauge | `kind` | Transfers running now: `upload`, `download` or `websocket`. |

### Files and storage

| Metric | Type | Labels | Notes |
|--------|------|--------|-------|
| `callfs_file_operations_total` | counter | `operation`, `backend_type` | `operation` is `create`, `read`, `update`, `delete` or `rename`. `backend_type` is `localfs`, `s3`, `internal_proxy` or `erasure`. |
| `callfs_backend_ops_total` | counter | `backend_type`, `operation` | Calls to a storage backend (`open`, `create`, `update`, `delete`, `move`, `stat`, and others). |
| `callfs_backend_op_duration_seconds` | histogram | `backend_type`, `operation` | Time of each storage call. Shows the difference between local disk and S3. |
| `callfs_backend_op_failures_total` | counter | `backend_type`, `operation` | Storage calls that returned an error. |
| `callfs_usage_bytes`, `callfs_usage_files` | gauge | `storage` | Bytes and files per storage. Refreshed every 10 minutes. |
| `callfs_replication_total` | counter | `result` | Backup copy attempts: `succeeded`, `failed`, `dropped` or `skipped`. |
| `callfs_erasure_pieces_repaired_total` | counter | none | Pieces of erasure-coded files rebuilt by repairs. |

### File index and cache

| Metric | Type | Labels | Notes |
|--------|------|--------|-------|
| `callfs_metadata_db_queries_total` | counter | `operation` | Calls to the file index. `operation` includes `get`, `create`, `update`, `delete`, `rename`, `list_children`, `list_descendants`, `search_descendants` and `list_by_prefix`. |
| `callfs_metadata_db_query_duration_seconds` | histogram | `operation` | Time of each file index call. |
| `callfs_cache_lookups_total` | counter | `result` | `hit` or `miss`. |
| `callfs_cache_evictions_total`, `callfs_cache_entries` | counter, gauge | none | Cache churn and size. |

### Locks, links, jobs

| Metric | Type | Labels | Notes |
|--------|------|--------|-------|
| `callfs_lock_operations_total` | counter | `operation`, `status` | `acquire` ends as `acquired`, `timeout`, `cancelled` or `error`. `release` is `ok` or `error`. |
| `callfs_lock_operation_duration_seconds` | histogram | `operation` | For `acquire` this includes waiting for another holder. |
| `callfs_active_locks` | gauge | none | Locks held now. |
| `callfs_single_use_link_generations_total` | counter | none | Download links created. |
| `callfs_single_use_link_consumptions_total` | counter | `status` | Open attempts: `success`, `expired`, `invalid`, `not_found` or `error`. |
| `callfs_single_use_links_active` | gauge | none | Links that can still be opened. |
| `callfs_jobs_total` | counter | `kind`, `result` | Finished background jobs: `succeeded`, `partial`, `failed` or `cancelled`. |

### Cluster (embedded NATS)

| Metric | Type | Notes |
|--------|------|-------|
| `callfs_nats_connected` | gauge | 1 when the embedded NATS service is running. |
| `callfs_nats_routes` | gauge | Connected links to the other nodes. A healthy three-node cluster shows 2 on every node. |
| `callfs_nats_slow_consumers` | counter | Clients disconnected for being too slow. |
| `callfs_nats_in_msgs`, `callfs_nats_out_bytes` | counter | Messages received and bytes sent. |
| `callfs_nats_js_api_errors` | counter | Stream API requests that returned an error. |
| `callfs_nats_stream_messages`, `callfs_nats_stream_bytes` | gauge (label `stream`) | Size of each shared stream. |

The `callfs_raft_*` metrics of earlier releases no longer exist. Raft cluster mode was removed in 2.0.0; configs that use it are rejected at startup. Use `metadata_store.type: nats` and `dlm.type: nats`, and watch the `callfs_nats_*` series instead. In the console, **Cluster, Replication** shows the health of the shared file index, and the health findings `nats.not_connected`, `nats.no_quorum`, `nats.replica_offline`, `nats.log_lag`, `nats.snapshot_stale` and `nats.plain_transport` warn about the same problems.

---

## Prometheus Scrape Configuration

### Scraping the dedicated metrics port

```yaml
scrape_configs:
  - job_name: callfs
    scrape_interval: 15s
    scrape_timeout: 10s
    static_configs:
      - targets:
          - node-1.example.com:9090
          - node-2.example.com:9090
          - node-3.example.com:9090
```

Replace the targets with each CallFS node. Prometheus adds the `instance` label from the target address, so you can compare nodes. In Kubernetes, use a ServiceMonitor or pod annotations instead.

### Scraping the main port with a key

```yaml
scrape_configs:
  - job_name: callfs
    scheme: https
    metrics_path: /metrics
    scrape_interval: 15s
    tls_config:
      ca_file: /etc/prometheus/certs/callfs-ca.crt
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/callfs-key
    static_configs:
      - targets:
          - node-1.example.com:8443
```

Put the key in `/etc/prometheus/callfs-key` (mode `600`) so it does not appear in the config file. Use `scheme: http` and drop `tls_config` if the server runs without TLS.

### Check that it works

In the Prometheus UI, open **Status, Targets**: each node must show `UP`. Then run `callfs_http_requests_total` in the query box.

---

## Useful PromQL Queries

### Request rate

Requests per second by route:

```promql
sum by (path) (rate(callfs_http_requests_total[5m]))
```

By status code, to separate success from errors:

```promql
sum by (status_code) (rate(callfs_http_requests_total[5m]))
```

### Error rate

Share of requests that failed on the server side (5xx):

```promql
sum(rate(callfs_http_requests_total{status_code=~"5.."}[5m]))
/
sum(rate(callfs_http_requests_total[5m]))
```

All error responses (4xx and 5xx) by part of the API and kind:

```promql
sum by (component, error_type) (rate(callfs_errors_total[5m]))
```

4xx responses are usually clients (wrong key, missing file), so alert on 5xx and use this query to investigate.

### Latency

p99 over all routes:

```promql
histogram_quantile(0.99, sum by (le) (rate(callfs_http_request_duration_seconds_bucket[5m])))
```

p99 of file uploads (the `PUT` route for files):

```promql
histogram_quantile(
  0.99,
  sum by (le) (
    rate(callfs_http_request_duration_seconds_bucket{method="PUT",path="/v1/files/*"}[5m])
  )
)
```

### File operations by type

```promql
sum by (operation) (rate(callfs_file_operations_total[5m]))
```

### Storage latency and failures

p95 per backend and operation:

```promql
histogram_quantile(
  0.95,
  sum by (backend_type, operation, le) (rate(callfs_backend_op_duration_seconds_bucket[5m]))
)
```

Failures per backend:

```promql
sum by (backend_type, operation) (rate(callfs_backend_op_failures_total[5m]))
```

### Locks

Acquire attempts that did not succeed (timeouts and errors):

```promql
sum by (status) (rate(callfs_lock_operations_total{operation="acquire",status=~"timeout|error"}[5m]))
```

### Cache hit ratio

```promql
sum(rate(callfs_cache_lookups_total{result="hit"}[5m]))
/
sum(rate(callfs_cache_lookups_total[5m]))
```

### Rate limits and sign-in failures

```promql
sum by (endpoint) (rate(callfs_rate_limit_rejections_total[5m]))
sum by (reason) (rate(callfs_auth_failures_total[5m]))
```

### Storage use and backup copies

```promql
callfs_usage_bytes
sum by (result) (increase(callfs_replication_total[1h]))
```

### Download links

```promql
sum by (status) (rate(callfs_single_use_link_consumptions_total[1h]))
callfs_single_use_links_active
```

### Cluster

```promql
callfs_nats_routes
callfs_nats_connected
```

---

## Grafana Dashboard Panels

Add Prometheus as a data source, create a dashboard, and add an `instance` variable with the query `label_values(callfs_http_requests_total, instance)`. In panel queries, use `$__rate_interval` instead of a fixed `[5m]` and filter with `instance=~"$instance"`.

| Panel | Type | Query | Reading it |
|-------|------|-------|-----------|
| Requests by status | Time series, stacked | `sum by (status_code) (rate(callfs_http_requests_total{instance=~"$instance"}[$__rate_interval]))` | Throughput and the share of errors. Color 2xx green, 4xx orange, 5xx red. |
| Latency p50 / p95 / p99 | Time series | `histogram_quantile(0.99, sum by (le) (rate(callfs_http_request_duration_seconds_bucket{instance=~"$instance"}[$__rate_interval])))` (repeat with 0.95 and 0.5) | A p99 far above p50 means a subset of requests, often large transfers, is slow. |
| File operations | Time series | `sum by (operation) (rate(callfs_file_operations_total{instance=~"$instance"}[$__rate_interval]))` | Reads usually dominate. A sudden drop in everything points to a connectivity problem. |
| Errors by part of the API | Time series | `sum by (component, error_type) (rate(callfs_errors_total{instance=~"$instance"}[$__rate_interval]))` | First place to look in an incident. |
| Storage latency | Heatmap (format: Heatmap) | `sum by (le) (rate(callfs_backend_op_duration_seconds_bucket{instance=~"$instance"}[$__rate_interval]))` | A long tail often means a slow disk or slow S3 link. |
| Active locks and transfers | Stat | `callfs_active_locks`, `sum by (kind) (callfs_transfers_active)` | Locks that only grow point to a stuck holder. |
| Storage used | Stacked bars | `callfs_usage_bytes` | Where the data lives (`localfs`, `s3`, `erasure`). |
| Cluster links | Stat | `callfs_nats_routes` | Should equal the number of other nodes. |
| Cache hit ratio | Gauge | the cache ratio query above | Low values on a busy server mean `server.metadata_cache_size` is too small. |

Keep the console in mind too: **Insights** shows request, error, latency and storage charts from the same data with no setup (history length is `ui.metrics_retention`, default 24 hours).

---

## Alerting Rules

These rules cover the most important failure modes. Save them in a rules file loaded by Prometheus.

```yaml
groups:
  - name: callfs
    interval: 30s
    rules:
      - alert: CallFSDown
        expr: up{job="callfs"} == 0
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "CallFS {{ $labels.instance }} cannot be scraped"

      - alert: CallFSHighErrorRate
        expr: |
          sum(rate(callfs_http_requests_total{status_code=~"5.."}[5m]))
          /
          sum(rate(callfs_http_requests_total[5m])) > 0.05
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "More than 5% of CallFS requests fail with a 5xx"

      - alert: CallFSHighLatency
        expr: |
          histogram_quantile(0.99,
            sum by (le, instance) (rate(callfs_http_request_duration_seconds_bucket{path!~"/v1/files/\\*|/download/.*"}[5m]))
          ) > 2
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "p99 latency above 2 seconds on {{ $labels.instance }}"

      - alert: CallFSBackendFailures
        expr: sum by (backend_type, instance) (rate(callfs_backend_op_failures_total[5m])) > 0.1
        for: 3m
        labels:
          severity: warning
        annotations:
          summary: "{{ $labels.backend_type }} storage is failing on {{ $labels.instance }}"

      - alert: CallFSLockTimeouts
        expr: sum(rate(callfs_lock_operations_total{operation="acquire",status=~"timeout|error"}[5m])) > 0.5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Lock acquisitions are timing out or failing"

      - alert: CallFSBackupCopiesFailing
        expr: increase(callfs_replication_total{result=~"failed|dropped"}[1h]) > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Backup copies are failing on {{ $labels.instance }}"

      - alert: CallFSAuthFailureSpike
        expr: sum(rate(callfs_auth_failures_total[5m])) > 5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Many failed sign-in attempts"

      - alert: CallFSClusterLinkLost
        expr: callfs_nats_routes < 2
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "{{ $labels.instance }} has lost a cluster link"

      - alert: CallFSNatsDown
        expr: callfs_nats_connected == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "The embedded NATS service is not running on {{ $labels.instance }}"
```

Notes on the rules:

- `CallFSHighLatency` leaves out the file transfer routes, because a large upload or download legitimately takes many seconds and falls into the last histogram bucket. Watch transfers with `callfs_transfers_active` instead.
- `CallFSClusterLinkLost` assumes a three-node cluster, where every node should have 2 links. Change the number to the node count minus one. Leave the rule out on a single node.
- `CallFSAuthFailureSpike` uses a rate above 5 failures per second. Raise or lower it to match your traffic. Clients that fail repeatedly are blocked and then counted as `blocked`.

---

## Beyond Metrics: Health Checks

Prometheus tells you about trends. Two HTTP endpoints, on the main port and without a key, tell you about the current state:

| Endpoint | Use it for | Answer |
|----------|-----------|--------|
| `GET /health` | Liveness: restart the process if it fails | `{"status":"ok"}` while the process answers |
| `GET /health/ready` | Readiness and uptime probes (for example the Prometheus blackbox exporter) | `ok` or `degraded` with HTTP 200, `down` with HTTP 503 |

`/health/ready` checks the file index, locks, local disk, S3, split storage, the certificate (a warning when it expires in under 21 days), free disk space (a warning under 10% and `down` under 1%), the backup copy and the link cleanup worker. It reflects only the node you ask. Problems with other servers are listed under `peers` in the answer and never fail readiness, so a stopped node does not turn the others `down`. Use `/health/ready` for load balancer checks and alerts; watch `callfs_nats_routes` and the `peers` list for lost nodes.

`callfs healthcheck --config /etc/callfs/config.yaml` calls `/health` on the local server and exits with 0 on success, which suits a container health check.

CallFS also compares its own setup against a list of known problems and shows them as **health findings** in the console and at `GET /v1/admin/findings`, for example `backup.failing` and `metrics.public_port`. The full list is in [Monitoring](../docs_markdown/06-monitoring-metrics.md#health-findings).

---

## Putting It All Together

A complete monitoring setup for CallFS has four parts: CallFS exposing metrics on a private port, Prometheus scraping every node every 15 seconds, alert rules that fire before users notice, and Grafana dashboards that show where to look. Start with the nine panels and the alert rules above. Once you know what normal looks like for your workload, adjust the thresholds to cut false alarms without missing real incidents.

For multi-node deployments, keep the `instance` label on every query and compare nodes side by side, and add the cluster rules so a lost link is noticed within minutes. See [Setting up a CallFS cluster](06-callfs-clustering-nats.md).
