# Setting Up a Three-Node CallFS Cluster

A single CallFS server is easy to run, but one machine is also one point of failure. CallFS 2.0.0 can run as a **file server cluster** with nothing to install besides CallFS itself. Every node carries a built-in coordination service (embedded NATS) that shares the file index, the edit locks and the activity history between the nodes. You do not run a database, Redis or a separate consensus service. Raft cluster mode was removed in 2.0.0; configs that use it are rejected at startup. Use `metadata_store.type: nats` and `dlm.type: nats`.

You can send a request to any node. A file uploaded to node 1 shows up on node 2 and node 3 at once, and a download from node 2 is fetched from node 1 behind the scenes. If one node goes down, the other two keep accepting changes.

This tutorial builds a three-node cluster from scratch: certificates, secrets, one config file per node, startup, a cross-node test, the web console cluster pages, and what to expect when a node goes down.

---

## How a CallFS Cluster Works

- **Each node keeps its own file bytes.** A file you upload to node 2 is stored on node 2's disk (or in S3 if that is the default storage).
- **All nodes share the file index.** The index says which files exist and which node holds each one. Every node keeps a full copy, so any node can answer "does this file exist?" without asking the others.
- **Locks are shared.** Two clients changing the same file through different nodes still wait for each other.
- **Requests are routed for you.** When a node receives a request for a file whose bytes live on another node, it fetches or changes the file there and returns the result to your client.

Three nodes is the sweet spot. The cluster keeps working while a majority of the copies of its data are reachable, so three nodes survive the loss of any one node. Two nodes cost more than one and cannot survive a loss. See [Clustering](../docs_markdown/07-clustering.md) for the full table.

---

## What You Need

| Node | Name | IP address | HTTPS port | Cluster port |
|------|------|-----------|-----------|--------------|
| node-1 | node-1.example.com | 10.0.0.1 | 8443 | 6222 |
| node-2 | node-2.example.com | 10.0.0.2 | 8443 | 6222 |
| node-3 | node-3.example.com | 10.0.0.3 | 8443 | 6222 |

- Three Linux machines that can reach each other on both ports. The HTTPS port carries client requests and node-to-node file traffic. The cluster port (`nats.cluster_listen`) carries index, lock and event traffic.
- The same CallFS 2.0.0 binary on all three.
- `openssl` on one machine, to create certificates.
- The web console is included in the release binaries and the Docker image, at `/ui/`. See [Installation](../docs_markdown/01-installation.md).

Install the binary on each node and create its folders:

```bash
VERSION=v2.0.0
curl -LO https://github.com/ebogdum/callfs/releases/download/$VERSION/callfs-linux-amd64
curl -LO https://github.com/ebogdum/callfs/releases/download/$VERSION/SHA256SUMS
shasum -a 256 -c SHA256SUMS --ignore-missing
sudo install -m 755 callfs-linux-amd64 /usr/local/bin/callfs

sudo mkdir -p /var/lib/callfs/data /var/lib/callfs/nats /etc/callfs/tls
```

---

## Step 1: Create the Secrets

Three secrets must be the same on every node. Generate them once and copy them to all three config files.

```bash
openssl rand -hex 32   # use as the API key
openssl rand -hex 32   # use as auth.internal_proxy_secret
openssl rand -hex 32   # use as auth.single_use_link_secret
```

| Secret | Why it must match |
|--------|-------------------|
| API key | Clients can use the same key on any node. |
| `auth.internal_proxy_secret` | Nodes use it to trust each other. It is at least 16 characters and different from every API key and from the link secret. |
| `auth.single_use_link_secret` | A download link created on one node can be opened on any node. |

CallFS rejects values that contain `your-`, `change-me` or `a-strong-`, so do not paste the examples in this article as they are.

---

## Step 2: Create the Certificates

Nodes talk to each other over HTTPS, and CallFS requires TLS files for the cluster port whenever `nats.cluster_listen` is not a loopback address. A small private certificate authority (CA) is the simplest way to cover both.

On any one machine, create the CA:

```bash
openssl genrsa -out ca.key 4096
openssl req -x509 -new -nodes -key ca.key -sha256 -days 3650 \
  -subj "/CN=callfs-cluster-ca" -out ca.pem
```

Create one certificate per node. This example is for node-1; repeat it with the right name and IP for node-2 and node-3:

```bash
openssl genrsa -out node-1.key 2048
openssl req -new -key node-1.key -subj "/CN=node-1.example.com" -out node-1.csr
cat > node-1.ext <<'EOF'
subjectAltName = DNS:node-1.example.com, IP:10.0.0.1
extendedKeyUsage = serverAuth, clientAuth
EOF
openssl x509 -req -in node-1.csr -CA ca.pem -CAkey ca.key -CAcreateserial \
  -days 825 -sha256 -extfile node-1.ext -out node-1.pem
```

