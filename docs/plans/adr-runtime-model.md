# ADR: Phase 1 Runtime Model — Local-First, No Docker

> Superseded for authority by [`docs/phase1-runtime-adr.md`](../phase1-runtime-adr.md).
> This file is kept as historical context from earlier planning drafts.

## Status

**Superseded (historical)**

## Context

Issue #45 identified a potential conflict between the PRD and a supplemental Chinese PRD regarding the runtime execution model (local vs Docker-first). Upon review, the existing documentation is consistent:

- **PRD** (`Agent_Installation_Platform_PRD.md`): "Runtime is local-first (macOS/Linux host processes, Windows via WSL2)" and explicitly lists "Docker-based runtime" as out-of-scope.
- **Implementation Plan** (`Agent_Installation_Platform_Implementation_Plan.md`): "Runtime: no Docker path".
- **Product Design** (`Agent_Installation_Platform_Product_Design.md`): "Docker is out of scope for this version".

The referenced Chinese supplemental PRD (`Agent_Installation_Platform_PRD_extra.md`) does not exist in the repository. The conflict described in #45 may have been resolved in a prior edit or the supplemental document was never committed.

## Decision

Phase 1 uses **local-first runtime only**:

- **macOS/Linux**: native host processes managed by the daemon
- **Windows**: local processes inside WSL2
- **Docker**: explicitly out of scope for Phase 1

All manifest commands, install/start/stop actions, and runtime-prerequisite checks assume a local execution environment.

## Consequences

1. Manifest `runtime` field values are limited to local toolchain types (e.g., `node`, `python`, `go`, `binary`).
2. No container image references, Dockerfile support, or Docker-compose integration in Phase 1.
3. Docker/container support may be introduced in Phase 2+ as a separate runtime strategy.
4. WSL2 is treated as a Linux-compatible local environment, not a container runtime.

## Alignment Verification

| Document | Runtime Statement | Aligned? |
|----------|------------------|----------|
| PRD | Local-first, Docker excluded | ✅ |
| Implementation Plan | No Docker path | ✅ |
| Product Design | Docker out of scope | ✅ |
| WSL2 Support Matrix | WSL2 as local Linux environment | ✅ |
