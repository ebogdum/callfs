# Troubleshooting

Find your symptom, check the cause, apply the fix. Messages are quoted as CallFS prints them. Setting names link to [configuration](02-configuration.md).

- [Collecting diagnostics](#collecting-diagnostics)
- [Configuration errors](#configuration-errors)
- [Startup failures](#startup-failures)
- [HTTP errors](#http-errors)
- [Writes: locks, space and acknowledgement](#writes-locks-space-and-acknowledgement)
- [Cluster problems](#cluster-problems)
- [Console sign-in and sessions](#console-sign-in-and-sessions)
- [Rate limiting (429)](#rate-limiting-429)
- [Single-use links](#single-use-links)
- [Erasure coding and replication](#erasure-coding-and-replication)
- [Health findings](#health-findings)

## Collecting diagnostics

Do these first. They answer most questions.

### Validate the config

```bash
callfs config validate --config /etc/callfs/config.yaml
```

It prints `Configuration is valid` and a short summary, or `Configuration validation failed: <reason>`. Use `-c` as a short form of `--config`. Without `--config` CallFS looks in the default locations.

### Check that the server answers

```bash
callfs healthcheck --config /etc/callfs/config.yaml
```

It calls `/health` on `server.listen_addr` (a wildcard host becomes `localhost`, the timeout is 5 seconds) and exits 0 on HTTP 200. Otherwise it prints `health check failed: <reason>` or `health check returned status <code>` and exits 1.

The same checks over HTTP:

```bash
curl -k https://node-1.example.com:8443/health
curl -k https://node-1.example.com:8443/health/ready
```

| Endpoint | Sign-in | Answer |
|---|---|---|
| `/health` | none | `200` with `{"status":"ok"}`. It only shows that the process runs. |
| `/health/ready` | none | `200` with `{"status":"ok"}` or `{"status":"degraded"}` (warnings). `503` with `{"status":"down"}` when a health check of this server has failed. Problems with other servers are listed under `peers` and never make this server not ready. The answer is reused for 2 seconds. |

See [monitoring](06-monitoring-metrics.md) for what the checks cover.

### Raise the log level

```yaml
log:
  level: debug   # debug | info | warn | error
  format: text   # json | text
```

`log.level` applies without a restart (Settings page, or `CALLFS_LOG__LEVEL`). `log.format` needs a restart. Use `debug` while you reproduce a problem, then go back to `info`: debug logs every error response with its code and status. Logs go to standard error. The console shows them live on Activity, Logs.

### Find a request in the logs

Every `/v1` response carries an `X-Request-ID` header. Send it to your admin or search the logs and the audit trail for it.

```bash
curl -i -H "Authorization: Bearer your-api-key" https://files.example.com:8443/v1/files/missing.txt
```

### Read the metrics

```yaml
metrics:
  listen_addr: ":9090"   # empty turns the separate metrics port off
  profiling: false
```

```bash
curl http://10.0.0.5:9090/metrics | grep -E 'callfs_(errors|auth_failures|rate_limit|lock|http_requests)'
```

`/metrics` is also on the main port, where it needs a valid API key.

| Metric | Use it to find |
|---|---|
| `callfs_http_requests_total` | Which routes return 4xx and 5xx |
| `callfs_errors_total` | Error counts per component |
| `callfs_auth_failures_total` | Failed sign-ins by reason: `wrong_key`, `missing_key`, `expired_session`, `csrf`, `bad_delegation`, `blocked` |
| `callfs_rate_limit_rejections_total` | Which limit rejects clients |
| `callfs_lock_operations_total`, `callfs_lock_operation_duration_seconds`, `callfs_active_locks` | Lock waits and timeouts |
| `callfs_backend_op_failures_total`, `callfs_backend_op_duration_seconds` | Slow or failing storage |
| `callfs_metadata_db_query_duration_seconds` | Slow file index |
| `callfs_replication_total` | Backup copy results |
| `callfs_single_use_link_consumptions_total` | Download link use |

### Profile a slow server

Set `metrics.profiling: true` and restart. The metrics port then serves the standard profiling pages. Keep the port private.

```bash
go tool pprof http://10.0.0.5:9090/debug/pprof/profile?seconds=30
curl -o trace.out http://10.0.0.5:9090/debug/pprof/trace?seconds=5
```

Available pages: `/debug/pprof/`, `cmdline`, `profile`, `symbol`, `trace`. Turn profiling off again when you are done.

## Configuration errors

CallFS refuses to start when the config is invalid. The message names the key. Run `callfs config validate` to see it without starting the server. Environment variables use the `CALLFS_` prefix and a double underscore for nesting, for example `CALLFS_NATS__ROUTES`.

### Load errors

| Message | Cause and fix |
|---|---|
| `specified config file <path> not found` | The `--config` path is wrong. Fix the path. |
| `failed to load config file <path>` | The file cannot be read or is not valid YAML. Check permissions and indentation. |
| `failed to load environment variables` | A `CALLFS_` variable is malformed. Check the name and value. |
| `failed to unmarshal config` | A value has the wrong type, for example text where a number or duration is required. |
| `config validation failed: <reason>` | The values are read but a rule fails. Use the tables below. |

### Raft was removed

```
raft cluster mode was removed in v2.0.0; set metadata_store.type and dlm.type to nats and configure the nats section
```

Raft cluster mode was removed in 2.0.0. Configs that use it are rejected at startup. This happens when the file has a `raft:` section, when any `CALLFS_RAFT` or `CALLFS_RAFT_*` variable is set, or when `metadata_store.type` or `dlm.type` is `raft`. Remove those, then set:

```yaml
nats:
  enabled: true
metadata_store:
  type: nats
dlm:
  type: nats
```

See [clustering](07-clustering.md).

### Server and limits

| Message | Fix |
|---|---|
| `server.listen_addr is required` | Set an address such as `:8443`. |
| `server.protocol must be one of: http, https, auto` | Use one of the three values. |
| `server.cert_file and server.key_file are required when server.protocol=https` | Add both files or use `http`. |
| `server.cert_file and server.key_file are required when server.enable_quic=true` | QUIC needs a certificate. |
| `server.quic_listen_addr is required when server.enable_quic=true` | Set `server.quic_listen_addr`. |
| `server.<name>_timeout must be > 0` | Applies to `read_header_timeout`, `idle_timeout`, `transfer_timeout`, `read_timeout`, `write_timeout`, `file_op_timeout`, `metadata_op_timeout`. |
| `server.metadata_cache_ttl must be >= 0`, `server.metadata_cache_size must be > 0` | Use a valid cache size and lifetime. |
| `server.rate_limit.<name>_rps must be > 0` | Applies to `link_generate_rps`, `download_rps`, `session_rps`. |
| `server.rate_limit.<name>_burst must be >= 1` | Applies to `link_generate_burst`, `download_burst`, `session_burst`. |
| `server.trusted_proxies contains invalid CIDR "<value>"` | Write each entry as a CIDR, for example `10.0.0.0/24`. |

### File index and locks

| Message | Fix |
|---|---|
| `metadata_store.type must be one of: postgres, sqlite, redis, nats` | Pick one. |
| `metadata_store.dsn is required when metadata_store.type=postgres` | Set the PostgreSQL connection string. |
| `metadata_store.postgres.max_open_conns must be > 0 ...`, `max_idle_conns must be >= 0 ...`, `conn_max_lifetime must be >= 0 ...` | Use valid pool values. |
| `metadata_store.sqlite_path is required when metadata_store.type=sqlite` | Set the database file path. |
| `metadata_store.redis_addr is required when metadata_store.type=redis` | Set `host:port`. |
| `metadata_store.type=nats requires nats.enabled=true` | Set `nats.enabled: true`. |
| `dlm.type must be one of: redis, local, nats` | Pick one. |
| `dlm.redis_addr is required when dlm.type=redis` | Set the Redis address. `dlm.type` defaults to `redis`, so a file without a `dlm` section needs it. Use `local` for a single server or `nats` for a cluster. |
| `dlm.redis_pool_size must be > 0 ...`, `dlm.redis_min_idle_conns must be >= 0 ...` | Use valid Redis pool values. |
| `dlm.type=local cannot coordinate several nodes; set dlm.type to nats or redis` | Peers are configured. Use `nats` or `redis`. |
| `dlm.type=nats requires nats.enabled=true` | Set `nats.enabled: true`. |

### Keys, secrets and users

| Message | Fix |
|---|---|
| `auth.api_keys or auth.api_key_users must contain at least one key` | Add a key. |
| `auth.<source>: each key must be at least 16 characters` | Use 16 or more characters. |
| `auth.<source> must not use default value 'default-api-key'` | Replace the default. |
| `auth.<source> must not use an example placeholder value` | Replace the placeholder with a real random key. |
| `auth.api_keys: key at index <n> is already configured for <owner>` | Each key may appear once. |
| `auth.api_key_users: user ID must not be empty` | Give every entry a user ID. |
| `auth.api_key_users: user ID "<id>" is reserved` | Pick another ID. |
| `auth.api_key_users: key for "<id>" is already configured for <owner>` | Each key belongs to one user. |
| `auth.internal_proxy_secret must be set and not use default value` | Replace `change-me-internal-secret`. |
| `auth.internal_proxy_secret must be at least 16 characters` | Lengthen it. |
| `auth.internal_proxy_secret must not be reused as an API key` | Use a different value. |
| `auth.single_use_link_secret must be set and not use default value` | Replace `change-me-link-secret`. |
| `auth.single_use_link_secret must be at least 16 characters` | Lengthen it. |
| `auth.single_use_link_secret must not be the same as auth.internal_proxy_secret` | Use different values. |
| `auth.single_use_link_secret must not be reused as an API key` | Use a different value. |
| `auth.admin_users: "<id>" is not a configured user` | Admins must be users that have a key in `api_key_users` or the numbered `api-user-<n>` for `api_keys`. |

Generate strong values with `openssl rand -hex 32`.

### Instances and peers

| Message | Fix |
|---|---|
| `instance_discovery.instance_id is required` | Give each server a unique ID. |
| `instance_discovery.peer_endpoints: endpoint for "<id>" is not a valid URL` | Fix the URL. |
| `instance_discovery.peer_endpoints: endpoint for "<id>" must use https because the internal secret is sent to it` | Use `https://`. Only loopback addresses may use `http://`. |
| `instance_discovery.peer_endpoints requires nats.enabled=true` | Peers need the NATS section. Enable it or remove the peers. |

### NATS

| Message | Fix |
|---|---|
| `nats.cluster_name is required and must not contain whitespace, '.', '*' or '>'` | Use a plain name, the same on all servers. |
| `nats.server_name must not contain whitespace` | Remove spaces. An empty value uses the instance ID. |
| `nats.mode must be one of: embedded, external` | Pick one. |
| `nats.cluster_listen is required when nats.mode=embedded` | Default is `127.0.0.1:6222`. |
| `nats.cluster_listen must be host:port: ...`, `nats.client_listen must be host:port: ...`, `nats.monitor_listen must be host:port: ...` | Write the address as `host:port`, for example `10.0.0.5:6222`. `nats.client_listen` and `nats.monitor_listen` may be empty to turn them off. |
| `nats.<name> must include a port` | Add the port to the address. |
| `nats.tls.cert_file, nats.tls.key_file and nats.tls.ca_file are required when nats.cluster_listen is not a loopback address` | A cluster port open to the network needs TLS. |
| `nats.tls.cert_file and nats.tls.key_file are required when nats.client_listen is not a loopback address` | Same for the client port. |
| `nats.tls.cert_file and nats.tls.key_file must be set together` | Set both or neither. Same for `client_cert_file` with `client_key_file`. |
| `nats.tls.ca_file requires nats.tls.cert_file and nats.tls.key_file` | Add the certificate and key. |
| `nats.routes: "<value>" must use the nats-route:// or tls-route:// scheme` | Write routes as `nats-route://10.0.0.2:6222` or `tls-route://10.0.0.2:6222`. |
| `nats.routes: "<value>" must include host and port` | Add the port. |
| `nats.routes: "<value>" is not a valid URL: ...` | Fix the URL. |
| `nats.external.urls is required when nats.mode=external` | List the server URLs. |
| `nats.external.urls: "<value>" must use the nats://, tls://, ws:// or wss:// scheme` | Fix the scheme. |
| `nats.external.urls: "<value>" is not a valid URL: ...`, `... must include a host` | Fix the URL. |
| `nats.auth.mode must be nkey or creds when nats.mode=external` | External servers cannot use `derived`. |
| `nats.auth.mode=creds requires nats.mode=external` | The embedded server signs in with `derived` or `nkey`. Use `creds` only with your own NATS servers. |
| `nats.auth.mode must be one of: derived, nkey, creds` | Pick one. |
| `nats.auth.nkey_seed_file is required when nats.auth.mode=nkey` | Set the file. |
| `nats.auth.creds_file is required when nats.auth.mode=creds` | Set the file. |
| `nats.auth.previous_secret must be at least 16 characters` | Lengthen it. |
| `nats.auth.previous_secret must differ from auth.internal_proxy_secret` | Use the old secret here during a rotation, not the new one. |
| `nats.metadata.replicas must be between 1 and 5` | Same range for `nats.locks.replicas` and `nats.streams.replicas`. |
| `nats.metadata.read_barrier must be one of: info, publish` | Pick one. |
| `nats.metadata.apply_timeout must be > 0` | Use a positive duration. |
| `nats.metadata.snapshot_interval must be > 0`, `nats.metadata.snapshot_every must be > 0`, `nats.metadata.retain_commands must be > 0` | Use a positive value. |
| `nats.metadata.retain_snapshots must be >= 1` | Keep at least one snapshot. |
| `nats.metadata.max_msg_size must be between 64KiB and 64MiB` | Use a value in range. |
| `nats.locks.ttl must be >= 1s`, `nats.locks.queue_stale must be >= 1s` | Use at least one second. |
| `nats.jetstream.store_dir is required when nats.mode=embedded` | Set a directory, for example `/var/lib/callfs/nats`. |
| `nats.jetstream.sync must be one of: always, interval` | Pick one. |
| `nats.jetstream.sync_interval must be > 0 when nats.jetstream.sync=interval` | Set a positive interval. |
| `nats.jetstream.max_memory must be >= 0`, `nats.jetstream.max_store must be >= 0` | Use 0 or a positive size. |
| `nats.rpc_timeout must be > 0` | Use a positive duration. |
| `nats.bytes must be one of: http, chunked` | Pick one. |
| `nats.streams.logs_level must be one of: debug, info, warn, error` | Pick one. |
| `nats.streams.tasks_max_deliver must be >= 1` | Use 1 or more. |
| `nats.streams.tasks_max_deliver must be greater than the number of nats.streams.tasks_backoff entries` | Raise `tasks_max_deliver` or shorten `tasks_backoff`. |
| `nats.streams.<name> must be > 0` | Applies to `events_max_age`, `audit_max_age`, `logs_max_age`, `dlq_max_age`. `tasks_backoff entries must be > 0` applies to each retry wait. |
| `nats.streams.events_max_bytes must be >= 0` | Use 0 or a positive size. |
| `nats.server_tags: tag name "<name>" is invalid` | Use simple names. |

### Storage, performance, console and trash

| Message | Fix |
|---|---|
| `backend.default_backend must be one of: localfs, s3` | Pick one. |
| `backend.localfs_sync must be one of: full, barrier, none` | Pick one. |
| `backend.localfs_ack must be one of: memory, disk` | Pick one. |
| `backend.localfs_tmp_save_max_size must be at least 1 byte; leave it out for no size limit` | Remove the key, or set a size of 1 byte or more. |
| `backend.flush_workers must be >= 0 (0 means one per CPU)` | Use 0 or more. Same rule for `flush_max_pending_bytes` and `flush_max_delay`. |
| `performance.mode must be one of: fast, strict` | Pick one. |
| `ha.replica_backend must be one of: localfs, s3 when ha.replication_enabled=true` | Name the backup storage. |
| `erasure.data_shards must be >= 2`, `erasure.parity_shards must be >= 1` | Use valid counts. |
| `erasure.data_shards + erasure.parity_shards must not exceed 256` | Lower the total. |
| `erasure.min_file_size must be >= 0 (0 means only when an upload asks)` | Use 0 or a positive size. |
| `erasure.shard_path must be a clean relative path inside the storage (got "<value>")` | Use a relative folder such as `.callfs/shards`, with no leading `/`, no `..`, no `//` and no trailing `/`. An empty value uses `.callfs/shards`. |
| `ui.session_ttl must be between 5m and 720h` | Fix the value. |
| `ui.idle_timeout must be between 1m and ui.session_ttl` | Keep it below the session lifetime. |
| `ui.metrics_sample_interval must be between 1s and 60s` | Fix the value. |
| `ui.metrics_retention must be between 1h and 168h` | Fix the value. |
| `ui.metrics_retention / ui.metrics_sample_interval must not exceed 100000 samples per series` | Shorten retention or sample less often. |
| `ui.log_buffer_entries`, `ui.audit_buffer_entries`, `ui.event_buffer_entries` `must be between 100 and 1000000` | Fix the value. |
| `trash.retention must be between 1h and 8760h` | Fix the value. |

## Startup failures

CallFS prints `Error: <reason>` to standard error and exits with code 1.

| Message | Cause | Fix |
|---|---|---|
| `failed to load configuration: ...` | The config is invalid. | See [Configuration errors](#configuration-errors). |
| `failed to initialize logger` | `log.level` or `log.format` is not valid. | Use the allowed values. |
| `HTTP server failed: listen tcp :8443: bind: address already in use` (also `HTTPS server failed`, `metrics server failed`, `QUIC server failed`) | Another process holds the port. | Find it with `ss -ltnp \| grep 8443` or `lsof -i :8443`, or change `server.listen_addr`, `metrics.listen_addr`. |
| `HTTPS server failed: open <path>: no such file or directory` | `server.cert_file` or `server.key_file` is wrong. | Fix the paths. The service user must be able to read both files. |
| `HTTPS server failed: tls: failed to find any PEM data ...` | The file is not a PEM certificate or key, or the key does not match the certificate. | Check with `openssl x509 -in cert.pem -noout -subject` and use the matching key. |
| `protocol auto requires both cert_file and key_file; refusing to serve plaintext HTTP` | `server.protocol: auto` without a certificate. | Set both files, or set `server.protocol: http` on purpose. |
| `failed to initialize postgres metadata store: ...` | PostgreSQL is down, the DSN is wrong, or the credentials fail. | Test with `psql "<dsn>" -c "SELECT 1;"`. |
| `failed to run database migrations: ...` | The database user cannot create or alter tables. | Grant it the rights on the database. |
| `failed to initialize sqlite metadata store: ...` | The `metadata_store.sqlite_path` directory is missing or not writable. | Create it and fix ownership. |
| `failed to initialize redis metadata store: ...`, `failed to initialize redis lock manager: ...` | Redis is down or the address or password is wrong. | Test with `redis-cli -h 10.0.0.4 -p 6379 ping`. |
| `failed to read redis TLS CA file`, `failed to build redis TLS config`, `failed to build dlm redis TLS config` | The Redis TLS settings point to a missing or invalid CA file. | Fix `*_tls_ca_file`. |
| `failed to set up server-to-server TLS: read peer CA file: ...` | `backend.internal_proxy_ca_file` points to a file that is missing or unreadable. | Fix the path and make it readable by the service user. |
| `failed to set up server-to-server TLS: peer CA file <path> holds no PEM certificates` | The file in `backend.internal_proxy_ca_file` is not a PEM certificate bundle. | Use the PEM file of the authority that signed your servers' certificates. |
| `failed to initialize LocalFS backend: ...` | `backend.localfs_root_path` is missing or not writable. | Create the directory and give the service user write access. |
| `failed to initialize S3 backend: ...` | Bucket, region, endpoint or credentials are wrong. | Check the S3 settings and that the bucket exists. |
| `failed to ensure root directory exists: ...` | The file index cannot be written at startup. | Fix the index connection first. |
| `failed to start embedded NATS: ...` | The NATS ports are taken, `nats.jetstream.store_dir` is not writable, or a TLS file is unreadable. | Check `nats.cluster_listen`, `nats.client_listen`, the store directory and the `nats.tls.*` files. |
| `failed to provision NATS streams: ...` | The server waited 2 minutes for the other servers and none answered. | See [Cluster problems](#cluster-problems). |
| `failed to initialize nats metadata store` / `nats lock manager` | The NATS service is up but the index or lock storage cannot be created. | Read the lines above it in the log. Check disk space in the store directory. |
| `failed to initialize nats metadata store: ... natsmeta: metadata log has a gap not covered by any snapshot` | This server has no saved file index and the shared log no longer starts at the beginning, and no valid snapshot covers the gap. | Start from a server that holds the data, or take a snapshot on a healthy server (Cluster, Replication, Take snapshot, or `POST /v1/admin/cluster/snapshot`), then start this one again. Keep `nats.metadata.retain_snapshots` at 1 or more. |
| `failed to start NATS: nats: resolve client credentials: ...`, `nats: read nats.auth.nkey_seed_file: ...`, `nats: parse nats.auth.nkey_seed_file: ...`, `nats: nats.auth.nkey_seed_file must hold a user nkey seed`, `nats: nats.auth.creds_file is empty` | The NATS sign-in file is missing or wrong. | Fix `nats.auth.nkey_seed_file` (a user NKey seed) or `nats.auth.creds_file`. |
| `failed to start NATS: nats: the embedded server needs nats.auth.mode derived or nkey` | `nats.auth.mode: creds` with the embedded server. | Use `derived` or `nkey`, or set `nats.mode: external`. |
| `failed to start NATS: nats: in-process tls: ...`, `read nats ca file: ...`, `nats ca file <path> holds no PEM certificates`, `load nats.tls.client_cert_file: ...` | A NATS TLS file is missing, unreadable or not PEM, or the client certificate does not match its key. | Fix `nats.tls.ca_file`, `nats.tls.client_cert_file` and `nats.tls.client_key_file`. |
| `failed to start NATS: nats: build options: nats.tls: ...` / `nats.routes: ...` / `nats.client_listen: ...` / `nats.monitor_listen: ...` | The certificate or key cannot be loaded, or an address cannot be used. | Check the named setting and file. |
| `failed to start NATS: nats: server did not become ready`, `nats: jetstream not ready: ...` | The embedded server could not start in time, often because a port is in use or the store directory is not writable. | Check the ports and `nats.jetstream.store_dir`. |
| `failed to start NATS: nats: connect to external servers: ...` | None of `nats.external.urls` answered, or the sign-in or certificate was rejected. | Check the URLs, `nats.auth.*` and `nats.external.tls_ca_file`. |
| `failed to initialize node transport: ...` | The connection for server-to-server calls over NATS could not be set up. | Read the log line above it. Check that NATS is connected. |
| `failed to initialize session revocations: ...` | The shared list of signed-out sessions could not be opened. | Check that NATS is healthy and has disk space. |
| `failed to initialize cluster counters: ...` | `nats.counters: true` but the shared counters could not be created. | Check that NATS is healthy, or turn `nats.counters` off. |
| `failed to announce node presence: presence: ...` | This server could not register in the shared server list. | Check NATS health and disk space. The server must have a non-empty `instance_discovery.instance_id`. |
| `failed to start cluster log stream: ...` | The shared log stream (`nats.streams.logs_level`) could not be set up. | Check NATS health and the stream settings in `nats.streams`. |
| `failed to set up cluster workers: ...` | The once-per-cluster jobs (link cleanup, trash cleanup, usage refresh) could not register. | Check NATS health, then restart. |
| `failed to start node fs service: ...` | The service that lets other servers read this server's files could not start. | Check NATS health and the local storage path. |
| `failed to set up cluster node directory: ...` | The list of servers could not be built. | Check `instance_discovery` settings. |
| `failed to initialize internal proxy backend: ...` | A peer URL in `instance_discovery.peer_endpoints` is invalid. | Fix the URL (`https://`). |
| `metadata store does not keep erasure records` | `erasure.enabled: true` with a file index that cannot hold erasure records. | Use `postgres` or `nats`. |
| `metadata store type <type> does not support erasure coding` | `erasure.enabled: true` with a file index that cannot hold it. | Use `postgres` or `nats`, or turn erasure off. |
| `failed to mount web console: ...` | The console files could not be loaded. | Reinstall a build made with `make build`, or set `ui.enabled: false`. |
| `failed to start background jobs: ...` | A background job could not register. | Read the log line above it. |

Example of generating a self-signed certificate for a first test:

```bash
openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
  -subj "/CN=node-1.example.com" -keyout server.key -out server.crt
```

## HTTP errors

Errors from `/v1` return JSON:

```json
{"code": "FILE_NOT_FOUND", "message": "file not found"}
```

Server-side failures (500 and above, except 502) always return `{"code":"INTERNAL_ERROR","message":"an internal error occurred"}`. The real reason is in the server log, found by `X-Request-ID`.

| Status | Code | Message | Cause and fix |
|---|---|---|---|
| 400 | `BAD_REQUEST` | `invalid path` | The path is empty, escapes the root, or has forbidden characters. Use a clean absolute path. |
| 400 | `BAD_REQUEST` | `path is not a directory` | You listed or targeted a file as a directory. |
| 400 | `BAD_REQUEST` | `path exists as file, cannot create directory` / `path exists as directory, cannot create file` | A different kind of item already uses the name. Pick another name or delete it. |
| 400 | `NAME_TOO_LONG` | `file or directory name too long` | Shorten the name. |
| 400 | `BAD_REQUEST` | `If-None-Match only supports "*"` | Send `If-None-Match: *` or nothing. |
| 400 | `BAD_REQUEST` | `write ack must be one of: memory, disk (got "<value>")` | Fix the `X-CallFS-Write-Ack` header. See [Writes](#writes-locks-space-and-acknowledgement). |
| 400 | `BAD_REQUEST` | `X-CallFS-True-Overwrite must be true or false (got "<value>")` | Fix the `X-CallFS-True-Overwrite` header. See [Overwrites](05-storage.md#overwrites). |
| 400 | `BAD_REQUEST` | `invalid limit`, `limit must be between 1 and <max>`, `invalid cursor`, `depth must be 0 to 3` | A query parameter is out of range. See [API reference](03-api-reference.md). |
| 400 | `BAD_REQUEST` | `ops must contain 1 to <max> items`, `sync batches accept at most <n> ops, use mode job`, `mode must be sync or job`, `op must be delete, trash, move, rename, mode or restore` | Fix the batch body. For many operations use `"mode": "job"`. |
| 401 | `AUTHENTICATION_FAILED` | `authentication failed` | The key is missing or wrong (also on `POST /v1/session`), or the session expired or was signed out. See [Sign-in](#console-sign-in-and-sessions). The answer carries `WWW-Authenticate: Bearer`. |
| 403 | `PERMISSION_DENIED` | `permission denied` | You do not own the file, or the action needs an admin. Other users can read a file or list a folder only when the "other" read bit of its mode is set (for example `0644`, `0777`), and write or create items only when the "other" write bit is set. Only the owner (or an admin) can delete, and only they can change the mode. Ask the owner to change the mode (`PATCH` with `mode`, for example `0666`) or ask an admin. |
| 403 | `CSRF_FAILED` | `csrf token missing or invalid` | A browser session sent a change without the `X-CallFS-CSRF` header. API key requests are not affected. |
| 404 | `FILE_NOT_FOUND` | `file not found` | The path does not exist, or someone moved or deleted it. Check the spelling. |
| 404 | `FILE_NOT_FOUND` | `file not found` | Also returned when a file has no erasure record. Check the path. |
| 404 | `SHARD_NOT_FOUND` | `shard not found` | A piece of an erasure coded file is missing on this server. See [Erasure coding](#erasure-coding-and-replication). |
| 409 | `FILE_ALREADY_EXISTS` | `file already exists` or `file already exists, use PUT to update` | `POST` creates only. Use `PUT` to replace a file. |
| 409 | `RESOURCE_LOCKED` | `resource is locked, retry` | Another change to the same path is running. See [Writes](#writes-locks-space-and-acknowledgement). |
| 412 | `PRECONDITION_FAILED` | `precondition failed` | The file changed since your `If-Match` value. Read it again and retry. |
| 413 | `PAYLOAD_TOO_LARGE` | `upload exceeds size limit` or the size message | The body is larger than the allowed size. Use resumable uploads for big files. |
| 413 | `SUBTREE_TOO_LARGE` | `directory subtree too large` | A directory holds too many items for one operation. Work in smaller parts or use a batch job. |
| 416 | `RANGE_NOT_SATISFIABLE` | | The `Range` header is outside the file. |
| 429 | `RATE_LIMIT_EXCEEDED` | `Rate limit exceeded` | See [Rate limiting](#rate-limiting-429). |
| 501 | `NOT_IMPLEMENTED` | `this server is not set up for this request` | `PUT` or `POST` with `X-CallFS-Erasure: true` while `erasure.enabled` is `false`. Turn erasure on (Settings, Storage) or drop the header. The log says `erasure coding is not enabled on this server`. |
| 502 | `BAD_GATEWAY` | `failed to proxy request to owning server: ...` | The file lives on another server that did not answer. See [Cluster problems](#cluster-problems). |
| 507 | `INSUFFICIENT_STORAGE` | `insufficient storage space` | The disk is too full for this upload. See [Writes](#writes-locks-space-and-acknowledgement). |
| 500 | `INTERNAL_ERROR` | `an internal error occurred` | Read the server log for the `X-Request-ID`. |

A `404` for a path that exists in a listing usually means a trailing slash is wrong: directories end with `/` in URLs.

### Node gateway errors

Requests sent through `/v1/nodes/{node}/...` use `{"code","message"}` bodies too:

| Status | Code | Message | Fix |
|---|---|---|---|
| 401 | `UNAUTHORIZED` | `authentication required` | Send a valid key. |
| 400 | `BAD_REQUEST` | `this path cannot be sent to another server` | Only some routes can be controlled remotely. |
| 404 | `NODE_NOT_FOUND` | `unknown node: <id>` | The ID is not in the cluster. Take it from `GET /v1/nodes`. |
| 502 | `NODE_UNREACHABLE` | `server <id> did not answer` | The server is down or unreachable from this one. |
| 503 | `NOT_READY` | `router not ready` | The server is still starting. Retry. |
| 508 | `FORWARD_LOOP` | `requests can be forwarded only once` | Send the request to the server you want to control, not through two servers. |

## Writes: locks, space and acknowledgement

### 409 `resource is locked, retry`

- **Cause**: Every change takes a lock on the path. If another change holds it longer than `dlm.wait_timeout` (default `30s`), the request fails with 409. Typical reasons: a big upload to the same path, two clients saving the same file, or a lock left by a server that stopped.
- **Fix**:
  - Retry after a few seconds. The console shows "Someone is changing this file right now."
  - Raise `dlm.wait_timeout` if your uploads are long (applies without a restart).
  - Look at Cluster, Locks in the console to see who holds the lock and for how long. A lock from a stopped server frees itself when its lifetime ends (`nats.locks.ttl` with NATS locks).
  - On a cluster, all servers must use `dlm.type: nats` (or the same Redis). With `dlm.type: local` each server has its own locks and edits can collide.
  - Watch `callfs_lock_operations_total{outcome="timeout"}`.
- **500 `failed to acquire lock: ...` in the log**: the lock service itself failed (Redis down, or NATS has no quorum). Fix that service, not the file.

### 507 `insufficient storage space`

- **Cause**: The upload is larger than the free space on the disk that holds `backend.localfs_root_path`.
- **Fix**: Free space or extend the volume, empty the trash (Files, Trash), or send the file to S3 storage. Check the disk with `df -h /var/lib/callfs`. The health finding "Disk is almost full" warns before this happens.

### A file I saved is missing after a crash or power cut

- **Cause**: `backend.localfs_ack: memory` confirms a save once the file is written and indexed, before it reaches the disk. `backend.localfs_sync: none` never asks the disk to confirm. `barrier` is lighter than `full`.
- **Fix**: Use `backend.localfs_ack: disk` (the default) and `backend.localfs_sync: full`. To confirm one request only when the data is on disk, send the header:

```bash
curl -X PUT -H "Authorization: Bearer your-api-key" \
  -H "X-CallFS-Write-Ack: disk" \
  --data-binary @report.pdf https://files.example.com:8443/v1/files/docs/report.pdf
```

  Allowed values are `memory` and `disk`. Any other value returns 400. Both `performance.mode` values only confirm a write once it is durable.

### Saves are slow

- `backend.localfs_ack: disk` and `localfs_sync: full` are the safest and the slowest. Trade safety for speed only if you accept losing the last saves on a power cut.
- Tune the flush settings: `backend.flush_workers` (0 = one per CPU), `backend.flush_max_pending_bytes` (writers slow down above this much unflushed data), `backend.flush_max_delay`.
- `performance.mode: strict` reads back each saved file, locks every change and makes every read wait for the latest cluster state. Use `fast` unless you need the extra checks.
- Check `callfs_backend_op_duration_seconds` and `iostat -x 1` for slow disks, or the latency to your S3 endpoint.

### Reads are slow or time out

- Check `callfs_metadata_db_query_duration_seconds` for a slow file index.
- Raise `server.file_op_timeout` or `server.transfer_timeout` for long operations. Defaults are `10s` and `1h`.
- Raise `server.metadata_cache_size` or `server.metadata_cache_ttl` for repeated reads.
- Large downloads cut at 30 seconds: `server.write_timeout` (default `30s`) applies. Transfers use `server.transfer_timeout`, so check that you are not behind a proxy with a shorter timeout.

## Cluster problems

Clusters use the built-in NATS service. See [clustering](07-clustering.md) for the setup.

### The server waits at startup, then fails with `failed to provision NATS streams`

The log shows `Waiting for the NATS cluster before creating streams` once a second for up to 2 minutes. The server cannot reach enough other servers to agree on shared state. Check, on every server:

| Check | What to look for |
|---|---|
| `nats.routes` | Each entry points to another server's `nats.cluster_listen`, for example `nats-route://10.0.0.2:6222`. Without `nats.routes`, routes come from `instance_discovery.peer_endpoints` when `nats.routes_from_peers` is true (default). |
| `nats.cluster_listen` | Not `127.0.0.1` on servers that must be reached from others. A non-loopback address needs the `nats.tls.*` files. |
| Firewall | The cluster port (default 6222) is open between servers in both directions. |
| `nats.cluster_name` | The same on every server. |
| `auth.internal_proxy_secret` | The same on every server. See below. |
| Quorum | A cluster with 3 servers needs 2 running. With 2 servers both must run. |

Start the other servers. A first start of a new cluster only completes when enough servers are up.

### Mismatched internal secret

- **Symptom**: The cluster never forms (the same wait message as above), servers show as down in Cluster, Nodes, or cross-server requests return 401 or `NODE_UNREACHABLE`.
- **Cause**: Servers authenticate to each other with `auth.internal_proxy_secret`. Every server must have the same value.
- **Fix**: Set the same secret everywhere (use `CALLFS_AUTH__INTERNAL_PROXY_SECRET` to avoid typos). To rotate it without downtime, put the old value in `nats.auth.previous_secret` while you update each server, then remove it.

### Servers reject each other's certificates

- **Symptom**: Proxied requests, shard transfers or node probes fail with `x509: certificate signed by unknown authority` (or an expired or wrong-name error) in the log, and `/v1/nodes` shows the peer as unreachable.
- **Cause**: The peers use certificates from a private authority that this server does not trust.
- **Fix**: Set `backend.internal_proxy_ca_file` to the PEM file of that authority on every server and restart. Turning `backend.internal_proxy_skip_tls_verify` on also works but exposes the shared secret to anyone on the network. Do not use it in production.

### NATS TLS handshake fails

- **Symptom**: Servers do not join, with TLS or certificate errors in the log.
- **Fix**: Give every server a certificate signed by the authority in `nats.tls.ca_file`, valid for the host in `nats.routes`. Use `tls-route://` for routes when `nats.tls.cert_file` is set. For `nats.mode: external`, set `nats.external.tls_ca_file` and use `tls://` URLs. Keep `nats.auth.mode` identical on all servers.

### Cluster findings

| Finding in the console | Meaning and fix |
|---|---|
| "Cluster messaging is not encrypted" (`nats.plain_transport`) | Cluster traffic is not using TLS. Set `nats.tls.cert_file`, `key_file` and `ca_file` on every server (Settings, Coordination). |
| "This server is not connected to NATS" (`nats.not_connected`) | Check `nats.*`, the network and the NATS secret. |
| "File index has lost its majority" (`nats.no_quorum`) | Too many servers are offline, so changes cannot be saved. Start them. Reads still work. |
| "{n} file index copies are offline" (`nats.replica_offline`) | Bring those servers back, or remove them under Cluster, Members. |
| "This server is behind on changes" (`nats.log_lag`) | It has not applied recent changes yet. Check its network and disk speed. |
| "File index snapshot is out of date" (`nats.snapshot_stale`) | No snapshot in the last hours. Use Take snapshot under Cluster, Replication. Snapshots are also taken on their own according to `nats.metadata.snapshot_interval` and `snapshot_every`. |

### `instance_discovery.peer_endpoints ... must use https because the internal secret is sent to it`

Peers must be `https://` URLs. Only loopback addresses may use `http://`. Fix the URL and make sure the peer's certificate is trusted.

### Replicas greater than the number of servers

Not an error. When `nats.metadata.replicas`, `nats.locks.replicas` or `nats.streams.replicas` is higher than the number of servers, CallFS uses one copy per server. On a single server all three are effectively 1. Values above 5 are rejected (`must be between 1 and 5`). With 2 servers, losing one stops writes, so run 3 for fault tolerance.

### `/health/ready` returns 503

- **Cause**: A health check reports `down`.
- **Fix**: Open Home or Cluster, Hygiene in the console, or call `/v1/admin/health` with an admin key, and read the failing check. Common ones: a storage location that cannot be written, a peer server that cannot be reached, or an expired certificate. See [monitoring](06-monitoring-metrics.md). `degraded` with 200 means warnings only; the server still serves.

### A server shows as down in Cluster, Nodes

- Open `https://<peer>/health` from the server that shows it down. If that fails, fix network, DNS or the certificate.
- Check `instance_discovery.peer_endpoints` lists the peer under the same ID the peer uses as its own `instance_discovery.instance_id`.
- A peer with the same `instance_id` as another server confuses the cluster. IDs must be unique. The finding "Server ID is the default" warns about the default.

### A file exists on another server and returns 502

`failed to proxy request to owning server` means the server that holds the file did not answer. Bring it back or check `server.transfer_timeout`. The file is not lost.

### Controlling another server from the console shows an error

| Console message | Meaning |
|---|---|
| "A server did not answer." | The server that holds the file is down. |
| "That server is not in the cluster." | It left the list. Pick another server. |
| "The server you control does not answer." | This server cannot reach it. Check it runs, or go back to this server. |
| "The request went in a loop." | Switch back to the server you opened, then retry. |
| "callfs cannot accept changes right now." | The cluster has no quorum. Reading still works. Start the missing servers. |

## Console sign-in and sessions

### "That key did not work. Check it and try again."

- **Cause**: The API key is wrong. The console signs in with an API key from `auth.api_keys` or `auth.api_key_users`.
- **Fix**: Copy the key again without spaces or line breaks. Keys are case sensitive. Compare with your config. If you changed the key in the config file, restart or reload CallFS.

### "Too many tries. Wait 1 minute."

- **Cause**: Sign-in attempts are limited per client address by `server.rate_limit.session_rps` (default `1`) and `session_burst` (default `5`). Repeated wrong keys from one address are also blocked for a while (429).
- **Fix**: Wait a minute. Behind a proxy, set `server.trusted_proxies` so each user is seen by their own address; otherwise everyone shares the proxy address and hits the limit together.

### "We cannot reach the server. Check that callfs is running."

The browser got no answer. Check the URL, port, certificate warning and that the process runs (`callfs healthcheck`).

### "You are signed out." / "Session expired"

- **Cause**: The session lasted longer than `ui.session_ttl`, was idle longer than `ui.idle_timeout`, you signed out (on a cluster, this signs you out on every server), or the key behind it was removed or changed. The sign-in page shows a "session expired" notice after an expiry.
- **Fix**: Sign in again. The console keeps your place. Raise `ui.session_ttl` (5 minutes to 720 hours) if sessions end too soon.

### Signed in, but every change fails with `csrf token missing or invalid`

- **Cause**: A browser session must send the `X-CallFS-CSRF` header on changes. The console does it for you. A custom page or script using the session cookie does not.
- **Fix**: Use the `csrf_token` from the sign-in answer as the header, or call the API with `Authorization: Bearer your-api-key` instead.

### The cookie is not kept

Sign-in works but the next page shows signed out. Check that you use the same host name every time, and that HTTPS is used when `server.protocol` is `https`. A browser blocks secure cookies on plain HTTP across other hosts.

### "Only admins can do this."

Cluster, Settings and the admin pages need a user listed in `auth.admin_users`. Add the user ID there and restart.

### Sign-ins fail from a script with 401

- Send `Authorization: Bearer your-api-key`. The word `Bearer` and one space are required.
- More than a few wrong keys in a row from one address returns 429 `RATE_LIMIT_EXCEEDED` instead. Wait and fix the key. Failures are counted in `callfs_auth_failures_total` and listed in Activity, Sign-ins.

## Rate limiting (429)

```json
{"code": "RATE_LIMIT_EXCEEDED", "message": "Rate limit exceeded"}
```

Rate limit answers carry a `Retry-After` header with the seconds to wait. Limits are per client address:

| Limit | Settings (default) | Applies to |
|---|---|---|
| Link creation | `server.rate_limit.link_generate_rps` (`100`), `link_generate_burst` (`100`) | Creating single-use links |
| Downloads | `server.rate_limit.download_rps` (`10`), `download_burst` (`5`) | Single-use link downloads |
| Sign-in | `server.rate_limit.session_rps` (`1`), `session_burst` (`5`) | Creating a console session |
| Failed keys | fixed | Wrong API keys from one address |
| Active links | `server.rate_limit.max_active_links_per_user` (`1000`; `0` means 1000) | Link creation, answered with `too many active download links` |

Fixes:

- Slow the client down and honour `Retry-After`.
- Raise the limit. The `*_rps` and `*_burst` values apply without a restart; `max_active_links_per_user` needs one.
- Behind a load balancer, set `server.trusted_proxies` to the balancer's CIDR. Without it every user shares one address and one limit. The finding "All users may share one request limit" flags this.
- On a cluster, set `nats.counters: true` so the limits are counted across all servers instead of per server.
- `callfs_rate_limit_rejections_total` shows which limit rejects.

## Single-use links

| Status | Code | Message | Cause and fix |
|---|---|---|---|
| 404 | `LINK_NOT_FOUND` | `link not found` | The token is wrong or was cleaned up. Create a new link. |
| 410 | `LINK_EXPIRED` | `link has expired` | The link outlived its expiry. Create a new one. |
| 410 | `LINK_INVALID` | `link is invalid or has been used` | The link was already downloaded, or its signature does not match. Create a new one. |
| 400 | `BAD_REQUEST` | `missing token` | The URL has no token. |
| 400 | `BAD_REQUEST` | `expiry must be between 1 and 86400 seconds` | Choose an expiry from 1 second to 24 hours. |
| 400 | `BAD_REQUEST` | `links work for files only` | Links cannot point to directories. |
| 403 | `PERMISSION_DENIED` | `permission denied` | You cannot read that file. |
| 429 | `RATE_LIMIT_EXCEEDED` | `too many active download links` | Wait for links to expire or be used, or raise `server.rate_limit.max_active_links_per_user`. |

The console shows 410 as "This link does not work anymore. It was used or it expired."

Notes:

- A link downloads once. Opening it shows a confirmation page; the download starts when the recipient confirms, so a preview by a chat app or virus scanner does not use the link up. Share the link only with the recipient.
- A link made on one server works on every server only when they share the same `auth.single_use_link_secret`. Set it the same everywhere. If you change the secret, existing links stop working (`link is invalid or has been used`).
- Many failed links show up as the health finding "Many download links fail".

## Erasure coding and replication

### Erasure coded files

| Message | Cause | Fix |
|---|---|---|
| `erasure coding is not enabled on this server` | An erasure API was called with `erasure.enabled: false`. | Enable erasure in Settings, Storage, and restart. |
| `erasure: insufficient shards for reconstruction (code 3050)` | Fewer pieces than `erasure.data_shards` are readable. The file cannot be rebuilt right now. | Bring the servers or storage holding the pieces back. Reads recover once enough pieces return. |
| `erasure: shard checksum mismatch (code 3051)` | A stored piece is damaged. | Run repair (Cluster, Durability, or the erasure repair API). Repair rebuilds damaged pieces from the good ones. |
| `erasure: invalid erasure profile parameters (code 3054)` | The data and parity shards are not valid, or the total exceeds the limit per request. | Fix `erasure.data_shards` and `erasure.parity_shards`. |
| `erasure: shard not found on this node (code 3055)` | A piece is missing on that server. | Run repair. |
| `erasure: zero-byte uploads cannot be erasure coded (code 3056)` | An empty file was uploaded to erasure storage. | Upload a non-empty file, or store empty files outside erasure storage. |
| `failed to store shard <n> on <server>` | A server that should hold a piece was unreachable during the upload. | Check the server, then retry. |
| `no endpoint for instance <id>` / `no endpoint found for instance <id> (shard <n>)` | A server holding a piece is not listed in `instance_discovery.peer_endpoints`. | Add it. |
| `remote shard store failed with status <code>` / `remote shard fetch failed with status <code>` / `remote shard delete failed with status <code>` | The other server answered with an error. | Check that server's log. A 401 means a different `auth.internal_proxy_secret`. |
| `peer endpoint <url> must use https (http is allowed only for loopback peers)` | A peer URL uses plain HTTP. | Use `https://`. |
| `health must be one of ok, reduced, critical, unreadable` | Bad `health` filter on the erasure files list. | Use one of the four values. |
| `invalid shard path`, `invalid shard index`, `invalid shard file path`, `invalid erasure file path`, `prefix is too long`, `missing path` | A request parameter is wrong. | Check the request against [the API reference](03-api-reference.md). |

The health finding "N split files need repair" and "One server failure can break split files" point to the same causes. Use Cluster, Durability to see which files are reduced, critical or unreadable.

### Backup copies (replication)

- **Backup copies are failing**: the backup storage (`ha.replica_backend`) is down or rejects writes. Check its credentials and space. `callfs_replication_total` shows failures.
- **Uploads fail while the backup is down**: `ha.require_replica_success: true` makes an upload fail unless the backup copy is written. Turn it off if you prefer uploads to succeed and the copy to catch up.
- **`ha.replica_backend must be one of: localfs, s3 when ha.replication_enabled=true`**: name the backup storage.

### Moving files between storages

`moving to a different backend or instance requires admin` (or the batch form `moving to a different storage or instance requires admin`) returns 403 for non-admin users. Ask an admin or keep the move on the same storage.

## Health findings

The console and the admin API list findings: problems CallFS finds in your own setup, each with a plain explanation and a fix. Examples are "Default secret in use", "Certificate expires in N days", "Server X cannot be reached", "Disk is almost full" and "N saved changes need a restart". Open Home or Cluster, Hygiene to see them. The full list, what each means and how to act on it is in [monitoring](06-monitoring-metrics.md).

A setting saved in Settings that says it needs a restart does not take effect until you restart CallFS. The finding "N saved changes need a restart" lists them.
