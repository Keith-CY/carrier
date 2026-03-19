#!/usr/bin/env bash

test_restart_storm() {
    log_test INFO "Test 1: Restart Storm (${ITERATIONS}x rapid start/stop)"

    local failures=0
    local total_start_time=0
    local total_stop_time=0

    for i in $(seq 1 "$ITERATIONS"); do
        log "  Iteration $i/$ITERATIONS"

        local start_time
        if start_time=$(time_operation start_agent); then
            total_start_time=$(echo "$total_start_time + $start_time" | bc)
            log "    Start: ${start_time}s"
        else
            log_test WARN "    Start failed on iteration $i"
            failures=$((failures + 1))
        fi

        sleep 0.5

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

test_concurrent_operations() {
    log_test INFO "Test 2: Concurrent Operations (parallel start attempts)"

    stop_agent >/dev/null 2>&1 || true
    sleep 1

    local pids=()
    local success_count=0
    local temp_dir
    temp_dir=$(mktemp -d)

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

    for pid in "${pids[@]}"; do
        wait "$pid" 2>/dev/null || true
    done

    for i in $(seq 1 5); do
        if [[ -f "$temp_dir/result_$i" ]] && grep -q "SUCCESS" "$temp_dir/result_$i"; then
            success_count=$((success_count + 1))
        fi
    done

    rm -rf "$temp_dir"

    if [[ $success_count -ge 1 ]]; then
        log_test PASS "Concurrent operations: $success_count/5 succeeded (expected >= 1)"
        record_test "concurrent_operations" "PASS" "success_count=$success_count"
    else
        log_test FAIL "Concurrent operations: 0/5 succeeded"
        record_test "concurrent_operations" "FAIL" "success_count=0"
    fi

    stop_agent >/dev/null 2>&1 || true
}

test_crash_recovery() {
    log_test INFO "Test 3: Crash Recovery (kill -9 + auto-restart verification)"

    if ! start_agent; then
        log_test FAIL "Crash recovery: Could not start agent for testing"
        record_test "crash_recovery" "FAIL" "could_not_start"
        return
    fi

    sleep 2

    local agent_pid
    agent_pid=$(pgrep -f "$AGENT_ID" | head -n 1 || echo "")

    if [[ -z "$agent_pid" ]]; then
        log_test WARN "Crash recovery: Could not find agent process (may be in container)"
        record_test "crash_recovery" "SKIP" "process_not_found"
        stop_agent >/dev/null 2>&1 || true
        return
    fi

    log "  Found agent PID: $agent_pid"
    kill -9 "$agent_pid" 2>/dev/null || true
    log "  Sent SIGKILL to agent"

    sleep 2

    local status
    status=$(get_agent_status)

    if echo "$status" | grep -q '"health":"healthy"'; then
        log_test PASS "Crash recovery: Agent auto-restarted successfully"
        record_test "crash_recovery" "PASS" "auto_restart_verified"
    else
        log_test FAIL "Crash recovery: Agent did not auto-restart"
        record_test "crash_recovery" "FAIL" "no_auto_restart"
    fi

    stop_agent >/dev/null 2>&1 || true
}

test_port_conflict() {
    log_test INFO "Test 4: Port Conflict (start on occupied port)"

    if ! start_agent; then
        log_test FAIL "Port conflict: Could not start agent for baseline"
        record_test "port_conflict" "FAIL" "baseline_start_failed"
        return
    fi

    sleep 2

    local response
    response=$(api_call POST "/api/v1/agents/$AGENT_ID/start")
    local code
    code=$(echo "$response" | extract_http_code)

    if [[ "$code" == "200" ]] || [[ "$code" == "409" ]] || [[ "$code" == "400" ]]; then
        log_test PASS "Port conflict handling: API responded appropriately (HTTP $code)"
        record_test "port_conflict" "PASS" "http_code=$code"
    else
        log_test FAIL "Port conflict handling: Unexpected response (HTTP $code)"
        record_test "port_conflict" "FAIL" "http_code=$code"
    fi

    stop_agent >/dev/null 2>&1 || true
}

test_recovery_time() {
    log_test INFO "Test 5: Recovery Time Measurement"

    local recovery_times=()
    local failures=0

    for i in $(seq 1 5); do
        log "  Iteration $i/5"

        if ! stop_agent; then
            log_test WARN "    Stop failed"
            failures=$((failures + 1))
            continue
        fi

        sleep 1

        local start_ts
        start_ts=$(date +%s.%N)

        if ! start_agent; then
            log_test WARN "    Start failed"
            failures=$((failures + 1))
            continue
        fi

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
        for sample in "${recovery_times[@]}"; do
            total=$(echo "$total + $sample" | bc)
        done
        local avg
        avg=$(echo "scale=3; $total / ${#recovery_times[@]}" | bc)

        log_test PASS "Recovery time: avg=${avg}s, samples=${#recovery_times[@]}, failures=$failures"
        record_test "recovery_time" "PASS" "avg=${avg}s samples=${#recovery_times[@]} failures=$failures"
    else
        log_test FAIL "Recovery time: No successful recoveries measured"
        record_test "recovery_time" "FAIL" "no_data"
    fi

    stop_agent >/dev/null 2>&1 || true
}
