# Work-Oriented Orchestration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add first-class work-oriented orchestration to Carrier with local native work items, supervised runs, canonical project worktrees, and GitHub import/publish adapters, while reusing the existing execution plane.

**Architecture:** Introduce `Project`, `WorkItem`, and `Run` as new control-plane objects. `Run` binds to one primary `Execution`, which continues to own planner task units, workers, policy, memory, and evidence. Local storage is split into `~/.carrier/app`, `~/.carrier/projects`, and `~/.carrier/works`. WebUI and CLI are expanded around the new work model rather than exposing planner internals.

**Tech Stack:** Go (`gateway`, `daemon`, `baseagent`, `shared`), React WebUI, existing Carrier execution/evidence/audit model, git worktrees, local gh CLI / PAT integration.

---

## Canonical Types

### `Project`

```json
{
  "id": "proj_xxx",
  "name": "carrier",
  "sourceType": "git|github|local",
  "sourceRef": "git@github.com:org/repo.git",
  "defaultBranch": "main",
  "workflowPath": "WORKFLOW.md",
  "workflowDigest": "sha256:...",
  "state": "registered|syncing|ready|drifted|error|archived",
  "lastSyncAt": "RFC3339",
  "lastSyncError": "string"
}
```

### `WorkItem`

```json
{
  "id": "work_xxx",
  "projectId": "proj_xxx",
  "title": "string",
  "description": "string",
  "acceptance": ["string"],
  "priority": "low|normal|high|urgent",
  "source": "local|github",
  "sourceRef": "issue:123|pr:88|local:manual",
  "labels": ["string"],
  "state": "new|triaged|queued|claimed|running|blocked|awaiting_review|done|cancelled",
  "claimedByRunId": "run_xxx",
  "latestRunId": "run_xxx",
  "createdAt": "RFC3339",
  "updatedAt": "RFC3339"
}
```

### `Run`

```json
{
  "id": "run_xxx",
  "projectId": "proj_xxx",
  "workItemId": "work_xxx",
  "executionId": "exec_xxx",
  "workspaceId": "ws_xxx",
  "workspacePath": "~/.carrier/projects/<project>/worktrees/<run>",
  "backend": "local_sandboxed|managed_isolated|remote_vm",
  "phase": "created|preparing|ready|executing|verifying|publishing|completed|failed|cancelled|stale",
  "leaseOwner": "carrier:<host>",
  "leaseExpiresAt": "RFC3339",
  "verificationStatus": "pending|passed|failed|skipped",
  "publishStatus": "pending|published|failed|skipped",
  "workflowDigest": "sha256:...",
  "createdAt": "RFC3339",
  "updatedAt": "RFC3339"
}
```

### `OrchestratorExecution` Extension

```json
{
  "mode": "task|work",
  "work": {
    "projectId": "proj_xxx",
    "workItemId": "work_xxx",
    "runId": "run_xxx",
    "workspaceId": "ws_xxx",
    "workflowDigest": "sha256:...",
    "phase": "executing|verifying|publishing"
  }
}
```

## State Transition Tables

### Project

| From | Event | To |
|---|---|---|
| `registered` | clone or mirror started | `syncing` |
| `syncing` | sync succeeded | `ready` |
| `syncing` | sync failed | `error` |
| `ready` | upstream/base drift detected | `drifted` |
| `drifted` | sync succeeded | `ready` |
| `error` | retry succeeded | `ready` |
| `ready` | archive requested | `archived` |

### WorkItem

| From | Event | To |
|---|---|---|
| `new` | triage complete | `triaged` |
| `triaged` | queued for execution | `queued` |
| `queued` | claim accepted | `claimed` |
| `claimed` | run entered prepare/execute | `running` |
| `running` | active run blocked | `blocked` |
| `running` | publish complete, human review needed | `awaiting_review` |
| `running` | terminal success without review gate | `done` |
| `awaiting_review` | explicit completion | `done` |
| `blocked` | retry or resume scheduled | `queued` |
| any non-terminal | cancel requested | `cancelled` |

