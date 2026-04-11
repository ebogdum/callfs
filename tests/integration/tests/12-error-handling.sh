#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Error Handling"

test_name "GET /v1/files/nonexistent-file.txt returns 404 with JSON error"
BODY=$(callfs_curl_with_headers GET "${NODE1}/v1/files/nonexistent-file.txt")
assert_status 404
assert_body_contains "$BODY" "code"
assert_body_contains "$BODY" "message"
pass

test_name "POST /v1/files/test.txt to create file"
upload_file "$NODE1" "/test.txt" "error handling test"
assert_status 201
pass

test_name "POST /v1/files/test.txt again returns 409 (conflict)"
upload_file "$NODE1" "/test.txt" "duplicate"
assert_status 409
pass

test_name "PUT /v1/files/somedir/ (trailing slash) returns 400"
BODY=$(callfs_curl PUT "${NODE1}/v1/files/somedir/" \
  -H "Content-Type: application/octet-stream" \
  -d "bad request")
assert_status 400
pass

test_name "DELETE /v1/files/does-not-exist.txt returns 404"
delete_file "$NODE1" "/does-not-exist.txt"
assert_status 404
pass

test_name "GET /v1/directories/not-a-dir-path returns 404"
BODY=$(list_directory "$NODE1" "not-a-dir-path")
assert_status 404
pass

test_name "POST /v1/links/generate with empty body returns 400"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{}')
_read_status
assert_status "400"
pass

# Small delay to avoid rate limiter (100 req/s, burst 1)
sleep 1

test_name "POST /v1/links/generate with negative expiry returns 400"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/test.txt","expiry_seconds":-1}')
_read_status
assert_status "400"
pass

test_name "400 response has Content-Type: application/json header"
BODY=$(callfs_curl_with_headers PUT "${NODE1}/v1/files/somedir/" \
  -H "Content-Type: application/octet-stream" \
  -d "bad request")
assert_status 400
assert_header_contains "Content-Type" "application/json"
pass

test_name "401 response has Content-Type: application/json header"
BODY=$(callfs_curl_noauth GET "${NODE1}/v1/files/test.txt")
assert_status 401
# Re-fetch with headers captured
BODY=$(curl -s -w '\n%{http_code}' -D "$_HEADERS_FILE" \
  --max-time "$CURL_TIMEOUT" \
  -X GET \
  "${NODE1}/v1/files/test.txt" 2>/dev/null > "$_RESP_FILE" || true)
_read_headers
assert_header_contains "Content-Type" "application/json"
pass

test_name "404 response has Content-Type: application/json header"
BODY=$(callfs_curl_with_headers GET "${NODE1}/v1/files/nonexistent-err-check.txt")
assert_status 404
assert_header_contains "Content-Type" "application/json"
pass

test_name "409 response has Content-Type: application/json header"
BODY=$(callfs_curl_with_headers POST "${NODE1}/v1/files/test.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "duplicate again")
assert_status 409
assert_header_contains "Content-Type" "application/json"
pass

test_name "Cleanup error handling test files"
delete_file "$NODE1" "/test.txt" >/dev/null 2>&1 || true
pass

print_summary
exit $?
