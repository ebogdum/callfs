#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Environment Variable Overrides"

# This test uses docker run to start callfs with env var overrides.
# If running inside a container without docker CLI, skip gracefully.

if ! command -v docker &>/dev/null; then
  echo "SKIP: requires docker CLI (not available inside test container)"
  print_summary
  exit $?
fi

CONFIG_TEST_IMAGE="${CONFIG_TEST_IMAGE:-callfs:test}"
DOCKER_NETWORK="${DOCKER_NETWORK:-callfs-test-net}"

if ! docker image inspect "$CONFIG_TEST_IMAGE" &>/dev/null; then
  echo "SKIP: image ${CONFIG_TEST_IMAGE} not found"
  print_summary
  exit $?
fi

# Base config for env override tests
BASE_CONFIG=$(cat <<'YAMLEOF'
server:
  listen_addr: ":8443"
auth:
  api_keys:
    - "test-api-key-integration-0123456"
  internal_secret: "test-internal-secret-0123456789"
storage:
  backend: "sqlite"
  sqlite:
    dsn: "/tmp/envtest.db"
log:
  level: "info"
YAMLEOF
)

# --- Listen address override ---

test_name "CALLFS_SERVER__LISTEN_ADDR overrides listen port"
CONTAINER_NAME="callfs-env-test-port-$$"
# Start container with overridden port
docker run -d --rm \
  --name "$CONTAINER_NAME" \
  --network "$DOCKER_NETWORK" \
  -e "CALLFS_SERVER__LISTEN_ADDR=:9999" \
  "$CONFIG_TEST_IMAGE" sh -c "cat > /tmp/config.yaml && callfs serve -c /tmp/config.yaml" <<< "$BASE_CONFIG" >/dev/null 2>&1 || true

# Give the server time to start
sleep 3

# Check health on overridden port
HEALTH_OK=false
for i in 1 2 3 4 5; do
  if docker exec "$CONTAINER_NAME" wget -qO- "http://localhost:9999/health" 2>/dev/null | grep -q "ok"; then
    HEALTH_OK=true
    break
  fi
  sleep 1
done

docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true

if [ "$HEALTH_OK" = "true" ]; then
  pass
else
  fail "server did not start on overridden port 9999"
fi

# --- Log level override ---

test_name "CALLFS_LOG__LEVEL=error allows server to start"
CONTAINER_NAME="callfs-env-test-log-$$"
docker run -d --rm \
  --name "$CONTAINER_NAME" \
  --network "$DOCKER_NETWORK" \
  -e "CALLFS_LOG__LEVEL=error" \
  "$CONFIG_TEST_IMAGE" sh -c "cat > /tmp/config.yaml && callfs serve -c /tmp/config.yaml" <<< "$BASE_CONFIG" >/dev/null 2>&1 || true

sleep 3

HEALTH_OK=false
for i in 1 2 3 4 5; do
  if docker exec "$CONTAINER_NAME" wget -qO- "http://localhost:8443/health" 2>/dev/null | grep -q "ok"; then
    HEALTH_OK=true
    break
  fi
  sleep 1
done

docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true

if [ "$HEALTH_OK" = "true" ]; then
  pass
else
  fail "server did not start with CALLFS_LOG__LEVEL=error"
fi

print_summary
exit $?
