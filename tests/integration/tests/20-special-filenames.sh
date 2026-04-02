#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Special Filenames"

# ---------- Filenames with spaces ----------

test_name "POST file with space in name succeeds"
BODY=$(callfs_curl POST "${NODE1}/v1/files/file%20with%20spaces.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "spaces content")
assert_status 201
pass

test_name "GET file with space in name returns correct content"
BODY=$(callfs_curl GET "${NODE1}/v1/files/file%20with%20spaces.txt")
assert_status 200
assert_body_equals "$BODY" "spaces content"
pass

# ---------- Filenames with dots ----------

test_name "POST hidden file (.hidden) succeeds"
upload_file "$NODE1" "/.hidden-file" "hidden content"
assert_status 201
pass

test_name "GET hidden file returns correct content"
BODY=$(download_file "$NODE1" "/.hidden-file")
assert_status 200
assert_body_equals "$BODY" "hidden content"
pass

test_name "POST file with multiple dots (archive.tar.gz)"
upload_file "$NODE1" "/archive.tar.gz" "tarball content"
assert_status 201
pass

test_name "GET file with multiple dots returns correct content"
BODY=$(download_file "$NODE1" "/archive.tar.gz")
assert_status 200
assert_body_equals "$BODY" "tarball content"
pass

# ---------- Filenames with dashes and underscores ----------

test_name "POST file with dashes and underscores"
upload_file "$NODE1" "/my-file_v2_final.txt" "dashes and underscores"
assert_status 201
pass

test_name "GET file with dashes and underscores"
BODY=$(download_file "$NODE1" "/my-file_v2_final.txt")
assert_status 200
assert_body_equals "$BODY" "dashes and underscores"
pass

# ---------- Filenames with special URL characters ----------

test_name "POST file with plus sign in name"
BODY=$(callfs_curl POST "${NODE1}/v1/files/file%2Bplus.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "plus content")
# Should succeed or return 400 if not allowed
if [ "$LAST_STATUS" = "201" ] || [ "$LAST_STATUS" = "400" ]; then
  pass
else
  fail "unexpected status $LAST_STATUS for plus-sign filename"
fi

test_name "POST file with @ sign in name"
BODY=$(callfs_curl POST "${NODE1}/v1/files/file%40at.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "at content")
if [ "$LAST_STATUS" = "201" ] || [ "$LAST_STATUS" = "400" ]; then
  pass
else
  fail "unexpected status $LAST_STATUS for @-sign filename"
fi

# ---------- Deeply nested path with mixed names ----------

test_name "Create deeply nested path with various name styles"
upload_file "$NODE1" "/level-1/level_2/level.3/file.txt" "nested mixed"
assert_status 201
pass

test_name "GET nested file returns correct content"
BODY=$(download_file "$NODE1" "/level-1/level_2/level.3/file.txt")
assert_status 200
assert_body_equals "$BODY" "nested mixed"
pass

# ---------- Files with no extension ----------

test_name "POST file with no extension"
upload_file "$NODE1" "/Makefile" "makefile content"
assert_status 201
pass

test_name "GET file with no extension"
BODY=$(download_file "$NODE1" "/Makefile")
assert_status 200
assert_body_equals "$BODY" "makefile content"
pass

# ---------- Very short filename ----------

test_name "POST single-char filename"
upload_file "$NODE1" "/x" "x content"
assert_status 201
pass

test_name "GET single-char filename"
BODY=$(download_file "$NODE1" "/x")
assert_status 200
assert_body_equals "$BODY" "x content"
pass

# ---------- Cleanup ----------

test_name "Cleanup special filename test files"
callfs_curl DELETE "${NODE1}/v1/files/file%20with%20spaces.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/.hidden-file" >/dev/null 2>&1 || true
delete_file "$NODE1" "/archive.tar.gz" >/dev/null 2>&1 || true
delete_file "$NODE1" "/my-file_v2_final.txt" >/dev/null 2>&1 || true
callfs_curl DELETE "${NODE1}/v1/files/file%2Bplus.txt" >/dev/null 2>&1 || true
callfs_curl DELETE "${NODE1}/v1/files/file%40at.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/level-1/level_2/level.3/file.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/Makefile" >/dev/null 2>&1 || true
delete_file "$NODE1" "/x" >/dev/null 2>&1 || true
pass

print_summary
exit $?
