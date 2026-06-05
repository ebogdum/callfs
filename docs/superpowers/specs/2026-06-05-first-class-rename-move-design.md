# First-class Rename & Move — Design

**Date:** 2026-06-05
**Issue:** [#3 — Is a rename possible as a first class operation?](https://github.com/ebogdum/callfs/issues/3)
**Status:** Proposed (pending owner review)

---

## Problem

CallFS has no first-class rename or move operation. Today a client must do
`GET` (download) → `POST`/`PUT` (re-upload to new path) → `DELETE` (remove old),
streaming the entire payload through the client twice. This is slow, non-atomic,
and racy. The issue asks for a first-class `PATCH`.

## Two distinct operations

| Operation | Changes | Stays the same |
|-----------|---------|----------------|
| **Rename** | the **name** only | folder, backend, instance |
| **Move** | **location** — folder/path, backend, and/or instance | the name (unless the destination renames it too) |

Rename keeps the resource exactly where it is and only changes what it's called.
Move relocates it. They are separate intents with separate request shapes.

### Both operations affect **both** layers

There is no metadata-only rename. A resource's storage key *is* its path, so
changing the name or the location always does two things together:

1. **Store (metadata)** — re-key the inode: rename updates `name` + `path`;
   move updates `path` (and `backend_type` / `callfs_instance_id` when those change).
2. **Bytes (backend)** — physically move the object, because the key changed:
   - rename: `os.Rename` (localfs) or `CopyObject`+`DeleteObject` (S3) *within the same backend*;
   - move: relocate across folder, backend, and/or instance.

Order of operations: **move the bytes first, then re-key the metadata.** If the
byte move fails, the metadata is untouched and the source is left intact (rollback-safe).

## Public contract

Single endpoint, two mutually-exclusive payloads:

```
PATCH /v1/files/{path}
Authorization: Bearer <key>
Content-Type: application/json
```

**Rename** — `name` present (no slashes allowed):
```json
{ "name": "report-final.txt" }
```
`/old/report.txt` → `/old/report-final.txt`. Same folder, backend, instance.

**Move** — `destination` present:
```json
{
  "destination":    "/archive/2026/report.txt",   // required, absolute path
  "backend":        "s3",        // optional → defaults to source's backend_type
  "instance":       "node-b",    // optional → defaults to source's instance (N/A for shared backends)
  "overwrite":      false,       // optional, default false
  "create_parents": false        // optional, default false
}
```

Rules:
- Exactly one of `name` / `destination` must be present. Both → `400`. Neither → `400`.
- `name` must be a single path segment (no `/`).
- `overwrite` may also be `?overwrite=true`; body wins if both present.
- `backend` validated against configured backends; `instance` against known peer endpoints (`engine.GetPeerEndpoint`). Unknown → `400`.
- `create_parents` only matters for **move** when the destination's parent folder is missing: `false` → `409`; `true` → create the missing folders (owned by the requester), then move. (Rename never needs it — the folder already exists.)
- Source type (file/directory) inferred from the existing inode; trailing slash on `{path}` tolerated. Directory operations apply to the whole subtree.

### Responses

| Status | Condition |
|--------|-----------|
| `200 OK` | Done. Body: `{"path":"/new/full/path"}` |
| `400 Bad Request` | Neither/both of `name`/`destination`; `name` contains `/`; invalid `destination`; `destination == source`; moving a directory into its own subtree; unknown `backend`/`instance`; renaming/moving `/` |
| `403 Forbidden` | Caller is not the source owner, or lacks write on the destination parent |
| `404 Not Found` | Source does not exist |
| `409 Conflict` | Target exists and `overwrite=false`; overwrite type mismatch (file↔dir); overwrite target is a non-empty directory; destination parent missing and `create_parents=false` |
| `502 Bad Gateway` | Cross-instance proxy failure |
| `500` | Internal error |

### Authorization

Both operations are a delete-at-source + create-at-target:

- `DeletePerm` on the **source** (owner-only, per `UnixAuthorizer`).
- `WritePerm` on the **target parent** (existing `checkParentPermission`). For rename
  the target parent is the current folder.
- The inode retains its original `Owner`. (App users are not OS users; the app-level
  `Owner` string is preserved across rename/move.)

## Move: the three relocation cases

`destination` can change folder, backend, and/or instance — these **compose** in one request:

| # | Case | Byte move | Atomic? |
|---|------|-----------|---------|
| 1 | **Folder change, same backend+instance** | `os.Rename` (localfs) / `CopyObject`+`DeleteObject` (S3) / erasure shard-key rewrite | localfs same-instance: **yes**. S3: no. |
| 2 | **Cross-instance** — same backend type, different owning node | bytes transfer over the network to the target node; source deleted after target confirmed | No |
| 3 | **Cross-backend** — e.g. localfs → S3, or to/from erasure | stream out of source backend, write into target backend; source deleted after | No |

### Case 2 — cross-instance, explicit flow

Moving `/foo.txt` from `node-a` to `node-b`:

1. Read source bytes from the owning instance (`node-a`) via the internal proxy (`Open`/`Stat`).
2. Stream into the target instance (`node-b`) via new `CreateFileOnInstance` plumbing.
3. After `node-b` confirms the write, delete the source bytes on `node-a`.
4. Re-key shared metadata: update `callfs_instance_id` → `node-b` **and** `path` in one transaction.

## Atomicity & failure semantics

- **Rename and Case-1 move on same-instance localfs**: atomic via `os.Rename`.
- **S3 key change, and Cases 2 & 3**: copy-then-delete, **not atomic**. Rollback-safe:
  write target first; delete source only after the target write is confirmed; on target
  failure nothing is deleted and an error is returned. Directory subtree relocations that
  fail partway return a clear partial-failure error listing what already moved (no silent truncation).
- The metadata subtree re-key is always a **single transaction** (SQL) / atomic command
  (raft FSM) / `MULTI`-`EXEC` (redis), applied **after** the byte move succeeds.

## Components

### 1. Metadata store — `Store.Rename(ctx, oldPath, newPath string) error`

New `metadata.Store` method, no-clobber re-key (returns `ErrAlreadyExists` if `newPath`
exists; the engine handles overwrite by deleting the target first). Updates `name`,
`path`, `parent_id`; for a directory rewrites every descendant's `path`. Used by **both**
rename and move (move additionally updates `backend_type`/`callfs_instance_id` via the
existing `Update` within the same transaction where the backend/instance changed).

- **sqlite / postgres**: one transaction. Root row `UPDATE ... SET name=?, path=?, parent_id=? WHERE path=?`; descendants `UPDATE inodes SET path = ? || substr(path, ?) WHERE path LIKE ? ESCAPE '\'`. `path` is `NOT NULL UNIQUE`; collisions → `ErrAlreadyExists`.
- **redis**: rewrite the path-keyed entries and path/parent indexes inside `MULTI`/`EXEC`.
- **raft**: new `rename_metadata` command op; the FSM `Apply` performs the subtree rewrite. Add to the command struct and `applyCommand` switch.

### 2. Backend storage — `Storage.Move(ctx, oldRelPath, newRelPath string) error`

New `backends.Storage` method (used by rename and Case-1 move):

- **localfs**: `os.Rename`; on `EXDEV` fall back to copy+remove. Files and whole subtrees.
- **s3**: `CopyObject` + `DeleteObject` (single object); directory prefixes are virtual, engine drives per-file.
- **internalproxy**: proxy the move to the owning peer.

Cross-backend relocation (Case 3) does **not** use `Storage.Move`; the engine streams
`source.Open` → `target.Create` → `source.Delete` across two different backends.

### 3. Engine — `core/rename_operations.go`

- `Engine.Rename(ctx, path, newName string) error` — same-folder name change: validate `newName` has no slash; compute new path; lock; `Storage.Move` + `Store.Rename`.
- `Engine.Move(ctx, src, dst string, opts MoveOptions) error` where `MoveOptions{ Backend, Instance string; Overwrite, CreateParents bool }`:
  1. Validate `dst != src`, `dst` not a descendant of `src`, `src != "/"`, backend/instance known.
  2. Resolve effective target backend/instance (default to source's).
  3. Lock `src` and `dst` keys, ordered lexicographically (deadlock-safe), `file:`/`dir:` prefix as in `DeleteFile`.
  4. Resolve destination parent; missing → honor `create_parents` (reuse `ensureParentDirectories`) or `409`.
  5. Existing target: `overwrite=false` → `ErrAlreadyExists`; else delete target first (non-empty dir rejected, mirroring `DeleteFile`).
  6. Dispatch Case 1 / 2 / 3 (above), incl. erasure re-tier via the erasure `Manager` when backend changes to/from `erasure`.
  7. Replicate the move to the secondary backend if HA replication is on (best-effort).
  8. Invalidate `metadataCache` for `src`, `dst`, and both parent prefixes.

### 4. Cross-instance — `core/cross_instance_operations.go`

- `MoveFileOnInstance(ctx, instanceID, oldPath, newPath)` — proxy a same-node rename/Case-1 move to the owning peer.
- `CreateFileOnInstance(ctx, instanceID, path, reader, size)` — **new** create direction (today only Update/Delete/Stat exist on a peer); required for Case 2.

### 5. Handler — `server/handlers/patch_file.go` → `V1PatchFileEnhanced(...)`

Established handler shape (`ParseFilePath`, `GetUserID`, normalize, authorize, dispatch,
map errors, structured log). Decodes the body, enforces the exactly-one-of `name`/`destination`
rule, authorizes `DeletePerm` on source + `WritePerm` on the target parent, calls
`engine.Rename` or `engine.Move`, maps errors to the status table.

### 6. Route — `server/router.go`

Add `r.Patch("/*", handlers.V1PatchFileEnhanced(engine, authorizer, backendConfig, serverConfig, logger))`
inside the `/files` group.

## Documented limitations (v1)

- Directory rename/move locks the src/dst directory keys, **not** every descendant — concurrent writes *inside* a subtree mid-operation are not serialized (matches existing locking granularity). Documented, not solved.
- Cross-backend / cross-instance / S3 multi-object directory moves are **not atomic** — rollback-safe and report partial failure, but a crash mid-move can leave a partially-moved subtree. Single-instance localfs rename / Case-1 move *are* atomic.

## Testing

- **Metadata stores** (sqlite, postgres, redis, raft): `Rename` file; directory subtree rewrite; no-clobber → `ErrAlreadyExists`; descendant paths updated.
- **Backends**: localfs `Move` same-device + `EXDEV` fallback; s3 `Move` copy+delete.
- **Engine**: rename (file, dir); move Case 1/2/3 incl. erasure re-tier; overwrite (file, empty dir, non-empty rejected); `create_parents` on/off; cross-instance (mocked proxy); lock ordering.
- **Handler**: each status code; exactly-one-of enforcement; authorization (non-owner rejected, target-parent write enforced).
- **Integration**: end-to-end rename, folder move, cross-backend move over HTTP.

## Docs / housekeeping

- README: `PATCH` row in the Files API table + rename and move curl examples.
- `CHANGELOG.md`, `.context/INDEX.md`, `.context/CHANGELOG.md` per project process.

## Open decisions (default-resolved, flip on review)

- **(a)** Move target backend/instance via `backend`/`instance` body fields. *Default: yes.*
- **(b)** Cross-backend/instance moves are rollback-safe best-effort (source kept until target confirmed), not consensus-acknowledged. *Default: best-effort.*
- **(c)** Erasure re-tiering (move to/from `erasure`) included in v1. *Default: yes — flag if you'd rather defer it to keep the first cut lean.*
