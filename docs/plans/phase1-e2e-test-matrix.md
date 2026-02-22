# Phase 1 E2E Test Matrix (Happy/Failure/Memory)

> **Issue:** #85 · **Status:** Plan · **Track:** C4
>
> **Note:** This is a historical planning matrix and may diverge from the active test suite.
> For current execution and canonical references, use [`docs/current-architecture.md`](../current-architecture.md), [`docs/ci/first-response-playbook.md`](../ci/first-response-playbook.md), and the `scripts/` + CI workflow definitions under `.github/workflows/ci.yml`.

## Goals

- Define a comprehensive E2E test matrix covering happy paths, failure paths, and memory operations.
- Ensure every Phase 1 feature has at least one E2E test.
- Produce a machine-readable run report after each test suite execution.
- Use the shared parity categories from `docs/e2e-parity-taxonomy.md` when writing cross-provider assertions.

## Non-Goals

- Performance/load testing (Phase 2).
- Multi-node cluster E2E scenarios.
- UI/frontend E2E tests.

---

## 1. Happy Path: Pair → Install → Start → Healthy

These tests verify the core agent lifecycle works end-to-end.

| # | Test Name | Steps | Expected Result |
|---|-----------|-------|-----------------|
| H1 | Basic agent lifecycle | 1. Pair agent with daemon<br>2. Install agent (valid manifest)<br>3. Start agent<br>4. Wait for health check | Agent status = `healthy` |
| H2 | Agent with dependencies | 1. Install agent with runtime deps<br>2. Start agent | Dependencies resolved, agent healthy |
| H3 | Agent stop and restart | 1. Start agent<br>2. Stop agent<br>3. Verify stopped<br>4. Start again | Agent healthy after restart |
| H4 | Agent uninstall | 1. Install + start agent<br>2. Stop agent<br>3. Uninstall agent | Agent removed, no leftover files |
| H5 | Multiple agents | 1. Install 3 different agents<br>2. Start all<br>3. Health check all | All agents healthy, no port conflicts |
| H6 | Agent with config | 1. Install agent with custom env vars<br>2. Start agent<br>3. Verify config applied | Config values visible in agent runtime |

---

## 2. Failure Path: Error Handling and Recovery

These tests verify the system handles errors gracefully with clear diagnostics.

| # | Test Name | Steps | Expected Result |
|---|-----------|-------|-----------------|
| F1 | Missing runtime dependency | 1. Install agent requiring unavailable runtime<br>2. Attempt start | Clear error: "runtime X not found" |
| F2 | Missing environment variable | 1. Install agent requiring `$DB_URL`<br>2. Start without setting it | Error with missing var name |
| F3 | Port conflict | 1. Start agent on port 8080<br>2. Start second agent on same port | Second agent fails with port-conflict error |
| F4 | Forced crash recovery | 1. Start agent<br>2. Kill process with SIGKILL<br>3. Wait for supervisor restart | Agent auto-restarts and becomes healthy |
| F5 | Invalid manifest | 1. Attempt install with malformed manifest | 400 error with validation details |
| F6 | Disk full during install | 1. Fill disk to threshold<br>2. Attempt install | Graceful failure, no partial state |
| F7 | Agent exceeds memory limit | 1. Start agent with memory limit<br>2. Agent allocates beyond limit | Agent killed with OOM indicator in logs |
| F8 | Network timeout during pairing | 1. Simulate network delay<br>2. Attempt pair | Timeout error with retry guidance |

---

## 3. Memory Path: Import → Duplicate/Share → Attach → Export

These tests verify the memory package pipeline (depends on #77 and #78).

| # | Test Name | Steps | Expected Result |
|---|-----------|-------|-----------------|
| M1 | Import valid package | 1. Create valid memory.yaml + artifact<br>2. POST to import API | 201, package persisted |
| M2 | Import duplicate (idempotent) | 1. Import package<br>2. Import same package again | 200, no duplicate created |
| M3 | Import duplicate (conflict) | 1. Import package v1.0.0<br>2. Import different content as v1.0.0 | 409 Conflict |
| M4 | Force re-import | 1. Import package<br>2. Force re-import with different content | 200, backup created |
| M5 | Share package | 1. Import package as user A<br>2. Export and download as user B | User B gets valid package |
| M6 | Attach memory to agent | 1. Import memory package<br>2. Install agent with memory dependency<br>3. Start agent | Agent starts with memory available |
| M7 | Export and download | 1. Import package<br>2. Export (get token)<br>3. Download with token | Valid tar.gz with memory.yaml |
| M8 | Download token expiry | 1. Export package<br>2. Wait for token TTL<br>3. Attempt download | 401 Unauthorized |
| M9 | Rollback after force import | 1. Import v1<br>2. Force import v1 (new content)<br>3. Rollback | Original v1 restored |
| M10 | Import invalid package | 1. POST package with missing required fields | 400 with validation errors |

---

## 4. Run Report Format

After each test suite run, a JSON report is emitted:

```json
{
  "suite": "phase1-e2e",
  "timestamp": "2026-02-14T10:00:00Z",
  "duration_seconds": 245,
  "environment": {
    "os": "linux",
    "go_version": "1.22",
    "carrier_version": "0.1.0-dev"
  },
  "summary": {
    "total": 24,
    "passed": 22,
    "failed": 1,
    "skipped": 1
  },
  "tests": [
    {
      "id": "H1",
      "name": "Basic agent lifecycle",
      "path": "happy",
      "status": "passed",
      "duration_ms": 3200,
      "logs": "...(truncated)"
    }
  ],
  "failures": [
    {
      "id": "F6",
      "name": "Disk full during install",
      "error": "assertion failed: expected graceful error, got panic",
      "logs": "...(full output)"
    }
  ]
}
```

### Report Delivery

- Written to `test-results/e2e-report-<timestamp>.json`
- Summary printed to stdout
- CI can parse the JSON for pass/fail gating
- Failed tests include full log output for debugging

---

## 5. Test Infrastructure

### Requirements

- Isolated test environment (Docker or tmpfs-based)
- Ability to simulate: disk full, port conflicts, network delays, process kills
- Mock runtimes for CI (real runtimes for nightly)
- Parallel execution support (tests must not share ports or state)

### Test Harness

```go
// Package e2e provides the test framework
package e2e

type TestCase struct {
    ID       string   // e.g. "H1"
    Name     string
    Path     string   // "happy", "failure", "memory"
    Tags     []string
    Setup    func(ctx *TestContext) error
    Run      func(ctx *TestContext) error
    Teardown func(ctx *TestContext) error
}

type TestContext struct {
    DataDir    string
    DaemonAddr string
    Logger     *slog.Logger
}
```

### Execution

```bash
# Run all E2E tests
carrier test e2e --report test-results/

# Run specific path
carrier test e2e --path happy

# Run specific test
carrier test e2e --id H1
```

---

## Acceptance Criteria

1. All 24 test cases documented with clear steps and expected results.
2. Test harness framework implemented with setup/run/teardown lifecycle.
3. JSON run report generated automatically after each run.
4. At least happy-path tests (H1–H6) automated and passing in CI.
5. Failure and memory tests have documented manual procedures if not yet automated.

## Timeline Estimate

| Task | Estimate |
|------|----------|
| Test matrix documentation (this doc) | 1 day |
| Test harness framework | 2 days |
| Happy path automation (H1–H6) | 3 days |
| Failure path automation (F1–F5) | 2 days |
| Memory path automation (M1–M7) | 3 days |
| Report generation | 1 day |
| **Total** | **~12 days** |
