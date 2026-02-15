#!/bin/sh
# Carrier install script with checksum verification
# Downloads, verifies, and installs openclaw binary from GitHub releases
#
# Usage:
#   OPENCLAW_VERSION=1.0.0 OPENCLAW_CHECKSUM=abc123... ./install.sh
#   Or pipe from curl (checksum still required via env):
#   curl -fsSL https://raw.githubusercontent.com/Keith-CY/carrier/main/scripts/install.sh | \
#     OPENCLAW_VERSION=1.0.0 OPENCLAW_CHECKSUM=abc123... sh
#
# Environment Variables:
#   OPENCLAW_VERSION    - Version to install (default: 1.0.0)
#   OPENCLAW_CHECKSUM   - Expected SHA-256 checksum (REQUIRED)
#   INSTALL_DIR         - Installation directory (default: /usr/local/bin)
#   DRY_RUN             - If set, print actions without executing (for testing)

set -e

VERSION="${OPENCLAW_VERSION:-1.0.0}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BASE_URL="https://github.com/Keith-CY/carrier/releases/download/v${VERSION}"

# Platform detection
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

# Normalize architecture names
case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    armv7l) ARCH="armv7" ;;
    riscv64) ARCH="riscv64" ;;
    *) ARCH="$ARCH" ;;
esac

BINARY="openclaw-v${VERSION}-${OS}-${ARCH}"
BINARY_NAME="openclaw"

# Determine checksum command
if command -v sha256sum >/dev/null 2>&1; then
    CHECKSUM_CMD="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
    CHECKSUM_CMD="shasum -a 256"
else
    echo "ERROR: No SHA-256 checksum utility found (sha256sum or shasum)" >&2
    exit 1
fi

# Check for required checksum
EXPECTED="${OPENCLAW_CHECKSUM:-}"
if [ -z "$EXPECTED" ]; then
    echo "ERROR: OPENCLAW_CHECKSUM not set. Cannot verify artifact integrity." >&2
    echo "" >&2
    echo "Set OPENCLAW_CHECKSUM to the expected SHA-256 hash of ${BINARY}." >&2
    echo "Find checksums at: ${BASE_URL}/" >&2
    exit 1
fi

# Create temporary directory
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

cd "$TMPDIR"

# Download binary
echo "Downloading ${BINARY}..."
if [ "${DRY_RUN:-}" ]; then
    echo "[DRY_RUN] curl -fsSL -O ${BASE_URL}/${BINARY}"
else
    if ! curl -fsSL -O "${BASE_URL}/${BINARY}"; then
        echo "ERROR: Failed to download ${BINARY}" >&2
        echo "Check that version ${VERSION} exists and includes ${OS}-${ARCH} build" >&2
        exit 1
    fi
fi

# Verify checksum
echo "Verifying integrity against pinned checksum..."
if [ "${DRY_RUN:-}" ]; then
    echo "[DRY_RUN] Checksum verification: ${EXPECTED}"
else
    ACTUAL=$(${CHECKSUM_CMD} "$BINARY" | awk '{print $1}')
    if [ "$ACTUAL" != "$EXPECTED" ]; then
        echo "ERROR: Checksum mismatch!" >&2
        echo "  Expected: $EXPECTED" >&2
        echo "  Actual:   $ACTUAL" >&2
        echo "" >&2
        echo "This may indicate:" >&2
        echo "  - Corrupted download" >&2
        echo "  - Incorrect OPENCLAW_CHECKSUM value" >&2
        echo "  - Man-in-the-middle attack" >&2
        exit 1
    fi
    echo "Checksum OK"
fi

# Install binary
if [ -w "$INSTALL_DIR" ]; then
    # Can write directly
    echo "Installing to ${INSTALL_DIR}/${BINARY_NAME}..."
    if [ "${DRY_RUN:-}" ]; then
        echo "[DRY_RUN] install -m 755 $BINARY ${INSTALL_DIR}/${BINARY_NAME}"
    else
        install -m 755 "$BINARY" "${INSTALL_DIR}/${BINARY_NAME}"
    fi
else
    # Need sudo
    echo "Installing to ${INSTALL_DIR}/${BINARY_NAME} (requires sudo)..."
    if [ "${DRY_RUN:-}" ]; then
        echo "[DRY_RUN] sudo install -m 755 $BINARY ${INSTALL_DIR}/${BINARY_NAME}"
    else
        sudo install -m 755 "$BINARY" "${INSTALL_DIR}/${BINARY_NAME}"
    fi
fi

if [ "${DRY_RUN:-}" ]; then
    echo "[DRY_RUN] Installation complete"
else
    echo "OpenClaw ${VERSION} installed successfully"
    echo "Run: openclaw --version"
fi
