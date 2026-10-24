#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Object Store (ETag, conditional writes, ranges, objects API)"

etag_from_headers() {
  echo "$LAST_HEADERS" | grep -i '^etag:' | head -1 | sed 's/^[Ee][Tt][Aa][Gg]: *//' | tr -d '\r' || true
}

test_name "POST /obj-a.txt returns 201 with ETag header"
callfs_curl_with_headers POST "${NODE1}/v1/files/obj-a.txt" \
  -H "Content-Type: application/octet-stream" \
  -d "version one" >/dev/null
assert_status 201
assert_header_present "ETag"
pass

test_name "HEAD and GET /obj-a.txt return the same ETag"
callfs_head_method "${NODE1}/v1/files/obj-a.txt"
HEAD_ETAG=$(etag_from_headers)
callfs_curl_with_headers GET "${NODE1}/v1/files/obj-a.txt" >/dev/null
GET_ETAG=$(etag_from_headers)
if [ -n "$HEAD_ETAG" ] && [ "$HEAD_ETAG" = "$GET_ETAG" ]; then
  pass
else
  fail "HEAD ETag '${HEAD_ETAG}' vs GET ETag '${GET_ETAG}'"
fi

test_name "POST /obj-new.txt with If-None-Match: * returns 201"
callfs_curl POST "${NODE1}/v1/files/obj-new.txt" \
  -H "If-None-Match: *" \
  -H "Content-Type: application/octet-stream" \
  -d "created once" >/dev/null
_read_status
assert_status 201
pass

test_name "second POST /obj-new.txt with If-None-Match: * returns 412, content unchanged"
callfs_curl POST "${NODE1}/v1/files/obj-new.txt" \
  -H "If-None-Match: *" \
  -H "Content-Type: application/octet-stream" \
  -d "second" >/dev/null
_read_status
assert_status 412
BODY=$(download_file "$NODE1" "/obj-new.txt")
assert_status 200
assert_body_equals "$BODY" "created once"
pass

test_name "PUT /obj-a.txt with current If-Match ETag returns 200"
callfs_head_method "${NODE1}/v1/files/obj-a.txt"
ETAG_V1=$(etag_from_headers)
callfs_curl PUT "${NODE1}/v1/files/obj-a.txt" \
  -H "If-Match: ${ETAG_V1}" \
  -H "Content-Type: application/octet-stream" \
  -d "version two" >/dev/null
_read_status
assert_status 200
pass

test_name "PUT /obj-a.txt with stale If-Match ETag returns 412, content unchanged"
callfs_curl PUT "${NODE1}/v1/files/obj-a.txt" \
  -H "If-Match: ${ETAG_V1}" \
  -H "Content-Type: application/octet-stream" \
  -d "stale write" >/dev/null
_read_status
assert_status 412
BODY=$(download_file "$NODE1" "/obj-a.txt")
assert_status 200
assert_body_equals "$BODY" "version two"
pass

test_name "GET /obj-a.txt with Range bytes=0-6 returns 206 with Content-Range"
BODY=$(callfs_curl_with_headers GET "${NODE1}/v1/files/obj-a.txt" -H "Range: bytes=0-6")
assert_status 206
assert_header_contains "Content-Range" "bytes 0-6/11"
assert_body_equals "$BODY" "version"
pass

test_name "GET /obj-a.txt with unsatisfiable Range returns 416"
callfs_curl GET "${NODE1}/v1/files/obj-a.txt" -H "Range: bytes=500-600" >/dev/null
_read_status
assert_status 416
pass

test_name "Create three files under /objs/p/"
for f in one two three; do
  upload_file "$NODE1" "/objs/p/${f}.txt" "$f" >/dev/null
  _read_status
  [ "$LAST_STATUS" = "201" ] || fail "POST /objs/p/${f}.txt returned ${LAST_STATUS}"
done
pass

test_name "GET /v1/objects prefix limit=2 returns first page with next_start_after"
BODY=$(callfs_curl GET "${NODE1}/v1/objects?prefix=/objs/p/&limit=2")
assert_status 200
assert_json_field_equals "$BODY" "truncated" "true"
assert_json_field_equals "$BODY" "objects[0].path" "/objs/p/one.txt"
assert_json_field_equals "$BODY" "objects[1].path" "/objs/p/three.txt"
assert_json_field_equals "$BODY" "next_start_after" "/objs/p/three.txt"
pass

test_name "GET /v1/objects with start_after continues to the next page"
BODY=$(callfs_curl GET "${NODE1}/v1/objects?prefix=/objs/p/&limit=2&start_after=/objs/p/three.txt")
assert_status 200
assert_json_field_equals "$BODY" "truncated" "false"
assert_json_field_equals "$BODY" "objects[0].path" "/objs/p/two.txt"
pass

test_name "DELETE /v1/objects?prefix=/objs/p/ returns deleted count 3"
BODY=$(callfs_curl DELETE "${NODE1}/v1/objects?prefix=/objs/p/")
assert_status 200
assert_json_field_equals "$BODY" "deleted" "3"
pass

test_name "DELETE /v1/files/rdir?recursive=true removes nested files"
create_directory "$NODE1" "/rdir/" >/dev/null
create_directory "$NODE1" "/rdir/sub/" >/dev/null
upload_file "$NODE1" "/rdir/sub/x.txt" "nested" >/dev/null
callfs_curl DELETE "${NODE1}/v1/files/rdir?recursive=true" >/dev/null
_read_status
if [[ "$LAST_STATUS" != 2* ]]; then
  fail "recursive DELETE returned ${LAST_STATUS}"
fi
download_file "$NODE1" "/rdir/sub/x.txt" >/dev/null
_read_status
assert_status 404
pass

delete_file "$NODE1" "/obj-a.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/obj-new.txt" >/dev/null 2>&1 || true

print_summary
exit $?
