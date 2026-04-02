#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Directory Operations"

test_name "POST create directory /testdir/ returns 201"
create_directory "$NODE1" "/testdir/"
assert_status 201
pass

test_name "POST create same directory /testdir/ again returns 200"
create_directory "$NODE1" "/testdir/"
assert_status 200
pass

test_name "GET /v1/directories/testdir returns 200"
BODY=$(list_directory "$NODE1" "/testdir")
assert_status 200
pass

test_name "Upload /testdir/file1.txt"
upload_file "$NODE1" "/testdir/file1.txt" "file1 content"
assert_status 201
pass

test_name "Upload /testdir/file2.txt"
upload_file "$NODE1" "/testdir/file2.txt" "file2 content"
assert_status 201
pass

test_name "List /testdir contains file1.txt and file2.txt"
BODY=$(list_directory "$NODE1" "/testdir")
assert_status 200
assert_body_contains "$BODY" "file1.txt"
assert_body_contains "$BODY" "file2.txt"
pass

test_name "Create nested file /a/b/c/deep.txt with auto-created parents"
upload_file "$NODE1" "/a/b/c/deep.txt" "deep content"
assert_status 201
pass

test_name "List /a recursive contains deep.txt"
BODY=$(list_directory "$NODE1" "/a" --data-urlencode "recursive=true")
assert_status 200
assert_body_contains "$BODY" "deep.txt"
pass

test_name "List /a recursive max_depth=1 contains b but not deep.txt"
BODY=$(list_directory "$NODE1" "/a" --data-urlencode "recursive=true" --data-urlencode "max_depth=1")
assert_status 200
assert_body_contains "$BODY" "b"
if echo "$BODY" | grep -q "deep.txt"; then
    fail "body should not contain deep.txt with max_depth=1"
else
    pass
fi

test_name "List /nonexistent returns 404"
BODY=$(list_directory "$NODE1" "/nonexistent")
assert_status 404
pass

test_name "Cleanup test files and directories"
delete_file "$NODE1" "/testdir/file1.txt"
delete_file "$NODE1" "/testdir/file2.txt"
delete_file "$NODE1" "/a/b/c/deep.txt"
cleanup_test_files
pass

print_summary
exit $?
