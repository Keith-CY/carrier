# Agent Memory Binding And Delegation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement dual-mode agent memory behavior so persistent agents use live `public/shared` mounts while delegated child agents use frozen snapshots and distill results back into the parent `per_agent` memory.

**Architecture:** Add explicit lifecycle and memory-binding metadata to managed instances and orchestrator executions, then build two runtime paths on top of the existing memory store. Persistent agents reuse current attachment-based memory composition with next-turn refresh; delegated agents provision a temporary child instance, seed it with a read-only snapshot mount plus one writable `per_agent` memory, and finalize through distill, write-back, and cleanup.

**Tech Stack:** Go, Carrier gateway orchestrator, daemon lifecycle service, daemon internal memory store, existing `go test` suites, optional WebUI evidence surfacing.

---

### Task 1: Add Explicit Lifecycle And Memory-Binding Metadata

**Files:**
- Modify: `gateway/managed_instances.go`
- Modify: `gateway/webui_add.go`
- Modify: `gateway/agent_launcher_api.go`
- Modify: `gateway/webui_instances.go`
- Test: `gateway/managed_instances_more_test.go`
- Test: `gateway/webui_add_coverage_test.go`
- Test: `gateway/webui_instances_coverage_test.go`

**Step 1: Write the failing managed-instance schema tests**

Add tests that round-trip a `managedAgentInstance` with the new fields and verify defaults for user-created agents:

```go
func TestManagedInstanceRoundTripPreservesMemoryBindingFields(t *testing.T) {
	inst := managedAgentInstance{
		ID:                "openclaw-main",
		AgentID:           "openclaw",
		AgentLifecycleMode: "persistent",
		MemoryBindingMode:  "live_mount",
		PublicScopes:      []string{"public"},
		SharedScopes:      []string{"shared:team"},
		MemoryRefreshPolicy: "next_turn",
	}
	// save + load, then compare fields
}
```

**Step 2: Run the focused gateway tests and verify they fail**

Run:

```bash
go test ./gateway -run 'TestManagedInstanceRoundTripPreservesMemoryBindingFields|TestHandleWebUIAddDefaultsPersistentMemoryBinding' -count=1
```

Expected: FAIL because the new fields and defaults do not exist yet.

**Step 3: Add the new managed-instance fields and normalization**

Update `gateway/managed_instances.go` with explicit metadata and simple normalization helpers:

```go
type managedAgentInstance struct {
	// existing fields...
	AgentLifecycleMode string   `json:"agent_lifecycle_mode,omitempty"`
	MemoryBindingMode  string   `json:"memory_binding_mode,omitempty"`
	PublicScopes       []string `json:"public_scopes,omitempty"`
	SharedScopes       []string `json:"shared_scopes,omitempty"`
	PerAgentMemoryID   string   `json:"per_agent_memory_id,omitempty"`
	MemoryRefreshPolicy string  `json:"memory_refresh_policy,omitempty"`
	ParentAgentID      string   `json:"parent_agent_id,omitempty"`
	ParentExecutionID  string   `json:"parent_execution_id,omitempty"`
	TaskID             string   `json:"task_id,omitempty"`
	SnapshotID         string   `json:"snapshot_id,omitempty"`
	SnapshotDigest     string   `json:"snapshot_digest,omitempty"`
	DistillTarget      string   `json:"distill_target,omitempty"`
	CleanupPolicy      string   `json:"cleanup_policy,omitempty"`
}
```

Normalize empty user-created records to `persistent/live_mount/next_turn`.

**Step 4: Default user-created instances to persistent live mounts**

Update `gateway/webui_add.go` so the normal user-facing add flow creates:

```go
managedAgentInstance{
	AgentLifecycleMode:  "persistent",
	MemoryBindingMode:   "live_mount",
	MemoryRefreshPolicy: "next_turn",
}
```

Do not add snapshot metadata on this path.

**Step 5: Surface the metadata in launcher and instance APIs**

Update `gateway/agent_launcher_api.go` and `gateway/webui_instances.go` to return the new fields in summaries so later slices can inspect them.

**Step 6: Re-run the focused gateway tests**

Run:

