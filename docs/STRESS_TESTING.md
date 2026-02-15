# Stress Testing Guide

## Overview

The `scripts/stress-test.sh` script provides reliability stress testing for the Carrier daemon, targeting frequent-failure scenarios commonly encountered in production environments:

- **Restart storms** — Rapid start/stop cycles that can expose race conditions
- **Port conflicts** — Multiple agents attempting to bind to the same port
- **Crash recovery** — Process crashes (SIGKILL) and automatic restart verification
- **Concurrent operations** — Parallel install/start/stop requests
- **Recovery time measurement** — Quantifying system resilience under stress

## Prerequisites

### System Requirements

- **curl** — HTTP client for API calls
- **bc** — Arbitrary precision calculator for timing measurements
- **Bash 4.0+** — For associative arrays and modern features
- **Running Carrier daemon** — Accessible at `http://127.0.0.1:9090` (or custom URL)

### Daemon Setup

Before running stress tests, ensure the Carrier daemon is running:

```bash
# Start daemon (adjust path as needed)
./agentd &

# Verify daemon is accessible
curl -s http://127.0.0.1:9090/api/v1/agents
```

### Agent Availability

The stress test requires an agent to be available in the catalog. By default, it uses `openclaw`. Verify the agent is registered:

```bash
curl -s http://127.0.0.1:9090/api/v1/agents | grep openclaw
```

If the agent is not installed, the stress test will attempt to use it anyway (install is tested as part of the suite).

## Usage

### Basic Execution

Run all stress tests with default settings:

```bash
./scripts/stress-test.sh
```

### Command-Line Options

```bash
./scripts/stress-test.sh [OPTIONS]

Options:
  --daemon-url URL    Daemon API base URL (default: http://127.0.0.1:9090)
  --agent-id ID       Agent ID to test (default: openclaw)
  --iterations N      Number of test iterations (default: 10)
  --quiet             Suppress verbose output (show only summary)
  --help              Show help message and exit
```

### Examples

**Test against a remote daemon:**
```bash
./scripts/stress-test.sh --daemon-url http://192.168.1.100:9090
```

**Test a different agent:**
```bash
./scripts/stress-test.sh --agent-id pi-mono
```

**Run more iterations for thorough testing:**
```bash
./scripts/stress-test.sh --iterations 50
```

**Quiet mode (CI/automation):**
```bash
./scripts/stress-test.sh --quiet
```

**Combine options:**
```bash
./scripts/stress-test.sh \
  --daemon-url http://10.0.0.50:9090 \
  --agent-id openclaw \
  --iterations 20 \
  --quiet
```

### Environment Variables

You can also configure the daemon URL via environment variable:

```bash
export CARRIER_DAEMON_URL=http://127.0.0.1:9090
./scripts/stress-test.sh
```

## Test Scenarios

### Test 1: Restart Storm

**Purpose:** Detect race conditions and state management issues during rapid restart cycles.

**Method:**
- Performs N iterations (default: 10) of start → stop cycles
- Measures average start/stop times
- Reports failures and timing metrics

**Expected Results:**
- ✓ 0 failures across all iterations
- ✓ Average start time: 0.5s - 3.0s
- ✓ Average stop time: 0.2s - 1.0s

**Failure Indicators:**
- Start/stop operations fail intermittently
- Timing degrades significantly over iterations
- State becomes corrupted (agent stuck in transitional state)

### Test 2: Concurrent Operations

**Purpose:** Verify thread-safety and idempotency under parallel load.

**Method:**
- Launches 5 parallel `start` requests simultaneously
- Counts successful responses
- Checks for race conditions and deadlocks

**Expected Results:**
- ✓ At least 1 successful start (idempotency OK)
- ✓ No crashes or hangs
- ✓ All operations complete within 10s

**Failure Indicators:**
- All parallel requests fail
- Daemon becomes unresponsive
- Data corruption in agent state

### Test 3: Crash Recovery

**Purpose:** Validate automatic restart and health monitoring after catastrophic failures.

**Method:**
- Starts agent normally
- Sends SIGKILL (`kill -9`) to agent process
- Waits 2 seconds
- Verifies agent auto-restarted and is healthy

