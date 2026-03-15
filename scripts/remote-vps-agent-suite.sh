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
  6) optionally run provider-backed remote execution/evidence smoke when:
     CARRIER_LIVE_PROVIDER and CARRIER_LIVE_API_KEY are set
USAGE
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "error: required command not found: $cmd" >&2
    exit 1
  fi
}

default_live_model() {
  case "$1" in
    openrouter) printf '%s' 'openai/gpt-4o-mini' ;;
    openai|openai-compatible|openai-codex) printf '%s' 'openai/gpt-4.1-mini' ;;
    anthropic) printf '%s' 'claude-3-5-haiku-latest' ;;
    *) printf '%s' 'openai/gpt-4.1-mini' ;;
  esac
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
LIVE_PROVIDER="$(printf '%s' "${CARRIER_LIVE_PROVIDER:-}" | tr '[:upper:]' '[:lower:]' | xargs)"
LIVE_API_KEY="$(printf '%s' "${CARRIER_LIVE_API_KEY:-}" | xargs)"
LIVE_MODEL="$(printf '%s' "${CARRIER_LIVE_MODEL:-}" | xargs)"
LIVE_BASE_URL="$(printf '%s' "${CARRIER_LIVE_BASE_URL:-}" | xargs)"

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
if [[ -n "$LIVE_PROVIDER" || -n "$LIVE_API_KEY" || -n "$LIVE_MODEL" || -n "$LIVE_BASE_URL" ]]; then
  if [[ -z "$LIVE_PROVIDER" || -z "$LIVE_API_KEY" ]]; then
    echo "error: live provider smoke requires both CARRIER_LIVE_PROVIDER and CARRIER_LIVE_API_KEY" >&2
    exit 1
  fi
