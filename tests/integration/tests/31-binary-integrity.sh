#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Binary Integrity Suite"

# --- 1KB random binary ---

test_name "1KB random binary upload/download SHA-256 match"
UP_1K="${_TMPDIR}/bin-1k.bin"
DOWN_1K="${_TMPDIR}/bin-1k-down.bin"
generate_random_binary "$UP_1K" 1024
upload_file_binary "$NODE1" "/integrity-1k.bin" "$UP_1K" >/dev/null
_read_status
assert_status "201"
download_file_to_file "$NODE1" "/integrity-1k.bin" "$DOWN_1K"
_read_status
assert_status "200"
assert_sha256_match "$UP_1K" "$DOWN_1K"
pass

# --- 64KB random binary (one WS chunk size) ---

test_name "64KB random binary upload/download SHA-256 match"
UP_64K="${_TMPDIR}/bin-64k.bin"
DOWN_64K="${_TMPDIR}/bin-64k-down.bin"
generate_random_binary "$UP_64K" 65536
upload_file_binary "$NODE1" "/integrity-64k.bin" "$UP_64K" >/dev/null
_read_status
assert_status "201"
download_file_to_file "$NODE1" "/integrity-64k.bin" "$DOWN_64K"
_read_status
assert_status "200"
assert_sha256_match "$UP_64K" "$DOWN_64K"
pass

# --- 1MB random binary ---

test_name "1MB random binary upload/download SHA-256 match"
UP_1M="${_TMPDIR}/bin-1m.bin"
DOWN_1M="${_TMPDIR}/bin-1m-down.bin"
generate_random_binary "$UP_1M" 1048576
upload_file_binary "$NODE1" "/integrity-1m.bin" "$UP_1M" >/dev/null
_read_status
assert_status "201"
download_file_to_file "$NODE1" "/integrity-1m.bin" "$DOWN_1M"
_read_status
assert_status "200"
assert_sha256_match "$UP_1M" "$DOWN_1M"
pass

# --- All-zeros 1KB ---

test_name "All-zeros 1KB binary SHA-256 match"
UP_ZERO="${_TMPDIR}/bin-zero.bin"
DOWN_ZERO="${_TMPDIR}/bin-zero-down.bin"
generate_zero_binary "$UP_ZERO" 1024
upload_file_binary "$NODE1" "/integrity-zero.bin" "$UP_ZERO" >/dev/null
_read_status
assert_status "201"
download_file_to_file "$NODE1" "/integrity-zero.bin" "$DOWN_ZERO"
_read_status
assert_status "200"
assert_sha256_match "$UP_ZERO" "$DOWN_ZERO"
pass

# --- All-0xFF 1KB ---

test_name "All-0xFF 1KB binary SHA-256 match"
UP_FF="${_TMPDIR}/bin-ff.bin"
DOWN_FF="${_TMPDIR}/bin-ff-down.bin"
generate_ff_binary "$UP_FF" 1024
upload_file_binary "$NODE1" "/integrity-ff.bin" "$UP_FF" >/dev/null
_read_status
assert_status "201"
download_file_to_file "$NODE1" "/integrity-ff.bin" "$DOWN_FF"
_read_status
assert_status "200"
assert_sha256_match "$UP_FF" "$DOWN_FF"
pass

# --- Repeating pattern 0x00-0xFF 1KB ---

test_name "Repeating pattern 0x00-0xFF 1KB binary SHA-256 match"
UP_PAT="${_TMPDIR}/bin-pattern.bin"
DOWN_PAT="${_TMPDIR}/bin-pattern-down.bin"
generate_pattern_binary "$UP_PAT" 1024
upload_file_binary "$NODE1" "/integrity-pattern.bin" "$UP_PAT" >/dev/null
_read_status
assert_status "201"
download_file_to_file "$NODE1" "/integrity-pattern.bin" "$DOWN_PAT"
_read_status
assert_status "200"
assert_sha256_match "$UP_PAT" "$DOWN_PAT"
pass

# --- Cross-server: upload NODE1, download NODE2 ---

test_name "Cross-server binary integrity: upload NODE1, download NODE2"
UP_CROSS="${_TMPDIR}/bin-cross.bin"
DOWN_CROSS="${_TMPDIR}/bin-cross-down.bin"
generate_random_binary "$UP_CROSS" 4096
upload_file_binary "$NODE1" "/integrity-cross.bin" "$UP_CROSS" >/dev/null
_read_status
assert_status "201"
download_file_to_file "$NODE2" "/integrity-cross.bin" "$DOWN_CROSS"
_read_status
assert_status "200"
assert_sha256_match "$UP_CROSS" "$DOWN_CROSS"
pass

# --- PUT overwrite with new binary ---

test_name "PUT overwrite: new binary SHA-256 matches after update"
UP_ORIG="${_TMPDIR}/bin-overwrite-orig.bin"
UP_NEW="${_TMPDIR}/bin-overwrite-new.bin"
DOWN_NEW="${_TMPDIR}/bin-overwrite-new-down.bin"
generate_random_binary "$UP_ORIG" 2048
generate_random_binary "$UP_NEW" 3072
upload_file_binary "$NODE1" "/integrity-overwrite.bin" "$UP_ORIG" >/dev/null
_read_status
assert_status "201"
callfs_curl PUT "${NODE1}/v1/files/integrity-overwrite.bin" \
  -H "Content-Type: application/octet-stream" \
  --data-binary "@${UP_NEW}" >/dev/null
_read_status
assert_status "200"
download_file_to_file "$NODE1" "/integrity-overwrite.bin" "$DOWN_NEW"
_read_status
assert_status "200"
assert_sha256_match "$UP_NEW" "$DOWN_NEW"
pass

# --- Erasure coding binary integrity ---

test_name "Erasure coded 4KB binary upload/download SHA-256 match"
UP_ERASURE="${_TMPDIR}/bin-erasure.bin"
DOWN_ERASURE="${_TMPDIR}/bin-erasure-down.bin"
generate_random_binary "$UP_ERASURE" 4096
callfs_curl POST "${NODE1}/v1/files/integrity-erasure.bin?erasure=true&data_shards=2&parity_shards=1" \
  -H "Content-Type: application/octet-stream" \
  --data-binary "@${UP_ERASURE}" >/dev/null
_read_status
assert_status "201"
download_file_to_file "$NODE1" "/integrity-erasure.bin" "$DOWN_ERASURE"
_read_status
assert_status "200"
assert_sha256_match "$UP_ERASURE" "$DOWN_ERASURE"
pass

# --- Cleanup ---

test_name "Cleanup binary integrity test files"
delete_file "$NODE1" "/integrity-1k.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/integrity-64k.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/integrity-1m.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/integrity-zero.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/integrity-ff.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/integrity-pattern.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/integrity-cross.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/integrity-overwrite.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/integrity-erasure.bin" >/dev/null 2>&1 || true
pass

print_summary
exit $?
