#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Erasure Coding"

test_name "Generate 4KB random binary test file"
ERASURE_INPUT="${_TMPDIR}/erasure-input.bin"
ERASURE_OUTPUT="${_TMPDIR}/erasure-output.bin"
generate_random_binary "$ERASURE_INPUT" 4096
INPUT_SHA=$(sha256_file "$ERASURE_INPUT")
pass

test_name "POST with erasure=true query params returns 201"
upload_file_binary "$NODE1" "/erasure-test.bin?erasure=true&data_shards=2&parity_shards=1" "$ERASURE_INPUT" >/dev/null
_read_status
assert_status "201"
pass

test_name "GET erasure file, download to file, SHA-256 match"
download_file_to_file "$NODE1" "/erasure-test.bin" "$ERASURE_OUTPUT"
_read_status
assert_status "200"
assert_sha256_match "$ERASURE_INPUT" "$ERASURE_OUTPUT"
pass

test_name "GET ?manifest=true returns JSON with shards array"
BODY=$(callfs_curl GET "${NODE1}/v1/files/erasure-test.bin?manifest=true")
_read_status
assert_status "200"
assert_body_contains "$BODY" "shards"
pass

test_name "GET /v1/shards/erasure-test.bin/0 returns 200"
callfs_curl GET "${NODE1}/v1/shards/erasure-test.bin/0" >/dev/null
_read_status
assert_status "200"
pass

test_name "POST with erasure headers returns 201"
ERASURE_INPUT2="${_TMPDIR}/erasure-input2.bin"
generate_random_binary "$ERASURE_INPUT2" 4096
callfs_curl POST "${NODE1}/v1/files/erasure-header-test.bin" \
  -H "Content-Type: application/octet-stream" \
  -H "X-CallFS-Erasure: true" \
  -H "X-CallFS-Erasure-Data-Shards: 2" \
  -H "X-CallFS-Erasure-Parity-Shards: 1" \
  --data-binary "@${ERASURE_INPUT2}" >/dev/null
_read_status
assert_status "201"
pass

test_name "Manifest shard count equals data_shards + parity_shards"
SHARD_COUNT=$(echo "$BODY" | jq '.shards | length' 2>/dev/null) || SHARD_COUNT="0"
if [ "$SHARD_COUNT" -eq 3 ]; then
  pass
else
  fail "expected 3 shards, got ${SHARD_COUNT}"
fi

test_name "All shards accessible via their manifest endpoints"
ALL_OK=true
# Each shard has its own endpoint (may be on different nodes).
# Access each shard via its specific endpoint URL from the manifest.
for i in 0 1 2; do
  SHARD_URL=$(echo "$BODY" | jq -r ".shards[$i].endpoint" 2>/dev/null) || SHARD_URL=""
  if [ -z "$SHARD_URL" ] || [ "$SHARD_URL" = "null" ]; then
    fail "shard ${i} endpoint missing in manifest"
    ALL_OK=false
    break
  fi
  # Fetch the shard from its designated endpoint
  curl -s -o /dev/null -w '%{http_code}' --max-time "$CURL_TIMEOUT" \
    -H "Authorization: Bearer ${API_KEY}" "$SHARD_URL" > "$_STATUS_FILE" 2>/dev/null
  _read_status
  if [ "$LAST_STATUS" != "200" ]; then
    fail "shard ${i} at ${SHARD_URL} returned ${LAST_STATUS}, expected 200"
    ALL_OK=false
    break
  fi
done
if [ "$ALL_OK" = true ]; then
  pass
fi

# Cleanup
delete_file "$NODE1" "/erasure-test.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/erasure-header-test.bin" >/dev/null 2>&1 || true

print_summary
exit $?
