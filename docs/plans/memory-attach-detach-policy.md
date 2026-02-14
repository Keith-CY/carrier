# OpenClaw Memory Attach/Detach and Mount Policy Enforcement

## Overview

This design covers instance-level memory attach/detach lifecycle APIs and strict mount-policy enforcement for the OpenClaw daemon. The goal is to ensure agents can only access memory stores they are explicitly granted, with clear failure modes for invalid requests.

## Memory Mount Modes

| Mode | Access | Scope | Concurrency |
|------|--------|-------|-------------|
| Per-Agent (RW) | Read-Write | Single agent only | Exclusive — only one agent may hold RW at a time |
| Shared (RO) | Read-Only | Multiple agents | Concurrent reads allowed |
| Public (RO) | Read-Only | All agents | Concurrent reads allowed |

## API Design

### Attach

```
POST /agents/{agentId}/memory/attach
{
  "memoryId": "mem-xxx",
  "mode": "per-agent" | "shared" | "public"
}
```

**Behaviour:**
1. Validate `memoryId` exists and is in a detached or compatible state.
2. Run pre-start mount checks (see below).
3. Persist attachment in agent state.
4. Return success or rejection with reason.

### Detach

```
POST /agents/{agentId}/memory/detach
{
  "memoryId": "mem-xxx"
}
```

**Behaviour:**
1. Verify agent is stopped (reject if running with RW mount).
2. Remove attachment from agent state.
3. Clean up any mount-point references.

### Status

```
GET /agents/{agentId}/memory
```

Returns current attachments with mode, path, and health status.

## Pre-Start Mount Checks

Before an agent starts, the daemon validates all attached memories:

1. **Existence check**: Memory store path/volume must exist on disk.
2. **Permission check**: File-system permissions match requested mode (RW vs RO).
3. **Path conflict check**: No two RW mounts may overlap the same path.
4. **Mode policy check**: Per-Agent RW memory must not be simultaneously attached RW to another running agent.

If any check fails, the agent start is rejected with a descriptive error:

```go
var (
    ErrMemoryNotFound     = errors.New("memory store not found")
    ErrMemoryPermission   = errors.New("insufficient permissions for requested mount mode")
    ErrMemoryPathConflict = errors.New("mount path conflicts with existing attachment")
    ErrMemoryModeViolation = errors.New("mount mode policy violation")
)
```

## State Persistence

Attachments are stored in the agent state map alongside existing fields:

```go
type MemoryAttachment struct {
    MemoryID  string    `json:"memory_id"`
    Mode      string    `json:"mode"`       // "per-agent", "shared", "public"
    MountPath string    `json:"mount_path"`
    AttachedAt time.Time `json:"attached_at"`
}
```

The existing `memoryLinks` field in `Service` will be extended or replaced by a structured `MemoryAttachment` slice per agent.

## Policy Enforcement Flow

```
Agent Start Request
        │
        ▼
  Load attachments
        │
        ▼
  For each attachment:
    ├─ Exists?        ──No──▶ Reject (ErrMemoryNotFound)
    ├─ Permissions OK? ─No──▶ Reject (ErrMemoryPermission)
    ├─ Path conflict?  ─Yes─▶ Reject (ErrMemoryPathConflict)
    └─ Mode valid?     ─No──▶ Reject (ErrMemoryModeViolation)
        │
        ▼
  All checks pass → proceed with start
```

## Observability

- All attach/detach operations are recorded in the audit log.
- Mount check failures are logged with agent ID, memory ID, and failure reason.
- `AuditBufferStatus` endpoint (see #41) provides buffer health.

## E2E Test Scenarios

1. **Happy path**: Attach Per-Agent RW → start → stop → detach.
2. **Restart after detach**: Attach → start → stop → detach → start (should work with no memory).
3. **RW conflict**: Attach RW to agent-A (running) → attempt attach RW same memory to agent-B → expect rejection.
4. **Missing memory**: Attach non-existent memory → expect rejection on start.
5. **Mode downgrade**: Attach RW → detach → re-attach as Shared RO → verify read-only access.

## Dependencies

- #78: Memory model foundation (must be merged first)
- #41: Audit buffer status endpoint (for observability)

## Implementation Phases

1. **Data model**: Add `MemoryAttachment` struct and update `Service` state.
2. **Attach/Detach APIs**: Implement with validation.
3. **Pre-start checks**: Integrate into `Start()` flow.
4. **Tests**: Unit tests for policy logic, E2E for lifecycle scenarios.
