#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Cross-Server Advanced Operations"

# ---------- Create on different nodes, list from third ----------

test_name "Upload files on different nodes"
upload_file "$NODE1" "/cross-adv-1.txt" "from node1"
assert_status 201
upload_file "$NODE2" "/cross-adv-2.txt" "from node2"
assert_status 201
upload_file "$NODE3" "/cross-adv-3.txt" "from node3"
assert_status 201
pass

test_name "List root from NODE1 shows all 3 files"
BODY=$(list_directory "$NODE1" "/")
assert_status 200
assert_body_contains "$BODY" "cross-adv-1.txt"
assert_body_contains "$BODY" "cross-adv-2.txt"
assert_body_contains "$BODY" "cross-adv-3.txt"
pass

test_name "List root from NODE3 shows all 3 files"
BODY=$(list_directory "$NODE3" "/")
assert_status 200
assert_body_contains "$BODY" "cross-adv-1.txt"
assert_body_contains "$BODY" "cross-adv-2.txt"
assert_body_contains "$BODY" "cross-adv-3.txt"
pass

# ---------- Cross-server update and verify consistency ----------

test_name "Update file created on NODE1 via NODE3"
BODY=$(callfs_curl PUT "${NODE3}/v1/files/cross-adv-1.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "updated from node3")
assert_status 200
pass

test_name "Read updated file from NODE2 shows new content"
BODY=$(download_file "$NODE2" "/cross-adv-1.txt")
assert_status 200
assert_body_equals "$BODY" "updated from node3"
pass

# ---------- Cross-server delete and verify propagation ----------

test_name "Delete file created on NODE2 via NODE1"
delete_file "$NODE1" "/cross-adv-2.txt"
assert_status 204
pass

test_name "GET deleted file from NODE3 returns 404"
BODY=$(download_file "$NODE3" "/cross-adv-2.txt")
assert_status 404
pass

# ---------- Cross-server directory creation and file upload ----------

test_name "Create directory on NODE1, upload file on NODE2"
create_directory "$NODE1" "/cross-shared-dir/"
assert_status 201
upload_file "$NODE2" "/cross-shared-dir/node2-file.txt" "from node2 in shared dir"
assert_status 201
pass

test_name "List cross-shared-dir from NODE3 shows file"
BODY=$(list_directory "$NODE3" "/cross-shared-dir")
assert_status 200
assert_body_contains "$BODY" "node2-file.txt"
pass

test_name "Download cross-shared-dir file from NODE3"
BODY=$(download_file "$NODE3" "/cross-shared-dir/node2-file.txt")
assert_status 200
assert_body_equals "$BODY" "from node2 in shared dir"
pass

# ---------- HEAD across servers ----------

test_name "HEAD on NODE2 for file created on NODE1"
upload_file "$NODE1" "/cross-head.txt" "head test content"
assert_status 201
callfs_head_method "${NODE2}/v1/files/cross-head.txt"
assert_status 200
assert_header_present "X-CallFS-Size"
assert_header_present "X-CallFS-Instance-ID"
pass

# ---------- Cross-server conflict detection ----------

test_name "POST duplicate file via different node returns 409"
upload_file "$NODE1" "/cross-conflict.txt" "original"
assert_status 201
upload_file "$NODE2" "/cross-conflict.txt" "duplicate"
assert_status 409
pass

# ---------- Rapid cross-server round-trip ----------

test_name "Rapid create-read-update-read-delete across nodes"
upload_file "$NODE1" "/cross-rapid.txt" "v1"
assert_status 201

BODY=$(download_file "$NODE2" "/cross-rapid.txt")
assert_status 200
assert_body_equals "$BODY" "v1"

BODY=$(callfs_curl PUT "${NODE3}/v1/files/cross-rapid.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "v2")
assert_status 200

BODY=$(download_file "$NODE1" "/cross-rapid.txt")
assert_status 200
assert_body_equals "$BODY" "v2"

delete_file "$NODE2" "/cross-rapid.txt"
assert_status 204

BODY=$(download_file "$NODE3" "/cross-rapid.txt")
assert_status 404
pass

# ---------- Cleanup ----------

test_name "Cleanup cross-server advanced test files"
delete_file "$NODE1" "/cross-adv-1.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/cross-adv-2.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/cross-adv-3.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/cross-shared-dir/node2-file.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/cross-shared-dir" >/dev/null 2>&1 || true
delete_file "$NODE1" "/cross-head.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/cross-conflict.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/cross-rapid.txt" >/dev/null 2>&1 || true
pass

print_summary
exit $?
