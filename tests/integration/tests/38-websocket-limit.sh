#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

if ! command -v websocat &>/dev/null; then
  echo "SKIP: websocat not available"
  exit 0
fi

section "WebSocket Upload Size Limit"

OVERSIZE_FILE="${_TMPDIR}/ws-oversize.bin"
generate_zero_binary "$OVERSIZE_FILE" 105906176

test_name "WebSocket upload over 100 MB is rejected and no file is created"
WS_HOST="${NODE1#http://}"
websocat --binary "ws://${WS_HOST}/v1/files/ws/ws-oversize.txt?mode=upload" \
  --header "Authorization: Bearer ${API_KEY}" < "$OVERSIZE_FILE" >/dev/null 2>&1 || true
download_file "$NODE1" "/ws-oversize.txt" >/dev/null
_read_status
assert_status 404
pass

delete_file "$NODE1" "/ws-oversize.txt" >/dev/null 2>&1 || true

print_summary
exit $?
