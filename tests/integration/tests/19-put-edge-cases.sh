#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "PUT Edge Cases"

# ---------- PUT creates new file ----------

test_name "PUT to non-existent file creates it (returns 201)"
BODY=$(callfs_curl PUT "${NODE1}/v1/files/put-create.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "created via PUT")
assert_status 201
pass

test_name "GET confirms PUT-created file has correct content"
BODY=$(download_file "$NODE1" "/put-create.txt")
assert_status 200
assert_body_equals "$BODY" "created via PUT"
pass

# ---------- PUT updates existing file ----------

test_name "PUT overwrites existing file (returns 200)"
BODY=$(callfs_curl PUT "${NODE1}/v1/files/put-create.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "updated via PUT")
assert_status 200
pass

test_name "GET confirms updated content"
BODY=$(download_file "$NODE1" "/put-create.txt")
assert_status 200
assert_body_equals "$BODY" "updated via PUT"
pass

# ---------- PUT with different sizes ----------

test_name "PUT with larger content updates size metadata"
BODY=$(callfs_curl PUT "${NODE1}/v1/files/put-create.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "this is a much longer content string for testing size metadata update")
assert_status 200
callfs_head_method "${NODE1}/v1/files/put-create.txt"
assert_header_contains "X-CallFS-Size" "68"
pass

test_name "PUT with smaller content updates size metadata"
BODY=$(callfs_curl PUT "${NODE1}/v1/files/put-create.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "tiny")
assert_status 200
callfs_head_method "${NODE1}/v1/files/put-create.txt"
assert_header_contains "X-CallFS-Size" "4"
pass

# ---------- PUT with empty body ----------

test_name "PUT with empty body creates zero-byte file"
BODY=$(callfs_curl PUT "${NODE1}/v1/files/put-empty.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "")
# Should succeed (either 200 or 201)
if [ "$LAST_STATUS" = "200" ] || [ "$LAST_STATUS" = "201" ]; then
  pass
else
  fail "expected 200 or 201, got $LAST_STATUS"
fi

# ---------- PUT on directory path ----------

test_name "PUT with trailing slash (directory path) returns 400"
BODY=$(callfs_curl PUT "${NODE1}/v1/files/somedir/" \
  -H "Content-Type: application/octet-stream" \
  -d "should fail")
assert_status 400
pass

# ---------- PUT where directory exists ----------

test_name "Setup: create directory /put-dir-conflict/"
create_directory "$NODE1" "/put-dir-conflict/"
assert_status 201
pass

test_name "PUT on existing directory path returns 400"
BODY=$(callfs_curl PUT "${NODE1}/v1/files/put-dir-conflict" \
  -H "Content-Type: application/octet-stream" \
  -d "should fail")
assert_status 400
pass

# ---------- HEAD on non-existent file ----------

test_name "HEAD on non-existent file returns 404"
callfs_head_method "${NODE1}/v1/files/does-not-exist-head-test.txt"
assert_status 404
pass

# ---------- Cross-server PUT ----------

test_name "PUT on NODE1, verify from NODE2"
BODY=$(callfs_curl PUT "${NODE1}/v1/files/cross-put.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "cross-server put")
# 201 for new file
if [ "$LAST_STATUS" = "200" ] || [ "$LAST_STATUS" = "201" ]; then
  BODY=$(download_file "$NODE2" "/cross-put.txt")
  assert_status 200
  assert_body_equals "$BODY" "cross-server put"
  pass
else
  fail "PUT returned $LAST_STATUS"
fi

# ---------- Cleanup ----------

test_name "Cleanup PUT edge case files"
delete_file "$NODE1" "/put-create.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/put-empty.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/put-dir-conflict" >/dev/null 2>&1 || true
delete_file "$NODE1" "/cross-put.txt" >/dev/null 2>&1 || true
pass

print_summary
exit $?
