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
  CARRIER_LIVE_TRANSCRIPTION_MODEL Optional transcription model override for media smoke.
  CARRIER_LIVE_TRANSCRIPTION_MODE Optional mode override: hard_required, soft_optional, unsupported.
  CARRIER_LIVE_REQUIRE_TRANSCRIPTION When set to 1, fail if live transcription cannot run.
  CARRIER_LIVE_NATIVE_MEDIA_MODE Optional native media output mode override: hard_required, soft_optional, unsupported.
  CARRIER_LIVE_REQUIRE_NATIVE_MEDIA When set to 1, fail if native media output cannot run.
  CARRIER_E2E_STATE_DIR   Optional persistent state root. When set, Carrier's HOME, Lima cache,
                          and managed-instance records are reused across runs, while each run still
                          writes logs/artifacts into a fresh subdirectory under <state>/runs/.
  CARRIER_E2E_DAEMON_PORT Optional daemon port. Default: 9090
  CARRIER_E2E_GATEWAY_PORT Optional gateway port. Default: 8787

What this script verifies:
  1) real provider onboarding into Carrier config/credential store
  2) real local daemon + gateway boot
  3) real isolation install/start for zeroclaw
  4) real launcher/run/heartbeat/cron surfaces
  5) real media transcription and native speech output through a live provider
  6) real execution completion through orchestrator
  7) real memory contract/provenance + evidence/audit export
  8) real CLI derived execution flows (rerun/clone)
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

transcription_skip_allowed() {
  local message="$1"
  if printf '%s' "$message" | grep -qi 'No endpoints found that support input audio'; then
    return 0
  fi
  if printf '%s' "$message" | grep -qi 'in balance for audio'; then
    return 0
  fi
  return 1
}

provider_transcription_mode() {
  case "$1" in
    openai) printf '%s' 'hard_required' ;;
    openrouter) printf '%s' 'soft_optional' ;;
    anthropic) printf '%s' 'unsupported' ;;
    *)
      echo "error: unsupported provider for transcription mode mapping: $1" >&2
      exit 2
      ;;
  esac
}

provider_native_media_mode() {
  case "$1" in
    openai) printf '%s' 'hard_required' ;;
    openrouter) printf '%s' 'soft_optional' ;;
    anthropic) printf '%s' 'unsupported' ;;
    *)
      echo "error: unsupported provider for native media mode mapping: $1" >&2
      exit 2
      ;;
  esac
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

control_plane_processes_alive() {
  if [[ -z "${DAEMON_PID:-}" || -z "${GATEWAY_PID:-}" ]]; then
    return 1
  fi
  kill -0 "${DAEMON_PID}" >/dev/null 2>&1 && kill -0 "${GATEWAY_PID}" >/dev/null 2>&1
}

stop_control_plane_processes() {
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
  GATEWAY_PID=""
  DAEMON_PID=""
}