Copy `ca.pem`, `node-N.pem` and `node-N.key` to `/etc/callfs/tls/` on node N. Keep `ca.key` offline.

Each node must also trust the CA when it calls its peers. You do not need to install the CA on the machine: set `backend.internal_proxy_ca_file` to the CA file and CallFS uses it, in addition to the system roots, to verify the HTTPS certificates of the other servers. It applies to peer file requests, erasure coding, the node gateway and server health probes (see Step 3).

You use the same CA file with `curl --cacert ca.pem` from your own machine later.

---

## Step 3: Write the Config for Node 1

Create `/etc/callfs/config.yaml` on node 1:

```yaml
server:
  listen_addr: ":8443"
  protocol: "https"
  external_url: "https://node-1.example.com:8443"
  cert_file: "/etc/callfs/tls/node-1.pem"
  key_file: "/etc/callfs/tls/node-1.key"

auth:
  api_key_users:
    alice: "REPLACE_WITH_RANDOM_API_KEY_1"
  admin_users: ["alice"]
  internal_proxy_secret: "REPLACE_WITH_RANDOM_CLUSTER_SECRET"
  single_use_link_secret: "REPLACE_WITH_RANDOM_LINK_SECRET"

backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs/data"
  internal_proxy_ca_file: "/etc/callfs/tls/ca.pem"

metadata_store:
  type: "nats"

dlm:
  type: "nats"

instance_discovery:
  instance_id: "node-1"
  peer_endpoints:
    node-1: "https://node-1.example.com:8443"
    node-2: "https://node-2.example.com:8443"
    node-3: "https://node-3.example.com:8443"

nats:
  enabled: true
  mode: "embedded"
  cluster_name: "callfs"
  cluster_listen: "10.0.0.1:6222"
  routes_from_peers: true
  tls:
    cert_file: "/etc/callfs/tls/node-1.pem"
    key_file: "/etc/callfs/tls/node-1.key"
    ca_file: "/etc/callfs/tls/ca.pem"
  jetstream:
    store_dir: "/var/lib/callfs/nats"
    sync: "always"
  metadata:
    replicas: 3
    miss_barrier: true
  locks:
    replicas: 3
  streams:
    replicas: 3
```

What each part does:

- **`backend.internal_proxy_ca_file`** is an extra certificate authority used to verify the HTTPS certificates of the other servers. Use it when your certificates come from a private CA, as in this tutorial. Changing it needs a restart.
- **`metadata_store.type: nats`** and **`dlm.type: nats`** put the file index and the locks on the cluster. Both require `nats.enabled: true`. `dlm.type: local` is rejected when peers are listed.
- **`instance_discovery.instance_id`** is the unique name of this node. It also becomes the NATS server name unless you set `nats.server_name`.
- **`instance_discovery.peer_endpoints`** lists the nodes and the HTTPS address each one answers on. Addresses must use `https://` (plain `http://` is accepted only for loopback addresses) because the cluster secret is sent to them. An entry for the node itself is ignored, so the block can be identical on all three nodes.
- **`nats.routes_from_peers: true`** (the default) connects each node to the host of every peer on its own `nats.cluster_listen` port. Because all three nodes use port 6222, you do not need `nats.routes`.
- **`nats.cluster_listen`** is the address the other nodes connect to. Because it is not a loopback address, `nats.tls.cert_file`, `key_file` and `ca_file` are required. With them, the cluster port uses TLS and every node proves its identity to the others with its certificate (mutual verification against `ca_file`). The same files also protect the client port when you set `nats.client_listen` to a non-loopback address. If the client side should use a different certificate, set `nats.tls.client_cert_file` and `nats.tls.client_key_file` together.
- **`nats.jetstream.store_dir`** is where the node keeps its copy of the cluster data. Give every node its own local directory. `sync: always` flushes each write to disk before it is confirmed.
- **`replicas: 3`** keeps three copies of the file index, the lock data and the activity history. The number actually used never exceeds the number of nodes you run, so a cluster of two nodes uses two copies and a single node uses one.
- **`miss_barrier: true`** makes a node that cannot find a path wait for the latest cluster state and look once more, so a file created a moment ago on another node is found instead of returning `404`.

### Routes instead of peers

If you prefer to list the cluster addresses yourself, turn the automatic routes off:

```yaml
nats:
  routes_from_peers: false
  routes:
    - "nats-route://10.0.0.2:6222"
    - "nats-route://10.0.0.3:6222"
```

