# Current Architecture Index

This page is the canonical index for current implementation docs.
Use this before reading historical planning notes.

## Current Source of Truth

- Product scope: [`docs/Agent_Installation_Platform_PRD.md`](./Agent_Installation_Platform_PRD.md)
- Runtime decision: [`docs/phase1-runtime-adr.md`](./phase1-runtime-adr.md)
- System design overview: [`ARCHITECTURE.md`](../ARCHITECTURE.md)
- Gateway command behavior: [`docs/command-contract.md`](./command-contract.md)
- Daemon API contract: [`docs/daemon-api-contract.md`](./daemon-api-contract.md)
- Deployment and operations: [`docs/deployment.md`](./deployment.md)

## Task-Oriented Entry Points

- 5-minute first setup: [`docs/task-first-quickstart.md`](./task-first-quickstart.md)
- Pairing and command readiness: [`docs/runbooks/pairing-lifecycle.md`](./runbooks/pairing-lifecycle.md)
- Post-merge smoke checklist: [`docs/runbooks/post-merge-smoke-checklist.md`](./runbooks/post-merge-smoke-checklist.md)
- CI troubleshooting: [`docs/ci/first-response-playbook.md`](./ci/first-response-playbook.md)
- Rollback and go-live checks: [`docs/runbooks/go-live-rollback.md`](./runbooks/go-live-rollback.md)

## Historical Plan Documents

The documents below are retained for design history.
Do not treat them as the implementation source of truth.

- [`docs/plans/adr-runtime-model.md`](./plans/adr-runtime-model.md) -> use `docs/phase1-runtime-adr.md` (relative path assumes this index stays under `docs/`)
- [`docs/plans/memory-package-specification.md`](./plans/memory-package-specification.md) -> use `daemon/internal/memory/`
- [`docs/plans/memory-import-export-pipeline.md`](./plans/memory-import-export-pipeline.md) -> use `docs/command-contract.md` and `daemon/internal/memory/store.go`
- [`docs/plans/memory-attach-detach-policy.md`](./plans/memory-attach-detach-policy.md) -> use `daemon/internal/memory/policy.go` and `daemon/internal/lifecycle/memory.go`

## Documentation Hygiene

- Keep unresolved work as tracked issues, not inline placeholders.
- Keep this index updated when canonical architecture docs move.
- Relative links in this file assume it lives in `docs/`. Update links if this file moves.
