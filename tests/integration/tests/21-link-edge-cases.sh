#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Single-Use Link Edge Cases"

# ---------- Link for non-existent file ----------

test_name "Generate link for non-existent file returns error"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/does-not-exist-link.txt","expiry_seconds":3600}')
# Should return 404 or 500 (file not found)
if [ "$LAST_STATUS" = "404" ] || [ "$LAST_STATUS" = "500" ]; then
  pass
else
  fail "expected 404 or 500 for link to non-existent file, got $LAST_STATUS"
fi

# ---------- Link for directory ----------

test_name "Setup: create directory for link test"
create_directory "$NODE1" "/link-dir-test/"
assert_status 201
pass

test_name "Generate link for directory path"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-dir-test/","expiry_seconds":3600}')
# Should fail - links are for files
if [ "$LAST_STATUS" != "201" ]; then
  pass
else
  fail "expected error generating link for directory, got 201"
fi

# ---------- Link with boundary expiry values ----------

test_name "Generate link with expiry_seconds=1 (minimum)"
upload_file "$NODE1" "/link-edge.txt" "link edge content"
assert_status 201
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-edge.txt","expiry_seconds":1}')
assert_status 201
pass

test_name "Generate link with expiry_seconds=86400 (maximum, 24h)"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-edge.txt","expiry_seconds":86400}')
assert_status 201
pass

# ---------- Multiple links for same file ----------

test_name "Generate two links for same file, both work"
BODY1=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-edge.txt","expiry_seconds":3600}')
assert_status 201
TOKEN1=$(echo "$BODY1" | jq -r '.token')

BODY2=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-edge.txt","expiry_seconds":3600}')
assert_status 201
TOKEN2=$(echo "$BODY2" | jq -r '.token')

# Both should work independently
DL1=$(callfs_curl_noauth GET "${NODE1}/download/${TOKEN1}")
assert_status 200
assert_body_equals "$DL1" "link edge content"

DL2=$(callfs_curl_noauth GET "${NODE1}/download/${TOKEN2}")
assert_status 200
assert_body_equals "$DL2" "link edge content"
pass

# ---------- Link after file is deleted ----------

test_name "Generate link, delete file, download returns error"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/link-edge.txt","expiry_seconds":3600}')
assert_status 201
TOKEN=$(echo "$BODY" | jq -r '.token')

delete_file "$NODE1" "/link-edge.txt"
assert_status 204

BODY=$(callfs_curl_noauth GET "${NODE1}/download/${TOKEN}")
# Should fail - file no longer exists
if [ "$LAST_STATUS" != "200" ]; then
  pass
else
  fail "expected error downloading via link after file deletion, got 200"
fi

# ---------- Link with path traversal attempt ----------

test_name "Generate link with path traversal in path field returns error"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/../../../etc/passwd","expiry_seconds":3600}')
if [ "$LAST_STATUS" = "400" ] || [ "$LAST_STATUS" = "404" ] || [ "$LAST_STATUS" = "500" ]; then
  pass
else
  fail "expected error for path traversal in link generation, got $LAST_STATUS"
fi

# ---------- Link with malformed JSON ----------

test_name "Generate link with malformed JSON returns 400"
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d 'not json at all')
assert_status 400
pass

# ---------- Link with extra large JSON body ----------

test_name "Generate link with oversized JSON body returns 400"
BIG_PATH=$(printf 'a%.0s' $(seq 1 5000))
BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d "{\"path\":\"/${BIG_PATH}\",\"expiry_seconds\":3600}")
assert_status 400
pass

# ---------- Cross-server link download ----------

test_name "Generate link on NODE1, download from NODE2"
upload_file "$NODE1" "/cross-link.txt" "cross link content"
assert_status 201

BODY=$(callfs_curl POST "${NODE1}/v1/links/generate" \
  -H "Content-Type: application/json" \
  -d '{"path":"/cross-link.txt","expiry_seconds":3600}')
assert_status 201
TOKEN=$(echo "$BODY" | jq -r '.token')

BODY=$(callfs_curl_noauth GET "${NODE2}/download/${TOKEN}")
assert_status 200
assert_body_equals "$BODY" "cross link content"
pass

# ---------- Cleanup ----------

test_name "Cleanup link edge case test files"
delete_file "$NODE1" "/link-dir-test" >/dev/null 2>&1 || true
delete_file "$NODE1" "/link-edge.txt" >/dev/null 2>&1 || true
delete_file "$NODE1" "/cross-link.txt" >/dev/null 2>&1 || true
pass

print_summary
exit $?
