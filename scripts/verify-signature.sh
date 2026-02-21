#!/usr/bin/env bash
# verify-signature.sh - Verify artifact signatures using cosign or GPG
#
# Usage: ./verify-signature.sh <artifact-file> [signature-file]
#
# This script verifies signatures using either:
#   1. cosign (preferred) - keyless OIDC verification for GitHub releases
#   2. GPG (fallback) - traditional GPG signature verification
#
# If signature-file is not provided, it assumes <artifact-file>.sig.
# For cosign keyless verification, it prefers <artifact-file>.sigstore.json.

set -euo pipefail

ARTIFACT_FILE="${1:-}"
SIGNATURE_FILE="${2:-}"
SIGNATURE_FILE_EXPLICIT=false
if [[ -n "$SIGNATURE_FILE" ]]; then
    SIGNATURE_FILE_EXPLICIT=true
fi

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
    echo "  COSIGN_CERT_IDENTITY   Expected certificate identity (for cosign)"
    echo "  COSIGN_OIDC_ISSUER     Expected OIDC issuer (for cosign)"
    echo "  COSIGN_BUNDLE_FILE     Path to Sigstore bundle file (default: artifact.sigstore.json)"
    echo "  COSIGN_CERT_FILE       Path to PEM certificate file (fallback when no bundle)"
    echo "  GPG_KEYRING            Path to GPG keyring (for GPG verification)"
    echo ""
    echo "Examples:"
    echo "  $0 carrier-linux-x64.zip"
    echo "  $0 carrier-linux-x64.zip carrier-linux-x64.zip.sig"
    echo "  # For cosign, explicit signature-file requires COSIGN_CERT_FILE"
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

BUNDLE_FILE="${COSIGN_BUNDLE_FILE:-${ARTIFACT_FILE}.sigstore.json}"

[[ -f "$SIGNATURE_FILE" ]] && HAS_SIGNATURE_FILE=true || HAS_SIGNATURE_FILE=false

[[ -f "$BUNDLE_FILE" ]] && HAS_BUNDLE_FILE=true || HAS_BUNDLE_FILE=false

if [[ "$SIGNATURE_FILE_EXPLICIT" == true && "$HAS_SIGNATURE_FILE" == false ]]; then
    log_error "Signature file not found: $SIGNATURE_FILE"
    exit 1
fi

if [[ "$HAS_SIGNATURE_FILE" == false && "$HAS_BUNDLE_FILE" == false ]]; then
    log_warn "Signature file not found: $SIGNATURE_FILE"
    log_warn "Sigstore bundle not found: $BUNDLE_FILE"
    log_warn "Signature verification skipped (no verification material available)"
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
        
        # Build cosign verify-blob command.
        COSIGN_ARGS=(verify-blob)

        # Keep backward compatibility for older env var names.
        CERT_IDENTITY="${COSIGN_CERT_IDENTITY:-${COSIGN_CERTIFICATE:-https://github.com/Keith-CY/carrier/.github/workflows/release.yml@refs/heads/main}}"
        CERT_ISSUER="${COSIGN_OIDC_ISSUER:-${COSIGN_ISSUER:-https://token.actions.githubusercontent.com}}"

        COSIGN_ARGS+=(
            --certificate-identity "$CERT_IDENTITY"
            --certificate-oidc-issuer "$CERT_ISSUER"
        )

        if [[ "$SIGNATURE_FILE_EXPLICIT" == false && "$HAS_BUNDLE_FILE" == true ]]; then
            log_info "Using Sigstore bundle: $(basename "$BUNDLE_FILE")"
            COSIGN_ARGS+=(--bundle "$BUNDLE_FILE")
        else
            if [[ "$HAS_SIGNATURE_FILE" == false ]]; then
                log_error "Signature file not found: $SIGNATURE_FILE"
                exit 1
            fi

            CERT_FILE="${COSIGN_CERT_FILE:-${ARTIFACT_FILE}.pem}"
            if [[ ! -f "$CERT_FILE" ]]; then
                if [[ "$SIGNATURE_FILE_EXPLICIT" == true ]]; then
                    log_error "Explicit signature file requires a certificate for cosign verification."
                    log_error "Provide COSIGN_CERT_FILE (or place certificate at $CERT_FILE),"
                    log_error "or omit the signature-file argument to use bundle verification."
                else
                    log_error "Cosign keyless verification requires a bundle or certificate."
                    log_error "Expected bundle: $BUNDLE_FILE"
                    log_error "Or provide certificate via COSIGN_CERT_FILE (default path: $CERT_FILE)"
                fi
                exit 1
            fi

            COSIGN_ARGS+=(
                --certificate "$CERT_FILE"
                --signature "$SIGNATURE_FILE"
            )
        fi

        COSIGN_ARGS+=("$ARTIFACT_FILE")
        
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

        if [[ "$HAS_SIGNATURE_FILE" == false ]]; then
            log_warn "Signature file not found: $SIGNATURE_FILE"
            log_warn "GPG verification skipped (no signature available)"
            exit 2
        fi
        
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
