#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Link Download Rate Limiting"

test_name "Upload /rate-test.txt for rate limit tests"
upload_file "$NODE1" "/rate-test.txt" "rate limit content"
assert_status 201

test_name "Generate single-use link for rapid-fire test"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/rate-test.txt","expiry_seconds":3600}')
assert_status 201
TOKEN=$(echo "$BODY" | jq -r '.token')
if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
  fail "token not found in response"
fi

test_name "Rapid-fire 20 requests to /download/{token} triggers at least one 429"
COUNT_429=0
for i in $(seq 1 20); do
  STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    --max-time "$CURL_TIMEOUT" \
    "${NODE1}/download/${TOKEN}" 2>/dev/null) || true
  if [ "$STATUS" = "429" ]; then
    COUNT_429=$((COUNT_429 + 1))
  fi
done
echo "  429 responses: ${COUNT_429}/20"
if [ "$COUNT_429" -ge 1 ]; then
  pass
else
  fail "expected at least 1 rate-limited (429) response, got $COUNT_429"
fi

test_name "Cleanup rate limit test files"
delete_file "$NODE1" "/rate-test.txt" >/dev/null 2>&1 || true
pass

print_summary
exit $?
