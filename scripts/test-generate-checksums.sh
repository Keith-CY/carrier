#!/usr/bin/env bash
#
# test-generate-checksums.sh - Test suite for generate-checksums.sh
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GENERATE_SCRIPT="$SCRIPT_DIR/generate-checksums.sh"

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
pass() {
    echo -e "${GREEN}✓ PASS:${NC} $1"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

fail() {
    echo -e "${RED}✗ FAIL:${NC} $1"
    TESTS_FAILED=$((TESTS_FAILED + 1))
}

info() {
    echo -e "${YELLOW}→${NC} $1"
}

cleanup() {
    if [ -d "$TEST_DIR" ]; then
        rm -rf "$TEST_DIR"
    fi
}

# Create temporary test directory
TEST_DIR=$(mktemp -d)
trap cleanup EXIT

echo "======================================"
echo "Testing generate-checksums.sh"
echo "======================================"
echo "Test directory: $TEST_DIR"
echo ""

# Test 1: Script exists and is executable
info "Test 1: Script exists and is executable"
TESTS_RUN=$((TESTS_RUN + 1))
if [ ! -f "$GENERATE_SCRIPT" ]; then
    fail "Script not found at $GENERATE_SCRIPT"
elif [ ! -x "$GENERATE_SCRIPT" ]; then
    fail "Script is not executable (run: chmod +x $GENERATE_SCRIPT)"
else
    pass "Script exists and is executable"
fi

# Test 2: No arguments should fail
info "Test 2: No arguments should exit with error"
TESTS_RUN=$((TESTS_RUN + 1))
if "$GENERATE_SCRIPT" 2>/dev/null; then
    fail "Script should fail when no directory is provided"
else
    pass "Script correctly rejects missing directory argument"
fi

# Test 3: Invalid directory should fail
info "Test 3: Invalid directory should exit with error"
TESTS_RUN=$((TESTS_RUN + 1))
if "$GENERATE_SCRIPT" /nonexistent/directory 2>/dev/null; then
    fail "Script should fail for nonexistent directory"
else
    pass "Script correctly rejects invalid directory"
fi

# Test 4: Generate checksums for test files
info "Test 4: Generate checksums for test files"
TESTS_RUN=$((TESTS_RUN + 1))
mkdir -p "$TEST_DIR/artifacts"
echo "test content 1" > "$TEST_DIR/artifacts/file1.txt"
echo "test content 2" > "$TEST_DIR/artifacts/file2.bin"
echo "test content 3" > "$TEST_DIR/artifacts/release.tar.gz"

"$GENERATE_SCRIPT" "$TEST_DIR/artifacts" > /dev/null

if [ -f "$TEST_DIR/artifacts/file1.txt.sha256" ] && \
   [ -f "$TEST_DIR/artifacts/file2.bin.sha256" ] && \
   [ -f "$TEST_DIR/artifacts/release.tar.gz.sha256" ]; then
    pass "Checksum files generated for all test files"
else
    fail "Not all checksum files were generated"
fi

# Test 5: Verify checksum format
info "Test 5: Verify checksum format"
TESTS_RUN=$((TESTS_RUN + 1))
CHECKSUM_FILE="$TEST_DIR/artifacts/file1.txt.sha256"
CONTENT=$(cat "$CHECKSUM_FILE")

# SHA256 hash should be 64 hex characters followed by two spaces and filename
if [[ "$CONTENT" =~ ^[a-f0-9]{64}\ {2}file1\.txt$ ]]; then
    pass "Checksum file has correct format"
else
    fail "Checksum format is incorrect: $CONTENT"
fi

# Test 6: Verify checksum is correct
info "Test 6: Verify checksum accuracy"
TESTS_RUN=$((TESTS_RUN + 1))
EXPECTED_HASH=$(cat "$CHECKSUM_FILE" | cut -d' ' -f1)

# Detect SHA command
if command -v sha256sum &> /dev/null; then
    ACTUAL_HASH=$(sha256sum "$TEST_DIR/artifacts/file1.txt" | cut -d' ' -f1)
elif command -v shasum &> /dev/null; then
    ACTUAL_HASH=$(shasum -a 256 "$TEST_DIR/artifacts/file1.txt" | cut -d' ' -f1)
else
    fail "No SHA256 command available to verify"
    ACTUAL_HASH=""
fi

if [ "$EXPECTED_HASH" = "$ACTUAL_HASH" ]; then
    pass "Checksum is accurate"
else
    fail "Checksum mismatch: expected $EXPECTED_HASH, got $ACTUAL_HASH"
fi

# Test 7: Skip existing checksums
info "Test 7: Skip files with existing checksums"
TESTS_RUN=$((TESTS_RUN + 1))
# Modify the checksum file
echo "modified content" > "$CHECKSUM_FILE"
BEFORE=$(cat "$CHECKSUM_FILE")

# Run script again
"$GENERATE_SCRIPT" "$TEST_DIR/artifacts" > /dev/null
AFTER=$(cat "$CHECKSUM_FILE")

if [ "$BEFORE" = "$AFTER" ]; then
    pass "Existing checksum files are skipped"
else
    fail "Existing checksum file was overwritten"
fi

# Test 8: Skip .sha256 files themselves
info "Test 8: Don't generate checksums for .sha256 files"
TESTS_RUN=$((TESTS_RUN + 1))
if [ -f "$TEST_DIR/artifacts/file1.txt.sha256.sha256" ]; then
    fail "Script generated checksum for .sha256 file"
else
    pass "Script correctly skips .sha256 files"
fi

# Test 9: Empty directory
info "Test 9: Handle empty directory gracefully"
TESTS_RUN=$((TESTS_RUN + 1))
mkdir -p "$TEST_DIR/empty"
if "$GENERATE_SCRIPT" "$TEST_DIR/empty" > /dev/null 2>&1; then
    pass "Empty directory handled gracefully"
else
    fail "Script failed on empty directory"
fi

# Summary
echo ""
echo "======================================"
echo "Test Summary"
echo "======================================"
echo "Tests run:    $TESTS_RUN"
echo -e "${GREEN}Passed:       $TESTS_PASSED${NC}"
if [ $TESTS_FAILED -gt 0 ]; then
    echo -e "${RED}Failed:       $TESTS_FAILED${NC}"
    echo ""
    exit 1
else
    echo "Failed:       $TESTS_FAILED"
    echo ""
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
fi
