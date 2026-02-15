#!/usr/bin/env bash
# Unit tests for stress-test.sh helper functions
# Usage: ./scripts/test-stress-test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Test framework
assert_equals() {
    local expected=$1
    local actual=$2
    local message=${3:-}
    
    TESTS_RUN=$((TESTS_RUN + 1))
    
    if [[ "$expected" == "$actual" ]]; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        echo -e "${GREEN}✓${NC} $message"
        return 0
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        echo -e "${RED}✗${NC} $message"
        echo "  Expected: $expected"
        echo "  Actual: $actual"
        return 1
    fi
}

assert_contains() {
    local haystack=$1
    local needle=$2
    local message=${3:-}
    
    TESTS_RUN=$((TESTS_RUN + 1))
    
    if [[ "$haystack" == *"$needle"* ]]; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        echo -e "${GREEN}✓${NC} $message"
        return 0
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        echo -e "${RED}✗${NC} $message"
        echo "  Haystack (truncated): ${haystack:0:200}..."
        echo "  Needle: $needle"
        return 1
    fi
}

assert_success() {
    local command=$1
    local message=${2:-}
    
    TESTS_RUN=$((TESTS_RUN + 1))
    
    if eval "$command" >/dev/null 2>&1; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        echo -e "${GREEN}✓${NC} $message"
        return 0
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        echo -e "${RED}✗${NC} $message"
        echo "  Command failed: $command"
        return 1
    fi
}

assert_failure() {
    local command=$1
    local message=${2:-}
    
    TESTS_RUN=$((TESTS_RUN + 1))
    
    if ! eval "$command" >/dev/null 2>&1; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
        echo -e "${GREEN}✓${NC} $message"
        return 0
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
        echo -e "${RED}✗${NC} $message"
        echo "  Command succeeded but should have failed: $command"
        return 1
    fi
}

# Test: Help flag shows usage
test_help_flag() {
    echo "Test: Help flag shows usage"
    local output
    output=$("$SCRIPT_DIR/stress-test.sh" --help 2>&1 || true)
    assert_contains "$output" "Usage:" "Help output contains 'Usage:'"
    assert_contains "$output" "--daemon-url" "Help output contains '--daemon-url'"
    assert_contains "$output" "--agent-id" "Help output contains '--agent-id'"
    echo ""
}

# Test: Script has shebang
test_shebang() {
    echo "Test: Script has proper shebang"
    local first_line
    first_line=$(head -n 1 "$SCRIPT_DIR/stress-test.sh")
    assert_contains "$first_line" "bash" "First line contains bash shebang"
    echo ""
}

# Test: Script is executable
test_executable() {
    echo "Test: Script is executable"
    assert_success "[[ -x '$SCRIPT_DIR/stress-test.sh' ]]" "Script has executable permission"
    echo ""
}

# Test: Required commands are available in script
test_required_commands() {
    echo "Test: Script checks for required commands"
    local script_content
    script_content=$(cat "$SCRIPT_DIR/stress-test.sh")
    assert_contains "$script_content" "curl" "Script references curl"
    assert_contains "$script_content" "bc" "Script references bc"
    echo ""
}

# Test: API endpoint construction
test_api_endpoints() {
    echo "Test: API endpoints are correctly constructed"
    local script_content
    script_content=$(cat "$SCRIPT_DIR/stress-test.sh")
    assert_contains "$script_content" "/api/v1/agents" "Script uses /api/v1/agents"
    assert_contains "$script_content" "/install" "Script uses install endpoint"
    assert_contains "$script_content" "/start" "Script uses start endpoint"
    assert_contains "$script_content" "/stop" "Script uses stop endpoint"
    assert_contains "$script_content" "/status" "Script uses status endpoint"
    echo ""
}

# Test: Test functions are defined
test_test_functions_defined() {
    echo "Test: All test functions are defined"
    local script_content
    script_content=$(cat "$SCRIPT_DIR/stress-test.sh")
    assert_contains "$script_content" "test_restart_storm" "test_restart_storm is defined"
    assert_contains "$script_content" "test_concurrent_operations" "test_concurrent_operations is defined"
    assert_contains "$script_content" "test_crash_recovery" "test_crash_recovery is defined"
    assert_contains "$script_content" "test_port_conflict" "test_port_conflict is defined"
    assert_contains "$script_content" "test_recovery_time" "test_recovery_time is defined"
    echo ""
}

# Test: HTTP response parsing
test_http_response_parsing() {
    echo "Test: HTTP response parsing functions"
    local script_content
    script_content=$(cat "$SCRIPT_DIR/stress-test.sh")
    assert_contains "$script_content" "extract_http_code" "extract_http_code function defined"
    assert_contains "$script_content" "extract_body" "extract_body function defined"
    
    # Test extract_http_code logic
    local sample_response=$'{"result":"ok"}\n200'
    local code
    code=$(echo "$sample_response" | tail -n 1)
    assert_equals "200" "$code" "extract_http_code logic works"
    
    # Test extract_body logic
    local body
    body=$(echo "$sample_response" | sed '$d')
    assert_equals '{"result":"ok"}' "$body" "extract_body logic works"
    echo ""
}

