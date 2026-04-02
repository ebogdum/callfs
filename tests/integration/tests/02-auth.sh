#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Authentication"

test_name "GET /v1/directories/ with valid API key returns 200"
BODY=$(callfs_curl GET "${NODE1}/v1/directories/")
assert_status 200
pass

test_name "GET /v1/directories/ with no auth header returns 401"
BODY=$(callfs_curl_noauth GET "${NODE1}/v1/directories/")
assert_status 401
pass

test_name "GET /v1/directories/ with wrong API key returns 401"
BODY=$(callfs_curl_noauth GET "${NODE1}/v1/directories/" -H "Authorization: Bearer wrong-key-that-is-long-enough")
assert_status 401
pass

test_name "GET /v1/directories/ with malformed auth header returns 401"
BODY=$(callfs_curl_noauth GET "${NODE1}/v1/directories/" -H "Authorization: InvalidFormat")
assert_status 401
pass

test_name "GET /v1/directories/ with empty bearer token returns 401"
BODY=$(callfs_curl_noauth GET "${NODE1}/v1/directories/" -H "Authorization: Bearer ")
assert_status 401
pass

print_summary
exit $?
