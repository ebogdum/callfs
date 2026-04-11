#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Health & Metrics"

test_name "GET /health returns 200 with status ok (no auth)"
BODY=$(callfs_curl_noauth GET "${NODE1}/health")
_read_status
assert_status "200"
assert_body_contains "$BODY" '"status":"ok"'
pass

test_name "GET /metrics without auth returns 401"
callfs_curl_noauth GET "${NODE1}/metrics" >/dev/null
_read_status
assert_status "401"
pass

test_name "GET /metrics with auth returns 200 and contains callfs_http_requests_total"
BODY=$(callfs_curl GET "${NODE1}/metrics")
_read_status
assert_status "200"
assert_body_contains "$BODY" "callfs_http_requests_total"
pass

test_name "GET /metrics contains callfs_http_request_duration_seconds"
assert_body_contains "$BODY" "callfs_http_request_duration_seconds"
pass

test_name "Metrics counter increments after file operation"
BEFORE=$(callfs_curl GET "${NODE1}/metrics")
_read_status
COUNT_BEFORE=$(echo "$BEFORE" | grep 'callfs_http_requests_total{' | grep 'method="POST"' | head -1 | awk '{print $NF}') || COUNT_BEFORE="0"

upload_file "$NODE1" "/metrics-test.txt" "metrics-counter-test" >/dev/null
_read_status

AFTER=$(callfs_curl GET "${NODE1}/metrics")
_read_status
COUNT_AFTER=$(echo "$AFTER" | grep 'callfs_http_requests_total{' | grep 'method="POST"' | head -1 | awk '{print $NF}') || COUNT_AFTER="0"

if awk "BEGIN{exit !($COUNT_AFTER > $COUNT_BEFORE)}"; then
  pass
else
  fail "POST counter did not increment: before=${COUNT_BEFORE} after=${COUNT_AFTER}"
fi

test_name "Metrics duration histogram has observations"
# Re-fetch metrics to ensure all operations are recorded
FRESH=$(callfs_curl GET "${NODE1}/metrics")
_read_status
# Sum all duration counts across all labels
DURATION_TOTAL=$(echo "$FRESH" | grep '^callfs_http_request_duration_seconds_count{' | awk '{s+=$NF} END{print s+0}') || DURATION_TOTAL="0"
if [ -z "$DURATION_TOTAL" ]; then DURATION_TOTAL="0"; fi
if [ "$DURATION_TOTAL" -gt 0 ] 2>/dev/null; then
  pass
else
  fail "duration histogram total count is $DURATION_TOTAL"
fi

# Cleanup
delete_file "$NODE1" "/metrics-test.txt" >/dev/null 2>&1 || true

print_summary
exit $?
