# ADR-Phase1-001: Runtime Model Baseline (Local Host + WSL2, No Docker)

- Status: Accepted
- Date: 2026-02-17
- Scope: Phase 1 runtime model and terminology baseline
- Source issues: #74 #796

> Historical context: [`docs/plans/adr-runtime-model.md`](plans/adr-runtime-model.md)

## Context

Phase 1 documentation previously referenced runtime behavior in multiple places (PRD, implementation plan, product design, README, and historical supplemental notes). To avoid drift, one canonical runtime baseline is required.

## Decision

Phase 1 runtime is **local-first only**:

- macOS/Linux: run agent processes on host.
- Windows: run agent processes in WSL2.
- Docker/container runtime: out of scope for Phase 1.

The canonical reference chain is:

1. `docs/Agent_Installation_Platform_PRD.md`
2. `docs/phase1-runtime-adr.md` (this ADR)
3. `docs/Agent_Installation_Platform_Implementation_Plan.md`
4. `README.md`

## Alternatives Considered

1. Docker-first for all platforms.
- Pros: environment consistency, easier dependency isolation.
- Cons: extra operational complexity, conflicts with local-first onboarding goal, larger Phase 1 surface.

2. Hybrid local + Docker in Phase 1.
- Pros: gradual transition path.
- Cons: doubles validation matrix, adds ambiguous operator guidance, increases troubleshooting burden.

3. Platform-specific divergence (macOS/Linux local, Windows Docker).
- Pros: avoids WSL2 dependency.
- Cons: breaks cross-platform parity and complicates support runbooks.

## Tradeoffs

- Choosing local-first improves onboarding speed and Phase 1 focus.
- Choosing local-first reduces environment abstraction and may expose host-level dependency drift.
- Deferring Docker reduces immediate flexibility but keeps acceptance criteria testable.

## Risks and Mitigations

1. Host dependency drift causes install/start failures.
- Mitigation: strict runtime prerequisite checks and diagnose artifacts.

2. WSL2 setup inconsistency on Windows.
- Mitigation: explicit WSL2 support matrix and failure guidance.

3. Future docs regress to mixed runtime language.
- Mitigation: PR checklist gate: "Does this change affect runtime model?" and mandatory ADR reference.

## Consequences

- Manifest/runtime examples must describe host/WSL2 execution only in Phase 1.
- Runtime-related docs must not introduce Docker commands or Docker-only assumptions.
- Docker support, if added, requires a new ADR and explicit Phase 2+ scope change.

## Required Update List for Runtime-Model Changes

Any runtime-model change must update, in one PR:

1. `docs/phase1-runtime-adr.md`
2. `docs/Agent_Installation_Platform_PRD.md`
3. `docs/Agent_Installation_Platform_Implementation_Plan.md`
4. `README.md`
5. `.github/PULL_REQUEST_TEMPLATE.md`