### Run

| From | Event | To |
|---|---|---|
| `created` | lease + workspace provisioning | `preparing` |
| `preparing` | workflow/init complete | `ready` |
| `ready` | primary execution created | `executing` |
| `executing` | execution terminal success | `verifying` |
| `executing` | execution terminal failure | `failed` |
| `verifying` | verification passed | `publishing` |
| `verifying` | verification failed | `failed` |
| `publishing` | publish passed | `completed` |
| `publishing` | publish failed | `failed` |
| active phase | lease timed out | `stale` |
| `stale` | reclaim succeeded | `preparing` |
| active phase | cancel requested | `cancelled` |

### Aggregation Rules

- A `WorkItem` may have many runs, but only one active run at once.
- `WorkItem.state` is derived from the newest active run unless explicitly overridden by an operator action.
- `Execution` failure does not automatically imply `WorkItem` cancellation.
- `blocked` means intervention is required; it is not a terminal archive state.

## Local Storage Layout

### `~/.carrier/app`

System state:

- config
- credentials
- managed instance store
- remote-control store
- global indexes

### `~/.carrier/projects`

Project substrate:

- canonical bare repo or checkout
- per-project sync metadata
- derived worktrees
- workflow contract snapshots

Recommended layout:

```text
~/.carrier/projects/
  <project-id>/
    repo/
    worktrees/
    workflow/
```

### `~/.carrier/works`

Work-oriented control data:

```text
~/.carrier/works/
  <project-id>/
    items/
      <work-item-id>.json
    runs/
      <run-id>.json
    events/
      <work-item-id>.jsonl
    publish/
      <run-id>/
    verification/
      <run-id>/
```

## Gateway API Plan

### Projects
- `GET /api/v1/work/projects`
- `POST /api/v1/work/projects`
- `GET /api/v1/work/projects/:id`
- `POST /api/v1/work/projects/:id/sync`
- `POST /api/v1/work/projects/:id/archive`

### Work Items
- `GET /api/v1/work/items`
- `POST /api/v1/work/items`
- `GET /api/v1/work/items/:id`
- `PATCH /api/v1/work/items/:id`
- `POST /api/v1/work/items/:id/claim`
- `POST /api/v1/work/items/:id/cancel`
- `POST /api/v1/work/items/:id/complete`

### Runs
- `GET /api/v1/work/runs`
- `POST /api/v1/work/items/:id/runs`
- `GET /api/v1/work/runs/:id`
- `POST /api/v1/work/runs/:id/resume`
- `POST /api/v1/work/runs/:id/cancel`
- `POST /api/v1/work/runs/:id/reclaim`
- `POST /api/v1/work/runs/:id/cleanup`

### GitHub Adapter
- `POST /api/v1/work/adapters/github/import`
- `POST /api/v1/work/adapters/github/publish`

### Response Envelope Style

Follow existing gateway conventions:

```json
{
  "requestId": "req_xxx",
  "result": "ok",
  "project": {}
}
```

Errors continue to use the existing gateway error envelope.

## CLI Plan

### Projects
- `carrier work projects add`
- `carrier work projects list`
- `carrier work projects show <project_id>`
- `carrier work projects sync <project_id>`

### Work Items
- `carrier work items create`
- `carrier work items list`
- `carrier work items show <work_item_id>`
- `carrier work items claim <work_item_id>`
- `carrier work items update <work_item_id>`
- `carrier work items cancel <work_item_id>`
- `carrier work items complete <work_item_id>`

### Runs
- `carrier work runs start <work_item_id>`
- `carrier work runs list`
- `carrier work runs show <run_id>`
- `carrier work runs resume <run_id>`
- `carrier work runs cancel <run_id>`
- `carrier work runs reclaim <run_id>`
- `carrier work runs cleanup <run_id>`

### GitHub Adapter
- `carrier work github import`
- `carrier work github publish`

## WebUI Information Architecture

### Navigation
Add a new top-level route:

- `Work`

