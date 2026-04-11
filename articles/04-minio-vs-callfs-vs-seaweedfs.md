# MinIO vs CallFS vs SeaweedFS: Choosing the Right Self-Hosted File Storage

The self-hosted storage landscape has matured considerably. Where teams once defaulted to NFS mounts or a hand-rolled file server, there are now purpose-built distributed storage systems that cover most use cases without forcing a cloud dependency. MinIO, SeaweedFS, and CallFS each occupy a distinct position in that landscape -- and picking the wrong one for your workload will cost you.

This article compares the three systems honestly, including where each one falls short. There is no single winner; the right choice depends on your API requirements, operational constraints, and the nature of your files.

---

## What Each System Is

**MinIO** is an S3-compatible object store. Its primary design goal is API compatibility with Amazon S3, which means any application that already speaks the S3 protocol can point at MinIO without code changes. It is the most widely deployed self-hosted object store and has a large ecosystem of tooling built around it.

**SeaweedFS** is a distributed blob store optimized for storing billions of small files efficiently. It uses a master/volume server architecture borrowed from concepts in Facebook's Haystack paper. It has its own HTTP API and an S3-compatible gateway layer on top.

**CallFS** is a REST API filesystem with pluggable backends. Rather than emulating S3, it exposes a straightforward HTTP API with filesystem semantics -- paths, directories, and files -- that any HTTP client can use without an SDK. It can use local disk or S3-compatible storage (including MinIO) as its backend, and it supports Raft-based clustering with erasure coding for fault tolerance.

---

## Architecture

### MinIO

MinIO runs as a single process per node and uses erasure coding internally to distribute data and parity blocks across drives (or nodes in distributed mode). Metadata is stored inline with the data in a proprietary format on disk. For large-scale deployments, MinIO recommends its own distributed mode with multiple nodes in an erasure set, which requires careful upfront capacity planning since the number of drives per erasure set is fixed at deployment time.

MinIO's strength is its fidelity to the S3 API. Buckets, objects, multipart uploads, presigned URLs, ACLs, and object versioning all behave as they do in AWS S3. This makes it a drop-in replacement in S3-aware stacks.

### SeaweedFS

SeaweedFS separates concerns into master servers and volume servers. Master servers handle topology, volume assignment, and leader election. Volume servers hold the actual data in append-only "needle" files that pack many blobs into a single volume file, which is how it achieves efficiency at scale with billions of small objects. A filer layer adds directory semantics and an S3 gateway on top of the core store.

The architecture trades operational simplicity for raw throughput at scale. Running a production SeaweedFS cluster means operating at least one master (ideally three for HA), multiple volume servers, and optionally filer and S3 gateway processes.

### CallFS

CallFS runs as a single binary. Its architecture separates the API layer from the storage backend and the metadata store, which are independently configurable. Storage can be local filesystem, any S3-compatible endpoint (including MinIO), or both via HA replication. Metadata can be SQLite (for single-node simplicity), PostgreSQL, Redis, or a built-in Raft store for fully self-contained clusters.

Clustering is handled via Raft consensus for metadata consistency and optional Reed-Solomon erasure coding for data durability across nodes. Cross-node file operations are routed transparently -- clients always talk to any node and CallFS handles proxying internally.

---

## Setup Complexity

| System | Minimum Setup | Production Setup |
|--------|---------------|------------------|
| MinIO | Single binary, local disk | Multiple nodes, fixed erasure set size, external monitoring |
| SeaweedFS | Master + volume server | 3 masters + N volume servers + filer + S3 gateway |
| CallFS | Single binary + SQLite | Binary + Postgres/Raft + Redis (optional) |

**MinIO** has a smooth single-node startup but becomes more complex in distributed mode. The erasure set size must match your drive count at deploy time, and changing it later requires a migration.

**SeaweedFS** has the highest operational surface area. Each component (master, volume, filer, S3 gateway) is a separate process with its own configuration, ports, and failure modes. This is a reasonable trade-off if you need its specific strengths, but it is not a lightweight deployment.

