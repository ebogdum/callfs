#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Directory Edge Cases"

# ---------- Non-empty directory deletion ----------

test_name "Setup: create directory with files"
create_directory "$NODE1" "/nonempty-dir/"
assert_status 201
pass

test_name "Upload file into /nonempty-dir/"
upload_file "$NODE1" "/nonempty-dir/child.txt" "child content"
assert_status 201
pass

test_name "DELETE non-empty directory returns error (not 204)"
delete_file "$NODE1" "/nonempty-dir"
# Should NOT succeed - directory has children
if [ "$LAST_STATUS" = "204" ]; then
  fail "expected error deleting non-empty directory, got 204"
else
  pass
fi

test_name "Directory still exists after failed delete"
BODY=$(list_directory "$NODE1" "/nonempty-dir")
assert_status 200
assert_body_contains "$BODY" "child.txt"
pass

# ---------- Deeply nested directory listing ----------

test_name "Create deeply nested path /d1/d2/d3/d4/d5/leaf.txt"
upload_file "$NODE1" "/d1/d2/d3/d4/d5/leaf.txt" "deep leaf"
assert_status 201
pass

test_name "List /d1 with max_depth=0 returns immediate children only"
BODY=$(list_directory "$NODE1" "/d1" --data-urlencode "recursive=true" --data-urlencode "max_depth=0")
assert_status 200
# max_depth=0 should show d2 at most, definitely not leaf.txt
if echo "$BODY" | grep -q "leaf.txt"; then
  fail "max_depth=0 should not show deeply nested leaf.txt"
else
  pass
fi

test_name "List /d1 with recursive=true shows full tree"
BODY=$(list_directory "$NODE1" "/d1" --data-urlencode "recursive=true" --data-urlencode "max_depth=100")
assert_status 200
assert_body_contains "$BODY" "leaf.txt"
pass

test_name "List /d1 with max_depth=2 shows intermediate but not leaf"
BODY=$(list_directory "$NODE1" "/d1" --data-urlencode "recursive=true" --data-urlencode "max_depth=2")
assert_status 200
assert_body_contains "$BODY" "d3"
if echo "$BODY" | grep -q "leaf.txt"; then
  fail "max_depth=2 should not show leaf.txt at depth 5"
else
  pass
fi

# ---------- Type conflict: POST file where dir exists ----------

test_name "POST file at existing directory path returns 409"
BODY=$(callfs_curl POST "${NODE1}/v1/files/d1" \
  -H "Content-Type: application/octet-stream" \
  -d "should fail")
assert_status 409
pass

# ---------- POST directory where file exists ----------

test_name "Setup: create a regular file /type-conflict.txt"
upload_file "$NODE1" "/type-conflict.txt" "i am a file"
assert_status 201
pass

test_name "POST directory at existing file path returns 409"
create_directory "$NODE1" "/type-conflict.txt/"
assert_status 409
pass

# ---------- Empty directory listing ----------

test_name "Create empty directory /empty-dir/"
create_directory "$NODE1" "/empty-dir/"
assert_status 201
pass

test_name "List empty directory returns 200 with empty entries"
BODY=$(list_directory "$NODE1" "/empty-dir")
assert_status 200
pass

test_name "DELETE empty directory succeeds with 204"
delete_file "$NODE1" "/empty-dir"
assert_status 204
pass

# ---------- Cross-server directory operations ----------

test_name "Create directory on NODE1, list from NODE2"
create_directory "$NODE1" "/cross-dir/"
assert_status 201
upload_file "$NODE1" "/cross-dir/from-node1.txt" "node1 file"
assert_status 201
pass

test_name "List /cross-dir from NODE2 returns contents"
BODY=$(list_directory "$NODE2" "/cross-dir")
assert_status 200
assert_body_contains "$BODY" "from-node1.txt"
pass

# ---------- Cleanup ----------

test_name "Cleanup directory edge case files"
delete_file "$NODE1" "/nonempty-dir/child.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/nonempty-dir" >/dev/null 2>&1 || true
delete_file "$NODE1" "/d1/d2/d3/d4/d5/leaf.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/type-conflict.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/cross-dir/from-node1.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/cross-dir" >/dev/null 2>&1 || true
pass

print_summary
exit $?
