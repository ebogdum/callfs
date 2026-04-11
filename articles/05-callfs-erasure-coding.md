# Erasure Coding in CallFS: Protect Your Files Across Multiple Servers

Storing files on a single server is a single point of failure. When that server's disk fails, your data is gone. The traditional answer is full replication — keep a complete copy on a second server — but that doubles your storage bill for every byte you want to protect.

CallFS includes Reed-Solomon erasure coding as a first-class feature. Instead of storing full copies, erasure coding splits each file into a set of mathematical fragments that are distributed across your server cluster. A subset of those fragments is enough to reconstruct the original file, which means you can survive multiple simultaneous server failures while storing significantly less data than full replication requires.

This tutorial explains how erasure coding works in CallFS, how to configure it, and how to use it day-to-day from the command line.

---

## How Erasure Coding Works

Reed-Solomon erasure coding divides a file into two categories of fragments: data shards and parity shards.

- **Data shards** hold the actual file content, split into equal-sized pieces.
- **Parity shards** hold mathematically derived redundancy information computed from the data shards.

The key property: given any N data shards worth of fragments — from any combination of data or parity shards — the original file can be fully reconstructed. Parity shards are interchangeable with data shards during reconstruction.

CallFS defaults to a **4+2 profile**: four data shards and two parity shards, for six total fragments distributed across six nodes. Any four of those six fragments are sufficient to rebuild the file. This means you can lose any two servers simultaneously and continue operating without data loss — the same fault tolerance as keeping two full copies, at 1.5x storage overhead instead of 2x.

When you upload a file with erasure coding enabled, CallFS:

1. Reads the complete file into memory.
2. Splits it into four equal data shards.
3. Computes two parity shards using Reed-Solomon arithmetic.
4. Assigns one shard per node using round-robin placement.
5. Writes each shard to its assigned node in parallel, using the internal shard API over HTTPS.
6. Records shard locations, sizes, and SHA-256 checksums in the erasure metadata store.
7. Creates a file metadata record marking the file as `erasure_coded: true`.

When you download the file, CallFS fetches shards from all available nodes in parallel, verifies each shard's checksum, and reconstructs the original content before streaming it to the client. If a node is unreachable, the missing shard is treated as absent and reconstruction proceeds from whichever shards arrive first. As soon as four valid shards are received, CallFS cancels any remaining in-flight fetches and decodes immediately.

---

## Minimum File Size

Erasure coding introduces per-shard overhead and is designed for files large enough to benefit from distributed storage. CallFS skips erasure coding for files below the configured minimum size and stores them as regular files instead. The default threshold is 1 MB.

---

## Configuration

Add an `erasure` block to your `config.yaml`:

```yaml
erasure:
  enabled: true
  data_shards: 4
  parity_shards: 2
  min_file_size: 1048576  # 1MB minimum — small files skip erasure coding
  shard_backend: "localfs"  # or "s3"
```

| Field | Default | Description |
|---|---|---|
| `enabled` | `false` | Master switch for erasure coding. |
| `data_shards` | `4` | Number of data fragments. Must be >= 2. |
| `parity_shards` | `2` | Number of parity fragments. Must be >= 1. |
| `min_file_size` | `1048576` | Files smaller than this are stored without erasure coding. |
| `shard_backend` | `"localfs"` | Storage backend for shards on each node: `localfs` or `s3`. |

The total shard count (`data_shards + parity_shards`) must not exceed 256. The validation is enforced at startup — CallFS will refuse to start with an invalid erasure configuration.

---

## Multi-Node Setup

Erasure coding requires enough nodes to hold all shards. With the default 4+2 profile you need at least six reachable nodes. CallFS discovers peers through the `instance_discovery` block in each node's configuration.

Each node in the cluster needs its own `config.yaml` with a unique `instance_id` and the full list of peer endpoints:

```yaml
instance_discovery:
  instance_id: "node-1"
  peer_endpoints:
    node-1: "https://node1.internal:8443"
    node-2: "https://node2.internal:8443"
    node-3: "https://node3.internal:8443"
    node-4: "https://node4.internal:8443"
    node-5: "https://node5.internal:8443"
    node-6: "https://node6.internal:8443"
```

Repeat this block on every node, changing only `instance_id` to match the key for that node. All nodes must agree on the full peer map — each node uses the peer endpoints to push and pull shards over the internal HTTP API.

Internal shard traffic is authenticated with a shared secret configured in `auth.internal_proxy_secret`. Use HTTPS endpoints for all peers; CallFS will log a warning if any peer endpoint uses plain HTTP, because the internal token would be transmitted in cleartext.

---

## Uploading with Erasure Coding

To store a file using erasure coding, include the `X-CallFS-Erasure: true` header in your POST request:

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "X-CallFS-Erasure: true" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @large-file.bin \
  https://node1.internal:8443/v1/files/backups/large-file.bin
```

Alternatively, use the `erasure=true` query parameter if you cannot set custom headers:

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @large-file.bin \
  "https://node1.internal:8443/v1/files/backups/large-file.bin?erasure=true"
```

A successful upload returns HTTP `201 Created` with no body.

### Overriding Shard Counts Per Upload

