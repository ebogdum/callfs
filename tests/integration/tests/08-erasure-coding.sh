#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Erasure Coding"

test_name "Generate 2KB test file for erasure coding"
dd if=/dev/urandom of=/tmp/erasure-input.bin bs=2048 count=1 2>/dev/null
INPUT_MD5=$(md5sum /tmp/erasure-input.bin | awk '{print $1}')
pass

test_name "POST /erasure-test.bin with erasure=true query params returns 201"
BODY=$(callfs_curl POST "${NODE1}/v1/files/erasure-test.bin?erasure=true&data_shards=2&parity_shards=1" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @/tmp/erasure-input.bin)
assert_status 201
pass

test_name "GET /erasure-test.bin returns 200 with matching content"
callfs_curl GET "${NODE1}/v1/files/erasure-test.bin" -o /tmp/erasure-output.bin
assert_status 200
OUTPUT_MD5=$(md5sum /tmp/erasure-output.bin | awk '{print $1}')
if [ "$INPUT_MD5" = "$OUTPUT_MD5" ]; then
  pass
else
  fail "md5 mismatch: input=${INPUT_MD5} output=${OUTPUT_MD5}"
fi

test_name "GET /erasure-test.bin?manifest=true returns 200 with shards in body"
BODY=$(callfs_curl GET "${NODE1}/v1/files/erasure-test.bin?manifest=true")
assert_status 200
assert_body_contains "$BODY" "shards"
pass

test_name "GET /v1/shards/erasure-test.bin/0 returns 200 (shard 0 exists)"
BODY=$(callfs_curl GET "${NODE1}/v1/shards/erasure-test.bin/0")
assert_status 200
pass

test_name "POST /erasure-header-test.bin with erasure headers returns 201"
dd if=/dev/urandom of=/tmp/erasure-input2.bin bs=2048 count=1 2>/dev/null
BODY=$(callfs_curl POST "${NODE1}/v1/files/erasure-header-test.bin" \
  -H "Content-Type: application/octet-stream" \
  -H "X-CallFS-Erasure: true" \
  -H "X-CallFS-Erasure-Data-Shards: 2" \
  -H "X-CallFS-Erasure-Parity-Shards: 1" \
  --data-binary @/tmp/erasure-input2.bin)
assert_status 201
pass

test_name "Cleanup erasure coding test files"
delete_file "$NODE1" "/erasure-test.bin"
delete_file "$NODE1" "/erasure-header-test.bin"
rm -f /tmp/erasure-input.bin /tmp/erasure-output.bin /tmp/erasure-input2.bin
pass

print_summary
exit $?
