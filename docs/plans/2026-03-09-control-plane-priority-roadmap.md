# Carrier Control Plane Priority Roadmap Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Turn the current orchestration/gateway/WebUI surface into a production-grade execution control plane by prioritizing execution lifecycle, artifact handling, worker runtime recovery, observability, and RBAC/policy scope.

**Architecture:** Keep `Execution` as the primary product object and evolve the control plane in layers: persistence and API contracts first, then CLI/WebUI surfaces, then runtime recovery and governance. Reuse the existing gateway orchestrator store, execution detail UI, worker inventory UI, and policy engine instead of introducing a second control plane path.

**Tech Stack:** Go (`gateway`, `cmd/carrier`, `shared`), TypeScript (`webui/src`), Playwright (`webui/e2e`), existing file-backed remote/orchestrator store.

---

## Scope and sequencing

Implement these tracks in order:

1. Execution schema and retry/rerun/clone API
2. CLI + WebUI execution detail hardening + artifact API
3. Worker queue/lease/stale recovery
4. Observability first cut
5. RBAC + policy scope

Do not mix in templates, GitHub triggers, schedule launch, or evidence export during this pass. Those are backlog issues at the end of this document.

## Track 1: Execution schema and retry/rerun/clone API

### Task 1.1: Expand execution/result data model

**Files:**
- Modify: `gateway/orchestrator_types.go`
- Modify: `gateway/remote_store.go`
- Test: `gateway/orchestrator_types_more_test.go`
- Test: `gateway/remote_store_orchestrator_test.go`

**Implementation notes:**
- Add execution lineage fields:
  - `ParentExecutionID string`
  - `SourceExecutionID string`
  - `LaunchReason string`
- Add richer result fields:
  - `Summary string`
  - `FailureReason string`
  - `FailureCategory string`
- Add execution-level summary container:
  - `Outcome OrchestratorExecutionOutcome`
- Add terminal state support:
  - `partial_completed`
  - `retryable_failed`

**Step 1: Write failing tests**
- Add normalization/persistence tests covering:
  - new lineage fields are trimmed and preserved
  - new statuses round-trip through store
  - outcome fields survive `upsertOrchestratorExecution`

**Step 2: Run tests to verify failure**
- Run: `go test ./gateway -run 'TestNormalizeOrchestratorExecution|TestUpsertOrchestratorExecution' -count=1`

**Step 3: Implement minimal schema/store changes**
- Extend `OrchestratorExecution` and `OrchestratorTaskResult`.
- Update normalization helpers.
- Ensure file-backed store read/write preserves new fields.

**Step 4: Re-run focused tests**
- Run: `go test ./gateway -run 'TestNormalizeOrchestratorExecution|TestUpsertOrchestratorExecution' -count=1`

**Step 5: Commit**
- Commit: `git commit -m "feat(orchestrator): extend execution outcome schema"`

### Task 1.2: Add retry/rerun/clone gateway endpoints

**Files:**
- Modify: `gateway/orchestrator_api.go`
- Modify: `gateway/server.go`
- Create: `gateway/orchestrator_retry_api_test.go`
- Modify: `gateway/orchestrator_api_more_test.go`

**Implementation notes:**
- Add actions:
  - `POST /api/v1/orchestrator/executions/:id/retry`
  - `POST /api/v1/orchestrator/executions/:id/rerun`
  - `POST /api/v1/orchestrator/executions/:id/clone`
- Semantics:
  - `retry`: create a new execution containing only failed/retryable tasks from the source execution
  - `rerun`: create a new execution with the same plan and full task set
  - `clone`: create a new execution in `pending_authorization`, preserving plan/policy/provider inputs but not results
- Every derived execution must set:
  - `ParentExecutionID`
  - `SourceExecutionID`
  - `LaunchReason`

**Step 1: Write failing API tests**
- Add tests for:
  - retry rejects executions with no failed tasks
  - rerun produces a new execution ID and cleared results
  - clone preserves task units and policy snapshot but resets authorization

