#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/remote-vps-agent-suite.sh \
    --host-id <id> \
    --host <ip-or-domain> \
    --port <port> \
    --user <ssh-user> \
    --key-path <private-key-path> \
    [--agent-id <agent-id>] \
    [--gateway-url <url>] \
    [--workspace-root <absolute-path>] \
    [--runtime-mode <on_demand|managed_gateway>] \
    [--skip-reconnect-check] \
    [--no-reset-remote] \
    [--no-gateway-autostart]

What this script does:
  1) (optional) reset remote VPS state for OpenClaw/PicoClaw/ZeroClaw/CodeAgent tools
  2) install/verify OpenClaw on remote VPS via remote control-plane
  3) create/verify remote OpenClaw agents: picoclaw, zeroclaw
  4) install/verify remote codeagent backends: codex, opencode
  5) run backend smoke calls and print normalized result envelopes
USAGE
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "error: required command not found: $cmd" >&2
    exit 1
  fi
}

q() {
  printf "%q" "$1"
}

HOST_ID=""
HOST_ADDR=""
PORT=""
SSH_USER=""
KEY_PATH=""
AGENT_ID="main"
GATEWAY_URL="${CARRIER_GATEWAY_URL:-http://127.0.0.1:8787}"
WORKSPACE_ROOT="/home/carrier/workspace"
RUNTIME_MODE="on_demand"
SKIP_RECONNECT_CHECK=0
RESET_REMOTE=1
GATEWAY_AUTOSTART=1
GATEWAY_PID=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --host-id)
      HOST_ID="${2:-}"
      shift 2
      ;;
    --host)
      HOST_ADDR="${2:-}"
      shift 2
      ;;
    --port)
      PORT="${2:-}"
      shift 2
      ;;
    --user)
      SSH_USER="${2:-}"
      shift 2
      ;;
    --key-path)
      KEY_PATH="${2:-}"
      shift 2
      ;;
    --agent-id)
      AGENT_ID="${2:-}"
      shift 2
      ;;
    --gateway-url)
      GATEWAY_URL="${2:-}"
      shift 2
      ;;
    --workspace-root)
      WORKSPACE_ROOT="${2:-}"
      shift 2
      ;;
    --runtime-mode)
      RUNTIME_MODE="${2:-}"
      shift 2
      ;;
    --skip-reconnect-check)
      SKIP_RECONNECT_CHECK=1
      shift
      ;;
    --no-reset-remote)
      RESET_REMOTE=0
      shift
      ;;
    --no-gateway-autostart)
      GATEWAY_AUTOSTART=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

require_cmd curl
require_cmd jq
require_cmd ssh
require_cmd go
require_cmd carrier

if [[ -z "$HOST_ID" || -z "$HOST_ADDR" || -z "$PORT" || -z "$SSH_USER" || -z "$KEY_PATH" ]]; then
  echo "error: missing required arguments" >&2
  usage
  exit 1
fi
if [[ ! -f "$KEY_PATH" ]]; then
  echo "error: key file not found: $KEY_PATH" >&2
  exit 1
fi
if [[ ! "$PORT" =~ ^[0-9]+$ ]]; then
  echo "error: --port must be numeric" >&2
  exit 1
fi
if [[ "$RUNTIME_MODE" != "on_demand" && "$RUNTIME_MODE" != "managed_gateway" ]]; then
  echo "error: runtime-mode must be on_demand or managed_gateway" >&2
  exit 1
fi
if [[ "${WORKSPACE_ROOT#/}" == "$WORKSPACE_ROOT" ]]; then
  echo "error: --workspace-root must be an absolute path" >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
