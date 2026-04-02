#!/usr/bin/env bash
# CallFS Integration Test Runner
# Usage: ./run-tests.sh [--keep] [--filter PATTERN] [--skip-build]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
KEEP=false
FILTER=""
SKIP_BUILD=false

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --keep) KEEP=true; shift ;;
    --filter) FILTER="$2"; shift 2 ;;
    --skip-build) SKIP_BUILD=true; shift ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

# ---------- Build ----------
if [ "$SKIP_BUILD" = false ]; then
  echo "==> Building CallFS Docker image..."
  docker build -t callfs:test -f "${PROJECT_ROOT}/Dockerfile" "${PROJECT_ROOT}"
  echo "==> Build complete"
fi

# ---------- Cleanup on exit ----------
cleanup() {
  if [ "$KEEP" = true ]; then
    echo ""
    echo "==> Containers left running (--keep). To stop:"
    echo "    cd ${SCRIPT_DIR} && docker compose down -v"
  else
    echo ""
    echo "==> Tearing down containers..."
    cd "${SCRIPT_DIR}" && docker compose down -v --remove-orphans 2>/dev/null || true
  fi
}
trap cleanup EXIT

# ---------- Start infrastructure ----------
echo "==> Starting infrastructure (postgres, redis)..."
cd "${SCRIPT_DIR}"
docker compose up -d postgres redis
echo "==> Waiting for postgres and redis..."
docker compose exec -T postgres sh -c 'until pg_isready -U callfs -d callfs; do sleep 1; done' 2>/dev/null
docker compose exec -T redis sh -c 'until redis-cli ping 2>/dev/null | grep -q PONG; do sleep 1; done' 2>/dev/null
echo "==> Infrastructure ready"

# ---------- Start 3-node Raft cluster ----------
echo "==> Starting 3-node CallFS cluster..."
docker compose up -d node1 node2 node3

echo "==> Waiting for nodes to become healthy..."
for node in node1 node2 node3; do
  elapsed=0
  while [ $elapsed -lt 90 ]; do
    if docker compose exec -T "$node" /callfs-healthcheck 2>/dev/null; then
      echo "  ${node}: ready"
      break
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  if [ $elapsed -ge 90 ]; then
    echo "ERROR: ${node} not ready after 90s"
    docker compose logs "$node" | tail -30
    exit 1
  fi
done

# Give raft time to elect leader and stabilize
echo "==> Waiting for Raft leader election..."
sleep 5
echo "==> Cluster ready"

# ---------- Run cluster tests (01-12, 14-15) ----------
TOTAL_PASS=0
TOTAL_FAIL=0
FAILED_SUITES=""

run_test() {
  local test_file="$1"
  local test_name
  test_name=$(basename "$test_file" .sh)

  echo ""
  echo "============================================"
  echo "  Running: ${test_name}"
  echo "============================================"

  if docker compose exec -T node1 sh -c "cat > /tmp/lib.sh" < "${SCRIPT_DIR}/lib.sh" && \
     docker compose exec -T node1 sh -c "cat > /tmp/test.sh && chmod +x /tmp/test.sh" < "$test_file" && \
     docker compose exec -T node1 sh -c 'cd /tmp && /bin/sh -c "
       # Install test dependencies
       apk add --no-cache bash curl jq >/dev/null 2>&1 || true
       bash /tmp/test.sh
     "'; then
    TOTAL_PASS=$((TOTAL_PASS + 1))
    echo "  Suite ${test_name}: OK"
  else
    TOTAL_FAIL=$((TOTAL_FAIL + 1))
    FAILED_SUITES="${FAILED_SUITES} ${test_name}"
    echo "  Suite ${test_name}: FAILED"
  fi
}

# The scratch image has no shell. We need to run tests from outside the container.
# Use a temporary test runner container on the same network instead.
run_test_external() {
  local test_file="$1"
  local test_name
  test_name=$(basename "$test_file" .sh)

  echo ""
  echo "============================================"
  echo "  Running: ${test_name}"
  echo "============================================"

  # Mount lib.sh at /tests/lib.sh and test script at /tests/tests/test.sh
  # so the relative source path "${SCRIPT_DIR}/../lib.sh" resolves correctly
  if docker run --rm \
    --network callfs-test-network \
    -v "${SCRIPT_DIR}/lib.sh:/tests/lib.sh:ro" \
    -v "${test_file}:/tests/tests/test.sh:ro" \
    alpine/curl:latest \
    sh -c '
      apk add --no-cache bash jq coreutils >/dev/null 2>&1
      bash /tests/tests/test.sh
    '; then
    TOTAL_PASS=$((TOTAL_PASS + 1))
    echo "  Suite ${test_name}: OK"
  else
    TOTAL_FAIL=$((TOTAL_FAIL + 1))
    FAILED_SUITES="${FAILED_SUITES} ${test_name}"
    echo "  Suite ${test_name}: FAILED"
  fi
}

