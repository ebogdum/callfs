#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Cross-Server Operations"

test_name "Upload on NODE1, HEAD from NODE2 with X-CallFS-Instance-ID"
upload_file "$NODE1" "/cross-test.txt" "created on node1" >/dev/null
_read_status
assert_status "201"
callfs_head_method "${NODE2}/v1/files/cross-test.txt"
assert_status "200"
assert_header_present "X-CallFS-Instance-ID"
pass

test_name "GET from NODE2 returns correct body"
BODY=$(download_file "$NODE2" "/cross-test.txt")
_read_status
assert_status "200"
assert_body_equals "$BODY" "created on node1"
pass

test_name "PUT from NODE2, GET from NODE1 shows updated content"
callfs_curl PUT "${NODE2}/v1/files/cross-test.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "updated from node2" >/dev/null
_read_status
assert_status "200"
BODY=$(download_file "$NODE1" "/cross-test.txt")
_read_status
assert_status "200"
assert_body_equals "$BODY" "updated from node2"
pass

test_name "GET from NODE3 shows updated content"
BODY=$(download_file "$NODE3" "/cross-test.txt")
_read_status
assert_status "200"
assert_body_equals "$BODY" "updated from node2"
pass

test_name "DELETE from NODE2, GET from NODE1 returns 404"
delete_file "$NODE2" "/cross-test.txt" >/dev/null
_read_status
assert_status "204"
download_file "$NODE1" "/cross-test.txt" >/dev/null
_read_status
assert_status "404"
pass

test_name "8KB binary cross-server round-trip with SHA-256"
UPLOAD_8K="${_TMPDIR}/cross-8kb.bin"
DOWNLOAD_8K="${_TMPDIR}/cross-8kb-down.bin"
generate_random_binary "$UPLOAD_8K" 8192
upload_file_binary "$NODE1" "/cross-bin.dat" "$UPLOAD_8K" >/dev/null
_read_status
assert_status "201"
download_file_to_file "$NODE2" "/cross-bin.dat" "$DOWNLOAD_8K"
_read_status
assert_status "200"
assert_sha256_match "$UPLOAD_8K" "$DOWNLOAD_8K"
pass

# Cleanup
delete_file "$NODE1" "/cross-bin.dat" >/dev/null 2>&1 || true

print_summary
exit $?
