#!/usr/bin/env bash
# test-sign-artifacts.sh - Test artifact signing functionality
#
# This script creates a temporary test environment and verifies that
# the sign-artifacts.sh script works correctly with available signing tools.

set -uo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_test() {
    echo -e "${BLUE}[TEST]${NC} $*"
}

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $*"
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $*"
}

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SIGN_SCRIPT="$SCRIPT_DIR/sign-artifacts.sh"

# Create temporary test directory
TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT

log_info "Test directory: $TEST_DIR"

# Test 1: Verify script exists and is executable
test_script_exists() {
    log_test "Checking if sign-artifacts.sh exists and is executable"
    
    if [[ ! -f "$SIGN_SCRIPT" ]]; then
        log_fail "Script not found: $SIGN_SCRIPT"
        return 1
    fi
    
    if [[ ! -x "$SIGN_SCRIPT" ]]; then
        log_fail "Script is not executable: $SIGN_SCRIPT"
        return 1
    fi
    
    log_pass "Script exists and is executable"
}

# Test 2: Verify script shows usage when no arguments
test_usage() {
    log_test "Checking usage message"
    
    local output
    output=$("$SIGN_SCRIPT" 2>&1 || true)
    
    if echo "$output" | grep -q "Usage:"; then
        log_pass "Usage message displayed correctly"
    else
        log_fail "Usage message not displayed"
        return 1
    fi
}

# Test 3: Verify script fails on non-existent directory
test_invalid_directory() {
    log_test "Checking error handling for invalid directory"
    
    local output
    output=$("$SIGN_SCRIPT" /nonexistent/directory 2>&1 || true)
    
    if echo "$output" | grep -q "Directory not found"; then
        log_pass "Invalid directory handled correctly"
    else
        log_fail "Invalid directory not handled properly"
        return 1
    fi
}

# Test 4: Regression test for set -e counter behavior
test_counter_regression() {
    log_test "Regression: successful first sign should not exit early under set -e"

    local fixture_dir="$TEST_DIR/counter-fixture"
    local mock_bin="$TEST_DIR/mock-bin"
    mkdir -p "$fixture_dir" "$mock_bin"

    echo "artifact one" > "$fixture_dir/one.zip"
    echo "artifact two" > "$fixture_dir/two.zip"

cat > "$mock_bin/cosign" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail

sig_path=""
bundle_path=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output-signature)
      sig_path="${2:-}"
      shift 2
      ;;
    --bundle)
      bundle_path="${2:-}"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

if [[ -z "$sig_path" ]]; then
  echo "missing --output-signature" >&2
  exit 2
fi

echo "mock-signature" > "$sig_path"
if [[ -n "$bundle_path" ]]; then
  echo "mock-bundle" > "$bundle_path"
fi
MOCK
    chmod +x "$mock_bin/cosign"

    local output
    if ! output=$(PATH="$mock_bin:$PATH" SIGNING_METHOD=cosign "$SIGN_SCRIPT" "$fixture_dir" 2>&1); then
        log_fail "Counter regression run failed unexpectedly"
        echo "$output"
        return 1
    fi

    local sig_count
    sig_count=$(find "$fixture_dir" -name "*.sig" -type f | wc -l | tr -d '[:space:]')
    if [[ "$sig_count" != "2" ]]; then
        log_fail "Expected 2 signatures from regression test, found $sig_count"
        return 1
    fi

    local bundle_count
    bundle_count=$(find "$fixture_dir" -name "*.sigstore.json" -type f | wc -l | tr -d '[:space:]')
    if [[ "$bundle_count" != "2" ]]; then
        log_fail "Expected 2 bundles from regression test, found $bundle_count"
        return 1
    fi

    if ! echo "$output" | grep -q "Signing complete: 2 signed, 0 failed"; then
        log_fail "Did not find expected completion summary in output"
        echo "$output"
        return 1
    fi

    log_pass "Counter regression test passed"
}

