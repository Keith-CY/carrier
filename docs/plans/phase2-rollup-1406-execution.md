# Phase 2 Rollup Execution Plan (`#1406`)

This plan decomposes roadmap epics `#1224-#1239` into execution tracks under `#1406`.

## Child Task Register

| Bucket | Child Task | Owner | Verification Notes | Merge Evidence |
| --- | --- | --- | --- | --- |
| 1 | Multi-agent runtime message bus + routing semantics | @Keith-CY | Unit tests in `carrier/baseagent/controlplane_bus.go` and `carrier/baseagent/controlplane_sessions.go` validate queue, routing, and session behavior. | Referenced from `#1482` acceptance gate evidence (`docs/plans/phase2-acceptance-gate-1482.md`). |
| 2 | WebUI chat room/session model + remote/local routing | @Keith-CY | WebUI E2E suite `webui/e2e/tests/remote-control-plane.spec.ts` validates session lifecycle, chat routing, retry/cancel/reset, and session-level actions. | Referenced from `#1482` acceptance gate evidence (`docs/plans/phase2-acceptance-gate-1482.md`). |
| 2 | WebUI logs scale surfaces (filters/pause/resume/windowing) | @Keith-CY | WebUI E2E suite `webui/e2e/tests/logs.spec.ts` validates structured rendering, filters, search, pause/resume buffering, and clear behavior. | Referenced from `#1482` acceptance gate evidence (`docs/plans/phase2-acceptance-gate-1482.md`). |
| 3 | Onboarding continuity + deterministic state transitions | @Keith-CY | Gateway onboarding state-machine tests in `gateway/onboard_stepped_test.go` and `gateway/onboard_round7_test.go` validate resumable transitions and recovery paths. | Referenced from `#1482` acceptance gate evidence (`docs/plans/phase2-acceptance-gate-1482.md`). |
| 3 | Diagnose-and-fix safe remediation loop | @Keith-CY | Lifecycle repair/triage tests in `daemon/internal/lifecycle/repair_test.go` and `daemon/internal/lifecycle/triage_rules_test.go` validate bounded attempts, risk handling, and actionable output. | Referenced from `#1482` acceptance gate evidence (`docs/plans/phase2-acceptance-gate-1482.md`). |
| 3 | Starter templates + safe apply/rollback | @Keith-CY | Remote API and control-plane tests in `gateway/remote_api_test.go` and `webui/e2e/tests/remote-control-plane.spec.ts` validate preview/apply/rollback operations and idempotent rollback path behavior. | Referenced from `#1482` acceptance gate evidence (`docs/plans/phase2-acceptance-gate-1482.md`). |
| 4 | Isolation runtime and unsupported-host fail-fast path (`#1475`, `#1445`) | @Keith-CY | Isolation option propagation + daemon fail-fast behavior validated by `daemon/internal/lifecycle/start_isolation_test.go`, `gateway/daemonclient_test.go`, `tests/e2e/e2e_test.sh`, and API error mapping tests. | Referenced from `#1482` acceptance gate evidence (`docs/plans/phase2-acceptance-gate-1482.md`). |
| 4 | Reversible operations + upgrade safety gates | @Keith-CY | Upgrade and rollback metadata/tests in `daemon/internal/lifecycle/upgrade.go`, `daemon/internal/lifecycle/service_test.go`, `daemon/internal/api/server_test.go`, and remote rollback flows in `gateway/remote_api_test.go`. | Referenced from `#1482` acceptance gate evidence (`docs/plans/phase2-acceptance-gate-1482.md`). |

## Verification Baseline

- Automated matrix evidence is captured in `docs/plans/phase2-acceptance-gate-1482.md`.
- Matrix includes:
  - `cd daemon && go test ./...`
  - `cd gateway && go test ./...`
  - `bash scripts/build-webui.sh`
  - `bash scripts/check-doc-command-sync.sh`
  - `bash scripts/coverage-gate.sh`
  - `cd webui/e2e && CI=1 bunx playwright test tests/logs.spec.ts tests/remote-control-plane.spec.ts --reporter=line`

## Exit Conditions

1. Each bucket has explicit child tasks linked from `#1406`.
2. Each child task has owner, verification notes, and merge evidence.
3. `#1406` remains open until the acceptance gate issue `#1482` is complete.
4. `#1482` is the final closure reference for this rollup.
