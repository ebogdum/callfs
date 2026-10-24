# Erasure Coding in CallFS: Protect Your Files Across Multiple Servers

Storing files on a single server is a single point of failure. When that server's disk fails, your data is gone. The traditional answer is full replication (keep a complete copy on a second server), but that doubles your storage bill for every byte you want to protect.

CallFS includes Reed-Solomon erasure coding as a first-class feature. In the console and API it is called split storage. Instead of storing full copies, CallFS splits a file into a set of mathematical pieces that are spread across your servers. A subset of those pieces is enough to rebuild the original file, which means you can survive several simultaneous server failures while storing significantly less data than full replication requires.

This tutorial explains how erasure coding works in CallFS, how to configure it, and how to use it day to day from the command line and from the web console.

---

## How Erasure Coding Works

Reed-Solomon erasure coding divides a file into two kinds of pieces, called shards in the settings and API: data shards and parity shards.

- **Data shards** hold the file content, split into equal-sized pieces.
- **Parity shards** (spare pieces in the console) hold redundancy information computed from the data shards.

The key property: from any `data_shards` pieces, whether data or parity, the original file can be rebuilt. Parity pieces are interchangeable with data pieces during reconstruction.

CallFS defaults to a **4+2 profile**: four data shards and two parity shards, six pieces in total. Any four of those six are enough to rebuild the file, so you can lose any two pieces (for example, two servers) and keep reading it. That is the fault tolerance of keeping three full copies, at 1.5x storage overhead instead of 3x.

When you upload a file with erasure coding, CallFS:

1. Receives the upload and splits it into the configured number of data shards.
2. Computes the parity shards.
3. Assigns the pieces to servers, one after the other in a rotation, and writes them all in parallel.
4. Records where every piece lives, its size and its SHA-256 checksum.
5. Marks the file as erasure coded.

When you download the file, CallFS reads pieces from the servers that hold them, checks each against its checksum, and streams the rebuilt content to you. If a server is unreachable, or a piece is missing or damaged, CallFS uses the other pieces instead. Downloads work as long as at least `data_shards` good pieces remain.

---

## Which Files Are Split

Turning erasure coding on in the configuration makes it available. A new file is split when the upload asks for it (see "Uploading with Erasure Coding" below) or when its size reaches `erasure.min_file_size`. With `erasure.min_file_size: 0` (the default), only uploads that ask are split. Other files keep going to the normal storage. Files saved before you enabled the feature are not changed, and changing the shard counts later affects new files only.

Limits for split uploads:

- The file must not be empty. A zero-byte upload returns `400`.
- The maximum size is 1 GiB per upload.
- A split file cannot be overwritten in place with `PUT`; the request returns `412 Precondition Failed`. Delete it and upload it again to replace it.
- A split file cannot be moved to another storage, and a folder cannot be moved to another storage or server (`400`).

---

## Configuration

Add an `erasure` block to your `config.yaml`:

```yaml
erasure:
  enabled: true
  data_shards: 4
  parity_shards: 2
  shard_backend: "localfs"
  min_file_size: 104857600
  shard_path: ".callfs/shards"
```

| Key | Default | Allowed | What it does |
|---|---|---|---|
| `erasure.enabled` | `false` | `true`, `false` | Turns split storage on. |
| `erasure.data_shards` | `4` | 2 or more | Pieces that hold the file data. The console accepts up to 32. |
| `erasure.parity_shards` | `2` | 1 or more | Spare pieces. A file survives the loss of this many pieces. The console accepts up to 32. |
| `erasure.shard_backend` | `localfs` | `localfs`, `s3` | Where each server keeps its pieces: its own disk, or the S3 bucket. |
| `erasure.min_file_size` | `0` | 0 or more bytes (for example `100MiB` in the console) | New files at or above this size are split automatically, without any header. `0` means only uploads that ask are split. Automatic splitting covers `POST`, `PUT` of a new file and resumable uploads up to the 1 GiB split limit. Send `X-CallFS-Erasure: false` (or `erasure=false`) to keep one upload out of it. |
| `erasure.shard_path` | `.callfs/shards` | A clean relative folder name | Folder inside each storage where pieces are kept. |

All six need a restart to change. The total `data_shards + parity_shards` must not exceed 256, and CallFS refuses to start with an invalid erasure configuration, printing a message such as `erasure.data_shards must be >= 2 (got 1)`. These settings are also in the console under Settings, "Storage".

Pieces are stored under the `erasure.shard_path` folder (default `.callfs/shards`) in the shard storage, one folder per file and one file per piece (`<shard_path>/<id>/<index>`). A value that is absolute, contains `..` or is not clean is refused at startup with `erasure.shard_path must be a clean relative path inside the storage`. Leave these folders alone and include them in your backups. Metadata about the pieces lives in the metadata store.

