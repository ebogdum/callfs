#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Large File Upload/Download"

test_name "Generate 5MB test file"
generate_random_binary "${_TMPDIR}/large-5m.bin" 5242880
pass

test_name "Upload 5MB file to /large-5m.bin"
upload_file_binary "$NODE1" "/large-5m.bin" "${_TMPDIR}/large-5m.bin"
assert_status 201
pass

test_name "HEAD /large-5m.bin shows correct size (5242880)"
callfs_head_method "${NODE1}/v1/files/large-5m.bin"
assert_status 200
assert_header_contains "X-CallFS-Size" "5242880"
pass

test_name "Download 5MB file and SHA-256 match"
download_file_to_file "$NODE1" "/large-5m.bin" "${_TMPDIR}/large-5m-output.bin"
assert_status 200
assert_sha256_match "${_TMPDIR}/large-5m.bin" "${_TMPDIR}/large-5m-output.bin"
pass

test_name "Generate 10MB test file"
generate_random_binary "${_TMPDIR}/large-10m.bin" 10485760
pass

test_name "Upload 10MB file to /large-10m.bin"
upload_file_binary "$NODE1" "/large-10m.bin" "${_TMPDIR}/large-10m.bin"
assert_status 201
pass

test_name "Download 10MB file and SHA-256 match"
download_file_to_file "$NODE1" "/large-10m.bin" "${_TMPDIR}/large-10m-output.bin"
assert_status 200
assert_sha256_match "${_TMPDIR}/large-10m.bin" "${_TMPDIR}/large-10m-output.bin"
pass

test_name "Cross-server 5MB: upload NODE1, download from NODE2, SHA-256 match"
download_file_to_file "$NODE2" "/large-5m.bin" "${_TMPDIR}/large-5m-cross.bin"
assert_status 200
assert_sha256_match "${_TMPDIR}/large-5m.bin" "${_TMPDIR}/large-5m-cross.bin"
pass

test_name "Cleanup large files from server"
delete_file "$NODE1" "/large-5m.bin" >/dev/null 2>&1 || true
delete_file "$NODE1" "/large-10m.bin" >/dev/null 2>&1 || true
pass

print_summary
exit $?
