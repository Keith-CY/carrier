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

# Default configuration
DAEMON_URL="${CARRIER_DAEMON_URL:-http://127.0.0.1:9090}"
AGENT_ID="openclaw"
ITERATIONS=10
QUIET=0
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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

# Test 1: Restart storm - rapid start/stop cycles
test_restart_storm() {
    log_test INFO "Test 1: Restart Storm (${ITERATIONS}x rapid start/stop)"
    
    local failures=0
    local total_start_time=0
    local total_stop_time=0
    
    for i in $(seq 1 "$ITERATIONS"); do
        log "  Iteration $i/$ITERATIONS"
        
        # Start
        local start_time
        if start_time=$(time_operation start_agent); then
            total_start_time=$(echo "$total_start_time + $start_time" | bc)
            log "    Start: ${start_time}s"
        else
            log_test WARN "    Start failed on iteration $i"
            failures=$((failures + 1))
        fi
        
        sleep 0.5
        
        # Stop
        local stop_time
        if stop_time=$(time_operation stop_agent); then
            total_stop_time=$(echo "$total_stop_time + $stop_time" | bc)
            log "    Stop: ${stop_time}s"
        else
            log_test WARN "    Stop failed on iteration $i"
            failures=$((failures + 1))
        fi
        
        sleep 0.5
    done
    
    local avg_start
    avg_start=$(echo "scale=3; $total_start_time / $ITERATIONS" | bc)
    local avg_stop
    avg_stop=$(echo "scale=3; $total_stop_time / $ITERATIONS" | bc)
    
    if [[ $failures -eq 0 ]]; then
        log_test PASS "Restart storm: 0 failures, avg start=${avg_start}s, avg stop=${avg_stop}s"
        record_test "restart_storm" "PASS" "avg_start=${avg_start}s avg_stop=${avg_stop}s"
    else
        log_test FAIL "Restart storm: $failures failures out of $((ITERATIONS * 2)) operations"
        record_test "restart_storm" "FAIL" "failures=$failures"
    fi
}

# Test 2: Concurrent operations - parallel start attempts
test_concurrent_operations() {
    log_test INFO "Test 2: Concurrent Operations (parallel start attempts)"
    
    # Stop agent first
    stop_agent >/dev/null 2>&1 || true
    sleep 1
    
    local pids=()
    local success_count=0
    local temp_dir
    temp_dir=$(mktemp -d)
    
    # Launch 5 parallel start requests
    for i in $(seq 1 5); do
        (
            if start_agent; then
                echo "SUCCESS" > "$temp_dir/result_$i"
            else
                echo "FAIL" > "$temp_dir/result_$i"
            fi
        ) &
        pids+=($!)
    done
    
    # Wait for all operations
    for pid in "${pids[@]}"; do
        wait "$pid" 2>/dev/null || true
    done
    
    # Count successes
    for i in $(seq 1 5); do
        if [[ -f "$temp_dir/result_$i" ]] && grep -q "SUCCESS" "$temp_dir/result_$i"; then
            success_count=$((success_count + 1))
        fi
    done
    
    rm -rf "$temp_dir"
    
    # At least one should succeed, others should handle gracefully
    if [[ $success_count -ge 1 ]]; then
        log_test PASS "Concurrent operations: $success_count/5 succeeded (expected >= 1)"
        record_test "concurrent_operations" "PASS" "success_count=$success_count"
    else
        log_test FAIL "Concurrent operations: 0/5 succeeded"
        record_test "concurrent_operations" "FAIL" "success_count=0"
    fi
    
    # Cleanup
    stop_agent >/dev/null 2>&1 || true
}

