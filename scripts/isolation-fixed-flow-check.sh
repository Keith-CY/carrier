#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_LIVE=0
LIVE_ONLY=0

usage() {
  cat <<'EOF'
Usage:
  bash scripts/isolation-fixed-flow-check.sh [--live] [--live-only]

Modes:
  default      Run deterministic fixed-flow verification tests only.
  --live       Run tests, then run live smoke with `carrier add <agent> --isolation`.
  --live-only  Run live smoke only.

Live mode prerequisites:
  1) `carrier` is available in PATH.
  2) Provider credential already saved in Carrier (for auto-reuse).
  3) Telegram tokens are exported:
     - OPENCLAW_TELEGRAM_BOT_TOKEN
     - PICOCLAW_TELEGRAM_BOT_TOKEN
     - ZEROCLAW_TELEGRAM_BOT_TOKEN
EOF
}

for arg in "$@"; do
  case "$arg" in
    --live)
      RUN_LIVE=1
      ;;
    --live-only)
      RUN_LIVE=1
      LIVE_ONLY=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $arg" >&2
      usage >&2
      exit 2
      ;;
  esac
done

run_deterministic_tests() {
  echo "[1/3] daemon: isolation deterministic pipeline tests"
  (
    cd "$ROOT_DIR/daemon"
    go test ./internal/lifecycle -run 'TestInstallWithIsolation|TestStartWithIsolation' -count=1
  )

  echo "[2/3] gateway: remote codex/opencode deterministic installer tests"
  (
    cd "$ROOT_DIR/gateway"
    go test ./... -run 'TestRemoteInstallCodeAgentBinary' -count=1
  )

  echo "[3/3] cmd/carrier: add --isolation payload e2e tests (openclaw/picoclaw/zeroclaw)"
  (
    cd "$ROOT_DIR/cmd/carrier"
    GOTOOLCHAIN=auto go test ./... -run 'TestE2ECarrierBinaryAdd(OpenClawIsolationSendsInstallAndStartIsolationPayload|ManagedAgentsIsolationSendsInstallAndStartIsolationPayload)' -count=1
  )
}

require_live_prerequisites() {
  command -v carrier >/dev/null 2>&1 || {
    echo "carrier binary not found in PATH" >&2
    exit 1
  }
  : "${OPENCLAW_TELEGRAM_BOT_TOKEN:?OPENCLAW_TELEGRAM_BOT_TOKEN is required for --live}"
  : "${PICOCLAW_TELEGRAM_BOT_TOKEN:?PICOCLAW_TELEGRAM_BOT_TOKEN is required for --live}"
  : "${ZEROCLAW_TELEGRAM_BOT_TOKEN:?ZEROCLAW_TELEGRAM_BOT_TOKEN is required for --live}"
}

run_live_add() {
  local agent_id="$1"
  local bot_token="$2"

  echo "[live] carrier add ${agent_id} --isolation"
  CARRIER_TELEGRAM_BOT_TOKEN="" \
    printf '%s\n\n' "$bot_token" | carrier add "$agent_id" --isolation
}

run_live_smoke() {
  require_live_prerequisites
  echo "[live] running fixed-flow smoke for managed agents"
  echo "[live] provider credential must already be saved in Carrier for auto-reuse"

  run_live_add "openclaw" "$OPENCLAW_TELEGRAM_BOT_TOKEN"
  run_live_add "picoclaw" "$PICOCLAW_TELEGRAM_BOT_TOKEN"
  run_live_add "zeroclaw" "$ZEROCLAW_TELEGRAM_BOT_TOKEN"
}

if [[ "$LIVE_ONLY" -eq 0 ]]; then
  run_deterministic_tests
fi

if [[ "$RUN_LIVE" -eq 1 ]]; then
  run_live_smoke
fi

echo "isolation fixed-flow check completed"