Split storage works with every metadata store (SQLite, PostgreSQL, Redis and the embedded NATS store).

---

## Multi-Node Setup

Pieces can sit on one server or be spread over many. With the default 4+2 profile, the best protection comes from six servers, one piece each. If you have fewer servers, CallFS still works and places more than one piece on some of them; it just protects against fewer server failures (see "Planning a Layout" below). With no other servers configured, every piece stays on the same server, which protects against damaged or missing pieces but not against losing the server.

Each server needs a unique `instance_id`, the full list of peer addresses, its own public address, and the same shared secret. Server `node-1` looks like this:

```yaml
server:
  external_url: "https://node-1.example.com:8443"

auth:
  internal_proxy_secret: "REPLACE_WITH_SHARED_SECRET"

instance_discovery:
  instance_id: "node-1"
  peer_endpoints:
    node-1: "https://node-1.example.com:8443"
    node-2: "https://node-2.example.com:8443"
    node-3: "https://node-3.example.com:8443"
    node-4: "https://node-4.example.com:8443"
    node-5: "https://node-5.example.com:8443"
    node-6: "https://node-6.example.com:8443"
```

Repeat the block on every server, changing only `instance_id` and `external_url`. Rules to follow:

- Every server must list the same peers with the same IDs. Servers use these addresses to send and fetch pieces.
- Use `https://` addresses. A peer address with plain `http://` is rejected at startup, except for loopback addresses such as `127.0.0.1`, because the shared secret travels with every request.
- `auth.internal_proxy_secret` must be identical on every server. It authenticates piece traffic between servers.
- `server.external_url` must include the scheme and is how a server describes itself in a manifest.

Servers that share files, metadata and locks also need the cluster settings (`nats.enabled`, `metadata_store.type: nats`, `dlm.type: nats`); the steps are in [Clustering](../docs_markdown/07-clustering.md). The console shows your servers under Cluster, "Servers" and their reachability.

---

## Uploading with Erasure Coding

To store a file with erasure coding, create it with a `POST` and include the `X-CallFS-Erasure: true` header:

```bash
curl -X POST \
  -H "Authorization: Bearer your-api-key" \
  -H "X-CallFS-Erasure: true" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @large-file.bin \
  https://node-1.example.com:8443/v1/files/backups/large-file.bin
```

Send `X-CallFS-Erasure: false` to store one upload normally even when `erasure.min_file_size` would split it. If you cannot set custom headers, use the `erasure=true` query parameter:

```bash
curl -X POST \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @large-file.bin \
  "https://node-1.example.com:8443/v1/files/backups/large-file.bin?erasure=true"
```

A successful upload returns `201 Created` and the file's `ETag`, with no body. The answers you may see instead:

| Status | When |
|---|---|
| `501 Not Implemented` | Erasure coding is not enabled on this server. The same applies to `PUT` with `X-CallFS-Erasure: true`. |
| `409 Conflict` | A file or folder already exists at the path. |
| `400 Bad Request` | The upload is empty, a shard count is not a whole number of at least 1, the counts are more than 255 together, or more than 64 servers are named. |
| `413 Payload Too Large` | More than 1 GiB. |
| `500` | A piece could not be written (the upload is rolled back and the pieces already written are removed). |

### Overriding Shard Counts and Servers Per Upload

You can override the configured profile for one upload with the `X-CallFS-Erasure-Data-Shards` and `X-CallFS-Erasure-Parity-Shards` headers, and choose the servers with `X-CallFS-Erasure-Instances`:

```bash
curl -X POST \
  -H "Authorization: Bearer your-api-key" \
  -H "X-CallFS-Erasure: true" \
  -H "X-CallFS-Erasure-Data-Shards: 6" \
  -H "X-CallFS-Erasure-Parity-Shards: 3" \
  -H "X-CallFS-Erasure-Instances: node-2,node-3,node-4,node-5,node-6" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @critical-archive.bin \
  https://node-1.example.com:8443/v1/files/backups/critical-archive.bin
```

This 6+3 profile creates nine pieces and survives the loss of any three. The same overrides exist as query parameters: `data_shards`, `parity_shards` and `instances`. Header values win over query values, and values you leave out use the configured defaults.

Placement works like this: pieces are handed out in turn, starting with the server that receives the upload (it always keeps piece 0), then the servers from `X-CallFS-Erasure-Instances` in the order you gave them, or all configured peers when you give none. When there are more pieces than servers, the rotation starts over.

### Resumable Uploads

Resumable uploads accept the same options in the session request, which is the way to send a split file in parts:

```bash
curl -X POST \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"path": "/backups/big.bin", "size": 734003200, "erasure": {"data_shards": 4, "parity_shards": 2, "instances": ["node-2", "node-3"]}}' \
  https://node-1.example.com:8443/v1/uploads
```

