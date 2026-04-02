#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Metadata Backend Tests"

test_name "Wait for NODE1 ready"
wait_ready "$NODE1" 60
pass

test_name "Upload /meta-test.txt"
upload_file "$NODE1" "/meta-test.txt" "metadata backend test"
assert_status 201

test_name "GET /meta-test.txt returns 200 with correct body"
BODY=$(download_file "$NODE1" "/meta-test.txt")
assert_status 200
assert_body_equals "$BODY" "metadata backend test"

test_name "HEAD /meta-test.txt returns headers"
callfs_head_method "${NODE1}/v1/files/meta-test.txt"
assert_status 200

test_name "HEAD response has Content-Type header"
assert_header_present "Content-Type"

test_name "PUT /meta-test.txt with updated content"
BODY=$(callfs_curl PUT "${NODE1}/v1/files/meta-test.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "updated")
assert_status 200

test_name "GET /meta-test.txt returns updated content"
BODY=$(download_file "$NODE1" "/meta-test.txt")
assert_status 200
assert_body_equals "$BODY" "updated"

test_name "Create directory /meta-dir/"
create_directory "$NODE1" "/meta-dir/"
assert_status 201

test_name "List /meta-dir returns 200"
BODY=$(list_directory "$NODE1" "meta-dir")
assert_status 200

test_name "Generate single-use link for /meta-test.txt"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/meta-test.txt","expiry_seconds":3600}')
assert_status 201
TOKEN=$(echo "$BODY" | jq -r '.token')

test_name "Download via /download/{token} returns 200"
BODY=$(callfs_curl_noauth GET "${NODE1}/download/${TOKEN}")
assert_status 200
assert_body_equals "$BODY" "updated"

test_name "Download same token again returns 410 (already used)"
BODY=$(callfs_curl_noauth GET "${NODE1}/download/${TOKEN}")
assert_status 410

test_name "DELETE /meta-test.txt returns 204"
delete_file "$NODE1" "/meta-test.txt"
assert_status 204

test_name "DELETE /meta-dir returns 204"
delete_file "$NODE1" "/meta-dir"
assert_status 204

print_summary
exit $?
