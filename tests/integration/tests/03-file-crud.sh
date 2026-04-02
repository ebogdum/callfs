#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "File CRUD Operations"

test_name "POST create /test.txt returns 201"
upload_file "$NODE1" "/test.txt" "Hello, CallFS!"
assert_status 201
pass

test_name "GET /test.txt returns 200 with correct body"
BODY=$(download_file "$NODE1" "/test.txt")
assert_status 200
assert_body_equals "$BODY" "Hello, CallFS!"
pass

test_name "HEAD /test.txt returns 200 with metadata headers"
callfs_head_method "${NODE1}/v1/files/test.txt"
assert_status 200
assert_header_present "X-CallFS-Size"
assert_header_present "X-CallFS-Mode"
assert_header_present "X-CallFS-UID"
assert_header_present "X-CallFS-GID"
assert_header_present "X-CallFS-MTime"
pass

test_name "PUT update /test.txt returns 200"
BODY=$(callfs_curl PUT "${NODE1}/v1/files/test.txt" -d "Updated content")
assert_status 200
pass

test_name "GET /test.txt returns updated content"
BODY=$(download_file "$NODE1" "/test.txt")
assert_status 200
assert_body_equals "$BODY" "Updated content"
pass

test_name "DELETE /test.txt returns 204"
delete_file "$NODE1" "/test.txt"
assert_status 204
pass

test_name "GET /test.txt after delete returns 404"
BODY=$(download_file "$NODE1" "/test.txt")
assert_status 404
pass

test_name "POST re-create /test.txt after delete returns 201"
upload_file "$NODE1" "/test.txt" "Hello, CallFS!"
assert_status 201
pass

test_name "Cleanup /test.txt"
delete_file "$NODE1" "/test.txt"
assert_status 204
pass

test_name "POST create zero-byte file /zero.txt returns 201"
upload_file "$NODE1" "/zero.txt" ""
assert_status 201
pass

test_name "GET /zero.txt returns 200 with empty body"
BODY=$(download_file "$NODE1" "/zero.txt")
assert_status 200
assert_body_equals "$BODY" ""
pass

test_name "Cleanup /zero.txt"
delete_file "$NODE1" "/zero.txt"
assert_status 204
pass

test_name "Upload and verify 1MB file"
TMPFILE=$(mktemp)
dd if=/dev/zero bs=1048576 count=1 of="$TMPFILE" 2>/dev/null
callfs_curl POST "${NODE1}/v1/files/largefile.bin" --data-binary "@${TMPFILE}" > /dev/null
assert_status 201
callfs_head_method "${NODE1}/v1/files/largefile.bin"
assert_status 200
assert_header_contains "X-CallFS-Size" "1048576"
rm -f "$TMPFILE"
pass

test_name "Cleanup /largefile.bin"
delete_file "$NODE1" "/largefile.bin"
assert_status 204
pass

print_summary
exit $?