```bash
go test ./gateway -run 'TestManagedInstanceRoundTripPreservesMemoryBindingFields|TestHandleWebUIAddDefaultsPersistentMemoryBinding' -count=1
```

Expected: PASS.

**Step 7: Run a broader regression pass for the touched gateway files**

Run:

```bash
go test ./gateway -run 'TestManaged|TestHandleWebUIAdd|TestWebUIInstances' -count=1
```

Expected: PASS.

**Step 8: Commit**

```bash
git add gateway/managed_instances.go gateway/webui_add.go gateway/agent_launcher_api.go gateway/webui_instances.go gateway/managed_instances_more_test.go gateway/webui_add_coverage_test.go gateway/webui_instances_coverage_test.go
git commit -m "feat: add managed instance memory binding metadata"
```

### Task 2: Add Memory Snapshot Primitives To The Memory Store

**Files:**
- Create: `daemon/internal/memory/snapshot.go`
- Modify: `daemon/internal/memory/fusion_types.go`
- Modify: `daemon/internal/memory/fusion_store.go`
- Test: `daemon/internal/memory/snapshot_test.go`
- Test: `daemon/internal/memory/fusion_store_test.go`

**Step 1: Write the failing snapshot tests**

Create tests for:

- exporting a frozen snapshot from `public/shared` scopes
- preserving snapshot provenance and digest
- mounting the snapshot read-only for a child instance
- rejecting writes to the snapshot mount

Sketch:

```go
func TestCreateSnapshotForInstanceScopesProducesFrozenReadonlyMount(t *testing.T) {
	store := NewStore()
	// seed public/shared records
	// create snapshot
	// mount into child
	// assert digest, source scopes, readonly behavior
}
```

**Step 2: Run the focused memory tests and verify they fail**

Run:

```bash
go test ./daemon/internal/memory -run 'TestCreateSnapshotForInstanceScopesProducesFrozenReadonlyMount|TestSnapshotMountRejectsWrites' -count=1
```

Expected: FAIL because snapshot APIs do not exist yet.

**Step 3: Introduce snapshot types**

Extend `daemon/internal/memory/fusion_types.go` with explicit snapshot metadata:

```go
type SnapshotOptions struct {
	Actor        string
	RequestID    string
	SourceSubject string
	SourceScopes []Scope
	TargetInstanceID string
	Reason       string
}

type SnapshotRecord struct {
	ID           string    `json:"id"`
	Digest       string    `json:"digest"`
	SourceScopes []Scope   `json:"source_scopes"`
	TargetInstanceID string `json:"target_instance_id"`
	CreatedAt    time.Time `json:"created_at"`
}
```

**Step 4: Implement snapshot export and read-only mount helpers**

Add `daemon/internal/memory/snapshot.go` with helpers shaped like:

```go
func (s *Store) CreateSnapshotForInstance(ctx context.Context, opts SnapshotOptions) (SnapshotRecord, error)
func (s *Store) MountSnapshot(instanceID string, snapshotID string) error
func (s *Store) DeleteSnapshot(snapshotID string) error
```

Reuse existing record/export machinery where possible; avoid inventing a second storage stack.

**Step 5: Wire snapshot state into the memory store**

Update `fusion_store.go` to:

- persist snapshot metadata in the store state
- expose snapshot mounts in instance scope resolution
- keep snapshot mounts read-only
- leave the one-`per_agent`-mount policy intact

**Step 6: Re-run the focused memory tests**

Run:

```bash
go test ./daemon/internal/memory -run 'TestCreateSnapshotForInstanceScopesProducesFrozenReadonlyMount|TestSnapshotMountRejectsWrites' -count=1
```

Expected: PASS.

**Step 7: Run the broader daemon memory test suite**

Run:

```bash
go test ./daemon/internal/memory/... -count=1
```

Expected: PASS.

**Step 8: Commit**

```bash
git add daemon/internal/memory/snapshot.go daemon/internal/memory/fusion_types.go daemon/internal/memory/fusion_store.go daemon/internal/memory/snapshot_test.go daemon/internal/memory/fusion_store_test.go
git commit -m "feat: add read-only memory snapshots for delegated instances"
```

### Task 3: Refresh Persistent Agent Memory On The Next Turn

