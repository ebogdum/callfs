# How to Replicate Files Across Multiple Servers for Redundancy

File replication is not a nice-to-have. It is the difference between a brief incident and an unrecoverable data loss event. Hard drives fail without warning. Data centers lose power. Entire cloud regions go offline. Without replication, a single hardware fault can wipe out everything stored on that node — and no amount of uptime monitoring will bring those files back.

This guide covers the full spectrum of file replication strategies: from the classic approaches that most teams reach for first, through their real-world limitations, to application-level replication and erasure coding for environments that need genuine high availability.

---

## Why File Replication Matters

Three failure scenarios drive the need for file redundancy:

**Hardware failures.** Disk failure rates are well-documented — consumer drives average 1–5% annual failure rates, and enterprise drives are not immune. In a cluster of 50 nodes, you will almost certainly lose a disk this year. RAID protects against a disk loss within one machine, but not against the machine itself catching fire, flooding, or getting decommissioned.

**Data center outages.** Power failures, network partitions, and physical disasters take out entire racks or availability zones at once. Single-datacenter deployments have no answer to this class of failure.

**Disaster recovery.** Regulatory requirements in healthcare, finance, and e-commerce often mandate that data be recoverable from a geographically separate location with a defined recovery point objective (RPO). Replication is the mechanism that makes RPO achievable.

The goal of file replication is to ensure that if any single server — or group of servers — becomes unavailable, another copy of every file exists somewhere else and can be served immediately.

---

## Traditional Approaches and Their Limitations

### rsync Cron Jobs

The most common first step: a cron job runs `rsync` every hour (or every 15 minutes) to copy changed files from a primary server to one or more secondaries.

This works until it does not. The fundamental problem is the replication gap. If the primary fails at minute 59 of a 60-minute cycle, up to 59 minutes of writes are lost. Reducing the interval helps, but even a 1-minute sync window means potential data loss and significant I/O overhead from continuous directory comparisons on large file sets.

Other problems compound this:

- No consistency guarantees. A file can be partially written on the primary when rsync reads it, producing a corrupted replica.
- No split-brain protection. If the primary and secondary both believe they are authoritative, writes can diverge silently.
- Operational overhead. Monitoring whether rsync completed successfully requires additional tooling; silent failures are common.

### DRBD (Distributed Replicated Block Device)

DRBD operates at the block device layer, replicating every write synchronously or asynchronously to a secondary server. It is the closest you can get to RAID-1 across a network.

The downsides are significant in practice. DRBD is complex to configure and requires dedicated expertise to operate reliably. Split-brain recovery — the scenario where both nodes believe they are primary — requires manual intervention and can result in data loss if handled incorrectly. DRBD also does not help with geographic distribution: its latency sensitivity makes synchronous replication across regions impractical.

### GlusterFS

GlusterFS provides distributed or replicated volume semantics at the filesystem level. Replica volumes mirror data across bricks (storage units on different servers), giving you file-level redundancy.

The drawbacks are performance and complexity. GlusterFS adds a FUSE layer that introduces latency on every read and write. The distributed metadata model has historically suffered from split-brain scenarios, particularly during network partitions. Administering GlusterFS at scale — adding bricks, replacing failed nodes, rebalancing — is non-trivial and prone to operator error.

### Manual Replication Scripts

Custom scripts that call cloud SDKs or internal APIs to copy files on write are common in application code. The issue is that this logic must be maintained alongside the application, is difficult to make atomic, and typically has no mechanism to ensure that the replica write succeeded before acknowledging the client. Error handling is usually incomplete, leading to divergent state that is difficult to detect and repair.

---

## Application-Level Replication with Dual-Write

A cleaner architecture handles replication inside the storage layer itself, below the application. When replication is a first-class concern of the file storage service, it can be made consistent, observable, and configurable without burdening every application that writes files.

CallFS implements this as a dual-write HA feature. When replication is enabled, every file write goes to the primary backend first, then replicates to the secondary backend within the same request-response cycle. The calling application makes one API call and receives one response; the replication is handled internally.

