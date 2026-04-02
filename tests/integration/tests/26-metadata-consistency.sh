#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Metadata Consistency"

# ---------- Timestamps update on write ----------

test_name "Create file and record initial MTime"
upload_file "$NODE1" "/meta-ts.txt" "initial"
assert_status 201
callfs_head_method "${NODE1}/v1/files/meta-ts.txt"
assert_status 200
MTIME1=$(echo "$LAST_HEADERS" | grep -i "X-CallFS-MTime" | head -1 | sed 's/^[^:]*: *//' | tr -d '\r')
pass

test_name "Sleep and update file, MTime should change"
sleep 2
BODY=$(callfs_curl PUT "${NODE1}/v1/files/meta-ts.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "updated")
assert_status 200
callfs_head_method "${NODE1}/v1/files/meta-ts.txt"
MTIME2=$(echo "$LAST_HEADERS" | grep -i "X-CallFS-MTime" | head -1 | sed 's/^[^:]*: *//' | tr -d '\r')
if [ "$MTIME1" != "$MTIME2" ]; then
  pass
else
  fail "MTime did not change after update: $MTIME1 == $MTIME2"
fi

# ---------- Size metadata accuracy ----------

test_name "Size metadata matches actual content length"
upload_file "$NODE1" "/meta-size.txt" "exactly 26 characters long"
assert_status 201
callfs_head_method "${NODE1}/v1/files/meta-size.txt"
assert_status 200
assert_header_contains "X-CallFS-Size" "26"
pass

test_name "Size updates after PUT with different content"
BODY=$(callfs_curl PUT "${NODE1}/v1/files/meta-size.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "short")
assert_status 200
callfs_head_method "${NODE1}/v1/files/meta-size.txt"
assert_header_contains "X-CallFS-Size" "5"
pass

# ---------- Mode metadata ----------

test_name "File mode is 0644 by default"
upload_file "$NODE1" "/meta-mode.txt" "mode test"
assert_status 201
callfs_head_method "${NODE1}/v1/files/meta-mode.txt"
assert_header_contains "X-CallFS-Mode" "0644"
pass

test_name "Directory mode is 0755"
create_directory "$NODE1" "/meta-mode-dir/"
assert_status 201
callfs_head_method "${NODE1}/v1/files/meta-mode-dir"
assert_header_contains "X-CallFS-Mode" "0755"
pass

# ---------- Metadata consistent across cluster ----------

test_name "HEAD from all 3 nodes returns same Size"
upload_file "$NODE1" "/meta-cluster.txt" "cluster consistency test"
assert_status 201

callfs_head_method "${NODE1}/v1/files/meta-cluster.txt"
SIZE1=$(echo "$LAST_HEADERS" | grep -i "X-CallFS-Size" | head -1 | sed 's/^[^:]*: *//' | tr -d '\r')

callfs_head_method "${NODE2}/v1/files/meta-cluster.txt"
SIZE2=$(echo "$LAST_HEADERS" | grep -i "X-CallFS-Size" | head -1 | sed 's/^[^:]*: *//' | tr -d '\r')

callfs_head_method "${NODE3}/v1/files/meta-cluster.txt"
SIZE3=$(echo "$LAST_HEADERS" | grep -i "X-CallFS-Size" | head -1 | sed 's/^[^:]*: *//' | tr -d '\r')

if [ "$SIZE1" = "$SIZE2" ] && [ "$SIZE2" = "$SIZE3" ]; then
  pass
else
  fail "size mismatch across nodes: $SIZE1 $SIZE2 $SIZE3"
fi

test_name "HEAD from all 3 nodes returns same Mode"
callfs_head_method "${NODE1}/v1/files/meta-cluster.txt"
MODE1=$(echo "$LAST_HEADERS" | grep -i "X-CallFS-Mode" | head -1 | sed 's/^[^:]*: *//' | tr -d '\r')

callfs_head_method "${NODE2}/v1/files/meta-cluster.txt"
MODE2=$(echo "$LAST_HEADERS" | grep -i "X-CallFS-Mode" | head -1 | sed 's/^[^:]*: *//' | tr -d '\r')

callfs_head_method "${NODE3}/v1/files/meta-cluster.txt"
MODE3=$(echo "$LAST_HEADERS" | grep -i "X-CallFS-Mode" | head -1 | sed 's/^[^:]*: *//' | tr -d '\r')

if [ "$MODE1" = "$MODE2" ] && [ "$MODE2" = "$MODE3" ]; then
  pass
else
  fail "mode mismatch across nodes: $MODE1 $MODE2 $MODE3"
fi

# ---------- Directory listing metadata ----------

test_name "Directory listing contains type field for entries"
create_directory "$NODE1" "/meta-list-dir/"
assert_status 201
upload_file "$NODE1" "/meta-list-dir/file.txt" "in dir"
assert_status 201
create_directory "$NODE1" "/meta-list-dir/subdir/"
assert_status 201

BODY=$(list_directory "$NODE1" "/meta-list-dir")
assert_status 200
assert_body_contains "$BODY" "type"
assert_body_contains "$BODY" "file.txt"
assert_body_contains "$BODY" "subdir"
pass

# ---------- Cleanup ----------

test_name "Cleanup metadata consistency test files"
delete_file "$NODE1" "/meta-ts.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/meta-size.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/meta-mode.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/meta-mode-dir" >/dev/null 2>&1 || true
delete_file "$NODE1" "/meta-cluster.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/meta-list-dir/file.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/meta-list-dir/subdir" >/dev/null 2>&1 || true
delete_file "$NODE1" "/meta-list-dir" >/dev/null 2>&1 || true
pass

print_summary
exit $?
