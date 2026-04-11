# What Is a Distributed File System? A Practical Guide with Working Examples

Storing files on a single server works well until it does not. At some point every growing application hits the ceiling: the disk fills up, a hardware failure wipes out a week of uploads, or the machine simply cannot handle the volume of concurrent reads. At that point, you need a distributed file system. This guide explains what that means, how the core concepts fit together, and then walks through a concrete three-node cluster built with CallFS and Raft consensus so you can see the ideas in practice.

---

## What Is a File Server?

Before addressing distribution, it helps to be precise about what a file server is.

A file server is a process (or dedicated machine) that accepts requests to read, write, and list files, and stores those files on attached storage. Clients connect over a network protocol—NFS, SMB, HTTP, or a custom binary protocol—and the server translates those requests into local filesystem operations.

A file server has one job: expose storage over a network. The simplicity is its strength. The simplicity is also its weakness. A single file server is a single point of failure, a single bottleneck, and a single unit of capacity. When any of those become a problem, you need something more.

---

## Why One Server Is Not Enough

Consider a web application that lets users upload images. For the first few months everything fits on one machine. Then:

- **Capacity**: The 2 TB disk fills up. You can add more disks, but only so many fit in one chassis.
- **Throughput**: Thousands of concurrent uploads saturate the network interface or disk I/O.
- **Availability**: A power supply failure takes down every upload for hours.
- **Latency**: Users in different regions are all hitting one server in one data center.

None of these problems have a single-server solution. You need to spread the data—and the load—across multiple machines.

---

## What Is a Distributed File System?

A distributed file system is a file system whose storage and coordination are spread across multiple machines, while appearing to clients as a single coherent namespace.

The keyword is "coherent." Any client, connecting to any node, should see the same directory tree, the same files, and the same data. Achieving that coherence is the entire problem. Everything else—replication, consensus, sharding—exists to solve coherence at scale.

### Metadata vs Data

Every distributed file system makes a fundamental separation between two kinds of information:

**Metadata** describes a file: its name, path, size, permissions, timestamps, and which node actually holds the bytes. Metadata is small but critically important. Every operation—even checking whether a file exists—requires a metadata lookup.

**Data** is the actual file content. Data is large but relatively simple to distribute: you can store it on any node that has capacity, and reading it does not require global coordination.

This separation matters because the hard problems cluster around metadata. Metadata must be consistent across all nodes. Data can be more relaxed—you can tolerate slight inconsistency between replicas temporarily, as long as reads eventually see the latest write.

### Consistency Models

"Consistent" means different things depending on what guarantees you need:

- **Strong consistency**: Any read sees the most recently committed write, regardless of which node serves it. Expensive—requires coordination on every operation.
- **Eventual consistency**: All nodes converge to the same state eventually, but a read may temporarily return stale data. Much cheaper but requires your application to tolerate it.
- **Read-your-writes consistency**: You always see your own writes immediately, even if other clients may briefly see stale state.

Most distributed file systems targeting transactional workloads choose strong consistency for metadata and either strong or eventual consistency for data, depending on replication strategy.

### Replication

Replication means keeping copies of data on multiple nodes so that losing one node does not cause data loss. There are two broad strategies:

**Active replication** (synchronous): Every write is sent to all replicas before acknowledging success to the client. Guarantees no data loss but adds latency proportional to the slowest replica.

**Passive replication** (asynchronous): Writes are acknowledged after landing on the primary, then propagated to replicas in the background. Lower latency, but a primary failure before propagation means data loss.

### Sharding

Sharding distributes data across nodes rather than replicating it. A sharding scheme decides, for each file, which node is authoritative. A simple scheme is consistent hashing on the file path. A more sophisticated scheme considers node capacity, network topology, and load.

Sharding scales capacity and throughput linearly. The tradeoff is that a node failure makes its shard unavailable unless you also replicate within each shard.

### Consensus Algorithms

Consensus is the mechanism by which a cluster of nodes agrees on a single authoritative state despite failures and network partitions. For metadata—where you cannot afford divergence—consensus is non-negotiable.

The dominant algorithm in production systems today is **Raft**, introduced by Ongaro and Ousterhout in 2014. Raft is designed to be understandable without sacrificing correctness. It decomposes consensus into three relatively independent subproblems: leader election, log replication, and safety. We will see it in action in the practical section below.

