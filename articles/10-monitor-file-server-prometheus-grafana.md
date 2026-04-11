# Monitoring Your File Server with Prometheus and Grafana

File servers have a way of failing silently. Disk fills up overnight, a backend starts timing out under load, a flood of 500 errors goes unnoticed until a user complains. By the time you open a terminal and start poking around, the damage is done. A properly instrumented file server tells you what is happening before users do.

This guide covers how to set up **file server monitoring** using Prometheus and Grafana, using [CallFS](https://github.com/ebogdum/callfs) as the server being monitored. The principles apply to any Prometheus-scraped service, but the metrics shown here are real, running in production CallFS deployments.

---

## Why Monitoring File Servers Is Different from Monitoring Web Apps

Web applications tend to fail loud. An HTTP server returning 500s produces errors you can immediately see in logs. File servers compound that problem: they touch storage, lock state, metadata databases, and potentially multiple backend systems simultaneously. A single slow disk read can cascade into request queue buildup, which increases lock contention, which spikes error rates across seemingly unrelated operations.

The four signals worth tracking for any file server are:

**Disk and storage health.** File servers exist to store files. When storage degrades -- whether that is a full disk, a slow backend, or failed erasure-coding operations -- everything else suffers. You need to know before capacity is exhausted, not after.

**Request latency.** P99 latency matters more than average latency for file operations. A few slow requests caused by large file transfers can distort averages, masking the fact that small reads are also getting slow due to I/O contention.

**Error rates.** Errors from a file server often indicate backend failures, authentication misconfigurations, or storage issues. Tracking error rates by component lets you isolate root cause quickly.

**Backend and lock health.** Distributed file servers coordinate state across backends and use locking to prevent concurrent write conflicts. Active lock counts and backend operation rates tell you whether the coordination layer is healthy.

---

## CallFS Built-in Prometheus Metrics

CallFS exposes a `/metrics` endpoint in standard Prometheus text format. No plugins or agents required. The metrics are grouped into logical categories.

### HTTP Layer Metrics

`callfs_http_requests_total` is a counter with labels `method`, `path`, and `status`. It tracks every HTTP request the server handles. The `status` label is the HTTP response code, which lets you compute error rates directly from this single metric.

`callfs_http_request_duration_seconds` is a histogram of request latency. It includes standard Prometheus histogram buckets, which means you can compute any quantile using `histogram_quantile()`.

### Backend Operation Metrics

`callfs_backend_ops_total` counts operations against storage backends, labeled by `backend_type` and `operation`. If you run multiple backend types (local filesystem, S3-compatible storage), this metric shows you which backend is doing what work.

`callfs_backend_op_duration_seconds` is the corresponding latency histogram for backend operations. This is where you find out whether slow requests are caused by the application layer or the storage layer.

### File Operation Metrics

`callfs_file_operations_total` tracks create, read, update, and delete operations, labeled by `operation` and `backend_type`. This gives you a direct view of the workload pattern hitting your server -- whether you are primarily read-heavy, write-heavy, or mixed.

### Lock Metrics

`callfs_active_locks` is a gauge showing the current number of held locks. Sustained high lock counts indicate write contention or stalled clients that are not releasing locks.

`callfs_lock_operations_total` counts lock acquisitions and releases. A divergence between acquisitions and releases (tracked over time) indicates lock leaks.

### Infrastructure Metrics

`callfs_metadata_db_queries_total` counts queries against the metadata database. Spikes here correlate with high request rates and can warn you of database performance issues before latency climbs.

`callfs_single_use_link_generations_total` and `callfs_single_use_link_consumptions_total` track single-use download links. The consumption counter includes a `status` label (`success`, `expired`, `invalid`), which lets you detect whether links are expiring before users consume them.

`callfs_errors_total` is labeled by `component` and `type`. This is the most actionable error metric: it tells you not just that an error occurred but which subsystem produced it and what kind of error it was.

---

## Exposing the Metrics Endpoint

CallFS gives you two options for accessing metrics.

### Dedicated Metrics Port (Recommended for Internal Networks)

```yaml
metrics:
  listen_addr: ":9090"
```

This opens a separate listener on port 9090 that serves only the `/metrics` endpoint with no authentication. This is the standard Prometheus pattern: metrics are internal infrastructure data, not user data, so you typically expose them on an internal network interface only and block port 9090 at the firewall from external access.

### Main Port with Authentication

If your server is accessible only over HTTPS and you cannot expose a second port, access metrics through the main port with a bearer token:

```bash
curl -H "Authorization: Bearer YOUR_API_KEY" https://localhost:8443/metrics
```

This is appropriate for single-server setups where Prometheus runs on the same host, or where you configure Prometheus to include an authorization header in scrape requests.

---

## Prometheus Scrape Configuration

Add a scrape job to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'callfs'
    static_configs:
      - targets: ['fileserver:9090']
```

Replace `fileserver` with the hostname or IP address of your CallFS server. If you are running multiple CallFS instances, list them all under `targets`:

```yaml
scrape_configs:
  - job_name: 'callfs'
    static_configs:
      - targets:
          - 'fileserver-01:9090'
          - 'fileserver-02:9090'
          - 'fileserver-03:9090'
```

Prometheus will add an `instance` label automatically to distinguish each target. The default scrape interval of 15 seconds works well for file server metrics, though you can lower it to 10 seconds if you want finer resolution on lock and latency data.

If you are using the authenticated endpoint, add a `bearer_token` or `authorization` block to your scrape config:

```yaml
scrape_configs:
  - job_name: 'callfs'
    authorization:
      credentials: 'YOUR_API_KEY'
    scheme: https
    tls_config:
      insecure_skip_verify: false
    static_configs:
      - targets: ['fileserver:8443']
```

---

## Useful PromQL Queries

These queries cover the four core signals described above. Paste them into Prometheus's expression browser or use them as the basis for Grafana panels.

### Request Rate

```promql
rate(callfs_http_requests_total[5m])
```

This gives you requests per second, averaged over a five-minute window. Use `sum by (status)` to break it down by response code:

```promql
sum by (status) (rate(callfs_http_requests_total[5m]))
```

### Error Rate

```promql
rate(callfs_errors_total[5m])
```

To see errors by subsystem:

```promql
sum by (component, type) (rate(callfs_errors_total[5m]))
```

### P99 Request Latency

```promql
histogram_quantile(0.99, rate(callfs_http_request_duration_seconds_bucket[5m]))
```

The P99 is the latency that 99 percent of requests complete within. For file servers, a sudden climb in P99 latency is often the first sign of storage problems. Also compute P50 and P95 to understand the shape of the latency distribution:

```promql
histogram_quantile(0.50, rate(callfs_http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(callfs_http_request_duration_seconds_bucket[5m]))
```

### File Operations by Type

```promql
sum by (operation) (rate(callfs_file_operations_total[5m]))
```

This shows you the read/write/delete mix in real time. A sudden spike in delete operations can indicate a runaway cleanup job or an access control problem.

### Active Locks

```promql
callfs_active_locks
```

This is a gauge, not a rate. A healthy server should show a low, relatively stable number. Sustained growth in active locks during normal operation indicates a lock leak.

### Backend Latency by Type

```promql
histogram_quantile(0.99, sum by (backend_type, le) (rate(callfs_backend_op_duration_seconds_bucket[5m])))
```

This is one of the most useful queries for diagnosing slow requests: it shows whether latency is coming from a specific backend type.

---

## Building a Grafana Dashboard

A practical CallFS dashboard has five to seven panels, organized top-to-bottom from high-level throughput to low-level infrastructure.

### Panel 1: Request Rate (Time Series)

**Query:** `sum by (status) (rate(callfs_http_requests_total[5m]))`

**What it shows:** Total request throughput broken down by response code. You want to see mostly 2xx lines with 4xx and 5xx lines staying flat near zero. A rising 5xx line while 2xx holds steady suggests backend errors. A rising 4xx line suggests clients are sending bad requests or failing authentication.

### Panel 2: Request Latency Percentiles (Time Series)

**Queries:** P50, P95, P99 of `callfs_http_request_duration_seconds`

**What it shows:** The latency spread. The gap between P50 and P99 tells you how much variance exists in your response times. A wide gap means some requests are taking much longer than typical, which is worth investigating even if the average looks fine.

### Panel 3: Error Rate by Component (Time Series)

**Query:** `sum by (component) (rate(callfs_errors_total[5m]))`

**What it shows:** Which part of the system is generating errors. Labels like `storage`, `metadata`, `locks`, or `auth` give you an immediate direction for investigation without needing to dig through logs.

### Panel 4: File Operations per Second (Time Series)

**Query:** `sum by (operation) (rate(callfs_file_operations_total[5m]))`

**What it shows:** The actual workload mix. Useful for capacity planning and for detecting unexpected operation patterns (for example, a sudden burst of deletes or an unusual read spike after a deployment).

### Panel 5: Active Locks (Gauge or Time Series)

**Query:** `callfs_active_locks`

**What it shows:** Current lock pressure. Display as a gauge with a threshold at a value appropriate for your expected concurrency. Display as a time series to see whether locks are accumulating over time.

### Panel 6: Backend Operation Latency (Time Series)

**Query:** P99 of `callfs_backend_op_duration_seconds` grouped by `backend_type`

**What it shows:** Storage layer performance independent of the HTTP layer. If HTTP latency climbs but backend latency stays low, the bottleneck is in the application or network. If backend latency climbs first, the storage is the problem.

### Panel 7: Metadata Database Query Rate (Time Series)

**Query:** `rate(callfs_metadata_db_queries_total[5m])`

**What it shows:** Database activity. A sharp increase correlated with high request volume is expected. An increase without a corresponding request rate increase may indicate inefficient query patterns or background processes.

---

## Alerting Rules

Define alerting rules in a separate file and reference it from `prometheus.yml` with `rule_files`. Start with three alerts that cover the most actionable conditions.

```yaml
groups:
  - name: callfs
    rules:
      - alert: HighErrorRate
        expr: rate(callfs_errors_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "CallFS error rate elevated"
          description: "Error rate is {{ $value | printf \"%.2f\" }} errors/sec over the last 5 minutes."

      - alert: HighRequestLatency
        expr: histogram_quantile(0.99, rate(callfs_http_request_duration_seconds_bucket[5m])) > 2.0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "CallFS P99 latency above 2 seconds"
          description: "P99 request latency is {{ $value | printf \"%.2f\" }}s. Storage or backend issue likely."

      - alert: LockAccumulation
        expr: callfs_active_locks > 100
        for: 10m
        labels:
          severity: critical
        annotations:
          summary: "CallFS lock count abnormally high"
          description: "{{ $value }} active locks held for more than 10 minutes. Possible lock leak."
```

The `for` clause is important: it prevents alerting on transient spikes. Five minutes is a reasonable starting threshold for error rate and latency alerts. Ten minutes for lock accumulation gives the system time to recover from short bursts of write activity before paging anyone.

Tune the thresholds based on your baseline. If your server normally processes 50 requests per second, an error rate threshold of 0.1/sec is appropriate. If you run low-volume internal workloads, you may want to alert on even a single sustained error.

---

## Connecting Prometheus Alerts to Alertmanager

Prometheus evaluates the rules and fires alerts, but routing those alerts to Slack, PagerDuty, or email is Alertmanager's job. A minimal Alertmanager configuration that sends `warning` alerts to a Slack channel and `critical` alerts to an on-call rotation is a reasonable starting point for a file server running in production.

Once alerts are routing correctly, the next step is defining runbooks: short documents (or internal wiki pages) that describe what to do when a specific alert fires. The annotations in the alert rules are a good place to include a link to the relevant runbook. When an alert fires at 2 AM, the person on call should be able to open the Alertmanager notification, click a link, and immediately know what commands to run and what to look for.

---

## What to Monitor Beyond CallFS Metrics

CallFS metrics cover the application layer. A complete monitoring setup also tracks the host:

- **Disk space and inode usage** via the Prometheus `node_exporter`. Set alerts well before 100% capacity -- 80% is a common threshold that gives you time to act.
- **Network throughput** to catch saturation before it manifests as latency.
- **Memory and CPU** to detect resource exhaustion that can cause GC pressure and latency spikes in the application.

The node exporter runs as a separate process alongside CallFS and exposes its own `/metrics` endpoint, typically on port 9100. Add it as a second scrape job in your Prometheus config and build a second row of panels in your Grafana dashboard for host-level metrics.

---

## Summary

A working **prometheus monitoring setup** for a file server involves four pieces: the server exposing metrics, Prometheus scraping and storing them, alerting rules that fire before users notice problems, and Grafana dashboards that make the data readable at a glance.

CallFS makes the first piece straightforward -- the `/metrics` endpoint is built in, requires no configuration beyond enabling the listener, and exposes the metrics that actually matter for **file server monitoring**: request rates, latency histograms, error counts by component, active lock state, and backend performance. From there, the Prometheus and Grafana setup is standard, and the PromQL queries shown here give you a working starting point without needing to write them from scratch.

The goal is not a perfect dashboard from day one. Start with the five core panels, get the two or three most critical alerts routing to wherever your team actually looks, and iterate from there as you learn what normal looks like for your specific workload.
