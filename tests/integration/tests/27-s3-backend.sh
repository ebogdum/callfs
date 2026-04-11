#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "S3/MinIO Backend"

# --- Health ---

test_name "S3 node health check returns 200"
BODY=$(callfs_curl_noauth GET "${NODE_S3}/health")
_read_status
assert_status "200"
assert_body_contains "$BODY" '"status":"ok"'
pass

# --- Text CRUD ---

test_name "POST text file to S3 backend returns 201"
upload_file "$NODE_S3" "/s3-test.txt" "hello s3 backend" >/dev/null
_read_status
assert_status "201"
pass

test_name "GET text file from S3 backend returns correct body"
BODY=$(download_file "$NODE_S3" "/s3-test.txt")
_read_status
assert_status "200"
assert_body_equals "$BODY" "hello s3 backend"
pass

test_name "HEAD returns metadata headers on S3 backend"
callfs_head_method "${NODE_S3}/v1/files/s3-test.txt"
assert_status "200"
assert_header_present "X-CallFS-Size"
assert_header_present "X-CallFS-Mode"
assert_header_present "X-CallFS-Owner"
assert_header_present "X-CallFS-MTime"
pass

test_name "PUT update on S3 backend returns 200"
callfs_curl PUT "${NODE_S3}/v1/files/s3-test.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "updated s3 content" >/dev/null
_read_status
assert_status "200"
pass

test_name "GET returns updated content on S3 backend"
BODY=$(download_file "$NODE_S3" "/s3-test.txt")
_read_status
assert_status "200"
assert_body_equals "$BODY" "updated s3 content"
pass

test_name "DELETE on S3 backend returns 204"
delete_file "$NODE_S3" "/s3-test.txt" >/dev/null
_read_status
assert_status "204"
pass

test_name "GET after delete on S3 backend returns 404"
download_file "$NODE_S3" "/s3-test.txt" >/dev/null
_read_status
assert_status "404"
pass

# --- Binary 4KB ---

test_name "4KB binary upload/download SHA-256 match on S3 backend"
S3_UP_4K="${_TMPDIR}/s3-4kb.bin"
S3_DOWN_4K="${_TMPDIR}/s3-4kb-down.bin"
generate_random_binary "$S3_UP_4K" 4096
upload_file_binary "$NODE_S3" "/s3-bin-4k.bin" "$S3_UP_4K" >/dev/null
_read_status
assert_status "201"
download_file_to_file "$NODE_S3" "/s3-bin-4k.bin" "$S3_DOWN_4K"
_read_status
assert_status "200"
assert_sha256_match "$S3_UP_4K" "$S3_DOWN_4K"
pass

# --- Large 5MB ---

test_name "5MB binary upload, HEAD size, download SHA-256 match on S3 backend"
S3_UP_5M="${_TMPDIR}/s3-5mb.bin"
S3_DOWN_5M="${_TMPDIR}/s3-5mb-down.bin"
generate_random_binary "$S3_UP_5M" 5242880
upload_file_binary "$NODE_S3" "/s3-bin-5m.bin" "$S3_UP_5M" >/dev/null
_read_status
assert_status "201"
callfs_head_method "${NODE_S3}/v1/files/s3-bin-5m.bin"
assert_status "200"
assert_header_contains "X-CallFS-Size" "5242880"
download_file_to_file "$NODE_S3" "/s3-bin-5m.bin" "$S3_DOWN_5M"
_read_status
assert_status "200"
assert_sha256_match "$S3_UP_5M" "$S3_DOWN_5M"
pass

# --- Directory operations ---

test_name "Create directory on S3 backend"
create_directory "$NODE_S3" "/s3-dir/" >/dev/null
_read_status
assert_status "201"
pass

test_name "Upload file into directory on S3 backend"
upload_file "$NODE_S3" "/s3-dir/inner.txt" "inside s3 dir" >/dev/null
_read_status
assert_status "201"
pass

