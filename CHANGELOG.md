# Changelog

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