wait_for_control_plane_ready() {
  local daemon_url="$1"
  local gateway_url="$2"
  local attempts="${3:-60}"
  local attempt=""
  for attempt in $(seq 1 "$attempts"); do
    if ! control_plane_processes_alive; then
      return 1
    fi
    if curl -fsS "$daemon_url" >/dev/null 2>&1 && curl -fsS "$gateway_url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

port_is_available() {
  local port="$1"
  python3 - "$port" <<'PY'
import socket
import sys

port = int(sys.argv[1])
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
try:
    s.bind(("127.0.0.1", port))
except OSError:
    sys.exit(1)
finally:
    s.close()
PY
}

pick_ephemeral_port() {
  python3 - <<'PY'
import socket

s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

run_with_timeout() {
  local timeout_sec="$1"
  shift
  local pid=""
  local waited=0

  "$@" &
  pid=$!

  while kill -0 "$pid" >/dev/null 2>&1; do
    if [[ "$waited" -ge "$timeout_sec" ]]; then
      kill -9 "$pid" >/dev/null 2>&1 || true
      wait "$pid" >/dev/null 2>&1 || true
      return 124
    fi
    sleep 1
    waited=$((waited + 1))
  done

  wait "$pid"
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
    openrouter) printf '%s' 'openrouter/google/gemini-2.0-flash-001' ;;
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
TRANSCRIPTION_MODEL="$(printf '%s' "${CARRIER_LIVE_TRANSCRIPTION_MODEL:-}" | xargs)"
TRANSCRIPTION_MODE="$(printf '%s' "${CARRIER_LIVE_TRANSCRIPTION_MODE:-}" | xargs)"
REQUIRE_TRANSCRIPTION="$(printf '%s' "${CARRIER_LIVE_REQUIRE_TRANSCRIPTION:-}" | xargs)"
NATIVE_MEDIA_MODE="$(printf '%s' "${CARRIER_LIVE_NATIVE_MEDIA_MODE:-}" | xargs)"
REQUIRE_NATIVE_MEDIA="$(printf '%s' "${CARRIER_LIVE_REQUIRE_NATIVE_MEDIA:-}" | xargs)"
STATE_ROOT="$(printf '%s' "${CARRIER_E2E_STATE_DIR:-}" | xargs)"
DAEMON_PORT="$(printf '%s' "${CARRIER_E2E_DAEMON_PORT:-}" | xargs)"
GATEWAY_PORT="$(printf '%s' "${CARRIER_E2E_GATEWAY_PORT:-}" | xargs)"

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

if [[ -z "$TRANSCRIPTION_MODE" ]]; then
  TRANSCRIPTION_MODE="$(provider_transcription_mode "$PROVIDER")"
fi
case "$TRANSCRIPTION_MODE" in
  hard_required|soft_optional|unsupported) ;;
  *)
    echo "error: unsupported CARRIER_LIVE_TRANSCRIPTION_MODE: ${TRANSCRIPTION_MODE}" >&2
    exit 2
    ;;
esac

if [[ -z "$NATIVE_MEDIA_MODE" ]]; then
  NATIVE_MEDIA_MODE="$(provider_native_media_mode "$PROVIDER")"
fi
case "$NATIVE_MEDIA_MODE" in
  hard_required|soft_optional|unsupported) ;;
  *)
    echo "error: unsupported CARRIER_LIVE_NATIVE_MEDIA_MODE: ${NATIVE_MEDIA_MODE}" >&2
    exit 2
    ;;
esac

if [[ -z "$REQUIRE_TRANSCRIPTION" ]]; then
  if [[ "$TRANSCRIPTION_MODE" == "hard_required" ]]; then
    REQUIRE_TRANSCRIPTION="1"
  else
    REQUIRE_TRANSCRIPTION="0"
  fi
fi

if [[ -z "$REQUIRE_NATIVE_MEDIA" ]]; then
  if [[ "$NATIVE_MEDIA_MODE" == "hard_required" ]]; then
    REQUIRE_NATIVE_MEDIA="1"
  else
    REQUIRE_NATIVE_MEDIA="0"
  fi
fi

if [[ -z "$API_KEY" ]]; then
  echo "error: CARRIER_LIVE_API_KEY is required" >&2
  exit 2
fi

if [[ -z "$MODEL" ]]; then
  MODEL="$(provider_default_model "$PROVIDER")"
fi

PROVIDER_ENV_VAR="$(provider_env_var "$PROVIDER")"
REQUESTED_DAEMON_PORT="$DAEMON_PORT"
REQUESTED_GATEWAY_PORT="$GATEWAY_PORT"

require_cmd go
require_cmd jq
require_cmd curl
require_cmd unzip
require_cmd python3

if [[ -n "$STATE_ROOT" ]]; then
  mkdir -p "$STATE_ROOT" "$STATE_ROOT/runs"
  TMP_DIR="$(mktemp -d "$STATE_ROOT/runs/clp.XXXXXX")"
  if [[ -d "$STATE_ROOT/home" ]]; then
    HOME_DIR="$STATE_ROOT/home"
  elif [[ -d "$STATE_ROOT/h" ]]; then
    HOME_DIR="$STATE_ROOT/h"
  else
    HOME_DIR="$STATE_ROOT/home"
  fi
else
  TMP_DIR="$(mktemp -d "/tmp/clp.XXXXXX")"
  HOME_DIR="${TMP_DIR}/h"