**Step 2: Run tests to verify failure**
- Run: `go test ./gateway -run 'TestOrchestratorExecutionRetry|TestOrchestratorExecutionRerun|TestOrchestratorExecutionClone' -count=1`

**Step 3: Implement endpoint handlers**
- Add helper functions:
  - `buildRetryExecution(source OrchestratorExecution) (OrchestratorExecution, error)`
  - `buildRerunExecution(source OrchestratorExecution) OrchestratorExecution`
  - `buildCloneExecution(source OrchestratorExecution) OrchestratorExecution`
- Reuse existing `normalizeOrchestratorExecution` and `upsertOrchestratorExecution`.

**Step 4: Emit audit events**
- Add audit events:
  - `orchestrator_execution_retry`
  - `orchestrator_execution_rerun`
  - `orchestrator_execution_clone`

**Step 5: Re-run focused tests**
- Run: `go test ./gateway -run 'TestOrchestratorExecutionRetry|TestOrchestratorExecutionRerun|TestOrchestratorExecutionClone' -count=1`

**Step 6: Commit**
- Commit: `git commit -m "feat(orchestrator): add retry rerun and clone endpoints"`

### Task 1.3: Add CLI verbs for retry/rerun/clone

**Files:**
- Modify: `cmd/carrier/main.go`
- Modify: `cmd/carrier/main_orchestrate_test.go`

**Implementation notes:**
- Add:
  - `carrier executions retry <execution_id>`
  - `carrier executions rerun <execution_id>`
  - `carrier executions clone <execution_id>`
- Keep `carrier orchestrate` as the alias surface if you want parity later, but start with `carrier executions`.

**Step 1: Write failing CLI parser tests**
- Add command parsing coverage for retry/rerun/clone.

**Step 2: Run tests to verify failure**
- Run: `go test ./cmd/carrier -run 'TestParseExecutionsCommandArgs|TestRunExecutionMutationCommands' -count=1`

**Step 3: Implement CLI calls**
- POST to the new gateway endpoints.
- Print the new execution ID and next step:
  - `carrier executions show <new_id>`

**Step 4: Re-run focused tests**
- Run: `go test ./cmd/carrier -run 'TestParseExecutionsCommandArgs|TestRunExecutionMutationCommands' -count=1`

**Step 5: Commit**
- Commit: `git commit -m "feat(cli): add retry rerun and clone commands"`

## Track 2: CLI + WebUI execution detail hardening + artifact API

### Task 2.1: Introduce execution artifact contract

**Files:**
- Modify: `gateway/orchestrator_types.go`
- Create: `gateway/orchestrator_artifact_api.go`
- Modify: `gateway/server.go`
- Test: `gateway/orchestrator_api_more_test.go`
- Test: `gateway/server_test.go`

**Implementation notes:**
- Add `OrchestratorArtifact` and `OrchestratorExecutionOutcome` types.
- Store artifact metadata on the execution object first; do not build a separate artifact index yet.
- Add endpoints:
  - `GET /api/v1/orchestrator/executions/:id/artifacts`
  - `GET /api/v1/orchestrator/executions/:id/artifacts/:artifact_id`
- Use `GatewayConfig.ArtifactRoot` from `gateway/gateway.go` as the backing store root.

**Step 1: Write failing API tests**
- Cover:
  - listing artifacts for a completed execution
  - downloading an artifact by ID
  - `404` for missing artifact/execution

**Step 2: Run tests to verify failure**
- Run: `go test ./gateway -run 'TestOrchestratorExecutionArtifacts|TestOrchestratorExecutionArtifactDownload' -count=1`

**Step 3: Implement artifact metadata and handlers**
- Add metadata fields:
  - `ID`
  - `TaskID`
  - `Name`
  - `Kind`
  - `ContentType`
  - `SizeBytes`
  - `Path`
  - `CreatedAt`

**Step 4: Re-run focused tests**
- Run: `go test ./gateway -run 'TestOrchestratorExecutionArtifacts|TestOrchestratorExecutionArtifactDownload' -count=1`

**Step 5: Commit**
- Commit: `git commit -m "feat(orchestrator): add execution artifact api"`