**CallFS** is the simplest to get started with and scales to a multi-node cluster without introducing separate process types. The same binary serves all roles. The only external dependency for a clustered deploy is either a shared metadata store or the built-in Raft mode, which needs no external services at all.

### Getting Started: MinIO

```bash
# Download and run (single-node)
wget https://dl.min.io/server/minio/release/linux-amd64/minio
chmod +x minio
MINIO_ROOT_USER=admin MINIO_ROOT_PASSWORD=password ./minio server /data

# Upload via AWS CLI
aws --endpoint-url http://localhost:9000 s3 cp file.txt s3://my-bucket/file.txt
```

### Getting Started: SeaweedFS

```bash
# Start master server
weed master -port=9333

# Start volume server (separate terminal)
weed volume -mserver=localhost:9333 -port=8080 -dir=/data

# Upload via HTTP API
curl -F "file=@file.txt" http://localhost:9333/dir/assign
# Then PUT to the returned volume server URL
```

### Getting Started: CallFS

```bash
# Download binary or build from source
go build -o callfs ./cmd/main.go

# Minimal config (SQLite, no TLS, local disk)
cp config.yaml.example config.yaml
# Edit: set listen_addr, api_keys, localfs_root_path, metadata_store.type=sqlite

./callfs server --config config.yaml

# Upload with plain curl -- no SDK required
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @file.txt \
  http://localhost:8443/v1/files/data/file.txt
```

---

## API Style

This is where the three systems diverge most sharply in day-to-day developer experience.

**MinIO** requires the S3 protocol. Practically, this means using an S3 SDK (Boto3, aws-sdk-go, aws-sdk-js, etc.) or the AWS CLI. The S3 protocol involves request signing (SigV4), which is non-trivial to implement manually. If your team is already in the AWS ecosystem, this is a non-issue. If you are not, it is real friction.

**SeaweedFS** uses a custom HTTP API for its core store (assign a file ID, then PUT to the volume server URL). The S3-compatible gateway allows standard S3 clients, but the gateway is an optional component that adds latency and another process to operate.

**CallFS** uses plain HTTP REST. Upload is a POST or PUT with the file as the request body. Download is a GET. List is a GET on a directory path. There are no request signatures, no SDK dependencies, and no special client libraries. Any language with an HTTP client -- or just curl -- works directly.

```bash
# Upload
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @file.txt \
  http://localhost:8443/v1/files/data/file.txt

# Download
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/files/data/file.txt

# List directory
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8443/v1/directories/data/
```

The simplicity has a trade-off: CallFS does not implement the S3 API. Applications built around S3 SDK calls cannot point at CallFS without code changes.

---

## Storage Backends

**MinIO** manages its own storage. Data lives in MinIO's internal format on local disk. It does not delegate to other storage systems.

**SeaweedFS** also manages its own storage in volume files on local disk (or cloud storage with appropriate filer plugins).

**CallFS** separates the API from the storage backend. You can configure it to use:

- Local filesystem (default, zero dependencies)
- Any S3-compatible object store, including AWS S3, MinIO, DigitalOcean Spaces, or Cloudflare R2
- HA mode with replication across two backends simultaneously

This means CallFS can sit in front of MinIO, adding filesystem semantics, directory listings, owner-based access control, single-use download links, and Raft-coordinated clustering -- while MinIO handles the durable object storage underneath.

---

## Clustering and Data Durability

**MinIO** uses Reed-Solomon erasure coding within a distributed erasure set. Data and parity blocks are distributed across drives in the set. The erasure set size (number of drives) is fixed at cluster creation. Node failures are tolerated up to the parity count.

**SeaweedFS** assigns replication factors per volume. Volumes can be replicated across racks or data centers. The master handles topology and re-replication when volume servers fail.

**CallFS** offers two complementary mechanisms:

