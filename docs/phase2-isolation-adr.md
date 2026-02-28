# ADR-Phase2-001: Agent Instance Isolation (Opt-in, VM + bubblewrap)

- Status: Accepted
- Date: 2026-02-28
- Scope: Phase 2 opt-in isolation runtime for Carrier-managed agent instances
- Source issue: #1475

## Context

Phase 1 runtime is intentionally local-first:
- macOS/Linux: host runtime
- Windows: WSL2 runtime

That baseline remains valid and unchanged for Phase 1 delivery.

Phase 2 introduces an optional isolation path for managed instances to provide:
- filesystem default-deny behavior
- network default-deny behavior
- baseline resource quotas
- auditable and reproducible runtime behavior across platforms

## Decision

Carrier introduces a Phase 2 opt-in isolation mode for agent instances.

### User-facing behavior

- Local add/install supports explicit isolation:
  - `carrier add <agent_id> --isolation`
  - `carrier install <agent_id> --isolation`
- `carrier add` in CLI/TUI still performs install + auto-start.
- WebUI flow supports isolation via checkbox.
- `carrier add <agent_id> --webui --isolation` opens WebUI with isolation preselected; deployment/start remains in WebUI flow.
- Remote add supports explicit isolation:
  - `carrier remote add <agent_id> ... --isolation`
  - Supported for `on_demand` and `managed_gateway` runtime modes.

### Runtime model

- Linux: native bubblewrap backend.
- macOS: shared lightweight Linux VM (for example Lima) + bubblewrap.
- Windows: shared WSL2 Linux layer + bubblewrap.

### Lifecycle boundary

- `install`: remains host-side in Phase 2 for compatibility.
- `start/stop`: execute through isolation runtime when instance isolation is enabled.

### Persistence and override rules

- Isolation is an instance-level persisted property.
- `start` always follows the persisted instance mode.
- No one-off runtime override at `start` time.

### Failure policy

- If isolation is explicitly requested but backend/policy is unavailable, fail fast.
- No silent fallback to non-isolated execution.

## Security principles

1. Default deny:
- filesystem exposure is whitelist-only
- network egress is deny-by-default

2. Least privilege:
- policy additions are incremental and validated
- platform baseline restrictions cannot be relaxed by per-agent requests

3. Auditability:
- record resolved isolation policy and backend
- record startup/stop outcomes and denial events

## Consequences

- API/state contracts gain additive optional isolation fields.
- CLI reference and deployment guide must document isolation prerequisites and behavior.
- Manifest schema gains an `isolation` section for validated incremental requests.
- `docs/manifest-commands.md` can no longer state "no additional sandboxing is applied" as a global rule.

## Out of scope for this ADR

- Replacing all runtime execution paths with isolation-by-default globally.
- Introducing OCI container runtime as the primary backend in this phase.
- Changing Phase 1 authority or acceptance criteria.

## Required update list for isolation-model changes

Any change to Phase 2 isolation behavior must update, in one PR:

1. `docs/phase2-isolation-adr.md`
2. `docs/Agent_Installation_Platform_PRD.md`
3. `docs/carrier-cli.md`
4. `docs/daemon-api-contract.md`
5. `docs/command-contract.md`
6. `docs/deployment.md`
