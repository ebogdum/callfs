# Changelog

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
