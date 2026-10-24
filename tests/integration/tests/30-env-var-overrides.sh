#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Environment Variable Overrides"

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
    dsn: "/var/lib/callfs/envtest.db"
log:
  level: "info"
YAMLEOF
)

start_env_container() {
  local name="$1"
  shift
  local cfg rc=0
  cfg=$(mktemp)
  printf '%s\n' "$BASE_CONFIG" > "$cfg"
  docker create --name "$name" --network "$DOCKER_NETWORK" "$@" "$CONFIG_TEST_IMAGE" server -c /config.yaml >/dev/null &&
    docker cp "$cfg" "$name:/config.yaml" &&
    docker start "$name" >/dev/null || rc=$?
  rm -f "$cfg"
  return $rc
}

wait_for_health() {
  local name="$1" port="$2" i
  for i in 1 2 3 4 5 6 7 8 9 10; do
    if curl -fsS "http://${name}:${port}/health" 2>/dev/null | grep -q "ok"; then
      return 0
    fi
    sleep 1
  done
  return 1
}

test_name "CALLFS_SERVER__LISTEN_ADDR overrides listen port"
CONTAINER_NAME="callfs-env-test-port-$$"
if start_env_container "$CONTAINER_NAME" -e "CALLFS_SERVER__LISTEN_ADDR=:9999" && wait_for_health "$CONTAINER_NAME" 9999; then
  pass
else
  fail "server did not start on overridden port 9999"
fi
docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true

test_name "CALLFS_LOG__LEVEL=error allows server to start"
CONTAINER_NAME="callfs-env-test-log-$$"
if start_env_container "$CONTAINER_NAME" -e "CALLFS_LOG__LEVEL=error" && wait_for_health "$CONTAINER_NAME" 8443; then
  pass
else
  fail "server did not start with CALLFS_LOG__LEVEL=error"
fi
docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true

print_summary
exit $?
