#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Cross-Server Operations"

test_name "Upload /cross-test.txt on NODE1 returns 201"
upload_file "$NODE1" "/cross-test.txt" "created on node1"
assert_status 201
pass

test_name "HEAD /cross-test.txt on NODE2 returns 200 with X-CallFS-Instance-ID"
callfs_head_method "${NODE2}/v1/files/cross-test.txt"
assert_status 200
assert_header_present "X-CallFS-Instance-ID"
pass

test_name "GET /cross-test.txt on NODE2 returns body from NODE1"
BODY=$(download_file "$NODE2" "/cross-test.txt")
assert_status 200
assert_body_equals "$BODY" "created on node1"
pass

test_name "PUT /cross-test.txt on NODE2 with updated content returns 200"
BODY=$(callfs_curl PUT "${NODE2}/v1/files/cross-test.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "updated from node2")
assert_status 200
pass

test_name "GET /cross-test.txt on NODE1 returns updated content"
BODY=$(download_file "$NODE1" "/cross-test.txt")
assert_status 200
assert_body_equals "$BODY" "updated from node2"
pass

test_name "GET /cross-test.txt on NODE3 returns updated content"
BODY=$(download_file "$NODE3" "/cross-test.txt")
assert_status 200
assert_body_equals "$BODY" "updated from node2"
pass

test_name "DELETE /cross-test.txt on NODE2 returns 204"
delete_file "$NODE2" "/cross-test.txt"
assert_status 204
pass

test_name "GET /cross-test.txt on NODE1 returns 404 after delete"
BODY=$(download_file "$NODE1" "/cross-test.txt")
assert_status 404
pass

print_summary
exit $?