1. **Raft consensus** for metadata: all metadata operations go through a Raft log, giving strong consistency across nodes. Any node can serve reads and writes; the leader handles commits.
2. **Reed-Solomon erasure coding** for data: files above a configurable size threshold are sharded across nodes with configurable data and parity counts (e.g., 4 data + 2 parity shards, tolerating 2 node failures).

For lighter deployments, HA replication (synchronous or asynchronous) between two backends provides a simpler durability story without erasure coding overhead.

---

## Licensing

**MinIO** changed its license in 2021 from Apache 2.0 to AGPL-3.0. AGPL requires that if you run MinIO as part of a service you offer to others, you must publish the source code of that service. For internal use this is generally not a constraint, but for SaaS products it may be. MinIO also offers a commercial license.

**SeaweedFS** is licensed under Apache 2.0. No restrictions beyond attribution.

**CallFS** is licensed under MIT. The most permissive of the three -- no copyleft obligations, no attribution requirement in binaries.

---

## Comparison Table

| Dimension | MinIO | SeaweedFS | CallFS |
|-----------|-------|-----------|--------|
| Primary API | S3-compatible | Custom HTTP + S3 gateway | Plain REST (no SDK) |
| Architecture | Single process, own storage | Master + volume servers | Single binary, pluggable backends |
| Metadata | Inline with data | Separate filer process | SQLite / Postgres / Redis / Raft |
| Clustering | Erasure sets (fixed size) | Replicated volumes | Raft + erasure coding |
| Storage backends | Own disk only | Own disk (+ cloud filer plugins) | Local disk + any S3-compatible store |
| Minimum viable deploy | 1 binary + disk | 2 processes + disk | 1 binary + disk |
| Client requirement | S3 SDK or AWS CLI | Custom client or S3 SDK | Any HTTP client / curl |
| Small-file optimization | Moderate | Excellent (Haystack-style) | Moderate |
| License | AGPL-3.0 | Apache 2.0 | MIT |
| Best for | S3-compatible workloads | Billions of small files | REST file management, mixed backends |

---

## When to Choose Each

### Choose MinIO when:

- Your application already uses an S3 SDK and you want zero code changes
- You need full S3 API compatibility (multipart upload, versioning, object ACLs, presigned URLs)
- Your team has existing expertise with S3 tooling and IAM-style access control
- You are migrating from AWS S3 to self-hosted and need a transparent drop-in

### Choose SeaweedFS when:

- You are storing hundreds of millions to billions of small files (images, thumbnails, logs)
- Raw storage efficiency at massive scale is the primary requirement
- You can invest in operating a multi-process cluster
- You need fine-grained volume-level replication control

### Choose CallFS when:

- You want a simple REST API that any developer can use without SDK knowledge
- You need filesystem semantics (directories, paths) rather than flat object namespaces
- You want to use MinIO or S3 as the durable storage backend while adding a richer API layer
- You prefer a single binary with minimal operational overhead
- Your team values MIT licensing without copyleft constraints
- You need owner-based access control, single-use expiring download links, or WebSocket streaming

---

## Honest Caveats

CallFS does not implement the S3 API. If your entire stack is built around S3 clients, MinIO is the better choice -- or you use MinIO as a CallFS backend and keep your S3 tooling for direct bucket access.

SeaweedFS has years of production hardening at scale that CallFS does not yet have. For workloads at the billions-of-objects scale, SeaweedFS is the more battle-tested option.

MinIO's AGPL licensing may be a deal-breaker for commercial SaaS products. Evaluate carefully if you are building a service on top of it.

---

## Summary

MinIO wins when S3 compatibility is non-negotiable. SeaweedFS wins when scale and small-file density are the primary constraints. CallFS wins when simplicity, plain HTTP access, and flexibility in storage backends matter more than S3 API fidelity.

All three are genuinely useful tools. The decision comes down to what your application already speaks, how much operational complexity you are willing to accept, and whether the S3 API is a feature or a constraint for your team.

**CallFS** is available at [github.com/ebogdum/callfs](https://github.com/ebogdum/callfs) under the MIT license.
