#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Rename and Move (PATCH)"

patch_files() {
  # patch_files <node> <src-path> <json-body>
  callfs_curl PATCH "${1}/v1/files$(_strip_slash_path "$2")" \
    -H "Content-Type: application/json" \
    -d "$3"
}

_strip_slash_path() { echo "/${1#/}"; }

# --- Rename a file in place ---

test_name "PATCH rename file in place returns 200 and new path"
upload_file "$NODE1" "/rn-src.txt" "rename me" >/dev/null
BODY=$(patch_files "$NODE1" "/rn-src.txt" '{"name":"rn-dst.txt"}')
_read_status
assert_status "200"
assert_json_field_equals "$BODY" "path" "/rn-dst.txt"
pass

test_name "renamed file is readable at new path, gone from old"
BODY=$(download_file "$NODE1" "/rn-dst.txt"); _read_status
assert_status "200"
assert_body_equals "$BODY" "rename me"
download_file "$NODE1" "/rn-src.txt" >/dev/null; _read_status
assert_status "404"
pass

# --- Move a file to a new folder with create_parents ---

test_name "PATCH move with create_parents creates folder and moves file"
patch_files "$NODE1" "/rn-dst.txt" '{"destination":"/moved/dir/final.txt","create_parents":true}' >/dev/null
_read_status
assert_status "200"
BODY=$(download_file "$NODE1" "/moved/dir/final.txt"); _read_status
assert_status "200"
assert_body_equals "$BODY" "rename me"
pass

# --- No-clobber and overwrite ---

test_name "PATCH move onto existing destination returns 409 without overwrite"
upload_file "$NODE1" "/clobber-a.txt" "AAA" >/dev/null
upload_file "$NODE1" "/clobber-b.txt" "BBB" >/dev/null
patch_files "$NODE1" "/clobber-a.txt" '{"destination":"/clobber-b.txt"}' >/dev/null
_read_status
assert_status "409"
pass

test_name "PATCH move with overwrite:true replaces destination"
patch_files "$NODE1" "/clobber-a.txt" '{"destination":"/clobber-b.txt","overwrite":true}' >/dev/null
_read_status
assert_status "200"
BODY=$(download_file "$NODE1" "/clobber-b.txt"); _read_status
assert_body_equals "$BODY" "AAA"
pass

# --- Invalid requests ---

test_name "PATCH with neither name nor destination returns 400"
upload_file "$NODE1" "/bad.txt" "x" >/dev/null
patch_files "$NODE1" "/bad.txt" '{}' >/dev/null
_read_status
assert_status "400"
pass

test_name "PATCH with both name and destination returns 400"
patch_files "$NODE1" "/bad.txt" '{"name":"y.txt","destination":"/z.txt"}' >/dev/null
_read_status
assert_status "400"
pass

test_name "PATCH on missing source returns 404"
patch_files "$NODE1" "/does-not-exist.txt" '{"name":"whatever.txt"}' >/dev/null
_read_status
assert_status "404"
pass

# --- Directory move (subtree) ---

test_name "PATCH move directory relocates the whole subtree"
upload_file "$NODE1" "/srcdir/inner/deep.txt" "deepdata" >/dev/null
patch_files "$NODE1" "/srcdir" '{"destination":"/dstdir"}' >/dev/null
_read_status
assert_status "200"
BODY=$(download_file "$NODE1" "/dstdir/inner/deep.txt"); _read_status
assert_status "200"
assert_body_equals "$BODY" "deepdata"
download_file "$NODE1" "/srcdir/inner/deep.txt" >/dev/null; _read_status
assert_status "404"
pass

# Cleanup
for p in /rn-dst.txt /moved/dir/final.txt /clobber-b.txt /bad.txt \
         /dstdir/inner/deep.txt; do
  callfs_curl DELETE "${NODE1}/v1/files${p}" >/dev/null 2>&1 || true
done
for d in /moved/dir /moved /dstdir/inner /dstdir; do
  callfs_curl DELETE "${NODE1}/v1/files${d}" >/dev/null 2>&1 || true
done

print_summary
exit $?
