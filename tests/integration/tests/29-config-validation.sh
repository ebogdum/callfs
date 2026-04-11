#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/../lib.sh"

section "Config Validation"

# This test uses docker run directly to validate callfs config commands.
# If running inside a container without docker CLI, skip gracefully.

if ! command -v docker &>/dev/null; then
  echo "SKIP: requires docker CLI (not available inside test container)"
  # Record zero tests so print_summary passes
  print_summary
  exit $?
fi

CONFIG_TEST_IMAGE="${CONFIG_TEST_IMAGE:-callfs:test}"

# Verify the image exists
if ! docker image inspect "$CONFIG_TEST_IMAGE" &>/dev/null; then
  echo "SKIP: image ${CONFIG_TEST_IMAGE} not found"
  print_summary
  exit $?
fi

# --- Valid YAML config ---

test_name "callfs config validate with valid YAML exits 0"
VALID_YAML=$(cat <<'YAMLEOF'
server:
  listen_addr: ":8443"
auth:
  api_keys:
    - "test-api-key-integration-0123456"
  internal_secret: "test-internal-secret-0123456789"
storage:
  backend: "sqlite"
  sqlite:
    dsn: "/tmp/test.db"
log:
  level: "info"
YAMLEOF
)
if docker run --rm -i "$CONFIG_TEST_IMAGE" sh -c "cat > /tmp/valid.yaml && callfs config validate -c /tmp/valid.yaml" <<< "$VALID_YAML" 2>/dev/null; then
  pass
else
  fail "valid YAML config should exit 0"
fi

# --- Valid JSON config ---

test_name "callfs config validate with valid JSON exits 0"
VALID_JSON=$(cat <<'JSONEOF'
{
  "server": {"listen_addr": ":8443"},
  "auth": {
    "api_keys": ["test-api-key-integration-0123456"],
    "internal_secret": "test-internal-secret-0123456789"
  },
  "storage": {"backend": "sqlite", "sqlite": {"dsn": "/tmp/test.db"}},
  "log": {"level": "info"}
}
JSONEOF
)
if docker run --rm -i "$CONFIG_TEST_IMAGE" sh -c "cat > /tmp/valid.json && callfs config validate -c /tmp/valid.json" <<< "$VALID_JSON" 2>/dev/null; then
  pass
else
  fail "valid JSON config should exit 0"
fi

# --- Short API key ---

test_name "callfs config validate with short API key exits non-zero"
SHORT_KEY_YAML=$(cat <<'YAMLEOF'
server:
  listen_addr: ":8443"
auth:
  api_keys:
    - "short"
  internal_secret: "test-internal-secret-0123456789"
storage:
  backend: "sqlite"
  sqlite:
    dsn: "/tmp/test.db"
log:
  level: "info"
YAMLEOF
)
if docker run --rm -i "$CONFIG_TEST_IMAGE" sh -c "cat > /tmp/short.yaml && callfs config validate -c /tmp/short.yaml" <<< "$SHORT_KEY_YAML" 2>/dev/null; then
  fail "short API key should cause validation failure"
else
  pass
fi

# --- No API keys ---

test_name "callfs config validate with no API keys exits non-zero"
NO_KEYS_YAML=$(cat <<'YAMLEOF'
server:
  listen_addr: ":8443"
auth:
  api_keys: []
  internal_secret: "test-internal-secret-0123456789"
storage:
  backend: "sqlite"
  sqlite:
    dsn: "/tmp/test.db"
log:
  level: "info"
YAMLEOF
)
if docker run --rm -i "$CONFIG_TEST_IMAGE" sh -c "cat > /tmp/nokeys.yaml && callfs config validate -c /tmp/nokeys.yaml" <<< "$NO_KEYS_YAML" 2>/dev/null; then
  fail "empty API keys should cause validation failure"
else
  pass
fi

# --- Nonexistent config file ---

test_name "callfs config validate with nonexistent file exits non-zero"
if docker run --rm "$CONFIG_TEST_IMAGE" callfs config validate -c /nonexistent.yaml 2>/dev/null; then
  fail "nonexistent config file should cause failure"
else
  pass
fi

print_summary
exit $?