Routes accept the `nats-route://` and `tls-route://` schemes. From the environment, use `CALLFS_NATS__ROUTES="nats-route://10.0.0.2:6222,nats-route://10.0.0.3:6222"` (comma separated).

---

## Step 4: Write the Config for Nodes 2 and 3

Copy the file and change only these values:

| Setting | node-2 | node-3 |
|---------|--------|--------|
| `server.external_url` | `https://node-2.example.com:8443` | `https://node-3.example.com:8443` |
| `server.cert_file` and `nats.tls.cert_file` | `/etc/callfs/tls/node-2.pem` | `/etc/callfs/tls/node-3.pem` |
| `server.key_file` and `nats.tls.key_file` | `/etc/callfs/tls/node-2.key` | `/etc/callfs/tls/node-3.key` |
| `instance_discovery.instance_id` | `node-2` | `node-3` |
| `nats.cluster_listen` | `10.0.0.2:6222` | `10.0.0.3:6222` |

Everything else stays the same: the three secrets, the API key, `cluster_name`, the `peer_endpoints` block and the replica counts.

Check each file before you start:

```bash
callfs config validate --config /etc/callfs/config.yaml
```

The command prints the loaded settings, or the first error, for example `nats.cluster_name is required and must not contain whitespace, '.', '*' or '>'`.

You can also set any value from environment variables instead of YAML. The prefix is `CALLFS_` and nesting uses a double underscore: `CALLFS_NATS__ENABLED=true`, `CALLFS_NATS__CLUSTER_LISTEN=10.0.0.1:6222`, `CALLFS_INSTANCE_DISCOVERY__INSTANCE_ID=node-1`.

---

## Step 5: Start the Cluster

