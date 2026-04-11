#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Permissions & Metadata"

test_name "File default mode is 0644"
upload_file "$NODE1" "/perm-test.txt" "permission test" >/dev/null
_read_status
assert_status "201"
callfs_head_method "${NODE1}/v1/files/perm-test.txt"
assert_status "200"
assert_header_contains "X-CallFS-Mode" "0644"
pass

test_name "Directory default mode is 0755"
create_directory "$NODE1" "/perm-testdir/" >/dev/null
_read_status
callfs_head_method "${NODE1}/v1/files/perm-testdir/"
assert_status "200"
assert_header_contains "X-CallFS-Mode" "0755"
pass

test_name "HEAD has X-CallFS-Owner"
callfs_head_method "${NODE1}/v1/files/perm-test.txt"
assert_status "200"
assert_header_present "X-CallFS-Owner"
pass

test_name "X-CallFS-MTime is a parseable timestamp"
callfs_head_method "${NODE1}/v1/files/perm-test.txt"
assert_status "200"
assert_header_present "X-CallFS-MTime"
MTIME_VAL=$(echo "$LAST_HEADERS" | grep -i "^X-CallFS-MTime:" | head -1 | sed 's/^[^:]*: *//' | tr -d '\r')
if echo "$MTIME_VAL" | grep -qE '20[0-9]{2}-[0-9]{2}-[0-9]{2}T'; then
  pass
else
  fail "X-CallFS-MTime '${MTIME_VAL}' does not match expected timestamp pattern"
fi

# Cleanup
delete_file "$NODE1" "/perm-test.txt" >/dev/null 2>&1 || true
callfs_curl DELETE "${NODE1}/v1/files/perm-testdir" >/dev/null 2>&1 || true

print_summary
exit $?