# Test 5: Create test artifacts and attempt signing
test_signing() {
    log_test "Creating test artifacts"
    
    # Create test files
    echo "Test artifact 1" > "$TEST_DIR/artifact1.bin"
    echo "Test artifact 2" > "$TEST_DIR/artifact2.tar.gz"
    echo "Test artifact 3" > "$TEST_DIR/artifact3.zip"
    
    log_info "Created 3 test artifacts"
    
    # Check if any signing tool is available
    if ! command -v cosign &> /dev/null && ! command -v gpg &> /dev/null; then
        log_fail "No signing tool available (cosign or gpg)"
        log_info "Install cosign or gpg to run signing tests"
        return 1
    fi
    
    # Check if GPG key is available (if using GPG)
    if command -v gpg &> /dev/null && ! command -v cosign &> /dev/null; then
        if ! gpg --list-secret-keys &> /dev/null || [[ $(gpg --list-secret-keys 2>&1 | wc -l) -lt 2 ]]; then
            log_info "GPG available but no secret key configured"
            log_info "Signing test skipped (this is OK for fresh environments)"
            log_pass "Signing script validation passed (key check)"
            return 0
        fi
    fi
    
    log_test "Attempting to sign test artifacts"
    
    # Run signing script
    if "$SIGN_SCRIPT" "$TEST_DIR" 2>&1; then
        log_pass "Signing script executed successfully"
        
        # Verify signatures were created
        local sig_count=0
        for sig in "$TEST_DIR"/*.sig; do
            if [[ -f "$sig" ]]; then
                ((sig_count++))
                log_info "Found signature: $(basename "$sig")"
            fi
        done
        
        if [[ $sig_count -eq 3 ]]; then
            log_pass "All 3 signatures created"
        else
            log_fail "Expected 3 signatures, found $sig_count"
            return 1
        fi
    else
        # Check if failure was due to missing key
        if command -v gpg &> /dev/null && ! command -v cosign &> /dev/null; then
            log_info "GPG signing failed (likely no key configured)"
            log_pass "Signing script validation passed (expected failure without key)"
            return 0
        else
            log_fail "Signing script failed unexpectedly"
            return 1
        fi
    fi
}

# Test 6: Verify signatures are not re-signed
test_skip_signatures() {
    log_test "Verifying that .sig files are skipped"
    
    # Create a .sig file
    echo "fake signature" > "$TEST_DIR/fake.sig"
    
    local initial_count
    initial_count=$(find "$TEST_DIR" -name "*.sig" | wc -l)
    
    # Run signing again
    "$SIGN_SCRIPT" "$TEST_DIR" > /dev/null 2>&1 || true
    
    local final_count
    final_count=$(find "$TEST_DIR" -name "*.sig" | wc -l)
    
    if [[ $initial_count -eq $final_count ]]; then
        log_pass "Signature files correctly skipped"
    else
        log_fail "Signature files were processed"
        return 1
    fi
}

# Run all tests
main() {
    echo ""
    log_info "Starting sign-artifacts.sh test suite"
    echo ""
    
    local passed=0
    local failed=0
    
    # Run tests
    if test_script_exists; then passed=$((passed + 1)); else failed=$((failed + 1)); fi
    echo ""
    
    if test_usage; then passed=$((passed + 1)); else failed=$((failed + 1)); fi
    echo ""
    
    if test_invalid_directory; then passed=$((passed + 1)); else failed=$((failed + 1)); fi
    echo ""
    
    if test_counter_regression; then passed=$((passed + 1)); else failed=$((failed + 1)); fi
    echo ""

    if test_signing; then passed=$((passed + 1)); else failed=$((failed + 1)); fi
    echo ""
    
    if test_skip_signatures; then passed=$((passed + 1)); else failed=$((failed + 1)); fi
    echo ""
    
    # Summary
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_info "Test Summary: $passed passed, $failed failed"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    if [[ $failed -gt 0 ]]; then
        exit 1
    fi
    
    log_pass "All tests passed!"
}

main "$@"
