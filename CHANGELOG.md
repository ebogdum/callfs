# Changelog

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