Do not bury this under `Dashboard` or `Executions`.

### Routes
- `/work`
- `/work/projects`
- `/work/projects/:projectId`
- `/work/items`
- `/work/items/:workItemId`
- `/work/runs`
- `/work/runs/:runId`

### Work Home
Three primary panels:

1. `Projects`
2. `Queue`
3. `Active Runs`

### Project Detail
Show:
- source repo
- default branch
- workflow digest
- sync status
- imported and local work items
- recent runs

Actions:
- `Sync Project`
- `Create Work Item`
- `Import from GitHub`

### WorkItem Detail
Show:
- title
- description
- acceptance
- source metadata
- priority and labels
- latest run summary
- linked execution ids
- publish summary

Actions:
- `Claim`
- `Start Run`
- `Resume`
- `Cancel`
- `Mark Done`

Do not show planner internals by default.

### Run Detail
Show:
- phase timeline
- backend
- workspace path
- lease and heartbeat
- verification summary
- publish summary
- evidence and artifacts
- cleanup state

Actions:
- `Resume`
- `Cancel`
- `Reclaim`
- `Cleanup Workspace`
- `Open Execution`

### Execution Detail Extension
Existing execution detail gets a `Work Context` card with:
- project
- work item
- run
- workspace
- workflow digest
- publish state

## Isolation Backend Selection

### Default
- `local_sandboxed`
- dedicated git worktree
- workspace-root tool confinement

### Escalation
Use `managed_isolated` or `remote_vm` when:
- policy requires stronger isolation
- run needs tighter filesystem or network restrictions
- concurrent run density is high
- tool behavior is higher risk than local sandbox policy allows

### Required Per-Run Separation
Regardless of backend, each run must isolate:
- workspace path
- process group
- temp directories
- logs
- artifacts
- credentials scope
- heartbeat / lease identity

## Evidence And Audit Changes

### New Artifact Kinds
- `workflow_snapshot`
- `work_item_snapshot`
- `verification_report`
- `publish_record`
- `workspace_manifest`

### Execution Evidence Extensions
Execution evidence should include:
- work item metadata snapshot
- run metadata snapshot
- workspace id/path
- selected backend
- workflow digest
- publish outputs
- verification status

## Implementation Tasks

### Task 1: Shared Work Types And Path Resolution

**Files:**
- Create: `shared/work/types.go`
- Create: `shared/work/paths.go`
- Test: `shared/work/types_test.go`
- Test: `shared/work/paths_test.go`

**Step 1: Write failing tests**

Add coverage for:
- path resolution to `~/.carrier/app|projects|works`
- schema normalization for `Project`, `WorkItem`, and `Run`

**Step 2: Run the targeted tests**

Run: `go test ./shared/... -run 'TestWork.*|TestCarrierPaths.*' -count=1`
Expected: FAIL because work types and path helpers do not exist.

**Step 3: Write minimal implementation**

Implement:
- canonical work structs
- canonical local path resolvers
- no compatibility fallback to legacy root layout

**Step 4: Run the targeted tests again**

Run: `go test ./shared/... -run 'TestWork.*|TestCarrierPaths.*' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add shared/work/types.go shared/work/paths.go shared/work/types_test.go shared/work/paths_test.go
git commit -m "feat: add work model types and storage paths"
```

### Task 2: Project Store And Canonical Repo Substrate

**Files:**
- Create: `gateway/work_projects_store.go`
- Create: `daemon/internal/workspace/project_repo.go`
- Test: `gateway/work_projects_store_test.go`
- Test: `daemon/internal/workspace/project_repo_test.go`

**Step 1: Write failing tests**

Add coverage for:
- project registration
- sync state transitions
- canonical repo path creation

**Step 2: Run the targeted tests**

Run: `go test ./gateway/... ./daemon/... -run 'TestWorkProject.*|TestProjectRepo.*' -count=1`
Expected: FAIL because no project substrate exists.

**Step 3: Write minimal implementation**

Implement:
- project store
- repo sync metadata
- canonical repo root under `~/.carrier/projects`