fi
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
LAUNCHER_JSON="${TMP_DIR}/agent-launcher.json"
LAUNCHER_CRON_JSON="${TMP_DIR}/agent-launcher-after-cron.json"
HEARTBEAT_JSON="${TMP_DIR}/agent-heartbeat.json"
AGENT_RUN_JSON="${TMP_DIR}/agent-run.json"
AGENT_TRANSCRIPTION_JSON="${TMP_DIR}/agent-transcription.json"
AGENT_MEDIA_SPEAK_JSON="${TMP_DIR}/agent-media-speak.json"
AGENT_CRON_SCHEDULE_JSON="${TMP_DIR}/agent-cron-schedule.json"
AGENT_CRON_LIST_JSON="${TMP_DIR}/agent-cron-list.json"
AGENT_CRON_CANCEL_JSON="${TMP_DIR}/agent-cron-cancel.json"
RERUN_JSON="${TMP_DIR}/rerun.json"
CLONE_JSON="${TMP_DIR}/clone.json"
EVIDENCE_ZIP="${TMP_DIR}/execution-evidence.zip"
TRANSCRIPTION_AUDIO_FIXTURE="${ROOT_DIR}/scripts/testdata/transcription-smoke.wav"

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
export CARRIER_GATEWAY_HOST="127.0.0.1"
export CARRIER_SERVER_PORT=""
export CARRIER_GATEWAY_PORT=""
export CARRIER_DAEMON_BASE_URL=""
export CARRIER_SERVER_API_TOKEN=""
export CARRIER_GATEWAY_API_TOKEN=""
export CARRIER_TRANSCRIPTION_PROVIDER="${PROVIDER}"
if [[ -n "$TRANSCRIPTION_MODEL" ]]; then
  export CARRIER_TRANSCRIPTION_MODEL="${TRANSCRIPTION_MODEL}"
fi
export "${PROVIDER_ENV_VAR}=${API_KEY}"

select_control_plane_ports() {
  if [[ -n "$REQUESTED_DAEMON_PORT" ]]; then
    if ! port_is_available "$REQUESTED_DAEMON_PORT"; then
      echo "error: requested daemon port is already in use: ${REQUESTED_DAEMON_PORT}" >&2
      return 1
    fi
    DAEMON_PORT="$REQUESTED_DAEMON_PORT"
  else
    DAEMON_PORT="$(pick_ephemeral_port)"
  fi

  if [[ -n "$REQUESTED_GATEWAY_PORT" ]]; then
    if ! port_is_available "$REQUESTED_GATEWAY_PORT"; then
      echo "error: requested gateway port is already in use: ${REQUESTED_GATEWAY_PORT}" >&2
      return 1
    fi
    GATEWAY_PORT="$REQUESTED_GATEWAY_PORT"
  else
    while :; do
      GATEWAY_PORT="$(pick_ephemeral_port)"
      if [[ "$GATEWAY_PORT" != "$DAEMON_PORT" ]]; then
        break
      fi
    done
  fi

  export CARRIER_SERVER_PORT="${DAEMON_PORT}"
  export CARRIER_GATEWAY_PORT="${GATEWAY_PORT}"
  export CARRIER_DAEMON_BASE_URL="http://127.0.0.1:${DAEMON_PORT}"
}

start_control_plane() {
  local max_attempts="${1:-6}"
  local attempt=""
  : >"$DAEMON_LOG"
  : >"$GATEWAY_LOG"
  for attempt in $(seq 1 "$max_attempts"); do
    select_control_plane_ports || return 1
    printf '[start-attempt %s] daemon_port=%s gateway_port=%s\n' "$attempt" "$DAEMON_PORT" "$GATEWAY_PORT" | tee -a "$DAEMON_LOG" "$GATEWAY_LOG" >/dev/null
    "$BIN_PATH" daemon >>"$DAEMON_LOG" 2>&1 &
    DAEMON_PID=$!
    "$BIN_PATH" gateway >>"$GATEWAY_LOG" 2>&1 &
    GATEWAY_PID=$!

    if wait_for_control_plane_ready \
      "http://127.0.0.1:${DAEMON_PORT}/readyz" \
      "http://127.0.0.1:${GATEWAY_PORT}/healthz" \
      30; then
      if control_plane_processes_alive; then
        return 0
      fi
    fi

    if [[ -n "$REQUESTED_DAEMON_PORT" || -n "$REQUESTED_GATEWAY_PORT" ]]; then
      break
    fi

    printf '[start-attempt %s] control plane startup failed, retrying with fresh ports\n' "$attempt" | tee -a "$DAEMON_LOG" "$GATEWAY_LOG" >/dev/null
    stop_control_plane_processes
  done
  stop_control_plane_processes
  return 1
}

