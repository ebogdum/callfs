#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Health & Metrics"

test_name "GET /health returns 200 with status ok (no auth)"
BODY=$(callfs_curl_noauth GET "${NODE1}/health")
assert_status 200
assert_body_contains "$BODY" '"status":"ok"'
pass

test_name "GET /metrics without auth returns 401"
BODY=$(callfs_curl_noauth GET "${NODE1}/metrics")
assert_status 401
pass

test_name "GET /metrics with auth returns 200 and contains callfs_http_requests_total"
BODY=$(callfs_curl GET "${NODE1}/metrics")
assert_status 200
assert_body_contains "$BODY" "callfs_http_requests_total"
pass

test_name "GET /metrics body contains callfs_http_request_duration_seconds"
BODY=$(callfs_curl GET "${NODE1}/metrics")
assert_status 200
assert_body_contains "$BODY" "callfs_http_request_duration_seconds"
pass

print_summary
exit $?
