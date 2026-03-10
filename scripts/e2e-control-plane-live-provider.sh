#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

usage() {
  cat <<'EOF'
Usage:
  bash scripts/e2e-control-plane-live-provider.sh

Environment:
  CARRIER_LIVE_PROVIDER   Provider id to use. Supported here: openrouter, openai, anthropic.
                          Default: openrouter
  CARRIER_LIVE_API_KEY    Required provider credential.
  CARRIER_LIVE_MODEL      Optional model label for audit/profile metadata.
  CARRIER_LIVE_BASE_URL   Optional provider base URL for profile metadata.
  CARRIER_E2E_DAEMON_PORT Optional daemon port. Default: 9090
  CARRIER_E2E_GATEWAY_PORT Optional gateway port. Default: 8787

What this script verifies:
  1) real local daemon + gateway boot
  2) real provider onboarding into Carrier config/credential store
  3) real isolation install/start for zeroclaw
  4) real execution completion through orchestrator
  5) real memory contract/provenance + evidence/audit export
  6) real CLI derived execution flows (rerun/clone)
EOF
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "error: required command not found: $cmd" >&2
    exit 1
  fi
}

wait_for_http_ok() {
  local url="$1"
  local label="$2"
  local attempts="${3:-90}"
  for attempt in $(seq 1 "$attempts"); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    if [[ "$attempt" -eq "$attempts" ]]; then
      echo "error: ${label} did not become ready: ${url}" >&2
      return 1
    fi
    sleep 1
  done
}

json_get() {
  local file="$1"
  local expr="$2"
  jq -er "$expr" "$file"
}

capture_json_output() {
  local outfile="$1"
  shift
  local raw_file="${outfile}.raw"
  "$@" >"$raw_file"
  awk 'BEGIN{emit=0} /^[[:space:]]*[{[]/{emit=1} emit{print}' "$raw_file" >"$outfile"
  if [[ ! -s "$outfile" ]]; then
    echo "error: failed to extract JSON output from command: $*" >&2
    cat "$raw_file" >&2 || true
    exit 1
  fi
}

provider_default_model() {
  case "$1" in
    openrouter) printf '%s' 'openrouter/arcee-ai/trinity-mini:free' ;;
    openai) printf '%s' 'openai/gpt-5.2' ;;
    anthropic) printf '%s' 'anthropic/claude-opus-4-6' ;;
    *)
      echo "error: unsupported live provider for this script: $1" >&2
      exit 2
      ;;
  esac
}

provider_env_var() {
  case "$1" in
    openrouter) printf '%s' 'OPENROUTER_API_KEY' ;;
    openai) printf '%s' 'OPENAI_API_KEY' ;;
    anthropic) printf '%s' 'ANTHROPIC_API_KEY' ;;
    *)
      echo "error: unsupported live provider for env var mapping: $1" >&2
      exit 2
      ;;
  esac
}

PROVIDER="$(printf '%s' "${CARRIER_LIVE_PROVIDER:-openrouter}" | tr '[:upper:]' '[:lower:]' | xargs)"
API_KEY="$(printf '%s' "${CARRIER_LIVE_API_KEY:-}" | xargs)"
MODEL="$(printf '%s' "${CARRIER_LIVE_MODEL:-}" | xargs)"
BASE_URL="$(printf '%s' "${CARRIER_LIVE_BASE_URL:-}" | xargs)"
DAEMON_PORT="${CARRIER_E2E_DAEMON_PORT:-9090}"
GATEWAY_PORT="${CARRIER_E2E_GATEWAY_PORT:-8787}"

case "$PROVIDER" in
  openrouter|openai|anthropic) ;;
  -h|--help)
    usage
    exit 0
    ;;
  *)
    echo "error: unsupported provider for live control-plane smoke: $PROVIDER" >&2
    usage >&2
    exit 2
    ;;
esac

if [[ -z "$API_KEY" ]]; then
  echo "error: CARRIER_LIVE_API_KEY is required" >&2
  exit 2
fi

if [[ -z "$MODEL" ]]; then
  MODEL="$(provider_default_model "$PROVIDER")"
fi