# Test 3: Crash recovery - kill -9 and verify auto-restart
test_crash_recovery() {
    log_test INFO "Test 3: Crash Recovery (kill -9 + auto-restart verification)"
    
    # Start agent
    if ! start_agent; then
        log_test FAIL "Crash recovery: Could not start agent for testing"
        record_test "crash_recovery" "FAIL" "could_not_start"
        return
    fi
    
    sleep 2
    
    # Find agent process
    local agent_pid
    agent_pid=$(pgrep -f "$AGENT_ID" | head -n 1 || echo "")
    
    if [[ -z "$agent_pid" ]]; then
        log_test WARN "Crash recovery: Could not find agent process (may be in container)"
        record_test "crash_recovery" "SKIP" "process_not_found"
        stop_agent >/dev/null 2>&1 || true
        return
    fi
    
    log "  Found agent PID: $agent_pid"
    
    # Kill with SIGKILL
    kill -9 "$agent_pid" 2>/dev/null || true
    log "  Sent SIGKILL to agent"
    
    sleep 2
    
    # Check if auto-restart occurred
    local status
    status=$(get_agent_status)
    
    if echo "$status" | grep -q '"health":"healthy"'; then
        log_test PASS "Crash recovery: Agent auto-restarted successfully"
        record_test "crash_recovery" "PASS" "auto_restart_verified"
    else
        log_test FAIL "Crash recovery: Agent did not auto-restart"
        record_test "crash_recovery" "FAIL" "no_auto_restart"
    fi
    
    # Cleanup
    stop_agent >/dev/null 2>&1 || true
}

# Test 4: Port conflict simulation
test_port_conflict() {
    log_test INFO "Test 4: Port Conflict (start on occupied port)"
    
    # This test checks if daemon properly detects and reports port conflicts
    # We can't easily simulate this without knowing the agent's port configuration
    # So we test the error handling path
    
    # Start agent normally first
    if ! start_agent; then
        log_test FAIL "Port conflict: Could not start agent for baseline"
        record_test "port_conflict" "FAIL" "baseline_start_failed"
        return
    fi
    
    sleep 2
    
    # Try to start again (should fail or be idempotent)
    local response
    response=$(api_call POST "/api/v1/agents/$AGENT_ID/start")
    local code
    code=$(echo "$response" | extract_http_code)
    local body
    body=$(echo "$response" | extract_body)
    
    # Either 200 (idempotent) or error (already running) is acceptable
    if [[ "$code" == "200" ]] || [[ "$code" == "409" ]] || [[ "$code" == "400" ]]; then
        log_test PASS "Port conflict handling: API responded appropriately (HTTP $code)"
        record_test "port_conflict" "PASS" "http_code=$code"
    else
        log_test FAIL "Port conflict handling: Unexpected response (HTTP $code)"
        record_test "port_conflict" "FAIL" "http_code=$code"
    fi
    
    # Cleanup
    stop_agent >/dev/null 2>&1 || true
}

# Test 5: Recovery time measurement
test_recovery_time() {
    log_test INFO "Test 5: Recovery Time Measurement"
    
    local recovery_times=()
    local failures=0
    
    for i in $(seq 1 5); do
        log "  Iteration $i/5"
        
        # Stop agent
        if ! stop_agent; then
            log_test WARN "    Stop failed"
            failures=$((failures + 1))
            continue
        fi
        
        sleep 1
        
        # Measure restart time
        local start_ts
        start_ts=$(date +%s.%N)
        
        if ! start_agent; then
            log_test WARN "    Start failed"
            failures=$((failures + 1))
            continue
        fi
        
        # Wait for healthy status
        local recovered=0
        for _ in $(seq 1 30); do
            local status
            status=$(get_agent_status)
            if echo "$status" | grep -q '"health":"healthy"'; then
                recovered=1
                break
            fi
            sleep 0.5
        done
        
        if [[ $recovered -eq 1 ]]; then
            local end_ts
            end_ts=$(date +%s.%N)
            local recovery_time
            recovery_time=$(echo "$end_ts - $start_ts" | bc)
            recovery_times+=("$recovery_time")
            log "    Recovery time: ${recovery_time}s"
        else
            log_test WARN "    Failed to reach healthy state"
            failures=$((failures + 1))
        fi
        
        sleep 1
    done
    
    if [[ ${#recovery_times[@]} -gt 0 ]]; then
        local total=0
        for time in "${recovery_times[@]}"; do
            total=$(echo "$total + $time" | bc)
        done
        local avg
        avg=$(echo "scale=3; $total / ${#recovery_times[@]}" | bc)
        
        log_test PASS "Recovery time: avg=${avg}s, samples=${#recovery_times[@]}, failures=$failures"
        record_test "recovery_time" "PASS" "avg=${avg}s samples=${#recovery_times[@]} failures=$failures"
    else
        log_test FAIL "Recovery time: No successful recoveries measured"
        record_test "recovery_time" "FAIL" "no_data"
    fi
    
    # Cleanup
    stop_agent >/dev/null 2>&1 || true
}

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
