#!/bin/sh
# Carrier installer script with checksum verification.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Keith-CY/carrier/main/scripts/install.sh | bash
#
# Optional environment variables:
#   CARRIER_REPO       - GitHub repo (default: Keith-CY/carrier)
#   CARRIER_TAG        - Release tag (example: main-<full_commit_sha>)
#   CARRIER_SHA        - Full commit SHA (used as main-<sha> when CARRIER_TAG is unset)
#   CARRIER_LABEL      - Asset label (default: auto-detected, e.g. linux-x64)
#   INSTALL_DIR        - Install directory (default: /usr/local/bin)
#   BINARY_NAME        - Installed binary name (default: carrier)
#   VERIFY_CHECKSUM    - 1 to verify .sha256 sidecar, 0 to skip (default: 1)
#   DRY_RUN            - If set, print actions without executing

set -eu

REPO="${CARRIER_REPO:-Keith-CY/carrier}"
TAG="${CARRIER_TAG:-}"
SHA="${CARRIER_SHA:-}"
LABEL="${CARRIER_LABEL:-}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="${BINARY_NAME:-carrier}"
VERIFY_CHECKSUM="${VERIFY_CHECKSUM:-1}"

log() {
    printf '%s\n' "$*"
}

fail() {
    printf 'ERROR: %s\n' "$*" >&2
    exit 1
}

require_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        fail "required command not found: $1"
    fi
}

is_full_sha() {
    printf '%s' "$1" | grep -Eq '^[0-9a-f]{40}$'
}

detect_label() {
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    arch="$(uname -m)"

    case "$arch" in
        x86_64|amd64) arch="x64" ;;
        aarch64|arm64) arch="arm64" ;;
        *) fail "unsupported architecture: $arch (set CARRIER_LABEL to override)" ;;
    esac

    case "$os" in
        linux) printf 'linux-%s\n' "$arch" ;;
        darwin) printf 'darwin-%s\n' "$arch" ;;
        msys*|mingw*|cygwin*) printf 'windows-%s\n' "$arch" ;;
        *) fail "unsupported OS: $os (set CARRIER_LABEL to override)" ;;
    esac
}

resolve_main_sha() {
    repo="$1"
    api_url="https://api.github.com/repos/${repo}/commits/main"
    json="$(curl -fsSL --retry 3 --retry-delay 2 "$api_url")" || return 1
    sha="$(printf '%s\n' "$json" | awk -F'"' '/"sha":/ {print $4; exit}')"
    is_full_sha "$sha" || return 1
    printf '%s\n' "$sha"
}

http_code() {
    curl -sS -o /dev/null -w '%{http_code}' -L "$1" || true
}

if [ -n "$TAG" ] && [ -n "$SHA" ]; then
    fail "CARRIER_TAG and CARRIER_SHA cannot both be set"
fi

if [ -z "$LABEL" ]; then
    LABEL="$(detect_label)"
fi

if [ -z "$TAG" ]; then
    if [ -z "$SHA" ]; then
        log "Resolving main SHA from ${REPO}..."
        SHA="$(resolve_main_sha "$REPO")" || fail "failed to resolve main SHA from ${REPO}"
    fi
    is_full_sha "$SHA" || fail "CARRIER_SHA must be a full 40-character commit SHA"
    TAG="main-${SHA}"
fi

if [ -z "$SHA" ]; then
    case "$TAG" in
        main-*)
            maybe_sha="${TAG#main-}"
            if is_full_sha "$maybe_sha"; then
                SHA="$maybe_sha"
            fi
            ;;
    esac
fi

ZIP_TAG="$TAG"
ZIP_NAME="carrier-${ZIP_TAG}-${LABEL}.zip"
BASE_URL="https://github.com/${REPO}/releases/download/${TAG}"
ASSET_URL="${BASE_URL}/${ZIP_NAME}"
SUM_URL="${BASE_URL}/${ZIP_NAME}.sha256"

if [ "${DRY_RUN:-}" ]; then
    log "[DRY_RUN] release tag: ${TAG}"
    log "[DRY_RUN] asset URL: ${ASSET_URL}"
    log "[DRY_RUN] checksum URL: ${SUM_URL}"
    log "[DRY_RUN] unzip -o ${ZIP_NAME}"
    log "[DRY_RUN] install -m 755 ${BINARY_NAME} ${INSTALL_DIR}/${BINARY_NAME}"
    exit 0