### Task 2.2: Harden CLI execution detail output

**Files:**
- Modify: `cmd/carrier/main.go`
- Modify: `cmd/carrier/main_orchestrate_test.go`

**Implementation notes:**
- Extend `carrier executions show` output sections:
  - `overview`
  - `policy`
  - `workers`
  - `results`
  - `artifacts`
  - `lineage`
- Add `carrier executions artifacts <execution_id>`.

**Step 1: Write failing CLI render tests**
- Cover:
  - detail output includes artifact summary
  - detail output shows `parent/source/launchReason`
  - artifacts command lists artifact names and IDs

**Step 2: Run tests to verify failure**
- Run: `go test ./cmd/carrier -run 'TestRenderExecutionStatus|TestRunExecutionArtifactsCommand' -count=1`

**Step 3: Implement CLI rendering**
- Avoid dumping raw JSON when not using `--json`.
- Keep summaries compact and deterministic.

**Step 4: Re-run focused tests**
- Run: `go test ./cmd/carrier -run 'TestRenderExecutionStatus|TestRunExecutionArtifactsCommand' -count=1`

**Step 5: Commit**
- Commit: `git commit -m "feat(cli): add execution artifact and lineage views"`

### Task 2.3: Upgrade WebUI execution detail page

**Files:**
- Modify: `webui/src/app.ts`
- Modify: `webui/static/index.html`
- Modify: `webui/static/style.css`
- Modify: `webui/static/app.js`
- Test: `webui/e2e/tests/executions.spec.ts`
- Test: `webui/handler_test.go`

**Implementation notes:**
- Break execution detail into fixed cards:
  - Overview
  - Policy & Governance
  - Worker Assignments
  - Results
  - Artifacts
  - Lineage
- Add action buttons for:
  - Retry failed tasks
  - Rerun execution
  - Clone execution
- Add artifact preview/download affordance.

**Step 1: Write failing WebUI tests**
- Add e2e coverage for:
  - artifact list visible in detail
  - retry/rerun/clone buttons
  - lineage section visible on derived executions

**Step 2: Run tests to verify failure**
- Run: `bunx playwright test tests/executions.spec.ts --project=chromium`

**Step 3: Implement UI changes**
- Extend the existing `renderExecutionsView` flow instead of creating a second detail page.
- Rebuild static bundle with `./scripts/build-webui.sh`.

**Step 4: Re-run WebUI tests**
- Run: `bunx playwright test tests/executions.spec.ts --project=chromium`

**Step 5: Commit**
- Commit: `git commit -m "feat(webui): harden execution detail surface"`

## Track 3: Worker queue/lease/stale recovery

### Task 3.1: Model queue/runtime state and stale thresholds

**Files:**
- Modify: `gateway/orchestrator_types.go`
- Modify: `gateway/orchestrator_api.go`
- Modify: `gateway/remote_store.go`
- Test: `gateway/orchestrator_types_more_test.go`
- Test: `gateway/remote_store_more_test.go`

**Implementation notes:**
- Add worker/runtime fields:
  - `QueuePosition`
  - `LastHeartbeatAt`
  - `LeaseState`
  - `Stale bool`
  - `StaleReason string`
- Add helper functions:
  - `isWorkerLeaseStale(...)`
  - `markStaleWorkerLeases(...)`

**Step 1: Write failing tests**
- Cover stale detection for:
  - expired lease TTL
  - heartbeat timeout
  - execution completed but lease left busy

**Step 2: Run tests to verify failure**
- Run: `go test ./gateway -run 'TestMarkStaleWorkerLeases|TestIsWorkerLeaseStale' -count=1`

**Step 3: Implement runtime helpers**
- Keep thresholds config-driven via environment or gateway config defaults.

**Step 4: Re-run focused tests**
- Run: `go test ./gateway -run 'TestMarkStaleWorkerLeases|TestIsWorkerLeaseStale' -count=1`

**Step 5: Commit**
- Commit: `git commit -m "feat(workers): add stale lease detection"`

