#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Metrics Correctness"

test_name "Fetch initial metrics and verify callfs_http_requests_total present"
METRICS=$(callfs_curl GET "${NODE1}/metrics")
_read_status
assert_status "200"
assert_body_contains "$METRICS" "callfs_http_requests_total"
pass

test_name "Upload file for metrics testing"
upload_file "$NODE1" "/metrics-corr-test.txt" "metrics correctness data" >/dev/null
_read_status
assert_status "201"
pass

test_name "Download file for metrics testing"
download_file "$NODE1" "/metrics-corr-test.txt" >/dev/null
_read_status
assert_status "200"
pass

test_name "POST counter present in metrics after upload"
# Fetch metrics fresh and save to temp file to avoid shell variable truncation
curl -s -H "Authorization: Bearer ${API_KEY}" "${NODE1}/metrics" > "${_TMPDIR}/metrics.txt" 2>/dev/null
if grep -q 'method="POST"' "${_TMPDIR}/metrics.txt"; then
  pass
else
  fail "callfs_http_requests_total with method=POST not found"
fi

test_name "GET counter present in metrics after download"
if grep -q 'method="GET"' "${_TMPDIR}/metrics.txt"; then
  pass
else
  fail "callfs_http_requests_total with method=GET not found"
fi

test_name "callfs_file_operations_total present with create operation"
if grep -q 'callfs_file_operations_total.*create' "${_TMPDIR}/metrics.txt"; then
  pass
else
  fail "callfs_file_operations_total with create not found"
fi

test_name "callfs_file_operations_total present with read operation"
if grep -q 'callfs_file_operations_total.*read' "${_TMPDIR}/metrics.txt"; then
  pass
else
  fail "callfs_file_operations_total with read not found"
fi

test_name "callfs_backend_op_duration_seconds present in metrics"
if grep -q 'callfs_backend_op_duration_seconds' "${_TMPDIR}/metrics.txt"; then
  pass
else
  fail "callfs_backend_op_duration_seconds not found"
fi

sleep 1
test_name "callfs_single_use_link_generations_total present after generating link"
callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/metrics-corr-test.txt","expiry_seconds":3600}' >/dev/null
_read_status
assert_status "201"
curl -s -H "Authorization: Bearer ${API_KEY}" "${NODE1}/metrics" > "${_TMPDIR}/metrics2.txt" 2>/dev/null
if grep -q 'callfs_single_use_link_generations_total' "${_TMPDIR}/metrics2.txt"; then
  pass
else
  fail "callfs_single_use_link_generations_total not found"
fi

test_name "Cleanup metrics correctness test files"
delete_file "$NODE1" "/metrics-corr-test.txt" >/dev/null 2>&1 || true
pass

print_summary
exit $?
