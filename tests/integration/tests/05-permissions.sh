#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Permissions & Metadata"

test_name "Upload /test.txt and check default file mode is 0644"
upload_file "$NODE1" "/test.txt" "permission test"
assert_status 201
callfs_head_method "${NODE1}/v1/files/test.txt"
assert_status 200
assert_header_contains "X-CallFS-Mode" "0644"
pass

test_name "Create directory /testdir/ and check default mode is 0755"
create_directory "$NODE1" "/testdir/"
assert_status 201
callfs_head_method "${NODE1}/v1/directories/testdir"
assert_status 200
assert_header_contains "X-CallFS-Mode" "0755"
pass

test_name "HEAD /test.txt has X-CallFS-UID header"
callfs_head_method "${NODE1}/v1/files/test.txt"
assert_status 200
assert_header_present "X-CallFS-UID"
pass

test_name "HEAD /test.txt has X-CallFS-GID header"
callfs_head_method "${NODE1}/v1/files/test.txt"
assert_status 200
assert_header_present "X-CallFS-GID"
pass

test_name "Cleanup"
delete_file "$NODE1" "/test.txt"
cleanup_test_files
pass

print_summary
exit $?
