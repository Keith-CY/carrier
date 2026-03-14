#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cleanup_pids=()
tmpdir="$(mktemp -d)"

cleanup() {
  echo "[e2e-local] Cleaning up..."
  for pid in "${cleanup_pids[@]:-}"; do
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
  rm -rf "$tmpdir"
}
trap cleanup EXIT

find_port() {
  python3 -c "import socket; s=socket.socket(); s.bind(('', 0)); print(s.getsockname()[1]); s.close()"
}

wait_for_http() {
  local url="$1"
  local header_name="${2:-}"
  local header_value="${3:-}"
  for _ in $(seq 1 60); do
    if [[ -n "$header_name" ]]; then
      if curl -fsS -H "$header_name: $header_value" "$url" >/dev/null 2>&1; then
        return 0
      fi
    else
      if curl -fsS "$url" >/dev/null 2>&1; then
        return 0
      fi
    fi
    sleep 0.5
  done
  echo "[e2e-local] ERROR: timeout waiting for $url" >&2
  return 1
}

daemon_port="${CARRIER_E2E_DAEMON_PORT:-$(find_port)}"
gateway_port="${CARRIER_E2E_GATEWAY_PORT:-$(find_port)}"
state_root="$tmpdir/state"
bin_path="$tmpdir/carrier"

admin_token="${CARRIER_E2E_ADMIN_TOKEN:-admin-token}"
viewer_token="${CARRIER_E2E_VIEWER_TOKEN:-viewer-token}"
operator_token="${CARRIER_E2E_OPERATOR_TOKEN:-operator-token}"
approver_token="${CARRIER_E2E_APPROVER_TOKEN:-approver-token}"

export CARRIER_E2E_BASE_URL="http://127.0.0.1:${gateway_port}"
export CARRIER_E2E_DAEMON_URL="http://127.0.0.1:${daemon_port}"
export CARRIER_E2E_ADMIN_TOKEN="${admin_token}"
export CARRIER_E2E_VIEWER_TOKEN="${viewer_token}"
export CARRIER_E2E_OPERATOR_TOKEN="${operator_token}"
export CARRIER_E2E_APPROVER_TOKEN="${approver_token}"

mkdir -p "$state_root"

echo "[e2e-local] temp dir: $tmpdir"
echo "[e2e-local] daemon: ${CARRIER_E2E_DAEMON_URL}"
echo "[e2e-local] gateway: ${CARRIER_E2E_BASE_URL}"

if [[ "${CARRIER_E2E_SKIP_BUILD:-0}" != "1" ]]; then
  echo "[e2e-local] Building WebUI assets..."
  (cd "$repo_root" && bash scripts/build-webui.sh)
  echo "[e2e-local] Building carrier binary..."
  (cd "$repo_root" && go build -o "$bin_path" ./cmd/carrier)
else
  bin_path="${CARRIER_E2E_BIN_PATH:-$repo_root/carrier}"
fi

echo "[e2e-local] Starting daemon..."
(
  cd "$repo_root"
  CARRIER_SERVER_PORT="$daemon_port" \
  CARRIER_SERVER_HOST="127.0.0.1" \
  CARRIER_DEV_MODE=1 \
  SESSION_DATA_DIR="$state_root/daemon" \
  "$bin_path" daemon >"$tmpdir/daemon.log" 2>&1
) &
cleanup_pids+=("$!")
wait_for_http "${CARRIER_E2E_DAEMON_URL}/healthz"

echo "[e2e-local] Starting gateway..."
(
  cd "$repo_root"
  CARRIER_GATEWAY_PORT="$gateway_port" \
  CARRIER_GATEWAY_HOST="127.0.0.1" \
  CARRIER_DAEMON_BASE_URL="${CARRIER_E2E_DAEMON_URL}" \
  CARRIER_GATEWAY_ROLE_TOKENS="viewer:${viewer_token},operator:${operator_token},approver:${approver_token},admin:${admin_token}" \
  CARRIER_REMOTE_CONTROL_PLANE_ENABLED=1 \
  CARRIER_REMOTE_CHAT_ENABLED=1 \
  CARRIER_PROVIDER_BINDING_ENABLED=1 \
  CARRIER_TRIGGER_SCHEDULE_POLL_INTERVAL_SEC=1 \
  SESSION_DATA_DIR="$state_root/gateway" \
  "$bin_path" gateway >"$tmpdir/gateway.log" 2>&1
) &
cleanup_pids+=("$!")
wait_for_http "${CARRIER_E2E_BASE_URL}/healthz"

echo "[e2e-local] Running full-stack Playwright suite..."
cd "$repo_root/webui/e2e"
bunx playwright test -c playwright.fullstack.config.ts "$@"
