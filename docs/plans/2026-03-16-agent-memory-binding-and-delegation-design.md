# Carrier Agent Memory Binding And Delegation Design

**Scope:** Define how Carrier should provision memory for two agent creation modes:

- user-created long-lived agents that stay alive until explicitly destroyed
- Carrier-created delegated child agents that exist only to complete a subtask

## Goal

Give Carrier a consistent memory model that supports both live shared knowledge and isolated task execution.

The system must support two distinct behaviors:

1. Persistent agents must see live `public` and `shared` memory mounts so updates become visible to every mounted agent.
2. Delegated child agents must run against a frozen snapshot of `public` and `shared` memory so each child executes independently and can be audited against the exact input it received.

In both cases, every agent still needs its own writable `per_agent` memory.

## Non-Goals

- Replacing the existing Carrier memory platform with a separate memory service.
- Making `spawn_subagent` the primary control surface for rich delegation workflows.
- Supporting writable `shared` memory for delegated children in v1.
- Preserving raw delegated child memory after successful distill.
- Introducing mid-run live memory refresh inside one execution turn.

## Requirements

### Persistent Agent Requirements

- Agent is user-created and remains alive until explicitly destroyed.
- Agent memory view is `public + shared + per_agent`.
- `public` and `shared` are mounted live, not copied.
- Shared updates must become visible to all mounted persistent agents.
- Agent keeps one private writable `per_agent` memory.

### Delegated Agent Requirements

- Agent is created by Carrier for a specific subtask.
- Agent executes independently from later shared-memory changes.
- Agent receives a frozen snapshot of selected `public` and `shared` scopes.
- Agent writes only to its own `per_agent` memory.
- On successful completion, the child `per_agent` memory is distilled and summarized.
- Distilled results are written back into the parent agent's `per_agent` memory.
- Raw child memory is deleted after distill succeeds.

## Current Carrier Baseline

Carrier already has the core primitives needed for this design:

- memory types: `public`, `shared`, `per_agent`
- instance and agent scope attachment
- runtime memory contract preparation
- instance import/export flows
- instance-level distill with provenance
- orchestrator and delegate execution flows
- managed instance records and execution evidence

Carrier also has an important constraint in the current memory policy: one instance can mount at most one `per_agent` memory. That means a delegated child cannot represent both its frozen input baseline and its writable work area as two separate `per_agent` attachments. The snapshot must therefore be represented as a separate read-only snapshot mount, while the child keeps exactly one writable `per_agent` memory.

## Design Overview

Carrier should adopt a dual-mode memory binding model.

### Mode 1: Persistent Agent

- `agentLifecycleMode = persistent`
- `memoryBindingMode = live_mount`
- `public` and `shared` scopes are mounted live
- `per_agent` remains private and writable

### Mode 2: Delegated Agent

- `agentLifecycleMode = delegated`
- `memoryBindingMode = snapshot`
- selected `public` and `shared` scopes are materialized into a frozen snapshot
- snapshot is mounted read-only for the child
- child receives one writable `per_agent` memory
- child results are distilled back into the parent `per_agent`

This keeps the semantics explicit:

- persistent agents are knowledge subscribers
- delegated agents are isolated task workers

## Why This Must Not Be Hidden Inside `spawn_subagent`

The current `spawn_subagent` surface only carries a bounded task string and a job handle. It is suitable for light delegation bookkeeping but not for the full workflow needed here.

This design requires the delegation plane to carry:

- parent agent id
- parent execution id
- task id
- child instance id
- source scopes
- snapshot id and digest
- distill target
- cleanup policy
- retry and finalize state

That belongs in the orchestrator and managed-instance layers, not in the minimal in-memory subagent handle.

## Persistent Agent Model

Persistent agents should use live memory mounts.

### Memory Composition

The runtime memory view is:

- mounted `public` scopes
- mounted `shared` scopes
- one writable `per_agent` memory owned by the agent

### Refresh Semantics

Persistent agents should use next-turn consistency.

That means:

