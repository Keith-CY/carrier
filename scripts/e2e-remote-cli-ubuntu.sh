#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  bash scripts/e2e-remote-cli-ubuntu.sh

Required environment:
  OPENROUTER_API_KEY           OpenRouter API key used by `carrier onboard --cli` when provider=openrouter.
  ANTHROPIC_API_KEY            Anthropic API key used by `carrier onboard --cli` when provider=anthropic.

Optional environment:
  CARRIER_E2E_PROVIDER         LLM provider id. Default: openrouter when OPENROUTER_API_KEY is set, else anthropic.
  CARRIER_E2E_SKIP_BUILD       Set to 1 to reuse CARRIER_E2E_BIN_PATH.
  CARRIER_E2E_BIN_PATH         Existing carrier binary path when skip-build is enabled.
  CARRIER_E2E_DAEMON_PORT      Fixed local daemon port.
  CARRIER_E2E_GATEWAY_PORT     Fixed local gateway port.
  CARRIER_E2E_REMOTE_SSH_PORT  Fixed Docker Ubuntu SSH port.
  CARRIER_E2E_REMOTE_HOST_ID   Remote host id. Default: ubuntu-2404-docker
  CARRIER_E2E_KEEP_TMP         Set to 1 to keep the temp directory after the run.
  CARRIER_E2E_ORCHESTRATE_GOAL Goal used for dry-run and live orchestration verification.

What this script verifies:
  1. Build carrier from source.
  2. Complete `carrier onboard --cli` with Anthropic in WebUI-only mode.
  3. Install remote OpenClaw inside a Docker Ubuntu 24.04 host with synced provider config.
  4. Run a one-shot remote OpenClaw task through `carrier remote run`.
  5. Repeat remote install + run for PicoClaw and ZeroClaw.
  6. Decompose a goal, verify multiple remote workers in the dry-run plan, then execute it and export show/evidence/audit JSON.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "error: required command not found: $cmd" >&2
    exit 1
  fi
}

find_port() {
  python3 - <<'PY'
import socket

s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

wait_for_http_ok() {
  local url="$1"
  local label="$2"
  local attempts="${3:-90}"
  for _ in $(seq 1 "$attempts"); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "error: timed out waiting for ${label}: ${url}" >&2
  return 1
}

wait_for_ssh() {
  local user="$1"
  local key_path="$2"
  local port="$3"
  for _ in $(seq 1 60); do
    if ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i "$key_path" -p "$port" "${user}@127.0.0.1" true >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "error: timed out waiting for SSH on port ${port}" >&2
  return 1
}

run_logged() {
  local label="$1"
  local logfile="$2"
  shift 2
  echo "[run] ${label}"
  if ! "$@" >"$logfile" 2>&1; then
    echo "error: ${label} failed; log: ${logfile}" >&2
    sed -n '1,240p' "$logfile" >&2 || true
    exit 1
  fi
}

capture_json_output() {
  local outfile="$1"
  shift
  local raw_file="${outfile}.raw"
  if ! "$@" >"$raw_file" 2>&1; then
    echo "error: command failed while capturing JSON: $*" >&2
    sed -n '1,240p' "$raw_file" >&2 || true
    exit 1
  fi
  awk 'BEGIN{emit=0} /^[[:space:]]*[{[]/{emit=1} emit{print}' "$raw_file" >"$outfile"
  if ! jq empty "$outfile" >/dev/null 2>&1; then
    echo "error: failed to extract JSON output from command: $*" >&2
    sed -n '1,240p' "$raw_file" >&2 || true
    exit 1
  fi
}

assert_jq() {
  local file="$1"
  local expr="$2"
  local label="$3"
  if ! jq -e "$expr" "$file" >/dev/null 2>&1; then
    echo "error: assertion failed for ${label}" >&2
    sed -n '1,240p' "$file" >&2 || true
    exit 1
  fi
}

repo_root="$(git rev-parse --show-toplevel)"
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/carrier-e2e-remote-cli-ubuntu.XXXXXX")"
state_root="${tmp_dir}/state"
artifacts_dir="${tmp_dir}/artifacts"
home_dir="${state_root}/home"
ssh_dir="${tmp_dir}/ssh"
host_home="${HOME:-}"
host_docker_config="${DOCKER_CONFIG:-${host_home}/.docker}"
mkdir -p "$state_root" "$artifacts_dir" "$home_dir" "$ssh_dir"

bin_path=""
container_name=""
keep_tmp="${CARRIER_E2E_KEEP_TMP:-0}"

cleanup() {
  if [[ -n "$container_name" ]]; then
    docker rm -f "$container_name" >/dev/null 2>&1 || true
  fi
  if [[ -n "$bin_path" && -x "$bin_path" ]]; then
    "$bin_path" stop >/dev/null 2>&1 || true
  fi
  if [[ "$keep_tmp" == "1" ]]; then
    echo "[artifacts] kept temp dir: ${tmp_dir}"
    return
  fi
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

require_cmd awk
require_cmd curl
require_cmd docker
require_cmd git
require_cmd go
require_cmd jq
require_cmd python3
require_cmd ssh
require_cmd ssh-keygen

provider_id="${CARRIER_E2E_PROVIDER:-}"
if [[ -z "$provider_id" ]]; then
  if [[ -n "${OPENROUTER_API_KEY:-}" ]]; then
    provider_id="openrouter"
  else
    provider_id="anthropic"
  fi
fi
provider_key=""
case "$provider_id" in
  openrouter)
    if [[ -z "${OPENROUTER_API_KEY:-}" ]]; then
      echo "error: OPENROUTER_API_KEY is required when CARRIER_E2E_PROVIDER=openrouter" >&2
      usage >&2
      exit 1
    fi
    provider_key="${OPENROUTER_API_KEY}"
    unset OPENROUTER_API_KEY
    ;;
  anthropic)
    if [[ -z "${ANTHROPIC_API_KEY:-}" ]]; then
      echo "error: ANTHROPIC_API_KEY is required when CARRIER_E2E_PROVIDER=anthropic" >&2
      usage >&2
      exit 1
    fi
    provider_key="${ANTHROPIC_API_KEY}"
    unset ANTHROPIC_API_KEY
    ;;
  *)
    echo "error: unsupported CARRIER_E2E_PROVIDER=${provider_id}" >&2
    exit 1
    ;;