---

## A Brief Survey of Existing Solutions

Understanding the landscape helps calibrate expectations.

**NFS (Network File System)**: NFS is not truly distributed. It is a shared filesystem protocol backed by a single server. It scales reads via caching, but all writes go to one machine. A common pattern is to put NFS on top of a SAN or NAS appliance to get redundant storage, but the NFS server itself remains a single point of metadata management. NFS is simple to deploy and widely supported, which makes it practical for many internal use cases, but it is not a distributed file system in the strict sense.

**GlusterFS**: An open-source distributed file system that distributes and replicates data across commodity hardware using a client-side translator architecture. It requires no central metadata server—metadata is derived from the file path using an elastic hash algorithm. This eliminates the metadata bottleneck but makes certain operations (like recursive directory listings) expensive, because the client must query all nodes. Suitable for large-scale object-like workloads.

**CephFS**: A POSIX-compliant distributed file system built on top of the Ceph RADOS object store. Metadata is handled by a dedicated cluster of Metadata Servers (MDS) that use dynamic subtree partitioning to distribute the namespace across MDS nodes. Data is stored in RADOS and striped across OSDs. CephFS provides strong consistency and full POSIX semantics, making it suitable for HPC and general-purpose storage, but it is operationally complex.

**HDFS (Hadoop Distributed File System)**: Designed for batch analytics, not low-latency interactive access. HDFS uses a single NameNode for all metadata, which creates a well-known bottleneck. Data blocks (typically 128 MB) are replicated three times across DataNodes. HDFS is optimized for sequential reads of large files—the "write once, read many" pattern. It is not suitable for general-purpose file serving.

**SeaweedFS**: A simpler distributed file system focused on ease of operation. A central Master server handles volume assignment, and Volume Servers store the actual data. SeaweedFS trades some flexibility for operational simplicity. It is a reasonable choice when you need distributed blob storage without the complexity of Ceph.

---

## Building a 3-Node Distributed File System with CallFS and Raft

CallFS is a lightweight REST API file server written in Go. When configured with the Raft metadata store, it turns into a distributed file system where metadata consistency is guaranteed by Raft consensus and file content is served directly from whichever node holds the data, with automatic cross-node proxying so clients never need to know which node owns which file.

The architecture looks like this:

```
           Client
             |
    ┌────────┼────────┐
    ▼        ▼        ▼
  node-1   node-2   node-3
  (leader) (follower)(follower)
    │        │        │
    └────────┴────────┘
         Raft cluster
       (metadata only)

  Each node stores its own files locally.
  Metadata is replicated to all nodes via Raft.
  Cross-node file requests are proxied internally.
```

### How CallFS Separates Metadata from Data

Metadata in CallFS lives in the Raft-replicated in-memory FSM (finite state machine). Every write—create, rename, delete—goes through Raft. Writes on followers are forwarded to the current leader, committed to the Raft log, and applied to the FSM on every node before returning success to the client.

The actual file bytes live on the local storage of whichever node received the upload (localfs or S3). The metadata record carries a `CallFSInstanceID` field that records which node owns the physical file. When a client requests a file from any node, that node looks up the metadata, checks the `CallFSInstanceID`, and either serves the file locally or proxies the request to the owning node transparently.

### Node 1 Configuration (Bootstrap Node)

The first node bootstraps the Raft cluster. Save this as `/etc/callfs/config.yaml` on node 1:

```yaml
server:
  listen_addr: ":8443"
  protocol: "https"
  external_url: "https://node1:8443"
  cert_file: "/etc/callfs/tls/node1.crt"
  key_file: "/etc/callfs/tls/node1.key"

auth:
  api_keys:
    - "your-api-key-here-at-least-16-chars"
  internal_proxy_secret: "shared-internal-secret-16chars"
  single_use_link_secret: "your-link-secret-here-16chars"

backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs/data"
  internal_proxy_skip_tls_verify: false

metadata_store:
  type: "raft"

raft:
  enabled: true
  node_id: "node-1"
  bind_addr: "10.0.0.1:7000"
  data_dir: "/var/lib/callfs/raft"
  bootstrap: true

instance_discovery:
  instance_id: "node-1"
  peer_endpoints:
    node-2: "https://node2:8443"
    node-3: "https://node3:8443"

dlm:
  type: "local"
```