cleanup() {
  set +e
  if [[ -n "${BIN_PATH:-}" ]]; then
    if [[ -n "${ZERO_INSTANCE_ID:-}" ]]; then
      run_with_timeout 10 "$BIN_PATH" stop "$ZERO_INSTANCE_ID" >/dev/null 2>&1 || true
    fi
    run_with_timeout 10 "$BIN_PATH" stop zeroclaw >/dev/null 2>&1 || true
  fi
  stop_control_plane_processes
  echo "[artifacts] temp_dir=${TMP_DIR}"
  if [[ -n "$STATE_ROOT" ]]; then
    echo "[artifacts] state_dir=${STATE_ROOT}"
    echo "[artifacts] home_dir=${HOME_DIR}"
  fi
}
trap cleanup EXIT

echo "[1/10] build carrier binary"
(
  cd "$ROOT_DIR"
  go build -o "$BIN_PATH" ./cmd/carrier
)

echo "[2/10] onboard live provider (${PROVIDER}) in WebUI-only mode"
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

if [[ ! -f "$TRANSCRIPTION_AUDIO_FIXTURE" ]]; then
  echo "error: missing transcription audio fixture: ${TRANSCRIPTION_AUDIO_FIXTURE}" >&2
  exit 1
fi

echo "[3/10] start daemon + gateway"
if ! start_control_plane 6; then
  echo "error: failed to start daemon + gateway after retrying candidate ports" >&2
  tail -n 80 "$DAEMON_LOG" >&2 || true
  tail -n 80 "$GATEWAY_LOG" >&2 || true
  exit 1
fi
echo "  daemon_port=${DAEMON_PORT} gateway_port=${GATEWAY_PORT}"

"$BIN_PATH" stop zeroclaw >/dev/null 2>&1 || true

echo "[4/10] install local zeroclaw with isolation"
printf '\n\n' | "$BIN_PATH" add zeroclaw --isolation >"$ADD_LOG" 2>&1

ZERO_INSTANCE_ID="$(jq -r '.instances | map(select((.agent_id // .agentId // .agentID // .type // "") == "zeroclaw")) | first | .id // empty' "$CARRIER_INSTANCE_STORE")"
if [[ -z "$ZERO_INSTANCE_ID" ]]; then
  echo "error: failed to resolve zeroclaw instance id from ${CARRIER_INSTANCE_STORE}" >&2
  cat "$ADD_LOG" >&2 || true
  exit 1
fi
ZERO_GATEWAY_PORT="$(jq -r --arg instance_id "$ZERO_INSTANCE_ID" '.instances | map(select((.id // "") == $instance_id)) | first | (.port // empty)' "$CARRIER_INSTANCE_STORE")"
if [[ -n "$ZERO_GATEWAY_PORT" && "$ZERO_GATEWAY_PORT" != "null" ]]; then
  wait_for_http_ok "http://127.0.0.1:${ZERO_GATEWAY_PORT}/health" "zeroclaw gateway" 60
fi

echo "[5/10] validate standalone managed-agent launcher/run/heartbeat/cron surfaces"
capture_json_output "$LAUNCHER_JSON" "$BIN_PATH" agent launcher zeroclaw --json
jq -e --arg provider "$PROVIDER" '
  (.agentId == "zeroclaw") and
  (.providerReadiness.provider == $provider) and
  (.providerReadiness.ready == true) and
  (.session.instanceId | type == "string" and length > 0)
' "$LAUNCHER_JSON" >/dev/null

