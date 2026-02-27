#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/remote-openclaw-install.sh \
    --host-id <id> \
    --host <ip-or-domain> \
    --port <port> \
    --user <ssh-user> \
    --key-path <private-key-path> \
    [--name <display-name>] \
    [--agent-id <agent-id>] \
    [--gateway-url <url>] \
    [--runtime-mode <on_demand|managed_gateway>] \
    [--sync-channel <telegram|discord|feishu>]... \
    [--sync-provider <provider-id>]... \
    [--telegram-allow-from <id>]... \
    [--discord-allow-from <id>]... \
    [--config-path <path>] \
    [--credential-store <path>] \
    [--check-retries <n>] \
    [--check-retry-delay <seconds>] \
    [--skip-reconnect-check]

Environment:
  CARRIER_GATEWAY_TOKEN   Optional bearer token for gateway API.
  CARRIER_CONFIG          Optional local config path (default: ~/.carrier/config.v2.json).
  CARRIER_CREDENTIAL_STORE Optional local credential store path (default: ~/.carrier/credentials.json).

Examples:
  scripts/remote-openclaw-install.sh \
    --host-id vps-1 \
    --host 127.0.0.1 \
    --port 2224 \
    --user carrier \
    --key-path /tmp/carrier-e2e-keys/id_ed25519

Notes:
  - This script is deterministic and does not involve LLM decision-making.
  - It calls gateway remote APIs in a fixed sequence:
    upsert host -> check -> install(stream) -> check -> list instances -> reconnect check.
  - Optional sync selections read local Carrier config/credentials and patch remote openclaw.json.
USAGE
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "error: required command not found: $cmd" >&2
    exit 1
  fi
}

HOST_ID=""
HOST_NAME=""
HOST_ADDR=""
PORT=""
SSH_USER=""
KEY_PATH=""
AGENT_ID="main"
GATEWAY_URL="${CARRIER_GATEWAY_URL:-http://127.0.0.1:8787}"
RUNTIME_MODE="on_demand"
SKIP_RECONNECT_CHECK=0
CHECK_RETRIES=10
CHECK_RETRY_DELAY=2
CONFIG_PATH="${CARRIER_CONFIG:-}"
CREDENTIAL_STORE_PATH="${CARRIER_CREDENTIAL_STORE:-}"
SYNC_CHANNELS=()
SYNC_PROVIDERS=()
TELEGRAM_ALLOW_FROM=()
DISCORD_ALLOW_FROM=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --host-id)
      HOST_ID="${2:-}"
      shift 2
      ;;
    --name)
      HOST_NAME="${2:-}"
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
    --runtime-mode)
      RUNTIME_MODE="${2:-}"
      shift 2
      ;;
    --sync-channel)
      SYNC_CHANNELS+=("${2:-}")
      shift 2
      ;;
    --sync-provider)
      SYNC_PROVIDERS+=("${2:-}")
      shift 2
      ;;
    --telegram-allow-from)
      TELEGRAM_ALLOW_FROM+=("${2:-}")
      shift 2
      ;;
    --discord-allow-from)
      DISCORD_ALLOW_FROM+=("${2:-}")
      shift 2
      ;;
    --config-path)
      CONFIG_PATH="${2:-}"
      shift 2
      ;;
    --credential-store)
      CREDENTIAL_STORE_PATH="${2:-}"
      shift 2
      ;;
    --check-retries)
      CHECK_RETRIES="${2:-}"
      shift 2
      ;;
    --check-retry-delay)
      CHECK_RETRY_DELAY="${2:-}"
      shift 2
      ;;
    --skip-reconnect-check)
      SKIP_RECONNECT_CHECK=1
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
if [[ ! "$CHECK_RETRIES" =~ ^[0-9]+$ ]]; then
  echo "error: --check-retries must be numeric" >&2
  exit 1
fi
if [[ ! "$CHECK_RETRY_DELAY" =~ ^[0-9]+$ ]]; then
  echo "error: --check-retry-delay must be numeric" >&2
  exit 1
fi
if [[ "$RUNTIME_MODE" != "on_demand" && "$RUNTIME_MODE" != "managed_gateway" ]]; then
  echo "error: runtime-mode must be on_demand or managed_gateway" >&2
  exit 1
fi
if [[ -z "$HOST_NAME" ]]; then
  HOST_NAME="$HOST_ID"
fi
if [[ -z "$CONFIG_PATH" ]]; then
  CONFIG_PATH="$HOME/.carrier/config.v2.json"
