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
CHECKSUM_FILE="${BINARY}.sha256"

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

# Download checksum
echo "Downloading checksum file..."
curl -fsSL -O "${BASE_URL}/${CHECKSUM_FILE}"

# Verify checksum
echo "Verifying integrity..."
if ! ${CHECKSUM_CMD} -c "$CHECKSUM_FILE" >/dev/null 2>&1; then
    echo "ERROR: Checksum verification failed!" >&2
    exit 1
fi

# Install binary
echo "Installing to /usr/local/bin/openclaw..."
sudo install -m 755 "$BINARY" /usr/local/bin/openclaw

echo "OpenClaw ${VERSION} installed successfully"