Leave out a field of `erasure` to use the configured value; send `"erasure": {}` to split with the defaults. The upload limit for split files is 1 GiB. See the [File Storage API article](03-file-storage-api-rest-upload-download.md) for the part-by-part steps.

---

## Downloading

Downloading an erasure-coded file requires no special flags:

```bash
curl -H "Authorization: Bearer your-api-key" \
  https://node-1.example.com:8443/v1/files/backups/large-file.bin \
  -o large-file.bin
```

CallFS sees that the file is split, gathers the pieces and streams the original content as `application/octet-stream`, identical to what you uploaded. Range requests (`Range: bytes=0-1023`) and conditional requests (`If-None-Match`) work as for any other file. If servers are offline at download time, CallFS rebuilds the file from the pieces that remain as long as at least `data_shards` good pieces are reachable (four of six in the default profile). A piece whose checksum does not match is treated as lost. If too few good pieces remain, the download fails with `500`.

Any server can serve the download, whichever server received the upload.

---

## Checking Shard Status

### List and Inspect Split Files

`GET /v1/erasure/files` lists split files with their last known health. It accepts `prefix`, `health` (`ok`, `reduced`, `critical` or `unreadable`), `limit` and `cursor`:

```bash
curl -H "Authorization: Bearer your-api-key" \
  "https://node-1.example.com:8443/v1/erasure/files?prefix=/backups/&health=reduced"
```

Each item has `path`, `original_size`, `data_shards`, `parity_shards`, `shard_size`, `shards_total`, `healthy_shards`, `last_checked_at` and `health`. A file that has not been checked since the server started shows `health: "unknown"`. Asking for the pieces, manifest, verify or repair of a file that has no erasure record returns `404`.

`GET /v1/erasure/files/<path>` checks the pieces of one file and shows each one:

```bash
curl -H "Authorization: Bearer your-api-key" \
  https://node-1.example.com:8443/v1/erasure/files/backups/large-file.bin
```

```json
{
  "path": "/backups/large-file.bin",
  "original_size": 104857600,
  "profile": {"data_shards": 4, "parity_shards": 2, "shard_size": 26214400},
  "shards": [
    {"index": 0, "kind": "data", "instance_id": "node-1", "storage": "localfs", "path": ".callfs/shards/9c1e.../0", "size": 26214400, "checksum": "a3f5...", "state": "ok"},
    {"index": 5, "kind": "parity", "instance_id": "node-6", "storage": "localfs", "path": ".callfs/shards/9c1e.../5", "size": 26214400, "checksum": "7c21...", "state": "ok"}
  ],
  "tolerance": 2,
  "last_checked_at": "2026-04-11T10:00:00Z"
}
```

A piece's `state` is `ok`, `missing`, `corrupt` or `unreachable`. `tolerance` is how many pieces the file can lose. Overall health is:

| Health | Meaning |
|---|---|
| `ok` | Every piece is fine. |
| `reduced` | Some pieces are bad, but more than `data_shards` good ones remain. |
| `critical` | Exactly `data_shards` good pieces remain; one more loss makes the file unreadable. |
| `unreadable` | Fewer than `data_shards` good pieces remain. |

You need read permission on the file for these calls.

### Verify, Repair

`POST /v1/erasure/verify/<path>` reads every piece and reports, without changing anything:

```bash
curl -X POST -H "Authorization: Bearer your-api-key" \
  https://node-1.example.com:8443/v1/erasure/verify/backups/large-file.bin
```

```json
{"path": "/backups/large-file.bin", "readable": true, "healthy": 5, "required": 4, "shards": [{"index": 2, "state": "missing", "error": "shard not found"}]}
```

`POST /v1/erasure/repair/<path>` rebuilds missing or damaged pieces from the good ones and stores them again. It runs as a background job, so the answer is `202 Accepted` with `{"job_id": "..."}`, and you follow the job at `GET /v1/jobs/<job_id>`; the result lists the pieces repaired and the pieces that failed. Repair needs write permission on the file, which means the owner. Repair only works while the file is still readable, so repair as soon as a file turns `reduced`.

Administrators can check every split file under a prefix with `POST /v1/admin/erasure/verify-all` (body `{"prefix": "/backups/"}`, runs as a job) and see totals with `GET /v1/admin/erasure`.

### The Manifest

`GET /v1/erasure/manifest/<path>` returns the file's profile and the direct address of each piece, for auditing placement or building your own tooling:

```bash
curl -H "Authorization: Bearer your-api-key" \
  https://node-1.example.com:8443/v1/erasure/manifest/backups/large-file.bin
```