**Expected Results:**
- ✓ Agent process is found and killed
- ✓ Agent automatically restarts within 2s
- ✓ Health check passes after restart

**Failure Indicators:**
- Agent does not restart automatically
- Restart occurs but health check fails
- Daemon does not detect the crash

**Notes:**
- This test may be skipped if the agent process cannot be found (e.g., containerized environments)
- Auto-restart behavior depends on the agent's manifest configuration

### Test 4: Port Conflict

**Purpose:** Test error handling when ports are already occupied.

**Method:**
- Starts agent normally (occupies configured port)
- Attempts to start again (should fail or be idempotent)
- Validates API response codes and error messages

**Expected Results:**
- ✓ Second start returns HTTP 200 (idempotent), 409 (conflict), or 400 (bad request)
- ✓ No daemon crash or corruption
- ✓ Clear error messaging

**Failure Indicators:**
- Daemon crashes on second start attempt
- HTTP 500 internal server error
- State machine enters invalid state

### Test 5: Recovery Time Measurement

**Purpose:** Quantify system resilience and establish performance baselines.

**Method:**
- Performs 5 stop → start → wait-for-healthy cycles
- Measures time from start API call to healthy status
- Calculates average, min, max recovery times

**Expected Results:**
- ✓ At least 4/5 successful recoveries
- ✓ Average recovery time: 1s - 5s
- ✓ Consistent timing (low variance)

**Failure Indicators:**
- Recovery time exceeds 10s
- High variance between iterations
- Frequent timeout failures

## Interpreting Results

### Sample Output

```
Carrier Reliability Stress Test
================================

✓ Daemon is reachable

Running stress tests...

ℹ Test 1: Restart Storm (10x rapid start/stop)
  Iteration 1/10
    Start: 0.823s
    Stop: 0.145s
  ...
✓ Restart storm: 0 failures, avg start=0.891s, avg stop=0.156s

ℹ Test 2: Concurrent Operations (parallel start attempts)
✓ Concurrent operations: 5/5 succeeded (expected >= 1)

ℹ Test 3: Crash Recovery (kill -9 + auto-restart verification)
  Found agent PID: 12345
  Sent SIGKILL to agent
✓ Crash recovery: Agent auto-restarted successfully

ℹ Test 4: Port Conflict (start on occupied port)
✓ Port conflict handling: API responded appropriately (HTTP 200)

ℹ Test 5: Recovery Time Measurement
  Iteration 1/5
    Recovery time: 1.234s
  ...
✓ Recovery time: avg=1.456s, samples=5, failures=0

========================================
Carrier Reliability Stress Test Report
========================================

Configuration:
  Daemon URL: http://127.0.0.1:9090
  Agent ID: openclaw
  Iterations: 10

Results Summary:
  Total Tests: 5
  Passed: 5
  Failed: 0
  Success Rate: 100.0%

Test Details:
----------------------------------------
  ✓ restart_storm: PASS (avg_start=0.891s avg_stop=0.156s)
  ✓ concurrent_operations: PASS (success_count=5)
  ✓ crash_recovery: PASS (auto_restart_verified)
  ✓ port_conflict: PASS (http_code=200)
  ✓ recovery_time: PASS (avg=1.456s samples=5 failures=0)
========================================

All tests passed!
```

### Exit Codes

- **0** — All tests passed
- **1** — One or more tests failed (see report for details)
- **1** — Pre-flight checks failed (daemon unreachable, missing dependencies, etc.)

## Troubleshooting

### Common Issues

#### Daemon not reachable

**Symptom:**
```
Error: Cannot connect to daemon at http://127.0.0.1:9090
```

**Solutions:**
- Verify daemon is running: `ps aux | grep agentd`
- Check daemon logs for startup errors
- Confirm port 9090 is not blocked by firewall
- Try custom URL: `--daemon-url http://localhost:9090`

#### Agent not found

**Symptom:**
```
Error: Agent 'openclaw' not found in catalog
```

**Solutions:**
- List available agents: `curl -s http://127.0.0.1:9090/api/v1/agents`
- Use a different agent: `--agent-id <agent-id>`
- Verify catalog is properly loaded by the daemon

#### Missing dependencies

**Symptom:**
```
Error: curl is required but not installed
Error: bc is required but not installed
```

