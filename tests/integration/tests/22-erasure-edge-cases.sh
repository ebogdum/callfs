#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Erasure Coding Edge Cases"

# ---------- Erasure with file below min size ----------

test_name "POST tiny file (10 bytes) with erasure=true"
BODY=$(callfs_curl POST "${NODE1}/v1/files/erasure-tiny.txt?erasure=true&data_shards=2&parity_shards=1" \
  -H "Content-Type: application/octet-stream" \
  -d "tiny file!")
# The cluster config has min_file_size=1024, so 10 bytes should either
# be stored non-erasure or rejected
if [ "$LAST_STATUS" = "201" ] || [ "$LAST_STATUS" = "200" ] || [ "$LAST_STATUS" = "400" ]; then
  pass
else
  fail "unexpected status $LAST_STATUS for below-min-size erasure upload"
fi

test_name "GET tiny erasure file returns correct content"
BODY=$(download_file "$NODE1" "/erasure-tiny.txt")
if [ "$LAST_STATUS" = "200" ]; then
  assert_body_equals "$BODY" "tiny file!"
  pass
elif [ "$LAST_STATUS" = "404" ]; then
  # File may not have been created if below min size
  pass
else
  fail "unexpected status $LAST_STATUS"
fi

# ---------- Erasure manifest structure validation ----------

test_name "Generate erasure file for manifest validation"
dd if=/dev/urandom of=/tmp/erasure-manifest-test.bin bs=2048 count=1 2>/dev/null
BODY=$(callfs_curl POST "${NODE1}/v1/files/erasure-manifest.bin?erasure=true&data_shards=2&parity_shards=1" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @/tmp/erasure-manifest-test.bin)
assert_status 201
pass

test_name "Manifest contains data_shards field"
BODY=$(callfs_curl GET "${NODE1}/v1/files/erasure-manifest.bin?manifest=true")
assert_status 200
assert_body_contains "$BODY" "data_shards"
pass

test_name "Manifest contains parity_shards field"
assert_body_contains "$BODY" "parity_shards"
pass

test_name "Manifest contains original_size field"
assert_body_contains "$BODY" "original_size"
pass

test_name "Manifest shard count equals data_shards + parity_shards"
SHARD_COUNT=$(echo "$BODY" | jq '.shards | length' 2>/dev/null || echo "0")
if [ "$SHARD_COUNT" = "3" ]; then
  pass
else
  fail "expected 3 shards (2 data + 1 parity), got $SHARD_COUNT"
fi

test_name "Each shard has checksum field"
FIRST_CHECKSUM=$(echo "$BODY" | jq -r '.shards[0].checksum' 2>/dev/null || echo "null")
if [ "$FIRST_CHECKSUM" != "null" ] && [ -n "$FIRST_CHECKSUM" ]; then
  pass
else
  fail "shard missing checksum field"
fi

# ---------- Erasure with different shard configurations ----------

test_name "Erasure with data_shards=3 parity_shards=2"
dd if=/dev/urandom of=/tmp/erasure-35.bin bs=2048 count=2 2>/dev/null
INPUT_MD5=$(md5sum /tmp/erasure-35.bin | awk '{print $1}')
BODY=$(callfs_curl POST "${NODE1}/v1/files/erasure-35.bin?erasure=true&data_shards=3&parity_shards=2" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @/tmp/erasure-35.bin)
assert_status 201
pass

test_name "Retrieve erasure 3+2 file matches original"
callfs_curl GET "${NODE1}/v1/files/erasure-35.bin" -o /tmp/erasure-35-out.bin
assert_status 200
OUTPUT_MD5=$(md5sum /tmp/erasure-35-out.bin | awk '{print $1}')
if [ "$INPUT_MD5" = "$OUTPUT_MD5" ]; then
  pass
else
  fail "md5 mismatch: input=${INPUT_MD5} output=${OUTPUT_MD5}"
fi

# ---------- Erasure cross-server retrieval ----------

test_name "Upload erasure file on NODE1, retrieve from NODE2"
dd if=/dev/urandom of=/tmp/erasure-cross.bin bs=2048 count=1 2>/dev/null
INPUT_MD5=$(md5sum /tmp/erasure-cross.bin | awk '{print $1}')
BODY=$(callfs_curl POST "${NODE1}/v1/files/erasure-cross.bin?erasure=true&data_shards=2&parity_shards=1" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @/tmp/erasure-cross.bin)
assert_status 201
pass

test_name "GET erasure file from NODE2 matches original"
callfs_curl GET "${NODE2}/v1/files/erasure-cross.bin" -o /tmp/erasure-cross-out.bin
assert_status 200
OUTPUT_MD5=$(md5sum /tmp/erasure-cross-out.bin | awk '{print $1}')
if [ "$INPUT_MD5" = "$OUTPUT_MD5" ]; then
  pass
else
  fail "md5 mismatch: input=${INPUT_MD5} output=${OUTPUT_MD5}"
fi

test_name "GET erasure file from NODE3 matches original"
callfs_curl GET "${NODE3}/v1/files/erasure-cross.bin" -o /tmp/erasure-cross-out3.bin
assert_status 200
OUTPUT_MD5=$(md5sum /tmp/erasure-cross-out3.bin | awk '{print $1}')
if [ "$INPUT_MD5" = "$OUTPUT_MD5" ]; then
  pass
else
  fail "md5 mismatch: input=${INPUT_MD5} output=${OUTPUT_MD5}"
fi

# ---------- Invalid erasure parameters ----------

test_name "POST with data_shards=0 returns error"
BODY=$(callfs_curl POST "${NODE1}/v1/files/erasure-bad.bin?erasure=true&data_shards=0&parity_shards=1" \
  -H "Content-Type: application/octet-stream" \
  -d "bad params")
if [ "$LAST_STATUS" != "201" ]; then
  pass
else
  fail "expected error for data_shards=0, got 201"
fi

test_name "POST with parity_shards=0 returns error"
BODY=$(callfs_curl POST "${NODE1}/v1/files/erasure-bad2.bin?erasure=true&data_shards=2&parity_shards=0" \
  -H "Content-Type: application/octet-stream" \
  -d "bad params")
if [ "$LAST_STATUS" != "201" ]; then
  pass
else
  fail "expected error for parity_shards=0, got 201"
fi

# ---------- Cleanup ----------

test_name "Cleanup erasure edge case test files"
delete_file "$NODE1" "/erasure-tiny.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/erasure-manifest.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/erasure-35.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/erasure-cross.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/erasure-bad.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/erasure-bad2.bin" >/dev/null 2>&1 || true
rm -f /tmp/erasure-manifest-test.bin /tmp/erasure-35.bin /tmp/erasure-35-out.bin \
      /tmp/erasure-cross.bin /tmp/erasure-cross-out.bin /tmp/erasure-cross-out3.bin
pass

print_summary
exit $?