test_name "List directory contains uploaded file on S3 backend"
BODY=$(list_directory "$NODE_S3" "/s3-dir/")
_read_status
assert_status "200"
assert_body_contains "$BODY" "inner.txt"
pass

test_name "Recursive directory listing works on S3 backend"
BODY=$(list_directory "$NODE_S3" "/" -G -d "recursive=true")
_read_status
assert_status "200"
assert_body_contains "$BODY" "s3-dir"
pass

# --- Single-use link ---

test_name "Single-use link works on S3 backend"
upload_file "$NODE_S3" "/s3-link-test.txt" "s3 link data" >/dev/null
_read_status
assert_status "201"
BODY=$(callfs_curl POST "${NODE_S3}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/s3-link-test.txt","expiry_seconds":3600}')
_read_status
assert_status "201"
TOKEN=$(echo "$BODY" | jq -r '.token')
BODY=$(callfs_curl_noauth GET "${NODE_S3}/download/${TOKEN}")
_read_status
assert_status "200"
assert_body_equals "$BODY" "s3 link data"
pass

test_name "Reusing single-use link returns 410 on S3 backend"
BODY=$(callfs_curl_noauth GET "${NODE_S3}/download/${TOKEN}")
_read_status
assert_status "410"
pass

# --- Zero-byte file ---

test_name "Zero-byte file on S3 backend: POST returns 201"
upload_file "$NODE_S3" "/s3-zero.txt" "" >/dev/null
_read_status
assert_status "201"
pass

test_name "Zero-byte file on S3 backend: HEAD shows size 0"
callfs_head_method "${NODE_S3}/v1/files/s3-zero.txt"
assert_status "200"
assert_header_contains "X-CallFS-Size" "0"
pass

test_name "Zero-byte file on S3 backend: GET returns empty body"
BODY=$(download_file "$NODE_S3" "/s3-zero.txt")
_read_status
assert_status "200"
assert_body_equals "$BODY" ""
pass

# --- Duplicate returns 409 ---

test_name "POST duplicate returns 409 on S3 backend"
upload_file "$NODE_S3" "/s3-dup.txt" "original" >/dev/null
_read_status
assert_status "201"
upload_file "$NODE_S3" "/s3-dup.txt" "duplicate" >/dev/null
_read_status
assert_status "409"
pass

# --- PUT creates new file ---

test_name "PUT creates new file returns 201 on S3 backend"
callfs_curl PUT "${NODE_S3}/v1/files/s3-put-new.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "put-created" >/dev/null
_read_status
assert_status "201"
BODY=$(download_file "$NODE_S3" "/s3-put-new.txt")
_read_status
assert_status "200"
assert_body_equals "$BODY" "put-created"
pass

# --- File and directory modes ---

test_name "File mode is 0644 on S3 backend"
callfs_head_method "${NODE_S3}/v1/files/s3-put-new.txt"
assert_status "200"
assert_header_contains "X-CallFS-Mode" "0644"
pass

test_name "Directory mode is 0755 on S3 backend"
callfs_head_method "${NODE_S3}/v1/files/s3-dir/"
assert_status "200"
assert_header_contains "X-CallFS-Mode" "0755"
pass

# --- Cleanup ---

test_name "Cleanup S3 backend test files"
delete_file "$NODE_S3" "/s3-bin-4k.bin" >/dev/null 2>&1 || true
delete_file "$NODE_S3" "/s3-bin-5m.bin" >/dev/null 2>&1 || true
delete_file "$NODE_S3" "/s3-dir/inner.txt" >/dev/null 2>&1 || true
callfs_curl DELETE "${NODE_S3}/v1/files/s3-dir" >/dev/null 2>&1 || true
delete_file "$NODE_S3" "/s3-link-test.txt" >/dev/null 2>&1 || true
delete_file "$NODE_S3" "/s3-zero.txt" >/dev/null 2>&1 || true
delete_file "$NODE_S3" "/s3-dup.txt" >/dev/null 2>&1 || true
delete_file "$NODE_S3" "/s3-put-new.txt" >/dev/null 2>&1 || true
pass

print_summary
exit $?
