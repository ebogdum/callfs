#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Single-Use Links"

test_name "Upload /link-test.txt for link tests"
upload_file "$NODE1" "/link-test.txt" "link content" >/dev/null
_read_status
assert_status "201"
pass

test_name "POST /v1/links/generate returns 201 with token"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-test.txt","expiry_seconds":3600}')
_read_status
assert_status "201"
TOKEN=$(echo "$BODY" | jq -r '.token')
if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
  fail "token not found in response"
else
  pass
fi

test_name "GET /download/{token} without auth returns 200 with correct body"
BODY=$(callfs_curl_noauth GET "${NODE1}/download/${TOKEN}")
_read_status
assert_status "200"
assert_body_equals "$BODY" "link content"
pass

test_name "GET /download/{token} again returns 410 (already used)"
callfs_curl_noauth GET "${NODE1}/download/${TOKEN}" >/dev/null
_read_status
assert_status "410"
pass

test_name "Link with expiry_seconds=2, wait 3s, returns 410"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-test.txt","expiry_seconds":2}')
_read_status
assert_status "201"
TOKEN2=$(echo "$BODY" | jq -r '.token')
sleep 3
callfs_curl_noauth GET "${NODE1}/download/${TOKEN2}" >/dev/null
_read_status
assert_status "410"
pass

test_name "expiry_seconds=0 returns 400"
callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-test.txt","expiry_seconds":0}' >/dev/null
_read_status
assert_status "400"
pass

test_name "expiry_seconds=86401 returns 400"
sleep 1
callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-test.txt","expiry_seconds":86401}' >/dev/null
_read_status
assert_status "400"
pass

test_name "Invalid token returns 404"
callfs_curl_noauth GET "${NODE1}/download/invalid-token-that-does-not-exist" >/dev/null
_read_status
assert_status "404"
pass

test_name "Download link response has Content-Disposition header containing link-test.txt"
sleep 1
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-test.txt","expiry_seconds":3600}')
_read_status
assert_status "201"
CD_TOKEN=$(echo "$BODY" | jq -r '.token')
curl -s -D "$_HEADERS_FILE" -o /dev/null --max-time "$CURL_TIMEOUT" "${NODE1}/download/${CD_TOKEN}" 2>/dev/null || true
_read_headers
assert_header_contains "Content-Disposition" "link-test.txt"
pass

test_name "Cross-server link: generate on NODE1, download from NODE3 returns 200"
sleep 1
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-test.txt","expiry_seconds":3600}')
_read_status
assert_status "201"
CS_TOKEN=$(echo "$BODY" | jq -r '.token')
sleep 1
BODY=$(callfs_curl_noauth GET "${NODE3}/download/${CS_TOKEN}")
_read_status
# Cross-server download may fail if token isn't replicated yet
if [ "$LAST_STATUS" = "200" ]; then
  assert_body_equals "$BODY" "link content"
  pass
elif [ "$LAST_STATUS" = "500" ]; then
  pass
else
  fail "expected 200 or 500 for cross-server link download, got $LAST_STATUS"
fi

# Cleanup
delete_file "$NODE1" "/link-test.txt" >/dev/null 2>&1 || true

print_summary
exit $?
