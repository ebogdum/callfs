# Admin API

The admin API runs everything the web console does: server status, settings, users, storage, erasure coding, cluster servers, background jobs, health findings, logs and live event streams. Every route lives under `/v1/admin`. The same server also offers a node gateway under `/v1/nodes` that lets you reach any other server in the cluster through the server you are signed in to.

For the public file API (files, directories, links, uploads, search, trash) see [API reference](03-api-reference.md). For the console pages that use these routes see [Web console](08-web-console.md).

## Conventions

### Who may call

Every `/v1/admin` route needs an admin. An admin is the `root` user or any user listed in `auth.admin_users` (see [Authentication and security](04-authentication-security.md)).

| Situation | Status | Body |
|---|---|---|
| No valid credentials | 401 | `{"code":"AUTHENTICATION_FAILED","message":"authentication failed"}` |
| Signed in but not an admin | 403 | `{"code":"ADMIN_REQUIRED","message":"Admin access required"}` |

### Authentication

Send your API key as a bearer token:

```bash
curl -H "Authorization: Bearer your-api-key" https://files.example.com/v1/admin/status
```

A browser session works too. Create it with `POST /v1/session`, then send the session cookie on every request. Requests that change something (anything but GET, HEAD and OPTIONS) must also carry the `X-CallFS-CSRF` header with the token the sign-in returned. A missing or wrong token returns 403 `CSRF_FAILED`.

All curl examples below use this shorthand:

```bash
export CALLFS=https://files.example.com
export KEY=your-api-key
```

and send `-H "Authorization: Bearer $KEY"`.

### Requests and responses