### Task 3.2: Add recovery endpoints and queue summaries

**Files:**
- Modify: `gateway/orchestrator_workers_api.go`
- Modify: `gateway/server.go`
- Test: `gateway/orchestrator_api_more_test.go`
- Test: `gateway/server_mux_coverage_test.go`

**Implementation notes:**
- Add:
  - `POST /api/v1/orchestrator/workers/reclaim-stale`
  - `GET /api/v1/orchestrator/workers/queue`
- Queue summary should include:
  - active executions
  - queued tasks
  - stale leases
  - reclaimable workers

**Step 1: Write failing API tests**
- Reclaim only stale leases.
- Busy healthy workers are not reclaimed.
- Queue summary counts are deterministic.

**Step 2: Run tests to verify failure**
- Run: `go test ./gateway -run 'TestReclaimStaleWorkers|TestOrchestratorWorkerQueueSummary' -count=1`

**Step 3: Implement reclaim/queue handlers**
- Reuse existing lease listing and state mutation paths.

**Step 4: Re-run focused tests**
- Run: `go test ./gateway -run 'TestReclaimStaleWorkers|TestOrchestratorWorkerQueueSummary' -count=1`

**Step 5: Commit**
- Commit: `git commit -m "feat(workers): add queue summary and stale reclaim api"`

### Task 3.3: Surface recovery state in WebUI Workers

**Files:**
- Modify: `webui/src/app.ts`
- Modify: `webui/static/index.html`
- Modify: `webui/static/style.css`
- Modify: `webui/static/app.js`
- Test: `webui/e2e/tests/workers.spec.ts`

**Implementation notes:**
- Show:
  - stale badge
  - queue position
  - lease age
  - heartbeat age
- Add `Reclaim Stale` action separate from `Reclaim Idle`.

**Step 1: Write failing e2e tests**
- Cover stale worker filtering and reclaim flow.

**Step 2: Run tests to verify failure**
- Run: `bunx playwright test tests/workers.spec.ts --project=chromium`

**Step 3: Implement UI**
- Extend existing workers list/detail cards; do not create a second workers page.

**Step 4: Re-run e2e**
- Run: `bunx playwright test tests/workers.spec.ts --project=chromium`

**Step 5: Commit**
- Commit: `git commit -m "feat(webui): add stale lease worker recovery ui"`

## Track 4: Observability first cut

### Task 4.1: Add execution/worker/provider aggregate API

**Files:**
- Create: `gateway/orchestrator_observability_api.go`
- Modify: `gateway/server.go`
- Modify: `gateway/remote_store.go`
- Test: `gateway/orchestrator_observability_api_test.go`

**Implementation notes:**
- Add:
  - `GET /api/v1/orchestrator/metrics`
- Response groups:
  - `executions`
  - `workers`
  - `providers`
  - `policies`
- Minimum metrics:
  - total executions
  - completed/failed/cancelled/declined counts
  - average latency
  - retry count
  - stale lease count
  - worker busy/idle/error counts
  - provider failure count by requested/resolved provider
  - policy deny/ask counts

**Step 1: Write failing API tests**
- Seed store with mixed executions/leasing and assert aggregate counts.

**Step 2: Run tests to verify failure**
- Run: `go test ./gateway -run 'TestOrchestratorMetricsSummary' -count=1`

**Step 3: Implement aggregation**
- Compute from existing execution and lease store only.
- Do not introduce a new time-series backend in this pass.

**Step 4: Re-run focused tests**
- Run: `go test ./gateway -run 'TestOrchestratorMetricsSummary' -count=1`

**Step 5: Commit**
- Commit: `git commit -m "feat(observability): add orchestrator metrics api"`

### Task 4.2: Add WebUI execution observability page

**Files:**
- Modify: `webui/src/app.ts`
- Modify: `webui/static/index.html`
- Modify: `webui/static/style.css`
- Modify: `webui/static/app.js`
- Test: `webui/e2e/tests/remote-observability.spec.ts`
- Test: `webui/e2e/tests/responsive.spec.ts`

