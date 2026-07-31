# Changelog

## [v1.6.0] - 2026-07-31

Findings from an adversarial security and performance review of the whole codebase. Ten confirmed defects fixed at the root, each covered by a regression test that was first confirmed to fail against the pre-fix code. No existing behaviour was changed except where noted under Breaking Changes.

### **Security**
- **Path-confusion privilege escalation (critical).** The metadata stores key on the exact request path string, while the storage backends normalize it via `pathutil.Clean` before touching disk. `ParseFilePath` validated the path but built `FullPath` from the raw request, so the two disagreed for any non-canonical path. `PUT /v1/files/dir/../secret.txt` created metadata under `/dir/../secret.txt` — no `ErrAlreadyExists`, and the ownership check fell through to a parent that does not exist and was allowed — while the bytes landed on `<root>/secret.txt`, **overwriting another user's file**. Any authenticated user could destroy any other user's data and leave orphaned metadata behind. `ParseFilePath` now canonicalizes before deriving `FullPath`, `ParentPath`, or `Name`.
- **Same class via the move endpoint (critical).** `PATCH /v1/files` resolved the `destination` body field with no validation and no canonicalization at all, giving a second independent route to the same overwrite. Request-body paths never pass through `ParseFilePath`, so they are now validated and canonicalized explicitly.
- **Root-escape check bypass.** `pathutil.Clean` returned early when a path collapsed to `/`, skipping the traversal-depth check entirely, so `..` and `a/../..` were accepted instead of rejected. Combined with canonicalization this would have retargeted operations at the root — a `DELETE` on `..` becoming a delete of `/`. The depth check now runs before the early return.
- **Unstable API-key identity.** User IDs were derived from a key's **position** in `auth.api_keys` (`api-user-0`, `api-user-1`, …) and that string is persisted as each resource's `owner`. Revoking one key shifted every later key's identity, silently transferring ownership of the revoked user's files to the next key holder. See New Features for the fix.
- **WebSocket upload memory exhaustion.** Uploads were buffered whole in memory, so peak usage was upload size × concurrency with no cap on concurrent sockets. Payloads now spool to an immediately-unlinked temp file past 8 MiB, bounding memory per connection.

### **Bug Fixes**
- **Raft FSM non-determinism.** `Apply` called `time.Now()`, but it runs independently on every replica and again on log replay after a restart. Replicas persisted different `UpdatedAt` values for the same log entry, and a restarting node **rewrote history** — every renamed file's modification time jumped to the restart moment. Commands are now stamped once by the leader in `applyAsLeader`, the single point where any command (local or forwarded) enters the log. Pre-upgrade log entries carry no timestamp and preserve their existing value on replay.

### **New Features**
- **`auth.api_key_users`** — maps an explicit app user ID to its API key, so identity is bound to a name rather than to list position. Keys can be revoked, rotated, or reordered without reassigning ownership of existing files. Configurable per entry from the environment as `CALLFS_AUTH__API_KEY_USERS__<USER_ID>`. The legacy `auth.api_keys` list is unchanged and continues to work; both forms may be used together for migration, and a startup warning is logged when the positional form is in use.

### **Performance**
- **Raft directory listing is no longer O(total files).** Listing one directory scanned every metadata entry in the cluster while holding the FSM lock. A child index now answers listings directly. The index is derived, not persisted — it is rebuilt on snapshot restore, so the snapshot format is unchanged and a defect there cannot corrupt replicated state.
- **Redis directory listing no longer issues one round trip per child.** `ListChildren` called `GET` per entry, so latency grew linearly with directory size. It now batches through `MGET` in bounded chunks of 512.
- **Rate-limiter eviction no longer scans per request.** At the 100k-entry cap the limiter scanned the whole map under its mutex to evict a single entry, so every request from a new IP paid a full scan — an address-rotating client could hold the map at the cap and serialize all request handling. Eviction now reclaims expired entries first, otherwise frees a 1,000-entry batch.

### **Breaking Changes**
- CallFS now **refuses to start if `auth.internal_proxy_secret` is reused as an API key**. Such a configuration previously started and silently granted that key holder full admin bypass. A config doing this will need the duplicate removed before upgrading.
- `auth.api_key_users` rejects the reserved user IDs `root`, `internal-proxy`, and anything matching `api-user-<number>`, and a key may appear only once across both key forms.
- The `api_keys` default no longer pre-seeds the `default-api-key` placeholder. That value was always rejected at validation, so a server with no configured keys still fails to start — only the error message changes.

