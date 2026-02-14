#!/bin/sh
# OpenClaw installer with pinned artifacts and checksum verification
# Checksum MUST be provided as $2 (pinned in carrier manifest, not fetched remotely)

set -e

VERSION="${1:?VERSION required}"
EXPECTED_CHECKSUM="${2:?EXPECTED_CHECKSUM required (pinned in manifest)}"

# Platform detection
OS="$(uname -s)"
ARCH="$(uname -m)"
ARTIFACT="openclaw-${OS}-${ARCH}.tar.gz"
BASE_URL="https://github.com/openclaw/openclaw/releases/download/v${VERSION}"

# Create temporary directory
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

cd "$TMPDIR"

# Download artifact only (no remote checksum fetch)
echo "Downloading ${ARTIFACT} from ${BASE_URL}..."
curl -fsSL -o "$ARTIFACT" "${BASE_URL}/${ARTIFACT}"

# Verify against pinned checksum from carrier manifest
echo "Verifying integrity against pinned checksum..."
ACTUAL_CHECKSUM="$(sha256sum "$ARTIFACT" | awk '{print $1}')"
if [ "$ACTUAL_CHECKSUM" != "$EXPECTED_CHECKSUM" ]; then
    echo "ERROR: Checksum mismatch!" >&2
    echo "  Expected: $EXPECTED_CHECKSUM" >&2
    echo "  Actual:   $ACTUAL_CHECKSUM" >&2
    exit 1
fi
echo "Checksum OK"

# Extract
echo "Extracting archive..."
tar xzf "$ARTIFACT"

# Install binary
echo "Installing to /usr/local/bin/openclaw..."
install -m 755 openclaw /usr/local/bin/openclaw

echo "OpenClaw ${VERSION} installed successfully"