PROVIDER_ENV_VAR="$(provider_env_var "$PROVIDER")"

require_cmd go
require_cmd jq
require_cmd curl
require_cmd unzip

TMP_DIR="$(mktemp -d "/tmp/clp.XXXXXX")"
HOME_DIR="${TMP_DIR}/h"
mkdir -p "$HOME_DIR"

BIN_PATH="${TMP_DIR}/carrier"
DAEMON_LOG="${TMP_DIR}/daemon.log"
GATEWAY_LOG="${TMP_DIR}/gateway.log"
ONBOARD_LOG="${TMP_DIR}/onboard.log"
ADD_LOG="${TMP_DIR}/add-zeroclaw.log"
EXEC_CREATE_JSON="${TMP_DIR}/execution-create.json"
EXEC_AUTH_JSON="${TMP_DIR}/execution-authorize.json"
EXEC_STATUS_JSON="${TMP_DIR}/execution-status.json"
EVIDENCE_JSON="${TMP_DIR}/evidence.json"
AUDIT_JSON="${TMP_DIR}/audit.json"
METRICS_JSON="${TMP_DIR}/metrics.json"
DISTILL_JSON="${TMP_DIR}/distill.json"
RERUN_JSON="${TMP_DIR}/rerun.json"
CLONE_JSON="${TMP_DIR}/clone.json"
EVIDENCE_ZIP="${TMP_DIR}/execution-evidence.zip"

export HOME="$HOME_DIR"
export CARRIER_CONFIG="${TMP_DIR}/config.v2.json"
export CARRIER_CREDENTIAL_STORE="${TMP_DIR}/credentials.json"
export CARRIER_INSTANCE_STORE="${TMP_DIR}/instances.json"
export CARRIER_REMOTE_CONTROL_STORE="${TMP_DIR}/remote-control.json"
export CARRIER_DISABLE_KEYCHAIN=1
export CARRIER_TELEGRAM_BOT_TOKEN=""
export CARRIER_DISCORD_BOT_TOKEN=""
export CARRIER_DISCORD_PUBLIC_KEY=""
export CARRIER_FEISHU_APP_TOKEN=""
export CARRIER_FEISHU_VERIFICATION_TOKEN=""
export CARRIER_SERVER_HOST="127.0.0.1"
export CARRIER_SERVER_PORT="${DAEMON_PORT}"
export CARRIER_GATEWAY_HOST="127.0.0.1"
export CARRIER_GATEWAY_PORT="${GATEWAY_PORT}"
export CARRIER_DAEMON_BASE_URL="http://127.0.0.1:${DAEMON_PORT}"
export CARRIER_SERVER_API_TOKEN=""
export CARRIER_GATEWAY_API_TOKEN=""
export "${PROVIDER_ENV_VAR}=${API_KEY}"

cleanup() {
  set +e
  if [[ -n "${GATEWAY_PID:-}" ]]; then
    kill "${GATEWAY_PID}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${DAEMON_PID:-}" ]]; then
    kill "${DAEMON_PID}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${GATEWAY_PID:-}" ]]; then
    wait "${GATEWAY_PID}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${DAEMON_PID:-}" ]]; then
    wait "${DAEMON_PID}" >/dev/null 2>&1 || true
  fi
  echo "[artifacts] temp_dir=${TMP_DIR}"
}
trap cleanup EXIT

echo "[1/8] build carrier binary"
(
  cd "$ROOT_DIR"
  go build -o "$BIN_PATH" ./cmd/carrier
)

echo "[2/8] start daemon + gateway"
"$BIN_PATH" daemon >"$DAEMON_LOG" 2>&1 &
DAEMON_PID=$!
"$BIN_PATH" gateway >"$GATEWAY_LOG" 2>&1 &
GATEWAY_PID=$!

wait_for_http_ok "http://127.0.0.1:${DAEMON_PORT}/readyz" "daemon"
wait_for_http_ok "http://127.0.0.1:${GATEWAY_PORT}/healthz" "gateway"

echo "[3/8] onboard live provider (${PROVIDER}) in WebUI-only mode"
printf '\n%s\n%s\n' "$PROVIDER" "$API_KEY" | "$BIN_PATH" onboard >"$ONBOARD_LOG" 2>&1