**Implementation notes:**
- Prefer extending the existing observability navigation rather than inventing a parallel page.
- Add cards/tables for:
  - execution outcomes
  - worker health
  - provider failures
  - policy blocks

**Step 1: Write failing e2e tests**
- Cover loading the page, showing cards, and filtering a provider error row.

**Step 2: Run tests to verify failure**
- Run: `bunx playwright test tests/remote-observability.spec.ts tests/responsive.spec.ts --project=chromium`

**Step 3: Implement UI**
- If needed, split current observability view into remote metrics and execution metrics sections.

**Step 4: Re-run e2e**
- Run: `bunx playwright test tests/remote-observability.spec.ts tests/responsive.spec.ts --project=chromium`

**Step 5: Commit**
- Commit: `git commit -m "feat(webui): add execution observability panels"`

## Track 5: RBAC + policy scope

### Task 5.1: Introduce role and permission model

**Files:**
- Create: `gateway/rbac.go`
- Create: `gateway/rbac_test.go`
- Modify: `gateway/server.go`
- Modify: `gateway/provider_auth.go`

**Implementation notes:**
- Start with 4 roles:
  - `viewer`
  - `operator`
  - `approver`
  - `admin`
- Add permission helpers:
  - `canViewExecutions`
  - `canLaunchExecutions`
  - `canApproveExecutions`
  - `canManagePolicies`
  - `canManageProviders`
  - `canManageHosts`
- Bind role resolution to the existing API token/auth context first; do not build user management in this pass.

**Step 1: Write failing authz tests**
- Verify viewer cannot mutate.
- Verify operator cannot edit policy/provider settings.
- Verify approver can approve but not edit hosts.

**Step 2: Run tests to verify failure**
- Run: `go test ./gateway -run 'TestRBACPermissions|TestRBACMiddleware' -count=1`

**Step 3: Implement role helpers and middleware**
- Add request-scoped role lookup from token metadata or environment-backed mapping.

**Step 4: Re-run focused tests**
- Run: `go test ./gateway -run 'TestRBACPermissions|TestRBACMiddleware' -count=1`

**Step 5: Commit**
- Commit: `git commit -m "feat(authz): add gateway rbac middleware"`

### Task 5.2: Enforce RBAC on execution/policy/provider/host surfaces

**Files:**
- Modify: `gateway/orchestrator_api.go`
- Modify: `gateway/orchestrator_policy_api.go`
- Modify: `gateway/remote_api.go`
- Modify: `gateway/orchestrator_plan_api.go`
- Test: `gateway/orchestrator_api_more_test.go`
- Test: `gateway/remote_api_test.go`

**Implementation notes:**
- Enforce:
  - `viewer`: read-only
  - `operator`: create/cancel/retry/rerun/clone executions
  - `approver`: authorization and policy approval
  - `admin`: hosts/providers/policies
- Return `403` with stable error codes.

**Step 1: Write failing permission tests**
- Cover one positive and one negative case per endpoint family.

**Step 2: Run tests to verify failure**
- Run: `go test ./gateway -run 'TestExecutionRBAC|TestPolicyRBAC|TestRemoteHostRBAC' -count=1`

**Step 3: Implement endpoint guards**
- Centralize guard functions to avoid duplicated permission checks.

**Step 4: Re-run focused tests**
- Run: `go test ./gateway -run 'TestExecutionRBAC|TestPolicyRBAC|TestRemoteHostRBAC' -count=1`

**Step 5: Commit**
- Commit: `git commit -m "feat(authz): enforce rbac on control plane routes"`

### Task 5.3: Extend policy scope and add explain API

**Files:**
- Modify: `gateway/orchestrator_policy_engine.go`
- Modify: `gateway/orchestrator_policy_api.go`
- Modify: `gateway/remote_store.go`
- Test: `gateway/orchestrator_api_more_test.go`
- Test: `gateway/remote_store_more_test.go`

**Implementation notes:**
- Extend policy rule matching with:
  - `team`
  - `project`
  - `environment`
  - `templateId`