capture_json_output "$HEARTBEAT_JSON" "$BIN_PATH" agent heartbeat zeroclaw --json
jq -e '
  (.agentId == "zeroclaw") and
  (.heartbeat.state | type == "string" and length > 0)
' "$HEARTBEAT_JSON" >/dev/null

capture_json_output "$AGENT_RUN_JSON" "$BIN_PATH" agent run zeroclaw -m "Reply with exactly the token live-agent-ok." --provider "$PROVIDER" --json
jq -e '
  (.agentId == "zeroclaw") and
  (.sessionId | type == "string" and length > 0) and
  (.message | type == "string" and length > 0)
' "$AGENT_RUN_JSON" >/dev/null
if ! jq -er '.message // ""' "$AGENT_RUN_JSON" | grep -qi 'live-agent-ok'; then
  echo "error: managed-agent one-shot run did not return the expected token" >&2
  cat "$AGENT_RUN_JSON" >&2
  exit 1
fi

AGENT_CRON_NEXT_RUN_AT="$(python3 - <<'PY'
import datetime
print((datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(minutes=30)).replace(microsecond=0).isoformat().replace("+00:00", "Z"))
PY
)"
capture_json_output "$AGENT_CRON_SCHEDULE_JSON" "$BIN_PATH" agent cron schedule zeroclaw -m "Check launcher readiness." --provider "$PROVIDER" --session-id live-agent-cron --next-run-at "$AGENT_CRON_NEXT_RUN_AT" --json
AGENT_CRON_JOB_ID="$(json_get "$AGENT_CRON_SCHEDULE_JSON" '.id')"
jq -e '.lastResult == "scheduled"' "$AGENT_CRON_SCHEDULE_JSON" >/dev/null

capture_json_output "$AGENT_CRON_LIST_JSON" "$BIN_PATH" agent cron list zeroclaw --json
jq -e --arg job_id "$AGENT_CRON_JOB_ID" '
  (.jobs | map(select(.id == $job_id and .lastResult == "scheduled")) | length) >= 1
' "$AGENT_CRON_LIST_JSON" >/dev/null

capture_json_output "$LAUNCHER_CRON_JSON" "$BIN_PATH" agent launcher zeroclaw --json
jq -e --arg job_id "$AGENT_CRON_JOB_ID" '
  (.cron.count >= 1) and
  (.cron.jobs | map(select(.id == $job_id)) | length) >= 1
' "$LAUNCHER_CRON_JSON" >/dev/null

capture_json_output "$AGENT_CRON_CANCEL_JSON" "$BIN_PATH" agent cron cancel zeroclaw "$AGENT_CRON_JOB_ID" --json
jq -e '.lastResult == "cancelled"' "$AGENT_CRON_CANCEL_JSON" >/dev/null

echo "[6/10] transcribe a real audio fixture and verify native speech output"
TRANSCRIPTION_STATUS="passed"
TRANSCRIPTION_OUTPUT=""
if [[ "$TRANSCRIPTION_MODE" == "unsupported" ]]; then
  TRANSCRIPTION_STATUS="skipped"
  TRANSCRIPTION_OUTPUT="provider ${PROVIDER} is marked unsupported for live transcription smoke"
  echo "  transcription skipped: ${TRANSCRIPTION_OUTPUT}"
