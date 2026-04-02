#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Security Headers & Response Headers"

# ---------- Security headers on API responses ----------

test_name "Setup: create test file"
upload_file "$NODE1" "/headers-test.txt" "header test content"
assert_status 201
pass

test_name "X-Content-Type-Options: nosniff is present"
callfs_head "${NODE1}/v1/files/headers-test.txt"
assert_header_contains "X-Content-Type-Options" "nosniff"
pass

test_name "X-Frame-Options: DENY is present"
callfs_head "${NODE1}/v1/files/headers-test.txt"
assert_header_contains "X-Frame-Options" "DENY"
pass

test_name "Referrer-Policy header is present"
callfs_head "${NODE1}/v1/files/headers-test.txt"
assert_header_present "Referrer-Policy"
pass

test_name "Content-Security-Policy header is present"
callfs_head "${NODE1}/v1/files/headers-test.txt"
assert_header_present "Content-Security-Policy"
pass

test_name "Permissions-Policy header is present"
callfs_head "${NODE1}/v1/files/headers-test.txt"
assert_header_present "Permissions-Policy"
pass

test_name "Cross-Origin-Opener-Policy header is present"
callfs_head "${NODE1}/v1/files/headers-test.txt"
assert_header_present "Cross-Origin-Opener-Policy"
pass

# ---------- Content headers ----------

test_name "GET file returns Content-Length header"
callfs_head "${NODE1}/v1/files/headers-test.txt"
assert_header_present "Content-Length"
pass

test_name "Content-Length matches file size"
callfs_head_method "${NODE1}/v1/files/headers-test.txt"
assert_header_contains "X-CallFS-Size" "18"
pass

test_name "GET file returns Content-Type header"
callfs_head "${NODE1}/v1/files/headers-test.txt"
assert_header_present "Content-Type"
pass

# ---------- X-Request-ID ----------

test_name "Response includes X-Request-ID header"
callfs_head "${NODE1}/v1/files/headers-test.txt"
assert_header_present "X-Request-ID"
pass

test_name "Health endpoint includes security headers"
curl -s -D "$_HEADERS_FILE" -o /dev/null \
  --max-time "$CURL_TIMEOUT" \
  "${NODE1}/health" 2>/dev/null || true
_read_headers
assert_header_contains "X-Content-Type-Options" "nosniff"
pass

# ---------- HEAD method returns same headers as GET ----------

test_name "HEAD returns X-CallFS metadata headers"
callfs_head_method "${NODE1}/v1/files/headers-test.txt"
assert_status 200
assert_header_present "X-CallFS-Size"
assert_header_present "X-CallFS-Mode"
assert_header_present "X-CallFS-UID"
assert_header_present "X-CallFS-GID"
assert_header_present "X-CallFS-MTime"
pass

# ---------- Security headers on error responses ----------

test_name "404 response includes security headers"
callfs_head "${NODE1}/v1/files/nonexistent-sec-test.txt"
assert_header_contains "X-Content-Type-Options" "nosniff"
pass

# ---------- Cleanup ----------

test_name "Cleanup security header test files"
delete_file "$NODE1" "/headers-test.txt" >/dev/null 2>&1 || true
pass

print_summary
exit $?
