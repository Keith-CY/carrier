# Phase 2 Acceptance Gate Evidence (`#1482`)

This document records acceptance evidence for issue `#1482` and rollup `#1406`.

## Scope Links

- Rollup: `#1406`
- Acceptance gate: `#1482`
- Isolation track: `#1475`
- Unsupported-host track: `#1445`

## A-K Acceptance Coverage

### A. Program / Tracking

- Child task register with owner + verification + merge evidence is maintained in:
  - `docs/plans/phase2-rollup-1406-execution.md`
- `#1482` is treated as final acceptance gate before `#1406` closure.

### B. Multi-Agent Runtime Messaging

- Room/session semantics and deterministic routing:
  - `carrier/baseagent/controlplane_sessions.go`
  - `carrier/baseagent/controlplane_bus.go`
- Stable error shapes:
  - `gateway/external_errors.go`
  - `daemon/internal/api/errors.go`
- Coverage references:
  - `carrier/baseagent/*_test.go`
  - `gateway/*_test.go`

### C. WebUI Chat Room Model

- Session/room lifecycle + routing UX:
  - `webui/src/app.ts` (`server-manage` and `remote-chat` flows)
  - `gateway/remote_api.go`
- E2E evidence:
  - `webui/e2e/tests/remote-control-plane.spec.ts`

### D. WebUI Logs Scalability

- Structured logs controls + bounded window rendering:
  - `webui/src/app.ts` (`LOG_ENTRY_LIMIT`, filtering, pause/resume, buffering)
- E2E evidence:
  - `webui/e2e/tests/logs.spec.ts`

### E. Onboarding Continuity + Summary Artifact

- Resumable/deterministic onboarding state machine:
  - `gateway/onboard.go`
  - `gateway/onboard_stepped_test.go`
  - `gateway/onboard_round7_test.go`
- Recovery guidance and deterministic failure path handling:
  - `gateway/onboard*.go` + tests.

### F. Diagnose-And-Fix Safe Remediation

- Risk-classified and bounded repair attempts:
  - `daemon/internal/lifecycle/repair.go`
  - `daemon/internal/lifecycle/triage_rules.go`
- Evidence:
  - `daemon/internal/lifecycle/repair_test.go`
  - `daemon/internal/lifecycle/triage_rules_test.go`

### G. Starter Templates + Preview + Safe Apply

- Preview/apply/rollback API and audit behavior:
  - `gateway/remote_api.go`
  - `gateway/remote_openclaw.go`
- Evidence:
  - `gateway/remote_api_test.go`
  - `webui/e2e/tests/remote-control-plane.spec.ts`

### H. Isolation + Security Baseline + CI Guards

- Isolation metadata + API propagation + fail-fast behavior:
  - `daemon/internal/lifecycle/start_options.go`
  - `daemon/internal/lifecycle/start_stop.go`
  - `daemon/internal/api/errors.go`
  - `gateway/daemonclient.go`
  - `gateway/webui_add.go`
  - `gateway/webui_instances.go`
  - `gateway/remote_api.go`
  - `gateway/remote_openclaw.go`
- Unsupported host error mapping:
  - `E_ISOLATION_UNAVAILABLE`, `E_ISOLATION_START_FAILED`
- Evidence:
  - `daemon/internal/lifecycle/start_isolation_test.go`
  - `tests/e2e/e2e_test.sh`
  - `gateway/*isolation*_test.go`

### I. Reversible Operations / Rollback Model

- Rollback metadata and operations:
  - `daemon/internal/lifecycle/upgrade.go`
  - `gateway/remote_api.go` (`/rollback` path)
  - `webui/src/app.ts` (rollback action UI)
- Evidence:
  - `daemon/internal/lifecycle/service_test.go`
  - `gateway/remote_api_test.go`
  - `webui/e2e/tests/remote-control-plane.spec.ts`

### J. Upgrade Safety Gates

- Pre/post checks and rollback metadata exposure:
  - `daemon/internal/lifecycle/upgrade.go`
  - `daemon/internal/api/server.go`
- Evidence:
  - `daemon/internal/api/server_test.go`
  - `daemon/internal/lifecycle/service_test.go`

### K. Docs / Contract Sync

- Updated docs:
  - `docs/phase2-isolation-adr.md`
  - `docs/carrier-cli.md`
  - `docs/daemon-api-contract.md`
  - `docs/command-contract.md`
  - `docs/deployment.md`
  - `docs/runbooks/go-live-rollback.md`
- Sync command:
  - `bash scripts/check-doc-command-sync.sh`

## Automated Verification Matrix (Final Run)

Executed from repo root with passing status:

1. `cd daemon && go test ./...` -> pass
2. `cd gateway && go test ./...` -> pass
3. `bash scripts/build-webui.sh` -> pass
4. `bash scripts/check-doc-command-sync.sh` -> pass
5. `bash scripts/coverage-gate.sh` -> pass
6. `cd webui/e2e && CI=1 bunx playwright test tests/logs.spec.ts tests/remote-control-plane.spec.ts --reporter=line` -> pass (`12 passed`)

## Scenario Validation Mapping

- `carrier add <managed-agent> --isolation` supported path + persistence validated by command parsing, payload propagation, and lifecycle/start tests:
  - `cmd/carrier/main_test.go`
  - `cmd/carrier/main_instance_commands_test.go`
  - `gateway/daemonclient_test.go`
  - `daemon/internal/lifecycle/start_isolation_test.go`
- Explicit isolation fail-fast on unsupported backend:
  - `daemon/internal/lifecycle/start_isolation_test.go`
  - `tests/e2e/e2e_test.sh`
- Multi-room/session isolation:
  - `webui/e2e/tests/remote-control-plane.spec.ts`
- Large-log behavior with filtering and pause/resume:
  - `webui/e2e/tests/logs.spec.ts`
- Template preview/apply/rollback:
  - `gateway/remote_api_test.go`
  - `webui/e2e/tests/remote-control-plane.spec.ts`
- Upgrade checks + rollback metadata:
  - `daemon/internal/lifecycle/service_test.go`
  - `daemon/internal/api/server_test.go`

