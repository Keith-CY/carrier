#!/bin/sh
# OpenClaw installer with pinned artifacts and checksum verification
# No pipe-to-shell execution - downloads, verifies, then installs

set -e

VERSION="${1:-1.0.0}"
BASE_URL="https://github.com/openclaw/openclaw/releases/download/v${VERSION}"
EXPECTED_CHECKSUM="${2:-}"

# Platform detection
OS="$(uname -s)"
ARCH="$(uname -m)"
ARTIFACT="openclaw-${OS}-${ARCH}.tar.gz"
CHECKSUM_FILE="${ARTIFACT}.sha256"

# Create temporary directory
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

cd "$TMPDIR"

# Download artifact
echo "Downloading ${ARTIFACT} from ${BASE_URL}..."
curl -fsSL -o "$ARTIFACT" "${BASE_URL}/${ARTIFACT}"

# Download checksum
echo "Downloading checksum file..."
curl -fsSL -o "$CHECKSUM_FILE" "${BASE_URL}/${CHECKSUM_FILE}"

# Verify checksum
echo "Verifying integrity..."
if ! sha256sum -c "$CHECKSUM_FILE" 2>&1 | grep -q "OK"; then
    echo "ERROR: Checksum verification failed!" >&2
    exit 1
fi

# Extract
echo "Extracting archive..."
tar xzf "$ARTIFACT"

# Install binary
echo "Installing to /usr/local/bin/openclaw..."
install -m 755 openclaw /usr/local/bin/openclaw

echo "OpenClaw ${VERSION} installed successfully"