# Run cluster tests
for test_file in "${SCRIPT_DIR}/tests/"*.sh; do
  test_name=$(basename "$test_file" .sh)

  # Skip metadata backend test (runs separately)
  if [ "$test_name" = "13-metadata-backends" ]; then
    continue
  fi

  # Apply filter
  if [ -n "$FILTER" ] && ! echo "$test_name" | grep -q "$FILTER"; then
    continue
  fi

  run_test_external "$test_file"
done

# ---------- Run metadata backend tests (13) ----------
if [ -z "$FILTER" ] || echo "13-metadata-backends" | grep -q "$FILTER"; then
  echo ""
  echo "============================================"
  echo "  Metadata Backend Tests"
  echo "============================================"

  for backend in sqlite postgres redis; do
    echo ""
    echo "--- Testing metadata backend: ${backend} ---"

    # Stop all CallFS nodes
    docker compose stop node1 node2 node3 2>/dev/null || true

    # Clear any leftover data
    docker compose rm -f node1 node2 node3 2>/dev/null || true

    # If postgres backend, reset the database
    if [ "$backend" = "postgres" ]; then
      docker compose exec -T postgres psql -U callfs -d callfs -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;" 2>/dev/null || true
    fi

    # If redis backend, flush redis
    if [ "$backend" = "redis" ]; then
      docker compose exec -T redis redis-cli FLUSHALL 2>/dev/null || true
    fi

    # Start node1 with the backend-specific config
    config_file="${SCRIPT_DIR}/configs/node-${backend}.yaml"
    if [ ! -f "$config_file" ]; then
      echo "  SKIP: config file not found: ${config_file}"
      continue
    fi

    # Run node1 with the specific config
    docker run -d \
      --name callfs-meta-test \
      --network callfs-test-network \
      -v "${config_file}:/config.yaml:ro" \
      --tmpfs /data \
      callfs:test \
      server --config /config.yaml

    # Wait for ready
    elapsed=0
    ready=false
    while [ $elapsed -lt 60 ]; do
      if docker exec callfs-meta-test /callfs-healthcheck 2>/dev/null; then
        ready=true
        break
      fi
      sleep 2
      elapsed=$((elapsed + 2))
    done

    if [ "$ready" = false ]; then
      echo "  ERROR: node1 with ${backend} not ready after 60s"
      docker logs callfs-meta-test 2>&1 | tail -20
      docker rm -f callfs-meta-test 2>/dev/null || true
      TOTAL_FAIL=$((TOTAL_FAIL + 1))
      FAILED_SUITES="${FAILED_SUITES} 13-${backend}"
      continue
    fi

    echo "  node1 (${backend}): ready"

    # Run the metadata backend test suite
    if docker run --rm \
      --network callfs-test-network \
      -v "${SCRIPT_DIR}/lib.sh:/tests/lib.sh:ro" \
      -v "${SCRIPT_DIR}/tests/13-metadata-backends.sh:/tests/tests/test.sh:ro" \
      alpine/curl:latest \
      sh -c '
        apk add --no-cache bash jq coreutils >/dev/null 2>&1
        bash /tests/tests/test.sh
      '; then
      TOTAL_PASS=$((TOTAL_PASS + 1))
      echo "  Suite 13-${backend}: OK"
    else
      TOTAL_FAIL=$((TOTAL_FAIL + 1))
      FAILED_SUITES="${FAILED_SUITES} 13-${backend}"
      echo "  Suite 13-${backend}: FAILED"
    fi

    # Cleanup the temporary container
    docker rm -f callfs-meta-test 2>/dev/null || true
  done

  # Restart the 3-node cluster for any remaining tests
  echo ""
  echo "--- Restarting 3-node cluster ---"
  docker compose up -d node1 node2 node3 2>/dev/null || true
fi

# ---------- Summary ----------
echo ""
echo "========================================================"
echo "  INTEGRATION TEST SUMMARY"
echo "========================================================"
echo "  Suites passed: ${TOTAL_PASS}"
echo "  Suites failed: ${TOTAL_FAIL}"
if [ -n "$FAILED_SUITES" ]; then
  echo "  Failed:${FAILED_SUITES}"
fi
echo "========================================================"

if [ "$TOTAL_FAIL" -gt 0 ]; then
  exit 1
fi
exit 0
