#!/usr/bin/env bash
# Shared test helpers for CallFS integration tests
set -euo pipefail

# ---------- Configuration ----------
API_KEY="test-api-key-integration-0123456"
NODE1="http://node1:8443"
NODE2="http://node2:8443"
NODE3="http://node3:8443"
CURL_TIMEOUT=10

# ---------- Counters ----------
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_TOTAL=0
CURRENT_TEST=""
LAST_STATUS=""
LAST_HEADERS=""

# ---------- Output helpers ----------
section() {
  echo ""
  echo "=== $1 ==="
}

test_name() {
  CURRENT_TEST="$1"
  TESTS_TOTAL=$((TESTS_TOTAL + 1))
}

pass() {
  TESTS_PASSED=$((TESTS_PASSED + 1))
  echo "  PASS: ${CURRENT_TEST}"
}

fail() {
  TESTS_FAILED=$((TESTS_FAILED + 1))
  local msg="${1:-}"
  if [ -n "$msg" ]; then
    echo "  FAIL: ${CURRENT_TEST} -- $msg"
  else
    echo "  FAIL: ${CURRENT_TEST}"
  fi
}

print_summary() {
  echo ""
  echo "========================================"
  echo "  TOTAL:  ${TESTS_TOTAL}"
  echo "  PASSED: ${TESTS_PASSED}"
  echo "  FAILED: ${TESTS_FAILED}"
  echo "========================================"
  [ "$TESTS_FAILED" -eq 0 ]
}

# ---------- HTTP helpers ----------
# Status and headers are written to temp files so they survive subshell captures
# like BODY=$(callfs_curl ...). The assert_* helpers read from these files.
_TMPDIR=$(mktemp -d)
_STATUS_FILE="${_TMPDIR}/status"
_HEADERS_FILE="${_TMPDIR}/headers"
_RESP_FILE="${_TMPDIR}/resp"
trap "rm -rf $_TMPDIR" EXIT

_read_status() {
  LAST_STATUS=$(cat "$_STATUS_FILE" 2>/dev/null) || LAST_STATUS=""
}

_read_headers() {
  LAST_HEADERS=$(cat "$_HEADERS_FILE" 2>/dev/null) || LAST_HEADERS=""
  LAST_STATUS=$(head -1 "$_HEADERS_FILE" 2>/dev/null | grep -oE '[0-9]{3}' | head -1) || LAST_STATUS=""
}

# Generic curl wrapper. Returns body on stdout. Status saved to file.
# Usage: BODY=$(callfs_curl GET "http://node1:8443/v1/files/test.txt"); _read_status
callfs_curl() {
  local method="$1"
  local url="$2"
  shift 2
  curl -s -w '\n%{http_code}' \
    --max-time "$CURL_TIMEOUT" \
    -X "$method" \
    -H "Authorization: Bearer ${API_KEY}" \
    "$@" \
    "$url" 2>/dev/null > "$_RESP_FILE" || true
  tail -1 "$_RESP_FILE" > "$_STATUS_FILE"
  sed '$d' "$_RESP_FILE"
}

# Curl without auth header
callfs_curl_noauth() {
  local method="$1"
  local url="$2"
  shift 2
  curl -s -w '\n%{http_code}' \
    --max-time "$CURL_TIMEOUT" \
    -X "$method" \
    "$@" \
    "$url" 2>/dev/null > "$_RESP_FILE" || true
  tail -1 "$_RESP_FILE" > "$_STATUS_FILE"
  sed '$d' "$_RESP_FILE"
}

# Get response headers via GET. Headers and status saved to file.
callfs_head() {
  local url="$1"
  shift
  curl -s -D "$_HEADERS_FILE" -o /dev/null \
    --max-time "$CURL_TIMEOUT" \
    -H "Authorization: Bearer ${API_KEY}" \
    "$@" \
    "$url" 2>/dev/null || true
  _read_headers
}

# HEAD request (actual HTTP HEAD method). Headers and status saved to file.
callfs_head_method() {
  local url="$1"
  shift
  curl -s -I \
    --max-time "$CURL_TIMEOUT" \
    -H "Authorization: Bearer ${API_KEY}" \
    "$@" \
    "$url" 2>/dev/null > "$_HEADERS_FILE" || true
  _read_headers
}

