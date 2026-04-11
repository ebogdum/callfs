#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "File CRUD Operations"

# --- Create, HEAD, GET, directory listing ---

test_name "POST create /test.txt returns 201"
upload_file "$NODE1" "/test.txt" "Hello, CallFS!" >/dev/null
_read_status
assert_status "201"
pass

test_name "HEAD /test.txt verifies size and mode"
callfs_head_method "${NODE1}/v1/files/test.txt"
assert_status "200"
assert_header_contains "X-CallFS-Size" "14"
assert_header_contains "X-CallFS-Mode" "0644"
pass

test_name "GET /test.txt returns correct body"
BODY=$(download_file "$NODE1" "/test.txt")
_read_status
assert_status "200"
assert_body_equals "$BODY" "Hello, CallFS!"
pass

test_name "/test.txt appears in directory listing"
BODY=$(list_directory "$NODE1" "/")
_read_status
assert_status "200"
assert_body_contains "$BODY" "test.txt"
pass

# --- Update ---

test_name "PUT update /test.txt returns 200"
callfs_curl PUT "${NODE1}/v1/files/test.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "Updated content" >/dev/null
_read_status
assert_status "200"
pass

test_name "GET /test.txt returns updated content"
BODY=$(download_file "$NODE1" "/test.txt")
_read_status
assert_status "200"
assert_body_equals "$BODY" "Updated content"
pass

test_name "HEAD /test.txt shows new size after update"
callfs_head_method "${NODE1}/v1/files/test.txt"
assert_status "200"
assert_header_contains "X-CallFS-Size" "15"
pass

# --- Delete ---

test_name "DELETE /test.txt returns 204"
delete_file "$NODE1" "/test.txt" >/dev/null
_read_status
assert_status "204"
pass

test_name "GET /test.txt after delete returns 404"
download_file "$NODE1" "/test.txt" >/dev/null
_read_status
assert_status "404"
pass

test_name "/test.txt absent from directory listing after delete"
BODY=$(list_directory "$NODE1" "/")
_read_status
if echo "$BODY" | grep -qF "test.txt"; then
  fail "test.txt still in directory listing"
else
  pass
fi

# --- Re-create after delete ---

test_name "POST re-create /test.txt after delete returns 201"
upload_file "$NODE1" "/test.txt" "re-created" >/dev/null
_read_status
assert_status "201"
pass

delete_file "$NODE1" "/test.txt" >/dev/null 2>&1 || true

# --- Zero-byte file ---

test_name "POST create zero-byte file returns 201"
upload_file "$NODE1" "/zero.txt" "" >/dev/null
_read_status
assert_status "201"
pass

test_name "HEAD /zero.txt shows size 0"
callfs_head_method "${NODE1}/v1/files/zero.txt"
assert_status "200"
assert_header_contains "X-CallFS-Size" "0"
pass

test_name "GET /zero.txt returns empty body"
BODY=$(download_file "$NODE1" "/zero.txt")
_read_status
assert_status "200"
assert_body_equals "$BODY" ""
pass

delete_file "$NODE1" "/zero.txt" >/dev/null 2>&1 || true

# --- 1MB binary file ---

test_name "1MB binary upload, HEAD size, download SHA-256 match"
UPLOAD_FILE="${_TMPDIR}/1mb.bin"
DOWNLOAD_FILE="${_TMPDIR}/1mb-down.bin"
generate_random_binary "$UPLOAD_FILE" 1048576
upload_file_binary "$NODE1" "/large.bin" "$UPLOAD_FILE" >/dev/null
_read_status
assert_status "201"
callfs_head_method "${NODE1}/v1/files/large.bin"
assert_status "200"
assert_header_contains "X-CallFS-Size" "1048576"
download_file_to_file "$NODE1" "/large.bin" "$DOWNLOAD_FILE"
_read_status
assert_status "200"
assert_sha256_match "$UPLOAD_FILE" "$DOWNLOAD_FILE"
pass

delete_file "$NODE1" "/large.bin" >/dev/null 2>&1 || true

# --- 4KB binary ---

test_name "4KB random binary upload/download SHA-256 match"
UPLOAD_4K="${_TMPDIR}/4kb.bin"
DOWNLOAD_4K="${_TMPDIR}/4kb-down.bin"
generate_random_binary "$UPLOAD_4K" 4096
upload_file_binary "$NODE1" "/small-bin.dat" "$UPLOAD_4K" >/dev/null
_read_status
assert_status "201"
download_file_to_file "$NODE1" "/small-bin.dat" "$DOWNLOAD_4K"
_read_status
assert_status "200"
assert_sha256_match "$UPLOAD_4K" "$DOWNLOAD_4K"
pass

delete_file "$NODE1" "/small-bin.dat" >/dev/null 2>&1 || true

# --- Duplicate returns 409 ---

test_name "POST duplicate returns 409, original content unchanged"
upload_file "$NODE1" "/dup-test.txt" "original" >/dev/null
_read_status
assert_status "201"
upload_file "$NODE1" "/dup-test.txt" "duplicate" >/dev/null
_read_status
assert_status "409"
BODY=$(download_file "$NODE1" "/dup-test.txt")
_read_status
assert_status "200"
assert_body_equals "$BODY" "original"
pass

delete_file "$NODE1" "/dup-test.txt" >/dev/null 2>&1 || true

# --- GET on directory path returns JSON listing ---

test_name "GET /v1/files/dir-test/ on directory returns JSON listing"
create_directory "$NODE1" "/dir-test/" >/dev/null
_read_status
upload_file "$NODE1" "/dir-test/inner.txt" "inner" >/dev/null
_read_status
BODY=$(callfs_curl GET "${NODE1}/v1/files/dir-test/")
_read_status
assert_status "200"
assert_body_contains "$BODY" "inner.txt"
pass

# Cleanup
delete_file "$NODE1" "/dir-test/inner.txt" >/dev/null 2>&1 || true
callfs_curl DELETE "${NODE1}/v1/files/dir-test" >/dev/null 2>&1 || true

print_summary
exit $?