### How the Dual-Write Works

The replication flow, from the source code, is:

1. The file is written to the primary backend (localfs or S3).
2. Metadata is created or updated in the metadata store.
3. The engine re-reads the file from the primary backend and writes it to the replica backend.
4. If `require_replica_success` is true and the replica write fails, the entire operation returns an error. If false, the failure is logged as a warning and the operation succeeds.

This means the replica is always written synchronously, in the same request. There is no replication lag and no background job to monitor.

### Configuration

```yaml
backend:
  default_backend: "localfs"
  localfs_root_path: "/var/lib/callfs"
  s3_bucket_name: "callfs-replica"
  s3_region: "us-east-1"
  s3_access_key: "YOUR_KEY"
  s3_secret_key: "YOUR_SECRET"

ha:
  replication_enabled: true
  replica_backend: "s3"
  require_replica_success: true  # fail the write if S3 is unreachable
```

With this configuration, every file written to local disk is simultaneously replicated to S3. If the S3 bucket is unreachable and `require_replica_success` is `true`, the write is rejected and the client receives an error. The primary never has data that the replica does not.

Setting `require_replica_success` to `false` gives you eventual consistency with best-effort replication — appropriate for scenarios where write availability matters more than strict consistency, and where you are comfortable with a brief replication gap during S3 outages.

### What This Solves

The dual-write approach eliminates the replication gap entirely. Every acknowledged write exists on both backends. If the primary server is lost, reads can be redirected to S3 with no data loss. If S3 becomes unavailable, the primary continues serving traffic (in the `require_replica_success: false` mode), and the replica catches up when S3 recovers.

This is also operationally simpler than rsync or DRBD. There is no separate process to monitor, no cron schedule to tune, and no split-brain scenario — one side is always the primary.

---

## Erasure Coding for Multi-Node Setups

Dual-write replication keeps two full copies of every file. For large files or large datasets, this doubles storage costs. Erasure coding is the mathematically efficient alternative: it breaks a file into shards such that a subset of shards is sufficient to reconstruct the original, tolerating a configurable number of node failures with less than 2x storage overhead.

### Reed-Solomon Erasure Coding

CallFS implements Reed-Solomon erasure coding. The default configuration uses 4 data shards and 2 parity shards (a 4+2 scheme). The file is split into 4 equal data shards, and 2 additional parity shards are computed. Any 4 of the 6 shards are sufficient to reconstruct the original file.

This means the cluster can lose any 2 nodes simultaneously — including the nodes holding parity shards — and every file remains fully recoverable. With 6 shards, storage overhead is 6/4 = 1.5x, compared to 2x for full replication.

Shards are distributed across nodes using a round-robin placement strategy. Each node stores a subset of shards for each erasure-coded file, so no single node is a bottleneck for either storage or retrieval.

### Enabling Erasure Coding

```yaml
erasure:
  enabled: true
  data_shards: 4
  parity_shards: 2
  min_file_size: 1048576  # 1 MB minimum — small files are stored normally
```

The `min_file_size` threshold is important. Erasure coding has fixed overhead per file (metadata, shard management, distributed I/O), so it is not cost-effective for small files. Files below the threshold are stored using the standard backend.

### Uploading an Erasure-Coded File

Pass the `X-CallFS-Erasure: true` header to request erasure coding for a specific upload:

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "X-CallFS-Erasure: true" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @large-file.bin \
  http://localhost:8443/v1/files/backups/large-file.bin
