#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cleanup_pids=()
cleanup_containers=()
tmpdir="$(mktemp -d)"

cleanup() {
  echo "[e2e-local] Cleaning up..."
  for pid in "${cleanup_pids[@]:-}"; do
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
  for container in "${cleanup_containers[@]:-}"; do
    docker rm -f "$container" >/dev/null 2>&1 || true
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

wait_for_ssh() {
  local key_path="$1"
  local port="$2"
  for _ in $(seq 1 60); do
    if ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i "$key_path" -p "$port" carrier@127.0.0.1 true >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "[e2e-local] ERROR: timeout waiting for remote SSH on port $port" >&2
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
export CARRIER_DISABLE_KEYCHAIN="${CARRIER_DISABLE_KEYCHAIN:-1}"
export CARRIER_CREDENTIAL_STORE="${CARRIER_CREDENTIAL_STORE:-$state_root/credentials.json}"
export CARRIER_REMOTE_CONTROL_STORE="${CARRIER_REMOTE_CONTROL_STORE:-$state_root/remote-control.json}"
export OPENROUTER_API_KEY="${OPENROUTER_API_KEY:-${CARRIER_LIVE_API_KEY:-${CARRIER_E2E_OPENROUTER_API_KEY:-test-openrouter-token}}}"

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

if [[ "${CARRIER_E2E_REMOTE_FIXTURE:-0}" == "1" ]]; then
  echo "[e2e-local] Starting remote Ubuntu fixture..."
  remote_ssh_port="${CARRIER_E2E_REMOTE_SSH_PORT:-$(find_port)}"
  remote_host_name="${CARRIER_E2E_REMOTE_HOST_NAME:-ubuntu-demo}"
  remote_container_name="carrier-e2e-remote-${remote_ssh_port}"
  remote_image_tag="carrier-remote-vps-e2e:latest"
  remote_ssh_dir="$tmpdir/ssh"
  remote_ssh_key="$remote_ssh_dir/id_ed25519"
  mkdir -p "$remote_ssh_dir"
  ssh-keygen -q -t ed25519 -N '' -f "$remote_ssh_key" >/dev/null
  remote_authorized_key="$(cat "$remote_ssh_key.pub")"
  docker build -t "$remote_image_tag" -f "$repo_root/tests/fixtures/remote-vps/Dockerfile" "$repo_root" >/dev/null
  docker run -d --name "$remote_container_name" -p "${remote_ssh_port}:22" -e AUTHORIZED_KEY="$remote_authorized_key" "$remote_image_tag" >/dev/null
  cleanup_containers+=("$remote_container_name")
  wait_for_ssh "$remote_ssh_key" "$remote_ssh_port"

  remote_host_payload="$(mktemp "$tmpdir/remote-host-payload.XXXXXX.json")"
  cat >"$remote_host_payload" <<EOF
{
  "name": "$remote_host_name",
  "host": "127.0.0.1",
  "port": $remote_ssh_port,
  "user": "carrier",
  "authMode": "private_key",
  "keyPath": "$remote_ssh_key",
  "runtimeMode": "on_demand"
}
EOF
  remote_host_response="$tmpdir/remote-host-response.json"
  curl -fsS \
    -H "Authorization: Bearer ${admin_token}" \
    -H "Content-Type: application/json" \
    -X POST \
    --data @"$remote_host_payload" \
    "${CARRIER_E2E_BASE_URL}/api/v1/remote/hosts" >"$remote_host_response"
  remote_host_id="$(jq -r '.host.id' "$remote_host_response")"
  export CARRIER_E2E_REMOTE_HOST_ID="$remote_host_id"
  export CARRIER_E2E_REMOTE_HOST_NAME="$remote_host_name"
  export CARRIER_E2E_REMOTE_SSH_PORT="$remote_ssh_port"
  export CARRIER_E2E_REMOTE_SSH_KEY="$remote_ssh_key"
  echo "[e2e-local] Remote host: ${CARRIER_E2E_REMOTE_HOST_NAME} (${CARRIER_E2E_REMOTE_HOST_ID})"
  curl -fsS \
    -H "Authorization: Bearer ${admin_token}" \
    -H "Content-Type: application/json" \
    -X POST \
    --data '{"pullNewInstances":false}' \
    "${CARRIER_E2E_BASE_URL}/api/v1/remote/hosts/${CARRIER_E2E_REMOTE_HOST_ID}/check" >/dev/null
fi

echo "[e2e-local] Running full-stack Playwright suite..."
cd "$repo_root/webui/e2e"
bunx playwright test -c playwright.fullstack.config.ts "$@"