Start node 1:

```bash
callfs server --config /etc/callfs/config.yaml
```

Node 1 will bootstrap a single-member Raft cluster and elect itself leader immediately.

### Node 2 and Node 3 Configurations

Node 2 and node 3 use nearly identical configuration. The key differences are the node ID, bind address, external URL, and `bootstrap: false`. Save this as `/etc/callfs/config.yaml` on node 2:

```yaml
server:
  listen_addr: ":8443"
  protocol: "https"
  external_url: "https://node2:8443"
  cert_file: "/etc/callfs/tls/node2.crt"
  key_file: "/etc/callfs/tls/node2.key"

auth:
  api_keys:
    - "your-api-key-here-at-least-16-chars"
  internal_proxy_secret: "shared-internal-secret-16chars"
  single_use_link_secret: "your-link-secret-here-16chars"

backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs/data"
  internal_proxy_skip_tls_verify: false

metadata_store:
  type: "raft"

raft:
  enabled: true
  node_id: "node-2"
  bind_addr: "10.0.0.2:7000"
  data_dir: "/var/lib/callfs/raft"
  bootstrap: false

instance_discovery:
  instance_id: "node-2"
  peer_endpoints:
    node-1: "https://node1:8443"
    node-3: "https://node3:8443"

dlm:
  type: "local"
```

### Joining Nodes to the Cluster

After starting node 2, tell it to join the cluster by pointing it at the leader:

```bash
callfs cluster join --leader https://node1:8443 --config /etc/callfs/config.yaml
```

The `join` command reads `raft.node_id`, `raft.bind_addr`, `server.external_url`, and `auth.internal_proxy_secret` from the config file automatically. It posts a join request to the leader's `/v1/internal/raft/join` endpoint, which calls `AddVoter` on the Raft instance. After a successful join, the leader immediately takes a snapshot and installs it on the new follower so it catches up with all committed metadata.

Repeat for node 3:

```bash
callfs cluster join --leader https://node1:8443 --config /etc/callfs/config.yaml
```

You now have a three-node Raft cluster with full metadata replication.

---

## How Raft Consensus Works Inside CallFS

### Leader Election

Raft uses a term-based election protocol. Every node starts as a follower. If a follower does not hear from a leader within an election timeout, it becomes a candidate, increments its term, and sends `RequestVote` RPCs to all peers. A node votes for a candidate if it has not voted in the current term and the candidate's log is at least as up-to-date as its own. A candidate that receives a majority of votes wins and becomes the new leader.

In a three-node cluster, a majority is two. The cluster can therefore tolerate the loss of one node and continue electing leaders. With five nodes, two can fail.

CallFS uses the `hashicorp/raft` library, which implements this election algorithm over TCP. The `bind_addr` in the Raft configuration is the TCP address used exclusively for Raft peer communication—it is separate from the HTTP API port.

### Log Replication

Every write operation in CallFS—creating a file record, updating metadata, deleting an entry—is serialized as a JSON command and submitted to Raft as a log entry. Only the leader can accept new entries. When a follower receives a write, it detects via `s.IsLeader()` that it is not the leader and forwards the command over HTTP to the leader's internal endpoint `/v1/internal/raft/metadata/apply`.

The leader appends the entry to its log and replicates it to all followers via `AppendEntries` RPCs. Once a majority of nodes acknowledge the entry, the leader commits it, applies it to the FSM, and returns the result to the caller. All followers also apply committed entries to their local FSM on the next heartbeat cycle.

This means metadata is always committed on at least two of three nodes before a write succeeds. A single node failure cannot cause metadata loss.

### Snapshots

The Raft log grows indefinitely if not compacted. Snapshots solve this. At a configurable interval (default: every 60 seconds or 256 new log entries), the leader serializes the entire FSM state—all file metadata, single-use links, and erasure coding information—into a snapshot file stored in the `data_dir`. Once the snapshot is persisted, log entries prior to the snapshot index are deleted.

