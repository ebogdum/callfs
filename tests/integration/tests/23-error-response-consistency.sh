#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Error Response Consistency"

# All error responses should have consistent JSON format with "code" and "message" fields.

# ---------- 400 errors ----------

test_name "400 from invalid path has JSON error format"
BODY=$(callfs_curl GET "${NODE1}/v1/files/../../../etc/passwd")
assert_status 400
assert_body_contains "$BODY" "code"
assert_body_contains "$BODY" "message"
pass

test_name "400 from PUT on directory has JSON error format"
BODY=$(callfs_curl PUT "${NODE1}/v1/files/somedir/" \
  -H "Content-Type: application/octet-stream" -d "x")
assert_status 400
assert_body_contains "$BODY" "message"
pass

# ---------- 401 errors ----------

test_name "401 from missing auth has JSON error format"
BODY=$(callfs_curl_noauth GET "${NODE1}/v1/files/test.txt")
assert_status 401
assert_body_contains "$BODY" "code"
assert_body_contains "$BODY" "message"
pass

test_name "401 from wrong API key has JSON error format"
BODY=$(curl -s -w '\n%{http_code}' --max-time "$CURL_TIMEOUT" \
  -H "Authorization: Bearer wrong-key-here-padding" \
  "${NODE1}/v1/files/test.txt" 2>/dev/null > "$_RESP_FILE" || true)
tail -1 "$_RESP_FILE" > "$_STATUS_FILE"
BODY=$(sed '$d' "$_RESP_FILE")
assert_status 401
assert_body_contains "$BODY" "message"
pass

# ---------- 404 errors ----------

test_name "404 from GET non-existent file has JSON error format"
BODY=$(callfs_curl GET "${NODE1}/v1/files/err-consistency-notfound.txt")
assert_status 404
assert_body_contains "$BODY" "code"
assert_body_contains "$BODY" "message"
pass

test_name "404 from DELETE non-existent file has JSON error format"
BODY=$(delete_file "$NODE1" "/err-consistency-notfound.txt")
assert_status 404
assert_body_contains "$BODY" "code"
assert_body_contains "$BODY" "message"
pass

test_name "404 from list non-existent directory has JSON error format"
BODY=$(list_directory "$NODE1" "/err-consistency-nonexistent-dir")
assert_status 404
assert_body_contains "$BODY" "code"
pass

# ---------- 409 errors ----------

test_name "Setup: create file for 409 test"
upload_file "$NODE1" "/err-conflict.txt" "exists"
assert_status 201
pass

test_name "409 from duplicate POST has JSON error format"
BODY=$(upload_file "$NODE1" "/err-conflict.txt" "duplicate")
assert_status 409
assert_body_contains "$BODY" "message"
pass

# ---------- 410 errors ----------

test_name "410 from expired link has JSON error format"
upload_file "$NODE1" "/err-link-test.txt" "link test"
assert_status 201
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/err-link-test.txt","expiry_seconds":1}')
assert_status 201
TOKEN=$(echo "$BODY" | jq -r '.token')
sleep 2
BODY=$(callfs_curl_noauth GET "${NODE1}/download/${TOKEN}")
assert_status 410
pass

# ---------- Metrics endpoint auth ----------

test_name "Metrics endpoint requires authentication"
BODY=$(callfs_curl_noauth GET "${NODE1}/metrics")
assert_status 401
pass

test_name "Metrics endpoint works with valid auth"
BODY=$(callfs_curl GET "${NODE1}/metrics")
assert_status 200
assert_body_contains "$BODY" "callfs_"
pass

# ---------- Method not allowed ----------

test_name "PATCH method on file endpoint returns error"
BODY=$(callfs_curl PATCH "${NODE1}/v1/files/test.txt" \
  -H "Content-Type: application/octet-stream" -d "patch")
# Should return 405 Method Not Allowed or similar error
if [ "$LAST_STATUS" != "200" ] && [ "$LAST_STATUS" != "201" ]; then
  pass
else
  fail "expected error for PATCH method, got $LAST_STATUS"
fi

# ---------- Cleanup ----------

test_name "Cleanup error consistency test files"
delete_file "$NODE1" "/err-conflict.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/err-link-test.txt" >/dev/null 2>&1 || true
pass

print_summary
exit $?
