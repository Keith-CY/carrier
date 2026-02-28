# Current Architecture Index

This page is the canonical index for current implementation docs.
Use this before reading historical planning notes.

## Current Source of Truth

- Product scope: [`docs/Agent_Installation_Platform_PRD.md`](./Agent_Installation_Platform_PRD.md)
- Runtime decision:
  - Phase 1 baseline: [`docs/phase1-runtime-adr.md`](./phase1-runtime-adr.md)
  - Phase 2 opt-in isolation: [`docs/phase2-isolation-adr.md`](./phase2-isolation-adr.md)
- System design overview: [`ARCHITECTURE.md`](../ARCHITECTURE.md)
- Module boundaries: `webui -> gateway -> daemon -> shared` and `webui -> gateway -> baseagent -> shared`
- Shared/base modules: [`shared/`](../shared/), [`baseagent/`](../baseagent/), [`gateway/`](../gateway/), [`daemon/`](../daemon/), [`webui/`](../webui/)
- Gateway command behavior: [`docs/command-contract.md`](./command-contract.md)
- Daemon API contract: [`docs/daemon-api-contract.md`](./daemon-api-contract.md)
- Deployment and operations: [`docs/deployment.md`](./deployment.md)

## Task-Oriented Entry Points

- Carrier CLI command reference: [`docs/carrier-cli.md`](./carrier-cli.md)
- 5-minute first setup: [`docs/task-first-quickstart.md`](./task-first-quickstart.md)
- Pairing and command readiness: [`docs/runbooks/pairing-lifecycle.md`](./runbooks/pairing-lifecycle.md)
- Post-merge smoke checklist: [`docs/runbooks/post-merge-smoke-checklist.md`](./runbooks/post-merge-smoke-checklist.md)
- CI troubleshooting: [`docs/ci/first-response-playbook.md`](./ci/first-response-playbook.md)
- Rollback and go-live checks: [`docs/runbooks/go-live-rollback.md`](./runbooks/go-live-rollback.md)
- Phase-2 roadmap rollup execution: [`docs/plans/phase2-rollup-1406-execution.md`](./plans/phase2-rollup-1406-execution.md)
- Phase-2 agent isolation execution: [`docs/plans/phase2-agent-instance-isolation-execution.md`](./plans/phase2-agent-instance-isolation-execution.md)

## Historical Plan Documents

The documents below are retained for design history.
Do not treat them as the implementation source of truth.

- [`docs/plans/adr-runtime-model.md`](./plans/adr-runtime-model.md) -> use `docs/phase1-runtime-adr.md` (relative path assumes this index stays under `docs/`)
- [`docs/plans/memory-package-specification.md`](./plans/memory-package-specification.md) -> use `daemon/internal/memory/`
- [`docs/plans/memory-import-export-pipeline.md`](./plans/memory-import-export-pipeline.md) -> use `docs/command-contract.md` and `daemon/internal/memory/store.go`
- [`docs/plans/memory-attach-detach-policy.md`](./plans/memory-attach-detach-policy.md) -> use `daemon/internal/memory/policy.go` and `daemon/internal/lifecycle/memory.go`
- [`docs/plans/phase1-execution-checklist.md`](./plans/phase1-execution-checklist.md) -> use `docs/Agent_Installation_Platform_PRD.md` + `ARCHITECTURE.md`
- [`docs/plans/log-rotation-retention-policy.md`](./plans/log-rotation-retention-policy.md) -> use `docs/deployment.md` + `daemon/internal/logging/`
- [`docs/plans/phase1-e2e-test-matrix.md`](./plans/phase1-e2e-test-matrix.md) -> use `.github/workflows/ci.yml` + `scripts/run-e2e-tests.sh`

## Documentation Hygiene

- Keep unresolved work as tracked issues, not inline placeholders.
- Keep this index updated when canonical architecture docs move.
- Relative links in this file assume it lives in `docs/`. Update links if this file moves.
- Audit automation must write snapshots to `docs/audit-snapshot.md` and never overwrite this index.