esac

daemon_port="${CARRIER_E2E_DAEMON_PORT:-$(find_port)}"
gateway_port="${CARRIER_E2E_GATEWAY_PORT:-$(find_port)}"
remote_ssh_port="${CARRIER_E2E_REMOTE_SSH_PORT:-$(find_port)}"
remote_host_id="${CARRIER_E2E_REMOTE_HOST_ID:-ubuntu-2404-docker}"
remote_host_name="${remote_host_id}"
remote_user="ubuntu"
remote_image_tag="carrier-remote-vps-ubuntu2404-e2e:latest"
remote_key_path="${ssh_dir}/id_ed25519"
remote_goal="${CARRIER_E2E_ORCHESTRATE_GOAL:-Decompose and execute this workflow on the remote Ubuntu host: zeroclaw drafts two concise release-note bullets about Carrier remote CLI stability; picoclaw drafts an independent, shorter operator handoff summary on the same topic; zeroclaw drafts an independent one-line release title on the same topic. Use both zeroclaw and picoclaw as separate workers. Keep each task self-contained and do not depend on outputs from other workers. Do not use shell commands, tools, or peripherals; answer from reasoning only.}"

export HOME="$home_dir"
export DOCKER_CONFIG="${host_docker_config}"
export CARRIER_CONFIG="${home_dir}/.carrier/config.v2.json"
export CARRIER_CREDENTIAL_STORE="${home_dir}/.carrier/credentials.json"
export CARRIER_REMOTE_CONTROL_STORE="${state_root}/remote-control.json"
export CARRIER_BOOTSTRAP_RUN_DIR="${state_root}/run"
export CARRIER_BOOTSTRAP_LOG_DIR="${artifacts_dir}/bootstrap-logs"
export CARRIER_DISABLE_KEYCHAIN="${CARRIER_DISABLE_KEYCHAIN:-1}"
export SESSION_DATA_DIR="${state_root}/session"
export CARRIER_SERVER_HOST="127.0.0.1"
export CARRIER_SERVER_PORT="${daemon_port}"
export CARRIER_GATEWAY_HOST="127.0.0.1"
export CARRIER_GATEWAY_PORT="${gateway_port}"
export CARRIER_DAEMON_BASE_URL="http://127.0.0.1:${daemon_port}"
export CARRIER_GATEWAY_API_TOKEN="admin-token"
export CARRIER_GATEWAY_ROLE_TOKENS="admin:admin-token"
export CARRIER_REMOTE_CONTROL_PLANE_ENABLED=1
export CARRIER_REMOTE_CHAT_ENABLED=1
export CARRIER_PROVIDER_BINDING_ENABLED=1
export CARRIER_TRIGGER_SCHEDULE_POLL_INTERVAL_SEC=1

if [[ -z "${DOCKER_HOST:-}" ]]; then
  if [[ -S "/var/run/docker.sock" ]]; then
    export DOCKER_HOST="unix:///var/run/docker.sock"
  elif [[ -n "$host_home" && -S "${host_home}/.docker/run/docker.sock" ]]; then
    export DOCKER_HOST="unix://${host_home}/.docker/run/docker.sock"
  fi
fi

