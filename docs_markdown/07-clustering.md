# Clustering

CallFS runs alone on one machine or as a cluster of several servers (nodes). Every node contains its own coordination service (embedded NATS), so you do not install a database, Redis or any other server to cluster. The cluster uses it for four jobs:

- the shared file index (which file lives where, who owns it, its mode and size),
- edit locks,
- node-to-node messages (events, audit, logs, background jobs),
- the files and settings every node needs to agree on.

You can use the built-in service (`nats.mode: embedded`, the default) or connect CallFS to NATS servers you run yourself (`nats.mode: external`, see [Using your own NATS servers](#using-your-own-nats-servers)).

File bytes stay on the node and backend that stored them. Any node can answer for any file: when the bytes live on another node, CallFS fetches or changes them there for you.

Raft cluster mode was removed in 2.0.0; configs that use it are rejected at startup. Use `metadata_store.type: nats` and `dlm.type: nats`. See [Upgrading from Raft](#upgrading-from-raft).

Related: [Configuration](02-configuration.md), [Storage](05-storage.md), [Monitoring](06-monitoring-metrics.md), [Web console](08-web-console.md), [Admin API](10-admin-api.md).

## Standalone node

One node needs no other servers. Turn the embedded service on and use it for the file index and locks:

```yaml
instance_discovery:
  instance_id: "node-1"

nats:
  enabled: true
  cluster_name: "callfs"
  jetstream:
    store_dir: "/var/lib/callfs/nats"

metadata_store:
  type: "nats"

dlm:
  type: "nats"
```

With no `nats.routes` and no `instance_discovery.peer_endpoints`, the node starts alone, opens no cluster port and no client port, and keeps everything in `nats.jetstream.store_dir`. Adding nodes later needs only the settings in [Building a cluster](#building-a-cluster).

You can also run one node on SQLite, PostgreSQL or Redis metadata with `nats.enabled: false` (the default). That is a single server. Several servers always need `nats.enabled: true`: `instance_discovery.peer_endpoints` is rejected when `nats.enabled` is `false`, and `dlm.type: local` is rejected when peers are listed.

## Building a cluster

Every node needs:

1. A unique `instance_discovery.instance_id`.
2. The same `nats.cluster_name` as all other nodes.
3. The same `auth.internal_proxy_secret` as all other nodes (at least 16 characters). Nodes use it to trust each other.
4. `instance_discovery.peer_endpoints` listing the other nodes and their HTTPS addresses.
5. TLS files for the cluster port when it is not a loopback address.
6. `metadata_store.type: nats` and `dlm.type: nats`.

The node list may include the node itself: an entry that matches this node's own ID is ignored, so you can give every node the same `peer_endpoints` block.

By default (`nats.routes_from_peers: true`) each node connects to the host of every peer in `peer_endpoints`, on its own `nats.cluster_listen` port. Give every node the same cluster port and you do not need `nats.routes`.

### Node identity: `instance_discovery`

| Key | Type | Default | What it does | Restart |
| --- | --- | --- | --- | --- |
| `instance_discovery.instance_id` | string | empty (required) | Name of this node. Unique in the cluster. Letters, digits, dot, dash and underscore. It appears in the `X-CallFS-Instance-ID` header and in the console node picker. | yes |
| `instance_discovery.peer_endpoints` | map of node ID to URL | empty | The other nodes and the web address each answers on. Addresses must use `https://` (plain `http://` is accepted only for loopback addresses) because the cluster secret is sent to them. | yes |

You can change a node's address while the server runs from the console Servers page or the admin API. See [Adding and removing nodes](#adding-and-removing-nodes).

### NATS settings

All settings live under `nats:`. Env var form: `CALLFS_NATS__CLUSTER_NAME`, `CALLFS_NATS__ROUTES` (comma-separated), and so on.

| Key | Type | Default | Allowed | What it does | Restart |
| --- | --- | --- | --- | --- | --- |
| `nats.enabled` | bool | `false` | | Turns on the coordination service. Required for `metadata_store.type: nats`, `dlm.type: nats` and any peers. | yes |
| `nats.mode` | enum | `embedded` | `embedded`, `external` | `embedded` runs NATS inside CallFS. `external` connects to NATS servers you run. | yes |
| `nats.server_name` | string | empty | no whitespace | Name of this node inside NATS. Empty uses `instance_discovery.instance_id`. | yes |
| `nats.cluster_name` | string | `callfs` | no whitespace, `.`, `*`, `>` | Name of the cluster. Must match on all nodes. | yes |
| `nats.cluster_listen` | host:port | `127.0.0.1:6222` | | Address other nodes connect to. Required in embedded mode. A non-loopback address requires `nats.tls.cert_file`, `key_file` and `ca_file`. | yes |
| `nats.client_listen` | host:port | empty | | Optional address for NATS command-line tools. Empty keeps NATS reachable only from inside CallFS. A non-loopback address requires `nats.tls.cert_file` and `key_file`. | yes |
| `nats.monitor_listen` | host:port | empty | | Optional NATS monitoring web page. Empty turns it off. The page has no sign-in: keep it on a private network. | yes |
| `nats.routes` | list | empty | `nats-route://host:port` or `tls-route://host:port` | Extra nodes to connect to, besides the ones taken from `peer_endpoints`. Env var: comma-separated. | yes |
| `nats.routes_from_peers` | bool | `true` | | Connect to every host in `instance_discovery.peer_endpoints` on this node's cluster port. | yes |
| `nats.server_tags` | map | empty | tag names without spaces or `:` | Labels attached to this node, for example `zone: a`. | yes |
| `nats.rpc_timeout` | duration | `10s` | greater than 0 | How long a node waits for an answer from another node. | yes |
| `nats.bytes` | enum | `http` | `http`, `chunked` | How file contents move between nodes. See [Node-to-node file transfer](#node-to-node-file-transfer). | yes |
| `nats.counters` | bool | `false` | | Share request limits across the cluster. See [Cluster-wide limits](#cluster-wide-limits). | yes |

If `cluster_listen` is a loopback address and no routes or peers exist, the node runs alone.

Example: use routes instead of deriving them from peers.

```yaml
nats:
  enabled: true
  cluster_name: "callfs"
  cluster_listen: "10.0.0.1:6222"
  routes_from_peers: false
  routes:
    - "tls-route://10.0.0.2:6222"
    - "tls-route://10.0.0.3:6222"
```

### Cluster security

Nodes authenticate to each other and encrypt the cluster port with TLS. The same certificate files protect the client port when `nats.client_listen` is set.

| Key | Type | Default | What it does | Restart |
| --- | --- | --- | --- | --- |
| `nats.tls.cert_file` | path | empty | Certificate this node presents. Set together with `key_file`. | yes |
| `nats.tls.key_file` | path | empty | Private key for the certificate. | yes |
| `nats.tls.ca_file` | path | empty | Certificate authority that signed all node certificates. Requires `cert_file` and `key_file`. | yes |
| `nats.tls.client_cert_file` | path | empty | Certificate this node presents when it connects out. Set together with `client_key_file`. | yes |
| `nats.tls.client_key_file` | path | empty | Key for `client_cert_file`. | yes |
| `nats.auth.mode` | enum | `derived` | `derived`, `nkey` or `creds`. `derived` builds the credentials from `auth.internal_proxy_secret`, so there is nothing else to set. `nkey` needs `nats.auth.nkey_seed_file`. `creds` needs `nats.auth.creds_file` and works only with `nats.mode: external`. External mode requires `nkey` or `creds`. | yes |
| `nats.auth.nkey_seed_file` | path | empty | Seed file when `mode: nkey`. | yes |
| `nats.auth.creds_file` | path | empty | Credentials file when `mode: creds`. | yes |
| `nats.auth.previous_secret` | secret | empty | The old `auth.internal_proxy_secret`, accepted while you change the secret node by node. At least 16 characters and different from the current secret. | yes |

#### Private CA and certificates

Use one certificate authority (CA) for the whole cluster, public or private. Give each node a certificate signed by it, valid for the node's cluster address (host name or IP), and put the CA certificate on every node.

- `nats.tls.cert_file` and `nats.tls.key_file`: this node's certificate and key.
- `nats.tls.ca_file`: the CA. Nodes verify each other against it in both directions.
- `nats.tls.client_cert_file` and `nats.tls.client_key_file`: optional certificate this node shows when it connects out. Leave empty to reuse the node certificate.
- A `cluster_listen` address that is not a loopback address needs `cert_file`, `key_file` and `ca_file`. A `client_listen` address that is not a loopback address needs `cert_file` and `key_file`. Without them the server refuses to start.
- Routes work with either `nats-route://` or `tls-route://` once the TLS files are set.

The web address between nodes (`instance_discovery.peer_endpoints`) is plain HTTPS and is checked separately. If your web certificates come from a private CA, set `backend.internal_proxy_ca_file` to that CA file on every node. CallFS then trusts it, besides the system CAs, when it talks to other nodes. It applies to requests forwarded to other nodes, node-to-node file reads and writes, erasure pieces on other nodes, the console server picker and the server address checks. Restart the node after changing it.

```yaml
backend:
  internal_proxy_ca_file: "/etc/callfs/tls/ca.pem"
```

Without TLS on a non-loopback cluster port the health findings show "Cluster messaging is not encrypted" (`nats.plain_transport`).

#### Auth modes

| Mode | Use it when | What to set |
| --- | --- | --- |
| `derived` | Embedded cluster. The default. | Only `auth.internal_proxy_secret`, equal on all nodes. |
| `nkey` | You want a key pair instead of a shared secret, embedded or external. | `nats.auth.nkey_seed_file`. |
| `creds` | External NATS that uses credentials files. | `nats.auth.creds_file`, with `nats.mode: external`. |

#### Rotating the secret

With `derived` mode, change `auth.internal_proxy_secret` without downtime:

1. On every node set `nats.auth.previous_secret` to the old secret and `auth.internal_proxy_secret` to the new one (at least 16 characters, and the two must differ).
2. Restart the nodes one at a time. Nodes on the old and the new secret work together while you do this.
3. When all nodes run the new secret, remove `nats.auth.previous_secret` and restart the nodes one at a time again.

### Storage of the cluster data (`nats.jetstream`)

The embedded service writes the file index, locks and messages to disk. Back this directory up with the rest of your state and keep it on a local, fast disk.

| Key | Type | Default | Allowed | What it does | Restart |
| --- | --- | --- | --- | --- | --- |
| `nats.jetstream.store_dir` | path | `./data/nats` | | Directory for cluster data. Required in embedded mode. Give each node its own directory. | yes |
| `nats.jetstream.max_memory` | bytes | `256MiB` | 0 or more | Memory limit for cluster data. | yes |
| `nats.jetstream.max_store` | bytes | `0` | 0 or more | Disk limit for cluster data. `0` means no limit. | yes |
| `nats.jetstream.sync` | enum | `always` | `always`, `interval` | `always` flushes each write to disk before it is confirmed. `interval` flushes on a timer. | yes |
| `nats.jetstream.sync_interval` | duration | `2m` | greater than 0 | Flush period when `sync: interval`. | yes |
| `nats.jetstream.unique_tag` | string | empty | | Tag family that keeps copies of data on different machines. Give each node a different value in the same family, for example `zone:a`, `zone:b`, `zone:c`. Empty turns the check off. | yes |

Use `sync: always` unless you accept that a power loss can lose the last writes of that node.

### Snapshots, compaction and restore

Every change to the file index is written to a log. To keep the log short, CallFS takes a snapshot of the whole index and then trims older changes.

- A snapshot is taken when `nats.metadata.snapshot_every` changes have happened since the last one, checked every `nats.metadata.snapshot_interval`.
- After a snapshot the log keeps the last `nats.metadata.retain_commands` changes. Older ones are removed.
- The newest `nats.metadata.retain_snapshots` snapshots are kept.
- When a node starts, it loads the newest valid snapshot and replays only the changes after it. A new or empty node does the same, so adding a node or restoring one from empty disk is fast and needs no manual step.
- To restore a node, start it with an empty `nats.jetstream.store_dir` and the same config. It rejoins, loads a snapshot and catches up.
- Take one yourself in the console under **Cluster, Replication, Take snapshot**, or with `POST /v1/admin/cluster/snapshot`. The page also lists the snapshots (ID, sequence, size, created) and how many are kept.
- If the newest snapshot gets older than expected, the health findings show "File index snapshot is out of date" (`nats.snapshot_stale`).

![Cluster, Replication](../docs/images/console/cluster-replication.png)

### Replicas

A replica is a copy of the cluster data on one node. Each kind of data has its own replica count. Copies are kept on as many nodes as the count says, up to the number of nodes you run.

| Key | Type | Default | Allowed | What it does | Restart |
| --- | --- | --- | --- | --- | --- |
| `nats.metadata.replicas` | int | `3` | 1 to 5 | Copies of the file index. | yes |
| `nats.locks.replicas` | int | `3` | 1 to 5 | Copies of the lock data. | yes |
| `nats.streams.replicas` | int | `3` | 1 to 5 | Copies of events, audit, logs, jobs and failed jobs. | no |

The effective number never exceeds the number of nodes (your own node plus the entries in `peer_endpoints`). One node uses one copy.

### Message and history retention (`nats.streams`)

| Key | Type | Default | Allowed | What it does | Restart |
| --- | --- | --- | --- | --- | --- |
| `nats.streams.events_max_age` | duration | `168h` | greater than 0 | How long cluster events are kept. | no |
| `nats.streams.events_max_bytes` | bytes | `1GiB` | 0 or more | Size limit for events. `0` means no limit. | no |
| `nats.streams.audit_max_age` | duration | `2160h` | greater than 0 | How long audit records are kept. | no |
| `nats.streams.logs_level` | enum | `warn` | `debug`, `info`, `warn`, `error` | Log lines at or above this level are shared, so the console log view shows the logs of all nodes. | no |
| `nats.streams.logs_max_age` | duration | `72h` | greater than 0 | How long shared logs are kept. | no |
| `nats.streams.tasks_max_deliver` | int | `8` | 1 or more | How many times a background copy task is tried before it moves to the failed tasks. Must be greater than the number of `tasks_backoff` entries. | no |
| `nats.streams.tasks_backoff` | list of durations | `1s, 5s, 30s, 2m, 10m` | each greater than 0 | Wait before each retry of a failed job. Env var: comma-separated. | no |
| `nats.streams.dlq_max_age` | duration | `720h` | greater than 0 | How long tasks that failed every try are kept. | no |

### File index tuning (`nats.metadata`)

| Key | Type | Default | Allowed | What it does | Restart |
| --- | --- | --- | --- | --- | --- |
| `nats.metadata.apply_timeout` | duration | `10s` | greater than 0 | How long a change waits to be confirmed by the cluster. | yes |
| `nats.metadata.read_barrier` | enum | `info` | `info`, `publish` | How a consistent read makes sure it sees the latest state. See [Consistent reads](#consistent-reads). | no |
| `nats.metadata.miss_barrier` | bool | `false` | | Re-check once with the latest state when a path is not found. See [Consistent reads](#consistent-reads). | yes |
| `nats.metadata.snapshot_interval` | duration | `5m` | greater than 0 | How often a snapshot of the index is taken. | yes |
| `nats.metadata.snapshot_every` | int | `50000` | 1 or more | Take a snapshot after this many changes. | yes |
| `nats.metadata.retain_commands` | int | `100000` | 1 or more | Number of recent changes kept for catching up. | yes |
| `nats.metadata.retain_snapshots` | int | `3` | 1 or more | Number of snapshots kept. | yes |
| `nats.metadata.max_msg_size` | bytes | `8MiB` | 64KiB to 64MiB | Largest single index change. | yes |

### Cluster-wide limits

With `nats.counters: true`, request limits are counted for the whole cluster instead of per node: download rate, link creation rate, sign-in session rate and the number of simultaneous websocket upload slots. A user cannot get around a limit by spreading requests over nodes. With `false` each node counts its own requests. Counts from before you turned it on are not included. See [Authentication and security](04-authentication-security.md) for the limits.

### Lock tuning (`nats.locks`)

| Key | Type | Default | Allowed | What it does | Restart |
| --- | --- | --- | --- | --- | --- |
| `nats.locks.ttl` | duration | `15s` | 1s or more | A lock held by a node that stops answering is released after this time. | yes |
| `nats.locks.queue_stale` | duration | `3s` | 1s or more | Callers waiting for a busy lock line up in arrival order (first come, first served). A waiter that stops answering is dropped from the line after this time. | yes |

### Using your own NATS servers

Set `nats.mode: external` to connect to NATS servers that you run instead of the built-in one. CallFS then does not start NATS or open cluster ports, and the `nats.cluster_listen`, `nats.jetstream.store_dir` and `nats.routes` settings do not apply. Your NATS must have JetStream enabled.

| Key | Type | Default | What it does | Restart |
| --- | --- | --- | --- | --- |
| `nats.external.urls` | list | empty (required) | Server addresses: `nats://`, `tls://`, `ws://` or `wss://`. Env var: comma-separated. | yes |
| `nats.external.js_domain` | string | empty | JetStream domain, if your NATS uses one. | yes |
| `nats.external.tls_ca_file` | path | empty | CA file to trust when connecting. | yes |

`nats.auth.mode` must be `nkey` or `creds` in external mode.

```yaml
nats:
  enabled: true
  mode: external
  cluster_name: "callfs"
  external:
    urls:
      - "tls://nats-1.example.com:4222"
      - "tls://nats-2.example.com:4222"
    tls_ca_file: "/etc/callfs/nats-ca.pem"
  auth:
    mode: creds
    creds_file: "/etc/callfs/callfs.creds"
```

### Node-to-node file transfer

Small control calls between nodes (asking who holds a file, locks, status) always go over the cluster connection when NATS is on, with the timeout `nats.rpc_timeout`. `nats.bytes` chooses how the file contents travel:

- `http` (default): contents go over HTTPS to the address in `instance_discovery.peer_endpoints`. Nodes need to reach each other's web port, and the certificates must be trusted (see `backend.internal_proxy_ca_file`).
- `chunked`: contents are sent in pieces over the cluster connection. Nodes then need only the cluster port between them for file transfers.

Both modes support downloads, uploads and range reads of files on another node, and the same permissions and errors (`502` when the node holding the file does not answer).

### Node presence and roles

Every node announces itself to the cluster every few seconds. `GET /v1/nodes` and the console use these announcements:

- `status`: `up` or `down`.
- `role`: `leader` (holds the lead for the file index), `follower`, or `offline` (no announcement received). `leader_id` names the current leader.
- `version`, `uptime_s` and `started_at` for admins.

**Cluster, Members** shows the same information as a table with state, online, last seen and how far each member is behind. **Cluster, Servers** adds free and total disk space per node.

### Cluster-wide background jobs

Some jobs must run once for the whole cluster, not once per node: link cleanup (every 5 minutes), trash cleanup (every 10 minutes) and usage refresh (every 10 minutes). Each runs on the one node that holds its lease. If that node stops, another takes over within about 30 seconds. **Run now** on a node that does not hold the lease is forwarded to the node that does. Jobs that only concern one node, such as the server address checks, run on every node. See the jobs list in the console under Activity, Jobs.

### Background copy tasks

Replication copies of files to other storages (see [Storage](05-storage.md)) are durable tasks shared by the cluster. If a node stops, its pending copies are picked up by another node and nothing is lost. A failed copy is retried after each wait in `nats.streams.tasks_backoff` (default 1s, 5s, 30s, 2m, 10m), up to `nats.streams.tasks_max_deliver` tries (default 8). After the last try the task moves to the failed tasks and is kept for `nats.streams.dlq_max_age` (default 30 days). `callfs_replication_total` counts the outcomes `succeeded`, `failed`, `dropped` and `skipped`, and the **Cluster, Backup copy** page shows the counts for the last 24 hours.

## How many nodes to run

The cluster keeps working while more than half of the copies of its data are reachable.

| Nodes | Replicas in use | Nodes you can lose and keep working |
| --- | --- | --- |
| 1 | 1 | 0 |
| 2 | 2 | 0 (losing either node stops writes) |
| 3 | 3 | 1 |
| 5 | 5 (set the three replica counts to 5) | 2 |

Run 3 nodes for most clusters. Avoid 2 nodes: they cost more than one node and do not survive a loss. For 5 nodes, set `nats.metadata.replicas`, `nats.locks.replicas` and `nats.streams.replicas` to `5`; the default of 3 still survives one lost node.

When a node is lost:

- The other nodes keep serving the file index, locks and the console, as long as the surviving nodes are a majority of the replicas.
- Files whose bytes live on the lost node cannot be read or changed until it returns. Requests for them return `502`. Files on the other nodes are not affected.
- When the node returns, it catches up on its own and its files are available again.
- If you lose the majority, writes that need the cluster wait and then fail until enough nodes return. Nothing that was confirmed is lost.

To also keep file bytes available when a node is lost, store them with replication or erasure coding. See [Storage](05-storage.md).

## Consistent reads

Each node holds a copy of the file index and receives changes from the cluster. A read on a node that has not yet received a very recent change from another node can briefly see old data. Three settings control this.

| Setting | Effect |
| --- | --- |
| `performance.mode: fast` (default) | Reads use the node's own copy. Writes are confirmed only after they are durable. |
| `performance.mode: strict` | Every read waits until the node has received everything the cluster has accepted. Also reads back each saved file and locks every change. Slower, always current. |
| `nats.metadata.miss_barrier: true` | In fast mode, when a path is not found, the node waits for the latest state and checks once more. Normal reads stay fast, and a file just created on another node is found instead of returning `404`. |

`nats.metadata.read_barrier` chooses how "wait for the latest state" works. This applies to strict reads, `miss_barrier` and operations that need a current answer:

- `info` (default): the node asks the cluster how much has been accepted and waits until it has received that much.
- `publish`: the node sends a marker through the cluster and waits until it receives it. Stronger and slower.

`read_barrier` can be changed at runtime. `performance.mode` and `miss_barrier` need a restart.

```yaml
performance:
  mode: strict

nats:
  metadata:
    read_barrier: "info"
    miss_barrier: true
```

## Locks

Locks stop two requests from changing the same file at the same time. In a cluster the lock must be shared by all nodes.

| Key | Type | Default | Allowed | What it does | Restart |
| --- | --- | --- | --- | --- | --- |
| `dlm.type` | enum | `redis` | `redis`, `local`, `nats` | Where locks are kept. Use `nats` in a cluster. `local` works on one node only and is rejected when peers are listed. `redis` uses a Redis server you run. | yes |
| `dlm.wait_timeout` | duration | `30s` | | How long a change waits for a lock before it fails. | no |
| `dlm.redis_addr`, `dlm.redis_password`, `dlm.redis_pool_size`, `dlm.redis_min_idle_conns`, `dlm.redis_tls`, `dlm.redis_tls_ca_file`, `dlm.redis_tls_skip_verify` | | | | Redis lock settings. Only used when `dlm.type: redis`. | yes |

With `dlm.type: nats`, `nats.locks.ttl`, `nats.locks.queue_stale` and `nats.locks.replicas` apply (see above). See locks held right now in the console under Cluster, Locks, or in the [Admin API](10-admin-api.md).

## Cross-server file operations

You can send a request to any node. Where the bytes live decides what happens.

- **Reads.** A download of a file stored on another node is fetched from that node and returned to you. You do not need to know which node holds it.
- **Metadata (`HEAD`).** Returns the file's details. `X-CallFS-Instance-ID` tells you which node holds the file. Other headers: `X-CallFS-Type`, `X-CallFS-Size`, `X-CallFS-Mode`, `X-CallFS-Owner`, `X-CallFS-MTime`, and `ETag` when available.
- **Updates (`PUT`).** If the file lives on another node, the upload is sent there and updated in place. A `PUT` to a new path creates the file on the node that received the request. `If-Match` and `If-None-Match` work as for a local file.
- **Deletes (`DELETE`).** Routed to the node that holds the file. `?recursive=true` on a directory is routed too. A non-empty directory without `recursive` returns `409`.
- **Creates (`POST`).** If the path is free everywhere, the file or directory is created on the node that received the request. If the path already exists on another node, the request returns `409`:

```json
{
  "error": "Resource exists on another server",
  "existing_path": "/shared/report.docx",
  "instance_id": "node-2",
  "backend_type": "localfs",
  "suggestion": "File already exists on another server. Use PUT to update it."
}
```

If the path exists on the same node you get `409` with a plain message such as `file already exists, use PUT to update`.

- **Rename and move (`PATCH`).** A move inside one node and backend is run by the node that holds the bytes. Add `"instance": "node-2"` to the body to move a file to another node, and `"backend"` to change the storage type at the same time. Moving to another node or backend needs an admin. Moving a directory to another backend or node is not supported and returns `400`. An unknown `instance` returns `400`. If `overwrite` is not `true` and the destination exists, the move returns `409`.

```bash
curl -X PATCH https://files.example.com/v1/files/reports/q3.pdf \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"destination":"/archive/q3.pdf","instance":"node-2","overwrite":false,"create_parents":true}'
```

The batch endpoint accepts the same `instance` field on `move` operations. See the [API reference](03-api-reference.md).

- **Errors.** When the node that holds the bytes does not answer, the request returns `502`. Permissions are checked on the node that receives the request, the same as for local files.

These features make one namespace out of all nodes: applications connect to any node (or a load balancer in front of all of them) and use the same paths.

Peers must be reachable over HTTPS at the address in `instance_discovery.peer_endpoints`, and all nodes must share the same `auth.internal_proxy_secret`.

## Controlling every node from one console

Sign in to the [web console](08-web-console.md) on any node and manage the whole cluster from there.

- Open **Cluster, Servers** (`/cluster/servers`). It lists every node with its status, connection speed, version, uptime and number of files it holds, and shows "N of M servers answer".
- Choose **Control** on a row, or use the server picker in the top bar, or the command palette command "Switch to server <name>". A strip at the top of the page shows "Controlling <name>" and says every change goes to that server. Choose **Back** to return to the server you signed in to.
- While you control a node, every console page (files, settings, activity, locks, storage and the rest) shows that node's data and applies changes to that node. A node that is down is greyed out in the picker and cannot be chosen.
- Only admin users see version, uptime and file counts for other nodes.
- Your sign-in is not copied to other nodes. The node you signed in to checks who you are and passes the request on with the cluster secret.

The same feature is available over the API at `/v1/nodes`:

| Request | What it does |
| --- | --- |
| `GET /v1/nodes` | Lists this node (`self`) and its peers with `id`, `endpoint`, `status` (`up` or `down`), `latency_ms`, `role` (`leader`, `follower` or `offline`), `leader_id`, and for admins `version`, `uptime_s`, `started_at`. |
| `ANY /v1/nodes/{node}/v1/...` | Sends the request to the named node. Allowed paths are `/v1/...`, `/health` and `/health/ready`, except `/v1/session` and `/v1/nodes`. |

Errors: `401` not signed in, `404` unknown node (`NODE_NOT_FOUND`), `400` path not allowed, `502` node not answering (`NODE_UNREACHABLE`), `508` a request that was already forwarded (`FORWARD_LOOP`). Responses from another node carry the header `X-CallFS-Node` with that node's ID.

```bash
curl -b cookies.txt https://node-1.example.com/v1/nodes
curl -b cookies.txt https://node-1.example.com/v1/nodes/node-2/v1/admin/status
```

## Adding and removing nodes

### Add a node

The console writes the new node's config for you.

1. Install CallFS on the new machine (see [Installation](01-installation.md)).
2. Open **Cluster, Members** and choose **Add member**. Enter the member ID (letters, digits, dot, dash, underscore; unique), the server address (`https://...`) and the cluster address (`host:port` that the other nodes use to reach it, for example `10.0.0.4:6222`). Choose **Create config**.
3. The dialog shows the config for the new node (`nats`, `instance_discovery`, `metadata_store`, `dlm`) and the next steps. Save it as the config file on the new machine. The same data is available from `GET /v1/admin/cluster/join-config?node_id=...&api_endpoint=...&cluster_addr=...`.
4. Add what the snippet leaves out: copy `auth.internal_proxy_secret`, `auth.single_use_link_secret` and the API keys from an existing node, and give the node its TLS files (`nats.tls.*`, `server.cert_file`, `server.key_file`, and `backend.internal_proxy_ca_file` for a private CA). Open the cluster port between the nodes.
5. Start the new node. It connects, loads a snapshot, catches up and takes copies of the cluster data.
6. Add `nats-route://<cluster address>` to `nats.routes` and the node to `instance_discovery.peer_endpoints` on the other nodes. They use it after their next restart (see [Rolling restarts](#rolling-restarts-and-upgrades)). To reach the node sooner for cross-server requests, use **Cluster, Servers, Add server address** below.
7. Check **Cluster, Members** and **Cluster, Servers** show it as online.

Without the console, write the same config by hand: a new `instance_id`, the same `nats.cluster_name` and `auth.internal_proxy_secret`, its own TLS files and `peer_endpoints` for the existing nodes.

To let a running node reach a new node without waiting for a restart, open **Cluster, Servers**, choose **Add server address**, enter the server ID and its web address (`https://...`). The address is used at once for cross-server requests. Choose **Change address** or **Remove address** on a row to edit or delete it. **Check now** tests a row. Settings saved this way can also be written to the config file. The same actions exist in the [Admin API](10-admin-api.md): `GET /v1/admin/instances`, `PUT /v1/admin/instances/{id}/endpoint`, `DELETE /v1/admin/instances/{id}/endpoint`, `POST /v1/admin/instances/{id}/probe`.

If you raise the node count from 1 or 2 to 3, restart each node so its replica count rises to 3.

### Remove a node

1. Move any files you want to keep off that node with `PATCH` and `"instance"`, or copy them out.
2. Stop the node.
3. Open **Cluster, Members**, open the row menu of the offline member and choose **Remove member** (or `DELETE /v1/admin/cluster/nodes/{id}`). Its copies of the file index move to the other servers. Only an offline member can be removed: an online member, or the server you are signed in to, returns `409`. An unknown ID returns `404`.
4. Remove it from `peer_endpoints` in the other nodes and restart them one at a time. If you use `nats.routes`, remove its route too.
5. Keep at least the majority of nodes running while you do this. Going from 3 nodes to 2 leaves a cluster that cannot lose a node.

Files whose bytes lived only on a removed node are no longer available.

### Move the file-index leader

One member leads the file index at a time. Failover is automatic. To move the lead on purpose, for example before maintenance on that node, open **Cluster, Replication, Move leader**, pick the new leader or **Automatic**, and confirm. The API is `POST /v1/admin/cluster/leadership-transfer` with optional `{"node_id": "node-2"}`. An unknown member returns `404`, an offline member returns `409`.

### Cluster admin API

| Request | What it does |
| --- | --- |
| `GET /v1/admin/cluster` | Mode, cluster name, leader, replica count, quorum, how many members can be lost, log numbers and the member list. |
| `GET /v1/admin/cluster/state-stats` | Counts of files, folders, links (active, used, expired, revoked), split files and index entries. |
| `GET /v1/admin/cluster/snapshots` | The snapshots and how many are kept. |
| `POST /v1/admin/cluster/snapshot` | Takes a snapshot now. |
| `POST /v1/admin/cluster/leadership-transfer` | Moves the leader. |
| `DELETE /v1/admin/cluster/nodes/{id}` | Removes an offline member. |
| `GET /v1/admin/cluster/join-config` | Config for a new member. |

On a node without the NATS cluster the write requests return `409`. See the [Admin API](10-admin-api.md).

### Cluster health findings

The console health view and the [monitoring](06-monitoring-metrics.md) page report these cluster findings: this server is not connected to NATS (`nats.not_connected`), the file index lost its majority (`nats.no_quorum`), file index copies are offline (`nats.replica_offline`), this server is behind on changes (`nats.log_lag`), the snapshot is out of date (`nats.snapshot_stale`) and cluster messaging is not encrypted (`nats.plain_transport`). Metrics are `callfs_nats_*`.

## Rolling restarts and upgrades

Restart one node at a time and wait for it to rejoin before you restart the next.

1. Stop the node (or `SIGTERM` it). The other nodes keep serving.
2. Upgrade the binary or change the config.
3. Start it. Wait until `GET /health/ready` returns `200` and **Cluster, Servers** shows it answering.
4. Move on to the next node.

With 3 nodes you can restart one at a time without interrupting service. Never stop two of three nodes together.

### Load balancer checks

Use `GET /health/ready` as the check of a load balancer. It answers only for the node you ask:

- `200` with `{"status":"ok"}` or `{"status":"degraded"}`: this node serves requests.
- `503` with `{"status":"down"}`: this node cannot serve (its own storage, file index or lock service is down). The balancer should skip it.
- Problems with other nodes never make a node fail the check. They are listed under `peers` in the answer, each with `name`, `status` and `detail`, so a lost node does not take the healthy ones out of rotation.

`GET /health` returns only `{"status": ...}` and is meant for process checks such as the Docker health check.

Changes marked "no" in the Restart column above apply without a restart. Everything else needs the node restarted.

## Upgrading from Raft

Raft cluster mode was removed in 2.0.0; configs that use it are rejected at startup. A config is rejected when it has a `raft:` section, any `CALLFS_RAFT_*` environment variable, or `metadata_store.type: raft` or `dlm.type: raft`. Use `metadata_store.type: nats` and `dlm.type: nats`. The `callfs cluster join` and `callfs cluster remove` commands no longer exist; add and remove nodes as described above.

## Full three-node example

All three nodes share the cluster name, the secrets, the API keys and the TLS authority (the same `ca.pem`). Each has its own `instance_id`, certificate and address. Because the node list may include the node itself, the `peer_endpoints` block is the same on all three.

`/etc/callfs/config.yaml` on node-1 (change `instance_id`, `external_url` and the TLS file names on the others):

```yaml
server:
  listen_addr: ":8443"
  external_url: "https://node-1.example.com:8443"
  cert_file: "/etc/callfs/tls/node-1.pem"
  key_file: "/etc/callfs/tls/node-1.key"

backend:
  internal_proxy_ca_file: "/etc/callfs/tls/ca.pem"

auth:
  api_key_users:
    admin: "your-api-key"
  admin_users: ["admin"]
  internal_proxy_secret: "a-long-random-secret-shared-by-all-nodes"
  single_use_link_secret: "another-long-random-secret"

instance_discovery:
  instance_id: "node-1"
  peer_endpoints:
    node-1: "https://node-1.example.com:8443"
    node-2: "https://node-2.example.com:8443"
    node-3: "https://node-3.example.com:8443"

metadata_store:
  type: "nats"

dlm:
  type: "nats"
  wait_timeout: 30s

performance:
  mode: fast

nats:
  enabled: true
  mode: embedded
  cluster_name: "callfs"
  server_name: "node-1"
  cluster_listen: "10.0.0.1:6222"
  routes_from_peers: true
  tls:
    cert_file: "/etc/callfs/tls/node-1.pem"
    key_file: "/etc/callfs/tls/node-1.key"
    ca_file: "/etc/callfs/tls/ca.pem"
  auth:
    mode: derived
  rpc_timeout: 10s
  bytes: http
  counters: true
  jetstream:
    store_dir: "/var/lib/callfs/nats"
    max_memory: 256MiB
    sync: always
  metadata:
    replicas: 3
    read_barrier: "info"
    miss_barrier: true
    snapshot_interval: 5m
    snapshot_every: 50000
    retain_snapshots: 3
  locks:
    replicas: 3
    ttl: 15s
    queue_stale: 3s
  streams:
    replicas: 3
    logs_level: warn
```

For this layout node-2 uses `10.0.0.2:6222` and node-3 uses `10.0.0.3:6222`. The cluster port must be open between the nodes, and the HTTPS port must be open between the nodes and to your clients. The same settings work from environment variables, for example `CALLFS_NATS__ENABLED=true`, `CALLFS_NATS__CLUSTER_LISTEN=10.0.0.1:6222` and `CALLFS_INSTANCE_DISCOVERY__INSTANCE_ID=node-1`.

Check the cluster:

```bash
curl -s https://node-1.example.com:8443/health/ready
curl -s -H "Authorization: Bearer your-api-key" https://node-1.example.com:8443/v1/nodes
```