if [[ -n "$MODEL" || -n "$BASE_URL" ]]; then
  echo "  normalize live provider model/base URL in Carrier config"
  CONFIG_PATCHED="${TMP_DIR}/config.patched.json"
  jq \
    --arg provider "$PROVIDER" \
    --arg model "$MODEL" \
    --arg base_url "$BASE_URL" \
    '
      .default_model = ($provider + "-live") |
      .model_list = [
        ((.model_list[0] // {}) + {
          "model_name": ($provider + "-live"),
          "provider_id": $provider,
          "credential_ref": $provider
        })
        | if $model != "" then .model = $model else . end
        | if $base_url != "" then .base_url = $base_url else . end
      ]
    ' "$CARRIER_CONFIG" >"$CONFIG_PATCHED"
  mv "$CONFIG_PATCHED" "$CARRIER_CONFIG"
fi

jq -e \
  --arg provider "$PROVIDER" \
  --arg model "$MODEL" \
  --arg base_url "$BASE_URL" \
  '
    .default_model == ($provider + "-live") and
    (.model_list | length == 1) and
    (.model_list[0].provider_id == $provider) and
    (.model_list[0].credential_ref == $provider) and
    ($model == "" or .model_list[0].model == $model) and
    ($base_url == "" or .model_list[0].base_url == $base_url)
  ' "$CARRIER_CONFIG" >/dev/null

echo "[4/8] install local zeroclaw with isolation"
printf '\n\n' | "$BIN_PATH" add zeroclaw --isolation >"$ADD_LOG" 2>&1

ZERO_INSTANCE_ID="$(jq -r '.instances | map(select((.agent_id // .agentId // .agentID // .type // "") == "zeroclaw")) | first | .id // empty' "$CARRIER_INSTANCE_STORE")"
if [[ -z "$ZERO_INSTANCE_ID" ]]; then
  echo "error: failed to resolve zeroclaw instance id from ${CARRIER_INSTANCE_STORE}" >&2
  cat "$ADD_LOG" >&2 || true
  exit 1
fi

echo "[5/8] attach/distill memory against the real instance"
"$BIN_PATH" memory attach --instance "$ZERO_INSTANCE_ID" --scope shared:incident-response >/dev/null
"$BIN_PATH" memory attach --instance "$ZERO_INSTANCE_ID" --scope shared:service-catalog >/dev/null
capture_json_output "$DISTILL_JSON" "$BIN_PATH" memory distill --instance "$ZERO_INSTANCE_ID" --dry-run --json
json_get "$DISTILL_JSON" '.result.runId // .run.runId // empty' >/dev/null

echo "[6/8] create and authorize a real execution"
curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  "http://127.0.0.1:${GATEWAY_PORT}/api/v1/orchestrator/executions" \
  --data @- >"$EXEC_CREATE_JSON" <<EOF
{
  "goal": "Live provider execution smoke for ${PROVIDER}",
  "requestedProvider": "${PROVIDER}",
  "requiredMemory": ["shared:incident-response", "shared:service-catalog", "shared:e2e-live-provider"],
  "distillOutputs": ["shared:incident-lessons", "shared:e2e-live-provider-distill"],
  "approvalScope": "infrastructure_only",
  "requiredWorkers": [
    {"hostId":"local","agentId":"zeroclaw","count":1}
  ],
  "taskUnits": [
    {
      "id":"task-weather",
      "input":"Provide a concise Tokyo weather summary in one short sentence. If live weather is unavailable, say so explicitly.",
      "hostId":"local",
      "agentId":"zeroclaw",
      "timeoutMs":120000,
      "retryBudget":1
    }
  ]
}
EOF

EXECUTION_ID="$(json_get "$EXEC_CREATE_JSON" '.execution.id')"
if [[ -z "$EXECUTION_ID" || "$EXECUTION_ID" == "null" ]]; then
  echo "error: missing execution id in create response" >&2
  cat "$EXEC_CREATE_JSON" >&2
  exit 1
fi

