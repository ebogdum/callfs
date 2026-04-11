#!/usr/bin/env bash
# CallFS Integration Test Runner
# Usage: ./run-tests.sh [--keep] [--filter PATTERN] [--skip-build] [--skip-s3] [--skip-config] [--skip-env]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
KEEP=false
FILTER=""
SKIP_BUILD=false
SKIP_S3=false
SKIP_CONFIG=false
SKIP_ENV=false

# Parse arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --keep) KEEP=true; shift ;;
    --filter) FILTER="$2"; shift 2 ;;
    --skip-build) SKIP_BUILD=true; shift ;;
    --skip-s3) SKIP_S3=true; shift ;;
    --skip-config) SKIP_CONFIG=true; shift ;;
    --skip-env) SKIP_ENV=true; shift ;;
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
    # Clean up any ephemeral containers from config/env tests
    docker rm -f callfs-config-test callfs-env-test 2>/dev/null || true
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

# Give raft time to elect leader on node1, create root dir, and take snapshot
echo "==> Waiting for Raft leader election and initial snapshot..."
sleep 10

# Join node2 and node3 to the Raft cluster via the internal API.
# After joining, the leader's snapshot will be installed on followers.
INTERNAL_SECRET="test-internal-secret-0123456789"
echo "==> Joining node2 to Raft cluster..."
docker run --rm --network callfs-test-network alpine/curl:latest \
  curl -sf -X POST \
    -H "Authorization: Bearer ${INTERNAL_SECRET}" \
    -H "Content-Type: application/json" \
    -d '{"node_id":"node-2","raft_addr":"node2:7000","api_endpoint":"http://node2:8443"}' \
    http://node1:8443/v1/internal/raft/join 2>/dev/null && echo "  node2: joined" || echo "  node2: join failed (may already be member)"

echo "==> Joining node3 to Raft cluster..."
docker run --rm --network callfs-test-network alpine/curl:latest \
  curl -sf -X POST \
    -H "Authorization: Bearer ${INTERNAL_SECRET}" \
    -H "Content-Type: application/json" \
    -d '{"node_id":"node-3","raft_addr":"node3:7000","api_endpoint":"http://node3:8443"}' \
    http://node1:8443/v1/internal/raft/join 2>/dev/null && echo "  node3: joined" || echo "  node3: join failed (may already be member)"

# Wait for followers to install snapshot and stabilize
echo "==> Waiting for Raft cluster to stabilize..."
sleep 10
echo "==> Cluster ready"

# ---------- Counters ----------
TOTAL_PASS=0
TOTAL_FAIL=0
FAILED_SUITES=""

# ---------- Test runner (external Alpine container) ----------
run_test_external() {
  local test_file="$1"
  local test_name
  test_name=$(basename "$test_file" .sh)

  echo ""
  echo "============================================"
  echo "  Running: ${test_name}"
  echo "============================================"

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

# ==========================================================
# PHASE 1: Cluster tests (01-12, 14-26, 28, 31-33)
# ==========================================================
echo ""
echo "=========================================="
echo "  PHASE 1: Cluster Tests"
echo "=========================================="

for test_file in "${SCRIPT_DIR}/tests/"*.sh; do
  test_name=$(basename "$test_file" .sh)

  # Skip tests that run in separate phases
  case "$test_name" in
    13-metadata-backends) continue ;;  # Phase 2
    27-s3-backend) continue ;;         # Phase 3
    29-config-validation) continue ;;  # Phase 4
    30-env-var-overrides) continue ;;  # Phase 5
  esac

  # Apply filter
  if [ -n "$FILTER" ] && ! echo "$test_name" | grep -q "$FILTER"; then
    continue
  fi

  run_test_external "$test_file"
done

# ==========================================================
# PHASE 2: Metadata backend tests (13)
# ==========================================================
if [ -z "$FILTER" ] || echo "13-metadata-backends" | grep -q "$FILTER"; then
  echo ""
  echo "=========================================="
  echo "  PHASE 2: Metadata Backend Tests"
  echo "=========================================="

  for backend in sqlite postgres redis; do
    echo ""
    echo "--- Testing metadata backend: ${backend} ---"

    # Stop all CallFS nodes
    docker compose stop node1 node2 node3 2>/dev/null || true

    # Clear any leftover data
    docker compose rm -f node1 node2 node3 2>/dev/null || true

    # If postgres backend, reset the database completely
    if [ "$backend" = "postgres" ]; then
      # Terminate any lingering connections from prior nodes
      docker compose exec -T postgres psql -U callfs -d callfs -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='callfs' AND pid <> pg_backend_pid();" 2>/dev/null || true
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

    # Run node1 with the specific config. Use --network-alias node1 so the test's
    # NODE1="http://node1:8443" resolves to this container within the Docker network.
    docker run -d \
      --name callfs-meta-test \
      --network callfs-test-network \
      --network-alias node1 \
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

