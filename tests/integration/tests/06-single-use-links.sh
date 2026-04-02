#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Single-Use Links"

test_name "Upload /link-test.txt for link tests"
upload_file "$NODE1" "/link-test.txt" "link content"
assert_status 201
pass

test_name "POST /v1/links/generate returns 201 with token"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-test.txt","expiry_seconds":3600}')
assert_status 201
TOKEN=$(echo "$BODY" | jq -r '.token')
if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
  fail "token not found in response"
fi
pass

test_name "GET /download/{token} without auth returns 200 with correct body"
BODY=$(callfs_curl_noauth GET "${NODE1}/download/${TOKEN}")
assert_status 200
assert_body_equals "$BODY" "link content"
pass

test_name "GET /download/{token} again returns 410 (already used)"
BODY=$(callfs_curl_noauth GET "${NODE1}/download/${TOKEN}")
assert_status 410
pass

test_name "Generate link with expiry_seconds=2, wait 3s, returns 410 (expired)"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-test.txt","expiry_seconds":2}')
assert_status 201
TOKEN2=$(echo "$BODY" | jq -r '.token')
sleep 3
BODY=$(callfs_curl_noauth GET "${NODE1}/download/${TOKEN2}")
assert_status 410
pass

test_name "POST /v1/links/generate with expiry_seconds=0 returns 400"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-test.txt","expiry_seconds":0}')
assert_status 400
pass

test_name "POST /v1/links/generate with expiry_seconds=86401 returns 400"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-test.txt","expiry_seconds":86401}')
assert_status 400
pass

test_name "GET /download/invalid-token-that-does-not-exist returns 404"
BODY=$(callfs_curl_noauth GET "${NODE1}/download/invalid-token-that-does-not-exist")
assert_status 404
pass

test_name "Cleanup link test files"
delete_file "$NODE1" "/link-test.txt"
pass

print_summary
exit $?