curl -fsS -X POST \
  -H 'Content-Type: application/json' \
  "http://127.0.0.1:${GATEWAY_PORT}/api/v1/orchestrator/executions/${EXECUTION_ID}/authorize" \
  --data '{"approved":true,"actor":"live-provider-e2e"}' >"$EXEC_AUTH_JSON"

FINAL_STATUS=""
for _ in $(seq 1 180); do
  curl -fsS "http://127.0.0.1:${GATEWAY_PORT}/api/v1/orchestrator/executions/${EXECUTION_ID}" >"$EXEC_STATUS_JSON"
  FINAL_STATUS="$(jq -r '.execution.status // empty' "$EXEC_STATUS_JSON")"
  case "$FINAL_STATUS" in
    completed|partial_completed|retryable_failed|failed|cancelled|declined)
      break
      ;;
  esac
  sleep 2
done

if [[ "$FINAL_STATUS" != "completed" ]]; then
  echo "error: live provider execution did not complete successfully (status=${FINAL_STATUS})" >&2
  cat "$EXEC_STATUS_JSON" >&2
  exit 1
fi

jq -e --arg provider "$PROVIDER" '
  (.execution.requestedProvider == $provider) and
  (.execution.requiredMemory | index("shared:e2e-live-provider")) and
  (.execution.memoryContractDigest | type == "string" and length > 0) and
  (.execution.memoryProvenance | index("shared:incident-response")) and
  (.execution.distillOutputs | index("shared:e2e-live-provider-distill")) and
  (.execution.results | length >= 1) and
  (.execution.results[0].status == "completed") and
  (.execution.results[0].output | type == "string" and length > 0)
' "$EXEC_STATUS_JSON" >/dev/null

echo "[7/8] export evidence, audit, metrics, and derived executions"
capture_json_output "$EVIDENCE_JSON" "$BIN_PATH" executions evidence "$EXECUTION_ID" --format json --json
"$BIN_PATH" executions evidence "$EXECUTION_ID" --format zip --output "$EVIDENCE_ZIP" >/dev/null
capture_json_output "$AUDIT_JSON" "$BIN_PATH" executions audit "$EXECUTION_ID" --json
curl -fsS "http://127.0.0.1:${GATEWAY_PORT}/api/v1/orchestrator/metrics" >"$METRICS_JSON"
capture_json_output "$RERUN_JSON" "$BIN_PATH" executions rerun "$EXECUTION_ID" --json
capture_json_output "$CLONE_JSON" "$BIN_PATH" executions clone "$EXECUTION_ID" --json

jq -e --arg exec_id "$EXECUTION_ID" '
  (.evidence.execution.id == $exec_id) and
  (.evidence.plan.memoryContractDigest | type == "string" and length > 0) and
  (.evidence.plan.requiredMemory | index("shared:e2e-live-provider")) and
  (.evidence.plan.distillOutputs | index("shared:e2e-live-provider-distill")) and
  (.evidence.resultSummary.completed >= 1)
' "$EVIDENCE_JSON" >/dev/null

jq -e '.events | length >= 1' "$AUDIT_JSON" >/dev/null
jq -e --arg exec_id "$EXECUTION_ID" '.executionId == $exec_id or .executionId == ""' "$AUDIT_JSON" >/dev/null
jq -e --arg exec_id "$EXECUTION_ID" '.execution.parentExecutionId == $exec_id and .execution.launchReason == "rerun_execution"' "$RERUN_JSON" >/dev/null
jq -e --arg exec_id "$EXECUTION_ID" '.execution.parentExecutionId == $exec_id and .execution.launchReason == "clone_execution"' "$CLONE_JSON" >/dev/null

unzip -l "$EVIDENCE_ZIP" | grep -q 'provider-attribution.json'
unzip -l "$EVIDENCE_ZIP" | grep -q 'plan.json'
unzip -l "$EVIDENCE_ZIP" | grep -q 'audit.json'

echo "[8/8] summarize"
echo "execution_id=${EXECUTION_ID}"
echo "provider=${PROVIDER}"
echo "model=${MODEL}"
echo "instance_id=${ZERO_INSTANCE_ID}"
echo "evidence_zip=${EVIDENCE_ZIP}"
echo "live provider control-plane smoke passed"
