#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Cross-Instance Move (PATCH via non-owning node)"

test_name "Upload /xi-src.txt on NODE1 returns 201"
upload_file "$NODE1" "/xi-src.txt" "cross instance bytes" >/dev/null
_read_status
assert_status 201
pass

test_name "PATCH move /xi-src.txt via NODE2 to /xi-dst.txt returns 200"
BODY=$(callfs_curl PATCH "${NODE2}/v1/files/xi-src.txt" \
  -H "Content-Type: application/json" \
  -d '{"destination":"/xi-dst.txt"}')
assert_status 200
assert_json_field_equals "$BODY" "path" "/xi-dst.txt"
pass

test_name "Moved file on NODE3 at /xi-dst.txt has original bytes"
BODY=$(download_file "$NODE3" "/xi-dst.txt")
assert_status 200
assert_body_equals "$BODY" "cross instance bytes"
pass

test_name "Source /xi-src.txt on NODE1 returns 404 after move"
download_file "$NODE1" "/xi-src.txt" >/dev/null
_read_status
assert_status 404
pass

delete_file "$NODE1" "/xi-dst.txt" >/dev/null 2>&1 || true

print_summary
exit $?
