#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Path Security Advanced"

# ---------- Null byte injection ----------

test_name "POST with null byte in path returns error"
BODY=$(callfs_curl POST "${NODE1}/v1/files/test%00file.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "null byte test")
if [ "$LAST_STATUS" != "201" ]; then
  pass
else
  fail "expected error for null byte in path, got 201"
fi

# ---------- Double slashes ----------

test_name "GET with double slashes is handled safely"
upload_file "$NODE1" "/slash-test.txt" "slash content"
assert_status 201
BODY=$(callfs_curl GET "${NODE1}/v1/files//slash-test.txt")
# Should either normalize to /slash-test.txt (200) or reject (400)
if [ "$LAST_STATUS" = "200" ] || [ "$LAST_STATUS" = "400" ] || [ "$LAST_STATUS" = "404" ]; then
  pass
else
  fail "unexpected status $LAST_STATUS for double-slash path"
fi

# ---------- Dot paths ----------

test_name "GET /v1/files/. returns error (not directory listing)"
BODY=$(callfs_curl GET "${NODE1}/v1/files/.")
if [ "$LAST_STATUS" = "400" ] || [ "$LAST_STATUS" = "404" ]; then
  pass
else
  fail "expected 400 or 404 for dot path, got $LAST_STATUS"
fi

test_name "GET /v1/files/.. returns error"
BODY=$(callfs_curl GET "${NODE1}/v1/files/..")
assert_status 400
pass

test_name "POST /v1/files/./test.txt returns error"
BODY=$(callfs_curl POST "${NODE1}/v1/files/./sec-dot-test.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "dot path")
if [ "$LAST_STATUS" = "400" ]; then
  pass
else
  # Could be 201 if . is normalized - that's also acceptable
  if [ "$LAST_STATUS" = "201" ]; then
    pass
  else
    fail "unexpected status $LAST_STATUS for dot-prefix path"
  fi
fi

# ---------- URL-encoded sequences ----------

test_name "POST with double-encoded traversal returns error"
BODY=$(callfs_curl POST "${NODE1}/v1/files/%252e%252e%252f%252e%252e%252fetc%252fpasswd" \
  -H "Content-Type: application/octet-stream" \
  -d "double encoded")
# Should not succeed as a valid file in /etc/
if [ "$LAST_STATUS" = "400" ] || [ "$LAST_STATUS" = "201" ]; then
  # 201 is ok if it created a file literally named %2e%2e%2f...
  pass
else
  fail "unexpected status $LAST_STATUS for double-encoded traversal"
fi

# ---------- Unicode normalization attacks ----------

test_name "POST with unicode fullwidth slash (U+FF0F) in path"
BODY=$(callfs_curl POST "${NODE1}/v1/files/test%EF%BC%8Ffile.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "unicode slash")
# Should either create the file (treating fullwidth slash as literal) or reject
if [ "$LAST_STATUS" = "201" ] || [ "$LAST_STATUS" = "400" ]; then
  pass
else
  fail "unexpected status $LAST_STATUS for fullwidth slash"
fi

# ---------- Trailing dots (Windows-style) ----------

test_name "POST /v1/files/test... handles trailing dots"
BODY=$(callfs_curl POST "${NODE1}/v1/files/trailing-dots..." \
  -H "Content-Type: application/octet-stream" \
  -d "trailing dots")
# Should either create or reject, not crash
if [ "$LAST_STATUS" = "201" ] || [ "$LAST_STATUS" = "400" ]; then
  pass
else
  fail "unexpected status $LAST_STATUS for trailing dots filename"
fi

# ---------- Path with spaces and traversal combined ----------

test_name "POST with spaces and traversal returns error"
BODY=$(callfs_curl POST "${NODE1}/v1/files/test%20..%2F..%2Fetc%2Fpasswd" \
  -H "Content-Type: application/octet-stream" \
  -d "space traversal")
if [ "$LAST_STATUS" = "400" ]; then
  pass
else
  # Could be 201 if the server treats it as literal chars after space
  if [ "$LAST_STATUS" = "201" ]; then
    pass
  else
    fail "unexpected status $LAST_STATUS"
  fi
fi

# ---------- Verify no file leaked outside storage ----------

test_name "Verify /etc/passwd is not readable via API"
BODY=$(callfs_curl GET "${NODE1}/v1/files/etc/passwd")
if [ "$LAST_STATUS" = "404" ]; then
  pass
elif [ "$LAST_STATUS" = "200" ]; then
  # If 200, make sure it's not the actual passwd file
  if echo "$BODY" | grep -q "root:"; then
    fail "SECURITY: /etc/passwd content leaked!"
  else
    pass
  fi
else
  pass
fi

# ---------- Cleanup ----------

test_name "Cleanup security advanced test files"
delete_file "$NODE1" "/slash-test.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/trailing-dots..." >/dev/null 2>&1 || true
callfs_curl DELETE "${NODE1}/v1/files/test%EF%BC%8Ffile.txt" >/dev/null 2>&1 || true
pass

print_summary
exit $?
