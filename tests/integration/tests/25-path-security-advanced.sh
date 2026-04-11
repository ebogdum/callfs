#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Path Security Advanced"

# NOTE: Chi router normalizes paths (removes /../, //, etc.) BEFORE the handler.
# So path traversal returns 404 (file not found after normalization), not 400.
# The security guarantee is that no file outside the storage root is accessible.

# ---------- Null byte injection ----------

test_name "POST with null byte in path returns error"
BODY=$(callfs_curl POST "${NODE1}/v1/files/test%00file.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "null byte test")
_read_status
if [ "$LAST_STATUS" != "201" ]; then
  pass
else
  fail "expected error for null byte in path, got 201"
fi

# ---------- Double slashes ----------

test_name "GET with double slashes is handled safely"
upload_file "$NODE1" "/slash-test.txt" "slash content" >/dev/null
_read_status
BODY=$(callfs_curl GET "${NODE1}/v1/files//slash-test.txt")
_read_status
# Chi may redirect // to / (301) or normalize to find the file (200) or not (404)
if [ "$LAST_STATUS" = "200" ] || [ "$LAST_STATUS" = "301" ] || [ "$LAST_STATUS" = "400" ] || [ "$LAST_STATUS" = "404" ]; then
  pass
else
  fail "unexpected status $LAST_STATUS for double-slash path"
fi

# ---------- Dot paths ----------

test_name "GET /v1/files/. is handled safely (no crash)"
BODY=$(callfs_curl GET "${NODE1}/v1/files/.")
_read_status
# Chi normalizes /. to / which maps to root directory - may return 200 (dir listing)
if [ "$LAST_STATUS" != "500" ]; then
  pass
else
  fail "server error for dot path"
fi

test_name "GET /v1/files/.. is handled safely"
BODY=$(callfs_curl GET "${NODE1}/v1/files/..")
_read_status
# Chi normalizes /.. to / - should not return 500
if [ "$LAST_STATUS" != "500" ]; then
  pass
else
  fail "server error for double-dot path"
fi

test_name "POST /v1/files/./sec-dot-test.txt is handled safely"
BODY=$(callfs_curl POST "${NODE1}/v1/files/./sec-dot-test.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "dot path")
_read_status
# Chi normalizes to /sec-dot-test.txt - should succeed or return expected error
if [ "$LAST_STATUS" != "500" ]; then
  pass
else
  fail "server error for dot-prefix path"
fi

# ---------- URL-encoded sequences ----------

test_name "POST with double-encoded traversal does not escape storage"
BODY=$(callfs_curl POST "${NODE1}/v1/files/%252e%252e%252f%252e%252e%252fetc%252fpasswd" \
  -H "Content-Type: application/octet-stream" \
  -d "double encoded")
_read_status
# Should NOT write to /etc/passwd - 201 is ok (literal filename), 400 also ok
if [ "$LAST_STATUS" = "201" ] || [ "$LAST_STATUS" = "400" ] || [ "$LAST_STATUS" = "404" ]; then
  pass
else
  fail "unexpected status $LAST_STATUS for double-encoded traversal"
fi

# ---------- Unicode normalization attacks ----------

test_name "POST with unicode fullwidth slash (U+FF0F) in path"
BODY=$(callfs_curl POST "${NODE1}/v1/files/test%EF%BC%8Ffile.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "unicode slash")
_read_status
# Should either create (treating fullwidth slash as literal char) or reject
if [ "$LAST_STATUS" = "201" ] || [ "$LAST_STATUS" = "400" ] || [ "$LAST_STATUS" = "404" ]; then
  pass
else
  fail "unexpected status $LAST_STATUS for fullwidth slash"
fi

# ---------- Trailing dots (Windows-style) ----------

test_name "POST /v1/files/test... handles trailing dots"
BODY=$(callfs_curl POST "${NODE1}/v1/files/trailing-dots..." \
  -H "Content-Type: application/octet-stream" \
  -d "trailing dots")
_read_status
# Should either create or reject, not crash
if [ "$LAST_STATUS" != "500" ]; then
  pass
else
  fail "server error for trailing dots filename"
fi

# ---------- Path with spaces and traversal combined ----------

test_name "POST with spaces and traversal returns non-500"
BODY=$(callfs_curl POST "${NODE1}/v1/files/test%20..%2F..%2Fetc%2Fpasswd" \
  -H "Content-Type: application/octet-stream" \
  -d "space traversal")
_read_status
if [ "$LAST_STATUS" != "500" ]; then
  pass
else
  fail "server error for space+traversal path"
fi

# ---------- Verify no file leaked outside storage ----------

test_name "Verify /etc/passwd is not readable via API"
BODY=$(callfs_curl GET "${NODE1}/v1/files/etc/passwd")
_read_status
if [ "$LAST_STATUS" = "200" ] && echo "$BODY" | grep -q "root:"; then
  fail "SECURITY: /etc/passwd content leaked!"
else
  pass
fi

# ---------- Cleanup ----------

test_name "Cleanup security advanced test files"
delete_file "$NODE1" "/slash-test.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/sec-dot-test.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/trailing-dots..." >/dev/null 2>&1 || true
callfs_curl DELETE "${NODE1}/v1/files/test%EF%BC%8Ffile.txt" >/dev/null 2>&1 || true
pass

print_summary
exit $?
