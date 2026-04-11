#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Authentication"

test_name "GET /v1/directories/ with valid API key returns 200"
BODY=$(callfs_curl GET "${NODE1}/v1/directories/")
_read_status
assert_status "200"
pass

test_name "GET /v1/directories/ with no auth header returns 401"
BODY=$(callfs_curl_noauth GET "${NODE1}/v1/directories/")
_read_status
assert_status "401"
pass

test_name "GET /v1/directories/ with wrong API key returns 401"
BODY=$(callfs_curl_noauth GET "${NODE1}/v1/directories/" -H "Authorization: Bearer wrong-key-that-is-long-enough")
_read_status
assert_status "401"
pass

test_name "GET /v1/directories/ with malformed auth header returns 401"
BODY=$(callfs_curl_noauth GET "${NODE1}/v1/directories/" -H "Authorization: InvalidFormat")
_read_status
assert_status "401"
pass

test_name "GET /v1/directories/ with empty bearer token returns 401"
BODY=$(callfs_curl_noauth GET "${NODE1}/v1/directories/" -H "Authorization: Bearer ")
_read_status
assert_status "401"
pass

test_name "Second API key authenticates successfully"
BODY=$(callfs_curl_token "$API_KEY_SECONDARY" GET "${NODE1}/v1/directories/")
_read_status
assert_status "200"
pass

test_name "Both API keys see same data"
upload_file "$NODE1" "/auth-shared-test.txt" "shared-key-data" >/dev/null
_read_status
BODY=$(callfs_curl_token "$API_KEY_SECONDARY" GET "${NODE1}/v1/files/auth-shared-test.txt")
_read_status
assert_status "200"
assert_body_equals "$BODY" "shared-key-data"
pass

test_name "Internal proxy secret authenticates as internal-proxy user"
# The internal proxy secret is registered as a valid auth key (user: internal-proxy)
# for cross-server operations. This is by design.
BODY=$(callfs_curl_token "$INTERNAL_SECRET" GET "${NODE1}/v1/directories/")
_read_status
assert_status "200"
pass

# Cleanup
delete_file "$NODE1" "/auth-shared-test.txt" >/dev/null 2>&1 || true

print_summary
exit $?