echo "[setup] temp dir: ${tmp_dir}"
echo "[setup] daemon: http://127.0.0.1:${daemon_port}"
echo "[setup] gateway: http://127.0.0.1:${gateway_port}"
if [[ -n "${DOCKER_HOST:-}" ]]; then
  echo "[setup] docker host: ${DOCKER_HOST}"
fi

if [[ "${CARRIER_E2E_SKIP_BUILD:-0}" == "1" ]]; then
  bin_path="${CARRIER_E2E_BIN_PATH:-}"
  if [[ -z "$bin_path" || ! -x "$bin_path" ]]; then
    echo "error: CARRIER_E2E_BIN_PATH must point to an executable carrier binary when CARRIER_E2E_SKIP_BUILD=1" >&2
    exit 1
  fi
else
  bin_path="${tmp_dir}/carrier"
  echo "[1/6] build carrier"
  run_logged "go build ./cmd/carrier" "${artifacts_dir}/build.log" \
    go build -o "$bin_path" ./cmd/carrier
fi

echo "[2/6] onboard via CLI with ${provider_id}"
onboard_input="${tmp_dir}/onboard-input.txt"
printf '\n%s\n%s\n' "$provider_id" "$provider_key" >"$onboard_input"
run_logged "carrier onboard --cli" "${artifacts_dir}/onboard.log" \
  bash -lc "cat '${onboard_input}' | '${bin_path}' onboard --cli"
wait_for_http_ok "http://127.0.0.1:${daemon_port}/healthz" "daemon health"
wait_for_http_ok "http://127.0.0.1:${gateway_port}/healthz" "gateway health"
if ! jq -e --arg provider "$provider_id" 'any(.model_list[]?; .provider_id == $provider)' "$CARRIER_CONFIG" >/dev/null 2>&1; then
  echo "error: expected onboarded model_list to include provider ${provider_id}" >&2
  sed -n '1,240p' "$CARRIER_CONFIG" >&2 || true
  exit 1
fi
if [[ ! -s "$CARRIER_CREDENTIAL_STORE" ]]; then
  echo "error: expected credential store at ${CARRIER_CREDENTIAL_STORE}" >&2
  exit 1
fi

echo "[3/6] start Docker Ubuntu 24.04 remote fixture"
ssh-keygen -q -t ed25519 -N '' -f "$remote_key_path" >/dev/null
container_name="carrier-e2e-ubuntu2404-${remote_ssh_port}"
docker build -t "$remote_image_tag" -f "$repo_root/tests/fixtures/remote-vps-ubuntu2404/Dockerfile" "$repo_root" >/dev/null
docker run -d \
  --name "$container_name" \
  -p "${remote_ssh_port}:22" \
  -e AUTHORIZED_KEY="$(cat "${remote_key_path}.pub")" \
  "$remote_image_tag" >/dev/null
wait_for_ssh "$remote_user" "$remote_key_path" "$remote_ssh_port"
ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i "$remote_key_path" -p "$remote_ssh_port" "${remote_user}@127.0.0.1" \
  'source /etc/os-release && test "$ID" = "ubuntu" && test "$VERSION_ID" = "24.04" && printf "%s %s\n" "$ID" "$VERSION_ID"' \
  >"${artifacts_dir}/remote-platform.txt"

remote_add_agent() {
  local agent_id="$1"
  local remote_bin="$2"
  local add_log="${artifacts_dir}/remote-add-${agent_id}.log"
  local status_log="${artifacts_dir}/remote-status-${agent_id}.log"
  run_logged "carrier remote add ${agent_id}" "$add_log" \
    bash -lc "printf '' | '$bin_path' remote add '$agent_id' \
      --host-id '$remote_host_id' \
      --host 127.0.0.1 \
      --port '$remote_ssh_port' \
      --user '$remote_user' \
      --key-path '$remote_key_path' \
      --runtime-mode managed_gateway \
      --sync-provider '$provider_id' \
      --skip-reconnect-check"
  run_logged "carrier remote status ${agent_id}" "$status_log" \
    "$bin_path" remote status "$remote_host_id" "$agent_id"
  if ! ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i "$remote_key_path" -p "$remote_ssh_port" "${remote_user}@127.0.0.1" \
    "bash -lc 'command -v ${remote_bin} >/dev/null && ${remote_bin} --version 2>&1 | head -n 1'" \
    >"${artifacts_dir}/remote-version-${agent_id}.txt"; then
    printf 'version probe unavailable for %s\n' "$agent_id" >"${artifacts_dir}/remote-version-${agent_id}.txt"
  fi
}

