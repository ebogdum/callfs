#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

if ! command -v websocat &>/dev/null; then
  echo "SKIP: websocat not available"
  exit 0
fi

section "WebSocket"

test_name "Upload /ws-download.txt for websocket download test"
upload_file "$NODE1" "/ws-download.txt" "websocket test content" >/dev/null
_read_status
assert_status "201"
pass

test_name "WebSocket download returns correct content"
WS_HOST="${NODE1#http://}"
WS_BODY=$(websocat "ws://${WS_HOST}/v1/files/ws/ws-download.txt?mode=download" \
  --header "Authorization: Bearer ${API_KEY}" 2>/dev/null) || true
if echo "$WS_BODY" | grep -qF "websocket test content"; then
  pass
else
  fail "expected 'websocket test content' in websocket response, got: ${WS_BODY}"
fi

test_name "WebSocket upload then verify via GET"
WS_HOST="${NODE1#http://}"
echo "ws uploaded data" | websocat "ws://${WS_HOST}/v1/files/ws/ws-upload.txt?mode=upload" \
  --header "Authorization: Bearer ${API_KEY}" 2>/dev/null || true
BODY=$(download_file "$NODE1" "/ws-upload.txt")
_read_status
assert_status "200"
assert_body_contains "$BODY" "ws uploaded data"
pass

# Cleanup
delete_file "$NODE1" "/ws-download.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/ws-upload.txt" >/dev/null 2>&1 || true

print_summary
exit $?