**Step 4: Run the targeted tests again**

Run: `go test ./gateway/... ./daemon/... -run 'TestWorkProject.*|TestProjectRepo.*' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/work_projects_store.go daemon/internal/workspace/project_repo.go gateway/work_projects_store_test.go daemon/internal/workspace/project_repo_test.go
git commit -m "feat: add project substrate for work-oriented orchestration"
```

### Task 3: Work Item Store And State Machine

**Files:**
- Create: `gateway/work_items_store.go`
- Create: `gateway/work_state_machine.go`
- Test: `gateway/work_items_store_test.go`
- Test: `gateway/work_state_machine_test.go`

**Step 1: Write failing tests**

Add coverage for:
- item creation
- transitions across `new -> triaged -> queued -> claimed -> running`
- blocked/awaiting_review/done/cancelled handling

**Step 2: Run the targeted tests**

Run: `go test ./gateway/... -run 'TestWorkItem.*|TestWorkStateMachine.*' -count=1`
Expected: FAIL because no work item store or state machine exists.

**Step 3: Write minimal implementation**

Implement:
- persisted work item store under `~/.carrier/works`
- state transition guards
- latest run linkage

**Step 4: Run the targeted tests again**

Run: `go test ./gateway/... -run 'TestWorkItem.*|TestWorkStateMachine.*' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/work_items_store.go gateway/work_state_machine.go gateway/work_items_store_test.go gateway/work_state_machine_test.go
git commit -m "feat: add work item store and state machine"
```

### Task 4: Run Supervisor And Execution Binding

**Files:**
- Create: `gateway/work_runs_store.go`
- Create: `gateway/work_runs_api.go`
- Modify: `gateway/orchestrator_types.go`
- Modify: `gateway/orchestrator_api.go`
- Test: `gateway/work_runs_store_test.go`
- Test: `gateway/work_runs_api_test.go`

**Step 1: Write failing tests**

Add coverage for:
- run creation
- run lease state
- execution extension payload
- run reclaim/resume behavior

**Step 2: Run the targeted tests**

Run: `go test ./gateway/... -run 'TestWorkRun.*|TestHandleWorkRuns.*' -count=1`
Expected: FAIL because runs are not yet modeled.

**Step 3: Write minimal implementation**

Implement:
- run store
- run phase updates
- execution `mode=work` + `work` metadata
- one active run per work item guard

**Step 4: Run the targeted tests again**

Run: `go test ./gateway/... -run 'TestWorkRun.*|TestHandleWorkRuns.*' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/work_runs_store.go gateway/work_runs_api.go gateway/orchestrator_types.go gateway/orchestrator_api.go gateway/work_runs_store_test.go gateway/work_runs_api_test.go
git commit -m "feat: add supervised runs for work items"
```

### Task 5: GitHub Import And Publish Adapter

**Files:**
- Create: `gateway/work_github_adapter.go`
- Test: `gateway/work_github_adapter_test.go`
- Modify: `cmd/carrier/main.go`
- Test: `cmd/carrier/main_work_test.go`

**Step 1: Write failing tests**

Add coverage for:
- issue import normalization
- publish request normalization for comment/branch/PR draft
- local work item remains source of truth after import

**Step 2: Run the targeted tests**

Run: `go test ./gateway/... ./cmd/carrier -run 'TestWorkGitHub.*|TestRunWorkCommand.*' -count=1`
Expected: FAIL because adapter endpoints and CLI are missing.

**Step 3: Write minimal implementation**

Implement:
- import endpoint
- publish endpoint
- CLI wrappers using local gh CLI / PAT mode
- no bidirectional state sync

**Step 4: Run the targeted tests again**

Run: `go test ./gateway/... ./cmd/carrier -run 'TestWorkGitHub.*|TestRunWorkCommand.*' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/work_github_adapter.go gateway/work_github_adapter_test.go cmd/carrier/main.go cmd/carrier/main_work_test.go
git commit -m "feat: add github import and publish adapter for work items"
```

