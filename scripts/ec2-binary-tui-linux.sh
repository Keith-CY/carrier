#!/usr/bin/env bash
# Download a main-push Carrier binary from GitHub Releases and validate
# TUI onboarding + TUI add(openclaw) on Linux.
#
# Examples:
#   scripts/ec2-binary-tui-linux.sh --sha <full_commit_sha>
#   scripts/ec2-binary-tui-linux.sh --tag main-<full_commit_sha>
#   scripts/ec2-binary-tui-linux.sh --main
#   scripts/ec2-binary-tui-linux.sh
#
# Optional non-interactive inputs:
#   CARRIER_TELEGRAM_BOT_TOKEN   Telegram bot token used by TUI prompts.
#   CARRIER_PROVIDER_OVERRIDE    Provider ID override (default: keep auto-selected openai-codex).
#   CARRIER_PROVIDER_SECRET      Provider credential input when override needs one.
#
# Notes:
# - Default provider selection is openai-codex; OAuth device-code flow is shown in terminal.
# - If OAuth is needed, complete it in browser while command waits.

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ec2-binary-tui-linux.sh --sha <full_commit_sha> [options]
  ec2-binary-tui-linux.sh --tag <release_tag> [options]
  ec2-binary-tui-linux.sh --main [options]
  ec2-binary-tui-linux.sh [options]

Options:
  --sha <sha>          Full commit SHA. Tag becomes main-<sha>.
  --tag <tag>          Explicit release tag (for example main-<sha>).
  --main               Resolve SHA from repository main HEAD.
  --repo <owner/repo>  GitHub repository (default: Keith-CY/carrier).
  --wait-seconds <n>   Wait up to n seconds for release asset (default: 600 with --main, else 0).
  --label <label>      Asset label (default: linux-x64).
  --out-dir <dir>      Download/extract directory (default: /tmp/carrier-ec2).
  --skip-onboard       Skip `carrier onboard`.
  --skip-add           Skip `carrier add openclaw`.
  -h, --help           Show this help message.

Environment (optional):
  CARRIER_TELEGRAM_BOT_TOKEN
  CARRIER_PROVIDER_OVERRIDE
  CARRIER_PROVIDER_SECRET
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: required command not found: $1" >&2
    exit 1
  fi
}

is_non_negative_integer() {
  [[ "$1" =~ ^[0-9]+$ ]]
}

resolve_main_sha() {
  local repo="$1"
  local api_url="https://api.github.com/repos/${repo}/commits/main"
  local json sha

  json="$(curl -fsSL --retry 3 --retry-delay 2 "$api_url")" || return 1
  sha="$(printf '%s\n' "$json" | sed -nE 's/^[[:space:]]*"sha":[[:space:]]*"([0-9a-f]{40})".*/\1/p' | head -n 1)"
  [[ "$sha" =~ ^[0-9a-f]{40}$ ]] || return 1
  printf '%s\n' "$sha"
}

wait_for_release_asset() {
  local url="$1"
  local timeout="$2"
  local deadline code

  (( timeout > 0 )) || return 0
  deadline=$((SECONDS + timeout))

  while :; do
    code="$(curl -sS -o /dev/null -w '%{http_code}' -L "$url" || true)"
    if [[ "$code" == "200" ]]; then
      echo "[ec2] Release asset is ready: $url"
      return 0
    fi
    if (( SECONDS >= deadline )); then
      echo "ERROR: release asset not ready after ${timeout}s: $url (last HTTP $code)" >&2
      echo "Hint: wait for Release workflow on main push to finish, then retry." >&2
      return 1
    fi
    echo "[ec2] Waiting for release asset (HTTP $code), retrying in 10s..."
    sleep 10
  done
}

TAG=""
SHA=""
LABEL="linux-x64"
OUT_DIR="/tmp/carrier-ec2"
SKIP_ONBOARD=0
SKIP_ADD=0
REPO="Keith-CY/carrier"
USE_MAIN=0
WAIT_SECONDS=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --sha)
      SHA="${2:-}"
      shift 2
      ;;
    --tag)
      TAG="${2:-}"
      shift 2
      ;;
    --main)
      USE_MAIN=1
      shift
      ;;
    --repo)
      REPO="${2:-}"
      shift 2
      ;;
    --wait-seconds)
      WAIT_SECONDS="${2:-}"
      shift 2
      ;;
    --label)
      LABEL="${2:-}"
      shift 2
      ;;
    --out-dir)
      OUT_DIR="${2:-}"
      shift 2
      ;;
    --skip-onboard)
      SKIP_ONBOARD=1
      shift
      ;;
    --skip-add)
      SKIP_ADD=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -n "$TAG" && -n "$SHA" ]]; then
  echo "ERROR: --tag and --sha cannot be used together" >&2
  usage
  exit 1
fi

if [[ -n "$WAIT_SECONDS" ]] && ! is_non_negative_integer "$WAIT_SECONDS"; then
  echo "ERROR: --wait-seconds must be a non-negative integer" >&2
  usage
  exit 1
