#!/usr/bin/env bash
# Carrier Reliability Stress Test Script
# Tests frequent-failure scenarios: restart storms, port conflicts, crash recovery
# Usage: ./scripts/stress-test.sh [options]
#
# Options:
#   --daemon-url URL    Daemon API base URL (default: http://127.0.0.1:9090)
#   --agent-id ID       Agent ID to test (default: openclaw)
#   --iterations N      Number of test iterations (default: 10)
#   --quiet             Suppress verbose output
#   --help              Show this help message

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Default configuration
DAEMON_URL="${CARRIER_DAEMON_URL:-http://127.0.0.1:9090}"
AGENT_ID="openclaw"
ITERATIONS=10
QUIET=0

# Test results tracking
declare -A TEST_RESULTS
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse command-line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --daemon-url)
                DAEMON_URL="$2"
                shift 2
                ;;
            --agent-id)
                AGENT_ID="$2"
                shift 2
                ;;
            --iterations)
                ITERATIONS="$2"
                shift 2
                ;;
            --quiet)
                QUIET=1
                shift
                ;;
            --help)
                grep '^#' "$0" | sed 's/^# //' | sed 's/^#//'
                exit 0
                ;;
            *)
                echo "Unknown option: $1"
                echo "Use --help for usage information"
                exit 1
                ;;
        esac
    done
}

# Logging functions
log() {
    if [[ $QUIET -eq 0 ]]; then
        echo -e "$@"
    fi
}

log_test() {
    local status=$1
    shift
    if [[ $status == "PASS" ]]; then
        echo -e "${GREEN}✓${NC} $*"
    elif [[ $status == "FAIL" ]]; then
        echo -e "${RED}✗${NC} $*"
    elif [[ $status == "INFO" ]]; then
        echo -e "${BLUE}ℹ${NC} $*"
    elif [[ $status == "WARN" ]]; then
        echo -e "${YELLOW}⚠${NC} $*"
    fi
}

# API helper functions
api_call() {
    local method=$1
    local endpoint=$2
    local data=${3:-}
    
    local url="${DAEMON_URL}${endpoint}"
    local opts=(-s -w "\n%{http_code}" -X "$method")
    
    if [[ -n "$data" ]]; then
        opts+=(-H "Content-Type: application/json" -d "$data")
    fi
    
    curl "${opts[@]}" "$url" 2>/dev/null || echo -e "\n000"
}

extract_http_code() {
    tail -n 1
}

extract_body() {
    sed '$d'
}

# Wait for condition with timeout
wait_for() {
    local condition=$1
    local timeout=${2:-30}
    local interval=${3:-1}
    local elapsed=0
    
    while ! eval "$condition" && [[ $elapsed -lt $timeout ]]; do
        sleep "$interval"
        elapsed=$((elapsed + interval))
    done
    
    eval "$condition"
}

# Check daemon connectivity
check_daemon() {
    log_test INFO "Checking daemon connectivity at $DAEMON_URL"
    local response
    response=$(api_call GET /api/v1/agents)
    local code
    code=$(echo "$response" | extract_http_code)
    
    if [[ "$code" == "200" ]]; then
        log_test PASS "Daemon is reachable"
        return 0
    else
        log_test FAIL "Daemon not reachable (HTTP $code)"
        return 1
    fi
}

# Check if agent exists
check_agent_exists() {
    local response
    response=$(api_call GET /api/v1/agents)
    local body
    body=$(echo "$response" | extract_body)
    
    if echo "$body" | grep -q "\"id\":\"$AGENT_ID\""; then
        return 0
    else
        return 1
    fi
}

# Get agent status
get_agent_status() {
    local response
    response=$(api_call GET "/api/v1/agents/$AGENT_ID/status")
    echo "$response" | extract_body
}

# Install agent
install_agent() {
    local response
    response=$(api_call POST "/api/v1/agents/$AGENT_ID/install")
    local code
    code=$(echo "$response" | extract_http_code)
    [[ "$code" == "200" ]]
}

# Start agent
start_agent() {
    local response
    response=$(api_call POST "/api/v1/agents/$AGENT_ID/start")
    local code
    code=$(echo "$response" | extract_http_code)
    [[ "$code" == "200" ]]
}

# Stop agent
stop_agent() {
    local response
    response=$(api_call POST "/api/v1/agents/$AGENT_ID/stop")
    local code
    code=$(echo "$response" | extract_http_code)
    [[ "$code" == "200" ]]
}

# Measure operation time
time_operation() {
    local start
    start=$(date +%s.%N)
    "$@"
    local status=$?
    local end
    end=$(date +%s.%N)
    local duration
    duration=$(echo "$end - $start" | bc)
    echo "$duration"
    return $status
}

# Record test result
record_test() {
    local test_name=$1
    local status=$2
    local details=${3:-}
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    TEST_RESULTS["$test_name"]="$status|$details"
    
    if [[ "$status" == "PASS" ]]; then
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
}

# shellcheck source=lib/stress-test_cases.sh
source "$SCRIPT_DIR/lib/stress-test_cases.sh"

# Generate summary report
generate_report() {
    echo ""
    echo "========================================"
    echo "Carrier Reliability Stress Test Report"
    echo "========================================"
    echo ""
    echo "Configuration:"
    echo "  Daemon URL: $DAEMON_URL"
    echo "  Agent ID: $AGENT_ID"
    echo "  Iterations: $ITERATIONS"
    echo ""
    echo "Results Summary:"
    echo "  Total Tests: $TOTAL_TESTS"
    echo "  Passed: $PASSED_TESTS"
    echo "  Failed: $FAILED_TESTS"
    echo "  Success Rate: $(echo "scale=1; $PASSED_TESTS * 100 / $TOTAL_TESTS" | bc)%"
    echo ""
    echo "Test Details:"
    echo "----------------------------------------"
    
    for test_name in "${!TEST_RESULTS[@]}"; do
        IFS='|' read -r status details <<< "${TEST_RESULTS[$test_name]}"
        if [[ "$status" == "PASS" ]]; then
            echo -e "  ${GREEN}✓${NC} $test_name: $status ($details)"
        else
            echo -e "  ${RED}✗${NC} $test_name: $status ($details)"
        fi
    done
    
    echo "========================================"
    echo ""
    
    if [[ $FAILED_TESTS -eq 0 ]]; then
        echo -e "${GREEN}All tests passed!${NC}"
        return 0
    else
        echo -e "${RED}Some tests failed. See details above.${NC}"
        return 1
    fi
}

# Main execution
main() {
    parse_args "$@"
    
    echo "Carrier Reliability Stress Test"
    echo "================================"
    echo ""
    
    # Pre-flight checks
    if ! command -v curl >/dev/null 2>&1; then
        echo "Error: curl is required but not installed"
        exit 1
    fi
    
    if ! command -v bc >/dev/null 2>&1; then
        echo "Error: bc is required but not installed"
        exit 1
    fi
    
    if ! check_daemon; then
        echo "Error: Cannot connect to daemon at $DAEMON_URL"
        exit 1
    fi
    
    if ! check_agent_exists; then
        echo "Error: Agent '$AGENT_ID' not found in catalog"
        exit 1
    fi
    
    echo ""
    echo "Running stress tests..."
    echo ""
    
    # Run tests
    test_restart_storm
    echo ""
    
    test_concurrent_operations
    echo ""
    
    test_crash_recovery
    echo ""
    
    test_port_conflict
    echo ""
    
    test_recovery_time
    echo ""
    
    # Generate report
    generate_report
}

main "$@"
