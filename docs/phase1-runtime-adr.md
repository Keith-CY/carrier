# ADR: Phase 1 Runtime Model Baseline

Status: Accepted (Phase 1)

> Historical context: [`docs/plans/adr-runtime-model.md`](plans/adr-runtime-model.md)

## Context

Carrier Phase 1 must provide a reliable local-first runtime for daemon-managed agents while keeping installation and troubleshooting approachable for contributors and operators.

Primary constraints:
- Deterministic lifecycle behavior for install/start/stop/status/logs/diagnose
- Minimal operational surface for Phase 1
- Compatibility with desktop environments

## Decision

Phase 1 runtime model is:
- **macOS/Linux**: local host runtime (native processes)
- **Windows**: local runtime inside **WSL2**
- **Docker runtime**: explicitly out of Phase 1 scope

OpenClaw is the only full-lifecycle target in Phase 1; other catalog entries remain candidates.

## Alternatives Considered

### A) Docker-first runtime

Pros:
- Better host isolation
- Reproducible container environment

Cons:
- Added complexity in networking/volumes/privilege model
- Increased onboarding burden for non-technical users
- Diverges from Phase 1 speed-to-healthy objective

Decision: **Rejected for Phase 1**.

### B) Windows native runtime (without WSL2)

Pros:
- Potentially fewer moving parts for users not using WSL2

Cons:
- Additional platform-specific process/service edge cases
- Harder parity with Linux-oriented runtime tooling
- Higher test matrix and maintenance cost in Phase 1

Decision: **Deferred**.

### C) Hybrid local+remote runtime in Phase 1

Pros:
- Flexibility for advanced deployment patterns

Cons:
- Significant policy/auth/diagnostics complexity increase
- Dilutes focus from core lifecycle reliability

Decision: **Rejected for Phase 1**.

## Tradeoffs

Accepted tradeoffs:
- Reduced platform breadth in exchange for predictable lifecycle behavior
- WSL2 prerequisite on Windows in exchange for runtime consistency
- No Docker convenience in exchange for lower orchestration complexity

## Risks and Mitigations

1. **Risk**: Users expect Docker support immediately
   - **Mitigation**: keep Docker explicitly documented as out-of-scope for Phase 1; revisit in later phase planning.

2. **Risk**: WSL2 setup friction on Windows
   - **Mitigation**: maintain WSL2 support matrix and prerequisite checks in daemon startup paths.

3. **Risk**: Conflicting wording across docs
   - **Mitigation**: treat PRD as source of truth and keep docs aligned through micro tasks under #75.

## Canonical Source Note

This file (`docs/phase1-runtime-adr.md`) is the canonical runtime ADR for Phase 1.

`docs/plans/adr-runtime-model.md` is retained as historical context and is superseded for decision authority.

## Acceptance Metadata

| Field            | Value                          |
|-----------------|--------------------------------|
| Effective date  | 2026-02-16                     |
| ADR version     | 1.0                           |
| Accepted by     | Phase 1 review (PR #1040)     |

## References

- PRD source-of-truth scope: `docs/Agent_Installation_Platform_PRD.md`
- Product design runtime model: `docs/Agent_Installation_Platform_Product_Design.md`
- Historical (superseded) ADR context: `docs/plans/adr-runtime-model.md`
- Parent plan: issue #74
- Micro task: issue #796
