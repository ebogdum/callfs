#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Directory Operations"

# Pre-cleanup: delete nested paths bottom-up to ensure test isolation
for p in /a/b/c/deep.txt /a/b/c /a/b /a /testdir/file1.txt /testdir/file2.txt /testdir; do
  callfs_curl DELETE "${NODE1}/v1/files${p}" >/dev/null 2>&1 || true
done

test_name "POST create directory /testdir/ returns 201"
create_directory "$NODE1" "/testdir/" >/dev/null
_read_status
assert_status "201"
pass

test_name "POST same dir again returns 200 (idempotent)"
create_directory "$NODE1" "/testdir/" >/dev/null
_read_status
assert_status "200"
pass

test_name "GET /v1/directories/testdir returns 200"
BODY=$(list_directory "$NODE1" "/testdir")
_read_status
assert_status "200"
pass

test_name "Upload files in testdir, list contains them"
upload_file "$NODE1" "/testdir/file1.txt" "file1 content" >/dev/null
_read_status
upload_file "$NODE1" "/testdir/file2.txt" "file2 content" >/dev/null
_read_status
BODY=$(list_directory "$NODE1" "/testdir")
_read_status
assert_status "200"
assert_body_contains "$BODY" "file1.txt"
assert_body_contains "$BODY" "file2.txt"
pass

test_name "Nested file /a/b/c/deep.txt auto-creates parents, 201"
upload_file "$NODE1" "/a/b/c/deep.txt" "deep content" >/dev/null
_read_status
assert_status "201"
# Verify the file exists via HEAD before testing listing
callfs_head_method "${NODE1}/v1/files/a/b/c/deep.txt"
assert_status "200"
pass

test_name "List /a recursive=true contains deep.txt"
BODY=$(list_directory "$NODE1" "/a" -G --data-urlencode "recursive=true")
_read_status
assert_status "200"
assert_body_contains "$BODY" "deep.txt"
pass

test_name "List /a recursive=true max_depth=1 has b but not deep.txt"
BODY=$(list_directory "$NODE1" "/a" -G --data-urlencode "recursive=true" --data-urlencode "max_depth=1")
_read_status
assert_status "200"
assert_body_contains "$BODY" "b"
if echo "$BODY" | grep -qF "deep.txt"; then
  fail "body should not contain deep.txt with max_depth=1"
else
  pass
fi

test_name "List /nonexistent returns 404"
list_directory "$NODE1" "/nonexistent" >/dev/null
_read_status
assert_status "404"
pass

test_name "HEAD on directory returns mode 0755"
callfs_head_method "${NODE1}/v1/files/testdir/"
assert_status "200"
assert_header_contains "X-CallFS-Mode" "0755"
pass

test_name "GET /v1/files/testdir/ returns JSON listing"
BODY=$(callfs_curl GET "${NODE1}/v1/files/testdir/")
_read_status
assert_status "200"
assert_body_contains "$BODY" "file1.txt"
pass

# Cleanup
delete_file "$NODE1" "/testdir/file1.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/testdir/file2.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/a/b/c/deep.txt" >/dev/null 2>&1 || true
callfs_curl DELETE "${NODE1}/v1/files/a" >/dev/null 2>&1 || true
callfs_curl DELETE "${NODE1}/v1/files/testdir" >/dev/null 2>&1 || true

print_summary
exit $?