When a new node joins the cluster and is significantly behind, the leader installs the latest snapshot on it directly rather than replaying thousands of individual log entries. CallFS forces a snapshot immediately after each `AddVoter` call to ensure new nodes can join cleanly even if the leader's log has been compacted.

---

## Transparent Cross-Server Routing

This is the feature that makes the cluster feel like a single system from the client's perspective.

When you upload a file to node 1, the file bytes are written to node 1's local storage (`/var/lib/callfs/data`). The metadata record is created with `CallFSInstanceID: "node-1"` and replicated via Raft to node 2 and node 3. Both follower nodes now know the file exists and know it lives on node 1.

When a client later downloads that file from node 2, the following happens:

1. Node 2 receives the GET request.
2. It looks up the metadata. Because the Raft FSM is replicated, node 2 has the metadata locally.
3. It checks `CallFSInstanceID`. The value is `"node-1"`, which is not the current instance.
4. Node 2 constructs a request to node 1's internal API using the `peer_endpoints` map.
5. Node 2 streams the response body from node 1 directly to the client.

The client receives the file without any knowledge that the bytes came from a different node. The HTTP response looks identical to a local serve.

The same logic applies to writes. If a client sends a PUT request to node 2 for a file that was originally created on node 1, node 2 detects the cross-instance ownership and proxies the PUT to node 1. Node 1 updates the bytes on disk. The metadata update goes through Raft to keep all nodes consistent.

Uploads, downloads, updates, and deletes all work transparently regardless of which node the client connects to. There is no client-side awareness of topology required.

```bash
# Upload to node 1
curl -X POST https://node1:8443/v1/files/reports/q1.csv \
  -H "Authorization: Bearer your-api-key" \
  --data-binary @q1.csv

# Download from node 2 — works transparently
curl https://node2:8443/v1/files/reports/q1.csv \
  -H "Authorization: Bearer your-api-key" \
  -o q1_from_node2.csv

# Download from node 3 — also works
curl https://node3:8443/v1/files/reports/q1.csv \
  -H "Authorization: Bearer your-api-key" \
  -o q1_from_node3.csv
```

All three commands return the same bytes.

---

## What You Get from This Architecture

**Fault tolerance for metadata**: With three nodes, one node can fail completely and the cluster continues serving reads and writes. Raft elects a new leader if the current one fails, typically within a few seconds.

**Capacity that grows with nodes**: Each node contributes its local storage. Files are distributed across nodes based on which node received the upload. To add capacity, add a node and join it to the cluster.

**No special client**: Clients use a standard HTTPS API. They do not need to know anything about Raft, node topology, or which node holds which file. Every node presents the same API surface.

**Audit-friendly metadata**: Because all metadata changes go through Raft, the Raft log is an append-only record of every create, update, and delete operation in order. This is useful for debugging and compliance.

---

## Limitations and When to Use Something Else

This architecture, like all architectures, has constraints worth understanding.

The Raft metadata store in CallFS keeps all metadata in memory, replicated across nodes. This works well for thousands to millions of files. For a namespace with hundreds of millions of entries, you would need a metadata store that pages to disk—CephFS with its MDS cluster, or a purpose-built metadata database.

File content is stored on whichever node received the upload. There is no automatic rebalancing or content replication between nodes. If node 1 fails, files that were stored only on node 1 are unavailable until it recovers. For content durability, you can configure CallFS to use an S3-compatible backend, which handles redundancy independently of the CallFS cluster.

If you need POSIX semantics—`fcntl` locking, `mmap`, hard links across nodes—none of the HTTP-API-based systems including CallFS will satisfy that requirement. CephFS with a kernel mount is the right tool for POSIX workloads.

---

## Summary

A distributed file system separates two concerns—metadata (where things are) and data (the bytes themselves)—and solves the hard problem of keeping metadata consistent across nodes. Consensus algorithms like Raft make that consistency possible without a single point of failure.

CallFS makes a pragmatic tradeoff: it uses Raft for metadata consensus and local (or S3) storage for file content, with transparent cross-node proxying so clients see a single unified namespace. The result is a system that is straightforward to operate, scales capacity by adding nodes, and tolerates node failures without manual intervention. For applications that currently rely on a single file server and are beginning to feel its limits, this is a concrete and operational path forward.