# ==========================================================
# PHASE 3: S3 backend tests (27)
# ==========================================================
if [ "$SKIP_S3" = false ] && { [ -z "$FILTER" ] || echo "27-s3-backend" | grep -q "$FILTER"; }; then
  echo ""
  echo "=========================================="
  echo "  PHASE 3: S3 Backend Tests (MinIO)"
  echo "=========================================="

  echo "==> Starting MinIO..."
  docker compose up -d minio
  echo "==> Waiting for MinIO to become healthy..."
  elapsed=0
  while [ $elapsed -lt 60 ]; do
    if docker compose exec -T minio mc ready local 2>/dev/null; then
      echo "  minio: ready"
      break
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  if [ $elapsed -ge 60 ]; then
    echo "ERROR: MinIO not ready after 60s"
    TOTAL_FAIL=$((TOTAL_FAIL + 1))
    FAILED_SUITES="${FAILED_SUITES} 27-s3-backend"
  else
    echo "==> Initializing MinIO bucket..."
    docker compose up minio-init 2>/dev/null || true

    echo "==> Starting S3-backend node..."
    docker compose up -d node-s3

    elapsed=0
    while [ $elapsed -lt 60 ]; do
      if docker compose exec -T node-s3 /callfs-healthcheck 2>/dev/null; then
        echo "  node-s3: ready"
        break
      fi
      sleep 2
      elapsed=$((elapsed + 2))
    done

    if [ $elapsed -ge 60 ]; then
      echo "ERROR: node-s3 not ready after 60s"
      docker compose logs node-s3 2>&1 | tail -30
      TOTAL_FAIL=$((TOTAL_FAIL + 1))
      FAILED_SUITES="${FAILED_SUITES} 27-s3-backend"
    else
      # Run S3 test suite
      if docker run --rm \
        --network callfs-test-network \
        -v "${SCRIPT_DIR}/lib.sh:/tests/lib.sh:ro" \
        -v "${SCRIPT_DIR}/tests/27-s3-backend.sh:/tests/tests/test.sh:ro" \
        alpine/curl:latest \
        sh -c '
          apk add --no-cache bash jq coreutils >/dev/null 2>&1
          bash /tests/tests/test.sh
        '; then
        TOTAL_PASS=$((TOTAL_PASS + 1))
        echo "  Suite 27-s3-backend: OK"
      else
        TOTAL_FAIL=$((TOTAL_FAIL + 1))
        FAILED_SUITES="${FAILED_SUITES} 27-s3-backend"
        echo "  Suite 27-s3-backend: FAILED"
      fi
    fi

    # Stop S3 infrastructure
    docker compose stop node-s3 minio 2>/dev/null || true
  fi
fi

# ==========================================================
# PHASE 4: Config validation tests (29)
# ==========================================================
if [ "$SKIP_CONFIG" = false ] && { [ -z "$FILTER" ] || echo "29-config-validation" | grep -q "$FILTER"; }; then
  echo ""
  echo "=========================================="
  echo "  PHASE 4: Config Validation Tests"
  echo "=========================================="

  CONFIG_PASS=0
  CONFIG_FAIL=0

  # Test 1: Valid YAML config
  echo "  Test: callfs config validate with valid YAML..."
  if docker run --rm \
    -v "${SCRIPT_DIR}/configs/node-sqlite.yaml:/config.yaml:ro" \
    --tmpfs /data \
    callfs:test \
    config validate --config /config.yaml 2>&1; then
    echo "  PASS: valid YAML config"
    CONFIG_PASS=$((CONFIG_PASS + 1))
  else
    echo "  FAIL: valid YAML config should pass validation"
    CONFIG_FAIL=$((CONFIG_FAIL + 1))
  fi

  # Test 2: Valid JSON config
  echo "  Test: callfs config validate with valid JSON..."
  if docker run --rm \
    -v "${SCRIPT_DIR}/configs/node-json.json:/config.json:ro" \
    --tmpfs /data \
    callfs:test \
    config validate --config /config.json 2>&1; then
    echo "  PASS: valid JSON config"
    CONFIG_PASS=$((CONFIG_PASS + 1))
  else
    echo "  FAIL: valid JSON config should pass validation"
    CONFIG_FAIL=$((CONFIG_FAIL + 1))
  fi

  # Test 3: Short API key
  echo "  Test: callfs config validate with short API key..."
  if docker run --rm \
    -v "${SCRIPT_DIR}/configs/node-invalid-short-key.yaml:/config.yaml:ro" \
    --tmpfs /data \
    callfs:test \
    config validate --config /config.yaml 2>&1; then
    echo "  FAIL: short API key should fail validation"
    CONFIG_FAIL=$((CONFIG_FAIL + 1))
  else
    echo "  PASS: short API key rejected"
    CONFIG_PASS=$((CONFIG_PASS + 1))
  fi

  # Test 4: No API keys
  echo "  Test: callfs config validate with no API keys..."
  if docker run --rm \
    -v "${SCRIPT_DIR}/configs/node-invalid-no-keys.yaml:/config.yaml:ro" \
    --tmpfs /data \
    callfs:test \
    config validate --config /config.yaml 2>&1; then
    echo "  FAIL: empty API keys should fail validation"
    CONFIG_FAIL=$((CONFIG_FAIL + 1))
  else
    echo "  PASS: empty API keys rejected"
    CONFIG_PASS=$((CONFIG_PASS + 1))
  fi

  # Test 5: Nonexistent config file
  echo "  Test: callfs config validate with nonexistent file..."
  if docker run --rm \
    callfs:test \
    config validate --config /nonexistent.yaml 2>&1; then
    echo "  FAIL: nonexistent config file should fail"
    CONFIG_FAIL=$((CONFIG_FAIL + 1))
  else
    echo "  PASS: nonexistent config file rejected"
    CONFIG_PASS=$((CONFIG_PASS + 1))
  fi

  echo ""
  echo "  Config validation: ${CONFIG_PASS} passed, ${CONFIG_FAIL} failed"
  if [ "$CONFIG_FAIL" -eq 0 ]; then
    TOTAL_PASS=$((TOTAL_PASS + 1))
    echo "  Suite 29-config-validation: OK"
  else
    TOTAL_FAIL=$((TOTAL_FAIL + 1))
    FAILED_SUITES="${FAILED_SUITES} 29-config-validation"
    echo "  Suite 29-config-validation: FAILED"
  fi
