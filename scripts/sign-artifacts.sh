#!/usr/bin/env bash
# sign-artifacts.sh - Sign release artifacts with cosign or GPG
#
# Usage: ./sign-artifacts.sh <artifacts-dir>
#
# This script signs all files in the specified directory using either:
#   1. cosign (preferred) - keyless OIDC signing for GitHub Actions
#   2. GPG (fallback) - traditional GPG signing with local key
#
# The script automatically detects which tool is available and uses the
# appropriate signing method.

set -euo pipefail

ARTIFACTS_DIR="${1:-}"

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
    echo "Usage: $0 <artifacts-dir>"
    echo ""
    echo "Sign all files in the specified directory using cosign or GPG."
    echo ""
    echo "Environment variables:"
    echo "  SIGNING_METHOD    Force specific method: 'cosign' or 'gpg'"
    echo "  GPG_KEY_ID        GPG key ID to use (for GPG signing)"
    echo "  GPG_PASSPHRASE    GPG key passphrase (for automated GPG signing)"
    echo ""
    echo "Examples:"
    echo "  $0 ./dist"
    echo "  SIGNING_METHOD=gpg GPG_KEY_ID=ABCD1234 $0 ./dist"
    exit 1
}

# Validate arguments
if [[ -z "$ARTIFACTS_DIR" ]]; then
    log_error "No artifacts directory specified"
    usage
fi

if [[ ! -d "$ARTIFACTS_DIR" ]]; then
    log_error "Directory not found: $ARTIFACTS_DIR"
    exit 1
fi

# Detect available signing tools
detect_signing_method() {
    if [[ -n "${SIGNING_METHOD:-}" ]]; then
        log_info "Using forced signing method: $SIGNING_METHOD" >&2
        echo "$SIGNING_METHOD"
        return
    fi

    if command -v cosign &> /dev/null; then
        log_info "Detected cosign - using keyless OIDC signing" >&2
        echo "cosign"
    elif command -v gpg &> /dev/null; then
        log_info "Detected GPG - using traditional GPG signing" >&2
        echo "gpg"
    else
        log_error "No signing tool available (cosign or gpg required)" >&2
        exit 1
    fi
}

# Sign with cosign (keyless OIDC)
sign_with_cosign() {
    local file="$1"
    local sig_file="${file}.sig"
    
    log_info "Signing with cosign: $(basename "$file")"
    
    # Check if running in CI with OIDC token
    if [[ -n "${GITHUB_ACTIONS:-}" ]] || [[ -n "${ACTIONS_ID_TOKEN_REQUEST_TOKEN:-}" ]]; then
        # GitHub Actions with keyless signing
        cosign sign-blob --yes "$file" --output-signature "$sig_file"
    else
        # Local signing (will prompt for ephemeral key or use existing key)
        if cosign sign-blob --yes "$file" --output-signature "$sig_file"; then
            log_info "Created signature: $(basename "$sig_file")"
        else
            log_error "cosign signing failed for: $(basename "$file")"
            return 1
        fi
    fi
}

# Sign with GPG
sign_with_gpg() {
    local file="$1"
    local sig_file="${file}.sig"
    
    log_info "Signing with GPG: $(basename "$file")"
    
    local gpg_opts=(--detach-sign --armor --output "$sig_file")
    
    # Add key ID if specified
    if [[ -n "${GPG_KEY_ID:-}" ]]; then
        gpg_opts+=(--local-user "$GPG_KEY_ID")
    fi
    
    # Add passphrase if specified (for automation)
    if [[ -n "${GPG_PASSPHRASE:-}" ]]; then
        gpg_opts+=(--batch --yes --passphrase "$GPG_PASSPHRASE")
    fi
    
    gpg_opts+=("$file")
    
    if gpg "${gpg_opts[@]}"; then
        log_info "Created signature: $(basename "$sig_file")"
    else
        log_error "GPG signing failed for: $(basename "$file")"
        return 1
    fi
}

# Main signing logic
main() {
    local method
    method=$(detect_signing_method)
    
    local artifacts=()
    while IFS= read -r -d '' file; do
        # Skip existing signature files
        if [[ "$file" == *.sig ]] || [[ "$file" == *.asc ]]; then
            continue
        fi
        artifacts+=("$file")
    done < <(find "$ARTIFACTS_DIR" -type f -print0)
    
    if [[ ${#artifacts[@]} -eq 0 ]]; then
        log_warn "No artifacts found in: $ARTIFACTS_DIR"
        exit 0
    fi
    
    log_info "Found ${#artifacts[@]} artifact(s) to sign"
    
    local signed=0
    local failed=0
    
    for artifact in "${artifacts[@]}"; do
        if [[ "$method" == "cosign" ]]; then
            if sign_with_cosign "$artifact"; then
                # Use pre-increment so set -e does not treat a 0-valued expression as failure.
                ((++signed))
            else
                ((++failed))
            fi
        elif [[ "$method" == "gpg" ]]; then
            if sign_with_gpg "$artifact"; then
                ((++signed))
            else
                ((++failed))
            fi
        else
            log_error "Unknown signing method: $method"
            exit 1
        fi
    done
    
    echo ""
    log_info "Signing complete: $signed signed, $failed failed"
    
    if [[ $failed -gt 0 ]]; then
        exit 1
    fi
}

main "$@"
