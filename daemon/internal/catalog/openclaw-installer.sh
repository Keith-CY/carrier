#!/bin/sh
# OpenClaw installer with pinned artifacts and checksum verification
# Checksum MUST be provided as $2 (pinned in carrier manifest) or via OPENCLAW_CHECKSUM env var

set -e

VERSION="${1:?VERSION required}"
# Support checksum from either $2 argument (daemon use) or OPENCLAW_CHECKSUM env var (standalone use)
EXPECTED_CHECKSUM="${2:-${OPENCLAW_CHECKSUM:-}}"

if [ -z "$EXPECTED_CHECKSUM" ]; then
    echo "ERROR: Checksum required but not provided" >&2
    echo "Provide checksum as second argument or set OPENCLAW_CHECKSUM environment variable" >&2
    exit 1
fi

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
if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL_CHECKSUM="$(sha256sum "$ARTIFACT" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
    ACTUAL_CHECKSUM="$(shasum -a 256 "$ARTIFACT" | awk '{print $1}')"
else
    echo "ERROR: no sha256sum or shasum found" >&2; exit 1
fi
if [ "$ACTUAL_CHECKSUM" != "$EXPECTED_CHECKSUM" ]; then
    echo "ERROR: Checksum mismatch!" >&2
    echo "  Expected: $EXPECTED_CHECKSUM" >&2
    echo "  Actual:   $ACTUAL_CHECKSUM" >&2
    exit 1
fi
echo "Checksum OK"

# Verify signature if available (optional but recommended)
SIGNATURE_FILE="${ARTIFACT}.sig"
if curl -fsSL -o "$SIGNATURE_FILE" "${BASE_URL}/${SIGNATURE_FILE}" 2>/dev/null; then
    echo "Signature file found, verifying..."
    
    # Try cosign first (preferred for GitHub releases)
    if command -v cosign >/dev/null 2>&1; then
        echo "Using cosign for signature verification..."
        # Use GitHub Actions OIDC for keyless verification
        if cosign verify-blob \
            --certificate-identity "https://github.com/openclaw/openclaw/.github/workflows/release.yml@refs/heads/main" \
            --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
            --signature "$SIGNATURE_FILE" \
            "$ARTIFACT" >/dev/null 2>&1; then
            echo "✓ Signature verification PASSED"
        else
            echo "WARNING: Signature verification FAILED" >&2
            echo "  The artifact signature could not be verified." >&2
            echo "  Installation will continue, but consider this a security risk." >&2
        fi
    # Fall back to GPG if available
    elif command -v gpg >/dev/null 2>&1; then
        echo "Using GPG for signature verification..."
        if gpg --verify "$SIGNATURE_FILE" "$ARTIFACT" >/dev/null 2>&1; then
            echo "✓ Signature verification PASSED"
        else
            echo "WARNING: Signature verification FAILED" >&2
            echo "  The artifact signature could not be verified." >&2
            echo "  Installation will continue, but consider this a security risk." >&2
        fi
    else
        echo "WARNING: No signature verification tool available (cosign or gpg)" >&2
        echo "  Consider installing cosign for enhanced security." >&2
    fi
else
    echo "No signature file available for this release"
    echo "  Signature verification skipped (checksum verification only)"
fi

# Extract
echo "Extracting archive..."
tar xzf "$ARTIFACT"

# Install binary
echo "Installing to /usr/local/bin/openclaw..."
install -m 755 openclaw /usr/local/bin/openclaw

echo "OpenClaw ${VERSION} installed successfully"