fi

# ==========================================================
# PHASE 5: Environment variable override tests (30)
# ==========================================================
if [ "$SKIP_ENV" = false ] && { [ -z "$FILTER" ] || echo "30-env-var-overrides" | grep -q "$FILTER"; }; then
  echo ""
  echo "=========================================="
  echo "  PHASE 5: Environment Variable Override Tests"
  echo "=========================================="

  ENV_PASS=0
  ENV_FAIL=0

  # Test 1: CALLFS_SERVER__LISTEN_ADDR overrides port
  echo "  Test: CALLFS_SERVER__LISTEN_ADDR=:9999 overrides listen address..."
  docker run -d --name callfs-env-test \
    --network callfs-test-network \
    --network-alias callfs-env-test \
    -v "${SCRIPT_DIR}/configs/node-sqlite.yaml:/config.yaml:ro" \
    -e "CALLFS_SERVER__LISTEN_ADDR=:9999" \
    --tmpfs /data \
    callfs:test \
    server --config /config.yaml 2>/dev/null

  # Wait for the node to be ready on port 9999
  # Note: the container uses a scratch base image (no shell), so we probe
  # from the Docker network using a curl container instead of docker exec.
  elapsed=0
  ready=false
  while [ $elapsed -lt 30 ]; do
    if docker run --rm --network callfs-test-network alpine/curl:latest \
      curl -sf --max-time 2 http://callfs-env-test:9999/health >/dev/null 2>&1; then
      ready=true
      break
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done

  if [ "$ready" = true ]; then
    echo "  PASS: server listening on overridden port 9999"
    ENV_PASS=$((ENV_PASS + 1))
  else
    echo "  FAIL: server not responding on port 9999"
    docker logs callfs-env-test 2>&1 | tail -10
    ENV_FAIL=$((ENV_FAIL + 1))
  fi
  docker rm -f callfs-env-test 2>/dev/null || true

  # Test 2: CALLFS_LOG__LEVEL override
  echo "  Test: CALLFS_LOG__LEVEL=error override..."
  docker run -d --name callfs-env-test \
    --network callfs-test-network \
    -v "${SCRIPT_DIR}/configs/node-sqlite.yaml:/config.yaml:ro" \
    -e "CALLFS_LOG__LEVEL=error" \
    --tmpfs /data \
    callfs:test \
    server --config /config.yaml 2>/dev/null

  elapsed=0
  ready=false
  while [ $elapsed -lt 30 ]; do
    if docker exec callfs-env-test /callfs-healthcheck 2>/dev/null; then
      ready=true
      break
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done

  if [ "$ready" = true ]; then
    echo "  PASS: server starts with log level override"
    ENV_PASS=$((ENV_PASS + 1))
  else
    echo "  FAIL: server failed to start with log level override"
    docker logs callfs-env-test 2>&1 | tail -10
    ENV_FAIL=$((ENV_FAIL + 1))
  fi
  docker rm -f callfs-env-test 2>/dev/null || true

  echo ""
  echo "  Env var overrides: ${ENV_PASS} passed, ${ENV_FAIL} failed"
  if [ "$ENV_FAIL" -eq 0 ]; then
    TOTAL_PASS=$((TOTAL_PASS + 1))
    echo "  Suite 30-env-var-overrides: OK"
  else
    TOTAL_FAIL=$((TOTAL_FAIL + 1))
    FAILED_SUITES="${FAILED_SUITES} 30-env-var-overrides"
    echo "  Suite 30-env-var-overrides: FAILED"
  fi
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