# ---------- Assertion helpers ----------
# These call fail() on failure and are no-ops on success.
# Use pass() explicitly at the end of each test block for the success counter.
# They always return 0 to avoid tripping set -e.

assert_status() {
  _read_status
  local expected="$1"
  if [ "$LAST_STATUS" != "$expected" ]; then
    fail "expected status $expected, got $LAST_STATUS"
  fi
  return 0
}

assert_body_contains() {
  local body="$1"
  local expected="$2"
  if ! echo "$body" | grep -qF "$expected"; then
    fail "body missing '$expected'"
  fi
  return 0
}

assert_body_equals() {
  local body="$1"
  local expected="$2"
  if [ "$body" != "$expected" ]; then
    fail "expected body '$expected', got '$body'"
  fi
  return 0
}

assert_header_contains() {
  _read_headers
  local name="$1"
  local expected="$2"
  local val
  val=$(echo "$LAST_HEADERS" | grep -i "^${name}:" | head -1 | sed "s/^${name}: *//i" | tr -d '\r') || true
  if ! echo "$val" | grep -qF "$expected"; then
    fail "header $name='$val' missing '$expected'"
  fi
  return 0
}

assert_header_present() {
  _read_headers
  local name="$1"
  if ! echo "$LAST_HEADERS" | grep -qi "^${name}:"; then
    fail "header $name not present"
  fi
  return 0
}

# ---------- File operation helpers ----------
# All path args are stripped of leading slash to avoid double-slash in URLs.

_strip_slash() { echo "${1#/}"; }

upload_file() {
  local node="$1"
  local path
  path=$(_strip_slash "$2")
  local content="$3"
  callfs_curl POST "${node}/v1/files/${path}" \
    -H "Content-Type: application/octet-stream" \
    -d "$content"
}

upload_file_binary() {
  local node="$1"
  local path
  path=$(_strip_slash "$2")
  local file="$3"
  callfs_curl POST "${node}/v1/files/${path}" \
    -H "Content-Type: application/octet-stream" \
    --data-binary "@${file}"
}

download_file() {
  local node="$1"
  local path
  path=$(_strip_slash "$2")
  callfs_curl GET "${node}/v1/files/${path}"
}

delete_file() {
  local node="$1"
  local path
  path=$(_strip_slash "$2")
  callfs_curl DELETE "${node}/v1/files/${path}"
}

create_directory() {
  local node="$1"
  local path
  path=$(_strip_slash "$2")
  # Ensure trailing slash for directory
  [[ "$path" == */ ]] || path="${path}/"
  callfs_curl POST "${node}/v1/files/${path}" \
    -H "Content-Type: application/octet-stream"
}

list_directory() {
  local node="$1"
  local path
  path=$(_strip_slash "$2")
  shift 2
  callfs_curl GET "${node}/v1/directories/${path}" "$@"
}

# ---------- Wait helpers ----------

wait_ready() {
  local url="$1"
  local max_wait="${2:-60}"
  local elapsed=0
  while [ $elapsed -lt "$max_wait" ]; do
    if curl -sf --max-time 2 "${url}/health" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  echo "ERROR: ${url} not ready after ${max_wait}s"
  return 1
}

wait_all_nodes() {
  echo "Waiting for nodes to become ready..."
  wait_ready "$NODE1" 90
  echo "  node1: ready"
  wait_ready "$NODE2" 90
  echo "  node2: ready"
  wait_ready "$NODE3" 90
  echo "  node3: ready"
  # Give raft a moment to elect leader
  sleep 3
  echo "  raft: leader election window passed"
}

# ---------- Cleanup ----------

cleanup_test_files() {
  # Best-effort cleanup of common test paths
  for p in test.txt hello.txt update.txt zero.txt large.txt chunked.txt \
           erasure-test.txt cross-test.txt raft-test1.txt raft-test2.txt \
           link-test.txt ws-download.txt ws-upload.txt security-test.txt; do
    callfs_curl DELETE "${NODE1}/v1/files/${p}" >/dev/null 2>&1 || true
  done
  for d in testdir a; do
    callfs_curl DELETE "${NODE1}/v1/files/${d}" >/dev/null 2>&1 || true
  done
}