You can override the cluster-wide shard configuration for a specific upload using the `X-CallFS-Erasure-Data-Shards` and `X-CallFS-Erasure-Parity-Shards` headers:

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "X-CallFS-Erasure: true" \
  -H "X-CallFS-Erasure-Data-Shards: 6" \
  -H "X-CallFS-Erasure-Parity-Shards: 3" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @critical-archive.bin \
  https://node1.internal:8443/v1/files/backups/critical-archive.bin
```

This 6+3 profile creates nine total shards and can survive any three simultaneous node failures. The per-upload headers take precedence over the configuration file values. The same overrides are available as query parameters (`data_shards`, `parity_shards`) if needed.

---

## Downloading

Downloading an erasure-coded file requires no special flags. Use a standard GET request:

```bash
curl -X GET \
  -H "Authorization: Bearer YOUR_API_KEY" \
  https://node1.internal:8443/v1/files/backups/large-file.bin \
  -o large-file.bin
```

CallFS reads the file's metadata, sees that it is erasure-coded, and automatically triggers the shard retrieval and reconstruction process. The response is the original file content streamed as `application/octet-stream` — identical to what you uploaded, with no client-side assembly required.

If one or more nodes are offline at download time, CallFS will reconstruct from whichever shards remain available, as long as at least N data shards' worth of fragments are reachable (four out of six in the default profile).

---

## Checking Shard Status

To inspect the erasure metadata for a file — which shards exist, where they live, their sizes, and their checksums — append `?manifest=true` to any GET request on an erasure-coded file:

```bash
curl -X GET \
  -H "Authorization: Bearer YOUR_API_KEY" \
  "https://node1.internal:8443/v1/files/backups/large-file.bin?manifest=true"
```

The response is a JSON manifest:

```json
{
  "path": "/backups/large-file.bin",
  "original_size": 104857600,
  "erasure_profile": {
    "data_shards": 4,
    "parity_shards": 2,
    "shard_size": 26214400
  },
  "shards": [
    {
      "index": 0,
      "endpoint": "https://node1.internal:8443/v1/shards/backups/large-file.bin/0",
      "size": 26214400,
      "checksum": "a3f5..."
    },
    {
      "index": 1,
      "endpoint": "https://node2.internal:8443/v1/shards/backups/large-file.bin/1",
      "size": 26214400,
      "checksum": "7c21..."
    }
  ]
}
```

Each shard entry includes the direct endpoint URL on the node that holds it. This manifest lets you audit placement, verify checksums independently, or build custom reconstruction tooling without going through the standard download path.

---

## Storage Efficiency

The primary motivation for erasure coding over full replication is storage overhead. Here is a concrete comparison for a 100 GB file with the same fault tolerance (survive two simultaneous failures):

| Strategy | Copies / Shards | Storage Used | Nodes Required |
|---|---|---|---|
| Full replication (3 copies) | 3 full copies | 300 GB | 3 |
| Erasure coding 4+2 | 6 shards at 25 GB each | 150 GB | 6 |

Both strategies survive two simultaneous server failures. Erasure coding uses half the storage of three-way replication for the same file.

The overhead ratio for erasure coding is `(data_shards + parity_shards) / data_shards`. For 4+2 that is 6/4 = 1.5x. For 6+2 it drops to 8/6 = 1.33x — more nodes required, but even less wasted storage.

Note that erasure coding distributes shards rather than full copies, so read throughput can benefit from parallelism across nodes. Reconstruction requires at least N shards, so reads are not bottlenecked by a single node's disk or network capacity.

---

## When to Use Erasure Coding vs. HA Replication

CallFS also supports high-availability replication (`ha.replication_enabled`), which pushes full file copies to replica nodes. The two approaches serve different use cases:

**Use erasure coding when:**

- Files are large (several hundred MB to multiple GB). The storage savings become significant at scale.
- You have six or more nodes available to distribute shards across.
- You can tolerate a small latency increase on uploads (all shards must be written before the request completes) and on downloads when nodes are degraded (reconstruction takes slightly more time than a direct read).
- Storage cost is a primary concern and you want maximum efficiency per byte of fault tolerance.

**Use HA replication when:**

- Files are small or access latency is your top priority. Reading from a local replica is always faster than reconstructing from shards.
- Your cluster has fewer nodes than the total shard count requires.
- You need the simplest possible failure model: a replica is a complete, immediately readable copy.
- You are storing files that fall below the `min_file_size` threshold, where erasure coding is automatically bypassed anyway.

The two features are not mutually exclusive at the cluster level — you can configure erasure coding for large files and standard storage for small files, using the `min_file_size` threshold to control the boundary automatically.

---

## Summary

Erasure coding in CallFS gives you enterprise-grade data redundancy at 1.5x storage overhead instead of the 2x or 3x cost of full replication. You enable it in `config.yaml`, point your cluster nodes at each other via `instance_discovery.peer_endpoints`, and then trigger it per-upload with a single HTTP header or query parameter. Downloads are completely transparent — CallFS handles reconstruction automatically, including partial-failure scenarios where some nodes are offline.

For large files in multi-node deployments, erasure coding is the most storage-efficient path to surviving server failures without storing unnecessary full copies of your data.
