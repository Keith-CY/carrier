#!/bin/sh
# Test suite for install.sh
# Runs the install script in DRY_RUN mode with various configurations.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_SCRIPT="${SCRIPT_DIR}/install.sh"
FAKE_SHA="1111111111111111111111111111111111111111"

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

# Test 2: Dry-run with explicit SHA succeeds
print_test "Dry-run with explicit CARRIER_SHA succeeds"
if OUTPUT=$(CARRIER_SHA="$FAKE_SHA" run_dry_run "$INSTALL_SCRIPT"); then
    if echo "$OUTPUT" | grep -q "\[DRY_RUN\]"; then
        print_pass "Dry-run mode works"
    else
        print_fail "Dry-run output missing"
    fi
else
    print_fail "Dry-run should succeed with explicit CARRIER_SHA"
fi

# Test 3: SHA resolves to main tag + expected asset naming
print_test "Constructs main-<sha> release asset URL"
EXPECTED_URL="https://github.com/Keith-CY/carrier/releases/download/main-${FAKE_SHA}/carrier-main-${FAKE_SHA}-linux-x64.zip"
if OUTPUT=$(CARRIER_SHA="$FAKE_SHA" CARRIER_LABEL=linux-x64 run_dry_run "$INSTALL_SCRIPT"); then
    if echo "$OUTPUT" | grep -q "$EXPECTED_URL"; then
        print_pass "Main tag URL format is correct"
    else
        print_fail "Main tag URL format is incorrect"
    fi
else
    print_fail "Dry-run should succeed for URL check"
fi

# Test 4: Custom INSTALL_DIR is respected
print_test "Custom INSTALL_DIR is respected"
if OUTPUT=$(CARRIER_SHA="$FAKE_SHA" \
            INSTALL_DIR=/opt/custom \
            run_dry_run "$INSTALL_SCRIPT"); then
    if echo "$OUTPUT" | grep -q "/opt/custom/carrier"; then
        print_pass "Custom install directory works"
    else
        print_fail "Custom install directory not respected"
    fi
else
    print_fail "Dry-run should succeed with custom INSTALL_DIR"
fi

# Test 5: Custom CARRIER_LABEL is respected
print_test "Custom CARRIER_LABEL is respected"
if OUTPUT=$(CARRIER_SHA="$FAKE_SHA" \
            CARRIER_LABEL=linux-arm64 \
            run_dry_run "$INSTALL_SCRIPT"); then
    if echo "$OUTPUT" | grep -q "carrier-main-${FAKE_SHA}-linux-arm64.zip"; then
        print_pass "Custom label works"
    else
        print_fail "Custom label not respected"
    fi
else
    print_fail "Dry-run should succeed with custom CARRIER_LABEL"
fi

# Test 6: Explicit CARRIER_TAG override works
print_test "Explicit CARRIER_TAG override works"
if OUTPUT=$(CARRIER_TAG="main-${FAKE_SHA}" CARRIER_LABEL=linux-x64 run_dry_run "$INSTALL_SCRIPT"); then
    if echo "$OUTPUT" | grep -q "releases/download/main-${FAKE_SHA}/carrier-main-${FAKE_SHA}-linux-x64.zip"; then
        print_pass "Tag override works"
    else
        print_fail "Tag override not working"
    fi
else
    print_fail "Dry-run should succeed with explicit CARRIER_TAG"
fi

# Test 7: CARRIER_TAG and CARRIER_SHA conflict fails
print_test "CARRIER_TAG + CARRIER_SHA conflict is rejected"
if CARRIER_TAG="main-${FAKE_SHA}" CARRIER_SHA="$FAKE_SHA" run_dry_run "$INSTALL_SCRIPT" | grep -q "cannot both be set"; then
    print_pass "Conflicting inputs rejected"
else
    print_fail "Conflicting inputs should be rejected"
fi

# Test 8: Custom CARRIER_REPO is respected
print_test "Custom CARRIER_REPO is respected"
if OUTPUT=$(CARRIER_SHA="$FAKE_SHA" \
            CARRIER_REPO="octo/example" \
            run_dry_run "$INSTALL_SCRIPT"); then
    if echo "$OUTPUT" | grep -q "https://github.com/octo/example/releases/download/main-${FAKE_SHA}"; then
        print_pass "Custom repo works"
    else
        print_fail "Custom repo not respected"
    fi
else
    print_fail "Dry-run should succeed with custom CARRIER_REPO"
fi

# Test 9: Checksum verification logic exists
print_test "Checksum verification logic is implemented"
if grep -q "checksum mismatch" "$INSTALL_SCRIPT" && \
   grep -q "sha256sum -c" "$INSTALL_SCRIPT"; then
    print_pass "Checksum verification implemented"
else
    print_fail "Checksum verification incomplete"
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
