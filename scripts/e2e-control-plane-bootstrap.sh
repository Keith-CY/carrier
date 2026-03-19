#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
cleanup_pids=()
tmpdir="${CARRIER_E2E_TMPDIR:-$(mktemp -d)}"
created_tmpdir=0
if [[ -z "${CARRIER_E2E_TMPDIR:-}" ]]; then
  created_tmpdir=1
fi

cleanup() {
  echo "[e2e-bootstrap] Cleaning up..."
  for pid in "${cleanup_pids[@]:-}"; do
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
  if [[ "$created_tmpdir" == "1" ]]; then
    rm -rf "$tmpdir"
  fi
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
  echo "[e2e-bootstrap] ERROR: timeout waiting for $url" >&2
  return 1
}

register_remote_host() {
  if [[ -z "${CARRIER_E2E_REMOTE_HOST_HOST:-}" ]]; then
    return 0
  fi

  local host_name="${CARRIER_E2E_REMOTE_HOST_NAME:-remote-e2e}"
  local host_port="${CARRIER_E2E_REMOTE_HOST_PORT:-22}"
  local host_user="${CARRIER_E2E_REMOTE_HOST_USER:-carrier}"
  local key_path="${CARRIER_E2E_REMOTE_HOST_KEY_PATH:-}"
  local runtime_mode="${CARRIER_E2E_REMOTE_RUNTIME_MODE:-on_demand}"

  if [[ -z "$key_path" || ! -f "$key_path" ]]; then
    echo "[e2e-bootstrap] ERROR: CARRIER_E2E_REMOTE_HOST_KEY_PATH must point to an SSH private key" >&2
    return 1
  fi

  local payload_path="$tmpdir/remote-host-payload.json"
  cat >"$payload_path" <<EOF
{
  "name": "$host_name",
  "host": "${CARRIER_E2E_REMOTE_HOST_HOST}",
  "port": ${host_port},
  "user": "$host_user",
  "authMode": "private_key",
  "keyPath": "$key_path",
  "runtimeMode": "$runtime_mode"
}
EOF

  local response_path="$tmpdir/remote-host-response.json"
  curl -fsS \
    -H "Authorization: Bearer ${CARRIER_E2E_ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -X POST \
    --data @"$payload_path" \
    "${CARRIER_E2E_BASE_URL}/api/v1/remote/hosts" >"$response_path"

  export CARRIER_E2E_REMOTE_HOST_ID
  CARRIER_E2E_REMOTE_HOST_ID="$(jq -r '.host.id' "$response_path")"
  export CARRIER_E2E_REMOTE_HOST_NAME="$host_name"

  curl -fsS \
    -H "Authorization: Bearer ${CARRIER_E2E_ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    -X POST \
    --data '{"pullNewInstances":false}' \
    "${CARRIER_E2E_BASE_URL}/api/v1/remote/hosts/${CARRIER_E2E_REMOTE_HOST_ID}/check" >/dev/null

  echo "[e2e-bootstrap] Remote host: ${CARRIER_E2E_REMOTE_HOST_NAME} (${CARRIER_E2E_REMOTE_HOST_ID})"
}

run_playwright_fullstack() {
  cd "$repo_root/webui/e2e"
  bunx playwright test -c playwright.fullstack.config.ts "$@"
}

daemon_port="${CARRIER_E2E_DAEMON_PORT:-$(find_port)}"
gateway_port="${CARRIER_E2E_GATEWAY_PORT:-$(find_port)}"
state_root="${CARRIER_E2E_STATE_ROOT:-$tmpdir/state}"
bin_path="${CARRIER_E2E_BIN_PATH:-$tmpdir/carrier}"

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
export CARRIER_DISABLE_KEYCHAIN="${CARRIER_DISABLE_KEYCHAIN:-1}"
export CARRIER_CREDENTIAL_STORE="${CARRIER_CREDENTIAL_STORE:-$state_root/credentials.json}"
export CARRIER_REMOTE_CONTROL_STORE="${CARRIER_REMOTE_CONTROL_STORE:-$state_root/remote-control.json}"
export OPENROUTER_API_KEY="${OPENROUTER_API_KEY:-${CARRIER_LIVE_API_KEY:-${CARRIER_E2E_OPENROUTER_API_KEY:-test-openrouter-token}}}"
export OPENAI_COMPATIBLE_API_KEY="${OPENAI_COMPATIBLE_API_KEY:-test-openai-compatible-token}"

mkdir -p "$state_root"

echo "[e2e-bootstrap] temp dir: $tmpdir"
echo "[e2e-bootstrap] daemon: ${CARRIER_E2E_DAEMON_URL}"
echo "[e2e-bootstrap] gateway: ${CARRIER_E2E_BASE_URL}"

if [[ "${CARRIER_E2E_SKIP_BUILD:-0}" != "1" ]]; then
  echo "[e2e-bootstrap] Building WebUI assets..."
  (cd "$repo_root" && bash scripts/build-webui.sh)
  echo "[e2e-bootstrap] Building carrier binary..."
  (cd "$repo_root" && go build -buildvcs=false -o "$bin_path" ./cmd/carrier)
elif [[ ! -x "$bin_path" && -x "$repo_root/carrier" ]]; then
  bin_path="$repo_root/carrier"
fi

echo "[e2e-bootstrap] Starting daemon..."
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

echo "[e2e-bootstrap] Starting gateway..."
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

register_remote_host

if [[ $# -eq 0 ]]; then
  echo "[e2e-bootstrap] ERROR: missing command to run after bootstrap" >&2
  exit 1
fi

if [[ "$1" == "--playwright-fullstack" ]]; then
  shift
  run_playwright_fullstack "$@"
  exit 0
fi

"$@"
