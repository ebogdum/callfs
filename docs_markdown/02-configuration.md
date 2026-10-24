# Configuration

This page lists every CallFS setting: its type, default, allowed values, whether a change needs a restart, when it is required, what it does and how it is checked. It also explains the config file, environment variables, secrets, the web console settings pages and `callfs config validate`.

Contents:

- [How CallFS reads its configuration](#how-callfs-reads-its-configuration)
- [Value formats](#value-formats)
- [Environment variables](#environment-variables)
- [Secrets](#secrets)
- [Editing settings in the web console](#editing-settings-in-the-web-console)
- [Check a config with `callfs config validate`](#check-a-config-with-callfs-config-validate)
- [How to read the reference tables](#how-to-read-the-reference-tables)
- Settings reference: [server](#server), [rate limits](#serverrate_limit), [auth](#auth), [log](#log), [metrics](#metrics), [performance](#performance), [backend](#backend), [metadata_store](#metadata_store), [nats](#nats), [dlm](#dlm), [ha](#ha), [instance_discovery](#instance_discovery), [erasure](#erasure), [ui](#ui), [trash](#trash), [log mode](#log-mode-environment-only)
- [Example: single node](#example-single-node)
- [Example: 3-node NATS cluster](#example-3-node-nats-cluster)
- [Checklist of every setting](#checklist-of-every-setting)

## How CallFS reads its configuration

CallFS builds its settings from three sources. A higher source overrides a lower one, key by key:

1. Environment variables (highest).
2. The config file.
3. Built-in defaults (lowest).

### Config file format and lookup

- The file is YAML (`.yaml` or `.yml`) or JSON (`.json`). The file name must end in one of those.
- Pass a file with `--config` (short form `-c`). It works on `callfs server`, `callfs config validate` and `callfs healthcheck`. If the file does not exist, CallFS exits with `specified config file ... not found`.
- Without `--config`, CallFS looks in the current directory for `config.yaml`, then `config.yml`, then `config.json`, and uses the first one it finds.
- If no file is found, CallFS runs on defaults plus environment variables. Several settings have no usable default (API keys, the two secrets, `instance_discovery.instance_id`, and the metadata store connection), so a start without a file works only when you supply them through environment variables.
- Keys CallFS does not know are ignored, so check spelling with `callfs config validate` and the console Settings pages.
- A config that contains a `raft` section, `metadata_store.type: raft` or `dlm.type: raft`, or any `CALLFS_RAFT_*` variable is rejected at startup. Raft cluster mode was removed in 2.0.0. Use `metadata_store.type: nats` and `dlm.type: nats`.
- Running `callfs` with no arguments is the same as `callfs server`.

Settings are grouped into sections. In YAML a section is a nested block; in these tables a key is written with dots, for example `server.rate_limit.download_rps`:

```yaml
server:
  rate_limit:
    download_rps: 10
```

CallFS reads the config file at startup. Settings marked `live` in the tables below can also change while CallFS runs, from the web console. Everything else takes effect after a restart.

## Value formats

| Type | How to write it | Example |
| --- | --- | --- |
| `string` | Text. | `"callfs-1"` |
| `bool` | `true` or `false`. | `true` |
| `int` | Whole number. | `1000` |
| `float` | Number with an optional fraction. | `7.5` |
| `enum` | One of the listed words. Words are lower case. | `"strict"` |
| `duration` | A number with a unit: `ms`, `s`, `m`, `h`, and combinations. There is no `d` unit; use hours. | `30s`, `5m`, `1h30m`, `720h` |
| `bytes` | A whole number of bytes, or a number with a unit: `KiB`, `MiB`, `GiB`, `TiB` (powers of 1024), `KB`, `MB`, `GB`, `TB` (powers of 1000), or `B`. Fractions are allowed (`1.5GiB`). | `8MiB`, `1073741824` |
| `hostport` | `host:port`. The host may be empty to listen on every interface. | `:8443`, `10.0.0.1:6222` |
| `url` | A URL, or for `server.external_url` an address. | `https://files.example.com` |
| `path` | A file or folder path. Relative paths start in the folder where CallFS runs. | `/var/lib/callfs` |
| `list` | A YAML list. In an environment variable, a comma-separated string. | `["10.0.0.0/24", "10.1.0.0/24"]` |
| `map` | A YAML mapping of names to values. In environment variables, one variable per entry. | `{alice: "key"}` |
| `secret` | Text that CallFS never shows back. See [Secrets](#secrets). | |

In the tables, `""` means an empty string, `[]` an empty list, `{}` an empty map and "unset" means there is no default value.

## Environment variables

Every setting in the tables can be set with an environment variable. The name is built from the key:

1. Start with `CALLFS_`.
2. Write the key in upper case.
3. Replace every `.` with two underscores `__`.
4. Keep single underscores that are part of a key name.

| Key | Environment variable |
| --- | --- |
| `server.listen_addr` | `CALLFS_SERVER__LISTEN_ADDR` |
| `server.rate_limit.download_rps` | `CALLFS_SERVER__RATE_LIMIT__DOWNLOAD_RPS` |
| `metadata_store.redis_key_prefix` | `CALLFS_METADATA_STORE__REDIS_KEY_PREFIX` |
| `nats.jetstream.max_memory` | `CALLFS_NATS__JETSTREAM__MAX_MEMORY` |

The console Config file page can download the effective settings as an `.env` list that uses these names. An environment variable overrides the config file, and a setting that comes from an environment variable cannot be changed in the console (it shows "set by environment").

Examples:

```bash
export CALLFS_SERVER__PROTOCOL=https
export CALLFS_SERVER__METADATA_CACHE_TTL=5s
export CALLFS_SERVER__RATE_LIMIT__DOWNLOAD_RPS=7.5
export CALLFS_ERASURE__ENABLED=true
export CALLFS_NATS__JETSTREAM__MAX_MEMORY=512MiB
```

**Lists** are comma-separated. Spaces around items are removed and empty items are dropped:

```bash
export CALLFS_NATS__ROUTES="nats-route://10.0.0.2:6222,nats-route://10.0.0.3:6222"
export CALLFS_NATS__EXTERNAL__URLS="nats://10.0.0.9:4222"
export CALLFS_NATS__STREAMS__TASKS_BACKOFF="1s,5s,30s,2m,10m"
export CALLFS_SERVER__TRUSTED_PROXIES="10.0.0.0/24,10.1.0.0/24"
export CALLFS_AUTH__API_KEYS="first-key-at-least-16-chars,second-key-at-least-16-chars"
export CALLFS_AUTH__ADMIN_USERS="alice,bob"
```

**Maps** take one variable per entry. The text after the map name is the entry name, and it is converted to lower case:

```bash
export CALLFS_AUTH__API_KEY_USERS__ALICE="alice-key-at-least-16-chars"
export CALLFS_AUTH__API_KEY_USERS__BOB="bob-key-at-least-16-chars"
export CALLFS_NATS__SERVER_TAGS__ZONE="a"
env "CALLFS_INSTANCE_DISCOVERY__PEER_ENDPOINTS__NODE-2=https://node-2.example.com:8443" callfs server
```

The first two lines create the users `alice` and `bob`. Because names are lower-cased, write the matching `instance_id`, `admin_users` and peer names in lower case when you use environment variables. Names must not contain `.` or `__`. A name that has a hyphen, such as `node-2`, cannot be exported in some shells; pass it with `env`, in a container `-e` option or in a compose file.

**Booleans, numbers, durations and sizes** are written as plain text (`true`, `100`, `30s`, `256MiB`).

**Environment-only setting.** `CALLFS_LOG_MODE` is read directly by name (one underscore) and has no config file key. See [Log mode](#log-mode-environment-only).

## Secrets

These settings hold secrets. The console and exports never show their values, only whether they are set (`Set` / `Not set` in the console, `<set>` in exports):

`auth.api_keys`, `auth.api_key_users`, `auth.internal_proxy_secret`, `auth.single_use_link_secret`, `backend.s3_access_key`, `backend.s3_secret_key`, `metadata_store.dsn`, `metadata_store.migration_dsn`, `metadata_store.redis_password`, `dlm.redis_password`, `nats.auth.previous_secret`.

Rules and advice:

- `callfs config validate` prints the metadata store address only as `scheme://host`, with no user name or password.
- The two required secrets (`auth.internal_proxy_secret`, `auth.single_use_link_secret`) and every API key must be at least 16 characters, must be unique, and must not contain `your-`, `change-me` or `a-strong-` (any letter case). The built-in default secrets are placeholders of that kind, so you must set your own. Use 32 or more characters for `auth.internal_proxy_secret`; the console warns when it is shorter. Create one with `openssl rand -hex 32`.
- The shared server secret and the download link secret must differ from each other and must not be reused as an API key.
- Prefer environment variables or a secrets manager for secrets, and keep the config file readable by the CallFS user only (`chmod 600`). Files the console writes keep the mode of the existing file, or `0600` for a new file. The backup copy it makes next to the file (`config.yaml.bak.<UTC time>`) holds the same secrets, so protect it the same way.
- Changing `auth.single_use_link_secret` stops every active download link.

## Editing settings in the web console

Administrators can change settings in the web console under **Settings** (see [Web console](08-web-console.md)). The settings pages are:

| Console page | Contents |
| --- | --- |
| Network | listener, public address, HTTP/3, metrics port |
| Limits and timeouts | timeouts, index cache, edit-lock wait, request limits, console sessions |
| Security | certificate, CA and checks for server-to-server HTTPS, secrets, log privacy (read-only) |
| Storage | speed or strictness, default storage, local disk, S3 bucket, split storage (threshold, pieces folder), backup copy, trash |
| File index | where the file index lives (Postgres, SQLite, Redis, NATS), plus the NATS index log and snapshot settings |
| Cluster and locks | server ID, other servers, every NATS setting (node, network, security, storage, external, locks, streams, transfer), edit locks |
| Logging | log level and format, console history, charts, console on/off |
| Access | users, access keys, admin users |

How saving works:

- You change values, review the list of changes, then save. CallFS checks the values first and shows what is wrong.
- Saving writes the config file. Before it writes, CallFS copies the old file to `<config file>.bak.<UTC time>`. Saving needs a config file in use; without one, nothing can be saved.
- Settings marked `live` apply at once. Settings marked `restart` show a "saved changes need a restart" banner. Press **Restart now** to restart (only when `ui.allow_remote_restart` is `true`), or restart the process yourself.
- A setting that comes from an environment variable shows "set by environment" and cannot be changed. `log.mode` (`CALLFS_LOG_MODE`) is always environment-only and is shown read-only on the Security page.
- If someone else changed the file after you opened the page, the save is refused with a conflict. Reload and try again.
- Secrets show only `Set` or `Not set`.
- The console's **Config file** page shows the file path, format, load time, how many settings come from the environment and how many are still at their default. It can validate the current file, and it can download the effective settings as YAML, JSON or an `.env` list (secrets appear as `<set>`).
- In the tables below, the Console column names the page that holds each key. Keys marked `File` are not on a settings page; set them in the config file or the environment.
- Each value is checked against the allowed values, minimum and maximum listed below.

## Check a config with `callfs config validate`

```bash
callfs config validate --config /etc/callfs/config.yaml
```

Without `--config`, it checks the file found by the normal lookup plus environment variables. It loads settings exactly like the server does (defaults, file, environment) and runs every check in the tables below. It does not open any storage, database or network connection.

When the config is valid, it prints:

```
Validating configuration...
Configuration is valid
Instance ID: node-1
Listen Address: :8443
Metadata Store DSN: postgres://db.example.com
Redis Address: localhost:6379
Local FS Root: /var/lib/callfs
```

The S3 bucket and region lines appear only when `backend.s3_bucket_name` is set. The password part of a DSN is never printed.

When something is wrong, it prints `Configuration validation failed: config validation failed: <reason>` and exits with a non-zero status. It reports the first problem it finds, so fix it and run it again. The reasons are listed in the "Allowed values and rules" column below.

## How to read the reference tables

| Column | Meaning |
| --- | --- |
| Key | The setting as a dotted path. In YAML, each dot is one nesting level. |
| Type | See [Value formats](#value-formats). |
| Default | The built-in value. "unset" means there is no default and (where noted) you must supply one. |
| Allowed values and rules | Allowed words, minimum and maximum, what makes CallFS reject the config, and when the key is required. |
| Applies | `restart`: needs a restart. `live`: applies at once when changed in the console. `live (auth)`: access keys and users reload at once. |
| Console | The settings page that holds the key, or `File` when it is set only in the config file or the environment. |
| What it does | Plain-language effect. |

Every key can also be set with its environment variable (see above).

## server

Listener, TLS, timeouts and the file index cache.

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `server.listen_addr` | hostport | `:8443` | Required. Must not be empty. | restart | Network | Host and port that callfs listens on. An empty host listens on every interface. |
| `server.protocol` | enum | `https` | `https`, `http`, `auto`. Anything else is rejected with `server.protocol must be one of: http, https, auto`. | restart | Network | Whether people connect over HTTPS. `https` and `auto` serve HTTPS; `auto` stops with an error when the certificate files are missing and never falls back to plain HTTP. `http` is not encrypted and sends access keys in the clear. |
| `server.external_url` | url | `localhost:8443` | Not checked. Set a public address people can open, such as `https://files.example.com`. | restart | Network | Address used in the links callfs creates, and the address this server announces to other servers. |
| `server.cert_file` | path | `server.crt` | Required when `server.protocol` is `https` or `server.enable_quic` is `true` (`server.cert_file and server.key_file are required ...`). `auto` also needs it at start. | restart | Security | Certificate used for HTTPS on this server. |
| `server.key_file` | path | `server.key` | Required in the same cases as `server.cert_file`. | restart | Security | Private key that fits the certificate. |
| `server.enable_quic` | bool | `false` | When `true`, `server.cert_file`, `server.key_file` and `server.quic_listen_addr` are required. | restart | Network | When on, callfs also answers HTTP/3 over UDP. Needs a certificate and an open UDP port. |
| `server.quic_listen_addr` | hostport | `:8443` | Required when `server.enable_quic` is `true`. | restart | Network | Host and UDP port for HTTP/3. |
| `server.read_header_timeout` | duration | `10s` | Must be greater than 0 (`server.read_header_timeout must be > 0`). | restart | File | Longest time to receive the headers of one request. |
| `server.idle_timeout` | duration | `120s` | Must be greater than 0. | restart | File | How long an idle keep-alive connection stays open. |
| `server.read_timeout` | duration | `30s` | Must be greater than 0. | restart | Limits and timeouts | Longest time to read one request. Uploads and downloads use `server.transfer_timeout` instead. |
| `server.write_timeout` | duration | `30s` | Must be greater than 0. | restart | Limits and timeouts | Longest time to send one response. Uploads and downloads use `server.transfer_timeout` instead. |
| `server.transfer_timeout` | duration | `1h` | Must be greater than 0. | restart | File | Longest time for one file upload or download. Raise it for very large files. |
| `server.file_op_timeout` | duration | `10s` | Must be greater than 0. | restart | Limits and timeouts | Longest time for one read or write to storage. |
| `server.metadata_op_timeout` | duration | `5s` | Must be greater than 0. | restart | Limits and timeouts | Longest time for one file index read or write. |
| `server.metadata_cache_ttl` | duration | `2s` | 0 or more (`server.metadata_cache_ttl must be >= 0`). `0s` turns the cache off. | live | Limits and timeouts | How long file index answers stay in memory. |
| `server.metadata_cache_size` | int | `1000` | Greater than 0. | restart | File | Number of file index answers kept in memory. |
| `server.trusted_proxies` | list | `[]` | Each item must be a CIDR range such as `10.0.0.0/24`; otherwise `server.trusted_proxies contains invalid CIDR`. | restart | File | Addresses of reverse proxies you trust. For requests that come from these ranges, callfs takes the client address from the `X-Forwarded-For` header; for all others it uses the connection address. |

### server.rate_limit

Rates are requests per second; a burst is how many requests are allowed above the steady rate in a short time.

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `server.rate_limit.link_generate_rps` | float | `100` | Greater than 0. | live | Limits and timeouts | Sustained rate for creating share links. |
| `server.rate_limit.link_generate_burst` | int | `100` | 1 or more. | live | Limits and timeouts | Short bursts of link creations allowed above the rate. |
| `server.rate_limit.max_active_links_per_user` | int | `1000` | 0 or more. | restart | File | Most active download links one user can hold. `0` uses the built-in limit of 1000. Creating a link at the limit returns 429. |
| `server.rate_limit.download_rps` | float | `10` | Greater than 0. | live | Limits and timeouts | Steady rate for opening download links, per client address. |
| `server.rate_limit.download_burst` | int | `5` | 1 or more. | live | Limits and timeouts | Short bursts of download opens allowed above the rate. |
| `server.rate_limit.session_rps` | float | `1` | Greater than 0. | live | Limits and timeouts | Sustained rate for sign-in attempts. |
| `server.rate_limit.session_burst` | int | `5` | 1 or more. | live | Limits and timeouts | Short bursts of sign-in attempts allowed above the rate. |

## auth

API keys, users and shared secrets. At least one key is required, through `auth.api_keys` or `auth.api_key_users` (or both). See [Authentication and security](04-authentication-security.md).

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `auth.api_keys` | list (secret) | unset | Each key at least 16 characters, unique, not `default-api-key`, no placeholder text (`your-`, `change-me`, `a-strong-`). Required unless `auth.api_key_users` has an entry (`auth.api_keys or auth.api_key_users must contain at least one key`). | live (auth) | Access | Keys that sign in to callfs. The user of each key is named by its position: `api-user-0`, `api-user-1` and so on. Removing or reordering an entry changes the user of every later key, so only add to the end, and prefer `auth.api_key_users`. |
| `auth.api_key_users` | map (secret) | unset | Map of user ID to key. The ID must not be empty, `root`, `internal-proxy` or `api-user-<number>`. Keys follow the same rules as `auth.api_keys`, and no key may appear twice. | live (auth) | Access | Which user each access key belongs to. The user stays the same when other keys are added, removed or reordered. Set one entry per variable in the environment (`CALLFS_AUTH__API_KEY_USERS__<ID>`). |
| `auth.admin_users` | list | `[]` | Every item must be a configured user ID (a key in `auth.api_key_users`, an `api-user-N` position, or `root`); otherwise `auth.admin_users: "<id>" is not a configured user`. | live (auth) | Access | Users who can change settings. |
| `auth.internal_proxy_secret` | secret | placeholder, must be replaced | Required. At least 16 characters. No placeholder text. Not reused as an API key or as `auth.single_use_link_secret` (`auth.internal_proxy_secret must be set and not use default value`). Use 32 characters or more. | restart | Security | Secret that servers use to trust each other. Every server in a cluster uses the same value. |
| `auth.single_use_link_secret` | secret | placeholder, must be replaced | Required. At least 16 characters. No placeholder text. Different from `auth.internal_proxy_secret` and from every API key. | restart | Security | Secret that signs download links. Replacing it stops all active download links. |

## log

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `log.level` | enum | `info` | `debug`, `info`, `warn`, `error`. Unknown words are treated as `info`. | live | Logging | How much detail goes into the log. `debug` writes a lot of data. |
| `log.format` | enum | `json` | `json` or `text`. `json` writes one JSON object per line; any other value writes readable text. | restart | Logging | How log lines are written. |

How much of each file path the log shows is set by [`CALLFS_LOG_MODE`](#log-mode-environment-only).

## metrics

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `metrics.listen_addr` | hostport | `:9090` | Empty turns the metrics port off. | restart | Network | Where the Prometheus endpoint (`/metrics`) listens. This port has no sign-in; keep it on a private network. See [Monitoring](06-monitoring-metrics.md). |
| `metrics.profiling` | bool | `false` | | restart | Network | When on, the metrics port also serves profiles at `/debug/pprof`. Profiles show code paths and memory, so turn it on only for a test run. Profiling adds a little work to every lock and wait. |

## performance

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `performance.mode` | enum | `fast` | `fast` or `strict`. Empty means `fast`. Other values are rejected with `performance.mode must be one of: fast, strict`. | restart | Storage | How hard the cluster checks each read and save. Both modes only confirm a write once it is durable. `strict` also reads back each saved file and checks it, locks every change and makes every read wait for the latest cluster state. |

## backend

Where file data is stored. See [Storage](05-storage.md). The local disk is used when `backend.localfs_root_path` is set. S3 is used when `backend.s3_bucket_name` is set.

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `backend.default_backend` | enum | `localfs` | `localfs` or `s3`. Others are rejected (`backend.default_backend must be one of: localfs, s3`). | restart | Storage | Where new files are written. Existing files stay where they are. |
| `backend.localfs_root_path` | path | `/var/lib/callfs` | | restart | Storage | Folder where local files are kept. Changing it does not move existing files. Back it up. |
| `backend.localfs_sync` | enum | `full` | `full`, `barrier`, `none`. Others are rejected (`backend.localfs_sync must be one of: full, barrier, none`). | restart | Storage | How hard the disk is asked to confirm each saved file. `full` survives power loss. `barrier` is faster on Mac and equal to `full` elsewhere. `none` is fastest and can lose recent files on power loss. |
| `backend.localfs_ack` | enum | `disk` | `memory` or `disk`. Others are rejected (`backend.localfs_ack must be one of: memory, disk`). | restart | Storage | When a client is told its file is saved. `disk`: when it is on disk (slower, survives a power cut right after the save). `memory`: when it is written and indexed. A client can pick per request with the `X-CallFS-Write-Ack` header. |
| `backend.localfs_true_overwrite` | bool | `false` | | restart | Storage | `false`: an overwrite on local disk is a fast temp save (see [Overwrites](05-storage.md#overwrites)). `true`: every overwrite uses a true overwrite, which writes the new copy in the staging area, records it in the index and then moves it into place before the reply. Slower under load. A client can pick per request with the `X-CallFS-True-Overwrite` header. |
| `backend.localfs_tmp_save_max_size` | size | not set | At least 1 byte. `0` is rejected at startup (`backend.localfs_tmp_save_max_size must be at least 1 byte; leave it out for no size limit`). | restart | Storage | Used only while `backend.localfs_true_overwrite` is `false`. Files up to this size use the temp save; bigger files use a true overwrite. Leave it out (or empty) for no limit. |
| `backend.flush_workers` | int | `0` | 0 to 1024. `0` means one per CPU. Below 0 is rejected. | restart | Storage | How many files are flushed to disk at the same time. |
| `backend.flush_max_pending_bytes` | int (bytes) | `268435456` (256 MiB) | 0 or more, in whole bytes. | restart | Storage | Writers slow down when this much saved data is not on disk yet. |
| `backend.flush_max_delay` | duration | `0s` | 0 or more. | restart | Storage | How long flushes wait to group more files into one disk flush. `0s` flushes without waiting. |
| `backend.s3_access_key` | secret | `""` | | restart | Storage | Key ID that callfs uses to reach the bucket. |
| `backend.s3_secret_key` | secret | `""` | | restart | Storage | Secret that pairs with the access key ID. |
| `backend.s3_region` | string | `us-east-1` | | restart | Storage | Region of the bucket. |
| `backend.s3_bucket_name` | string | `""` | An empty value turns S3 off. | restart | Storage | Bucket that stores files. |
| `backend.s3_endpoint` | url | `""` | A `http://` address is refused unless `backend.s3_allow_insecure` is `true`. | restart | Storage | Address of an S3 service that is not Amazon, such as MinIO. Plain HTTP sends files and keys without encryption. |
| `backend.s3_allow_insecure` | bool | `false` | | restart | File | Lets `backend.s3_endpoint` use plain `http://`. Without it, callfs refuses to start S3 with such an endpoint. |
| `backend.s3_server_side_encryption` | enum | `AES256` | `AES256`, `aws:kms`, or empty `""` for off. | restart | Storage | How the bucket encrypts stored files. Choose off for MinIO servers without encryption. |
| `backend.s3_acl` | string | `private` | For example `private` or `public-read`. | restart | Storage | Access that new files get in the bucket. `public-read` lets anyone on the internet read new files. |
| `backend.s3_kms_key_id` | string | `""` | Used with `backend.s3_server_side_encryption: aws:kms`. | restart | Storage | Key that AWS KMS uses to encrypt files. |
| `backend.internal_proxy_ca_file` | path | `""` | Empty uses only the system CA list. The file must hold at least one PEM certificate; otherwise CallFS stops at start (`peer CA file ... holds no PEM certificates`). | restart | Security | Extra CA that signs the certificates of your other CallFS servers. Used whenever this server opens HTTPS to another server: requests passed on to a peer, erasure pieces on peers, the cross-server gateway and server health probes. Needed when servers use certificates from a private CA. |
| `backend.internal_proxy_skip_tls_verify` | bool | `false` | | restart | Security | When `true`, servers do not check each other's certificates and trust any certificate, so someone on the network could read the shared secret. The console shows it inverted as "Check server certificates". |

## metadata_store

Where the file index lives. See [Storage](05-storage.md) and [Clustering](07-clustering.md). Switching the type does not copy the index; CallFS starts with an empty file list.

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `metadata_store.type` | enum | `postgres` | `postgres`, `sqlite`, `redis`, `nats`. Empty means `postgres`. `raft` is rejected (see the note at the top). Others give `metadata_store.type must be one of: postgres, sqlite, redis, nats`. `nats` also needs `nats.enabled: true`. | restart | File index | Storage engine for the list of files. Choose `nats` for a cluster that needs no separate database. |
| `metadata_store.dsn` | secret | `""` | Required when the type is `postgres` (`metadata_store.dsn is required when metadata_store.type=postgres`). | restart | File index | Address and login for the Postgres index, for example `postgres://callfs:secret@db.example.com/callfs?sslmode=require`. |
| `metadata_store.migration_dsn` | secret | `""` | Optional. | restart | File | Separate Postgres login used to create and update tables at start. When empty, `metadata_store.dsn` is used. |
| `metadata_store.sqlite_path` | path | `./callfs.sqlite3` | Required when the type is `sqlite`. | restart | File index | File that holds the SQLite index. Relative paths start in the folder where callfs runs. Back it up. |
| `metadata_store.redis_addr` | hostport | `localhost:6379` | Required when the type is `redis`. | restart | File index | Redis address for the index. |
| `metadata_store.redis_username` | string | `""` | | restart | File | Redis user name, for Redis servers with named users. |
| `metadata_store.redis_password` | secret | `""` | | restart | File index | Password for the Redis index. Without a password, anyone who reaches Redis can change the index. |
| `metadata_store.redis_db` | int | `0` | 0 to 15. | restart | File index | Redis database that holds the index. |
| `metadata_store.redis_key_prefix` | string | `callfs:` | | restart | File index | Text put before each index key in Redis. Changing it hides current files. |
| `metadata_store.redis_tls` | bool | `false` | | restart | File | Connect to Redis over TLS. |
| `metadata_store.redis_tls_ca_file` | path | `""` | | restart | File | CA certificate used to check the Redis server certificate. |
| `metadata_store.redis_tls_skip_verify` | bool | `false` | | restart | File | Skip the Redis certificate check. Use only for tests. |
| `metadata_store.postgres.max_open_conns` | int | `25` | Greater than 0 when the type is `postgres`. | restart | File | Most database connections open at once. |
| `metadata_store.postgres.max_idle_conns` | int | `10` | 0 or more when the type is `postgres`. | restart | File | Idle database connections kept ready. |
| `metadata_store.postgres.conn_max_lifetime` | duration | `30m` | 0 or more when the type is `postgres`. | restart | File | How long a database connection is reused before it is replaced. |

## nats

CallFS runs an embedded NATS server for clustering, metadata replication, locks and messages between nodes. It works on one node with no other servers, or as a cluster. Set `nats.enabled: true`, `metadata_store.type: nats` and `dlm.type: nats`. If `nats.enabled` is `false` and `instance_discovery.peer_endpoints` lists any other server, the config is rejected (`instance_discovery.peer_endpoints requires nats.enabled=true`). The NATS rules below are checked only when `nats.enabled` is `true`. See [Clustering](07-clustering.md).

### nats: server and cluster

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `nats.enabled` | bool | `false` | | restart | Cluster and locks | Turns the NATS server on. Needed whenever more than one server runs. |
| `nats.mode` | enum | `embedded` | `embedded` or `external`. Required when `nats.enabled` is `true` (`nats.mode must be one of: embedded, external`). | restart | Cluster and locks | `embedded` runs NATS inside callfs. `external` connects to NATS servers you run yourself. |
| `nats.server_name` | string | `""` | No spaces. When empty it takes the value of `instance_discovery.instance_id`. | restart | Cluster and locks | Name of this server inside the NATS cluster. |
| `nats.cluster_name` | string | `callfs` | Required when `nats.enabled` is `true`. No spaces, `.`, `*` or `>`. | restart | Cluster and locks | Name of the cluster. Every server in one cluster uses the same name. |
| `nats.client_listen` | hostport | `""` | Optional; must be `host:port`. Empty means in-process only. A non-loopback address needs `nats.tls.cert_file` and `nats.tls.key_file`. | restart | Cluster and locks | Address where tools such as the `nats` command can connect, for example `127.0.0.1:4222`. |
| `nats.cluster_listen` | hostport | `127.0.0.1:6222` | Required when `nats.mode` is `embedded`. Must be `host:port`. A non-loopback address needs `nats.tls.cert_file`, `nats.tls.key_file` and `nats.tls.ca_file`. | restart | Cluster and locks | Address where other servers connect to this one. Use the same port on every server. |
| `nats.routes` | list | `[]` | Each item is a URL with the `nats-route://` or `tls-route://` scheme and includes host and port. | restart | Cluster and locks | Other servers to join, for example `nats-route://10.0.0.2:6222`. |
| `nats.routes_from_peers` | bool | `true` | | restart | Cluster and locks | When on, routes are also built from the hosts in `instance_discovery.peer_endpoints`, using this server's `nats.cluster_listen` port. |
| `nats.monitor_listen` | hostport | `""` | Optional; must be `host:port`. | restart | Cluster and locks | Optional address for NATS monitoring. Empty is off. |
| `nats.server_tags` | map | `{}` | Tag names cannot be empty or contain spaces or `:`. | restart | Cluster and locks | Labels for this server, for example `zone: a`. |
| `nats.rpc_timeout` | duration | `10s` | Greater than 0. | restart | Cluster and locks | Longest time to wait for another server to answer a request sent over NATS. |
| `nats.bytes` | enum | `http` | `http` or `chunked`. | restart | Cluster and locks | How file contents travel between servers. `http` uses the servers' web addresses from `instance_discovery.peer_endpoints`. `chunked` sends the file in pieces over NATS. Control calls between servers always use NATS when it is on. |
| `nats.counters` | bool | `false` | | restart | Cluster and locks | When on, request limits are shared across the cluster: download, link-creation and sign-in rates and the slots for websocket uploads count all servers together instead of each server alone. Counts from before it was turned on are not included. |

### nats.tls

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `nats.tls.cert_file` | path | `""` | Set together with `nats.tls.key_file`. | restart | Cluster and locks | Certificate this server presents to other servers and clients. Encrypts the NATS client port and the cluster routes. |
| `nats.tls.key_file` | path | `""` | Set together with `nats.tls.cert_file`. | restart | Cluster and locks | Private key for that certificate. |
| `nats.tls.ca_file` | path | `""` | Needs `nats.tls.cert_file` and `nats.tls.key_file`. | restart | Cluster and locks | CA certificate that signs the other servers' certificates. With it, servers check each other's certificates in both directions. |
| `nats.tls.client_cert_file` | path | `""` | Set together with `nats.tls.client_key_file`. | restart | Cluster and locks | Certificate this server presents when it connects to NATS, including its own in-process connection. |
| `nats.tls.client_key_file` | path | `""` | Set together with `nats.tls.client_cert_file`. | restart | Cluster and locks | Private key for that client certificate. |

### nats.auth

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `nats.auth.mode` | enum | `derived` | `derived`, `nkey`, `creds`. Required when `nats.enabled` is `true`. With `nats.mode: external` it must be `nkey` or `creds`. | restart | Cluster and locks | How servers prove who they are. `derived` uses `auth.internal_proxy_secret`. `nkey` uses the seed in `nats.auth.nkey_seed_file`. `creds` uses `nats.auth.creds_file` and works only with `nats.mode: external`. |
| `nats.auth.nkey_seed_file` | path | `""` | Required when `nats.auth.mode` is `nkey`. | restart | Cluster and locks | File with the NKey seed. |
| `nats.auth.creds_file` | path | `""` | Required when `nats.auth.mode` is `creds`. | restart | Cluster and locks | NATS credentials file. |
| `nats.auth.previous_secret` | secret | `""` | Used with `derived`. At least 16 characters and different from `auth.internal_proxy_secret`. | restart | Cluster and locks | The old `auth.internal_proxy_secret`, still accepted while you change the secret one server at a time. |

### nats.jetstream

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `nats.jetstream.store_dir` | path | `./data/nats` | Required when `nats.mode` is `embedded`. | restart | Cluster and locks | Folder where the cluster state (file index copy, locks data, events) is saved. Keep it on a persistent disk and back it up. |
| `nats.jetstream.max_memory` | bytes | `256MiB` | 0 or more. | restart | Cluster and locks | Most memory the cluster state may use. |
| `nats.jetstream.max_store` | bytes | `0` | 0 or more. `0` is no limit. | restart | Cluster and locks | Most disk space the cluster state may use. |
| `nats.jetstream.sync` | enum | `always` | `always` or `interval`. | restart | Cluster and locks | `always` flushes every change to disk. `interval` flushes on a timer, which is faster and can lose the last changes on power loss. |
| `nats.jetstream.sync_interval` | duration | `2m` | Greater than 0 when `nats.jetstream.sync` is `interval`. | restart | Cluster and locks | Time between flushes in `interval` mode. |
| `nats.jetstream.unique_tag` | string | `""` | | restart | Cluster and locks | Tag name that must differ between the servers that hold copies of the same data, such as a zone. |

### nats.external

Used when `nats.mode` is `external`.

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `nats.external.urls` | list | `[]` | Required when `nats.mode` is `external`. Each item starts with `nats://`, `tls://`, `ws://` or `wss://` and has a host. | restart | Cluster and locks | Addresses of your NATS servers. |
| `nats.external.js_domain` | string | `""` | | restart | Cluster and locks | JetStream domain to use on your NATS servers. |
| `nats.external.tls_ca_file` | path | `""` | | restart | Cluster and locks | CA certificate used to check your NATS servers. |

### nats.metadata

The replicated file index.

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `nats.metadata.replicas` | int | `3` | 1 to 5. | restart | File index | Copies of the file index kept in the cluster. Never more than the number of servers. |
| `nats.metadata.apply_timeout` | duration | `10s` | Greater than 0. | restart | File index | Longest time to save one index change in the cluster. |
| `nats.metadata.read_barrier` | enum | `info` | `info` or `publish`. | live | File index | How a read waits for the latest state. `info` checks the cluster position. `publish` writes a marker first and waits for it. |
| `nats.metadata.miss_barrier` | bool | `false` | | restart | File index | When a lookup finds nothing, wait for the latest state and look again. |
| `nats.metadata.snapshot_interval` | duration | `5m` | Greater than 0. | restart | File index | How often the cluster checks whether to take a snapshot of the index. Old changes are removed after a snapshot, and a server that starts restores from the newest valid snapshot. |
| `nats.metadata.snapshot_every` | int | `50000` | 1 or more. | restart | File index | Number of changes that triggers a snapshot. |
| `nats.metadata.retain_commands` | int | `100000` | 1 or more. | restart | File index | Number of recent changes kept. |
| `nats.metadata.retain_snapshots` | int | `3` | 1 or more. | restart | File index | Number of snapshots kept. |
| `nats.metadata.max_msg_size` | bytes | `8MiB` | `64KiB` to `64MiB` (`nats.metadata.max_msg_size must be between 64KiB and 64MiB`). | restart | File index | Largest single cluster message, which limits how big one index change can be. |

### nats.locks

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `nats.locks.replicas` | int | `3` | 1 to 5. | restart | Cluster and locks | Copies of the edit locks kept in the cluster. |
| `nats.locks.ttl` | duration | `15s` | 1s or more. | restart | Cluster and locks | How long a lock lasts if its holder stops answering. |
| `nats.locks.queue_stale` | duration | `3s` | 1s or more. | restart | Cluster and locks | How long a waiting request stays in the line before it is dropped. Waiting writers are served in the order they arrived. |

### nats.streams

Events, audit entries, logs and background jobs shared across the cluster. Settings marked `live` can change without a restart from the console.

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `nats.streams.replicas` | int | `3` | 1 to 5. | live | Cluster and locks | Copies kept of events, audit, jobs and other shared data. |
| `nats.streams.events_max_age` | duration | `168h` | Greater than 0. | live | Cluster and locks | How long events are kept. |
| `nats.streams.events_max_bytes` | bytes | `1GiB` | 0 or more. | live | Cluster and locks | Most space the events may take. |
| `nats.streams.audit_max_age` | duration | `2160h` | Greater than 0. | live | Cluster and locks | How long audit entries are kept. |
| `nats.streams.logs_level` | enum | `warn` | `debug`, `info`, `warn`, `error`. | live | Cluster and locks | Lowest log level copied to the shared cluster log. The console log view then shows lines from all servers. |
| `nats.streams.logs_max_age` | duration | `72h` | Greater than 0. | live | Cluster and locks | How long shared log lines are kept. |
| `nats.streams.tasks_max_deliver` | int | `8` | 1 or more. Must be greater than the number of entries in `nats.streams.tasks_backoff`. | live | Cluster and locks | Times a failed background task, such as copying a file to the backup storage, is tried before it moves to the failed-jobs list. |
| `nats.streams.tasks_backoff` | list of durations | `[1s, 5s, 30s, 2m, 10m]` | Every item greater than 0. | live | Cluster and locks | Wait before each retry of a failed job. |
| `nats.streams.dlq_max_age` | duration | `720h` | Greater than 0. | live | Cluster and locks | How long failed jobs are kept. |

## dlm

Edit locks, which keep two writers from changing one file at the same time.

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `dlm.type` | enum | `redis` | `redis`, `local`, `nats`. Empty means `redis`. `raft` is rejected. `nats` needs `nats.enabled: true`. `local` is rejected when `nats.enabled` is `true` and other servers are listed (`dlm.type=local cannot coordinate several nodes`). | restart | Cluster and locks | Where edit locks are kept. `local` works only on one server. |
| `dlm.redis_addr` | hostport | `localhost:6379` | Required when the type is `redis`. | restart | Cluster and locks | Redis address for edit locks. |
| `dlm.redis_password` | secret | `""` | | restart | Cluster and locks | Password for Redis edit locks. |
| `dlm.wait_timeout` | duration | `30s` | | live | Limits and timeouts | How long a save waits for another edit to finish. |
| `dlm.redis_pool_size` | int | `10` | Greater than 0 when the type is `redis`. | restart | File | Redis connections kept for locks. |
| `dlm.redis_min_idle_conns` | int | `5` | 0 or more when the type is `redis`. | restart | File | Idle Redis connections kept ready. |
| `dlm.redis_tls` | bool | `false` | | restart | File | Connect to Redis over TLS. |
| `dlm.redis_tls_ca_file` | path | `""` | | restart | File | CA certificate used to check the Redis server certificate. |
| `dlm.redis_tls_skip_verify` | bool | `false` | | restart | File | Skip the Redis certificate check. Use only for tests. |

## ha

A second copy of every file. See [Storage](05-storage.md).

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `ha.replication_enabled` | bool | `false` | | restart | Storage | When on, each file gets a second copy. |
| `ha.replica_backend` | enum | `""` | `localfs` or `s3`. Required when `ha.replication_enabled` is `true` (`ha.replica_backend must be one of: localfs, s3 ...`). | restart | Storage | Where the backup copy is stored. |
| `ha.require_replica_success` | bool | `false` | | restart | Storage | When on, an upload fails unless the backup copy is written. Uploads fail while the backup storage is down. |

## instance_discovery

How this server is named and how it finds the others. See [Clustering](07-clustering.md).

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `instance_discovery.instance_id` | string | `""` | Required, always (`instance_discovery.instance_id is required`). Unique for each server. | restart | Cluster and locks | Name of this server for the other servers. |
| `instance_discovery.peer_endpoints` | map | `{}` | Map of server ID to URL. Each URL must be valid and use `https`, except plain `http` to localhost. Listing other servers needs `nats.enabled: true`. This server's own ID may be listed. | restart | Cluster and locks | Other servers and their web addresses, for example `node-2: https://node-2.example.com:8443`. |

## erasure

Split storage: each file is cut into data pieces plus spare pieces, so it survives lost pieces. Files saved before you turn it on stay as they are. See [Storage](05-storage.md).

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `erasure.enabled` | bool | `false` | | restart | Storage | When on, files are cut into pieces with spare pieces. |
| `erasure.data_shards` | int | `4` | 2 to 32 when `erasure.enabled` is `true`. With the spare pieces, the total must not exceed 256. | restart | Storage | Pieces that hold the file data. Changing this affects new files only. |
| `erasure.parity_shards` | int | `2` | 1 to 32 when `erasure.enabled` is `true`. | restart | Storage | Spare pieces that let a file survive lost pieces. A file survives the loss of this many pieces. |
| `erasure.shard_backend` | enum | `""` | `localfs` or `s3`. Empty means `localfs`. | restart | Storage | Where the pieces are stored. |
| `erasure.min_file_size` | bytes | `0` | 0 or more (`erasure.min_file_size must be >= 0`). Checked even when erasure is off. | restart | Storage | Uploads at or above this size are split automatically. `0` splits only uploads that ask for it with the `X-CallFS-Erasure: true` header. A client can opt a large upload out with `X-CallFS-Erasure: false`. Sending `true` while `erasure.enabled` is `false` returns 501. |
| `erasure.shard_path` | string | `.callfs/shards` | A clean relative path inside the storage, with no `..` and no leading or trailing `/` (`erasure.shard_path must be a clean relative path inside the storage`). Empty becomes the default. | restart | Storage | Folder inside each storage (the local root folder or the S3 bucket) where the pieces are kept. Back it up with the rest of the storage. |

## ui

The web console. See [Web console](08-web-console.md).

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `ui.enabled` | bool | `true` | | restart | Logging | When on, the web console is available. Turning it off also turns off the console. |
| `ui.session_ttl` | duration | `8h` | `5m` to `720h` (`ui.session_ttl must be between 5m and 720h`). | live | Limits and timeouts | How long a console session lasts before you sign in again. Applies to new sign-ins. |
| `ui.idle_timeout` | duration | `1h` | `1m` up to `ui.session_ttl`. | live | Limits and timeouts | A console session ends after this long with no use. |
| `ui.allow_remote_restart` | bool | `false` | | restart | Logging | When on, the console can restart callfs. |
| `ui.metrics_sample_interval` | duration | `10s` | `1s` to `60s`. The history length divided by this interval must not exceed 100000 samples. The console offers `5s`, `10s` and `30s`. | restart | Logging | How often charts take a sample. |
| `ui.metrics_retention` | duration | `24h` | `1h` to `168h`. The console offers `6h`, `24h` and `168h` (7 days). | restart | Logging | How far back charts reach. |
| `ui.log_buffer_entries` | int | `5000` | 100 to 1000000. | restart | Logging | Number of recent log lines shown in the console. |
| `ui.audit_buffer_entries` | int | `20000` | 100 to 1000000. | restart | Logging | Number of recent requests shown in Audit. |
| `ui.event_buffer_entries` | int | `5000` | 100 to 1000000. | restart | Logging | Number of recent events kept. |

## trash

Deleted files can be restored for a while.

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `trash.enabled` | bool | `true` | | live | Storage | When on, deleted files can be restored for a while. |
| `trash.retention` | duration | `720h` | `1h` to `8760h` (`trash.retention must be between 1h and 8760h`). | live | Storage | How long files stay in trash before they are deleted for good. |

## Log mode (environment only)

| Key | Type | Default | Allowed values and rules | Applies | Console | What it does |
| --- | --- | --- | --- | --- | --- | --- |
| `log.mode` (variable `CALLFS_LOG_MODE`) | enum | `production` | `production`, `development`, `debug`. Other values are ignored. The variable is read once at start. It has no config file key and cannot be changed in the console. | restart | Security (shown, read-only) | How much file path detail goes into logs. `production` hashes paths. `development` shortens them. `debug` shows all paths. |

```bash
CALLFS_LOG_MODE=development callfs server --config /etc/callfs/config.yaml
```

## Example: single node

One server with local disk storage, a SQLite index and local locks. Replace every secret before you use it; generate each with `openssl rand -hex 32`.

```yaml
server:
  listen_addr: ":8443"
  protocol: "https"                      # https | http | auto
  external_url: "https://files.example.com"
  cert_file: "/etc/callfs/tls/server.crt"
  key_file: "/etc/callfs/tls/server.key"
  trusted_proxies: []                    # CIDRs of reverse proxies, for X-Forwarded-For

auth:
  api_key_users:                         # user ID -> key (at least 16 characters)
    alice: "6d1f0b7c9e3a4f58b2c6a0d1e7f93b42"
  admin_users: ["alice"]                 # who can change settings in the console
  internal_proxy_secret: "a3f9c1e75b0d48a29c6e1f07d4b83a5e91c2d7f60b4e8a13"
  single_use_link_secret: "e81b4d7a0c935f26b1a7d3e90c64f58a2b7e1d09c3a6f845"

log:
  level: "info"
  format: "json"

metrics:
  listen_addr: "127.0.0.1:9090"          # no sign-in on this port

performance:
  mode: "fast"                           # fast | strict

backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs"
  localfs_sync: "full"                   # full | barrier | none
  localfs_ack: "disk"                    # memory | disk

metadata_store:
  type: "sqlite"
  sqlite_path: "/var/lib/callfs/callfs.sqlite3"

dlm:
  type: "local"                          # one server only

instance_discovery:
  instance_id: "node-1"

trash:
  enabled: true
  retention: 720h

ui:
  enabled: true
  session_ttl: 8h
  idle_timeout: 1h
```

To use the embedded NATS on a single server instead, add this and remove the `sqlite_path` line:

```yaml
nats:
  enabled: true
metadata_store:
  type: "nats"
dlm:
  type: "nats"
```

Check it:

```bash
callfs config validate --config /etc/callfs/config.yaml
```

## Example: 3-node NATS cluster

Three servers, `node-1`, `node-2` and `node-3`, each with its own folder for files and for the NATS data. The file index and the locks live in the cluster, so no database or Redis is needed. This is the config for `node-1`; the other two differ only in the lines listed after it.

Before you start:

- Create a CA and a certificate for each server for the NATS port (`/etc/callfs/nats/server.crt`, `server.key`, `ca.crt`). Cluster traffic leaves loopback, so the certificate, key and CA are required.
- Every server needs the same `auth.internal_proxy_secret`, `auth.single_use_link_secret`, API keys and `nats.cluster_name`.
- Open the web port (`8443`) and the NATS cluster port (`6222`) between the servers.

```yaml
server:
  listen_addr: ":8443"
  protocol: "https"
  external_url: "https://node-1.example.com:8443"   # differs per node
  cert_file: "/etc/callfs/tls/server.crt"
  key_file: "/etc/callfs/tls/server.key"

auth:
  api_key_users:                         # same on every node
    alice: "6d1f0b7c9e3a4f58b2c6a0d1e7f93b42"
  admin_users: ["alice"]
  internal_proxy_secret: "a3f9c1e75b0d48a29c6e1f07d4b83a5e91c2d7f60b4e8a13"   # same on every node
  single_use_link_secret: "e81b4d7a0c935f26b1a7d3e90c64f58a2b7e1d09c3a6f845"  # same on every node

metrics:
  listen_addr: "10.0.0.1:9090"           # private network only

backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs"

metadata_store:
  type: "nats"                           # file index replicated in the cluster

dlm:
  type: "nats"                           # locks kept in the cluster

instance_discovery:
  instance_id: "node-1"                  # differs per node, no spaces
  peer_endpoints:                        # the other servers (https required)
    node-2: "https://node-2.example.com:8443"
    node-3: "https://node-3.example.com:8443"

nats:
  enabled: true
  mode: "embedded"
  cluster_name: "callfs"                 # same on every node
  cluster_listen: "0.0.0.0:6222"         # same port on every node
  routes_from_peers: true                # routes come from peer_endpoints hosts
  tls:
    cert_file: "/etc/callfs/nats/server.crt"
    key_file: "/etc/callfs/nats/server.key"
    ca_file: "/etc/callfs/nats/ca.crt"
  auth:
    mode: "derived"                      # uses auth.internal_proxy_secret
  jetstream:
    store_dir: "/var/lib/callfs/nats"    # persistent disk, back it up
    sync: "always"
  metadata:
    replicas: 3
  locks:
    replicas: 3
  streams:
    replicas: 3
```

What differs on the other nodes:

| Key | node-1 | node-2 | node-3 |
| --- | --- | --- | --- |
| `instance_discovery.instance_id` | `node-1` | `node-2` | `node-3` |
| `server.external_url` | `https://node-1.example.com:8443` | `https://node-2.example.com:8443` | `https://node-3.example.com:8443` |
| `instance_discovery.peer_endpoints` | node-2, node-3 | node-1, node-3 | node-1, node-2 |
| `metrics.listen_addr` | `10.0.0.1:9090` | `10.0.0.2:9090` | `10.0.0.3:9090` |

To list routes yourself instead of deriving them from the peers, set `nats.routes_from_peers: false` and add `nats.routes: ["nats-route://node-2.example.com:6222", "nats-route://node-3.example.com:6222"]`.

Start the servers, then check each one with `callfs config validate` before launching it. See [Clustering](07-clustering.md) for adding and removing nodes, and [Monitoring](06-monitoring-metrics.md) for cluster health.

## Checklist of every setting

Every key below appears once in the reference above, in the section named in the second column. The `Env` column is its environment variable.

| Key | Section | Environment variable |
| --- | --- | --- |
| `server.listen_addr` | server | `CALLFS_SERVER__LISTEN_ADDR` |
| `server.protocol` | server | `CALLFS_SERVER__PROTOCOL` |
| `server.external_url` | server | `CALLFS_SERVER__EXTERNAL_URL` |
| `server.cert_file` | server | `CALLFS_SERVER__CERT_FILE` |
| `server.key_file` | server | `CALLFS_SERVER__KEY_FILE` |
| `server.enable_quic` | server | `CALLFS_SERVER__ENABLE_QUIC` |
| `server.quic_listen_addr` | server | `CALLFS_SERVER__QUIC_LISTEN_ADDR` |
| `server.read_header_timeout` | server | `CALLFS_SERVER__READ_HEADER_TIMEOUT` |
| `server.idle_timeout` | server | `CALLFS_SERVER__IDLE_TIMEOUT` |
| `server.read_timeout` | server | `CALLFS_SERVER__READ_TIMEOUT` |
| `server.write_timeout` | server | `CALLFS_SERVER__WRITE_TIMEOUT` |
| `server.transfer_timeout` | server | `CALLFS_SERVER__TRANSFER_TIMEOUT` |
| `server.file_op_timeout` | server | `CALLFS_SERVER__FILE_OP_TIMEOUT` |
| `server.metadata_op_timeout` | server | `CALLFS_SERVER__METADATA_OP_TIMEOUT` |
| `server.metadata_cache_ttl` | server | `CALLFS_SERVER__METADATA_CACHE_TTL` |
| `server.metadata_cache_size` | server | `CALLFS_SERVER__METADATA_CACHE_SIZE` |
| `server.trusted_proxies` | server | `CALLFS_SERVER__TRUSTED_PROXIES` |
| `server.rate_limit.link_generate_rps` | server.rate_limit | `CALLFS_SERVER__RATE_LIMIT__LINK_GENERATE_RPS` |
| `server.rate_limit.link_generate_burst` | server.rate_limit | `CALLFS_SERVER__RATE_LIMIT__LINK_GENERATE_BURST` |
| `server.rate_limit.max_active_links_per_user` | server.rate_limit | `CALLFS_SERVER__RATE_LIMIT__MAX_ACTIVE_LINKS_PER_USER` |
| `server.rate_limit.download_rps` | server.rate_limit | `CALLFS_SERVER__RATE_LIMIT__DOWNLOAD_RPS` |
| `server.rate_limit.download_burst` | server.rate_limit | `CALLFS_SERVER__RATE_LIMIT__DOWNLOAD_BURST` |
| `server.rate_limit.session_rps` | server.rate_limit | `CALLFS_SERVER__RATE_LIMIT__SESSION_RPS` |
| `server.rate_limit.session_burst` | server.rate_limit | `CALLFS_SERVER__RATE_LIMIT__SESSION_BURST` |
| `auth.api_keys` | auth | `CALLFS_AUTH__API_KEYS` |
| `auth.api_key_users` | auth | `CALLFS_AUTH__API_KEY_USERS` |
| `auth.internal_proxy_secret` | auth | `CALLFS_AUTH__INTERNAL_PROXY_SECRET` |
| `auth.single_use_link_secret` | auth | `CALLFS_AUTH__SINGLE_USE_LINK_SECRET` |
| `auth.admin_users` | auth | `CALLFS_AUTH__ADMIN_USERS` |
| `log.level` | log | `CALLFS_LOG__LEVEL` |
| `log.format` | log | `CALLFS_LOG__FORMAT` |
| `metrics.listen_addr` | metrics | `CALLFS_METRICS__LISTEN_ADDR` |
| `metrics.profiling` | metrics | `CALLFS_METRICS__PROFILING` |
| `backend.default_backend` | backend | `CALLFS_BACKEND__DEFAULT_BACKEND` |
| `backend.localfs_root_path` | backend | `CALLFS_BACKEND__LOCALFS_ROOT_PATH` |
| `performance.mode` | performance | `CALLFS_PERFORMANCE__MODE` |
| `backend.localfs_sync` | backend | `CALLFS_BACKEND__LOCALFS_SYNC` |
| `backend.localfs_ack` | backend | `CALLFS_BACKEND__LOCALFS_ACK` |
| `backend.localfs_true_overwrite` | backend | `CALLFS_BACKEND__LOCALFS_TRUE_OVERWRITE` |
| `backend.localfs_tmp_save_max_size` | backend | `CALLFS_BACKEND__LOCALFS_TMP_SAVE_MAX_SIZE` |
| `backend.flush_workers` | backend | `CALLFS_BACKEND__FLUSH_WORKERS` |
| `backend.flush_max_pending_bytes` | backend | `CALLFS_BACKEND__FLUSH_MAX_PENDING_BYTES` |
| `backend.flush_max_delay` | backend | `CALLFS_BACKEND__FLUSH_MAX_DELAY` |
| `backend.s3_access_key` | backend | `CALLFS_BACKEND__S3_ACCESS_KEY` |
| `backend.s3_secret_key` | backend | `CALLFS_BACKEND__S3_SECRET_KEY` |
| `backend.s3_region` | backend | `CALLFS_BACKEND__S3_REGION` |
| `backend.s3_bucket_name` | backend | `CALLFS_BACKEND__S3_BUCKET_NAME` |
| `backend.s3_endpoint` | backend | `CALLFS_BACKEND__S3_ENDPOINT` |
| `backend.s3_allow_insecure` | backend | `CALLFS_BACKEND__S3_ALLOW_INSECURE` |
| `backend.s3_server_side_encryption` | backend | `CALLFS_BACKEND__S3_SERVER_SIDE_ENCRYPTION` |
| `backend.s3_acl` | backend | `CALLFS_BACKEND__S3_ACL` |
| `backend.s3_kms_key_id` | backend | `CALLFS_BACKEND__S3_KMS_KEY_ID` |
| `backend.internal_proxy_ca_file` | backend | `CALLFS_BACKEND__INTERNAL_PROXY_CA_FILE` |
| `backend.internal_proxy_skip_tls_verify` | backend | `CALLFS_BACKEND__INTERNAL_PROXY_SKIP_TLS_VERIFY` |
| `metadata_store.type` | metadata_store | `CALLFS_METADATA_STORE__TYPE` |
| `metadata_store.dsn` | metadata_store | `CALLFS_METADATA_STORE__DSN` |
| `metadata_store.migration_dsn` | metadata_store | `CALLFS_METADATA_STORE__MIGRATION_DSN` |
| `metadata_store.sqlite_path` | metadata_store | `CALLFS_METADATA_STORE__SQLITE_PATH` |
| `metadata_store.redis_addr` | metadata_store | `CALLFS_METADATA_STORE__REDIS_ADDR` |
| `metadata_store.redis_username` | metadata_store | `CALLFS_METADATA_STORE__REDIS_USERNAME` |
| `metadata_store.redis_password` | metadata_store | `CALLFS_METADATA_STORE__REDIS_PASSWORD` |
| `metadata_store.redis_db` | metadata_store | `CALLFS_METADATA_STORE__REDIS_DB` |
| `metadata_store.redis_key_prefix` | metadata_store | `CALLFS_METADATA_STORE__REDIS_KEY_PREFIX` |
| `metadata_store.redis_tls` | metadata_store | `CALLFS_METADATA_STORE__REDIS_TLS` |
| `metadata_store.redis_tls_ca_file` | metadata_store | `CALLFS_METADATA_STORE__REDIS_TLS_CA_FILE` |
| `metadata_store.redis_tls_skip_verify` | metadata_store | `CALLFS_METADATA_STORE__REDIS_TLS_SKIP_VERIFY` |
| `metadata_store.postgres.max_open_conns` | metadata_store | `CALLFS_METADATA_STORE__POSTGRES__MAX_OPEN_CONNS` |
| `metadata_store.postgres.max_idle_conns` | metadata_store | `CALLFS_METADATA_STORE__POSTGRES__MAX_IDLE_CONNS` |
| `metadata_store.postgres.conn_max_lifetime` | metadata_store | `CALLFS_METADATA_STORE__POSTGRES__CONN_MAX_LIFETIME` |
| `nats.enabled` | nats | `CALLFS_NATS__ENABLED` |
| `nats.mode` | nats | `CALLFS_NATS__MODE` |
| `nats.server_name` | nats | `CALLFS_NATS__SERVER_NAME` |
| `nats.cluster_name` | nats | `CALLFS_NATS__CLUSTER_NAME` |
| `nats.client_listen` | nats | `CALLFS_NATS__CLIENT_LISTEN` |
| `nats.cluster_listen` | nats | `CALLFS_NATS__CLUSTER_LISTEN` |
| `nats.routes` | nats | `CALLFS_NATS__ROUTES` |
| `nats.routes_from_peers` | nats | `CALLFS_NATS__ROUTES_FROM_PEERS` |
| `nats.monitor_listen` | nats | `CALLFS_NATS__MONITOR_LISTEN` |
| `nats.server_tags` | nats | `CALLFS_NATS__SERVER_TAGS` |
| `nats.tls.cert_file` | nats.tls | `CALLFS_NATS__TLS__CERT_FILE` |
| `nats.tls.key_file` | nats.tls | `CALLFS_NATS__TLS__KEY_FILE` |
| `nats.tls.ca_file` | nats.tls | `CALLFS_NATS__TLS__CA_FILE` |
| `nats.tls.client_cert_file` | nats.tls | `CALLFS_NATS__TLS__CLIENT_CERT_FILE` |
| `nats.tls.client_key_file` | nats.tls | `CALLFS_NATS__TLS__CLIENT_KEY_FILE` |
| `nats.auth.mode` | nats.auth | `CALLFS_NATS__AUTH__MODE` |
| `nats.auth.nkey_seed_file` | nats.auth | `CALLFS_NATS__AUTH__NKEY_SEED_FILE` |
| `nats.auth.creds_file` | nats.auth | `CALLFS_NATS__AUTH__CREDS_FILE` |
| `nats.auth.previous_secret` | nats.auth | `CALLFS_NATS__AUTH__PREVIOUS_SECRET` |
| `nats.jetstream.store_dir` | nats.jetstream | `CALLFS_NATS__JETSTREAM__STORE_DIR` |
| `nats.jetstream.max_memory` | nats.jetstream | `CALLFS_NATS__JETSTREAM__MAX_MEMORY` |
| `nats.jetstream.max_store` | nats.jetstream | `CALLFS_NATS__JETSTREAM__MAX_STORE` |
| `nats.jetstream.sync` | nats.jetstream | `CALLFS_NATS__JETSTREAM__SYNC` |
| `nats.jetstream.sync_interval` | nats.jetstream | `CALLFS_NATS__JETSTREAM__SYNC_INTERVAL` |
| `nats.jetstream.unique_tag` | nats.jetstream | `CALLFS_NATS__JETSTREAM__UNIQUE_TAG` |
| `nats.external.urls` | nats.external | `CALLFS_NATS__EXTERNAL__URLS` |
| `nats.external.js_domain` | nats.external | `CALLFS_NATS__EXTERNAL__JS_DOMAIN` |
| `nats.external.tls_ca_file` | nats.external | `CALLFS_NATS__EXTERNAL__TLS_CA_FILE` |
| `nats.rpc_timeout` | nats | `CALLFS_NATS__RPC_TIMEOUT` |
| `nats.metadata.replicas` | nats.metadata | `CALLFS_NATS__METADATA__REPLICAS` |
| `nats.metadata.apply_timeout` | nats.metadata | `CALLFS_NATS__METADATA__APPLY_TIMEOUT` |
| `nats.metadata.read_barrier` | nats.metadata | `CALLFS_NATS__METADATA__READ_BARRIER` |
| `nats.metadata.miss_barrier` | nats.metadata | `CALLFS_NATS__METADATA__MISS_BARRIER` |
| `nats.metadata.snapshot_interval` | nats.metadata | `CALLFS_NATS__METADATA__SNAPSHOT_INTERVAL` |
| `nats.metadata.snapshot_every` | nats.metadata | `CALLFS_NATS__METADATA__SNAPSHOT_EVERY` |
| `nats.metadata.retain_commands` | nats.metadata | `CALLFS_NATS__METADATA__RETAIN_COMMANDS` |
| `nats.metadata.retain_snapshots` | nats.metadata | `CALLFS_NATS__METADATA__RETAIN_SNAPSHOTS` |
| `nats.metadata.max_msg_size` | nats.metadata | `CALLFS_NATS__METADATA__MAX_MSG_SIZE` |
| `nats.locks.replicas` | nats.locks | `CALLFS_NATS__LOCKS__REPLICAS` |
| `nats.locks.ttl` | nats.locks | `CALLFS_NATS__LOCKS__TTL` |
| `nats.locks.queue_stale` | nats.locks | `CALLFS_NATS__LOCKS__QUEUE_STALE` |
| `nats.streams.replicas` | nats.streams | `CALLFS_NATS__STREAMS__REPLICAS` |
| `nats.streams.events_max_age` | nats.streams | `CALLFS_NATS__STREAMS__EVENTS_MAX_AGE` |
| `nats.streams.events_max_bytes` | nats.streams | `CALLFS_NATS__STREAMS__EVENTS_MAX_BYTES` |
| `nats.streams.audit_max_age` | nats.streams | `CALLFS_NATS__STREAMS__AUDIT_MAX_AGE` |
| `nats.streams.logs_level` | nats.streams | `CALLFS_NATS__STREAMS__LOGS_LEVEL` |
| `nats.streams.logs_max_age` | nats.streams | `CALLFS_NATS__STREAMS__LOGS_MAX_AGE` |
| `nats.streams.tasks_max_deliver` | nats.streams | `CALLFS_NATS__STREAMS__TASKS_MAX_DELIVER` |
| `nats.streams.tasks_backoff` | nats.streams | `CALLFS_NATS__STREAMS__TASKS_BACKOFF` |
| `nats.streams.dlq_max_age` | nats.streams | `CALLFS_NATS__STREAMS__DLQ_MAX_AGE` |
| `nats.bytes` | nats | `CALLFS_NATS__BYTES` |
| `nats.counters` | nats | `CALLFS_NATS__COUNTERS` |
| `dlm.type` | dlm | `CALLFS_DLM__TYPE` |
| `dlm.redis_addr` | dlm | `CALLFS_DLM__REDIS_ADDR` |
| `dlm.redis_password` | dlm | `CALLFS_DLM__REDIS_PASSWORD` |
| `dlm.wait_timeout` | dlm | `CALLFS_DLM__WAIT_TIMEOUT` |
| `dlm.redis_pool_size` | dlm | `CALLFS_DLM__REDIS_POOL_SIZE` |
| `dlm.redis_min_idle_conns` | dlm | `CALLFS_DLM__REDIS_MIN_IDLE_CONNS` |
| `dlm.redis_tls` | dlm | `CALLFS_DLM__REDIS_TLS` |
| `dlm.redis_tls_ca_file` | dlm | `CALLFS_DLM__REDIS_TLS_CA_FILE` |
| `dlm.redis_tls_skip_verify` | dlm | `CALLFS_DLM__REDIS_TLS_SKIP_VERIFY` |
| `ha.replication_enabled` | ha | `CALLFS_HA__REPLICATION_ENABLED` |
| `ha.replica_backend` | ha | `CALLFS_HA__REPLICA_BACKEND` |
| `ha.require_replica_success` | ha | `CALLFS_HA__REQUIRE_REPLICA_SUCCESS` |
| `instance_discovery.instance_id` | instance_discovery | `CALLFS_INSTANCE_DISCOVERY__INSTANCE_ID` |
| `instance_discovery.peer_endpoints` | instance_discovery | `CALLFS_INSTANCE_DISCOVERY__PEER_ENDPOINTS` |
| `erasure.enabled` | erasure | `CALLFS_ERASURE__ENABLED` |
| `erasure.data_shards` | erasure | `CALLFS_ERASURE__DATA_SHARDS` |
| `erasure.parity_shards` | erasure | `CALLFS_ERASURE__PARITY_SHARDS` |
| `erasure.shard_backend` | erasure | `CALLFS_ERASURE__SHARD_BACKEND` |
| `erasure.min_file_size` | erasure | `CALLFS_ERASURE__MIN_FILE_SIZE` |
| `erasure.shard_path` | erasure | `CALLFS_ERASURE__SHARD_PATH` |
| `ui.enabled` | ui | `CALLFS_UI__ENABLED` |
| `ui.session_ttl` | ui | `CALLFS_UI__SESSION_TTL` |
| `ui.idle_timeout` | ui | `CALLFS_UI__IDLE_TIMEOUT` |
| `ui.allow_remote_restart` | ui | `CALLFS_UI__ALLOW_REMOTE_RESTART` |
| `ui.metrics_sample_interval` | ui | `CALLFS_UI__METRICS_SAMPLE_INTERVAL` |
| `ui.metrics_retention` | ui | `CALLFS_UI__METRICS_RETENTION` |
| `ui.log_buffer_entries` | ui | `CALLFS_UI__LOG_BUFFER_ENTRIES` |
| `ui.audit_buffer_entries` | ui | `CALLFS_UI__AUDIT_BUFFER_ENTRIES` |
| `ui.event_buffer_entries` | ui | `CALLFS_UI__EVENT_BUFFER_ENTRIES` |
| `trash.enabled` | trash | `CALLFS_TRASH__ENABLED` |
| `trash.retention` | trash | `CALLFS_TRASH__RETENTION` |
| `log.mode` | Log mode | `CALLFS_LOG_MODE` |