```

You can also override the shard counts per-request using `X-CallFS-Erasure-Data-Shards` and `X-CallFS-Erasure-Parity-Shards` headers, which is useful if different datasets have different durability requirements.

### How Retrieval Works

On read, CallFS fetches all available shards in parallel across the cluster, cancelling remaining fetches as soon as it has collected the minimum number of data shards required for reconstruction. If a node is down, the missing shards are reconstructed from the parity shards using Reed-Solomon decoding. The client receives the complete file with no awareness of the underlying shard distribution.

Shard checksums are verified on every read. A shard that has been silently corrupted on disk is detected and the correct data is reconstructed from the remaining shards.

---

## Raft Consensus for Metadata Replication

File data replication solves one half of the problem. The other half is metadata: which files exist, where their data lives, their sizes, modification times, and ownership. In a multi-node cluster, metadata must also be replicated consistently — otherwise, a node can serve stale directory listings or fail to locate files that were created on a different node.

CallFS addresses this with a Raft consensus-based metadata store. Raft is a distributed consensus algorithm that ensures all nodes agree on the state of the metadata store. Writes are applied to a leader, which replicates the log entry to a quorum of followers before acknowledging the write. Reads can be served from any node.

### Configuration

```yaml
metadata_store:
  type: "raft"

raft:
  enabled: true
  node_id: "callfs-node-1"
  bind_addr: "10.0.0.1:7000"
  data_dir: "/var/lib/callfs/raft"
  bootstrap: true  # set to true only on the initial leader
  peers:
    callfs-node-2: "10.0.0.2:7000"
    callfs-node-3: "10.0.0.3:7000"
  api_peer_endpoints:
    callfs-node-2: "https://10.0.0.2:8443"
    callfs-node-3: "https://10.0.0.3:8443"
  apply_timeout: "10s"
  forward_timeout: "10s"
  snapshot_interval: "60s"
  snapshot_threshold: 256
  retain_snapshot_count: 2
```

The Raft store handles leader election automatically. If the leader node fails, a new leader is elected from the remaining nodes within seconds (depending on the election timeout configuration), and the cluster continues serving requests. Writes directed to a non-leader node are automatically forwarded to the current leader.

### Why Metadata Consistency Matters

Without consistent metadata replication, you can encounter scenarios like:

- A file is created on node 1. Node 2's directory listing does not include it yet.
- A file is deleted on node 1. Node 2 still serves it from its local metadata cache.
- Two clients write the same path on different nodes simultaneously, resulting in two divergent copies.

Raft eliminates these by making all metadata mutations go through a single ordered log that every node applies in the same sequence. There is no eventual consistency window for metadata.

---

## Choosing the Right Strategy

The approaches covered here are not mutually exclusive. A production setup might combine all of them:

| Layer | Mechanism | What It Protects Against |
|---|---|---|
| File data | Dual-write (localfs + S3) | Single server failure, regional availability |
| Large files | Erasure coding (4+2) | Up to 2 simultaneous node failures |
| Metadata | Raft consensus | Metadata loss, split-brain, stale reads |

For a small deployment with a handful of servers and moderate file sizes, dual-write replication with `require_replica_success: true` gives strong durability guarantees with minimal operational complexity. S3 provides eleven nines of object durability, which exceeds what most self-hosted replication schemes can offer.

For larger clusters with terabytes of data, erasure coding becomes attractive because it cuts storage costs by 25% compared to full replication while maintaining configurable fault tolerance. The trade-off is slightly higher read latency (parallel shard fetches across the network) and the requirement that all nodes in the erasure group are reachable for writes.

For any multi-node setup, Raft metadata replication is not optional. File data consistency without metadata consistency is meaningless — a perfectly replicated file that the metadata store does not know about cannot be served to clients.

---

## Summary

File replication is a layered problem. Traditional tools like rsync introduce replication gaps and operational complexity. DRBD and GlusterFS provide stronger guarantees but at the cost of significant administrative overhead and failure modes that are difficult to manage in production.

Application-level replication, implemented correctly inside the storage layer, gives you synchronous dual-write semantics without exposing that complexity to the applications writing files. Combined with erasure coding for storage efficiency and Raft for metadata consistency, it is possible to build a file storage system that tolerates multiple simultaneous node failures without data loss and without manual intervention.

The key metrics to track once replication is in place: replication lag (should be zero for synchronous dual-write), shard availability per file (should remain above the data shard threshold at all times), and Raft leader election events (unexpected elections indicate network instability that warrants investigation).
