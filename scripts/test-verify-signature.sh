#!/usr/bin/env bash
# test-verify-signature.sh - Test signature verification script
#
# Tests the verify-signature.sh script with various scenarios

set -uo pipefail  # Don't use -e since we're testing failure cases

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERIFY_SCRIPT="$SCRIPT_DIR/verify-signature.sh"

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_test() {
    echo -e "${YELLOW}[TEST]${NC} $*"
}

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $*"
    ((TESTS_PASSED++))
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $*"
    ((TESTS_FAILED++))
}

run_test() {
    ((TESTS_RUN++))
    log_test "$1"
}

# Setup test directory
TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT

cd "$TEST_DIR"

# Test 1: Missing artifact file
run_test "Test 1: Missing artifact file should fail"
if "$VERIFY_SCRIPT" nonexistent.tar.gz 2>/dev/null; then
    log_fail "Should fail when artifact doesn't exist"
else
    EXIT_CODE=$?
    if [[ $EXIT_CODE -eq 1 ]]; then
        log_pass "Correctly failed with exit code 1"
    else
        log_fail "Wrong exit code: expected 1, got $EXIT_CODE"
    fi
fi

# Test 2: Artifact exists but no signature file
run_test "Test 2: No signature file should exit with code 2 (skipped)"
echo "dummy content" > artifact.tar.gz
if "$VERIFY_SCRIPT" artifact.tar.gz 2>/dev/null; then
    EXIT_CODE=$?
    log_fail "Should exit with non-zero when signature missing"
else
    EXIT_CODE=$?
    if [[ $EXIT_CODE -eq 2 ]]; then
        log_pass "Correctly skipped with exit code 2"
    else
        log_fail "Wrong exit code: expected 2, got $EXIT_CODE"
    fi
fi

# Test 3: Both artifact and signature exist, but GPG/cosign not available
run_test "Test 3: No verification tools available"
echo "dummy signature" > artifact.tar.gz.sig
# Force no method available
if VERIFICATION_METHOD=none "$VERIFY_SCRIPT" artifact.tar.gz 2>/dev/null; then
    EXIT_CODE=$?
    log_fail "Should exit with non-zero when no tools available"
else
    EXIT_CODE=$?
    if [[ $EXIT_CODE -eq 2 ]]; then
        log_pass "Correctly skipped with exit code 2 when no tools available"
    else
        log_fail "Wrong exit code: expected 2, got $EXIT_CODE"
    fi
fi

# Test 4: Custom signature file path
run_test "Test 4: Custom signature file path"
echo "test data" > custom-artifact.bin
echo "test signature" > custom-signature.asc
# This should fail verification but should use the custom signature path
if VERIFICATION_METHOD=none "$VERIFY_SCRIPT" custom-artifact.bin custom-signature.asc 2>/dev/null; then
    EXIT_CODE=$?
    log_fail "Should not succeed with no verification method"
else
    EXIT_CODE=$?
    # Should use the custom signature file (even if verification fails/skips)
    if [[ $EXIT_CODE -eq 2 ]]; then
        log_pass "Custom signature file path accepted"
    else
        log_fail "Unexpected exit code: $EXIT_CODE"
    fi
fi

# Test 5: GPG verification (if gpg available)
if command -v gpg &>/dev/null; then
    run_test "Test 5: GPG verification flow (simulated)"
    
    # Create a temporary GPG home
    GNUPGHOME="$TEST_DIR/gpg-home"
    export GNUPGHOME
    mkdir -p "$GNUPGHOME"
    chmod 700 "$GNUPGHOME"
    
    # Generate a test key (non-interactive)
    cat > "$GNUPGHOME/key-gen-params" <<EOF
%no-protection
Key-Type: RSA
Key-Length: 2048
Name-Real: Test Signer
Name-Email: test@example.com
Expire-Date: 0
%commit
EOF
    
    if gpg --batch --generate-key "$GNUPGHOME/key-gen-params" 2>/dev/null; then
        # Create and sign a test file
        echo "test data for signing" > signed-artifact.tar.gz
        
        if gpg --armor --detach-sign -o signed-artifact.tar.gz.sig signed-artifact.tar.gz 2>/dev/null; then
            # Verify with our script
            if VERIFICATION_METHOD=gpg "$VERIFY_SCRIPT" signed-artifact.tar.gz 2>/dev/null; then
                log_pass "GPG verification succeeded"
            else
                log_fail "GPG verification should have succeeded with valid signature"
            fi
            
            # Test with tampered file
            echo "tampered" > signed-artifact.tar.gz
            if VERIFICATION_METHOD=gpg "$VERIFY_SCRIPT" signed-artifact.tar.gz 2>/dev/null; then
                log_fail "GPG verification should fail with tampered file"
            else
                log_pass "GPG verification correctly failed with tampered file"
            fi
        else
            log_fail "Could not create test signature"
        fi
    else
        log_fail "Could not generate test GPG key"
    fi
    
    unset GNUPGHOME
else
    run_test "Test 5: GPG verification (skipped - gpg not available)"
    log_pass "Skipped (gpg not installed)"
fi

# Test 6: Script has correct executable permissions
run_test "Test 6: Script is executable"
if [[ -x "$VERIFY_SCRIPT" ]]; then
    log_pass "Script has executable permissions"
else
    log_fail "Script is not executable"
fi

# Test 7: Usage message
run_test "Test 7: Usage message when no arguments"
if "$VERIFY_SCRIPT" 2>&1 | grep -q "Usage:"; then
    log_pass "Shows usage message when called without arguments"
else
    log_fail "Should show usage message when called without arguments"
fi

# Test 8: Environment variable override (VERIFICATION_METHOD)
run_test "Test 8: VERIFICATION_METHOD environment variable"
echo "test" > env-test.tar.gz
echo "sig" > env-test.tar.gz.sig

# Test that VERIFICATION_METHOD is respected
# Temporarily disable pipefail since we expect the command to fail
set +o pipefail
if VERIFICATION_METHOD=invalid "$VERIFY_SCRIPT" env-test.tar.gz 2>&1 | grep -q "Unknown verification method"; then
    log_pass "VERIFICATION_METHOD environment variable is respected"
else
    log_fail "Should reject invalid VERIFICATION_METHOD"
fi
set -o pipefail

# Summary
echo ""
echo "========================================="
echo "Test Summary"
echo "========================================="
echo "Tests run:    $TESTS_RUN"
echo "Tests passed: $TESTS_PASSED"
echo "Tests failed: $TESTS_FAILED"
echo "========================================="

if [[ $TESTS_FAILED -eq 0 ]]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed!${NC}"
    exit 1
fi