fi

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/carrier-remote-vps-suite.XXXXXX")"
cleanup() {
  if [[ -n "$GATEWAY_PID" ]]; then
    kill "$GATEWAY_PID" >/dev/null 2>&1 || true
  fi
  echo "[artifacts] temp_dir=${TMP_DIR}"
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
  require_cmd go
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

if [[ -n "$LIVE_PROVIDER" && -n "$LIVE_API_KEY" ]]; then
  echo "[8/12] create provider profile + host binding for remote execution"
  profile_id="$(printf 'live-%s-%s' "$HOST_ID" "$LIVE_PROVIDER" | tr '/: ' '---')"
  profile_name="live-${HOST_ID}-${LIVE_PROVIDER}"
  profile_payload="$(jq -cn \
    --arg id "$profile_id" \
    --arg name "$profile_name" \
    --arg provider "$LIVE_PROVIDER" \
    --arg model "${LIVE_MODEL:-$(default_live_model "$LIVE_PROVIDER")}" \
    --arg base_url "$LIVE_BASE_URL" \
    --arg auth_ref "$LIVE_API_KEY" \
    '{
      id: $id,
      name: $name,
      provider: $provider,
      model: $model,
      enabled: true
    }
    + (if $base_url != "" then {baseUrl: $base_url} else {} end)
    + {authRef: $auth_ref}')"
  profile_out="$TMP_DIR/live-provider-profile.json"
  profile_code="$(api_json POST "/api/v1/provider-profiles" "$profile_payload" "$profile_out")"
  expect_2xx "$profile_code" "provider profile upsert" "$profile_out"

  binding_id="$(printf 'bind-live-%s-%s' "$HOST_ID" "$LIVE_PROVIDER" | tr '/: ' '---')"
  binding_payload="$(jq -cn \
    --arg id "$binding_id" \
    --arg profile_id "$profile_id" \
    --arg host_id "$HOST_ID" \
    '{
      id: $id,
      profileId: $profile_id,
      targetType: "host",
      targetId: $host_id,
      syncMode: "always_push"
    }')"
  binding_out="$TMP_DIR/live-provider-binding.json"
  binding_code="$(api_json POST "/api/v1/provider-bindings" "$binding_payload" "$binding_out")"
  expect_2xx "$binding_code" "provider binding upsert" "$binding_out"

  echo "[9/12] resolve provider governance for remote agent ${AGENT_ID}"
  resolve_out="$TMP_DIR/live-provider-resolve.json"
  resolve_code="$(api_json GET "/api/v1/provider-governance/resolve?hostId=$(printf '%s' "$HOST_ID" | jq -sRr @uri)&agentId=$(printf '%s' "$AGENT_ID" | jq -sRr @uri)" "" "$resolve_out")"
  expect_2xx "$resolve_code" "provider governance resolve" "$resolve_out"
  jq -e --arg provider "$LIVE_PROVIDER" '
    .resolution.status == "resolved" and
    .resolution.provider == $provider and
    (.resolution.model | type == "string" and length > 0)
  ' "$resolve_out" >/dev/null

  echo "[10/12] create and authorize remote execution"
  execution_create_out="$TMP_DIR/live-remote-execution-create.json"
  execution_payload="$(jq -cn \
    --arg goal "Remote live provider execution smoke" \
    --arg provider "$LIVE_PROVIDER" \
    --arg host_id "$HOST_ID" \
    --arg agent_id "$AGENT_ID" \
    '{
      goal: $goal,
      requestedProvider: $provider,
      requiredMemory: ["shared:remote-runbook", "shared:service-catalog"],
      distillOutputs: ["shared:remote-lessons"],
      approvalScope: "infrastructure_only",
      requiredWorkers: [
        {hostId: $host_id, agentId: $agent_id, count: 1}
      ],
      taskUnits: [
        {
          id: "remote-task-1",
          input: "Reply with one short sentence that includes REMOTE_EXECUTION_OK and shared:remote-runbook.",
          hostId: $host_id,
          agentId: $agent_id,
          timeoutMs: 120000,
          retryBudget: 1
        }
      ]
    }')"
  execution_create_code="$(api_json POST "/api/v1/orchestrator/executions" "$execution_payload" "$execution_create_out")"
  expect_2xx "$execution_create_code" "remote execution create" "$execution_create_out"
  execution_id="$(jq -r '.execution.id // empty' "$execution_create_out")"
  if [[ -z "$execution_id" ]]; then
    echo "error: missing execution id in remote execution create response" >&2
    sed -n '1,220p' "$execution_create_out" >&2 || true
    exit 1
  fi

  execution_auth_out="$TMP_DIR/live-remote-execution-authorize.json"
  execution_auth_code="$(api_json POST "/api/v1/orchestrator/executions/$execution_id/authorize" '{"approved":true,"actor":"remote-vps-suite"}' "$execution_auth_out")"
  expect_2xx "$execution_auth_code" "remote execution authorize" "$execution_auth_out"

  final_status=""
  execution_status_out="$TMP_DIR/live-remote-execution-status.json"
  for _ in $(seq 1 180); do
    status_code="$(api_json GET "/api/v1/orchestrator/executions/$execution_id" "" "$execution_status_out")"
    expect_2xx "$status_code" "remote execution status" "$execution_status_out"
    final_status="$(jq -r '.execution.status // empty' "$execution_status_out")"
    case "$final_status" in
      completed|partial_completed|retryable_failed|failed|cancelled|declined)
        break
        ;;
    esac
    sleep 2
  done
  if [[ "$final_status" != "completed" ]]; then
    echo "error: remote execution did not complete successfully (status=${final_status})" >&2
    sed -n '1,260p' "$execution_status_out" >&2 || true
    exit 1
  fi
  jq -e '
    (.execution.memoryContractDigest | type == "string" and length > 0) and
    (.execution.memoryProvenance | index("shared:remote-runbook")) and
    (.execution.results | length >= 1) and
    (.execution.results[0].status == "completed") and
    (.execution.results[0].output | tostring | contains("REMOTE_EXECUTION_OK"))
  ' "$execution_status_out" >/dev/null

  echo "[11/12] export evidence, audit, and metrics"
  evidence_json="$TMP_DIR/live-remote-evidence.json"
  evidence_zip="$TMP_DIR/live-remote-evidence.zip"
  audit_json="$TMP_DIR/live-remote-audit.json"
  metrics_json="$TMP_DIR/live-remote-metrics.json"
  carrier executions evidence "$execution_id" --format json --json >"$evidence_json"
  carrier executions evidence "$execution_id" --format zip --output "$evidence_zip" >/dev/null
  carrier executions audit "$execution_id" --json >"$audit_json"
  metrics_code="$(api_json GET "/api/v1/orchestrator/metrics" "" "$metrics_json")"
  expect_2xx "$metrics_code" "orchestrator metrics" "$metrics_json"
  jq -e --arg exec_id "$execution_id" --arg provider "$LIVE_PROVIDER" '
    (.evidence.execution.id == $exec_id) and
    (.evidence.providerAttribution.totalEstimatedCostUsd > 0) and
    (.evidence.governance.providerResolutions | length >= 1) and
    (.evidence.governance.providerResolutions[0].provider == $provider)
  ' "$evidence_json" >/dev/null
  jq -e '.events | length >= 1' "$audit_json" >/dev/null
  jq -e '.metrics.providers.totalEstimatedCostUsd > 0' "$metrics_json" >/dev/null
  unzip -l "$evidence_zip" | grep -q 'provider-attribution.json'
  unzip -l "$evidence_zip" | grep -q 'audit.json'

  echo "[12/12] remote live execution summary"
  echo "remote_execution_id=$execution_id"
  echo "remote_provider=$LIVE_PROVIDER"
else
  echo "[8/8] live provider execution smoke skipped (set CARRIER_LIVE_PROVIDER and CARRIER_LIVE_API_KEY to enable)"
fi

echo "done: remote VPS agent suite succeeded for host=$HOST_ID agent=$AGENT_ID"
