#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Chunked Upload (Transfer-Encoding)"

# --- 4KB chunked upload ---

test_name "Generate 4KB random binary for chunked upload"
CHUNKED_INPUT="${_TMPDIR}/chunked-input.bin"
CHUNKED_DOWN="${_TMPDIR}/chunked-down.bin"
generate_random_binary "$CHUNKED_INPUT" 4096
INPUT_SHA=$(sha256_file "$CHUNKED_INPUT")
pass

test_name "POST with chunked Transfer-Encoding returns 201"
callfs_curl POST "${NODE1}/v1/files/chunked-test.bin" \
  -H "Content-Type: application/octet-stream" \
  -H "Transfer-Encoding: chunked" \
  --data-binary "@${CHUNKED_INPUT}" >/dev/null
_read_status
assert_status "201"
pass

test_name "HEAD shows correct size after chunked upload"
callfs_head_method "${NODE1}/v1/files/chunked-test.bin"
assert_status "200"
assert_header_contains "X-CallFS-Size" "4096"
pass

test_name "GET download after chunked upload SHA-256 matches"
download_file_to_file "$NODE1" "/chunked-test.bin" "$CHUNKED_DOWN"
_read_status
assert_status "200"
DOWN_SHA=$(sha256_file "$CHUNKED_DOWN")
if [ "$INPUT_SHA" != "$DOWN_SHA" ]; then
  fail "SHA-256 mismatch: input=${INPUT_SHA} download=${DOWN_SHA}"
else
  pass
fi

# --- PUT with chunked encoding ---

test_name "PUT with chunked encoding updates file"
CHUNKED_UPDATE="${_TMPDIR}/chunked-update.bin"
CHUNKED_UPDATE_DOWN="${_TMPDIR}/chunked-update-down.bin"
generate_random_binary "$CHUNKED_UPDATE" 2048
UPDATE_SHA=$(sha256_file "$CHUNKED_UPDATE")
callfs_curl PUT "${NODE1}/v1/files/chunked-test.bin" \
  -H "Content-Type: application/octet-stream" \
  -H "Transfer-Encoding: chunked" \
  --data-binary "@${CHUNKED_UPDATE}" >/dev/null
_read_status
assert_status "200"
download_file_to_file "$NODE1" "/chunked-test.bin" "$CHUNKED_UPDATE_DOWN"
_read_status
assert_status "200"
DOWN_SHA2=$(sha256_file "$CHUNKED_UPDATE_DOWN")
if [ "$UPDATE_SHA" != "$DOWN_SHA2" ]; then
  fail "SHA-256 mismatch after PUT: expected=${UPDATE_SHA} got=${DOWN_SHA2}"
else
  pass
fi

# --- Large 2MB chunked ---

test_name "2MB chunked upload, download, SHA-256 match"
CHUNKED_LARGE="${_TMPDIR}/chunked-large.bin"
CHUNKED_LARGE_DOWN="${_TMPDIR}/chunked-large-down.bin"
generate_random_binary "$CHUNKED_LARGE" 2097152
LARGE_SHA=$(sha256_file "$CHUNKED_LARGE")
callfs_curl POST "${NODE1}/v1/files/chunked-large.bin" \
  -H "Content-Type: application/octet-stream" \
  -H "Transfer-Encoding: chunked" \
  --data-binary "@${CHUNKED_LARGE}" >/dev/null
_read_status
assert_status "201"
download_file_to_file "$NODE1" "/chunked-large.bin" "$CHUNKED_LARGE_DOWN"
_read_status
assert_status "200"
LARGE_DOWN_SHA=$(sha256_file "$CHUNKED_LARGE_DOWN")
if [ "$LARGE_SHA" != "$LARGE_DOWN_SHA" ]; then
  fail "SHA-256 mismatch: expected=${LARGE_SHA} got=${LARGE_DOWN_SHA}"
else
  pass
fi

# --- Cleanup ---

test_name "Cleanup chunked upload test files"
delete_file "$NODE1" "/chunked-test.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/chunked-large.bin" >/dev/null 2>&1 || true
pass

print_summary
exit $?
