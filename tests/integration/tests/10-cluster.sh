#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Cluster"

test_name "NODE1 /health returns 200"
BODY=$(callfs_curl_noauth GET "${NODE1}/health")
assert_status 200
pass

test_name "NODE2 /health returns 200"
BODY=$(callfs_curl_noauth GET "${NODE2}/health")
assert_status 200
pass

test_name "NODE3 /health returns 200"
BODY=$(callfs_curl_noauth GET "${NODE3}/health")
assert_status 200
pass

test_name "Upload /cluster-test1.txt on NODE1 returns 201"
upload_file "$NODE1" "/cluster-test1.txt" "from node1"
assert_status 201
pass

test_name "GET /cluster-test1.txt on NODE2 returns 200"
BODY=$(download_file "$NODE2" "/cluster-test1.txt")
assert_status 200
assert_body_equals "$BODY" "from node1"
pass

test_name "GET /cluster-test1.txt on NODE3 returns 200"
BODY=$(download_file "$NODE3" "/cluster-test1.txt")
assert_status 200
assert_body_equals "$BODY" "from node1"
pass

test_name "Upload /cluster-test2.txt on NODE2 returns 201"
upload_file "$NODE2" "/cluster-test2.txt" "from node2"
assert_status 201
pass

test_name "GET /cluster-test2.txt on NODE1 returns correct body"
BODY=$(download_file "$NODE1" "/cluster-test2.txt")
assert_status 200
assert_body_equals "$BODY" "from node2"
pass

test_name "GET /cluster-test2.txt on NODE3 returns correct body"
BODY=$(download_file "$NODE3" "/cluster-test2.txt")
assert_status 200
assert_body_equals "$BODY" "from node2"
pass

test_name "List root directory on NODE1 contains both cluster test files"
BODY=$(list_directory "$NODE1" "/")
assert_status 200
assert_body_contains "$BODY" "cluster-test1.txt"
assert_body_contains "$BODY" "cluster-test2.txt"
pass

test_name "List root directory on NODE2 contains both cluster test files"
BODY=$(list_directory "$NODE2" "/")
assert_status 200
assert_body_contains "$BODY" "cluster-test1.txt"
assert_body_contains "$BODY" "cluster-test2.txt"
pass

test_name "List root directory on NODE3 contains both cluster test files"
BODY=$(list_directory "$NODE3" "/")
assert_status 200
assert_body_contains "$BODY" "cluster-test1.txt"
assert_body_contains "$BODY" "cluster-test2.txt"
pass

test_name "Generate link on NODE1, download via NODE2 returns 200"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/cluster-test1.txt","expiry_seconds":3600}')
assert_status 201
TOKEN=$(echo "$BODY" | jq -r '.token')
if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
  fail "token not found in response"
fi
BODY=$(callfs_curl_noauth GET "${NODE2}/download/${TOKEN}")
assert_status 200
assert_body_equals "$BODY" "from node1"
pass

test_name "Cleanup cluster test files"
delete_file "$NODE1" "/cluster-test1.txt"
delete_file "$NODE1" "/cluster-test2.txt"
pass

print_summary
exit $?