else
  curl -fsS -X POST \
    -H 'Content-Type: application/json' \
    "http://127.0.0.1:${DAEMON_PORT}/api/v1/agents/picoclaw/chat" \
    --data @- >"$AGENT_TRANSCRIPTION_JSON" <<EOF
{
  "provider": "${PROVIDER}",
  "chatId": "live-media-transcription",
  "requestId": "live-media-transcription",
  "message": "Transcribe the attached audio and reply with only the transcription text.",
  "attachments": [
    {
      "kind": "audio",
      "name": "transcription-smoke.wav",
      "path": "${TRANSCRIPTION_AUDIO_FIXTURE}",
      "mediaType": "audio/wav",
      "externalId": "live-audio-smoke"
    }
  ]
}
EOF
  if ! jq -e '(.agentId == "picoclaw") and (.message | type == "string" and length > 0)' "$AGENT_TRANSCRIPTION_JSON" >/dev/null; then
    echo "error: live transcription returned malformed payload" >&2
    cat "$AGENT_TRANSCRIPTION_JSON" >&2
    exit 1
  fi
  TRANSCRIPTION_OUTPUT="$(jq -r '.message // ""' "$AGENT_TRANSCRIPTION_JSON")"
  TRANSCRIPTION_ACTION="$(jq -r '.action // ""' "$AGENT_TRANSCRIPTION_JSON")"
  if [[ -z "$TRANSCRIPTION_OUTPUT" ]]; then
    echo "error: live transcription returned empty output" >&2
    cat "$AGENT_TRANSCRIPTION_JSON" >&2
    exit 1
  fi
  if [[ -n "$TRANSCRIPTION_ACTION" ]]; then
    if transcription_skip_allowed "$TRANSCRIPTION_OUTPUT"; then
      if [[ "$REQUIRE_TRANSCRIPTION" == "1" ]]; then
        echo "error: live transcription is unavailable for the configured provider/model and strict mode is enabled" >&2
        printf '%s\n' "$TRANSCRIPTION_OUTPUT" >&2
        exit 1
      fi
      TRANSCRIPTION_STATUS="skipped"
      echo "  transcription skipped: ${TRANSCRIPTION_OUTPUT}"
    else
      echo "error: live transcription returned unsupported action unexpectedly" >&2
      cat "$AGENT_TRANSCRIPTION_JSON" >&2
      exit 1
    fi
  else
    for expected_token in carrier transcription smoke works; do
      if ! printf '%s' "$TRANSCRIPTION_OUTPUT" | grep -qi "$expected_token"; then
        echo "error: live transcription output missing token: ${expected_token}" >&2
        printf '%s\n' "$TRANSCRIPTION_OUTPUT" >&2
        exit 1
      fi
    done
  fi
fi

NATIVE_MEDIA_STATUS="passed"
NATIVE_MEDIA_OUTPUT=""
if [[ "$NATIVE_MEDIA_MODE" == "unsupported" ]]; then
  NATIVE_MEDIA_STATUS="skipped"
  NATIVE_MEDIA_OUTPUT="provider ${PROVIDER} is marked unsupported for native media smoke"
  echo "  native media skipped: ${NATIVE_MEDIA_OUTPUT}"
