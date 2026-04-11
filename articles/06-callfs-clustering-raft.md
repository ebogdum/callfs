# Setting Up a CallFS Cluster with Raft Consensus

CallFS ships with a built-in distributed metadata store powered by the Raft consensus algorithm. Instead of treating clustering as an afterthought, it is a first-class operational mode: set `metadata_store.type: "raft"` in your configuration, point nodes at each other, and you have a **file server cluster** where every node shares the same consistent metadata view. Files uploaded to any node are immediately accessible from any other node — the cluster handles routing transparently.

This tutorial walks through a three-node production-grade cluster: how Raft keeps metadata consistent, how to configure and start each node, how to join them together, and how to tune the system for your workload.

---

## How Raft Works in CallFS

Raft is a consensus algorithm designed for understandability without sacrificing correctness. In the context of CallFS, it solves one specific problem: every node in a **file server cluster** must agree on which files exist, where each file is stored, and what its metadata looks like. Without consensus, two nodes could disagree on the state of the filesystem after a network partition or a leader failure.

### Leader Election

At startup, all nodes in the cluster participate in an election. The candidate that wins a majority of votes becomes the leader. In a three-node cluster, a majority is two nodes. The leader is the single authoritative source of write operations and remains in that role until it fails or loses connectivity. If the leader becomes unreachable, the remaining two nodes elect a new one automatically — no manual intervention required.

### Log Replication

Every metadata change — creating a file, updating its attributes, deleting it — is represented as a command that the leader appends to its Raft log. The leader replicates that log entry to all followers. Once a majority of nodes (two out of three) acknowledge the entry, the leader marks it as committed and applies it to the in-memory state machine. Followers apply the same entry to their own state machines, keeping all nodes consistent.

From a caller's perspective: when you upload a file to a follower node, the follower automatically forwards the metadata write to the current leader via an internal HTTP endpoint. The leader commits it through Raft and the follower waits for confirmation before responding to your client. You never need to know which node is the leader.

### Snapshots

Raft log entries accumulate indefinitely. Snapshots compact the log: the state machine serializes its entire in-memory state to disk, the log entries up to that point are discarded, and new nodes joining the cluster receive the snapshot instead of replaying every historical entry. CallFS triggers automatic snapshots based on `snapshot_interval` (elapsed time) and `snapshot_threshold` (number of committed entries since the last snapshot). It also forces a snapshot immediately after a new node joins, so late-joining followers can bootstrap quickly.

---

## Setting Up a Three-Node Cluster

The example below uses three machines:

| Node | IP Address | Raft Port | API Port |
|------|-----------|-----------|----------|
| node-1 | 10.0.0.1 | 7000 | 8443 |
| node-2 | 10.0.0.2 | 7000 | 8443 |
| node-3 | 10.0.0.3 | 7000 | 8443 |

All three nodes must be able to reach each other on both ports. The Raft port (7000) carries log replication and leader election traffic over raw TCP. The API port (8443) carries client requests and the internal forwarding protocol over HTTPS.

### Install CallFS on All Three Nodes

```bash
curl -Lo callfs https://github.com/ebogdum/callfs/releases/latest/download/callfs-linux-amd64 \
  && chmod +x callfs \
  && sudo mv callfs /usr/local/bin/

sudo mkdir -p /var/lib/callfs/raft /var/lib/callfs/data /etc/callfs
```

### Node 1 Configuration (Bootstrap Node)

Node 1 bootstraps the cluster. This means it initializes the Raft configuration with all three peers listed and starts accepting votes. Only one node in the initial cluster should have `bootstrap: true`. Set it to `false` on all subsequent nodes.

Create `/etc/callfs/config.yaml` on node 1:

```yaml
server:
  listen_addr: ":8443"
  protocol: "https"
  external_url: "https://node1:8443"
  cert_file: "/etc/callfs/server.crt"
  key_file: "/etc/callfs/server.key"

metadata_store:
  type: "raft"

raft:
  enabled: true
  node_id: "node-1"
  bind_addr: "10.0.0.1:7000"
  data_dir: "/var/lib/callfs/raft"
  bootstrap: true
  peers:
    node-2: "10.0.0.2:7000"
    node-3: "10.0.0.3:7000"
  api_peer_endpoints:
    node-2: "https://node2:8443"
    node-3: "https://node3:8443"
  apply_timeout: "10s"
  forward_timeout: "10s"
  snapshot_interval: "60s"
  snapshot_threshold: 256
  retain_snapshot_count: 2

auth:
  api_keys:
    - "your-api-key-here-change"
  internal_proxy_secret: "shared-cluster-secret-16ch"
  single_use_link_secret: "your-link-secret-here-16ch"

backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs/data"

dlm:
  type: "local"

instance_discovery:
  instance_id: "node-1"
  peer_endpoints:
    node-2: "https://node2:8443"
    node-3: "https://node3:8443"
```