- Add:
  - `POST /api/v1/policies/evaluate`
- Response must include:
  - matched rule
  - final decision
  - effective clamps
  - required approvals

**Step 1: Write failing policy tests**
- Cover deny/ask/allow across mixed scopes and priorities.

**Step 2: Run tests to verify failure**
- Run: `go test ./gateway -run 'TestPolicyScopeEvaluation|TestPolicyExplainAPI' -count=1`

**Step 3: Implement scoped matching and explain handler**
- Keep the current priority rules: `deny > ask > allow`.

**Step 4: Re-run focused tests**
- Run: `go test ./gateway -run 'TestPolicyScopeEvaluation|TestPolicyExplainAPI' -count=1`

**Step 5: Commit**
- Commit: `git commit -m "feat(policy): add scoped evaluation and explain api"`

### Task 5.4: Surface RBAC and policy scope in WebUI

**Files:**
- Modify: `webui/src/app.ts`
- Modify: `webui/static/index.html`
- Modify: `webui/static/style.css`
- Modify: `webui/static/app.js`
- Test: `webui/e2e/tests/auth-expiry.spec.ts`
- Create: `webui/e2e/tests/rbac.spec.ts`

**Implementation notes:**
- Hide/disable UI actions based on effective role:
  - no cancel/retry/rerun/clone for viewer
  - no policy edit for operator
  - no host/provider edit for approver
- Add policy scope fields to the profile/policy editor.

**Step 1: Write failing e2e tests**
- Cover action visibility by role.
- Cover saving a scoped policy.

**Step 2: Run tests to verify failure**
- Run: `bunx playwright test tests/auth-expiry.spec.ts tests/rbac.spec.ts --project=chromium`

**Step 3: Implement UI**
- Resolve permissions from the gateway bootstrap/session payload instead of hardcoding role logic client-side.

**Step 4: Re-run e2e**
- Run: `bunx playwright test tests/auth-expiry.spec.ts tests/rbac.spec.ts --project=chromium`

**Step 5: Commit**
- Commit: `git commit -m "feat(webui): add rbac-aware control plane actions"`

## Full verification pass

After Tracks 1-5 land, run:

```bash
go test ./... -count=1
./scripts/build-webui.sh
cd webui/e2e && bunx playwright test tests/executions.spec.ts tests/workers.spec.ts tests/remote-observability.spec.ts tests/auth-expiry.spec.ts tests/rbac.spec.ts --project=chromium
```

Expected:
- all gateway and CLI tests pass
- WebUI static bundle rebuilds cleanly
- execution/workers/observability/authz flows pass end-to-end

## Backlog issues to open after this plan

These are important, but intentionally not part of the priority execution package above.

### Issue A: Execution templates and launch presets
- Goal: ship `PR triage`, `issue investigation`, `incident diagnosis`, `rollout smoke check`
- Depends on: Track 1 and Track 2
- Why later: templates are only valuable once execution lineage, artifacts, and retry flows are stable

### Issue B: Trigger system for GitHub/webhook/schedule
- Goal: allow execution creation from PR comments, webhook posts, and cron
- Depends on: Track 1, Track 4, Track 5
- Why later: needs stable lineage, audit, and RBAC before exposing external launch paths

### Issue C: Evidence bundle and audit export
- Goal: export one execution with artifacts, policy snapshot, approvals, and worker trace
- Depends on: Track 2 and Track 5
- Why later: the artifact contract and RBAC need to be settled first

### Issue D: Provider governance v2 and cost attribution
- Goal: show requested vs resolved provider/model, success/failure by provider, and basic cost attribution
- Depends on: Track 4 and Track 5
- Why later: needs the observability schema first

### Issue E: Documentation/website repositioning
- Goal: rewrite top-level product story around execution control plane, not installer semantics
- Depends on: Track 1 and Track 2
- Why later: the story should match the actual stabilized surface

### Issue F: Approval workflow v2
- Goal: separate infrastructure approval, policy approval, and future human review chains
- Depends on: Track 5
- Why later: current approval model should not be expanded before RBAC and scoped policy land