**Files:**
- Modify: `daemon/internal/lifecycle/memory.go`
- Modify: `daemon/server/server.go`
- Test: `daemon/internal/lifecycle/memory_auto_test.go`
- Test: `daemon/server/server_handlers_coverage_test.go`
- Test: `daemon/server/server_memory_test.go`

**Step 1: Write the failing next-turn refresh tests**

Add tests covering:

- persistent agent chat turn refreshes when mounted shared digest changes
- delegated agent path does not attempt live refresh
- no refresh occurs mid-turn

Sketch:

```go
func TestHandleAgentChatRefreshesPersistentMemoryContractBeforeTurn(t *testing.T) {
	// prepare svc + runtime + persistent instance metadata
	// mutate shared memory between two turns
	// assert PrepareAgentMemory runs before second turn
}
```

**Step 2: Run the focused lifecycle/server tests and verify they fail**

Run:

```bash
go test ./daemon/internal/lifecycle ./daemon/server -run 'TestHandleAgentChatRefreshesPersistentMemoryContractBeforeTurn|TestPersistentMemoryRefreshSkipsDelegatedInstances' -count=1
```

Expected: FAIL because there is no refresh hook.

**Step 3: Implement a refresh helper in lifecycle**

Add a helper in `daemon/internal/lifecycle/memory.go` shaped like:

```go
func (s *Service) RefreshPersistentAgentMemoryIfNeeded(agentID string) error
```

Behavior:

- no-op unless instance metadata says `persistent/live_mount`
- compare current mounted-scope digest with stored memory contract digest
- rebuild the effective view with `PrepareAgentMemory` when digests differ

**Step 4: Call the refresh helper before each new agent turn**

Update `daemon/server/server.go` so `handleAgentChat` triggers refresh before invoking runtime chat. Keep the refresh boundary outside the actual provider turn.

**Step 5: Re-run the focused tests**

Run:

```bash
go test ./daemon/internal/lifecycle ./daemon/server -run 'TestHandleAgentChatRefreshesPersistentMemoryContractBeforeTurn|TestPersistentMemoryRefreshSkipsDelegatedInstances' -count=1
```

Expected: PASS.

**Step 6: Run broader lifecycle/server regressions**

Run:

```bash
go test ./daemon/internal/lifecycle -run 'Test.*Memory' -count=1
go test ./daemon/server -run 'TestHandleAgentChat|Test.*Memory' -count=1
```

Expected: PASS.

**Step 7: Commit**

```bash
git add daemon/internal/lifecycle/memory.go daemon/server/server.go daemon/internal/lifecycle/memory_auto_test.go daemon/server/server_handlers_coverage_test.go daemon/server/server_memory_test.go
git commit -m "feat: refresh persistent agent memory on next turn"
```

### Task 4: Extend Orchestrator Execution State For Delegated Memory Flow

**Files:**
- Modify: `gateway/orchestrator_types.go`
- Modify: `gateway/templates_api.go`
- Modify: `gateway/orchestrator_plan_api.go`
- Modify: `gateway/delegate.go`
- Test: `gateway/orchestrator_types_more_test.go`
- Test: `gateway/orchestrator_api_more_test.go`
- Test: `gateway/delegate_command_test.go`

**Step 1: Write the failing orchestrator state tests**

Add tests for:

- new delegated-memory execution fields normalize correctly
- memory digest derives from the snapshot source scopes when absent
- delegate-created executions persist the new state machine fields

Sketch:

```go
func TestNormalizeOrchestratorExecutionStoresDelegatedMemoryFields(t *testing.T) {
	in := OrchestratorExecution{
		AgentLifecycleMode: " delegated ",
		MemoryBindingMode:  " snapshot ",
		SourceScopes:       []string{" shared:team "},
	}
	// normalize and assert trimmed/deduped fields
}
```

**Step 2: Run the focused orchestrator tests and verify they fail**

Run:

```bash
go test ./gateway -run 'TestNormalizeOrchestratorExecutionStoresDelegatedMemoryFields|TestDelegateExecutionSeedsDelegatedMemoryState' -count=1
```

Expected: FAIL because the fields do not exist yet.