```json
{
  "path": "/backups/large-file.bin",
  "original_size": 104857600,
  "erasure_profile": {"data_shards": 4, "parity_shards": 2, "shard_size": 26214400},
  "shards": [
    {"index": 0, "endpoint": "https://node-1.example.com:8443/v1/shards/backups/large-file.bin/0", "size": 26214400, "checksum": "a3f5..."},
    {"index": 1, "endpoint": "https://node-2.example.com:8443/v1/shards/backups/large-file.bin/1", "size": 26214400, "checksum": "7c21..."}
  ]
}
```

Each address returns the raw piece with your normal bearer key, as long as you may read the file. If a server that holds a piece has no known address, the manifest request fails with `500`.

### Planning a Layout

Before you change settings, ask what a layout would give you. This administrator call places pieces on paper without writing anything:

```bash
curl -X POST \
  -H "Authorization: Bearer your-admin-api-key" \
  -H "Content-Type: application/json" \
  -d '{"data_shards": 4, "parity_shards": 2, "instances": ["node-1", "node-2", "node-3"]}' \
  https://node-1.example.com:8443/v1/admin/erasure/plan
```

The answer lists the `assignments`, the pieces per server (`per_instance`), `max_on_one_instance`, `tolerated_instance_failures`, `tolerated_piece_losses`, `overhead_ratio`, `fault_tolerant` and any `warnings`. With six pieces over three servers you get two pieces per server, so one server can fail (two lost pieces equal the spare count) but not two. When a server holds more pieces than there are spare pieces, one server failure can make files unreadable; CallFS raises the health finding "One server failure can break split files" until you spread pieces over more servers or add spare pieces.

---

## In the Web Console

Open `https://node-1.example.com:8443/ui/`, sign in, and go to Cluster, "Durability":

- The numbers at the top show split files, fully protected files, less protected files and files that cannot be read.
- A chart shows pieces per server, and a plan shows what a layout would tolerate (the same calculation as `/v1/admin/erasure/plan`).
- The table lists every split file with its layout (`4 data + 2 spare`), pieces OK, protection and last check.
- "Verify all split files" checks everything; "Repair damaged files" starts repairs.
- Select a file for its pieces, one row per piece with the server and state, and use "Verify pieces" or "Repair pieces".

The Files page also marks split files in a "Split" column and offers "Verify pieces" and "Repair pieces" in the file's menu. Health findings (Home page and Cluster) warn when files need repair. For metrics, see `callfs_erasure_pieces_repaired_total` in [Monitoring and metrics](../docs_markdown/06-monitoring-metrics.md). The console page is described in [Web console](../docs_markdown/08-web-console.md).

---

## Storage Efficiency

The primary motivation for erasure coding over full replication is storage overhead. Here is a comparison for a 100 GB file with the same fault tolerance (survive two simultaneous failures):

| Strategy | Copies / Shards | Storage Used | Servers Required |
|---|---|---|---|
| Full replication (3 copies) | 3 full copies | 300 GB | 3 |
| Erasure coding 4+2 | 6 pieces at 25 GB each | 150 GB | 6 |

Both strategies survive two simultaneous server failures. Erasure coding uses half the storage of three-way replication for the same file.

The overhead ratio is `(data_shards + parity_shards) / data_shards`. For 4+2 that is 6/4 = 1.5x. For 6+2 it drops to 8/6 = 1.33x, which needs more servers but wastes even less.

---

## When to Use Erasure Coding vs. a Backup Copy

CallFS also supports a backup copy (`ha.replication_enabled`), which writes a full second copy of every file to another storage, such as an S3 bucket. The two approaches serve different needs:

**Use erasure coding when:**

- Files are large (hundreds of MB to a few GB, up to 1 GiB per upload). The storage savings become significant at scale.
- You have several servers to spread pieces across.
- You can accept that every piece must be written before the upload completes, and that reads from a damaged set take a little longer than a direct read.
- Storage cost is a primary concern and you want the most fault tolerance per byte.

**Use a backup copy when:**

- Files are small or very large (above 1 GiB), or read latency is your top priority. Reading a whole file from one place is always faster than gathering pieces.
- Your cluster has fewer servers than a split layout needs.
- You want the simplest failure model: a copy is a complete, immediately readable file.

The two features can run side by side, because you choose erasure coding per upload or by file size with `erasure.min_file_size`. See [Storage](../docs_markdown/05-storage.md) and the [S3 backend article](04-callfs-s3-backend-hybrid-storage.md) for the backup copy.

---

## Summary

Erasure coding in CallFS gives you enterprise-grade data redundancy at 1.5x storage overhead instead of the 3x cost of full replication. You enable it in `config.yaml`, point your servers at each other with `instance_discovery.peer_endpoints`, and then request it per upload with a single HTTP header or query parameter, or let `erasure.min_file_size` split large files for you. Downloads are transparent, including when some servers are offline, and you can verify, repair and plan from the API or from the Durability page of the console.

For large files in multi-server deployments, erasure coding is the most storage-efficient path to surviving server failures without storing full copies of your data.