### **Internal Changes**
- The release workflow is now idempotent: it uploads into an existing release for the tag instead of failing, so a tag published by `release.sh` no longer leaves a red build.
- Raft FSMs are built through a single `newFSM` constructor shared by production and tests, so state fields cannot be initialized in one path and missed in the other.

### **Tests**
- Nine new test files covering path canonicalization and root-escape rejection, move-destination validation, FSM determinism and legacy-log replay, the Raft child index (validated against the full scan it replaced, including snapshot restore), API-key identity stability across revocation, auth config validation, environment-variable overrides, rate-limiter eviction, and upload spooling across the memory/disk boundary.

### **Documentation**
- `README.md`, `config.yaml.example`, `docs_markdown/02-configuration.md`, and `docs_markdown/04-authentication-security.md` document `api_key_users`, the positional-identity hazard in `api_keys`, and the reserved user IDs. The documented environment-variable form was verified by an executing test rather than assumed.

---

## [v1.5.1] - 2026-07-31

Security patch release. Resolves all 16 open dependency advisories reported against the repository. `govulncheck` now reports **0 vulnerabilities affecting CallFS code** and 0 in imported packages.

### **Security**
- **quic-go 0.59.0 → 0.59.1** — fixes HTTP/3 QPACK trailer expansion memory exhaustion ([GHSA/quic-go](https://github.com/quic-go/quic-go/security/advisories)). This was the one directly reachable advisory: `http3.Server` is exposed whenever QUIC is enabled, so an unauthenticated peer could drive memory growth via crafted trailers.
- **go-chi/chi 5.2.2 → 5.3.0** — fixes an open redirect in the `RedirectSlashes` middleware plus three further routing advisories (GO-2026-5774/5775/5777). CallFS does not mount `RedirectSlashes`, so the open redirect was not exploitable here; upgraded regardless.
- **golang.org/x/crypto 0.45.0 → 0.53.0** — clears 12 advisories (4 critical), all in `x/crypto/ssh`: agent-constraint bypass, `@revoked` auth bypass, FIDO/U2F presence-check bypass, `VerifiedPublicKeyCallback` permission skips, server deadlock, and several panic/DoS paths. CallFS imports no SSH code, so these were transitive-only.
- **golang.org/x/net 0.47.0 → 0.56.0** — fixes an HTML-parser DoS (GO-2026-5942) and related advisories.
- **golang.org/x/text 0.37.0 → 0.39.0** — fixes GO-2026-5970.

Three advisories remain unresolved upstream with no patched release available: `GO-2026-5932` (`x/crypto`) and `GO-2022-0635`/`GO-2022-0646` (`aws-sdk-go` S3 crypto client). None are reachable from CallFS code.

### **Breaking Changes**
- **Minimum Go version for building from source is now 1.25** (was 1.24), required by quic-go 0.59.1. The Docker build image moves to `golang:1.25-alpine`. Prebuilt binaries and container images are unaffected.

### **Internal Changes**
- Closes Dependabot PRs [#4](https://github.com/ebogdum/callfs/pull/4), [#6](https://github.com/ebogdum/callfs/pull/6), [#7](https://github.com/ebogdum/callfs/pull/7), [#8](https://github.com/ebogdum/callfs/pull/8) — superseded by this consolidated upgrade, which lands newer patched versions than the individual PRs proposed.
- Updated the Go version requirement in `README.md`, `docs_markdown/01-installation.md`, and `docs_markdown/08-developer-guide.md`.

---

## [v1.5.0] - 2026-06-05

### **New Features**
- **First-class rename and move** (`PATCH /v1/files/{path}`), resolving [#3](https://github.com/ebogdum/callfs/issues/3). Two distinct operations selected by the request body:
  - **Rename** (`{"name":"..."}`) — change a resource's name in place, keeping the same folder, backend, and instance.
  - **Move** (`{"destination":"..."}`) — relocate to a different folder and, optionally, a different `backend` or `instance`, with `overwrite` and `create_parents` flags.
  - Both files and directories (with their entire subtree) are supported. Each operation updates the metadata store **and** relocates the underlying bytes together — bytes first, then metadata, leaving the source intact if the byte move fails. Same-instance localfs renames are atomic.
  - Supported on all metadata stores (SQLite, PostgreSQL, Redis, Raft) and all backends (local FS, S3, internal proxy); erasure-coded files are renamed by re-keying their shard metadata without moving shard data.
  - Cross-server aware: requests are proxied to the owning (or target) node automatically.

### **Enhancements**
- New `metadata.Store.Rename` primitive (atomic subtree re-key) and `backends.Storage.Move` primitive (atomic local rename / server-side S3 copy+delete).

### **Tests**
- Added unit tests for SQLite subtree rename, local FS move (incl. cross-device fallback), and engine-level rename/move (folder move, overwrite, cross-backend relocation, directory move, validation). Added integration suite `34-rename-move.sh`.

### **Documentation**
- Documented the rename/move API in `README.md`, `docs_markdown/03-api-reference.md`, and the OpenAPI/Swagger specs.

### **v1 Limitations**
- Re-tiering a file to/from the `erasure` backend and moving a *directory* across backends or instances are not supported and return `400`.

---

## [v1.3.0] - 2026-04-11

### **Breaking Changes**
- **Authorization model replaced**: Removed Unix-style UID/GID permission system. Authorization is now based on an `owner` string field (the app user ID). App users have no relationship to OS users. The `Metadata` struct no longer has `UID` or `GID` fields; use `Owner` instead.
- **API response headers changed**: `X-CallFS-UID` and `X-CallFS-GID` headers replaced with `X-CallFS-Owner`.
- **JSON response format changed**: File listing `uid`/`gid` integer fields replaced with `owner` string field.
- **Database schema migration**: SQLite schema replaces `uid`/`gid` INTEGER columns with `owner` TEXT. Postgres migration `003_replace_uid_gid_with_owner` adds `owner` column and drops `uid`/`gid`.

### **New Features**
- Owner-based access control: files and directories track their creator via the `Owner` field. Owners have full access; other authenticated users can read files and create children in directories; admin users (`root`, `internal-proxy`) bypass all checks.
- `Engine.GetMetadataUncached()` for bypassing the metadata cache when fresh data is needed (used for cross-server Content-Length accuracy).

### **Bug Fixes**
- **Raft stale reads**: Fixed `ListChildren` on follower nodes returning partially-replicated results. Followers now forward `ListChildren` to the leader for consistent reads, with local FSM as fallback.
- **Cross-server Content-Length mismatch**: Fixed empty responses when reading files after concurrent cross-server writes. The GET handler now re-fetches metadata from the store (bypassing cache) for cross-server files.
- **S3/MinIO SSE error**: Fixed 500 errors on S3 file creation with MinIO. Default `s3_server_side_encryption` is `AES256` which MinIO rejects without KMS. Test config now explicitly disables SSE for MinIO.
- **Integration test env-var override**: Fixed suite 30 failing due to `docker exec sh` in scratch container. Now probes from a separate network container.
- **Test framework counting**: Fixed `lib.sh` assertion counter double-counting failures within retried test cases.

### **Tests**
- Added 16 unit tests: `auth/unix_authorizer_test.go` (10 tests), `auth/apikey_authenticator_test.go` (6 tests).
- All 35 Docker-based integration test suites pass (Raft cluster, SQLite/Postgres/Redis metadata, S3/MinIO, erasure coding, concurrent ops, config validation, env overrides).

### **Documentation**
- Comprehensive audit of all 12 documentation files against the codebase. Fixed 23 discrepancies including: authorization model, metrics endpoint auth, error response format, S3 config structure, env var separator format, Go version, removed nonexistent make commands and metrics.

---

## [Unreleased] - TBD

### **New Features**
- Added Raft metadata mode with leader-forwarded applies and node join workflow.
- Added metadata store options for SQLite and Redis.
- Added local distributed lock manager option (`dlm.type=local`).

### **Enhancements**
- Improved internal proxy HTTP transport tuning for high-concurrency traffic.
- Added WebSocket transfer endpoint for file upload/download streaming.

### **Bug Fixes**
- Fixed API key identity mapping regression by removing special-case key-to-root behavior.
- Fixed missing resource authorization semantics to return not-found instead of permission-denied for read/delete.

### **Internal Changes**
- Removed MinIO services from `docker-compose.yml` and kept compose focused on PostgreSQL and Redis dependencies.
- Added support in runtime/config validation for `metadata_store.type` and `dlm.type` selection paths.

### **Tests**
- Validated 3-node Raft cluster health, cross-node HTTP/WS operations, and load/failover scenarios.
- Re-ran test suite after auth fixes (`go test` pass).

### **Documentation**
- Updated install/config/cluster docs for Raft join flow, protocol modes, and current compose usage.
- Fixed documentation drift in setup instructions and removed duplicated requirements in configuration reference.

---