**Step 3: Add delegated-memory execution fields**

Extend `gateway/orchestrator_types.go` with fields such as:

```go
AgentLifecycleMode string   `json:"agentLifecycleMode,omitempty"`
MemoryBindingMode  string   `json:"memoryBindingMode,omitempty"`
SourceScopes       []string `json:"sourceScopes,omitempty"`
SnapshotID         string   `json:"snapshotId,omitempty"`
SnapshotDigest     string   `json:"snapshotDigest,omitempty"`
ChildAgentID       string   `json:"childAgentId,omitempty"`
ChildPerAgentMemoryID string `json:"childPerAgentMemoryId,omitempty"`
DistillRunID       string   `json:"distillRunId,omitempty"`
CleanupStatus      string   `json:"cleanupStatus,omitempty"`
```

Normalize and persist them the same way the existing memory metadata is normalized.

**Step 4: Populate the fields in plan/template/delegate creation**

Update `gateway/templates_api.go`, `gateway/orchestrator_plan_api.go`, and `gateway/delegate.go` so delegated execution records are born with:

- `agentLifecycleMode = delegated`
- `memoryBindingMode = snapshot`
- `sourceScopes = requiredMemory`
- empty `snapshot/child/distill/cleanup` fields ready to be filled later

Do not change persistent user-created agent flows here.

**Step 5: Re-run the focused gateway tests**

Run:

```bash
go test ./gateway -run 'TestNormalizeOrchestratorExecutionStoresDelegatedMemoryFields|TestDelegateExecutionSeedsDelegatedMemoryState' -count=1
```

Expected: PASS.

**Step 6: Run a broader orchestrator regression pass**

Run:

```bash
go test ./gateway -run 'TestOrchestrator|TestDelegate' -count=1
```

Expected: PASS.

**Step 7: Commit**

```bash
git add gateway/orchestrator_types.go gateway/templates_api.go gateway/orchestrator_plan_api.go gateway/delegate.go gateway/orchestrator_types_more_test.go gateway/orchestrator_api_more_test.go gateway/delegate_command_test.go
git commit -m "feat: track delegated memory state in orchestrator executions"
```

### Task 5: Provision Delegated Child Instances With Snapshot And Writable Memory

**Files:**
- Create: `gateway/delegated_memory_runtime.go`
- Modify: `gateway/orchestrator_api.go`
- Modify: `gateway/managed_instances.go`
- Test: `gateway/delegated_memory_runtime_test.go`
- Test: `gateway/orchestrator_local_test.go`

**Step 1: Write the failing delegated provisioning tests**

Create tests for:

- temporary child instance creation
- snapshot creation from source scopes
- child metadata persisted into managed instances and orchestrator execution state
- exactly one writable `per_agent` memory created for the child

Sketch:

```go
func TestProvisionDelegatedChildCreatesSnapshotAndWritablePerAgentMemory(t *testing.T) {
	// seed execution with requiredMemory
	// provision child
	// assert child instance metadata, snapshot id/digest, per-agent memory id
}
```

**Step 2: Run the focused provisioning tests and verify they fail**

Run:

```bash
go test ./gateway -run 'TestProvisionDelegatedChildCreatesSnapshotAndWritablePerAgentMemory' -count=1
```

Expected: FAIL because the provisioning helper does not exist yet.

**Step 3: Implement delegated provisioning helpers**

Create `gateway/delegated_memory_runtime.go` with helpers shaped like:

```go
func provisionDelegatedChild(ctx context.Context, execution *OrchestratorExecution, task *OrchestratorTaskUnit) error
func buildDelegatedChildInstance(execution OrchestratorExecution, task OrchestratorTaskUnit) managedAgentInstance
```

Provisioning responsibilities:

- create child instance id
- create child writable `per_agent` memory
- create snapshot from `execution.SourceScopes`
- mount snapshot read-only into child
- persist child instance metadata
- update execution with `childAgentID`, `snapshotID`, `snapshotDigest`, `childPerAgentMemoryID`

**Step 4: Hook provisioning into the local orchestrator worker path**

Update `gateway/orchestrator_api.go` so the local execution path provisions a delegated child before the child task run starts and uses the child agent/session metadata when invoking the task.

