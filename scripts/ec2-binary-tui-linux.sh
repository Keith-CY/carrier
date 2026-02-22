#!/usr/bin/env bash
# Download a main-push Carrier binary from GitHub Releases and validate
# TUI onboarding + TUI add(openclaw) on Linux.
#
# Examples:
#   scripts/ec2-binary-tui-linux.sh --sha <full_commit_sha>
#   scripts/ec2-binary-tui-linux.sh --tag main-<full_commit_sha>
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

Options:
  --sha <sha>          Full commit SHA. Tag becomes main-<sha>.
  --tag <tag>          Explicit release tag (for example main-<sha>).
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

TAG=""
SHA=""
LABEL="linux-x64"
OUT_DIR="/tmp/carrier-ec2"
SKIP_ONBOARD=0
SKIP_ADD=0

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

if [[ -z "$TAG" ]]; then
  if [[ -z "$SHA" ]]; then
    echo "ERROR: provide --sha or --tag" >&2
    usage
    exit 1
  fi
  TAG="main-$SHA"
fi

require_cmd curl
require_cmd unzip
if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
  echo "ERROR: sha256sum or shasum is required for checksum verification" >&2
  exit 1
fi

ZIP_NAME="carrier-${TAG}-${LABEL}.zip"
BASE_URL="https://github.com/Keith-CY/carrier/releases/download/${TAG}"
ZIP_PATH="${OUT_DIR}/${ZIP_NAME}"
SUM_PATH="${OUT_DIR}/${ZIP_NAME}.sha256"

mkdir -p "$OUT_DIR"

echo "[ec2] Downloading release asset: ${BASE_URL}/${ZIP_NAME}"
curl -fL --retry 3 --retry-delay 2 -o "$ZIP_PATH" "${BASE_URL}/${ZIP_NAME}"

echo "[ec2] Downloading checksum: ${BASE_URL}/${ZIP_NAME}.sha256"
curl -fL --retry 3 --retry-delay 2 -o "$SUM_PATH" "${BASE_URL}/${ZIP_NAME}.sha256"

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
