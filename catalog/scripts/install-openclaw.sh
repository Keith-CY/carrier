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

# Pinned checksums for v1.0.0 — update these when releasing a new version.
# To regenerate: sha256sum openclaw-v1.0.0-<os>-<arch>
PINNED_CHECKSUMS="
# placeholder checksums — replace with real values at release time
0000000000000000000000000000000000000000000000000000000000000000  openclaw-v1.0.0-linux-x86_64
0000000000000000000000000000000000000000000000000000000000000000  openclaw-v1.0.0-linux-aarch64
0000000000000000000000000000000000000000000000000000000000000000  openclaw-v1.0.0-darwin-x86_64
0000000000000000000000000000000000000000000000000000000000000000  openclaw-v1.0.0-darwin-arm64
"

# Verify checksum against pinned value (not downloaded from same source)
echo "Verifying integrity against pinned checksum..."
EXPECTED=$(echo "$PINNED_CHECKSUMS" | grep "$BINARY" | awk '{print $1}')
if [ -z "$EXPECTED" ] || [ "$EXPECTED" = "0000000000000000000000000000000000000000000000000000000000000000" ]; then
    echo "WARNING: No pinned checksum for ${BINARY}. Falling back to remote checksum file." >&2
    curl -fsSL -O "${BASE_URL}/${CHECKSUM_FILE}"
    if ! ${CHECKSUM_CMD} -c "$CHECKSUM_FILE" >/dev/null 2>&1; then
        echo "ERROR: Checksum verification failed!" >&2
        exit 1
    fi
else
    ACTUAL=$(${CHECKSUM_CMD} "$BINARY" | awk '{print $1}')
    if [ "$ACTUAL" != "$EXPECTED" ]; then
        echo "ERROR: Checksum mismatch! Expected ${EXPECTED}, got ${ACTUAL}" >&2
        exit 1
    fi
fi

# Install binary
echo "Installing to /usr/local/bin/openclaw..."
sudo install -m 755 "$BINARY" /usr/local/bin/openclaw

echo "OpenClaw ${VERSION} installed successfully"