cleanup() {
  if [[ -n "$GATEWAY_PID" ]]; then
    kill "$GATEWAY_PID" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

ssh_remote() {
  local cmd="$1"
  ssh -i "$KEY_PATH" -p "$PORT" -o StrictHostKeyChecking=accept-new "${SSH_USER}@${HOST_ADDR}" "$cmd"
}

gateway_health_ok() {
  curl -fsS "${GATEWAY_URL}/healthz" >/dev/null 2>&1
}

start_gateway_if_needed() {
  if gateway_health_ok; then
    echo "[pre] gateway already healthy: $GATEWAY_URL"
    return 0
  fi
  if [[ "$GATEWAY_AUTOSTART" -ne 1 ]]; then
    echo "error: gateway is not healthy and --no-gateway-autostart is set" >&2
    exit 1
  fi
  if [[ "$GATEWAY_URL" != http://127.0.0.1:* ]]; then
    echo "error: automatic gateway startup only supports local loopback gateway-url, got: $GATEWAY_URL" >&2
    exit 1
  fi

  echo "[pre] bootstrapping local carrier runtime (daemon + gateway)"
  mkdir -p "$TMP_DIR/go-cache"
  GOCACHE="$TMP_DIR/go-cache" go run ./cmd/carrier >"$TMP_DIR/carrier-bootstrap.log" 2>&1 || true
  for _ in $(seq 1 60); do
    if gateway_health_ok; then
      echo "[pre] gateway healthy"
      return 0
    fi
    sleep 1
  done
  echo "error: gateway failed to become healthy at $GATEWAY_URL" >&2
  sed -n '1,260p' "$TMP_DIR/carrier-bootstrap.log" >&2 || true
  exit 1
}

agent_exists_in_json() {
  local name="$1"
  jq -e --arg name "$name" '
    def rows:
      if type=="array" then .
      elif type=="object" then (.agents // .items // .data // .rows // [])
      else []
      end;
    [rows[]? | select(((.name // .id // .agent // .agentId // "") | tostring | ascii_downcase) == ($name | ascii_downcase))] | length > 0
  ' >/dev/null
}

ensure_remote_agent() {
  local agent_name="$1"
  local ws_dir="$2"
  local list_json
  list_json="$(ssh_remote "set -e; export LC_ALL=C LANG=C PATH=\"\$HOME/.npm-global/bin:\$HOME/.local/bin:\$PATH\"; openclaw agents list --json")"
  if printf '%s' "$list_json" | agent_exists_in_json "$agent_name"; then
    echo "  remote agent already exists: $agent_name"
    return 0
  fi

  ssh_remote "set -e; export LC_ALL=C LANG=C PATH=\"\$HOME/.npm-global/bin:\$HOME/.local/bin:\$PATH\"; mkdir -p $(q "$ws_dir"); openclaw agents add $(q "$agent_name") --workspace $(q "$ws_dir") --model openai/gpt-4.1 --non-interactive --json >/dev/null"
  list_json="$(ssh_remote "set -e; export LC_ALL=C LANG=C PATH=\"\$HOME/.npm-global/bin:\$HOME/.local/bin:\$PATH\"; openclaw agents list --json")"
  if ! printf '%s' "$list_json" | agent_exists_in_json "$agent_name"; then
    echo "error: remote agent not found after add: $agent_name" >&2
    printf '%s\n' "$list_json" >&2
    exit 1
  fi
  echo "  remote agent added: $agent_name"
}

api_json() {
  local method="$1"
  local path="$2"
  local body="$3"
  local out_file="$4"
  local status
  if [[ -n "$body" ]]; then
    status="$(curl -sS -X "$method" -H "Content-Type: application/json" "$GATEWAY_URL$path" --data "$body" -o "$out_file" -w "%{http_code}")"
  else
    status="$(curl -sS -X "$method" -H "Content-Type: application/json" "$GATEWAY_URL$path" -o "$out_file" -w "%{http_code}")"
  fi
  printf "%s" "$status"
}

expect_2xx() {
  local code="$1"
  local label="$2"
  local out_file="$3"
  if [[ ! "$code" =~ ^2 ]]; then
    echo "[$label] failed: HTTP $code" >&2
    sed -n '1,220p' "$out_file" >&2 || true
    exit 1
  fi
}

start_gateway_if_needed

if [[ "$RESET_REMOTE" -eq 1 ]]; then
  echo "[1/7] reset remote VPS state"
  ssh_remote "set -euo pipefail; rm -rf ~/.openclaw ~/.picoclaw ~/.zeroclaw ~/.carrier $(q "$WORKSPACE_ROOT"); if command -v npm >/dev/null 2>&1; then npm uninstall -g openclaw @openai/codex opencode opencode-ai >/dev/null 2>&1 || true; fi; if command -v bun >/dev/null 2>&1; then bun remove -g openclaw @openai/codex opencode opencode-ai >/dev/null 2>&1 || true; fi; rm -f \"\$HOME/.npm-global/bin/openclaw\" \"\$HOME/.npm-global/bin/codex\" \"\$HOME/.npm-global/bin/opencode\" || true; for b in openclaw codex opencode; do if command -v \"\$b\" >/dev/null 2>&1; then echo \"\$b still present: \$(command -v \$b)\"; else echo \"\$b not found\"; fi; done"
else
  echo "[1/7] reset remote VPS state skipped (--no-reset-remote)"
fi

echo "[2/7] install/verify remote OpenClaw"
install_cmd=(carrier remote add openclaw
  --host-id "$HOST_ID"
  --host "$HOST_ADDR"
  --port "$PORT"
  --user "$SSH_USER"
  --key-path "$KEY_PATH"
  --runtime-mode "$RUNTIME_MODE")
if [[ "$SKIP_RECONNECT_CHECK" -eq 1 ]]; then
  install_cmd+=(--skip-reconnect-check)
fi
gateway_host_port="$(printf '%s' "$GATEWAY_URL" | sed -E 's#^https?://##; s#/.*$##')"
carrier_gateway_host="${gateway_host_port%%:*}"
carrier_gateway_port="${gateway_host_port##*:}"
if [[ "$carrier_gateway_host" == "$carrier_gateway_port" ]]; then
  if [[ "$GATEWAY_URL" == https://* ]]; then
    carrier_gateway_port="443"
  else
    carrier_gateway_port="80"
  fi
fi
CARRIER_GATEWAY_HOST="$carrier_gateway_host" CARRIER_GATEWAY_PORT="$carrier_gateway_port" "${install_cmd[@]}"

echo "[3/7] verify remote OpenClaw CLI and ensure workspace git repo"
ssh_remote "set -e; export LC_ALL=C LANG=C PATH=\"\$HOME/.npm-global/bin:\$HOME/.local/bin:\$PATH\"; command -v openclaw; openclaw --version | head -n 1; mkdir -p $(q "$WORKSPACE_ROOT"); cd $(q "$WORKSPACE_ROOT"); if [ ! -d .git ]; then git init >/dev/null 2>&1; git config user.email carrier@example.local; git config user.name carrier-bot; fi; pwd"

echo "[4/7] ensure remote agents: picoclaw, zeroclaw"
ensure_remote_agent "picoclaw" "$WORKSPACE_ROOT/agents/picoclaw"
ensure_remote_agent "zeroclaw" "$WORKSPACE_ROOT/agents/zeroclaw"
echo "  remote agent list:"
ssh_remote "set -e; export LC_ALL=C LANG=C PATH=\"\$HOME/.npm-global/bin:\$HOME/.local/bin:\$PATH\"; openclaw agents list --json" | jq '.'

echo "[5/7] install codeagent backends: codex, opencode"
install_codex_out="$TMP_DIR/install-codex.json"
install_codex_code="$(api_json POST "/api/v1/remote/hosts/$HOST_ID/instances/$AGENT_ID/codeagent/install" "{\"backend\":\"codex\",\"workspaceRoot\":\"$WORKSPACE_ROOT\"}" "$install_codex_out")"
expect_2xx "$install_codex_code" "codeagent install codex" "$install_codex_out"
jq '{backend: .install.backend, installed: .install.installed, version: .install.version}' "$install_codex_out"

install_opencode_out="$TMP_DIR/install-opencode.json"
install_opencode_code="$(api_json POST "/api/v1/remote/hosts/$HOST_ID/instances/$AGENT_ID/codeagent/install" "{\"backend\":\"opencode\",\"workspaceRoot\":\"$WORKSPACE_ROOT\"}" "$install_opencode_out")"
expect_2xx "$install_opencode_code" "codeagent install opencode" "$install_opencode_out"
jq '{backend: .install.backend, installed: .install.installed, version: .install.version}' "$install_opencode_out"

echo "[6/7] verify codeagent version + health"
for backend in codex opencode; do
  ver_out="$TMP_DIR/version-$backend.json"
  ver_code="$(api_json GET "/api/v1/remote/hosts/$HOST_ID/instances/$AGENT_ID/codeagent/version?backend=$backend" "" "$ver_out")"
  expect_2xx "$ver_code" "codeagent version $backend" "$ver_out"
  jq '{backend: .version.backend, version: .version.value}' "$ver_out"

  health_out="$TMP_DIR/health-$backend.json"
  health_code="$(api_json GET "/api/v1/remote/hosts/$HOST_ID/instances/$AGENT_ID/codeagent/health?backend=$backend&workspaceRoot=$(printf '%s' "$WORKSPACE_ROOT" | jq -sRr @uri)" "" "$health_out")"
  expect_2xx "$health_code" "codeagent health $backend" "$health_out"
  jq '{backend: .health.backend, healthy: .health.healthy, workspaceRoot: .health.workspaceRoot}' "$health_out"
done

echo "[7/7] codeagent run smoke (non-blocking for backend/runtime-specific auth/policy exits)"
for backend in codex opencode; do
  run_out="$TMP_DIR/run-$backend.json"
  payload="$(jq -cn --arg backend "$backend" --arg ws "$WORKSPACE_ROOT" '{
    backend: $backend,
    workspaceRoot: $ws,
    capability: "run_shell",
    command: "echo CODEAGENT_REMOTE_SMOKE && pwd",
    cwd: $ws
  }')"
  run_code="$(api_json POST "/api/v1/remote/hosts/$HOST_ID/instances/$AGENT_ID/codeagent/run" "$payload" "$run_out")"
  expect_2xx "$run_code" "codeagent run $backend" "$run_out"
  jq '{backend: .run.backend, ok: .run.result.ok, exit_code: .run.result.exit_code, policy_decision: .run.result.policy_decision, stderr: .run.result.stderr}' "$run_out"
done

echo "done: remote VPS agent suite succeeded for host=$HOST_ID agent=$AGENT_ID"