**Step 5: Re-run the focused gateway tests**

Run:

```bash
go test ./gateway -run 'TestProvisionDelegatedChildCreatesSnapshotAndWritablePerAgentMemory|TestLocalOrchestratorRunProvisionsDelegatedChild' -count=1
```

Expected: PASS.

**Step 6: Run broader local orchestrator regressions**

Run:

```bash
go test ./gateway -run 'TestOrchestratorLocal|TestProvisionDelegated' -count=1
```

Expected: PASS.

**Step 7: Commit**

```bash
git add gateway/delegated_memory_runtime.go gateway/orchestrator_api.go gateway/managed_instances.go gateway/delegated_memory_runtime_test.go gateway/orchestrator_local_test.go
git commit -m "feat: provision delegated child instances with snapshots"
```

### Task 6: Finalize Delegated Children Through Distill, Write-Back, And Cleanup

**Files:**
- Modify: `gateway/delegated_memory_runtime.go`
- Modify: `gateway/orchestrator_api.go`
- Modify: `gateway/orchestrator_evidence_api.go`
- Modify: `daemon/internal/memory/fusion_store.go`
- Test: `gateway/delegated_memory_runtime_test.go`
- Test: `gateway/orchestrator_evidence_api_test.go`
- Test: `daemon/internal/memory/fusion_store_test.go`

**Step 1: Write the failing finalize tests**

Add tests for:

- distilling child `per_agent` memory into parent `per_agent`
- provenance on the write-back record
- deleting child raw memory and snapshot after successful distill
- `cleanup_pending` when cleanup fails after distill succeeds
- idempotent finalize retry

Sketch:

```go
func TestFinalizeDelegatedChildDistillsIntoParentPerAgentMemory(t *testing.T) {
	// seed child memory
	// run finalize
	// assert parent record provenance + child cleanup
}
```

**Step 2: Run the focused finalize tests and verify they fail**

Run:

```bash
go test ./gateway ./daemon/internal/memory -run 'TestFinalizeDelegatedChildDistillsIntoParentPerAgentMemory|TestFinalizeDelegatedChildSetsCleanupPending' -count=1
```

Expected: FAIL because finalize logic does not exist yet.

**Step 3: Add a parent write-back helper in the memory store**

Extend `daemon/internal/memory/fusion_store.go` with a helper shaped like:

```go
func (s *Store) WriteDistilledResultToParent(parentInstanceID string, rec MemoryRecord, provenance map[string]string) (string, error)
```

Make sure the resulting record lands in the parent `agent:<parent>` scope and carries provenance for:

- parent agent id
- child agent id
- task id
- snapshot digest
- distill run id

**Step 4: Implement delegated finalize logic**

Extend `gateway/delegated_memory_runtime.go` with:

```go
func finalizeDelegatedChild(ctx context.Context, execution *OrchestratorExecution) error
```

Responsibilities:

- run `DistillForInstance` on the child
- write distilled records to the parent `per_agent`
- update `distillRunID` and `cleanupStatus`
- delete raw child `per_agent` memory
- delete snapshot
- destroy the child instance
- mark `cleanup_pending` instead of rolling back when cleanup fails after successful distill

**Step 5: Call finalize after successful task completion**

Update `gateway/orchestrator_api.go` so successful delegated task completion triggers finalize before the execution is marked completed.

**Step 6: Surface the new fields in evidence export**

Update `gateway/orchestrator_evidence_api.go` to include the delegated memory lifecycle fields in exported evidence bundles.

**Step 7: Re-run the focused tests**

Run:

```bash
go test ./gateway ./daemon/internal/memory -run 'TestFinalizeDelegatedChildDistillsIntoParentPerAgentMemory|TestFinalizeDelegatedChildSetsCleanupPending' -count=1
```

Expected: PASS.

**Step 8: Run broader evidence and gateway regressions**

Run:

```bash
go test ./gateway -run 'TestOrchestrator|TestDelegate|TestExecutionEvidence' -count=1
go test ./daemon/internal/memory/... -count=1
```

Expected: PASS.

**Step 9: Commit**