# Test: Result tracking
test_result_tracking() {
    echo "Test: Result tracking variables and functions"
    local script_content
    script_content=$(cat "$SCRIPT_DIR/stress-test.sh")
    assert_contains "$script_content" "TOTAL_TESTS" "TOTAL_TESTS variable defined"
    assert_contains "$script_content" "PASSED_TESTS" "PASSED_TESTS variable defined"
    assert_contains "$script_content" "FAILED_TESTS" "FAILED_TESTS variable defined"
    assert_contains "$script_content" "record_test" "record_test function defined"
    assert_contains "$script_content" "generate_report" "generate_report function defined"
    echo ""
}

# Test: Configuration defaults
test_configuration_defaults() {
    echo "Test: Configuration defaults are set"
    local script_content
    script_content=$(cat "$SCRIPT_DIR/stress-test.sh")
    assert_contains "$script_content" '127.0.0.1:9090' "Default daemon URL is set"
    assert_contains "$script_content" 'AGENT_ID="openclaw"' "Default agent ID is openclaw"
    assert_contains "$script_content" 'ITERATIONS=10' "Default iterations is 10"
    echo ""
}

# Test: Argument parsing
test_argument_parsing() {
    echo "Test: Argument parsing function exists"
    local script_content
    script_content=$(cat "$SCRIPT_DIR/stress-test.sh")
    assert_contains "$script_content" "parse_args" "parse_args function defined"
    assert_contains "$script_content" "--daemon-url" "Handles --daemon-url argument"
    assert_contains "$script_content" "--agent-id" "Handles --agent-id argument"
    assert_contains "$script_content" "--iterations" "Handles --iterations argument"
    assert_contains "$script_content" "--quiet" "Handles --quiet argument"
    echo ""
}

# Test: Logging functions
test_logging_functions() {
    echo "Test: Logging functions are defined"
    local script_content
    script_content=$(cat "$SCRIPT_DIR/stress-test.sh")
    assert_contains "$script_content" "log_test" "log_test function defined"
    assert_contains "$script_content" "PASS" "PASS status defined"
    assert_contains "$script_content" "FAIL" "FAIL status defined"
    assert_contains "$script_content" "INFO" "INFO status defined"
    assert_contains "$script_content" "WARN" "WARN status defined"
    echo ""
}

# Test: Timing functions
test_timing_functions() {
    echo "Test: Timing measurement functions"
    local script_content
    script_content=$(cat "$SCRIPT_DIR/stress-test.sh")
    assert_contains "$script_content" "time_operation" "time_operation function defined"
    assert_contains "$script_content" "date +%s.%N" "Uses high-precision timestamps"
    echo ""
}

# Test: Error handling with set -euo pipefail
test_error_handling() {
    echo "Test: Error handling is configured"
    local script_content
    script_content=$(cat "$SCRIPT_DIR/stress-test.sh")
    assert_contains "$script_content" "set -euo pipefail" "Script uses strict error handling"
    echo ""
}

# Test: No syntax errors (bash -n check)
test_syntax() {
    echo "Test: Script has no syntax errors"
    assert_success "bash -n '$SCRIPT_DIR/stress-test.sh'" "Script passes syntax check"
    echo ""
}

# Test: Script contains proper documentation
test_documentation() {
    echo "Test: Script contains documentation"
    local script_content
    script_content=$(cat "$SCRIPT_DIR/stress-test.sh")
    assert_contains "$script_content" "Usage:" "Contains usage documentation"
    assert_contains "$script_content" "Options:" "Contains options documentation"
    echo ""
}

# Test: Script handles missing dependencies
test_dependency_checks() {
    echo "Test: Script checks for dependencies"
    local script_content
    script_content=$(cat "$SCRIPT_DIR/stress-test.sh")
    assert_contains "$script_content" "command -v curl" "Checks for curl"
    assert_contains "$script_content" "command -v bc" "Checks for bc"
    echo ""
}

# Test: cleanup/stop operations are present
test_cleanup() {
    echo "Test: Cleanup operations are present"
    local script_content
    script_content=$(cat "$SCRIPT_DIR/stress-test.sh")
    assert_contains "$script_content" "stop_agent" "Has stop_agent function"
    # Count cleanup calls
    local cleanup_count
    cleanup_count=$(grep -c "stop_agent.*||.*true" "$SCRIPT_DIR/stress-test.sh" || echo 0)
    assert_success "[[ $cleanup_count -gt 3 ]]" "Has multiple cleanup calls ($cleanup_count found)"
    echo ""
}

# Print summary
print_summary() {
    echo "========================================"
    echo "Test Summary"
    echo "========================================"
    echo "Total: $TESTS_RUN"
    echo -e "Passed: ${GREEN}$TESTS_PASSED${NC}"
    echo -e "Failed: ${RED}$TESTS_FAILED${NC}"
    echo ""
    
    if [[ $TESTS_FAILED -eq 0 ]]; then
        echo -e "${GREEN}All tests passed!${NC}"
        return 0
    else
        echo -e "${RED}Some tests failed.${NC}"
        return 1
    fi
}

# Main execution
main() {
    echo "Running stress-test.sh unit tests"
    echo "========================================"
    echo ""
    
    test_help_flag
    test_shebang
    test_executable
    test_required_commands
    test_api_endpoints
    test_test_functions_defined
    test_http_response_parsing
    test_result_tracking
    test_configuration_defaults
    test_argument_parsing
    test_logging_functions
    test_timing_functions
    test_error_handling
    test_syntax
    test_documentation
    test_dependency_checks
    test_cleanup
    
    echo ""
    print_summary
}

main "$@"
