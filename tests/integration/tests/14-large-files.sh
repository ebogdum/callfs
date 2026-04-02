#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Large File Upload/Download"

test_name "Generate 5MB test file"
dd if=/dev/urandom of=/tmp/large-5m.bin bs=1048576 count=5 2>/dev/null
pass

test_name "Upload 5MB file to /large-5m.bin"
upload_file_binary "$NODE1" "/large-5m.bin" "/tmp/large-5m.bin"
assert_status 201

test_name "HEAD /large-5m.bin shows correct size (5242880)"
callfs_head_method "${NODE1}/v1/files/large-5m.bin"
assert_status 200
assert_header_contains "X-CallFS-Size" "5242880"

test_name "Download 5MB file"
curl -s --max-time 30 \
  -H "Authorization: Bearer ${API_KEY}" \
  -o /tmp/large-5m-output.bin \
  "${NODE1}/v1/files/large-5m.bin" 2>/dev/null
pass

test_name "5MB file checksums match"
HASH_ORIG=$(md5sum /tmp/large-5m.bin | awk '{print $1}')
HASH_DOWN=$(md5sum /tmp/large-5m-output.bin | awk '{print $1}')
if [ "$HASH_ORIG" = "$HASH_DOWN" ]; then
  pass
else
  fail "checksum mismatch: original=$HASH_ORIG downloaded=$HASH_DOWN"
fi

test_name "Generate 10MB test file"
dd if=/dev/urandom of=/tmp/large-10m.bin bs=1048576 count=10 2>/dev/null
pass

test_name "Upload 10MB file to /large-10m.bin"
upload_file_binary "$NODE1" "/large-10m.bin" "/tmp/large-10m.bin"
assert_status 201

test_name "Download 10MB file"
curl -s --max-time 60 \
  -H "Authorization: Bearer ${API_KEY}" \
  -o /tmp/large-10m-output.bin \
  "${NODE1}/v1/files/large-10m.bin" 2>/dev/null
pass

test_name "10MB file checksums match"
HASH_ORIG=$(md5sum /tmp/large-10m.bin | awk '{print $1}')
HASH_DOWN=$(md5sum /tmp/large-10m-output.bin | awk '{print $1}')
if [ "$HASH_ORIG" = "$HASH_DOWN" ]; then
  pass
else
  fail "checksum mismatch: original=$HASH_ORIG downloaded=$HASH_DOWN"
fi

test_name "Cleanup large files from server"
delete_file "$NODE1" "/large-5m.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/large-10m.bin" >/dev/null 2>&1 || true
pass

test_name "Cleanup local temp files"
rm -f /tmp/large-5m.bin /tmp/large-5m-output.bin /tmp/large-10m.bin /tmp/large-10m-output.bin
pass

print_summary
exit $?