run_remote_prompt() {
  local agent_id="$1"
  local token="$2"
  local session_id="e2e-${agent_id}-session"
  local json_file="${artifacts_dir}/remote-run-${agent_id}.json"
  local prompt="Do not use shell commands, tools, or peripherals. Reply with exactly ${token} and nothing else."
  capture_json_output "$json_file" \
    "$bin_path" remote run "$remote_host_id" "$agent_id" \
      -m "$prompt" \
      --session-id "$session_id" \
      --timeout-ms 120000 \
      --json
  if ! jq -e --arg token "$token" '.run.output | tostring | contains($token)' "$json_file" >/dev/null 2>&1; then
    echo "error: remote run token assertion failed for ${agent_id}" >&2
    sed -n '1,240p' "$json_file" >&2 || true
    exit 1
  fi
  if ! jq -e --arg session_id "$session_id" '.run.sessionId == $session_id' "$json_file" >/dev/null 2>&1; then
    echo "error: remote run session assertion failed for ${agent_id}" >&2
    sed -n '1,240p' "$json_file" >&2 || true
    exit 1
  fi
}

echo "[4/6] install OpenClaw and run a one-shot task"
remote_add_agent "openclaw" "openclaw"
run_remote_prompt "openclaw" "REMOTE_OPENCLAW_E2E_OK"

echo "[5/6] install PicoClaw and ZeroClaw, then run one-shot tasks"
remote_add_agent "picoclaw" "picoclaw"
run_remote_prompt "picoclaw" "REMOTE_PICOCLAW_E2E_OK"
remote_add_agent "zeroclaw" "zeroclaw"
run_remote_prompt "zeroclaw" "REMOTE_ZEROCLAW_E2E_OK"

echo "[6/6] verify dry-run decomposition, then execute remote orchestration"
plan_json="${artifacts_dir}/orchestrate-plan.json"
capture_json_output "$plan_json" \
  "$bin_path" orchestrate "$remote_goal" \
    --host-id "$remote_host_id" \
    --provider "$provider_id" \
    --max-concurrency 2 \
    --dry-run \
    --json
assert_jq "$plan_json" '.taskUnits | length >= 2' "orchestrate plan task count"
assert_jq "$plan_json" '[.taskUnits[]?.agentId] | index("picoclaw") != null' "orchestrate plan includes picoclaw"
assert_jq "$plan_json" '[.taskUnits[]?.agentId] | index("zeroclaw") != null' "orchestrate plan includes zeroclaw"
if ! jq -e --arg host "$remote_host_id" 'all(.taskUnits[]?; .hostId == $host)' "$plan_json" >/dev/null 2>&1; then
  echo "error: orchestrate plan host binding assertion failed" >&2
  sed -n '1,240p' "$plan_json" >&2 || true
  exit 1
fi

execution_json="${artifacts_dir}/orchestrate-execution.json"
capture_json_output "$execution_json" \
  "$bin_path" orchestrate "$remote_goal" \
    --host-id "$remote_host_id" \
    --provider "$provider_id" \
    --max-concurrency 2 \
    --policy-approve \
    --timeout 20m \
    --json
assert_jq "$execution_json" '.execution.id | type == "string" and length > 0' "orchestrate execution id"
assert_jq "$execution_json" '[.execution.results[]?.agentId] | index("picoclaw") != null' "orchestrate execution includes picoclaw result"
assert_jq "$execution_json" '[.execution.results[]?.agentId] | index("zeroclaw") != null' "orchestrate execution includes zeroclaw result"

execution_id="$(jq -r '.execution.id' "$execution_json")"

show_json="${artifacts_dir}/executions-show.json"
capture_json_output "$show_json" "$bin_path" executions show "$execution_id" --json
if ! jq -e --arg execution_id "$execution_id" '.execution.id == $execution_id' "$show_json" >/dev/null 2>&1; then
  echo "error: executions show id assertion failed" >&2
  sed -n '1,240p' "$show_json" >&2 || true
  exit 1
fi

evidence_json="${artifacts_dir}/executions-evidence.json"
capture_json_output "$evidence_json" "$bin_path" executions evidence "$execution_id" --json
if ! jq -e --arg execution_id "$execution_id" '.evidence.execution.id == $execution_id' "$evidence_json" >/dev/null 2>&1; then
  echo "error: executions evidence id assertion failed" >&2
  sed -n '1,240p' "$evidence_json" >&2 || true
  exit 1
fi

audit_json="${artifacts_dir}/executions-audit.json"
capture_json_output "$audit_json" "$bin_path" executions audit "$execution_id" --json
if ! jq -e --arg execution_id "$execution_id" '.executionId == $execution_id and (.events | type == "array")' "$audit_json" >/dev/null 2>&1; then
  echo "error: executions audit assertion failed" >&2
  sed -n '1,240p' "$audit_json" >&2 || true
  exit 1
fi

echo "[done] remote CLI Ubuntu flow completed"
echo "[artifacts] ${artifacts_dir}"