```bash
git add gateway/delegated_memory_runtime.go gateway/orchestrator_api.go gateway/orchestrator_evidence_api.go daemon/internal/memory/fusion_store.go gateway/delegated_memory_runtime_test.go gateway/orchestrator_evidence_api_test.go daemon/internal/memory/fusion_store_test.go
git commit -m "feat: distill delegated child memory back to parent"
```

### Task 7: Surface Delegated Memory Lifecycle In The UI

**Files:**
- Modify: `webui/src/features/executions/components/ExecutionGovernanceBlock.tsx`
- Modify: `webui/src/features/executions/ExecutionDetailContent.test.tsx`
- Modify: `webui/e2e/tests/agent-detail.spec.ts`
- Modify: `webui/e2e/tests/fullstack-memory.spec.ts`

**Step 1: Write the failing UI tests**

Add assertions for:

- `agentLifecycleMode`
- `memoryBindingMode`
- snapshot digest
- child agent id
- distill run id
- cleanup status

Sketch:

```tsx
expect(screen.getByText(/memory binding: snapshot/i)).toBeInTheDocument()
expect(screen.getByText(/cleanup pending/i)).toBeInTheDocument()
```

**Step 2: Run the focused WebUI tests and verify they fail**

Run:

```bash
bun test webui/src/features/executions/ExecutionDetailContent.test.tsx
```

Expected: FAIL because the new metadata is not rendered.

**Step 3: Render the delegated memory lifecycle block**

Update `ExecutionGovernanceBlock.tsx` to render:

- lifecycle mode
- binding mode
- source scopes
- snapshot id/digest
- child agent id
- distill run id
- cleanup status

Keep the existing memory-contract section, but extend it rather than duplicating it.

**Step 4: Re-run the focused WebUI tests**

Run:

```bash
bun test webui/src/features/executions/ExecutionDetailContent.test.tsx
```

Expected: PASS.

**Step 5: Run the related E2E coverage**

Run:

```bash
bun test webui/e2e/tests/agent-detail.spec.ts
bun test webui/e2e/tests/fullstack-memory.spec.ts
```

Expected: PASS.

**Step 6: Commit**

```bash
git add webui/src/features/executions/components/ExecutionGovernanceBlock.tsx webui/src/features/executions/ExecutionDetailContent.test.tsx webui/e2e/tests/agent-detail.spec.ts webui/e2e/tests/fullstack-memory.spec.ts
git commit -m "feat: surface delegated memory lifecycle in execution ui"
```

### Task 8: Final Verification And Docs Sync

**Files:**
- Modify: `docs/plans/2026-03-16-agent-memory-binding-and-delegation-design.md`
- Modify: `docs/plans/2026-03-16-agent-memory-binding-and-delegation-implementation.md`

**Step 1: Run the complete backend verification pass**

Use `@superpowers:verification-before-completion`.

Run:

```bash
go test ./gateway/... ./daemon/internal/memory/... ./daemon/internal/lifecycle/... ./daemon/server/... -count=1
```

Expected: PASS.

**Step 2: Run the complete WebUI verification pass if Task 7 shipped**

Run:

```bash
bun test webui/src/features/executions/ExecutionDetailContent.test.tsx
```

Expected: PASS.

**Step 3: Update the design doc with any implementation-driven deviations**

If implementation differs from the original design, record the final decision in the design doc instead of leaving stale intent behind.

**Step 4: Re-read the implementation plan and mark follow-up work**

Add short notes for anything intentionally deferred, especially:

- remote worker delegated-child provisioning
- snapshot garbage collection
- richer distill summarizer integration

**Step 5: Commit**

```bash
git add docs/plans/2026-03-16-agent-memory-binding-and-delegation-design.md docs/plans/2026-03-16-agent-memory-binding-and-delegation-implementation.md
git commit -m "docs: sync agent memory binding implementation notes"
```

## Execution Notes

- Follow `@superpowers:test-driven-development` for each task: write the failing test first, then the minimal code to pass it.
- Keep commits small and aligned to the task boundaries above.
- Prefer introducing thin helper files over growing `gateway/orchestrator_api.go` and `daemon/internal/memory/fusion_store.go` into larger god files.
- Execute this plan from a dedicated worktree even though the plan itself was authored in the current workspace.