### Node 2 and Node 3 Configuration

Nodes 2 and 3 use identical structure with three differences: `node_id`, `bind_addr`, `bootstrap: false`, and their own `external_url` and `instance_id`. Node 2's `/etc/callfs/config.yaml`:

```yaml
server:
  listen_addr: ":8443"
  protocol: "https"
  external_url: "https://node2:8443"
  cert_file: "/etc/callfs/server.crt"
  key_file: "/etc/callfs/server.key"

metadata_store:
  type: "raft"

raft:
  enabled: true
  node_id: "node-2"
  bind_addr: "10.0.0.2:7000"
  data_dir: "/var/lib/callfs/raft"
  bootstrap: false
  peers:
    node-1: "10.0.0.1:7000"
    node-3: "10.0.0.3:7000"
  api_peer_endpoints:
    node-1: "https://node1:8443"
    node-3: "https://node3:8443"
  apply_timeout: "10s"
  forward_timeout: "10s"
  snapshot_interval: "60s"
  snapshot_threshold: 256
  retain_snapshot_count: 2

auth:
  api_keys:
    - "your-api-key-here-change"
  internal_proxy_secret: "shared-cluster-secret-16ch"
  single_use_link_secret: "your-link-secret-here-16ch"

backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs/data"

dlm:
  type: "local"

instance_discovery:
  instance_id: "node-2"
  peer_endpoints:
    node-1: "https://node1:8443"
    node-3: "https://node3:8443"
```

Node 3 follows the same pattern with `node_id: "node-3"`, `bind_addr: "10.0.0.3:7000"`, `instance_id: "node-3"`, and peer entries pointing at nodes 1 and 2.

Two values must be identical across all nodes: `api_keys` and `internal_proxy_secret`. The internal proxy secret authenticates node-to-node requests — it must be at least 16 characters and must not be the literal string `change-me-internal-secret`.

---

## Starting the Cluster

Start node 1 first so it can bootstrap and become the initial leader:

```bash
# On node 1
sudo callfs server --config /etc/callfs/config.yaml
```

Then start nodes 2 and 3:

```bash
# On node 2
sudo callfs server --config /etc/callfs/config.yaml

# On node 3
sudo callfs server --config /etc/callfs/config.yaml
```

Because all peers are declared in the configuration and `bootstrap: true` on node 1 pre-seeds the Raft configuration with the full membership list, the nodes connect automatically. No separate join step is required for the initial three-node bootstrap.

---

## Joining Additional Nodes

If you need to expand the cluster after it is already running, bring up the new node with `bootstrap: false` and a configuration that names the existing peers, then register it with the current leader:

```bash
callfs cluster join \
  --leader https://node1:8443 \
  --config /etc/callfs/config.yaml
```

The `cluster join` command reads `raft.node_id`, `raft.bind_addr`, `server.external_url`, and `auth.internal_proxy_secret` from the config file automatically. You can override any of these with explicit flags (`--node-id`, `--raft-addr`, `--api-endpoint`, `--internal-secret`) if your config is not yet in place. The command sends a join request to the leader's internal endpoint, the leader adds the new node as a Raft voter, forces a snapshot, and the new node installs that snapshot to catch up with the committed history.

---

## Cross-Server Routing

Each file stored in CallFS is tagged with the `instance_id` of the node that wrote it. This tag lives in the shared Raft metadata. When you read a file from a node that did not write it, CallFS resolves the owning instance from metadata and proxies the request transparently:

1. Client sends `GET /v1/files/reports/q1.pdf` to node 2.
2. Node 2 queries its local metadata (forwarding to the leader if needed) and finds that `q1.pdf` lives on node 1.
3. Node 2 opens an internal HTTPS connection to node 1 and streams the file content back to the client.
4. The client receives the file with no knowledge of which physical node holds the bytes.

The same logic applies to writes, deletes, and HEAD requests. The routing is fully automatic; clients only need to know the address of any single node in the cluster.

The `instance_discovery.peer_endpoints` map is what enables this proxy routing. Every node must list all other nodes' API endpoints there, and the values must match the `server.external_url` of each peer.

---

## Testing the Cluster

With all three nodes running, verify cross-node consistency with a simple upload-and-download test.

Upload a file to node 1:

```bash
curl -k -X POST https://node1:8443/v1/files/hello.txt \
  -H "Authorization: Bearer your-api-key-here-change" \
  -H "Content-Type: application/octet-stream" \
  --data-binary "hello from node 1"
```

Download it from node 2:

```bash
curl -k https://node2:8443/v1/files/hello.txt \
  -H "Authorization: Bearer your-api-key-here-change"
```

You should receive `hello from node 1`. Node 2 found the metadata in the replicated Raft store, identified node 1 as the owning instance, proxied the request, and returned the content — all in a single HTTP round trip from the client's point of view.

To confirm the reverse direction, upload to node 2 and download from node 3:

```bash
curl -k -X POST https://node2:8443/v1/files/hello2.txt \
  -H "Authorization: Bearer your-api-key-here-change" \
  -H "Content-Type: application/octet-stream" \
  --data-binary "hello from node 2"

curl -k https://node3:8443/v1/files/hello2.txt \
  -H "Authorization: Bearer your-api-key-here-change"
```

---

## Leader Failover

In a three-node cluster, the system tolerates the failure of one node. If the current leader goes down, the two surviving nodes detect the absence of heartbeats, increment their election term, and vote for a new leader. The election completes in seconds. Once the new leader is established, writes and forwarded reads resume automatically.

No configuration changes are needed on surviving nodes. The `api_peer_endpoints` map already lists all peers, so the newly elected leader can forward internal commands to the remaining follower without any reconfiguration.

If the failed node comes back online, it reconnects to the cluster, discovers the current leader via Raft's membership configuration, and begins receiving log entries or installs a snapshot to catch up — whichever is more efficient based on how far behind it is.

To simulate a failover:

```bash
# Stop node 1 (the initial leader)
sudo systemctl stop callfs   # or kill the process

# After a few seconds, upload to node 2
curl -k -X POST https://node2:8443/v1/files/after-failover.txt \
  -H "Authorization: Bearer your-api-key-here-change" \
  -H "Content-Type: application/octet-stream" \
  --data-binary "written after leader failover"

# Download from node 3
curl -k https://node3:8443/v1/files/after-failover.txt \
  -H "Authorization: Bearer your-api-key-here-change"
```

The write succeeds because node 2 or node 3 has been elected the new leader. The download from node 3 succeeds because the metadata was committed to the Raft log and both remaining nodes have it.

---

## Raft Configuration Tuning

The default values are conservative and suitable for most deployments. The parameters worth adjusting are:

### `snapshot_interval`

How often CallFS checks whether a snapshot is due. Default: `60s`. On a busy cluster that commits thousands of entries per minute, you may lower this to `30s` so that snapshots are taken more frequently and log files stay smaller. On a low-traffic cluster, `120s` or higher reduces unnecessary I/O.

### `snapshot_threshold`

The number of committed log entries that must accumulate since the last snapshot before a new one is triggered. Default: `256`. Raise this (e.g., `1024`) if your workload is write-heavy and you want snapshots to be less frequent but cover more changes. Lower it (e.g., `64`) if new nodes join regularly and you want them to get a recent snapshot quickly.

### `apply_timeout`

How long the leader waits for a Raft `Apply` call to be committed by a quorum. Default: `10s`. In high-latency wide-area deployments, increase this to `30s` to avoid spurious timeouts when inter-node round trips are slow. In a low-latency LAN cluster, `5s` is usually sufficient.

### `forward_timeout`

How long a follower waits for the leader to respond to a forwarded command. Default: `10s`. This should be at least as large as `apply_timeout` so the follower does not time out before the leader has had a chance to commit the entry. In practice, set both to the same value.

### `retain_snapshot_count`

How many snapshots to keep on disk. Default: `2`. Keeping two snapshots means you always have a fallback if the most recent snapshot is corrupt. Increase to `3` or more if you want additional recovery options; each snapshot is the full serialized state of the metadata store, so size it accordingly.

### Example Production Configuration

For a LAN cluster with moderate write traffic:

```yaml
raft:
  apply_timeout: "10s"
  forward_timeout: "10s"
  snapshot_interval: "30s"
  snapshot_threshold: 512
  retain_snapshot_count: 3
```

For a geographically distributed **distributed file system** deployment with higher latency between nodes:

```yaml
raft:
  apply_timeout: "30s"
  forward_timeout: "30s"
  snapshot_interval: "120s"
  snapshot_threshold: 1024
  retain_snapshot_count: 2
```

---

## Summary

A CallFS cluster with Raft consensus gives you a **distributed file system** that is straightforward to operate: one binary, one configuration file per node, no external coordination service. Raft handles leader election and log replication automatically. Cross-server routing is transparent to clients. Snapshots compact the log and allow late-joining nodes to catch up quickly. Failover is automatic the moment a quorum of surviving nodes can elect a new leader.

Start with a three-node cluster on a LAN, validate cross-node upload and download, then tune `snapshot_interval`, `snapshot_threshold`, `apply_timeout`, and `forward_timeout` to match your actual write volume and network characteristics.