fi

require_cmd curl
require_cmd unzip
require_cmd install

status="$(http_code "$ASSET_URL")"
if [ "$status" != "200" ] && [ -n "$SHA" ] && [ "$TAG" = "main-${SHA}" ]; then
    legacy_name="carrier-${SHA}-${LABEL}.zip"
    legacy_asset_url="${BASE_URL}/${legacy_name}"
    legacy_sum_url="${BASE_URL}/${legacy_name}.sha256"
    legacy_status="$(http_code "$legacy_asset_url")"
    if [ "$legacy_status" = "200" ]; then
        log "Primary asset name not found, falling back to legacy naming: ${legacy_name}"
        ZIP_NAME="$legacy_name"
        ASSET_URL="$legacy_asset_url"
        SUM_URL="$legacy_sum_url"
        status="$legacy_status"
    fi
fi

if [ "$status" != "200" ]; then
    fail "release asset not found (HTTP ${status}): ${ASSET_URL}"
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

ZIP_PATH="${TMPDIR}/${ZIP_NAME}"
SUM_PATH="${TMPDIR}/${ZIP_NAME}.sha256"
EXTRACT_DIR="${TMPDIR}/extract"

log "Downloading ${ASSET_URL}"
curl -fL --retry 3 --retry-delay 2 -o "$ZIP_PATH" "$ASSET_URL"

if [ "$VERIFY_CHECKSUM" = "1" ]; then
    log "Downloading checksum ${SUM_URL}"
    curl -fL --retry 3 --retry-delay 2 -o "$SUM_PATH" "$SUM_URL"

    if command -v sha256sum >/dev/null 2>&1; then
        log "Verifying checksum with sha256sum"
        (
            cd "$TMPDIR"
            sha256sum -c "${ZIP_NAME}.sha256"
        )
    elif command -v shasum >/dev/null 2>&1; then
        log "Verifying checksum with shasum"
        expected="$(awk '{print $1}' "$SUM_PATH" | tr '[:upper:]' '[:lower:]')"
        actual="$(shasum -a 256 "$ZIP_PATH" | awk '{print $1}' | tr '[:upper:]' '[:lower:]')"
        if [ "$expected" != "$actual" ]; then
            fail "checksum mismatch for ${ZIP_NAME} (expected=${expected} actual=${actual})"
        fi
    else
        fail "no SHA-256 checksum utility found (sha256sum or shasum)"
    fi
fi

log "Extracting ${ZIP_NAME}"
mkdir -p "$EXTRACT_DIR"
unzip -o "$ZIP_PATH" -d "$EXTRACT_DIR" >/dev/null

BIN_PATH="${EXTRACT_DIR}/${BINARY_NAME}"
if [ ! -f "$BIN_PATH" ]; then
    BIN_PATH="$(find "$EXTRACT_DIR" -type f -name "$BINARY_NAME" 2>/dev/null | head -n 1 || true)"
fi

if [ -z "$BIN_PATH" ] || [ ! -f "$BIN_PATH" ]; then
    fail "binary ${BINARY_NAME} not found in extracted archive"
fi

chmod +x "$BIN_PATH"
TARGET_PATH="${INSTALL_DIR}/${BINARY_NAME}"

if [ -d "$INSTALL_DIR" ] && [ -w "$INSTALL_DIR" ]; then
    install -m 755 "$BIN_PATH" "$TARGET_PATH"
elif command -v sudo >/dev/null 2>&1; then
    sudo mkdir -p "$INSTALL_DIR"
    sudo install -m 755 "$BIN_PATH" "$TARGET_PATH"
else
    FALLBACK_DIR="${HOME}/.local/bin"
    mkdir -p "$FALLBACK_DIR"
    TARGET_PATH="${FALLBACK_DIR}/${BINARY_NAME}"
    install -m 755 "$BIN_PATH" "$TARGET_PATH"
    log "sudo unavailable; installed to ${TARGET_PATH}"
fi

log "Installed ${BINARY_NAME} to ${TARGET_PATH}"
if "$TARGET_PATH" --version >/dev/null 2>&1; then
    "$TARGET_PATH" --version
fi