Run CallFS as a service on every node (the unit file is in [Installation](../docs_markdown/01-installation.md#run-as-a-systemd-service)) or start it by hand:

```bash
sudo callfs server --config /etc/callfs/config.yaml
```

Start the three nodes within about two minutes of each other. A node that starts first waits for the cluster to form and logs `Waiting for the NATS cluster before creating streams`. It gives up after two minutes. There is no bootstrap node and no join command: all nodes are equal, and the order does not matter.

Check readiness on each node:

```bash
curl --cacert ca.pem https://node-1.example.com:8443/health/ready
```

```json
{"status":"ok"}
```

`ok` means this node is fine (HTTP 200). `degraded` (HTTP 200) means a warning, such as a certificate that expires soon. `down` (HTTP 503) means a component of this node failed. Readiness reflects only this node: if a peer is down, the answer stays `ok` or `degraded` and the peer is listed under `peers`, for example `{"status":"ok","peers":[{"name":"storage.internal_proxy","status":"down","detail":"peer down"}]}`. A peer problem never makes readiness fail. `/health/ready` is public and needs no key. `/health` is a cheaper liveness check that answers `{"status":"ok"}` as long as the process is up.

Confirm that the nodes see each other. With your API key:

```bash
curl --cacert ca.pem -H "Authorization: Bearer your-api-key" \
  https://node-1.example.com:8443/v1/nodes
```

The answer lists this node (`"self": "node-1"`) and its peers, each with `id`, `endpoint`, `status` (`up` or `down`) and `latency_ms`. Every node announces itself to the others, so the list also shows nodes that went offline. Each entry has a `role` (`leader`, `follower` or `offline`). Admin users also get `version`, `uptime_s` and `started_at`.

The Prometheus metric `callfs_nats_routes` shows how many cluster links a node has. On a healthy three-node cluster every node reports 2. See [Monitoring](../docs_markdown/06-monitoring-metrics.md#embedded-nats-clustering).

---

## Step 6: Test the Cluster

Upload a file to node 1:

```bash
curl --cacert ca.pem -X POST https://node-1.example.com:8443/v1/files/hello.txt \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/octet-stream" \
  --data-binary "hello from node 1"
```

A new file returns `201 Created` with an `ETag` header. Download it from node 2:

```bash
curl --cacert ca.pem https://node-2.example.com:8443/v1/files/hello.txt \
  -H "Authorization: Bearer your-api-key"
```

You get `hello from node 1`. Node 2 found the file in the shared index, saw that node 1 holds the bytes, fetched them and returned them to you. To see which node owns a file, send a `HEAD` request and read `X-CallFS-Instance-ID`:

```bash
curl --cacert ca.pem -I https://node-3.example.com:8443/v1/files/hello.txt \
  -H "Authorization: Bearer your-api-key"
```

```
HTTP/2 200
x-callfs-type: file
x-callfs-size: 17
x-callfs-instance-id: node-1
```

Now upload to node 2 and read from node 3 the same way. Some behaviors to know:

| Request | What happens |
|---------|--------------|
| `GET` for a file held by another node | The node fetches the bytes from the owner and returns them. |
| `PUT` for a file held by another node | The upload is sent to the owner and the file is updated in place. A `PUT` to a new path creates the file on the node that received the request. |
| `DELETE` for a file held by another node | The owner deletes the bytes, and the index entry is removed. |
| `POST` to a path that already exists on another node | `409` with the owner's `instance_id` and a hint to use `PUT`. |
| Owner node does not answer | `502`. |

---

## Step 7: Use the Web Console

If you run the Docker image or a source build, open `https://node-1.example.com:8443/ui/` and sign in with an access key (see [Web console](../docs_markdown/08-web-console.md)). The cluster pages are under **Cluster**:

- **Topology** draws the cluster. **Members** is a table of the nodes with their role (leader, follower or offline); an admin can remove an offline member there, and **Add member** shows a generated config snippet and the steps to join a new node.
- **Replication** shows the state of the shared file index (log statistics), how far each node's copy lags behind, the snapshots (an admin can press **Take snapshot**) and **Move leader** to hand the leader role to another node.
- **Servers** lists every node with its status, connection speed, version, uptime, free and total disk space and the number of files it holds, and shows "N of M servers answer". Admin users see version, uptime, disk and file counts; other users see status.
- **Control** on a row, the server picker in the top bar, or the command palette entry "Switch to server node-2" makes the whole console act on that node. A strip at the top says "Controlling node-2" and **Back** returns you to the node you signed in to. Files, settings, activity, locks and storage pages all show and change that node.
- A node that is down is greyed out in the picker and cannot be chosen.
- **Add server address**, **Change address**, **Remove address** and **Check now** edit and test a peer address while the server runs, without a restart.
- **Locks** shows the edit locks held across the cluster. **Storage** shows where each node keeps its bytes.

Your sign-in is not copied to the other nodes. The node you signed in to checks who you are and passes the request on with the cluster secret, so one sign-in is enough to control every node.

---

## What Happens When a Node Goes Down

With three nodes and `replicas: 3`, the cluster keeps working while any two nodes are up. Try it. Stop node 1:

```bash
sudo systemctl stop callfs
```

Then, from your machine:

```bash
# Writes still work: node 2 and node 3 are a majority
curl --cacert ca.pem -X POST https://node-2.example.com:8443/v1/files/after-failure.txt \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/octet-stream" \
  --data-binary "written while node 1 is down"

# The new file is readable from node 3
curl --cacert ca.pem https://node-3.example.com:8443/v1/files/after-failure.txt \
  -H "Authorization: Bearer your-api-key"

# A file whose bytes live on node 1 cannot be read right now
curl --cacert ca.pem -i https://node-3.example.com:8443/v1/files/hello.txt \
  -H "Authorization: Bearer your-api-key"
```

The last request returns `502` because the node that holds `hello.txt` does not answer. Files on node 2 and node 3 are not affected.

| What you see | Why |
|--------------|-----|
| New uploads, changes to files on the live nodes and locks keep working | A majority of the copies of the file index and locks is still reachable. |
| Files stored on the stopped node return `502` | Their bytes are only on that node. |
| The console picker greys out node-1 and **Servers** shows "2 of 3 servers answer" | Status is checked live. |
| `/health/ready` on node 2 and node 3 stays `200` and lists node 1 under `peers` as `down` | Readiness reflects only the node you ask. Use it for load balancers; watch `peers` and `callfs_nats_routes` for alerts. |
| The metric `callfs_nats_routes` drops to 1 on the survivors | Each survivor has one cluster link left. |

When you start node 1 again it reconnects, catches up on every change it missed and its files are served again. Nothing to run by hand.

If a second node goes down, the cluster no longer has a majority. Requests that need the cluster wait and then fail until a node returns. Everything the cluster confirmed before that is kept.

Never stop two of three nodes at the same time on purpose. To upgrade, restart one node at a time and wait for `/health/ready` to return `200` before the next one.

To keep file bytes readable when a node is lost, also turn on a backup copy (`ha.replication_enabled`) or erasure coding. See [Storage](../docs_markdown/05-storage.md) and [Erasure coding](05-callfs-erasure-coding.md).

---

## Tuning

The defaults suit most clusters. These are the settings worth knowing:

| Setting | Default | When to change it |
|---------|---------|-------------------|
| `performance.mode` | `fast` | Set `strict` when every read must see the latest cluster state. Strict also reads back every saved file and locks every change. Both modes only confirm a write once it is durable. |
| `nats.metadata.miss_barrier` | `false` | Set `true` (as in this tutorial) so a just-created file on another node is found. |
| `nats.metadata.read_barrier` | `info` | `publish` waits for a marker sent through the cluster. Stronger and slower. Can be changed at runtime. |
| `nats.metadata.replicas`, `nats.locks.replicas`, `nats.streams.replicas` | `3` | Set all three to `5` for a five-node cluster that survives two lost nodes. Allowed range is 1 to 5. |
| `nats.metadata.apply_timeout` | `10s` | Raise it on a slow wide-area network. |
| `nats.rpc_timeout` | `10s` | How long a node waits for an answer from another node. |
| `nats.bytes` | `http` | `http` sends file bytes between nodes over HTTPS. `chunked` sends them in chunks over the cluster connection. Control calls always use the cluster connection. |
| `nats.locks.queue_stale` | `3s` | Waiting lock requests are served first come, first served; a waiter that stopped asking is skipped after this time. |
| `nats.metadata.snapshot_interval`, `snapshot_every`, `retain_commands`, `retain_snapshots` | `5m`, `50000`, `100000`, `3` | The file index is snapshotted and old entries are removed. A node restores from the newest valid snapshot when it starts. |
| `nats.counters` | `false` | Share request limits (download, link and session rates, websocket upload slots) across the whole cluster instead of per node. |
| `nats.streams.logs_level` | `warn` | Logs at or above this level are shared, so the console log view shows every node. |
| `nats.streams.tasks_max_deliver`, `tasks_backoff` | `8`, `1s 5s 30s 2m 10m` | Background copy (replication) tasks are retried with these delays. Tasks that still fail are kept for review for `nats.streams.dlq_max_age` (default `720h`). |
| `nats.auth.mode` | `derived` | `derived` builds the cluster credentials from `auth.internal_proxy_secret`. `nkey` signs in with the seed in `nats.auth.nkey_seed_file`; `creds` is only for your own NATS servers. Use `nats.auth.previous_secret` to change the secret one node at a time. |
| `nats.monitor_listen` | empty | Address for the built-in NATS HTTP monitoring page. |
| `nats.server_tags`, `nats.jetstream.unique_tag` | empty | Tags that spread copies across zones; with `unique_tag` set, no two copies land on nodes with the same tag value. |
| `nats.locks.ttl` | `15s` | A lock held by a node that stops answering is released after this time. |
| `nats.jetstream.sync` | `always` | `interval` flushes on a timer (`nats.jetstream.sync_interval`, default `2m`). A power loss can then lose the last writes of that node. |
| `nats.jetstream.max_store` | `0` (no limit) | Cap the disk used by the cluster data. |
| `nats.streams.events_max_age`, `audit_max_age`, `logs_max_age` | `168h`, `2160h`, `72h` | How long cluster events, audit records and shared logs are kept. |

Back up `nats.jetstream.store_dir` on at least one healthy node together with each node's `backend.localfs_root_path`.

A change to `nats.*` and `instance_discovery.*` needs a restart of that node, except `nats.metadata.read_barrier` and the `nats.streams.*` retention settings.

---

## Adding or Removing a Node

To add a node, give it a new `instance_id`, the same cluster name and secrets, its own certificate and the `peer_endpoints` block. Add it to `peer_endpoints` on the existing nodes and restart them one at a time. If you grow from one or two nodes to three, restart each node so its replica count rises to three.

To remove a node, move the files you want to keep off it with a `PATCH` that names another `instance`, stop it, delete it from `peer_endpoints` on the others and restart them one at a time. Files whose bytes lived only on the removed node are no longer available.

The `callfs cluster join` and `callfs cluster remove` commands of earlier releases no longer exist. Full details are in [Clustering](../docs_markdown/07-clustering.md).

---

## Summary

A CallFS 2.0.0 cluster gives you a **distributed file system** with one binary and one config file per node: no database, no Redis, no separate coordination service. Set `nats.enabled: true`, `metadata_store.type: nats` and `dlm.type: nats`, list the other nodes in `instance_discovery.peer_endpoints`, give each node TLS files for the cluster port, and start them in any order. Any node accepts any request, three nodes survive the loss of one, and the console lets you control every node from one place.

Next steps: add the [Prometheus and Grafana monitoring](10-callfs-prometheus-grafana-monitoring.md) from this series, enable a backup copy or erasure coding so file bytes survive a lost node, and read [Clustering](../docs_markdown/07-clustering.md) for rolling upgrades and your own NATS servers.
