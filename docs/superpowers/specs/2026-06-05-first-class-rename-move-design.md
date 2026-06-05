# First-class Rename / Move — Design

**Date:** 2026-06-05
**Issue:** [#3 — Is a rename possible as a first class operation?](https://github.com/ebogdum/callfs/issues/3)
**Status:** Proposed (pending owner review)

---

## Problem

CallFS has no first-class rename or move operation. Today a client must do
`GET` (download) → `POST`/`PUT` (re-upload to new path) → `DELETE` (remove old),
streaming the entire payload through the client twice. This is slow, non-atomic,
and racy. The issue asks for a first-class `PATCH` rename.

This design covers **rename and move as one operation**: changing a file or
directory's path, and — when requested — physically relocating its bytes to a
different storage backend and/or owning instance.

## Core model

A file's physical location in CallFS is the triple
`(backend_type, callfs_instance_id, path-derived key)`. A rename/move can change
**any** of the three. There is no such thing as a "metadata-only" rename: the
path *is* the storage key, so changing it always moves bytes within (or across)
a backend.

Three cases, which **compose** (one request may change all three at once):

| # | Case | What moves | Atomic? |
|---|------|-----------|---------|
| 1 | **Path-only (rename)** — same backend + instance, new key | Bytes move *within* the backend: localfs `os.Rename`; S3 `CopyObject`+`DeleteObject`; erasure shard-key rewrite | localfs same-instance: **yes**. S3: no (copy+delete). |
| 2 | **Cross-instance (move)** — same backend type, different owning node | Bytes transfer over the network to the target node's store; source deleted after target confirmed | No |
| 3 | **Cross-backend (move / retier)** — e.g. localfs → S3, or to/from erasure | Bytes stream out of source backend, written into target backend; `backend_type` updated; source deleted after | No |

Directory operations apply the case to the whole subtree.

## Public contract

```
PATCH /v1/files/{path}
Authorization: Bearer <key>
Content-Type: application/json

{
  "destination":    "/archive/2026/report.txt",   // required, absolute path
  "backend":        "s3",        // optional → defaults to source's backend_type
  "instance":       "node-b",    // optional → defaults to source's instance (N/A for shared backends)
  "overwrite":      false,       // optional, default false
  "create_parents": false        // optional, default false
}
```

- `overwrite` may also be supplied as `?overwrite=true` (query param); body wins if both present.
- Source type (file/directory) is inferred from the existing inode. A trailing slash on `{path}` is tolerated.
- `backend` is validated against configured backends; `instance` against known peer endpoints (`engine.GetPeerEndpoint`). Unknown values → `400`.
- `create_parents` only matters when the destination's parent folder does not exist:
  - in-place rename (same folder) never triggers it;
  - `false` + missing parent → `409`;
  - `true` → create the missing folders, owned by the requester, then move.

### Responses

| Status | Condition |
|--------|-----------|
| `200 OK` | Renamed/moved. Body: `{"path":"/archive/2026/report.txt"}` |
| `400 Bad Request` | Missing/invalid `destination`; `destination == source`; moving a directory into its own subtree; unknown `backend`/`instance`; attempt to rename `/` |
| `403 Forbidden` | Caller is not the source owner, or lacks write on the destination parent |
| `404 Not Found` | Source does not exist |
| `409 Conflict` | Destination exists and `overwrite=false`; overwrite type mismatch (file↔dir); overwrite target is a non-empty directory; destination parent missing and `create_parents=false` |
| `502 Bad Gateway` | Cross-instance proxy failure |
| `500` | Internal error |

### Authorization

A move is a delete-at-source + create-at-destination:

- `DeletePerm` on the **source** (owner-only, per `UnixAuthorizer` — files: owner only; directories: owner only to remove).
- `WritePerm` on the **destination parent** (uses existing `checkParentPermission`).
- The moved inode retains its original `Owner`. (App users are not OS users; ownership is the app-level `Owner` string and is preserved across the move.)

## Atomicity & failure semantics

- **Case 1 (same-instance localfs)**: atomic via `os.Rename`.
- **Cases 2 & 3 (physical relocation)**: inherently copy-then-delete, **not atomic**. Rollback-safe best-effort:
  - write target first; **delete source only after the target write is confirmed**;
  - if the target write fails, the source is left intact and an error is returned (nothing deleted);
  - directory subtree relocations that fail partway return a clear partial-failure error listing what already moved (no silent truncation).
- Metadata re-key for the subtree is always performed in a **single transaction** (SQL stores) / atomic command (raft FSM) / `MULTI`-`EXEC` (redis), applied *after* the byte moves succeed.

## Components

### 1. Metadata store — `Store.Rename(ctx, oldPath, newPath string) error`

New interface method on `metadata.Store`, no-clobber re-key (returns `ErrAlreadyExists`
if `newPath` exists; engine handles overwrite by deleting the destination first).
Updates `name`, `path`, `parent_id`; for a directory, rewrites every descendant's `path`.

- **sqlite / postgres**: one transaction. Root row: `UPDATE ... SET name=?, path=?, parent_id=? WHERE path=?`. Descendants: `UPDATE inodes SET path = ? || substr(path, ?) WHERE path LIKE ? ESCAPE '\'` (prefix `oldPath || '/'`). `path` is `NOT NULL UNIQUE`, so dest-exists collisions surface as constraint errors mapped to `ErrAlreadyExists`.
- **redis**: read affected keys, rewrite the path-keyed entries and any path/parent indexes inside `MULTI`/`EXEC`.
- **raft**: new `rename_metadata` command op; the FSM `Apply` performs the subtree rewrite against in-memory state. Add to the `applyCommand` switch and command struct.

### 2. Backend storage — `Storage.Move(ctx, oldRelPath, newRelPath string) error`

New method on `backends.Storage`:

- **localfs**: `os.Rename`; on `EXDEV` (cross-device) fall back to copy+remove. Works for files and whole directory subtrees in one call.
- **s3**: `CopyObject` + `DeleteObject` (single object). Directory prefixes are virtual/metadata-only — the engine drives per-file moves.
- **internalproxy**: proxy the move to the owning peer instance.

Cross-backend relocation (case 3) does **not** use `Storage.Move`; the engine streams
`source.Open` → `target.Create` and then `source.Delete`, because the two ends are
different backends.

### 3. Engine — `core/rename_operations.go` → `Engine.Move(ctx, src, dst string, opts MoveOptions) error`

`MoveOptions{ Backend, Instance string; Overwrite, CreateParents bool }`.

1. Validate: `dst != src`; `dst` not a descendant of `src`; `src != "/"`; backend/instance known.
2. Load source metadata; resolve effective target backend/instance (default to source's).
3. Acquire locks on `src` and `dst` keys, ordered lexicographically to avoid deadlock (`file:`/`dir:` prefix as in `DeleteFile`).
4. Resolve destination parent; if missing → honor `create_parents` (reuse `ensureParentDirectories`) or `409`.
5. Handle existing destination: `overwrite=false` → `ErrAlreadyExists`; else delete destination first (non-empty dir → reject, mirroring `DeleteFile`).
6. Dispatch:
   - **Case 1** (same backend+instance): `Storage.Move` (or `os.Rename` subtree for localfs dirs; per-object for S3 dirs) → `Store.Rename`.
   - **Case 2** (cross-instance): proxy byte move to owning/target node (`MoveFileOnInstance` / create-on-target + delete-on-source) → `Store.Rename` + update `callfs_instance_id`.
   - **Case 3** (cross-backend): stream `source.Open` → `target.Create`, update `backend_type`, `source.Delete` → `Store.Rename`. Erasure re-tiering: re-shard (to erasure) or reassemble (from erasure) via the erasure `Manager`.
7. Replicate the move to the secondary backend if HA replication is enabled (best-effort, mirroring `replicateFileToSecondaryBackend`/`deleteReplicatedFile`).
8. Invalidate `metadataCache` for `src`, `dst`, and both parent prefixes.

### 4. Cross-instance — `core/cross_instance_operations.go`

- `MoveFileOnInstance(ctx, instanceID, oldPath, newPath)` — same-node rename proxied to the owning peer (peer receives a normal `PATCH`).
- `CreateFileOnInstance(ctx, instanceID, path, reader, size)` — needed for case 2 (today only Update/Delete/Stat-on-instance exist). Adds the create direction to the internal proxy adapter.

### 5. Handler — `server/handlers/patch_file.go` → `V1PatchFileEnhanced(...)`

Follows the established handler shape (`ParseFilePath`, `GetUserID`, normalize path,
authorize, dispatch, map errors, structured log). Decodes the JSON body, validates
`destination`, authorizes `DeletePerm` on source + `WritePerm` on destination parent,
calls `engine.Move`, maps engine errors to the status table above.

### 6. Route — `server/router.go`

Add `r.Patch("/*", handlers.V1PatchFileEnhanced(engine, authorizer, backendConfig, serverConfig, logger))`
inside the `/files` route group.

## Documented limitations (v1)

- Directory rename/move locks the src/dst directory keys, **not** every descendant individually — concurrent writes *inside* a subtree mid-move are not serialized. This matches the locking granularity the codebase already uses; documented, not solved in v1.
- Cross-backend / cross-instance / S3 multi-object directory moves are **not atomic**. They are rollback-safe (source preserved until target confirmed) and report partial failure, but a crash mid-move can leave a partially-moved subtree. Single-instance localfs renames *are* atomic.

## Testing

- **Metadata stores** (sqlite, postgres, redis, raft): `Rename` for a file; directory subtree rewrite; no-clobber returns `ErrAlreadyExists`; descendants' paths updated correctly.
- **Backends**: localfs `Move` same-device and cross-device (`EXDEV` fallback); s3 `Move` copy+delete.
- **Engine**: case 1 file + directory; overwrite (file, empty dir, non-empty dir rejected); case 2 cross-instance (mocked proxy); case 3 cross-backend incl. erasure re-tier; `create_parents` on/off; lock ordering.
- **Handler**: each status code in the table; body parsing; authorization (non-owner rejected, dest-parent write enforced).
- **Integration**: end-to-end rename and cross-backend move over HTTP.

## Docs / housekeeping

- README: add `PATCH` row to the Files API table + a curl example.
- `CHANGELOG.md`, `.context/INDEX.md`, `.context/CHANGELOG.md` per project process.

## Open decisions (default-resolved, flip on review)

- **(a)** Target backend/instance expressed via `backend`/`instance` body fields. *Default: yes.*
- **(b)** Cross-backend/instance moves are rollback-safe best-effort (source kept until target confirmed), not consensus-acknowledged. *Default: best-effort.*
