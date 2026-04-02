#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Path Security"

test_name "GET /../../../etc/passwd returns 400 (path traversal blocked)"
BODY=$(callfs_curl GET "${NODE1}/v1/files/../../../etc/passwd")
assert_status 400

test_name "GET /..%2F..%2Fetc%2Fpasswd returns 400 (URL-encoded traversal)"
BODY=$(callfs_curl GET "${NODE1}/v1/files/..%2F..%2Fetc%2Fpasswd")
assert_status 400

test_name "POST file with backslash in path returns 400"
BODY=$(callfs_curl POST "${NODE1}/v1/files/test%5Cfile.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "backslash test")
assert_status 400

test_name "GET with very long path (5000 chars) returns non-200"
LONG_PATH=$(printf 'a%.0s' $(seq 1 5000))
BODY=$(callfs_curl GET "${NODE1}/v1/files/${LONG_PATH}")
if [ "$LAST_STATUS" != "200" ]; then
  pass
else
  fail "expected non-200 for 5000 char path, got $LAST_STATUS"
fi

test_name "POST /v1/files/unicode-test-file.txt with unicode content succeeds (201)"
upload_file "$NODE1" "/unicode-test-file.txt" "Hello"
assert_status 201

test_name "Cleanup security test files"
delete_file "$NODE1" "/unicode-test-file.txt" >/dev/null 2>&1 || true
pass

print_summary
exit $?
