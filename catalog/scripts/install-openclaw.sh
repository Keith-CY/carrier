#!/bin/sh
# OpenClaw installer with pinned artifacts and checksum verification
# No pipe-to-shell execution - downloads, verifies, then installs

set -e

VERSION="${OPENCLAW_VERSION:-1.0.0}"
BASE_URL="https://github.com/Keith-CY/carrier/releases/download/v${VERSION}"

# Platform detection
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
BINARY="openclaw-v${VERSION}-${OS}-${ARCH}"

# Determine checksum command
if [ "$OS" = "darwin" ]; then
    CHECKSUM_CMD="shasum -a 256"
else
    CHECKSUM_CMD="sha256sum"
fi

# Create temporary directory
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

cd "$TMPDIR"

# Download binary
echo "Downloading ${BINARY}..."
curl -fsSL -O "${BASE_URL}/${BINARY}"

# Checksum MUST be provided via OPENCLAW_CHECKSUM env var (pinned at release time).
# Fail-closed: no checksum = no install. Never fall back to remote checksum files.
EXPECTED="${OPENCLAW_CHECKSUM:-}"

echo "Verifying integrity against pinned checksum..."
if [ -z "$EXPECTED" ]; then
    echo "ERROR: OPENCLAW_CHECKSUM not set. Cannot verify artifact integrity." >&2
    echo "Set OPENCLAW_CHECKSUM to the expected SHA-256 hash of ${BINARY}." >&2
    exit 1
fi

ACTUAL=$(${CHECKSUM_CMD} "$BINARY" | awk '{print $1}')
if [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "ERROR: Checksum mismatch!" >&2
    echo "  Expected: $EXPECTED" >&2
    echo "  Actual:   $ACTUAL" >&2
    exit 1
fi
echo "Checksum OK"

# Install binary
echo "Installing to /usr/local/bin/openclaw..."
sudo install -m 755 "$BINARY" /usr/local/bin/openclaw

echo "OpenClaw ${VERSION} installed successfully"