- shared/public changes are not injected into a run already in flight
- before each new chat turn or execution turn, Carrier checks the mounted-scope contract digest
- if the digest has changed, Carrier rebuilds the effective memory view before the next turn starts

This preserves deterministic behavior within a single turn while still making shared updates visible quickly.

### Why Next-Turn Consistency

Immediate mid-turn refresh would make outputs non-deterministic and harder to debug. Next-turn refresh gives a stable boundary:

- one turn sees one memory contract
- the next turn sees the refreshed contract

## Delegated Agent Model

Delegated agents should use frozen snapshots.

### Memory Composition

The child memory view is:

- one read-only delegation snapshot mount, built from selected `public` and `shared` scopes
- one writable child `per_agent` memory

The child does not get live mounts to upstream shared scopes.

### Snapshot Representation

In v1, the delegation snapshot should be modeled as a synthetic read-only snapshot mount, not as a second `per_agent` memory. This aligns with the current policy constraint that limits each instance to a single `per_agent` mount.

Implementation-wise, the snapshot can be represented as:

- an ephemeral read-only memory package owned by the delegation pipeline, or
- a dedicated snapshot artifact imported as a read-only shared-like mount

The important property is behavioral, not naming:

- the child can read it
- the child cannot write it
- the child does not observe later shared-memory changes

### Write Rules

Delegated children may write only to their own `per_agent` memory.

They must not:

- mutate the snapshot
- write directly into parent memory
- write directly into source shared scopes

## Delegation Data Flow

The delegated child lifecycle is:

1. Parent execution decomposes work into task units.
2. Orchestrator selects source `public/shared` scopes for the child.
3. Carrier exports those scopes into a frozen delegation snapshot.
4. Carrier creates a temporary child instance.
5. Carrier creates or attaches the child writable `per_agent` memory.
6. Carrier mounts the snapshot read-only into the child.
7. Child runs the assigned task.
8. Child `per_agent` memory is distilled after task completion.
9. Distilled output is written into the parent `per_agent` memory.
10. Raw child `per_agent` memory is deleted.
11. Delegation snapshot is deleted.
12. Child instance is destroyed.

This makes the direction of information flow explicit:

- upstream knowledge goes into the child as frozen input
- child conclusions come back only through distill

## State Model

Delegated work should use explicit recovery states.

### Delegation States

- `plan_created`
- `snapshot_ready`
- `child_created`
- `child_seeded`
- `child_running`
- `distill_completed`
- `child_cleaned`
- `cleanup_pending`
- `failed`

### Recovery Rules

- If failure happens before `child_created`, the execution stops with no child cleanup needed.
- If failure happens after `child_created` but before `distill_completed`, the run is failed and may be retried or finalized later.
- If `distill_completed` succeeds but cleanup fails, the run is marked `cleanup_pending`. The business result stands; cleanup is retried later.
- Distill must be idempotent so finalize can be retried safely.

## Data Model Changes

Carrier should make the memory behavior explicit in managed instance and execution state.

### Managed Instance Fields

Add the following fields to managed instance records:

- `agentLifecycleMode`: `persistent | delegated`
- `memoryBindingMode`: `live_mount | snapshot`
- `publicScopes`
- `sharedScopes`
- `perAgentMemoryID`
- `memoryRefreshPolicy`
- `parentAgentID`
- `parentExecutionID`
- `taskID`
- `snapshotID`
- `snapshotDigest`
- `distillTarget`
- `cleanupPolicy`

### Execution Evidence Fields

Delegation evidence should record:

- `parentAgentID`
- `childAgentID`
- `parentExecutionID`
- `taskID`
- `sourceScopes`
- `snapshotID`
- `snapshotDigest`
- `childPerAgentMemoryID`
- `distillRunID`
- `distillTarget`
- `cleanupStatus`

### Distill Provenance Fields

Every distilled write-back into the parent `per_agent` memory should carry provenance linking it to:

- parent agent
- child agent
- task id
- snapshot digest
- distill run id

This lets Carrier explain exactly where each promoted memory record came from.

## API Shape

### Persistent Agent Creation