else
  capture_json_output "$AGENT_MEDIA_SPEAK_JSON" "$BIN_PATH" agent media speak zeroclaw --text "Carrier native media smoke works." --voice alloy --format mp3 --json
  if ! jq -e '(.agentId == "zeroclaw") and (.message | type == "string" and length > 0)' "$AGENT_MEDIA_SPEAK_JSON" >/dev/null; then
    echo "error: native media speak returned malformed payload" >&2
    cat "$AGENT_MEDIA_SPEAK_JSON" >&2
    exit 1
  fi
  NATIVE_MEDIA_OUTPUT="$(jq -r '.message // ""' "$AGENT_MEDIA_SPEAK_JSON")"
  NATIVE_MEDIA_ACTION="$(jq -r '.action // ""' "$AGENT_MEDIA_SPEAK_JSON")"
  if [[ -n "$NATIVE_MEDIA_ACTION" ]]; then
    if [[ "$NATIVE_MEDIA_ACTION" == "unsupported" && "$REQUIRE_NATIVE_MEDIA" != "1" ]]; then
      NATIVE_MEDIA_STATUS="skipped"
      echo "  native media skipped: ${NATIVE_MEDIA_OUTPUT}"
    else
      echo "error: native media output is unavailable for the configured provider/model" >&2
      cat "$AGENT_MEDIA_SPEAK_JSON" >&2
      exit 1
    fi
  else
    jq -e '
      (.richContent.renderMode == "rich_media") and
      (.richContent.attachments | length >= 1) and
      ((.richContent.attachments | map(select(.kind == "audio" and (.outputRole // "") == "generated")) | length) >= 1) and
      ((.richContent.blocks | map(select((.type == "audio" or .type == "voice") and (.outputRole // "") == "generated")) | length) >= 1)
    ' "$AGENT_MEDIA_SPEAK_JSON" >/dev/null
    NATIVE_MEDIA_PATH="$(jq -r '.richContent.attachments | map(select(.kind == "audio")) | .[0].path // empty' "$AGENT_MEDIA_SPEAK_JSON")"
    if [[ -z "$NATIVE_MEDIA_PATH" || ! -f "$NATIVE_MEDIA_PATH" ]]; then
      echo "error: native media output did not persist an audio attachment on disk" >&2
      cat "$AGENT_MEDIA_SPEAK_JSON" >&2
      exit 1
    fi
  fi
fi

MEDIA_VERIFICATION_STATUS="passed"
if [[ "$TRANSCRIPTION_STATUS" == "skipped" || "$NATIVE_MEDIA_STATUS" == "skipped" ]]; then
  MEDIA_VERIFICATION_STATUS="partial"
fi
if [[ "$TRANSCRIPTION_STATUS" == "skipped" && "$NATIVE_MEDIA_STATUS" == "skipped" ]]; then
  MEDIA_VERIFICATION_STATUS="skipped"
fi

echo "[7/10] attach/distill memory against the real instance"
"$BIN_PATH" memory attach --instance "$ZERO_INSTANCE_ID" --scope shared:incident-response >/dev/null
"$BIN_PATH" memory attach --instance "$ZERO_INSTANCE_ID" --scope shared:service-catalog >/dev/null
capture_json_output "$DISTILL_JSON" "$BIN_PATH" memory distill --instance "$ZERO_INSTANCE_ID" --dry-run --json
json_get "$DISTILL_JSON" '.result.runId // .run.runId // empty' >/dev/null

echo "[8/10] create and authorize a real execution"
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

FINAL_OUTPUT="$(jq -r '.execution.results[0].output // ""' "$EXEC_STATUS_JSON")"
if [[ -z "$FINAL_OUTPUT" ]]; then
  echo "error: live provider execution returned empty output" >&2
  cat "$EXEC_STATUS_JSON" >&2
  exit 1
fi
if printf '%s' "$FINAL_OUTPUT" | grep -Eiq "<tool_call>|</tool_call>|<tool_name>|\`\`\`tool_call|\`\`\`"; then
  echo "error: live provider execution returned raw tool-call output instead of a final answer" >&2
  printf '%s\n' "$FINAL_OUTPUT" >&2
  exit 1
fi
if ! printf '%s' "$FINAL_OUTPUT" | grep -Eq '[.!?]$'; then
  echo "error: live provider execution output is not a finalized sentence" >&2
  printf '%s\n' "$FINAL_OUTPUT" >&2
  exit 1
fi

echo "[9/10] export evidence, audit, metrics, and derived executions"
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

ZIP_ENTRIES="$(unzip -Z1 "$EVIDENCE_ZIP")"
printf '%s\n' "$ZIP_ENTRIES" | grep -Fxq 'provider-attribution.json'
printf '%s\n' "$ZIP_ENTRIES" | grep -Fxq 'plan.json'
printf '%s\n' "$ZIP_ENTRIES" | grep -Fxq 'audit.json'

echo "[10/10] summarize"
echo "execution_id=${EXECUTION_ID}"
echo "provider=${PROVIDER}"
echo "model=${MODEL}"
echo "transcription_mode=${TRANSCRIPTION_MODE}"
echo "native_media_mode=${NATIVE_MEDIA_MODE}"
echo "media_verification_status=${MEDIA_VERIFICATION_STATUS}"
echo "instance_id=${ZERO_INSTANCE_ID}"
echo "agent_cron_job_id=${AGENT_CRON_JOB_ID}"
echo "transcription_status=${TRANSCRIPTION_STATUS}"
echo "transcription_output=${TRANSCRIPTION_OUTPUT}"
echo "native_media_status=${NATIVE_MEDIA_STATUS}"
echo "native_media_output=${NATIVE_MEDIA_OUTPUT}"
echo "output=${FINAL_OUTPUT}"
echo "evidence_zip=${EVIDENCE_ZIP}"
echo "live provider control-plane smoke passed"
