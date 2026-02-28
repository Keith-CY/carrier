# Phase 2 Execution Plan: Agent Instance Isolation (`--isolation`)

This document defines the implementation sequence for Phase 2 opt-in isolation.
Decision authority is:
- `docs/Agent_Installation_Platform_PRD.md`
- `docs/phase2-isolation-adr.md`

## 1. Goal

Deliver a production-ready opt-in isolation runtime for Carrier-managed instances:
- explicit enablement from CLI/TUI/WebUI and remote add
- install + auto-start unchanged for local CLI/TUI add
- start/stop isolation execution for isolation-enabled instances
- additive, backward-compatible API/state contract updates

## 2. Locked product semantics

1. Enablement model
- explicit user request only (`--isolation` or WebUI checkbox)
- no global default-on behavior

2. Local add behavior
- `carrier add` (CLI/TUI): install + auto-start (unchanged)
- with `--isolation`, auto-start runs in isolation runtime

3. WebUI add behavior
- `carrier add ... --webui --isolation` only opens WebUI and preselects isolation
- CLI does not perform deployment/start in this mode

4. Remote add behavior
- `carrier remote add ... --isolation` supported in:
  - `--runtime-mode on_demand`
  - `--runtime-mode managed_gateway`

5. Start behavior
- isolation mode persists per instance
- `start` strictly follows persisted instance mode
- no temporary override at `start` time

6. Failure behavior
- explicit isolation request + unavailable backend => fail fast, no fallback

## 3. Milestones and PR breakdown

## M1: Documentation and contract alignment

Deliverables:
- Add Phase 2 isolation ADR and this execution plan.
- Update PRD with a Phase 2 extension section.
- Update CLI/API/command/deployment/lifecycle docs to reflect isolation semantics.

Acceptance:
- all docs reference a consistent runtime/isolation model
- no conflict with Phase 1 baseline language

## M2: CLI and WebUI input surface

Deliverables:
- Add `--isolation` parsing for:
  - `carrier add`
  - `carrier install` (alias)
  - `carrier remote add`
- Add WebUI query/state prefill for isolation checkbox in add flow.

Acceptance:
- command parsing handles isolation option consistently across local/remote paths
- webui mode preserves "open-page-only" semantics

## M3: Isolation abstraction in daemon lifecycle

Deliverables:
- Introduce isolation runtime abstraction:
  - policy resolution
  - backend interface
  - lifecycle integration hooks
- Add persisted instance/runtime isolation metadata.

Acceptance:
- local process path and isolation path are both supported
- non-isolated instances keep existing behavior

## M4: Backend implementation (VM + bubblewrap)

Deliverables:
- Linux native bubblewrap execution path.
- macOS/Windows Linux capability layer adapters (Lima/WSL2).
- baseline quotas and default-deny fs/network policies.

Acceptance:
- explicit isolation start succeeds in ready environments
- unavailable isolation layer returns explicit isolation errors

## M5: API, audit, and error mapping

Deliverables:
- additive isolation fields in daemon status/list payloads.
- isolation-related error mapping in command layer.
- audit events for policy resolution/start/stop/denials.

Acceptance:
- API additions are backward compatible
- gateway command responses preserve existing envelope shape

## M6: Stabilization and operational runbooks

Deliverables:
- troubleshooting runbook updates for isolation failures.
- rollout/rollback guidance for isolation-enabled deployments.

Acceptance:
- operators can diagnose common isolation failures with documented steps

## 4. Required interface additions

1. CLI
- `carrier add <agent_id> [--isolation] ...`
- `carrier install <agent_id> [--isolation] ...`
- `carrier remote add <agent_id> ... [--isolation]`

2. Manifest
- top-level `isolation` section for incremental policy requests

3. Daemon API state additions (optional)
- `isolation.mode`
- `isolation.backend`
- `isolation.state`
- `isolation.policyVersion`
- `isolation.lastError`

4. Error codes
- `E_ISOLATION_UNAVAILABLE`
- `E_ISOLATION_POLICY_INVALID`
- `E_ISOLATION_START_FAILED`
- `E_ISOLATION_NETWORK_DENIED`
- `E_ISOLATION_QUOTA_EXCEEDED`

## 5. Validation scenarios

1. Local add + isolation
- `carrier add openclaw --isolation` performs install + isolated auto-start

2. WebUI path
- `carrier add openclaw --webui --isolation` opens target page with isolation preselected
- deployment/start remains in WebUI flow

3. Remote path
- `carrier remote add openclaw ... --isolation --runtime-mode on_demand` succeeds
- `carrier remote add openclaw ... --isolation --runtime-mode managed_gateway` succeeds

4. Start persistence
- restarting an isolation-enabled instance uses isolation mode without extra flags

5. Fail-fast guarantees
- explicit isolation request with unavailable backend returns isolation error and does not fallback

## 6. Rollout defaults and assumptions

1. Phase 1 baseline remains authoritative for non-isolated runtime behavior.
2. Isolation is opt-in and instance-scoped.
3. API changes are additive only.
4. Backward compatibility is required for non-isolation users.