### Task 6: WebUI Work Pages

**Files:**
- Create: `webui/src/features/work/*`
- Modify: `webui/src/app/routes.tsx`
- Test: `webui/src/features/work/*.test.tsx`
- Test: `webui/e2e/tests/work.spec.ts`

**Step 1: Write failing tests**

Add coverage for:
- new `Work` navigation route
- project list
- work item queue
- run detail actions

**Step 2: Run the targeted tests**

Run: `cd webui && bun run test && cd e2e && bunx playwright test tests/work.spec.ts --config playwright.config.ts --project=chromium --workers=1`
Expected: FAIL because work pages do not exist.

**Step 3: Write minimal implementation**

Implement:
- `Work` landing page
- project detail
- work item detail
- run detail
- execution detail `Work Context`

**Step 4: Run the targeted tests again**

Run: `cd webui && bun run test && cd e2e && bunx playwright test tests/work.spec.ts tests/executions.spec.ts --config playwright.config.ts --project=chromium --workers=1`
Expected: PASS

**Step 5: Commit**

```bash
git add webui/src/features/work webui/src/app/routes.tsx webui/e2e/tests/work.spec.ts
git commit -m "feat(webui): add work-oriented orchestration pages"
```

### Task 7: CLI Surface

**Files:**
- Modify: `cmd/carrier/main.go`
- Test: `cmd/carrier/main_work_test.go`
- Modify: `docs/carrier-cli.md`

**Step 1: Write failing tests**

Add coverage for:
- `carrier work projects ...`
- `carrier work items ...`
- `carrier work runs ...`
- `carrier work github ...`

**Step 2: Run the targeted tests**

Run: `go test ./cmd/carrier -run 'TestParseWorkCommandArgs|TestRunWorkCommand' -count=1`
Expected: FAIL because `carrier work` command surface does not exist.

**Step 3: Write minimal implementation**

Implement parser, help text, and output rendering using the new API endpoints.

**Step 4: Run the targeted tests again**

Run: `go test ./cmd/carrier -run 'TestParseWorkCommandArgs|TestRunWorkCommand' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/carrier/main.go cmd/carrier/main_work_test.go docs/carrier-cli.md
git commit -m "feat(cli): add work-oriented orchestration commands"
```

### Task 8: E2E And Evidence Wiring

**Files:**
- Modify: `gateway/orchestrator_evidence_api.go`
- Modify: `webui/e2e/playwright.fullstack.config.ts`
- Create: `webui/e2e/tests/fullstack-work.spec.ts`
- Test: `gateway/orchestrator_evidence_api_test.go`

**Step 1: Write failing tests**

Add coverage for:
- work item -> run -> execution linkage in evidence
- work context visible in execution detail
- stale run reclaim and cleanup flow

**Step 2: Run the targeted tests**

Run: `go test ./gateway/... -run 'TestExecutionEvidence.*Work' -count=1 && bash scripts/e2e-control-plane-local.sh --project=chromium tests/fullstack-work.spec.ts`
Expected: FAIL because work linkage is not yet in evidence or full-stack flows.

**Step 3: Write minimal implementation**

Implement:
- work snapshot in evidence bundle
- run metadata in execution detail API
- full-stack work flow smoke

**Step 4: Run the targeted tests again**

Run: `go test ./gateway/... -run 'TestExecutionEvidence.*Work' -count=1 && bash scripts/e2e-control-plane-local.sh --project=chromium tests/fullstack-work.spec.ts`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/orchestrator_evidence_api.go gateway/orchestrator_evidence_api_test.go webui/e2e/tests/fullstack-work.spec.ts
git commit -m "test: add work-oriented orchestration evidence coverage"
```

## Verification Before Completion

Run, at minimum:

```bash
git diff --check
```

For the implementation phase, each task must verify its own targeted tests before commit.

## Deferred From This Plan

- GitHub App mode
- Linear/Jira adapters
- planner task graph editing in UI
- automatic merge or review submission
- backwards compatibility for old `~/.carrier` root layout