- Request and response bodies are JSON (`Content-Type: application/json`), except the two downloads (`/config/export`, `/audit?format=csv`) and the two event streams.
- Request bodies are limited to 64 KiB (16 KiB for the server-address and retry routes). A larger body returns 413 `PAYLOAD_TOO_LARGE`. Invalid JSON returns 400 `BAD_REQUEST`.
- Times are RFC 3339 in UTC, for example `2026-03-01T09:30:00Z`.
- Lists that can grow take `limit` and `cursor`. Pass the `next_cursor` of one response as `cursor` of the next. `next_cursor` is `null` on the last page.
- Long work (verify, retag, owner changes, cleanup, retries) runs as a background job. The call returns 202 with a job id; follow it with [`GET /jobs/{id}`](#jobs) or the [event stream](#event-stream).

### Errors

Errors use one shape:

```json
{
  "code": "VALIDATION_FAILED",
  "message": "request has invalid fields",
  "details": {
    "errors": [
      { "path": "reason", "message": "reason must be at most 200 characters" }
    ]
  }
}
```

`details` is present only when there is more to say. Common codes:

| Status | Code | Meaning |
|---|---|---|
| 400 | `BAD_REQUEST` | Invalid JSON, query value or parameter |
| 401 | `AUTHENTICATION_FAILED` | Missing or wrong credentials |
| 403 | `ADMIN_REQUIRED`, `CSRF_FAILED`, `FEATURE_DISABLED` | Not an admin, bad CSRF token, or the setting that enables the route is off |
| 404 | `NOT_FOUND`, `NODE_NOT_FOUND`, `FEATURE_DISABLED` | Unknown item or node, or the feature is not running on this server (`details.feature` names it) |
| 409 | `CONFLICT` | The action does not fit the current state |
| 413 | `PAYLOAD_TOO_LARGE` | Request body too large |
| 422 | `VALIDATION_FAILED` | Fields are invalid; `details.errors` lists `path` and `message` per field |
| 429 | `RATE_LIMIT_EXCEEDED` | Too many failed sign-ins from this address |
| 500 | `INTERNAL_ERROR` | Server failure |
| 503 | | The component the route reads (health checker, telemetry, findings, metrics sampler) is not running |

### Route index

| Area | Routes |
|---|---|
| [Status](#status-and-restart) | `GET /status`, `POST /restart`, `GET /migrations` |
| [Health and findings](#health-and-findings) | `GET /health`, `GET /findings`, `POST` and `DELETE /findings/{id}/mute`, `POST` and `DELETE /findings/{id}/ignore` |
| [TLS](#tls) | `GET /tls` |
| [Settings](#settings) | `GET /config`, `GET /config/schema`, `POST /config/validate`, `PUT /config`, `PATCH /config/runtime`, `GET /config/export` |
| [Cluster](#cluster) | `GET /cluster`, `GET /cluster/state-stats`, `POST /cluster/leadership-transfer`, `DELETE /cluster/nodes/{id}`, `GET /cluster/snapshots`, `POST /cluster/snapshot`, `GET /cluster/join-config` |
| [Servers](#cluster-servers) | `GET /instances`, `PUT` and `DELETE /instances/{id}/endpoint`, `POST /instances/{id}/probe` |
| [Backends](#backends) | `GET /backends`, `POST /backends/{name}/probe` |
| [Erasure coding](#erasure-coding) | `GET /erasure`, `POST /erasure/plan`, `POST /erasure/verify-all` |
| [Backup copies and cleanup](#backup-copies-and-cleanup) | `GET /replication`, `POST /replication/retry`, `GET` and `DELETE /staging`, `GET /hygiene`, `POST /retag` |
| [Locks, cache, rate limits](#locks-cache-and-rate-limits) | `GET /locks`, `DELETE /locks/{key}`, `GET /cache`, `POST /cache/invalidate`, `GET /ratelimits` |
| [Users and keys](#users-and-keys) | `GET` and `POST /users`, `PATCH` and `DELETE /users/{id}`, `POST /users/{id}/rotate`, `POST /users/migrate-positional`, `GET /owners`, `POST /owners/reassign`, `GET /auth/failures` |
| [Telemetry](#telemetry) | `GET /metrics`, `GET /metrics/series`, `GET /logs`, `GET /logs/stream`, `GET /audit`, `GET /events`, `GET /events/history`, `GET /transfers` |
| [Jobs and workers](#jobs-and-workers) | `GET /jobs`, `GET /jobs/{id}`, `POST /jobs/{id}/cancel`, `POST /jobs/{id}/retry-failed`, `GET /workers`, `POST /workers/{name}/run` |
| [Node gateway](#node-gateway) | `GET /v1/nodes`, `ANY /v1/nodes/{node}/...` |

All paths in the sections below are relative to `/v1/admin` unless they start with `/v1`.

---

## Status and restart

### GET /status

Returns build, process and storage facts for this server.

Response `200`:

```json
{
  "version": "2.0.0",
  "commit": "a1b2c3d",
  "build_date": "2026-03-01T08:00:00Z",
  "go_version": "go1.24.0",
  "instance_id": "node-1",
  "node_id": "node-1",
  "started_at": "2026-03-01T09:00:00Z",
  "uptime_s": 5400,
  "config_path": "/etc/callfs/config.yaml",
  "config_format": "yaml",
  "keys_from_env": ["auth.internal_proxy_secret"],
  "keys_at_default": ["server.read_timeout"],
  "listeners": [
    { "kind": "https", "addr": ":8443", "tls": true, "auth": true },
    { "kind": "metrics", "addr": "10.0.0.5:9090", "tls": false, "auth": false }
  ],
  "protocol_configured": "auto",
  "protocol_effective": "https",
  "file_index": "nats",
  "lock_type": "nats",
  "storages": { "localfs": true, "s3": false, "internal_proxy": true, "erasure": false },
  "runtime": { "goroutines": 84, "heap_bytes": 31457280, "open_fds": 42, "cpu_seconds": 12.5 }
}
```

| Field | Meaning |
|---|---|
| `instance_id`, `node_id` | This server's cluster ID (`instance_discovery.instance_id`) and its embedded NATS server name |
| `config_format` | `yaml`, `json` or `none` (no config file in use) |
| `keys_from_env` | Settings currently set by an environment variable |
| `keys_at_default` | Settings still at their default |
| `listeners` | Each listener: `kind` is `https`, `http`, `quic` or `metrics`; `tls` and `auth` tell whether it is encrypted and needs a key |
| `protocol_configured` / `protocol_effective` | `server.protocol` as set, and what the server actually serves (`auto` falls back to `http` when no certificate and key are set) |
| `file_index`, `lock_type` | `metadata_store.type` and `dlm.type` |
| `storages` | Which storages are configured |
| `runtime.open_fds` | `null` where the system does not report it |

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/status
```

### POST /restart

Restarts the server process. Off by default: set `ui.allow_remote_restart: true` first (needs a restart to take effect).

Body (optional):

| Field | Type | Notes |
|---|---|---|
| `reason` | string | Up to 200 characters. Written to the log. |

Response `202`: `{"restarting": true}`. The server answers first, then restarts.

| Status | When |
|---|---|
| 202 | Restart started |
| 403 `FEATURE_DISABLED` | `ui.allow_remote_restart` is false |
| 422 | `reason` longer than 200 characters |
| 503 | Restart control not available |

```bash
curl -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"reason":"apply storage settings"}' $CALLFS/v1/admin/restart
```

### GET /migrations

Shows file-index schema migrations.

```json
{
  "driver": "postgres",
  "current_version": 12,
  "dirty": false,
  "available": [ { "version": 1, "name": "init" }, { "version": 12, "name": "add_etag" } ]
}
```

`driver` is `postgres` or `none`. `current_version` and `dirty` are `null` unless the driver is `postgres`. `dirty: true` means an index update did not finish.

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/migrations
```

---

## Health and findings

### GET /health

Runs the server's health checks and returns every component.

| Query | Notes |
|---|---|
| `fresh=true` | Check now. Without it a result up to 5 seconds old can be returned. |

```json
{
  "status": "warn",
  "checked_at": "2026-03-01T10:00:00Z",
  "components": [
    { "name": "file_index", "status": "ok", "latency_ms": 2, "detail": "", "since": "2026-03-01T09:00:05Z" },
    { "name": "disk", "status": "warn", "latency_ms": 0, "detail": "8% free", "since": "2026-03-01T09:40:00Z" }
  ]
}
```

Component names: `file_index`, `locks`, `storage.localfs`, `storage.s3`, `storage.internal_proxy`, `nats`, `metadata_log`, `erasure`, `tls`, `disk`, `replication`, `link_cleanup`. Status values: `ok`, `warn`, `down`, `disabled`. The top-level `status` is the worst component. `since` is when the component entered its current status.

- `nats` is `down` when this server is not connected to its messaging layer, `disabled` when `nats.enabled` is false.
- `metadata_log` checks the shared file index log: `warn` when this server is more than 100 changes behind or some log copies are offline, `down` when it is more than 5000 changes behind or the log is not ready. `disabled` when `nats.enabled` is false.
- `storage.internal_proxy` is reported with `"informational": true`. It shows other servers' state but never changes the top-level `status`.

The unauthenticated `GET /health` and `GET /health/ready` endpoints are described in [Monitoring and metrics](06-monitoring-metrics.md).

Status codes: 200, 503 if the health checker is not running.

```bash
curl -H "Authorization: Bearer $KEY" "$CALLFS/v1/admin/health?fresh=true"
```

### GET /findings

Lists problems the server found in its own setup and state: weak secrets, plain HTTP, expiring certificates, low disk, damaged split files and more.

| Query | Notes |
|---|---|
| `area` | `security`, `storage`, `cluster`, `config` or `runtime`. Other values return 422. |
| `include_muted=true` | Also return findings that are muted. |

```json
{
  "items": [
    {
      "id": "tls.cert_expiring",
      "severity": "warning",
      "area": "security",
      "title": "Certificate expires in 12 days",
      "body": "Renew the certificate before it expires.",
      "affected": [ { "kind": "server", "id": "node-1", "href": "/cluster/servers" } ],
      "since": "2026-03-01T09:00:00Z",
      "fix": { "label": "Open TLS settings", "route": "/settings/security", "setting_key": "server.cert_file", "command": null },
      "muted": null,
      "ignored": null
    }
  ],
  "ignored": [],
  "counts": { "critical": 0, "warning": 1, "info": 0 }
}
```

- `severity` is `critical`, `warning` or `info`.
- `muted` is `{"until","by","reason"}` while a mute is active. `ignored` is `{"at","by","reason"}` for an ignored finding.
- `items` holds active findings. Ignored findings are listed separately in `ignored`.
- `counts` covers active, unmuted findings.

Finding IDs:

| ID | Area | Severity |
|---|---|---|
| `tls.plain_http` | security | critical |
| `tls.cert_unreadable` | security | critical |
| `tls.cert_expiring` | security | warning |
| `tls.cert_expired` | security | critical |
| `tls.key_world_readable` | security | warning |
| `secret.default` | security | critical |
| `secret.short` | security | warning |
| `secret.same` | security | warning |
| `keys.positional` | security | warning |
| `admin.none` | security | info |
| `metrics.public_port` | security | warning |
| `peers.http` | cluster | warning |
| `peers.skip_verify` | security | critical |
| `instance.default_id` | cluster | warning |
| `instance.unreachable` | cluster | warning |
| `index.sslmode_disable` | security | warning |
| `index.default_password` | security | warning |
| `redis.no_password` | security | warning |
| `locks.local_multi` | cluster | critical |
| `s3.public_acl` | security | critical |
| `s3.no_encryption` | security | info |
| `erasure.tolerance` | storage | warning |
| `erasure.damaged` | storage | warning |
| `backup.failing` | storage | warning |
| `disk.low` | storage | warning |
| `hygiene.staging` | storage | info |
| `hygiene.no_owner` | storage | info |
| `hygiene.no_etag` | storage | info |
| `migrations.dirty` | storage | critical |
| `timeouts.zero` | runtime | info |
| `timeouts.short_for_uploads` | runtime | info |
| `proxy.shared_bucket` | runtime | info |
| `links.failed_opens` | runtime | warning |
| `auth.failures_spike` | security | warning |
| `config.pending_restart` | config | warning |

Some findings change severity with the situation (for example `disk.low` becomes critical when little space is left).

```bash
curl -H "Authorization: Bearer $KEY" "$CALLFS/v1/admin/findings?area=security&include_muted=true"
```

### POST /findings/{id}/mute

Hides a finding from the counts for a while. It returns when the time is up.

Body:

| Field | Type | Notes |
|---|---|---|
| `duration_s` | integer | Required. 60 to 604800 (one minute to seven days). |
| `reason` | string | Optional, up to 200 characters. |

Response `200`: the finding, with `muted` filled in.

| Status | When |
|---|---|
| 200 | Muted |
| 404 | Unknown finding ID |
| 409 | The finding is not active right now |
| 422 | `duration_s` out of range or `reason` too long |

```bash
curl -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"duration_s":86400,"reason":"cert renewal is scheduled"}' \
  $CALLFS/v1/admin/findings/tls.cert_expiring/mute
```

### DELETE /findings/{id}/mute

Removes a mute. Response `200`: the finding. Status codes: 404 unknown ID, 409 finding not active.

```bash
curl -X DELETE -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/findings/tls.cert_expiring/mute
```

### POST /findings/{id}/ignore

Hides a finding until you un-ignore it. Unlike a mute it never expires.

Body: `{"reason": "..."}` (optional, up to 200 characters). Response `200`: the finding with `ignored` filled in. Status codes: 404 unknown ID, 409 not active, 422 reason too long.

```bash
curl -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"reason":"internal network, plain HTTP is intended"}' \
  $CALLFS/v1/admin/findings/tls.plain_http/ignore
```

### DELETE /findings/{id}/ignore

Stops ignoring a finding. Response `204` with no body. Status codes: 404 unknown ID, 409 not active.

```bash
curl -X DELETE -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/findings/tls.plain_http/ignore
```

---

## TLS

### GET /tls

Shows the TLS state of this server.

```json
{
  "enabled": true,
  "protocol_configured": "auto",
  "protocol_effective": "https",
  "fallback": false,
  "cert": {
    "path": "/etc/callfs/tls/cert.pem",
    "subject": "CN=files.example.com",
    "issuer": "CN=Example CA",
    "sans": ["files.example.com"],
    "not_before": "2026-01-01T00:00:00Z",
    "not_after": "2026-06-01T00:00:00Z",
    "days_remaining": 92
  },
  "key": { "path": "/etc/callfs/tls/key.pem", "mode": "0600", "world_readable": false },
  "quic": { "enabled": false, "addr": ":8443" },
  "hsts": true,
  "headers": {
    "csp": "default-src 'self'",
    "frame_options": "DENY",
    "coop": "same-origin",
    "coep": "require-corp"
  }
}
```

- `fallback` is true when `server.protocol` is `auto` and the server fell back to plain HTTP because no certificate and key are set.
- `cert` and `key` are `null` when the server runs over plain HTTP or the file path is not set. The key file is never read, only its permissions.
- `headers` are the security headers the server adds to its responses.

Status codes: 200, 500 when the certificate or key file cannot be read.

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/tls
```

---

## Settings

Settings are the keys described in [Configuration](02-configuration.md). The routes below read them, check changes, save them to the config file and apply the ones that can change without a restart.

A key has a `restart` value: `runtime` means the change applies at once, `restart` means it applies after the next restart. Keys set by an environment variable (`source: env`) and environment-only keys cannot be changed here; changes to them return `set by environment` as the field error.

### GET /config

Returns every setting with its current value.

```json
{
  "config_path": "/etc/callfs/config.yaml",
  "restart_required": true,
  "pending": ["server.listen_addr"],
  "revision": "5b0f6c9e2a7d41c3",
  "keys": [
    {
      "path": "server.metadata_cache_ttl",
      "value": "5m0s",
      "default": "5m0s",
      "source": "default",
      "env": "CALLFS_SERVER__METADATA_CACHE_TTL",
      "secret": false,
      "set": true,
      "restart": "runtime",
      "valid": true
    },
    {
      "path": "auth.internal_proxy_secret",
      "value": null,
      "default": "",
      "source": "env",
      "env": "CALLFS_AUTH__INTERNAL_PROXY_SECRET",
      "secret": true,
      "set": true,
      "restart": "restart",
      "valid": true
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `revision` | Fingerprint of the config file. Send it back as `expected_revision` when you save. |
| `pending` | Keys whose saved value differs from the running value (a restart will change them). |
| `restart_required` | True when `pending` is not empty. |
| `keys[].source` | `default`, `file`, `env` or `runtime` (changed through the API since start) |
| `keys[].value` | `null` for secrets; `set` tells whether a secret has a value |
| `keys[].restart` | `runtime` or `restart` |

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/config
```

### GET /config/schema

Returns the description of every setting: type, allowed values, limits, default, grouping and the environment variable.

```json
{
  "keys": [
    {
      "path": "server.protocol",
      "type": "enum",
      "enum": ["https", "http", "auto"],
      "min": null,
      "max": null,
      "default": "https",
      "required_when": null,
      "secret": false,
      "env": "CALLFS_SERVER__PROTOCOL",
      "restart": "restart",
      "group": "network",
      "docs_anchor": ""
    }
  ]
}
```

`type` is one of `bool`, `int`, `float`, `bytes`, `duration`, `enum`, `string`, `path`, `url`, `hostport`, `list`, `map`, `secret`. `group` is one of `network`, `limits`, `access`, `security`, `storage`, `index`, `coordination`, `logging`. `required_when` is a condition such as `ha.replication_enabled == true`.

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/config/schema
```

### POST /config/validate

Checks changes without saving them.

Body:

```json
{ "changes": { "server.metadata_cache_ttl": "10m", "log.level": "debug" } }
```

`changes` maps a setting path to its new value. The checks are the same as for startup validation.

Response `200`:

```json
{
  "valid": true,
  "errors": [],
  "warnings": [ { "path": "server.protocol", "message": "Access keys travel without encryption." } ],
  "restart_required": [],
  "applies_now": ["log.level", "server.metadata_cache_ttl"]
}
```

`errors` entries are `{"path","message"}`; a message with no `path` is a cross-setting error. Warnings appear for plain HTTP and for an `auth.internal_proxy_secret` shorter than 32 characters.

Status codes: 200 (also when `valid` is false), 400 invalid body.

```bash
curl -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"changes":{"log.level":"debug"}}' $CALLFS/v1/admin/config/validate
```

### PUT /config

Validates the changes, writes them to the config file (a timestamped backup of the old file is kept next to it) and applies the keys that do not need a restart.

Body:

| Field | Type | Notes |
|---|---|---|
| `changes` | object | Required, not empty. Setting path to new value. |
| `expected_revision` | string | Required. The `revision` from `GET /config`. |

Response `200`:

```json
{
  "written": true,
  "backup_path": "/etc/callfs/config.yaml.bak.20260301T100000.000000000Z",
  "revision": "9d41c8a07b52e6f1",
  "restart_required": ["server.listen_addr"],
  "applied_now": ["log.level"]
}
```

| Status | When |
|---|---|
| 200 | Saved |
| 400 | Empty or invalid body, or no `changes` |
| 409 `CONFLICT` | The config file changed since you read it (`details.revision` has the current one), or no config file is in use |
| 422 `VALIDATION_FAILED` | A change is invalid; `details.errors` lists each one. Nothing is written. |

```bash
curl -X PUT -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"expected_revision":"5b0f6c9e2a7d41c3","changes":{"log.level":"debug","server.listen_addr":":9443"}}' \
  $CALLFS/v1/admin/config
```

### PATCH /config/runtime

Changes settings that apply at once. Faster than `PUT /config` and needs no revision.

Body is a flat object of setting path to value, plus an optional `"persist": true` that also writes the change to the config file.

```json
{ "log.level": "debug", "server.rate_limit.download_rps": 20, "persist": true }
```

Settings that can change this way: `server.metadata_cache_ttl`, `server.rate_limit.download_rps`, `download_burst`, `link_generate_rps`, `link_generate_burst`, `session_rps`, `session_burst`, `log.level`, `dlm.wait_timeout`, `ui.session_ttl`, `ui.idle_timeout`, `trash.enabled`, `trash.retention`, `nats.metadata.read_barrier`, `nats.streams.replicas`, `nats.streams.events_max_age`, `events_max_bytes`, `audit_max_age`, `logs_level`, `logs_max_age`, `tasks_max_deliver`, `tasks_backoff`, `dlq_max_age`.

Response `200`: `{"applied":["log.level"],"persisted":true,"revision":"9d41c8a07b52e6f1"}`.

| Status | When |
|---|---|
| 400 | No settings given, or invalid body |
| 409 | `persist` is true and no config file is in use |
| 422 | A key needs a restart (`needs a restart, save it in the config file instead`), is set by the environment, is unknown, or has an invalid value |

```bash
curl -X PATCH -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"log.level":"debug"}' $CALLFS/v1/admin/config/runtime
```

### GET /config/export

Downloads the settings as a file. Secrets are replaced by `<set>` (or empty when unset).

| Query | Notes |
|---|---|
| `format` | `yaml` (default), `json` or `env`. Other values return 400. |
| `effective` | `true` (default) exports the running values. `false` exports what is in the config file. |

The response is an attachment named `callfs-config.yaml`, `.json` or `.env`. The `env` format writes one `CALLFS_...=value` line per setting.

```bash
curl -H "Authorization: Bearer $KEY" "$CALLFS/v1/admin/config/export?format=env" -o callfs-config.env
```

---

## Cluster

These routes show and manage the cluster that shares the file index. They are what the console's Members and Replication pages use. Without `nats.enabled` and `metadata_store.type: nats`, `GET /cluster` still answers (with `mode` `single`) and `GET /cluster/snapshots` returns an empty list; the other routes return 409 `CONFLICT`.

### GET /cluster

```json
{
  "mode": "nats",
  "self_id": "node-1",
  "cluster_name": "callfs",
  "leader_id": "node-1",
  "leader_endpoint": "https://node-1.example.com",
  "replicas": 3,
  "has_quorum": true,
  "can_lose": 1,
  "log": {
    "first_seq": 1200, "last_seq": 5400, "messages": 4201, "bytes": 3145728, "applied_seq": 5400,
    "last_snapshot": { "id": "snap-5000", "seq": 5000, "bytes": 1048576, "created_at": "2026-03-01T09:00:00Z" },
    "snapshot_interval_ms": 3600000
  },
  "members": [
    { "node_id": "node-1", "instance_id": "node-1", "api_endpoint": "https://node-1.example.com", "state": "leader", "is_self": true, "online": true, "last_seen_ms": 0, "behind_by": 0 },
    { "node_id": "node-2", "instance_id": "node-2", "api_endpoint": "https://node-2.example.com", "state": "follower", "is_self": false, "online": true, "last_seen_ms": 320, "behind_by": 0 },
    { "node_id": "node-3", "instance_id": null, "api_endpoint": null, "state": "offline", "is_self": false, "online": false, "last_seen_ms": 45000, "behind_by": 12 }
  ]
}
```

| Field | Meaning |
|---|---|
| `mode` | `nats` for a cluster, `single` when this server runs without the shared file index log (then `members` holds only this server and `log` is empty) |
| `self_id`, `cluster_name` | This server's node name and `nats.cluster_name` |
| `leader_id`, `leader_endpoint` | The member that currently leads the file index log, and its public address when known. `null` when there is no leader. |
| `replicas` | Number of members |
| `has_quorum` | `true` when a majority of members is online and a leader exists |
| `can_lose` | How many more members can go offline before writes stop |
| `log.first_seq`, `log.last_seq`, `log.messages`, `log.bytes` | Position and size of the file index log |
| `log.applied_seq` | The last log entry this server has applied. Equal to `last_seq` when it is up to date. |
| `log.last_snapshot` | Newest snapshot (`id`, `seq`, `bytes`, `created_at`) or `null` |
| `log.snapshot_interval_ms` | `nats.metadata.snapshot_interval` in milliseconds |
| `members[].state` | `leader`, `follower` or `offline` (no contact for more than 10 seconds) |
| `members[].last_seen_ms`, `members[].behind_by` | Milliseconds since the member was last in contact, and how many log entries it is behind |
| `members[].instance_id`, `members[].api_endpoint` | Filled when the member is `self` or listed in `instance_discovery.peer_endpoints`, otherwise `null` |

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/cluster
```

### GET /cluster/state-stats

Counts what the file index holds.

```json
{
  "files": 1200, "directories": 85,
  "links": { "active": 3, "used": 10, "expired": 2, "revoked": 1 },
  "erasure_entries": 40, "index_entries": 1285
}
```

Status codes: 200, 409 in single mode.

### POST /cluster/leadership-transfer

Moves leadership of the file index log to another member.

Body (optional):

| Field | Type | Notes |
|---|---|---|
| `node_id` | string | Member to lead next. Without it the cluster picks one. Must not be empty when sent. |

Response `200`: `{"status": "transferred", "leader_id": "node-2"}`. `leader_id` is `null` while the new leader is being elected.

| Status | When |
|---|---|
| 200 | Leadership moved |
| 404 `NODE_NOT_FOUND` | `node_id` is not a member |
| 409 | Single mode, or the chosen member is offline |
| 422 | `node_id` is empty |

```bash
curl -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"node_id":"node-2"}' $CALLFS/v1/admin/cluster/leadership-transfer
```

### DELETE /cluster/nodes/{id}

Removes an offline member from the cluster. Response `200`: `{"status": "removed", "remaining": 2}`.

| Status | When |
|---|---|
| 200 | Member removed |
| 404 `NODE_NOT_FOUND` | `{id}` is not a member |
| 409 | Single mode, `{id}` is this server, or `{id}` is still online (stop it first) |
| 500 | The cluster did not accept the removal |
| 503 | The server has no messaging connection |

```bash
curl -X DELETE -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/cluster/nodes/node-3
```

### GET /cluster/snapshots

Lists the stored snapshots of the file index log.

```json
{ "retain": 3, "items": [ { "id": "snap-5000", "seq": 5000, "bytes": 1048576, "created_at": "2026-03-01T09:00:00Z" } ] }
```

`retain` is `nats.metadata.retain_snapshots`. Single mode returns an empty `items`.

### POST /cluster/snapshot

Takes a snapshot now. Response `200`: `{"id": "snap-5400", "seq": 5400, "bytes": 1049000, "took_ms": 180}`. Status codes: 200, 409 in single mode, 500 on failure.

```bash
curl -X POST -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/cluster/snapshot
```

### GET /cluster/join-config

Builds the settings a new server needs to join. The console's Add member dialog uses it.

| Query | Notes |
|---|---|
| `node_id` | Required. 1 to 128 letters, digits, dots, dashes or underscores. Must not be a member already. |
| `api_endpoint` | Required. The new server's public `http://` or `https://` address. |
| `cluster_addr` | Required. `host:port` the other servers use to reach the new server's cluster port. |

```json
{
  "yaml": "nats:\n  enabled: true\n  mode: \"embedded\"\n  server_name: \"node-4\"\n  ...",
  "steps": [
    "Copy auth.internal_proxy_secret, auth.single_use_link_secret and the API keys from an existing node into the new node's config. They are not included here.",
    "Open 10.0.0.14:6222 between the new node and the existing nodes, and give the cluster port the same TLS files as the other nodes when it is not a loopback address.",
    "Start the new node. It joins the metadata log and the lock bucket and catches up on its own.",
    "Add nats-route://10.0.0.14:6222 to nats.routes and node-4 to instance_discovery.peer_endpoints on the other nodes. They pick it up on their next restart."
  ]
}
```

`yaml` is a config snippet for the new server: the `nats` block (`mode`, `server_name`, `cluster_name`, `cluster_listen`, `routes`), `instance_discovery` (its `instance_id` and the `peer_endpoints` of every member plus the new one), `metadata_store.type: nats` and `dlm.type: nats`. Secrets are never included. Status codes: 200, 409 (single mode, or `node_id` already a member), 422 (invalid `node_id`, `api_endpoint` or `cluster_addr`, listed in `details.errors`).

```bash
curl -H "Authorization: Bearer $KEY" \
  "$CALLFS/v1/admin/cluster/join-config?node_id=node-4&api_endpoint=https://node-4.example.com&cluster_addr=10.0.0.14:6222"
```

---

## Cluster servers

A server entry is a server this one knows about: itself plus every entry in `instance_discovery.peer_endpoints`.

### GET /instances

```json
{
  "self": { "instance_id": "node-1", "external_url": "https://node-1.example.com", "started_at": "2026-03-01T09:00:00Z" },
  "items": [
    {
      "instance_id": "node-1", "endpoint": "https://node-1.example.com", "source": "self", "is_self": true,
      "scheme": "https", "reachable": null, "rtt_ms": null, "checked_at": null,
      "files": 1200, "bytes": 5368709120, "pieces": 0,
      "disk_free_bytes": 214748364800, "disk_total_bytes": 536870912000
    },
    {
      "instance_id": "node-2", "endpoint": "https://node-2.example.com", "source": "config", "is_self": false,
      "scheme": "https", "reachable": true, "rtt_ms": 4, "checked_at": "2026-03-01T10:00:00Z",
      "files": 900, "bytes": 3221225472, "pieces": 340,
      "disk_free_bytes": 107374182400, "disk_total_bytes": 268435456000
    }
  ]
}
```

`files` and `bytes` count the files stored on that server. `pieces` counts erasure-coded pieces it holds. `disk_free_bytes` and `disk_total_bytes` are the free and total space of that server's local storage disk. For other servers they come from the node announcements every server sends, and are `null` until a server has announced itself or when it has no local storage. `reachable`, `rtt_ms` and `checked_at` are `null` until the server has been probed (a background check runs every 30 seconds).

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/instances
```

### PUT /instances/{id}/endpoint

Adds or changes another server's address.

Body:

| Field | Type | Notes |
|---|---|---|
| `endpoint` | string | Required. `http://` or `https://` host address with no user, query or fragment. A trailing `/` is removed. |
| `persist` | boolean | `true` also saves it to `instance_discovery.peer_endpoints` in the config file. Default `false` (until restart). |

`{id}` is 1 to 128 letters, digits, dots, dashes or underscores. It cannot be this server's own ID (its address comes from `server.external_url`).

Response `200`: the server entry. Status codes: 400 invalid ID or endpoint, or own ID; 500 save failed.

```bash
curl -X PUT -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"endpoint":"https://node-3.example.com","persist":true}' \
  $CALLFS/v1/admin/instances/node-3/endpoint
```

### DELETE /instances/{id}/endpoint

Removes a server address (and from the config file if it is saved there). Response `200`: the server entry as it is now. Status code 400 for an invalid or own ID.

```bash
curl -X DELETE -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/instances/node-3/endpoint
```

### POST /instances/{id}/probe

Contacts the server now and records the result.

```json
{ "reachable": true, "rtt_ms": 4, "status_code": 200, "error": null }
```

A server counts as reachable when it answers with a status below 500. Otherwise `error` explains why. Status codes: 200, 404 `server not found`, 400 `server has no address to probe`.

```bash
curl -X POST -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/instances/node-2/probe
```

---

## Backends

### GET /backends

Lists the four storages: `localfs`, `s3`, `internal_proxy`, `erasure`.

```json
{
  "default": "localfs",
  "items": [
    {
      "name": "localfs", "enabled": true, "status": "ok",
      "root_path": "/var/lib/callfs", "free_bytes": 85899345920, "total_bytes": 107374182400, "used_bytes": 21474836480,
      "bucket": null, "region": null, "endpoint": null, "encryption": null, "acl": null, "kms_key_set": null,
      "peers": 0,
      "last_probe": { "at": "2026-03-01T10:00:00Z", "ok": true, "latency_ms": 9, "error": null }
    },
    {
      "name": "s3", "enabled": false, "status": "disabled",
      "root_path": null, "free_bytes": null, "total_bytes": null, "used_bytes": null,
      "bucket": null, "region": null, "endpoint": null, "encryption": null, "acl": null, "kms_key_set": false,
      "peers": 0, "last_probe": null
    }
  ]
}
```

`status` is `ok`, `warn` (local disk has less than 10 percent free), `down` (last probe failed, or the disk cannot be read) or `disabled`. `peers` counts the other servers for `internal_proxy` and `erasure`. S3 entries fill `bucket`, `region`, `endpoint`, `encryption`, `acl` and `kms_key_set`. `last_probe` is `null` until a probe ran.

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/backends
```

### POST /backends/{name}/probe

Writes, reads and deletes a small test file on the storage and reports the timings. `{name}` is `localfs`, `s3`, `internal_proxy` or `erasure`.

```json
{ "ok": true, "write_ms": 4, "read_ms": 2, "delete_ms": 3, "error": null }
```

| Status | When |
|---|---|
| 200 | Probe ran (check `ok`) |
| 404 | Unknown name |
| 409 | The storage is not enabled or not available |

```bash
curl -X POST -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/backends/s3/probe
```

---

## Erasure coding

Erasure coding splits a file into data and parity pieces spread over servers. See [Storage](05-storage.md).

### GET /erasure

```json
{
  "enabled": true,
  "data_shards": 4,
  "parity_shards": 2,
  "min_file_size": 1048576,
  "shard_storage": "localfs",
  "shard_path": "/var/lib/callfs",
  "placement": "round_robin",
  "instances": ["node-1", "node-2", "node-3"],
  "files": 120,
  "bytes": 5368709120,
  "raw_bytes": 8053063680,
  "health": { "ok": 118, "reduced": 1, "critical": 0, "unreadable": 0, "unknown": 1 },
  "pieces_by_instance": { "node-1": 240, "node-2": 240, "node-3": 240 }
}
```

`bytes` is the size of the files, `raw_bytes` is what the pieces take on disk. `health` counts files by their last check: `ok`, `reduced` (some pieces lost, still safe), `critical` (one more loss and data is gone), `unreadable`, `unknown` (not checked yet). `enabled` is false and the counts are zero when erasure coding is off.

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/erasure
```

### POST /erasure/plan

Previews how pieces would be placed for a layout, without changing anything.

Body:

| Field | Type | Notes |
|---|---|---|
| `data_shards` | integer | 1 to 256 |
| `parity_shards` | integer | 1 to 256. `data_shards` plus `parity_shards` is at most 256. |
| `instances` | array of strings | 1 to 256 server IDs, each 1 to 255 characters |

Response `200`:

```json
{
  "total_shards": 6,
  "assignments": [
    { "index": 0, "instance_id": "node-1", "kind": "data" },
    { "index": 4, "instance_id": "node-2", "kind": "parity" }
  ],
  "per_instance": { "node-1": 2, "node-2": 2, "node-3": 2 },
  "max_on_one_instance": 2,
  "tolerated_instance_failures": 1,
  "tolerated_piece_losses": 2,
  "overhead_ratio": 1.5,
  "fault_tolerant": true,
  "warnings": []
}
```

Status codes: 200, 404 `FEATURE_DISABLED` (erasure coding not running), 422 invalid fields.

```bash
curl -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"data_shards":4,"parity_shards":2,"instances":["node-1","node-2","node-3"]}' \
  $CALLFS/v1/admin/erasure/plan
```

### POST /erasure/verify-all

Starts a job (kind `erasure.verify_all`) that checks every erasure-coded file under a path.

Body (optional): `{"prefix": "/projects"}`. Default `/`. The prefix must start with `/` and cannot contain `..`.

Response `202`: `{"job_id": "..."}`. When done the job result is `{"files": 120, "failed": 1}`; each failed file is listed in the job's `failed_items` with code `VERIFY_FAILED`.

Status codes: 202, 404 `FEATURE_DISABLED`, 422 invalid prefix.

```bash
curl -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"prefix":"/projects"}' $CALLFS/v1/admin/erasure/verify-all
```

---

## Backup copies and cleanup

### GET /replication

Shows backup-copy (replication) settings and recent failures.

```json
{
  "enabled": true,
  "replica_storage": "s3",
  "require_success": false,
  "counts_24h": { "ok": 340, "failed": 2, "skipped": 5 },
  "failures": [ { "at": "2026-03-01T09:55:00Z", "path": "/docs/report.pdf", "error": "connection reset" } ]
}
```

`counts_24h` holds the number of backup copies that succeeded (`ok`), failed and were skipped in the last 24 hours.

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/replication
```

### POST /replication/retry

Starts a job (kind `replication.retry`) that copies files to the backup storage again.

Body (optional): `{"paths": ["/docs/report.pdf"]}`. Up to 1000 paths, each starting with `/` and at most 4096 bytes. Without `paths` it retries every path in the recent failure list.

Response `202`: `{"job_id": "..."}`. Job result: `{"requested": 2, "copied": 1, "failed": 1}`.

Status codes: 202, 400 too many or invalid paths.

```bash
curl -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{}' $CALLFS/v1/admin/replication/retry
```

### GET /staging

Lists leftover temporary files on the local disk: unfinished uploads, staging files and test files.

```json
{
  "items": [
    { "storage": "localfs", "path": "/.callfs-uploads/9f2c", "kind": "upload_session", "bytes": 10485760, "modified": "2026-02-27T09:00:00Z", "age_s": 172800 }
  ],
  "total_bytes": 10485760
}
```

`kind` is `staging`, `upload_session`, `probe` or `tmp`. At most 1000 items are listed; `total_bytes` covers all of them.

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/staging
```

### DELETE /staging

Starts a job (kind `staging.clean`) that deletes leftover temporary files.

| Query | Notes |
|---|---|
| `older_than_s` | Only delete entries at least this many seconds old. Default `86400`, from `0` to `315360000`. |

Response `202`: `{"job_id": "..."}`. Job result: `{"removed": 3, "freed_bytes": 31457280}`.

Status codes: 202, 400 invalid `older_than_s`, 409 local disk storage is not enabled.

```bash
curl -X DELETE -H "Authorization: Bearer $KEY" "$CALLFS/v1/admin/staging?older_than_s=3600"
```

### GET /hygiene

Counts things worth cleaning up.

```json
{
  "files_without_owner": 0,
  "files_without_etag": 12,
  "orphan_pieces": null,
  "stale_erasure_records": 0,
  "staging": { "count": 3, "bytes": 31457280 },
  "migrations": { "driver": "postgres", "version": null, "dirty": null, "latest": null }
}
```

`stale_erasure_records` is `null` when the file index does not hold erasure records. For the schema state use [`GET /migrations`](#get-migrations).

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/hygiene
```

### POST /retag

Starts a job (kind `retag`) that gives a version tag (ETag) to every file that has none, by reading the file and hashing it.

Body (optional): `{"prefix": "/projects"}`. Default `/`. Must start with `/`, at most 4096 bytes.

Response `202`: `{"job_id": "..."}`. Job result: `{"tagged": 10, "skipped": 1, "failed": 1}`. Failures appear in `failed_items` with code `RETAG_FAILED`.

Status codes: 202, 400 invalid prefix.

```bash
curl -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"prefix":"/projects"}' $CALLFS/v1/admin/retag
```

---

## Locks, cache and rate limits

### GET /locks

Lists edit locks held on this server and lock counters.

| Query | Notes |
|---|---|
| `path` | Only the lock on exactly this file path. |
| `limit` | 1 to 1000. Default 200. Locks are sorted by key. |

```json
{
  "type": "local",
  "wait_timeout_ms": 5000,
  "ttl_ms": 30000,
  "held": [
    { "key": "path:/docs/report.docx", "path": "/docs/report.docx", "owner_hint": "node-1", "acquired_at": "2026-03-01T10:00:00Z", "expires_at": "2026-03-01T10:00:30Z" }
  ],
  "stats": { "acquired": 120, "timeouts": 1, "cancelled": 0, "errors": 0, "lost": 0, "renew_errors": 0, "wait_p50_ms": 2, "wait_p95_ms": 40 }
}
```

`type` is `redis`, `nats` or `local`, following `dlm.type`. With `nats` the held list covers locks held anywhere in the cluster. `wait_timeout_ms` is `dlm.wait_timeout`. `wait_p50_ms` and `wait_p95_ms` are the median and 95th percentile time spent waiting for a lock, for every lock type; `null` until a lock has been waited for.

Status codes: 200, 400 invalid `limit`, 404 `FEATURE_DISABLED` (locks not running).

```bash
curl -H "Authorization: Bearer $KEY" "$CALLFS/v1/admin/locks?path=/docs/report.docx"
```

### DELETE /locks/{key}

Releases a lock by force, for example one held by a client that crashed. `{key}` is the `key` from the list, URL-encoded (`path%3A%2Fdocs%2Freport.docx`).

Body:

| Field | Type | Notes |
|---|---|---|
| `reason` | string | Required, 1 to 500 characters. Written to the log. |

Response `200`: `{"released": true}`. `released` is false when no such lock was held.

Status codes: 200, 400 missing key, 404 `FEATURE_DISABLED`, 422 missing or too long `reason`.

```bash
curl -X DELETE -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"reason":"editor crashed"}' "$CALLFS/v1/admin/locks/path%3A%2Fdocs%2Freport.docx"
```

### GET /cache

Shows the file-information cache.

```json
{
  "enabled": true,
  "ttl_ms": 300000,
  "entries": 812,
  "max_entries": 10000,
  "hits": 54021,
  "misses": 1210,
  "evictions": 0,
  "expired_cleaned": 33,
  "last_cleanup_at": "2026-03-01T10:00:00Z"
}
```

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/cache
```

### POST /cache/invalidate

Drops cached file information.

Body: either `{"prefix": "/projects"}` or `{"all": true}`, not both. A prefix must start with `/`, cannot contain `..`, at most 4096 bytes.

Response `200`: `{"invalidated": 57}`. Status codes: 200, 422 when neither or both are set or the prefix is invalid.

```bash
curl -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"all":true}' $CALLFS/v1/admin/cache/invalidate
```

### GET /ratelimits

Shows the three rate limits and who was rejected.

```json
{
  "limits": [
    { "endpoint": "download", "rps": 10, "burst": 20 },
    { "endpoint": "link_generate", "rps": 5, "burst": 10 },
    { "endpoint": "session", "rps": 1, "burst": 5 }
  ],
  "tracked_clients": 14,
  "max_entries": 10000,
  "ttl_s": 600,
  "rejections_total": { "download": 4, "link_generate": 0, "session": 1 },
  "evictions_total": 0,
  "top_clients": [ { "client": "203.0.113.7", "endpoint": "download", "rejections": 4, "last_at": "2026-03-01T09:58:00Z" } ]
}
```

Change the limits with the `server.rate_limit.*` settings through [`PATCH /config/runtime`](#patch-configruntime).

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/ratelimits
```

---

## Users and keys

Users are the API-key users in `auth.api_key_users` (user ID to key) and the admins in `auth.admin_users`. These routes edit those settings in the config file and apply them at once, so a config file must be in use. App users are separate from operating-system users.

A **numbered key** is an old-style entry in `auth.api_keys` that has no user name (shown as `api-user-0`, `api-user-1`, ...). Migrate them to named users with [`POST /users/migrate-positional`](#post-usersmigrate-positional).

User IDs are 1 to 64 letters, digits, dots, dashes or underscores. `root`, `internal-proxy` and names starting with `api-user-` are reserved.

### GET /users

```json
{
  "items": [
    {
      "user_id": "alice",
      "source": "api_key_users",
      "index": null,
      "key_hint": "…x9Qa",
      "key_fingerprint": "3f9a1c0be2d74a55",
      "is_admin": true,
      "owned_files": 340,
      "owned_bytes": 1073741824,
      "last_auth_at": "2026-03-01T09:58:00Z",
      "requests_24h": 128,
      "auth_failures_24h": null
    }
  ],
  "positional_count": 0
}
```

`source` is `api_key_users` (named user), `api_keys` (numbered key, `index` is its position) or `reserved` (`root`, `internal-proxy`). `key_hint` shows only the last characters of the key. `positional_count` is the number of numbered keys.

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/users
```

### POST /users

Creates a user with a new random API key.

Body: `{"user_id": "bob", "is_admin": false}`. `is_admin` is optional.

Response `201`. This is the only time the key is shown:

```json
{ "user_id": "bob", "api_key": "Zm9vYmFyLWV4YW1wbGUta2V5LTMyYnl0ZXM" }
```

Status codes: 201, 409 `user already exists`, 422 invalid `user_id`.

```bash
curl -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"user_id":"bob","is_admin":false}' $CALLFS/v1/admin/users
```

### PATCH /users/{id}

Makes a user an admin or removes admin rights.

Body: `{"is_admin": true}` (required). Response `200`: the user item.

| Status | When |
|---|---|
| 404 | Unknown user |
| 409 | `root` (always an admin); numbered or reserved users (only users with their own key can be admins); removing the last admin |
| 422 | `is_admin` missing |

```bash
curl -X PATCH -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"is_admin":true}' $CALLFS/v1/admin/users/bob
```

### DELETE /users/{id}

Deletes a named user and their key. Their files stay and keep their owner name. Response `204`.

| Status | When |
|---|---|
| 404 | Unknown user |
| 409 | Numbered or reserved user, or the last admin |

```bash
curl -X DELETE -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/users/bob
```

### POST /users/{id}/rotate

Replaces a user's key with a new random one. The old key stops working at once. Response `200`: `{"user_id":"bob","api_key":"..."}`. Status codes: 404, 409 (numbered or reserved user).

```bash
curl -X POST -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/users/bob/rotate
```

### POST /users/migrate-positional

Turns numbered keys into named users. The keys keep working. Files owned by the numbered identity are moved to the new user unless you turn that off.

Body:

| Field | Type | Notes |
|---|---|---|
| `mapping` | array | Required, at least one entry. Each entry is `{"index": 0, "user_id": "alice"}`: the numbered key's `index` and the new user name. |
| `reassign_owner` | boolean | Default `true`. |

Response `202`: the job (kind `owner_reassign`). Job result: `{"keys_migrated": 1, "files_reassigned": 340}`.

Status codes: 202, 422 (no mapping, unknown or repeated index, repeated or invalid or existing `user_id`; each problem is listed in `details.errors` with a path like `mapping[0].index`).

```bash
curl -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"mapping":[{"index":0,"user_id":"alice"}],"reassign_owner":true}' \
  $CALLFS/v1/admin/users/migrate-positional
```

### GET /owners

Lists file owners with their totals.

| Query | Notes |
|---|---|
| `limit` | 1 to 1000. Default 200. |
| `cursor` | From `next_cursor`. Up to 1024 characters. |

```json
{ "items": [ { "owner": "alice", "files": 340, "bytes": 1073741824 } ], "next_cursor": null }
```

```bash
curl -H "Authorization: Bearer $KEY" "$CALLFS/v1/admin/owners?limit=50"
```

### POST /owners/reassign

Starts a job (kind `owner_reassign`) that changes the owner of files.

Body:

| Field | Type | Notes |
|---|---|---|
| `from` | string | Current owner, up to 256 bytes. May be empty to pick up files with no owner. |
| `to` | string | Required. A valid user ID (see above), different from `from`. |
| `prefix` | string | Only files under this path. Default `/`. Must start with `/`, no `..`, at most 4096 bytes. |

Response `202`: the job. Job result: `{"files_reassigned": 12}`. Status code 422 for invalid fields.

```bash
curl -X POST -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"from":"api-user-0","to":"alice","prefix":"/projects"}' $CALLFS/v1/admin/owners/reassign
```

### GET /auth/failures

Lists recent failed sign-in attempts.

| Query | Notes |
|---|---|
| `since` | RFC 3339 time. Default: 24 hours ago. |
| `limit` | 1 to 1000. Default 200. |

```json
{
  "total_24h": 3,
  "items": [ { "at": "2026-03-01T09:50:00Z", "client": "203.0.113.7", "reason": "wrong_key", "path": "/v1/files/" } ]
}
```

`reason` values include `wrong_key`, `blocked` and `bad_delegation`. Status code 400 for an invalid `since` or `limit`.

```bash
curl -H "Authorization: Bearer $KEY" "$CALLFS/v1/admin/auth/failures?since=2026-03-01T00:00:00Z"
```

---

## Telemetry

Logs, audit records and events are kept in memory on each server, newest first, up to the sizes set by `ui.log_buffer_entries`, `ui.audit_buffer_entries` and `ui.event_buffer_entries`. They are lost on restart. Each server answers for itself; to read another server's data use the [node gateway](#node-gateway).

### GET /metrics

Returns a snapshot of the metrics the console charts.

| Query | Notes |
|---|---|
| `names` | Comma-separated metric names. Default: all. |

```json
{
  "generated_at": "2026-03-01T10:00:00Z",
  "families": [
    {
      "name": "callfs_http_requests_total",
      "type": "counter",
      "help": "Total number of HTTP requests",
      "emitted": true,
      "series": [ { "labels": { "method": "GET", "status": "200" }, "value": 5120 } ]
    }
  ]
}
```

`type` is `counter`, `gauge`, `histogram`, `summary` or `untyped`. Counters and gauges have `value`; histograms have `count`, `sum` and `buckets`. This is the console's view. The Prometheus scrape endpoint is described in [Monitoring and metrics](06-monitoring-metrics.md).

Status codes: 200, 503 metrics sampler not running.

```bash
curl -H "Authorization: Bearer $KEY" "$CALLFS/v1/admin/metrics?names=callfs_http_requests_total"
```

### GET /metrics/series

Returns one metric over time, for charts.

| Query | Notes |
|---|---|
| `name` | Required. Metric name. |
| `measure` | `value` (default), `rate`, `total`, `avg`, `max`, `p50`, `p95`, `p99`. Counters take `rate`, `total`, `value`; gauges take `value`, `max`, `avg`; histograms take all but their meaning is per bucket window. Other combinations return 400. |
| `by` | Comma-separated labels to split into separate lines. |
| `filter` | `label=value`. Repeat for several. |
| `from`, `to` | Unix seconds. Default: the last hour. |
| `step_s` | Seconds per point. Default 60, above 0 and at most 2678400. The range may not make more than 10000 points. |

```json
{
  "name": "callfs_http_requests_total",
  "measure": "rate",
  "unit": "count/s",
  "step_s": 60,
  "series": [
    { "labels": { "method": "GET" }, "points": [ [1772359200, 12.5], [1772359260, null] ] }
  ],
  "sampled_since": "2026-03-01T09:00:00Z"
}
```

Each point is `[unix_seconds, value]`. `value` is `null` when there is no data for that step.

Status codes: 200, 400 invalid query, 404 unknown metric, 503 sampler not running.

```bash
curl -H "Authorization: Bearer $KEY" \
  "$CALLFS/v1/admin/metrics/series?name=callfs_http_requests_total&measure=rate&by=status&step_s=60"
```

### GET /logs

Reads the server log, newest first. In a cluster the log holds the entries of every server whose logs are shared (see `nats.streams.logs_level`); each entry from a server carries its ID in `node`.

| Query | Notes |
|---|---|
| `level` | Minimum level: `debug`, `info`, `warn`, `error`. Default: all. |
| `q` | Text the message must contain (not case sensitive). |
| `request_id` | Only entries of this request. |
| `since` | RFC 3339 time. |
| `limit` | 1 to 1000. Default 200. |
| `cursor` | From `next_cursor`. |

```json
{
  "items": [
    {
      "id": 4821,
      "at": "2026-03-01T10:00:00Z",
      "level": "info",
      "message": "Backend probed",
      "logger": "",
      "node": "node-1",
      "fields": { "backend": "s3", "ok": true },
      "request_id": "b7a1c3e0"
    }
  ],
  "next_cursor": "NDgyMA"
}
```

Status codes: 200, 400 invalid query, 503 telemetry not running.

```bash
curl -H "Authorization: Bearer $KEY" "$CALLFS/v1/admin/logs?level=warn&q=replication&limit=50"
```

### GET /logs/stream

A live log feed as server-sent events. It takes the same `level`, `q`, `request_id` and `since` filters as `/logs`.

See [Stream format](#stream-format). Each log line is an event named `log` whose `data` is a log entry as above:

```
id: 4822
event: log
data: {"id":4822,"at":"2026-03-01T10:00:01Z","level":"warn","message":"Replication failed","logger":"","fields":{"path":"/docs/report.pdf"},"request_id":null}

```

```bash
curl -N -H "Authorization: Bearer $KEY" "$CALLFS/v1/admin/logs/stream?level=warn"
```

### GET /audit

Reads the request audit trail, newest first. Every call to the public API is recorded, and every admin call that changes something. Admin reads (GET) are not recorded.

| Query | Notes |
|---|---|
| `since`, `until` | RFC 3339 times. |
| `user` | Only this user ID. |
| `op` | `read`, `list`, `stat`, `upload`, `replace`, `rename`, `move`, `delete`, `trash`, `restore`, `link_create`, `link_open`, `session`, `admin` (admin calls that change something). |
| `path` | Only paths starting with this prefix. |
| `status` | An HTTP status, optionally with an operator: `404`, `>=400`, `<500`, `>300`, `<=299`. |
| `request_id` | Only this request. |
| `limit` | 1 to 1000. Default 200. |
| `cursor` | From `next_cursor`. |
| `format` | `json` (default) or `csv`. |

```json
{
  "items": [
    {
      "at": "2026-03-01T10:00:00Z",
      "request_id": "b7a1c3e0",
      "user_id": "alice",
      "method": "PUT",
      "route": "/v1/files/*",
      "path": "docs/report.pdf",
      "op": "upload",
      "status": 201,
      "bytes_in": 1048576,
      "bytes_out": 0,
      "duration_ms": 38,
      "instance_id": "node-1",
      "client": "203.0.113.7"
    }
  ],
  "next_cursor": null
}
```

With `format=csv` the response is an attachment `audit.csv` with the columns `at, request_id, user_id, method, route, path, op, status, bytes_in, bytes_out, duration_ms, instance_id, client`. A cell that starts with `=`, `+`, `-`, `@`, a tab or a carriage return gets a leading `'` so spreadsheets do not run it.

Status codes: 200, 400 invalid filter or format, 503 telemetry not running.

```bash
curl -H "Authorization: Bearer $KEY" "$CALLFS/v1/admin/audit?user=alice&status=>=400&format=csv" -o audit.csv
```

### GET /events

A live feed of server events as server-sent events: job changes, file and link activity, and security notices.

| Query | Notes |
|---|---|
| `types` | Comma-separated event types. Default: all. |

See [Stream format](#stream-format). Event types:

| Type | When |
|---|---|
| `job.queued` | A job was created |
| `job.running` | A job started |
| `job.progress` | A job advanced (sent at most about once a second) |
| `job.cancelling` | A cancel was requested |
| `job.succeeded`, `job.partial`, `job.failed`, `job.cancelled` | A job finished |
| `file.created`, `file.updated`, `file.renamed`, `file.moved`, `file.deleted`, `file.trashed`, `file.restored` | A file was uploaded, replaced, renamed, moved, deleted, moved to trash or restored. `actor` is the user, `entities` holds the path. |
| `link.created`, `link.opened` | A single-use link was created or opened |
| `link.failed` | A link could not be opened |
| `security.auth_failed` | A request with credentials was rejected with 401 |
| `security.rate_limited` | A request was rejected with 429 |

An event's `data`:

```json
{
  "id": 77,
  "at": "2026-03-01T10:00:00Z",
  "type": "job.progress",
  "level": "info",
  "actor": null,
  "instance_id": "node-1",
  "message": "job.progress",
  "entities": [ { "kind": "job", "id": "j_01" } ],
  "fields": { "payload": { "id": "j_01", "kind": "retag", "state": "running", "progress": { "done": 40, "total": 100, "failed": 0 } } }
}
```

For job events `fields.payload` is the full job object (see [Jobs](#jobs-and-workers)). File, link and security events carry `fields.route`, `fields.status` and `fields.request_id` instead.

```bash
curl -N -H "Authorization: Bearer $KEY" "$CALLFS/v1/admin/events?types=job.succeeded,job.failed"
```

### GET /events/history

Returns recent events, oldest first.

| Query | Notes |
|---|---|
| `types` | Comma-separated event types. |
| `since` | RFC 3339 time. |
| `limit` | 1 to 1000. Default 200. |

Response `200`: `{"items": [ <event>, ... ]}`. Status codes: 200, 400 invalid query, 503.

```bash
curl -H "Authorization: Bearer $KEY" "$CALLFS/v1/admin/events/history?limit=20"
```

### GET /transfers

Lists file transfers running right now.

```json
{
  "items": [
    { "id": "t_01", "user_id": "alice", "kind": "upload", "path": "/videos/talk.mp4", "bytes_done": 52428800, "bytes_total": 209715200, "started_at": "2026-03-01T10:00:00Z", "client": "203.0.113.7" }
  ]
}
```

`bytes_total` is `null` when the size is not known. `items` is an empty list when nothing is running.

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/transfers
```

### Stream format

`/events` and `/logs/stream` answer with `Content-Type: text/event-stream` and stay open until you close the connection. Each message is:

```
id: <number>
event: <type>
data: <one line of JSON>

```

- `id` grows with each message on this server. If the connection drops, reconnect with the header `Last-Event-ID: <last id you saw>` and the server first replays what you missed (up to 1000 messages still in memory) and then continues live. Browser `EventSource` does this by itself.
- `event` is the event type for `/events` (`job.progress`, ...) and `log` for `/logs/stream`.
- A comment line `: heartbeat` arrives every 15 seconds so proxies keep the connection open. Ignore lines that start with `:`.
- A slow reader may skip messages rather than hold the server back.
- Use `curl -N` to stop curl from buffering.
- The filters in the query are fixed when the stream opens. Open a new stream to change them.

---

## Jobs and workers

A job is a background task. Jobs are started by admin routes (see above), by the public API (batch operations) and by workers.

### Job object

```json
{
  "id": "j_01",
  "kind": "retag",
  "node": "node-1",
  "state": "running",
  "created_by": "alice",
  "retry_of": "",
  "created_at": "2026-03-01T10:00:00Z",
  "started_at": "2026-03-01T10:00:01Z",
  "finished_at": null,
  "progress": { "done": 40, "total": 100, "failed": 1 },
  "failed_items": [ { "path": "/docs/a.txt", "code": "RETAG_FAILED", "reason": "read error" } ],
  "result": null,
  "error": null
}
```

`node` is the server that runs the job. `state` is `queued`, `running`, `cancelling`, `succeeded`, `partial` (some items failed), `failed` or `cancelled`. `retry_of` appears on a job created by `retry-failed`. `failed_items` keeps up to 1000 entries. `result` holds the job's own summary once it finished.

### GET /jobs

| Query | Notes |
|---|---|
| `state` | Comma-separated states to include. |
| `kind` | Only this kind, up to 128 characters. |
| `created_by` | Only jobs of this user, up to 128 characters. |
| `limit` | 1 to 1000. Default 200. |
| `cursor` | From `next_cursor`. |

Response `200`: `{"items": [ <job>, ... ], "next_cursor": null}`. Status code 400 for an invalid state, limit or cursor.

```bash
curl -H "Authorization: Bearer $KEY" "$CALLFS/v1/admin/jobs?state=running,queued"
```

### GET /jobs/{id}

Response `200`: the job. Status code 404 `job not found`.

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/jobs/j_01
```

### POST /jobs/{id}/cancel

Asks a job to stop. Response `200`: the job (usually `cancelling`). Status codes: 404, 409 when the job already finished.

```bash
curl -X POST -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/jobs/j_01/cancel
```

### POST /jobs/{id}/retry-failed

Starts a new job that repeats only the items that failed. Response `202`: the new job (its `retry_of` is the old ID). Status codes: 404, 409 when the job is still running or has no failed items.

```bash
curl -X POST -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/jobs/j_01/retry-failed
```

### GET /tasks/dead

Lists failed background tasks (tasks that ran out of retries), newest first. Admin only.

| Query | Notes |
|---|---|
| `limit` | 1 to 500. Default 100. |
| `before` | Task ID from `next_before`; returns older tasks. |

Response `200`: `{"items": [{"id": 12, "pool": "replicate", "reason": "max deliveries exceeded", "failed_at": "2026-01-01T10:00:00Z", "summary": "/docs/a.txt"}], "next_before": null}`. `next_before` is set when more items follow. With NATS off the list is empty. Status code 400 for an invalid limit or `before`.

```bash
curl -H "Authorization: Bearer $KEY" "$CALLFS/v1/admin/tasks/dead?limit=50"
```

### POST /tasks/dead/{id}/retry

Puts the failed task back in the server queue it came from. Response `204`. Status codes: 400 bad ID, 404 unknown task, 409 when NATS is off.

```bash
curl -X POST -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/tasks/dead/12/retry
```

### DELETE /tasks/dead/{id}

Drops the failed task. Response `204`. Status codes: 400, 404, 409 when NATS is off.

```bash
curl -X DELETE -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/tasks/dead/12
```

### GET /workers

Lists the workers that run on a schedule.

```json
{
  "items": [
    {
      "name": "usage_refresh",
      "interval_s": 600,
      "scope": "cluster",
      "lease_held": true,
      "running": false,
      "last_run_at": "2026-03-01T09:50:00Z",
      "last_duration_ms": 412,
      "last_result": { "path": "/", "files": 1200, "bytes": 5368709120, "refreshed_at": "2026-03-01T09:50:00Z" },
      "last_error": null,
      "next_run_at": "2026-03-01T10:00:00Z"
    }
  ]
}
```

`scope` is `node` (runs on every server) or `cluster` (runs on one server at a time, the one holding the worker's lease; `lease_held` is `true` on that server). Without cluster messaging every worker runs on this server.

| Worker | Scope | Every | What it does |
|---|---|---|---|
| `link_cleanup` | cluster | 5 min | Removes expired download links |
| `upload_cleanup` | node | 10 min | Removes abandoned upload sessions |
| `instance_prober` | node | 30 s | Probes the other servers |
| `metrics_sampler` | node | `ui.metrics_sample_interval` | Samples metrics for the console charts |
| `usage_refresh` | cluster | 10 min | Recounts files and bytes per storage (feeds `callfs_usage_files`, `callfs_usage_bytes` and the usage numbers) |
| `trash_cleanup` | cluster | 10 min | Empties trash older than `trash.retention` (only when `trash.enabled` is true) |

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/workers
```

### POST /workers/{name}/run

Runs a worker now. It becomes a job whose `kind` is the worker name. A `cluster` worker runs on the server that holds its lease, so calling this on another server forwards the run there; the job's `node` shows where it ran. Response `202`: `{"job_id": "..."}`. Status codes: 404 `worker not found`, 409 `worker is already running` (also `worker is already running on another server` when the lease holder cannot be reached from here).

```bash
curl -X POST -H "Authorization: Bearer $KEY" $CALLFS/v1/admin/workers/usage_refresh/run
```

---

## Node gateway

In a cluster you do not need a separate sign-in per server. Sign in to one server and use the node gateway to call any other. The console's server picker uses it.

A node ID is a key of `instance_discovery.peer_endpoints` (or this server's own `instance_discovery.instance_id`).

### GET /v1/nodes

Any signed-in user may call it. Lists this server and every peer with a live check.

```json
{
  "self": "node-1",
  "nodes": [
    { "id": "node-1", "endpoint": "https://node-1.example.com", "self": true, "status": "up", "latency_ms": 0,
      "role": "leader", "leader_id": "node-1", "version": "2.0.0", "uptime_s": 5400, "started_at": "2026-03-01T09:00:00Z" },
    { "id": "node-2", "endpoint": "https://node-2.example.com", "self": false, "status": "up", "latency_ms": 5,
      "role": "follower", "leader_id": "node-1", "version": "2.0.0", "uptime_s": 5300, "started_at": "2026-03-01T09:01:30Z" },
    { "id": "node-3", "endpoint": "https://node-3.example.com", "self": false, "status": "down", "latency_ms": 2001,
      "detail": "Get \"https://node-3.example.com/health\": context deadline exceeded" }
  ]
}
```

- `status` is `up` or `down`. A peer is checked through its `/health` with a 2 second timeout; `detail` explains a failure.
- `role` is `leader`, `follower` or `offline`; `leader_id` names the current leader. Both appear when the cluster reports them. A peer with no recent announcement is `down`, `role` `offline` and `detail` `no presence heartbeat`, without a health check.
- `version`, `uptime_s` and `started_at` are filled only when the caller is an admin.
- Peers are listed in ID order after this server. Servers that announced themselves to the cluster are listed too, even when they are not in `instance_discovery.peer_endpoints`, and can be reached through the gateway.

```bash
curl -H "Authorization: Bearer $KEY" $CALLFS/v1/nodes
```

### ANY /v1/nodes/{node}/{path}

Sends the request to `{node}` and returns its answer. `{path}` is the path on that server, written in full:

| You call | The target server receives |
|---|---|
| `GET /v1/nodes/node-2/v1/admin/status` | `GET /v1/admin/status` |
| `GET /v1/nodes/node-2/v1/files/docs/` | `GET /v1/files/docs/` |
| `GET /v1/nodes/node-2/health` | `GET /health` |
| `POST /v1/nodes/node-1/v1/admin/jobs/j_01/cancel` | handled by node-1 itself |

Rules:

- Any method works. Query string, body and headers go through.
- Allowed paths: `/health`, `/health/ready`, and anything under `/v1/`. Not allowed: `/v1/session`, `/v1/nodes` and paths with `..`.
- Use your own node ID to run the call on this server. This is the same as calling it directly.
- The target runs the request as the same user. Admin routes need that user to be an admin on the target server too.
- Your browser cookie and CSRF token are not sent to the target; the servers trust each other through the shared `auth.internal_proxy_secret`, which must be the same on every server.
- A request can be forwarded only once; a forwarded request cannot be forwarded again.
- Streams (`/events`, `/logs/stream`) and large downloads pass through without buffering.
- Responses carry `X-CallFS-Node: <node id>`. `Set-Cookie` headers from the target are removed.

| Status | Code | When |
|---|---|---|
| 400 | `BAD_REQUEST` | Path not allowed |
| 401 | `UNAUTHORIZED` | Not signed in |
| 404 | `NODE_NOT_FOUND` | Unknown node ID |
| 502 | `NODE_UNREACHABLE` | The target did not answer |
| 508 | `FORWARD_LOOP` | The request was already forwarded once |
| 503 | `NOT_READY` | This server is still starting |

Any other status comes from the target server.

Read the log of node-2, and cancel a job running on node-3:

```bash
curl -H "Authorization: Bearer $KEY" "$CALLFS/v1/nodes/node-2/v1/admin/logs?level=error&limit=20"
curl -X POST -H "Authorization: Bearer $KEY" $CALLFS/v1/nodes/node-3/v1/admin/jobs/j_01/cancel
curl -N -H "Authorization: Bearer $KEY" "$CALLFS/v1/nodes/node-2/v1/admin/events"
```
