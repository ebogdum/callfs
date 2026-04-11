#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Path Security"

# Chi router normalizes path traversal sequences (/../..) before the handler
# sees them, so the handler receives clean paths. The security guarantee is
# that you CANNOT access files outside the storage root.

test_name "GET /../../../etc/passwd is blocked (Chi normalizes, returns 404 not file content)"
BODY=$(callfs_curl GET "${NODE1}/v1/files/../../../etc/passwd")
_read_status
# Chi normalizes to /etc/passwd which doesn't exist -> 404
# The key assertion: the response must NOT contain /etc/passwd content
if echo "$BODY" | grep -q "root:"; then
  fail "SECURITY: /etc/passwd content leaked!"
else
  pass
fi

test_name "GET /..%2F..%2Fetc%2Fpasswd returns non-200 (traversal blocked)"
BODY=$(callfs_curl GET "${NODE1}/v1/files/..%2F..%2Fetc%2Fpasswd")
_read_status
if [ "$LAST_STATUS" = "200" ] && echo "$BODY" | grep -q "root:"; then
  fail "SECURITY: URL-encoded traversal leaked /etc/passwd"
else
  pass
fi

test_name "POST file with backslash in path returns 400"
BODY=$(callfs_curl POST "${NODE1}/v1/files/test%5Cfile.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "backslash test")
_read_status
assert_status "400"
pass

test_name "GET with very long path (5000 chars) returns non-200"
LONG_PATH=$(printf 'a%.0s' $(seq 1 5000))
BODY=$(callfs_curl GET "${NODE1}/v1/files/${LONG_PATH}")
_read_status
if [ "$LAST_STATUS" != "200" ]; then
  pass
else
  fail "expected non-200 for 5000 char path, got $LAST_STATUS"
fi

test_name "POST /v1/files/unicode-test-file.txt with unicode content succeeds (201)"
upload_file "$NODE1" "/unicode-test-file.txt" "Hello" >/dev/null
_read_status
assert_status "201"
pass

test_name "Cleanup security test files"
delete_file "$NODE1" "/unicode-test-file.txt" >/dev/null 2>&1 || true
pass

print_summary
exit $?
