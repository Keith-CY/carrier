#!/bin/sh
# Test suite for install.sh
# Runs the install script in DRY_RUN mode with various configurations

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_SCRIPT="${SCRIPT_DIR}/install.sh"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test result tracking
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
print_test() {
    printf '%s[TEST]%s %s\n' "$YELLOW" "$NC" "$1"
    TESTS_RUN=$((TESTS_RUN + 1))
}

print_pass() {
    printf '%s[PASS]%s %s\n' "$GREEN" "$NC" "$1"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

print_fail() {
    printf '%s[FAIL]%s %s\n' "$RED" "$NC" "$1"
    TESTS_FAILED=$((TESTS_FAILED + 1))
}

run_dry_run() {
    DRY_RUN=1 "$@" 2>&1
}

# Test 1: Script exists and is executable
print_test "Install script exists and is executable"
if [ -f "$INSTALL_SCRIPT" ] && [ -x "$INSTALL_SCRIPT" ]; then
    print_pass "Script found and executable"
else
    print_fail "Script not found or not executable"
    chmod +x "$INSTALL_SCRIPT" 2>/dev/null || true
fi

# Test 2: Missing checksum fails
print_test "Missing OPENCLAW_CHECKSUM causes failure"
if run_dry_run "$INSTALL_SCRIPT" 2>&1 | grep -q "OPENCLAW_CHECKSUM not set"; then
    print_pass "Correctly rejects missing checksum"
else
    print_fail "Should reject missing checksum"
fi

# Test 3: Valid dry-run execution
print_test "Dry-run with valid checksum succeeds"
OUTPUT=$(OPENCLAW_VERSION=1.0.0 \
         OPENCLAW_CHECKSUM=0000000000000000000000000000000000000000000000000000000000000000 \
         run_dry_run "$INSTALL_SCRIPT")

if echo "$OUTPUT" | grep -q "\[DRY_RUN\]"; then
    print_pass "Dry-run mode works"
else
    print_fail "Dry-run output missing"
fi

# Test 4: Platform detection
print_test "Platform detection works"
OUTPUT=$(OPENCLAW_VERSION=1.0.0 \
         OPENCLAW_CHECKSUM=0000000000000000000000000000000000000000000000000000000000000000 \
         run_dry_run "$INSTALL_SCRIPT")

if echo "$OUTPUT" | grep -qE "openclaw-v1.0.0-(linux|darwin|windows)-(amd64|arm64)"; then
    print_pass "Platform detected correctly"
else
    print_fail "Platform detection issue"
fi

# Test 5: Custom install directory
print_test "Custom INSTALL_DIR is respected"
OUTPUT=$(OPENCLAW_VERSION=1.0.0 \
         OPENCLAW_CHECKSUM=0000000000000000000000000000000000000000000000000000000000000000 \
         INSTALL_DIR=/opt/custom \
         run_dry_run "$INSTALL_SCRIPT")

if echo "$OUTPUT" | grep -q "/opt/custom/openclaw"; then
    print_pass "Custom install directory works"
else
    print_fail "Custom install directory not respected"
fi

# Test 6: Version override
print_test "OPENCLAW_VERSION override works"
OUTPUT=$(OPENCLAW_VERSION=2.0.0-beta \
         OPENCLAW_CHECKSUM=0000000000000000000000000000000000000000000000000000000000000000 \
         run_dry_run "$INSTALL_SCRIPT")

if echo "$OUTPUT" | grep -q "openclaw-v2.0.0-beta-"; then
    print_pass "Version override works"
else
    print_fail "Version override not working"
fi

# Test 7: Architecture normalization
print_test "Architecture names are normalized"
# We can't easily override uname output, but we can check the script logic
if grep -q 'x86_64) ARCH="amd64"' "$INSTALL_SCRIPT" && \
   grep -q 'aarch64) ARCH="arm64"' "$INSTALL_SCRIPT"; then
    print_pass "Architecture normalization present"
else
    print_fail "Architecture normalization missing"
fi

# Test 8: Checksum verification logic
print_test "Checksum verification is implemented"
if grep -q "Checksum mismatch" "$INSTALL_SCRIPT" && \
   grep -q "ACTUAL.*EXPECTED" "$INSTALL_SCRIPT"; then
    print_pass "Checksum verification implemented"
else
    print_fail "Checksum verification incomplete"
fi

# Test 9: Error handling for missing utilities
print_test "Handles missing checksum utilities gracefully"
if grep -q "No SHA-256 checksum utility found" "$INSTALL_SCRIPT"; then
    print_pass "Checksum utility check present"
else
    print_fail "Missing checksum utility check"
fi

# Test 10: Shell compatibility (no bashisms)
print_test "Script is POSIX sh compatible"
if command -v shellcheck >/dev/null 2>&1; then
    if shellcheck -s sh "$INSTALL_SCRIPT" 2>&1 | grep -qE "(error|warning)"; then
        SHELLCHECK_OUTPUT=$(shellcheck -s sh "$INSTALL_SCRIPT" 2>&1)
        print_fail "ShellCheck found issues:"
        echo "$SHELLCHECK_OUTPUT"
    else
        print_pass "ShellCheck passed"
    fi
else
    # Run basic bashism check manually
    # Note: $(…) is POSIX-compliant, not a bashism
    # Real bashisms: [[, function keyword, == in tests, $'...'
    if grep -qE '(\[\[|function [a-z]|== )' "$INSTALL_SCRIPT"; then
        print_fail "Potential bashisms detected"
    else
        print_pass "No obvious bashisms (shellcheck not available for full check)"
    fi
fi

# Summary
echo ""
echo "================================"
echo "Test Summary"
echo "================================"
echo "Total:  $TESTS_RUN"
echo "Passed: $TESTS_PASSED"
echo "Failed: $TESTS_FAILED"
echo "================================"

if [ "$TESTS_FAILED" -eq 0 ]; then
    printf '%sAll tests passed!%s\n' "$GREEN" "$NC"
    exit 0
else
    printf '%sSome tests failed%s\n' "$RED" "$NC"
    exit 1
fi