User-facing create flows should accept:

- `lifecycleMode = persistent`
- mounted `public` scopes
- mounted `shared` scopes

Persistent creation should not clone those scopes. It stores live mount intent and prepares the first runtime contract.

### Delegated Child Provisioning

Delegated child creation should remain an internal orchestration API.

It should accept:

- `parentAgentID`
- `parentExecutionID`
- `taskID`
- `sourceScopes`
- `distillTarget = parent_per_agent`
- `cleanupPolicy = delete_raw_after_distill`

It should return:

- `childAgentID`
- `snapshotID`
- `snapshotDigest`
- `childPerAgentMemoryID`

### Delegated Child Finalize

Carrier should add a finalize path that performs:

1. distill child `per_agent`
2. write distilled result into parent `per_agent`
3. delete raw child memory
4. delete snapshot
5. destroy child instance

This path must be safe to retry.

## Memory Semantics

### Public And Shared For Persistent Agents

- mounted live
- refreshed on the next turn when digest changes
- never copied by default

### Public And Shared For Delegated Agents

- exported into a task-scoped snapshot
- mounted read-only into the child
- detached from future upstream updates

### Per-Agent Memory For Persistent Agents

- long-lived
- owned by the agent
- preserved until the agent is destroyed

### Per-Agent Memory For Delegated Agents

- writable scratch space for that child
- used as the input to distill
- deleted after distill succeeds

## Implementation Slices

### Slice 1: Managed Instance Schema

Extend managed instance state and related API DTOs with lifecycle and memory-binding metadata.

Primary impact areas:

- managed instance storage
- launcher summaries
- instance creation APIs

### Slice 2: Persistent Agent Live Refresh

Add next-turn memory refresh for persistent agents.

Primary behavior:

- detect mounted-scope digest changes before a new run or chat turn
- rebuild effective memory contract when needed

Primary impact areas:

- runtime entrypoints for chat and execution
- lifecycle memory preparation
- managed instance memory status reporting

### Slice 3: Delegation Snapshot Pipeline

Add a new orchestration pipeline for delegated child provisioning and finalize.

Primary behavior:

- export snapshot from source scopes
- create child instance
- mount snapshot read-only
- run child
- distill child
- write back to parent
- clean up child

Primary impact areas:

- orchestrator execution flow
- delegate execution flow
- managed instance lifecycle

### Slice 4: Snapshot And Provenance Tooling

Add explicit memory snapshot primitives and write-back provenance.

Primary behavior:

- create snapshot artifacts from selected scopes
- attach snapshot mount to child
- persist provenance for distill write-back
- support cleanup and evidence inspection

Primary impact areas:

- memory store APIs
- distill outputs
- execution evidence and UI

## Testing Strategy

### Persistent Agent Tests

Verify that:

- persistent agents mount `public/shared` live
- shared updates become visible on the next turn
- the same turn does not see mid-run memory drift

### Delegated Agent Tests

Verify that:

- child sees the exact snapshot created at delegation time
- later shared-memory changes do not affect the child
- child can only write to its own `per_agent` memory
- distilled output is written to the parent `per_agent`
- raw child memory is deleted after successful distill

### Failure Recovery Tests

Verify:

- failure before child creation
- failure after child creation but before distill
- failure after distill but before cleanup
- idempotent finalize retry

## Rollout Notes

This design should ship in phases:

1. schema and evidence fields
2. persistent live-mount refresh
3. delegated snapshot provisioning
4. delegated distill and cleanup
5. UI and evidence surfacing

This keeps the persistent-agent behavior useful early while limiting delegated-child complexity to a later slice.

## Decision Summary

Carrier should support two explicit memory-binding modes:

- `persistent` agents use live `public/shared` mounts plus one writable `per_agent`
- `delegated` agents use a frozen read-only snapshot plus one writable child `per_agent`

Delegated child output should be promoted only through distill into the parent `per_agent` memory. Raw child memory should be deleted after successful distill. This gives Carrier a clear model for live shared knowledge, isolated subtask execution, and auditable memory promotion.
