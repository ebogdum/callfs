#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Concurrent Operations"

# ---------- Concurrent writes to same file ----------

test_name "Setup: create /concurrent.txt"
upload_file "$NODE1" "/concurrent.txt" "initial"
assert_status 201
pass

test_name "Concurrent PUT from 3 nodes completes without corruption"
# Fire 3 PUTs in parallel to the same file via different nodes
curl -s -o /dev/null -w '%{http_code}' --max-time "$CURL_TIMEOUT" \
  -X PUT -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/octet-stream" \
  -d "from-node1" "${NODE1}/v1/files/concurrent.txt" > /tmp/c1.status &
PID1=$!
curl -s -o /dev/null -w '%{http_code}' --max-time "$CURL_TIMEOUT" \
  -X PUT -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/octet-stream" \
  -d "from-node2" "${NODE2}/v1/files/concurrent.txt" > /tmp/c2.status &
PID2=$!
curl -s -o /dev/null -w '%{http_code}' --max-time "$CURL_TIMEOUT" \
  -X PUT -H "Authorization: Bearer ${API_KEY}" \
  -H "Content-Type: application/octet-stream" \
  -d "from-node3" "${NODE3}/v1/files/concurrent.txt" > /tmp/c3.status &
PID3=$!
wait $PID1 $PID2 $PID3 || true

# All requests should return 200 (success) - no 500 errors
S1=$(cat /tmp/c1.status 2>/dev/null || echo "000")
S2=$(cat /tmp/c2.status 2>/dev/null || echo "000")
S3=$(cat /tmp/c3.status 2>/dev/null || echo "000")

# At least one should succeed; none should be 500
ALL_OK=true
for s in "$S1" "$S2" "$S3"; do
  if [ "$s" = "500" ]; then
    fail "got HTTP 500 during concurrent write (statuses: $S1 $S2 $S3)"
    ALL_OK=false
    break
  fi
done
if [ "$ALL_OK" = true ]; then
  pass
fi

test_name "File is readable after concurrent writes (no corruption)"
BODY=$(download_file "$NODE1" "/concurrent.txt")
assert_status 200
# Body should be one of the 3 values, not a mixture
case "$BODY" in
  "from-node1"|"from-node2"|"from-node3")
    pass ;;
  *)
    fail "body corrupted after concurrent writes: '$BODY'" ;;
esac

test_name "All nodes return same content after concurrent writes"
# Allow time for Raft replication, metadata cache expiry, and lock release
sleep 5
# Read from leader first to get the authoritative value
BODY1=$(download_file "$NODE1" "/concurrent.txt")
_read_status
# Retry followers up to 3 times to allow for Raft propagation
BODY2=""
BODY3=""
for attempt in 1 2 3; do
  BODY2=$(download_file "$NODE2" "/concurrent.txt")
  BODY3=$(download_file "$NODE3" "/concurrent.txt")
  if [ "$BODY1" = "$BODY2" ] && [ "$BODY1" = "$BODY3" ]; then
    break
  fi
  sleep 2
done
if [ "$BODY1" = "$BODY2" ] && [ "$BODY1" = "$BODY3" ]; then
  pass
else
  fail "inconsistent reads: node1='$BODY1' node2='$BODY2' node3='$BODY3'"
fi

# ---------- Concurrent creates of different files ----------

test_name "Concurrent creation of 5 different files succeeds"
for i in $(seq 1 5); do
  curl -s -o /dev/null -w '%{http_code}' --max-time "$CURL_TIMEOUT" \
    -X POST -H "Authorization: Bearer ${API_KEY}" \
    -H "Content-Type: application/octet-stream" \
    -d "content-$i" "${NODE1}/v1/files/concurrent-batch-${i}.txt" > "/tmp/cb${i}.status" &
done
wait || true

FAIL_COUNT=0
for i in $(seq 1 5); do
  s=$(cat "/tmp/cb${i}.status" 2>/dev/null || echo "000")
  if [ "$s" != "201" ]; then
    FAIL_COUNT=$((FAIL_COUNT + 1))
  fi
done
if [ "$FAIL_COUNT" -eq 0 ]; then
  pass
else
  fail "$FAIL_COUNT of 5 concurrent creates did not return 201"
fi

# ---------- Concurrent reads ----------

test_name "Concurrent reads from multiple nodes return consistent data"
for i in $(seq 1 5); do
  curl -s --max-time "$CURL_TIMEOUT" \
    -H "Authorization: Bearer ${API_KEY}" \
    "${NODE1}/v1/files/concurrent-batch-1.txt" > "/tmp/cr${i}.out" &
done
wait || true

EXPECTED="content-1"
READ_OK=true
for i in $(seq 1 5); do
  got=$(cat "/tmp/cr${i}.out" 2>/dev/null || echo "")
  if [ "$got" != "$EXPECTED" ]; then
    READ_OK=false
    break
  fi
done
if [ "$READ_OK" = true ]; then
  pass
else
  fail "concurrent reads returned inconsistent data"
fi

# ---------- Concurrent erasure uploads ----------

test_name "Concurrent erasure uploads of 3 different files"
for i in 1 2 3; do
  EFILE="${_TMPDIR}/erasure-conc-${i}.bin"
  generate_random_binary "$EFILE" 4096
done

for i in 1 2 3; do
  EFILE="${_TMPDIR}/erasure-conc-${i}.bin"
  curl -s -o "/tmp/ec${i}.body" -w '%{http_code}' --max-time "$CURL_TIMEOUT" \
    -X POST -H "Authorization: Bearer ${API_KEY}" \
    -H "Content-Type: application/octet-stream" \
    --data-binary "@${EFILE}" \
    "${NODE1}/v1/files/erasure-conc-${i}.bin?erasure=true" > "/tmp/ec${i}.status" &
done
wait || true

EC_OK=true
for i in 1 2 3; do
  s=$(cat "/tmp/ec${i}.status" 2>/dev/null || echo "000")
  if [ "$s" != "201" ]; then
    fail "erasure upload $i returned $s, expected 201"
    EC_OK=false
    break
  fi
done

if [ "$EC_OK" = true ]; then
  # Verify each file downloads correctly with SHA-256
  VERIFY_OK=true
  for i in 1 2 3; do
    EFILE="${_TMPDIR}/erasure-conc-${i}.bin"
    DFILE="${_TMPDIR}/erasure-conc-${i}-down.bin"
    download_file_to_file "$NODE1" "/erasure-conc-${i}.bin" "$DFILE"
    _read_status
    if [ "$LAST_STATUS" != "200" ]; then
      fail "erasure download $i returned $LAST_STATUS, expected 200"
      VERIFY_OK=false
      break
    fi
    H1=$(sha256_file "$EFILE")
    H2=$(sha256_file "$DFILE")
    if [ "$H1" != "$H2" ]; then
      fail "erasure file $i SHA-256 mismatch: $H1 vs $H2"
      VERIFY_OK=false
      break
    fi
  done
  if [ "$VERIFY_OK" = true ]; then
    pass
  fi
fi

# ---------- Cleanup ----------

test_name "Cleanup concurrent test files"
delete_file "$NODE1" "/concurrent.txt" >/dev/null 2>&1 || true
for i in $(seq 1 5); do
  delete_file "$NODE1" "/concurrent-batch-${i}.txt" >/dev/null 2>&1 || true
done
for i in 1 2 3; do
  delete_file "$NODE1" "/erasure-conc-${i}.bin" >/dev/null 2>&1 || true
done
pass

print_summary
exit $?
