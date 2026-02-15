#!/usr/bin/env bash
# verify-signature.sh - Verify artifact signatures using cosign or GPG
#
# Usage: ./verify-signature.sh <artifact-file> [signature-file]
#
# This script verifies signatures using either:
#   1. cosign (preferred) - keyless OIDC verification for GitHub releases
#   2. GPG (fallback) - traditional GPG signature verification
#
# If signature-file is not provided, it assumes <artifact-file>.sig

set -euo pipefail

ARTIFACT_FILE="${1:-}"
SIGNATURE_FILE="${2:-}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

usage() {
    local exit_code="${1:-1}"  # Default to 1 for errors, 0 for help
    echo "Usage: $0 <artifact-file> [signature-file]"
    echo ""
    echo "Verify the signature of an artifact using cosign or GPG."
    echo ""
    echo "Environment variables:"
    echo "  VERIFICATION_METHOD    Force specific method: 'cosign' or 'gpg'"
    echo "  COSIGN_CERTIFICATE     Expected certificate identity (for cosign)"
    echo "  COSIGN_ISSUER          Expected OIDC issuer (for cosign)"
    echo "  GPG_KEYRING            Path to GPG keyring (for GPG verification)"
    echo ""
    echo "Examples:"
    echo "  $0 openclaw-Linux-x86_64.tar.gz"
    echo "  $0 openclaw-Linux-x86_64.tar.gz openclaw-Linux-x86_64.tar.gz.sig"
    echo "  VERIFICATION_METHOD=gpg $0 artifact.zip"
    exit "$exit_code"
}

if [[ -z "$ARTIFACT_FILE" ]] || [[ "$ARTIFACT_FILE" == "-h" ]] || [[ "$ARTIFACT_FILE" == "--help" ]]; then
    usage 0  # Exit with 0 when showing help
fi

if [[ ! -f "$ARTIFACT_FILE" ]]; then
    log_error "Artifact file not found: $ARTIFACT_FILE"
    exit 1
fi

# Default signature file is artifact.sig
if [[ -z "$SIGNATURE_FILE" ]]; then
    SIGNATURE_FILE="${ARTIFACT_FILE}.sig"
fi

if [[ ! -f "$SIGNATURE_FILE" ]]; then
    log_warn "Signature file not found: $SIGNATURE_FILE"
    log_warn "Signature verification skipped (no signature available)"
    exit 2  # Exit code 2 = signature not available (not a hard failure)
fi

# Detect or use forced verification method
VERIFICATION_METHOD="${VERIFICATION_METHOD:-auto}"

detect_method() {
    if command -v cosign &>/dev/null; then
        echo "cosign"
    elif command -v gpg &>/dev/null; then
        echo "gpg"
    else
        echo "none"
    fi
}

if [[ "$VERIFICATION_METHOD" == "auto" ]]; then
    VERIFICATION_METHOD=$(detect_method)
fi

case "$VERIFICATION_METHOD" in
    cosign)
        if ! command -v cosign &>/dev/null; then
            log_error "cosign not found but VERIFICATION_METHOD=cosign"
            exit 1
        fi

        log_info "Verifying signature using cosign (keyless verification)..."
        
        # Build cosign verify-blob command
        COSIGN_ARGS=(verify-blob)
        
        # Use GitHub Actions OIDC defaults if not specified
        CERT_IDENTITY="${COSIGN_CERTIFICATE:-https://github.com/openclaw/openclaw/.github/workflows/release.yml@refs/heads/main}"
        CERT_ISSUER="${COSIGN_ISSUER:-https://token.actions.githubusercontent.com}"
        
        COSIGN_ARGS+=(
            --certificate-identity "$CERT_IDENTITY"
            --certificate-oidc-issuer "$CERT_ISSUER"
            --signature "$SIGNATURE_FILE"
            "$ARTIFACT_FILE"
        )
        
        if cosign "${COSIGN_ARGS[@]}"; then
            log_info "✓ Signature verification PASSED (cosign)"
            exit 0
        else
            log_error "✗ Signature verification FAILED (cosign)"
            exit 1
        fi
        ;;
        
    gpg)
        if ! command -v gpg &>/dev/null; then
            log_error "gpg not found but VERIFICATION_METHOD=gpg"
            exit 1
        fi

        log_info "Verifying signature using GPG..."
        
        GPG_ARGS=(--verify)
        
        # Use custom keyring if specified
        if [[ -n "${GPG_KEYRING:-}" ]]; then
            GPG_ARGS+=(--keyring "$GPG_KEYRING")
        fi
        
        GPG_ARGS+=("$SIGNATURE_FILE" "$ARTIFACT_FILE")
        
        if gpg "${GPG_ARGS[@]}" 2>&1; then
            log_info "✓ Signature verification PASSED (GPG)"
            exit 0
        else
            log_error "✗ Signature verification FAILED (GPG)"
            exit 1
        fi
        ;;
        
    none)
        log_warn "No verification tool available (cosign or gpg required)"
        log_warn "Signature verification skipped"
        exit 2  # Exit code 2 = tool not available
        ;;
        
    *)
        log_error "Unknown verification method: $VERIFICATION_METHOD"
        log_error "Supported methods: cosign, gpg, auto"
        usage
        ;;
esac