fi
if [[ -z "$CREDENTIAL_STORE_PATH" ]]; then
  CREDENTIAL_STORE_PATH="$HOME/.carrier/credentials.json"
fi

for ch in "${SYNC_CHANNELS[@]}"; do
  case "$(printf '%s' "$ch" | tr '[:upper:]' '[:lower:]')" in
    telegram|discord|feishu) ;;
    "")
      echo "error: --sync-channel cannot be empty" >&2
      exit 1
      ;;
    *)
      echo "error: unsupported --sync-channel value: $ch" >&2
      exit 1
      ;;
  esac
done
for p in "${SYNC_PROVIDERS[@]}"; do
  if [[ -z "$(printf '%s' "$p" | tr -d '[:space:]')" ]]; then
    echo "error: --sync-provider cannot be empty" >&2
    exit 1
  fi
done
if (( ${#SYNC_CHANNELS[@]} > 0 || ${#SYNC_PROVIDERS[@]} > 0 )); then
  if [[ ! -f "$CONFIG_PATH" ]]; then
    echo "error: local config not found for sync: $CONFIG_PATH" >&2
    exit 1
  fi
fi

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

AUTH_HEADER=()
if [[ -n "${CARRIER_GATEWAY_TOKEN:-}" ]]; then
  AUTH_HEADER=(-H "Authorization: Bearer ${CARRIER_GATEWAY_TOKEN}")
fi

api_json() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local out_file="$4"
  local status
  if [[ -n "$body" ]]; then
    status="$(curl -sS -X "$method" \
      -H "Content-Type: application/json" \
      "${AUTH_HEADER[@]}" \
      "$GATEWAY_URL$path" \
      --data "$body" \
      -o "$out_file" \
      -w "%{http_code}")"
  else
    status="$(curl -sS -X "$method" \
      -H "Content-Type: application/json" \
      "${AUTH_HEADER[@]}" \
      "$GATEWAY_URL$path" \
      -o "$out_file" \
      -w "%{http_code}")"
  fi
  printf "%s" "$status"
}

api_sse() {
  local method="$1"
  local path="$2"
  local body="$3"
  local out_file="$4"
  local status
  status="$(curl -sS -N -X "$method" \
    -H "Content-Type: application/json" \
    "${AUTH_HEADER[@]}" \
    "$GATEWAY_URL$path" \
    --data "$body" \
    -o "$out_file" \
    -w "%{http_code}")"
  printf "%s" "$status"
}

expect_2xx() {
  local code="$1"
  local name="$2"
  local out_file="$3"
  if [[ ! "$code" =~ ^2 ]]; then
    echo "[$name] failed: HTTP $code" >&2
    sed -n '1,220p' "$out_file" >&2 || true
    exit 1
  fi
}

check_host_with_retry() {
  local label="$1"
  local out_file="$2"
  local attempt=1
  local code

  while :; do
    code="$(api_json POST "/api/v1/remote/hosts/$HOST_ID/check" '{}' "$out_file")"
    if [[ "$code" =~ ^2 ]]; then
      return 0
    fi
    if (( attempt > CHECK_RETRIES )); then
      echo "[$label] failed after ${CHECK_RETRIES} retries: HTTP $code" >&2
      sed -n '1,220p' "$out_file" >&2 || true
      return 1
    fi
    echo "  [$label] retry $attempt/$CHECK_RETRIES (HTTP $code), waiting ${CHECK_RETRY_DELAY}s..."
    sleep "$CHECK_RETRY_DELAY"
    attempt=$((attempt + 1))
  done
}

map_provider_to_managed() {
  local provider_id
  provider_id="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')"
  case "$provider_id" in
    openai-codex|openai-compatible|vllm|openai-v1)
      printf '%s' "openai"
      ;;
    *)
      printf '%s' "$provider_id"
      ;;
  esac
}

load_local_credential() {
  local provider_id="$1"
  local token=""

  if [[ "$(uname -s)" == "Darwin" ]] && [[ "${CARRIER_DISABLE_KEYCHAIN:-}" != "1" ]] && command -v security >/dev/null 2>&1; then
    if token="$(security find-generic-password -a carrier -s "carrier.provider.${provider_id}" -w 2>/dev/null)"; then
      printf '%s' "$(printf '%s' "$token" | tr -d '\r' | sed 's/[[:space:]]*$//')"
      return 0
    fi
  fi

  if [[ -f "$CREDENTIAL_STORE_PATH" ]]; then
    token="$(jq -r --arg id "$provider_id" '.providers[$id] // ""' "$CREDENTIAL_STORE_PATH" 2>/dev/null || true)"
    printf '%s' "$(printf '%s' "$token" | tr -d '\r' | sed 's/[[:space:]]*$//')"
    return 0
  fi

  printf '%s' ""
}

