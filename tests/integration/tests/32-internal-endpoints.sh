#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Internal Endpoints"

# --- PUT internal shard ---

test_name "PUT /v1/internal/shards with internal secret returns 200 or 201"
SHARD_DATA="test-shard-binary-payload-12345"
callfs_curl_token "$INTERNAL_SECRET" PUT "${NODE1}/v1/internal/shards/test-shard-file/0" \
  -H "Content-Type: application/octet-stream" \
  -d "$SHARD_DATA" >/dev/null
_read_status
if [ "$LAST_STATUS" != "200" ] && [ "$LAST_STATUS" != "201" ]; then
  fail "expected status 200 or 201, got $LAST_STATUS"
else
  pass
fi

# --- GET internal shard ---

test_name "GET /v1/internal/shards with internal secret returns stored data"
BODY=$(callfs_curl_token "$INTERNAL_SECRET" GET "${NODE1}/v1/internal/shards/test-shard-file/0")
_read_status
assert_status "200"
assert_body_equals "$BODY" "$SHARD_DATA"
pass

# --- DELETE internal shard ---

test_name "DELETE /v1/internal/shards with internal secret returns 204"
callfs_curl_token "$INTERNAL_SECRET" DELETE "${NODE1}/v1/internal/shards/test-shard-file/0" >/dev/null
_read_status
assert_status "204"
pass

# --- Auth failures ---

test_name "GET /v1/internal/shards without auth returns 401"
callfs_curl_noauth GET "${NODE1}/v1/internal/shards/test-shard-file/0" >/dev/null
_read_status
assert_status "401"
pass

test_name "GET /v1/internal/shards with regular API key returns 401 or 403"
callfs_curl GET "${NODE1}/v1/internal/shards/test-shard-file/0" >/dev/null
_read_status
if [ "$LAST_STATUS" != "401" ] && [ "$LAST_STATUS" != "403" ]; then
  fail "expected status 401 or 403, got $LAST_STATUS"
else
  pass
fi

test_name "POST /v1/internal/raft/join without auth returns 401"
callfs_curl_noauth POST "${NODE1}/v1/internal/raft/join" \
  -H "Content-Type: application/json" \
  -d '{"node_id":"fake","addr":"fake:8443"}' >/dev/null
_read_status
assert_status "401"
pass

test_name "POST /v1/internal/raft/metadata/apply without auth returns 401"
callfs_curl_noauth POST "${NODE1}/v1/internal/raft/metadata/apply" \
  -H "Content-Type: application/json" \
  -d '{"op":"set","key":"test","value":"test"}' >/dev/null
_read_status
assert_status "401"
pass

# --- Cleanup ---

test_name "Cleanup internal endpoint test artifacts"
callfs_curl_token "$INTERNAL_SECRET" DELETE "${NODE1}/v1/internal/shards/test-shard-file/0" >/dev/null 2>&1 || true
pass

print_summary
exit $?