**Solutions:**
- **Ubuntu/Debian:** `sudo apt-get install curl bc`
- **macOS:** `brew install curl bc` (curl is usually pre-installed)
- **Fedora/RHEL:** `sudo dnf install curl bc`

#### Crash recovery test skipped

**Symptom:**
```
⚠ Crash recovery: Could not find agent process (may be in container)
```

**Explanation:**
The test uses `pgrep` to find the agent process for `kill -9` testing. If the agent runs in an isolated environment (Docker container, systemd sandbox), the test cannot locate the PID and will skip.

**Solutions:**
- Run daemon directly (not containerized) for full crash recovery testing
- This is expected in CI/container environments — other tests still provide value

#### High failure rate

**Symptom:**
```
✗ Restart storm: 8 failures out of 20 operations
```

**Possible Causes:**
- System resource exhaustion (CPU, memory, file descriptors)
- Race conditions in agent or daemon code
- Network latency/instability (if testing remote daemon)
- Insufficient cooldown/sleep intervals

**Solutions:**
- Reduce `--iterations` to lower system load
- Check system resources: `top`, `free -h`, `ulimit -n`
- Increase sleep intervals in script (edit `sleep 0.5` → `sleep 1.0`)
- Review daemon logs for detailed error messages

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: Stress Test

on:
  push:
    branches: [main]
  pull_request:

jobs:
  stress-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Install dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y curl bc
      
      - name: Start daemon
        run: |
          cd daemon
          go build -o ../agentd ./cmd/agentd
          cd ..
          ./agentd &
          sleep 3
      
      - name: Run stress tests
        run: ./scripts/stress-test.sh --quiet --iterations 5
      
      - name: Upload results
        if: failure()
        uses: actions/upload-artifact@v3
        with:
          name: stress-test-logs
          path: /tmp/carrier-stress-*.log
```

### Makefile Integration

Add to project Makefile:

```makefile
.PHONY: stress-test
stress-test:
	@echo "Running stress tests..."
	./scripts/stress-test.sh --iterations 10

.PHONY: stress-test-quick
stress-test-quick:
	@echo "Running quick stress tests..."
	./scripts/stress-test.sh --iterations 3 --quiet
```

## Performance Baselines

Typical performance ranges on reference hardware (4-core, 8GB RAM, SSD):

| Metric | Good | Acceptable | Poor |
|--------|------|------------|------|
| Start time (avg) | < 1s | 1-3s | > 3s |
| Stop time (avg) | < 0.5s | 0.5-1s | > 1s |
| Recovery time (avg) | < 2s | 2-5s | > 5s |
| Restart storm success | 100% | ≥ 90% | < 90% |
| Concurrent success | ≥ 80% | ≥ 50% | < 50% |

**Note:** Performance varies based on agent complexity, system load, and hardware. Establish your own baselines by running tests in a controlled environment.

## Best Practices

### Running in Production-like Environments

1. **Match resource constraints** — Use similar CPU/memory limits as production
2. **Test with real workloads** — Run stress tests while agent handles typical tasks
3. **Vary network conditions** — Simulate latency, packet loss with tools like `tc` (Linux)
4. **Multi-agent scenarios** — Test with multiple agents running concurrently

### Continuous Monitoring

Integrate stress testing into:
- **Pre-release validation** — Gate releases on stress test success
- **Regression detection** — Track performance metrics over time
- **Load capacity planning** — Determine safe agent limits per host

### Customization

For project-specific needs, extend `stress-test.sh`:
- Add custom failure scenarios
- Modify iteration counts per test
- Integrate with monitoring systems (Prometheus, DataDog)
- Export results in structured formats (JSON, CSV)

## Related Documentation

- [ARCHITECTURE.md](../ARCHITECTURE.md) — System design and component interactions
- [daemon/internal/lifecycle](../daemon/internal/lifecycle/) — Lifecycle service implementation
- [CONTRIBUTING.md](../CONTRIBUTING.md) — Development workflow and testing guidelines

## Support

For issues or questions:
- Open a GitHub issue with stress test output and daemon logs
- Include system information: OS, hardware specs, daemon version
- Tag issues with `[stress-test]` label for easier triage