merge_patch_file() {
  local patch_file="$1"
  local patch_json="$2"
  jq -s '.[0] * .[1]' "$patch_file" <(printf '%s' "$patch_json") > "${patch_file}.tmp"
  mv "${patch_file}.tmp" "$patch_file"
}

sync_selected_local_config() {
  local patch_file="$1"
  local sync_response_file="$2"

  if (( ${#SYNC_CHANNELS[@]} == 0 && ${#SYNC_PROVIDERS[@]} == 0 )); then
    return 0
  fi

  echo "{}" > "$patch_file"

  local ch_lower channel_json ch_token ch_secret allow_json channel_patch
  for ch in "${SYNC_CHANNELS[@]}"; do
    ch_lower="$(printf '%s' "$ch" | tr '[:upper:]' '[:lower:]')"
    channel_json="$(jq -c --arg id "$ch_lower" '[.channels[]? | select((.id|ascii_downcase)==($id|ascii_downcase))] | first // empty' "$CONFIG_PATH")"
    if [[ -z "$channel_json" || "$channel_json" == "null" ]]; then
      echo "[config sync] local channel '$ch_lower' not found in $CONFIG_PATH" >&2
      return 1
    fi
    ch_token="$(jq -r '.bot_token // ""' <<<"$channel_json")"
    ch_secret="$(jq -r '.webhook_secret // ""' <<<"$channel_json")"
    if [[ -z "$ch_token" ]]; then
      echo "[config sync] local channel '$ch_lower' has empty bot_token" >&2
      return 1
    fi

    allow_json="[]"
    case "$ch_lower" in
      telegram)
        if (( ${#TELEGRAM_ALLOW_FROM[@]} > 0 )); then
          allow_json="$(printf '%s\n' "${TELEGRAM_ALLOW_FROM[@]}" | jq -R . | jq -s .)"
        fi
        ;;
      discord)
        if (( ${#DISCORD_ALLOW_FROM[@]} > 0 )); then
          allow_json="$(printf '%s\n' "${DISCORD_ALLOW_FROM[@]}" | jq -R . | jq -s .)"
        fi
        ;;
    esac
    if [[ "$allow_json" == "[]" ]]; then
      allow_json="$(jq -c '.allow_from // []' <<<"$channel_json")"
    fi

    channel_patch="$(jq -n \
      --arg channel "$ch_lower" \
      --arg token "$ch_token" \
      --arg secret "$ch_secret" \
      --argjson allowFrom "$allow_json" \
      '{
        channels: {
          ($channel): {
            enabled: true,
            token: $token
          }
        }
      }
      | if ($secret|length) > 0 then .channels[$channel].webhook_secret = $secret else . end
      | if ($allowFrom|length) > 0 then .channels[$channel].allow_from = $allowFrom else . end')"
    merge_patch_file "$patch_file" "$channel_patch"
  done

  local provider_id model_json model_raw model_name_raw provider_key defaults_model credential_ref auth_mode provider_patch token
  for provider in "${SYNC_PROVIDERS[@]}"; do
    provider_id="$(printf '%s' "$provider" | tr '[:upper:]' '[:lower:]')"
    model_json="$(jq -c --arg pid "$provider_id" '[.model_list[]? | select((.provider_id|ascii_downcase)==($pid|ascii_downcase))] | first // empty' "$CONFIG_PATH")"
    if [[ -z "$model_json" || "$model_json" == "null" ]]; then
      echo "[config sync] local provider '$provider_id' not found in model_list of $CONFIG_PATH" >&2
      return 1
    fi

    model_raw="$(jq -r '.model // ""' <<<"$model_json")"
    model_name_raw="$(jq -r '.model_name // ""' <<<"$model_json")"
    credential_ref="$(jq -r '.credential_ref // .provider_id // ""' <<<"$model_json" | tr '[:upper:]' '[:lower:]')"
    auth_mode="$(jq -r '.auth_mode // ""' <<<"$model_json" | tr '[:upper:]' '[:lower:]')"

    provider_key="$provider_id"
    if [[ "$model_raw" == */* ]]; then
      provider_key="${model_raw%%/*}"
    fi
    provider_key="$(map_provider_to_managed "$provider_key")"

    defaults_model="$model_name_raw"
    if [[ -z "$defaults_model" ]]; then
      if [[ "$model_raw" == */* ]]; then
        defaults_model="${model_raw##*/}"
      else
        defaults_model="$model_raw"
      fi
    fi
    if [[ -z "$defaults_model" ]]; then
      echo "[config sync] provider '$provider_id' has empty model in local config" >&2
      return 1
    fi

    token=""
    if [[ -n "$credential_ref" ]]; then
      token="$(load_local_credential "$credential_ref")"
    fi
    if [[ -z "$token" && "$auth_mode" != "none" && "$provider_id" != "openai-codex" ]]; then
      echo "[config sync] missing credential for provider '$provider_id' (ref: '$credential_ref')" >&2
      return 1
    fi

    provider_patch="$(jq -n \
      --arg providerKey "$provider_key" \
      --arg providerID "$provider_id" \
      --arg modelName "$defaults_model" \
      --arg modelRaw "$model_raw" \
      --arg credentialRef "$credential_ref" \
      --arg token "$token" \
      '{
        agents: {
          defaults: {
            provider: $providerKey,
            model: $modelName
          }
        },
        model_list: [
          {
            model_name: $modelName,
            model: (if ($modelRaw|length)>0 then $modelRaw else $modelName end)
          }
        ],
        providers: {
          ($providerKey): {
            credential_ref: (if ($credentialRef|length)>0 then $credentialRef else $providerID end)
          }
        }
      }
      | if ($providerID == "openai-codex") then
          .model_list[0].auth_method = "oauth"
          | .providers[$providerKey].auth_method = "oauth"
        else .
        end
      | if ($token|length) > 0 and ($providerID != "openai-codex") then .providers[$providerKey].api_key = $token else . end')"
    merge_patch_file "$patch_file" "$provider_patch"
  done

  local payload code
  payload="$(jq -c '{patch:.}' "$patch_file")"
  code="$(api_json PATCH "/api/v1/remote/hosts/$HOST_ID/config" "$payload" "$sync_response_file")"
  expect_2xx "$code" "sync selected local config" "$sync_response_file"
}

echo "[1/8] gateway health check"
health_file="$TMP_DIR/health.json"
health_code="$(api_json GET "/healthz" "" "$health_file")"
expect_2xx "$health_code" "healthz" "$health_file"

host_payload="$(
  jq -n \
    --arg id "$HOST_ID" \
    --arg name "$HOST_NAME" \
    --arg host "$HOST_ADDR" \
    --argjson port "$PORT" \
    --arg user "$SSH_USER" \
    --arg keyPath "$KEY_PATH" \
    --arg runtimeMode "$RUNTIME_MODE" \
    '{
      id: $id,
      name: $name,
      host: $host,
      port: $port,
      user: $user,
      authMode: "private_key",
      keyPath: $keyPath,
      runtimeMode: $runtimeMode
    }'
)"

echo "[2/8] upsert remote host: $HOST_ID"
upsert_file="$TMP_DIR/upsert.json"
upsert_code="$(api_json POST "/api/v1/remote/hosts" "$host_payload" "$upsert_file")"
expect_2xx "$upsert_code" "upsert host" "$upsert_file"

echo "[3/8] pre-check host"
precheck_file="$TMP_DIR/precheck.json"
check_host_with_retry "pre-check host" "$precheck_file"
pre_openclaw="$(jq -r '.check.openclawFound // false' "$precheck_file" 2>/dev/null || echo false)"
pre_ssh="$(jq -r '.check.sshOk // false' "$precheck_file" 2>/dev/null || echo false)"
echo "  sshOk=$pre_ssh openclawFound=$pre_openclaw"

echo "[4/8] install openclaw (stream)"
install_sse_file="$TMP_DIR/install.sse"
install_code="$(api_sse POST "/api/v1/remote/hosts/$HOST_ID/instances/$AGENT_ID/install/stream" '{}' "$install_sse_file")"
expect_2xx "$install_code" "install stream" "$install_sse_file"
install_ok="$(grep '^data:' "$install_sse_file" | sed 's/^data: //' | jq -r 'select(.type=="result") | .install.installed' | tail -n 1)"
if [[ -z "$install_ok" ]]; then
  install_ok="false"
fi
echo "  installed=$install_ok"
if [[ "$install_ok" != "true" ]]; then
  echo "[install stream] install did not complete successfully" >&2
  sed -n '1,260p' "$install_sse_file" >&2 || true
  exit 1
fi

if (( ${#SYNC_CHANNELS[@]} > 0 || ${#SYNC_PROVIDERS[@]} > 0 )); then
  echo "[4.5/8] sync selected local config to remote openclaw.json"
  sync_patch_file="$TMP_DIR/config-sync-patch.json"
  sync_result_file="$TMP_DIR/config-sync-result.json"
  sync_selected_local_config "$sync_patch_file" "$sync_result_file"
  synced_channels="$(printf '%s\n' "${SYNC_CHANNELS[@]:-}" | paste -sd ',' -)"
  synced_providers="$(printf '%s\n' "${SYNC_PROVIDERS[@]:-}" | paste -sd ',' -)"
  if [[ -z "$synced_channels" ]]; then
    synced_channels="<none>"
  fi
  if [[ -z "$synced_providers" ]]; then
    synced_providers="<none>"
  fi
  echo "  synced.channels=$synced_channels synced.providers=$synced_providers"
fi

echo "[5/8] post-check host"
postcheck_file="$TMP_DIR/postcheck.json"
check_host_with_retry "post-check host" "$postcheck_file"
post_openclaw="$(jq -r '.check.openclawFound // false' "$postcheck_file" 2>/dev/null || echo false)"
post_ssh="$(jq -r '.check.sshOk // false' "$postcheck_file" 2>/dev/null || echo false)"
echo "  sshOk=$post_ssh openclawFound=$post_openclaw"
if [[ "$post_openclaw" != "true" ]]; then
  echo "[post-check host] openclawFound is false" >&2
  sed -n '1,220p' "$postcheck_file" >&2 || true
  exit 1
fi

echo "[6/8] list instances"
instances_file="$TMP_DIR/instances.json"
instances_code="$(api_json GET "/api/v1/remote/hosts/$HOST_ID/instances" "" "$instances_file")"
expect_2xx "$instances_code" "list instances" "$instances_file"
instance_ids="$(jq -r '.instances[]?.id' "$instances_file" | paste -sd ',' -)"
if [[ -z "$instance_ids" ]]; then
  echo "  instances=<none>"
else
  echo "  instances=$instance_ids"
fi

if [[ "$SKIP_RECONNECT_CHECK" -eq 0 ]]; then
  echo "[7/8] reconnect simulation (delete + re-upsert)"
  delete_file="$TMP_DIR/delete.json"
  delete_code="$(api_json DELETE "/api/v1/remote/hosts/$HOST_ID" "" "$delete_file")"
  expect_2xx "$delete_code" "delete host" "$delete_file"
  reupsert_file="$TMP_DIR/reupsert.json"
  reupsert_code="$(api_json POST "/api/v1/remote/hosts" "$host_payload" "$reupsert_file")"
  expect_2xx "$reupsert_code" "re-upsert host" "$reupsert_file"

  echo "[8/8] reconnect check + list"
  reconnect_check_file="$TMP_DIR/reconnect-check.json"
  check_host_with_retry "reconnect check" "$reconnect_check_file"
  reconnect_openclaw="$(jq -r '.check.openclawFound // false' "$reconnect_check_file" 2>/dev/null || echo false)"
  reconnect_instances_file="$TMP_DIR/reconnect-instances.json"
  reconnect_instances_code="$(api_json GET "/api/v1/remote/hosts/$HOST_ID/instances" "" "$reconnect_instances_file")"
  expect_2xx "$reconnect_instances_code" "reconnect list instances" "$reconnect_instances_file"
  reconnect_instance_ids="$(jq -r '.instances[]?.id' "$reconnect_instances_file" | paste -sd ',' -)"
  if [[ "$reconnect_openclaw" != "true" ]]; then
    echo "[reconnect check] openclawFound is false" >&2
    sed -n '1,220p' "$reconnect_check_file" >&2 || true
    exit 1
  fi
  echo "  reconnect.openclawFound=$reconnect_openclaw"
  if [[ -z "$reconnect_instance_ids" ]]; then
    echo "  reconnect.instances=<none>"
  else
    echo "  reconnect.instances=$reconnect_instance_ids"
  fi
else
  echo "[7/8] reconnect check skipped"
  echo "[8/8] reconnect check skipped"
fi

echo "done: remote openclaw install flow succeeded for host=$HOST_ID agent=$AGENT_ID"