fi

if [[ -z "$TAG" && -z "$SHA" ]]; then
  USE_MAIN=1
fi

if [[ "$USE_MAIN" -eq 1 && -z "$SHA" && -z "$TAG" ]]; then
  echo "[ec2] Resolving main HEAD SHA from ${REPO}"
  SHA="$(resolve_main_sha "$REPO")" || {
    echo "ERROR: failed to resolve main HEAD SHA from ${REPO}" >&2
    exit 1
  }
  echo "[ec2] Resolved main SHA: ${SHA}"
fi

if [[ -z "$TAG" ]]; then
  if [[ -z "$SHA" ]]; then
    echo "ERROR: provide --sha or --tag (or use --main)" >&2
    usage
    exit 1
  fi
  TAG="main-$SHA"
fi

if [[ -z "$WAIT_SECONDS" ]]; then
  if [[ "$USE_MAIN" -eq 1 ]]; then
    WAIT_SECONDS=600
  else
    WAIT_SECONDS=0
  fi
fi

require_cmd curl
require_cmd unzip
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
  echo "ERROR: sha256sum or shasum is required for checksum verification" >&2
  exit 1
fi

ZIP_NAME="carrier-${TAG}-${LABEL}.zip"
BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"
ZIP_PATH="${OUT_DIR}/${ZIP_NAME}"
SUM_PATH="${OUT_DIR}/${ZIP_NAME}.sha256"
ASSET_URL="${BASE_URL}/${ZIP_NAME}"
SUM_URL="${BASE_URL}/${ZIP_NAME}.sha256"

mkdir -p "$OUT_DIR"

wait_for_release_asset "$ASSET_URL" "$WAIT_SECONDS"

echo "[ec2] Downloading release asset: ${ASSET_URL}"
curl -fL --retry 3 --retry-delay 2 -o "$ZIP_PATH" "$ASSET_URL"

echo "[ec2] Downloading checksum: ${SUM_URL}"
curl -fL --retry 3 --retry-delay 2 -o "$SUM_PATH" "$SUM_URL"

echo "[ec2] Verifying checksum"
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$OUT_DIR" && sha256sum -c "${ZIP_NAME}.sha256")
else
  expected="$(awk '{print $1}' "$SUM_PATH" | tr '[:upper:]' '[:lower:]')"
  actual="$(shasum -a 256 "$ZIP_PATH" | awk '{print $1}' | tr '[:upper:]' '[:lower:]')"
  if [[ "$expected" != "$actual" ]]; then
    echo "ERROR: checksum mismatch for $ZIP_NAME" >&2
    echo "expected=$expected" >&2
    echo "actual=$actual" >&2
    exit 1
  fi
fi

echo "[ec2] Extracting ${ZIP_NAME}"
unzip -o "$ZIP_PATH" -d "$OUT_DIR" >/dev/null

BIN_PATH="${OUT_DIR}/carrier"
if [[ ! -x "$BIN_PATH" ]]; then
  chmod +x "$BIN_PATH" 2>/dev/null || true
fi
if [[ ! -x "$BIN_PATH" ]]; then
  echo "ERROR: extracted binary not found or not executable: $BIN_PATH" >&2
  exit 1
fi

echo "[ec2] Binary ready: $BIN_PATH"
"$BIN_PATH" --help >/dev/null

run_tui() {
  local token="${CARRIER_TELEGRAM_BOT_TOKEN:-}"
  local override="${CARRIER_PROVIDER_OVERRIDE:-}"
  local secret="${CARRIER_PROVIDER_SECRET:-}"

  if [[ -n "$token" ]]; then
    if [[ -n "$override" && "$override" != "openai-codex" && -z "$secret" ]]; then
      echo "ERROR: CARRIER_PROVIDER_SECRET is required when CARRIER_PROVIDER_OVERRIDE=$override" >&2
      exit 1
    fi
    if [[ -n "$override" && "$override" != "openai-codex" ]]; then
      printf '%s\n%s\n%s\n' "$token" "$override" "$secret" | "$BIN_PATH" "$@"
    else
      printf '%s\n%s\n' "$token" "$override" | "$BIN_PATH" "$@"
    fi
    return
  fi

  echo "[ec2] CARRIER_TELEGRAM_BOT_TOKEN not set, running interactively for: carrier $*"
  "$BIN_PATH" "$@"
}

if [[ "$SKIP_ONBOARD" -eq 0 ]]; then
  echo "[ec2] Running: carrier onboard (TUI)"
  run_tui onboard
fi

if [[ "$SKIP_ADD" -eq 0 ]]; then
  echo "[ec2] Running: carrier add openclaw (TUI)"
  run_tui add openclaw
fi

echo "[ec2] Current managed instances:"
"$BIN_PATH" list || true

echo "[ec2] OpenClaw status from daemon API (best effort):"
curl -fsS http://127.0.0.1:9090/api/v1/agents/openclaw/status || true
echo

echo "[ec2] Done."
