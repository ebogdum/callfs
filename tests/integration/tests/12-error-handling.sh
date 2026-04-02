#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Error Handling"

test_name "GET /v1/files/nonexistent-file.txt returns 404 with JSON error"
BODY=$(callfs_curl GET "${NODE1}/v1/files/nonexistent-file.txt")
assert_status 404

test_name "404 response body contains 'code'"
assert_body_contains "$BODY" "code"

test_name "404 response body contains 'message'"
assert_body_contains "$BODY" "message"

test_name "POST /v1/files/test.txt to create file"
upload_file "$NODE1" "/test.txt" "error handling test"
assert_status 201

test_name "POST /v1/files/test.txt again returns 409 (conflict)"
upload_file "$NODE1" "/test.txt" "duplicate"
assert_status 409

test_name "PUT /v1/files/somedir/ (trailing slash) returns 400"
BODY=$(callfs_curl PUT "${NODE1}/v1/files/somedir/" \
  -H "Content-Type: application/octet-stream" \
  -d "bad request")
assert_status 400

test_name "DELETE /v1/files/does-not-exist.txt returns 404"
delete_file "$NODE1" "/does-not-exist.txt"
assert_status 404

test_name "GET /v1/directories/not-a-dir-path returns 404"
BODY=$(list_directory "$NODE1" "not-a-dir-path")
assert_status 404

test_name "POST /v1/links/generate with empty body returns 400"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{}')
assert_status 400

test_name "POST /v1/links/generate with negative expiry returns 400"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/test.txt","expiry_seconds":-1}')
assert_status 400

test_name "Cleanup error handling test files"
delete_file "$NODE1" "/test.txt" >/dev/null 2>&1 || true
pass

print_summary
exit $?
