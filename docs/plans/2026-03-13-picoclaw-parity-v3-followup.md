# PicoClaw Parity V3 Follow-up Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Continue single-agent parity after Tasks 25-30 by making the managed model surface editable, turning alias-group metadata into real runtime behavior, then deepening rich media and skills/runtime operator depth.

**Architecture:** Keep `managedAgentModelSurface` as the canonical agent-local direct-surface contract. Mutations must persist both the Carrier instance store and the managed agent config file so `sync` remains idempotent. Alias-group metadata must drive deterministic round-robin selection and persist runtime trace/cursors in the same managed instance record. Later media/runtime and skills/runtime slices build on the same agent-local contract without introducing transport-specific special cases.

**Tech Stack:** Go (`gateway`, `daemon/server`, `cmd/carrier`, `baseagent`), React WebUI (`AgentDetailPage`), existing managed instance/config renderers, Vitest, Playwright, live-provider smoke.

---

### Task 31: Add Managed Model Profile Update API

**Status:** done on 2026-03-13

**Files:**
- Modify: `gateway/webui_agents.go`
- Modify: `gateway/managed_model_surface.go`
- Modify: `gateway/managed_instances.go`
- Modify: `gateway/agent_launcher_api_test.go`
- Modify: `cmd/carrier/main.go`
- Modify: `cmd/carrier/main_agent_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- `POST /api/v1/agents/:id/models/profile`
- validation of `profileName`
- updating model/profile policy metadata in store
- persisting the update back into managed config
- CLI `carrier agent models update-profile <agent_id> <profile_name> ...`

**Step 2: Run test to verify it fails**

Run: `go test ./gateway ./cmd/carrier -run 'TestHandleAgentModels|TestParseAgentCommandArgs|TestRunAgentCommand' -count=1`
Expected: FAIL because managed models are inspect/sync/default-only.

**Step 3: Write minimal implementation**

Add:
- gateway profile update endpoint
- store update for existing profile only
- config rewrite for PicoClaw/ZeroClaw/OpenClaw managed configs
- CLI update-profile surface

Rules:
- No profile creation/deletion yet; update existing profiles only
- `profileName` remains stable identifier
- config rewrite must preserve non-model/channel settings

**Step 4: Run test to verify it passes**

Run: `go test ./gateway ./cmd/carrier -run 'TestHandleAgentModels|TestParseAgentCommandArgs|TestRunAgentCommand' -count=1`
Expected: PASS

### Task 32: Surface Profile Edit Controls In Agent Detail

**Status:** done on 2026-03-13

**Files:**
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Modify: `webui/src/features/agents/AgentDetailPage.test.tsx`
- Modify: `webui/e2e/tests/agent-detail.spec.ts`

**Step 1: Write the failing tests**

Add coverage for:
- opening an edit form for a profile
- editing model alias/model/provider timeout/retry/fallback strategy
- success message + launcher/models refetch

**Step 2: Run test to verify it fails**

Run: `cd webui && bun test src/features/agents/AgentDetailPage.test.tsx && cd e2e && bunx playwright test tests/agent-detail.spec.ts --config playwright.config.ts --project=chromium --workers=1`
Expected: FAIL because Agent Detail only supports sync/default profile actions.

**Step 3: Write minimal implementation**

Add:
- inline profile editor in Agent Detail
- mutation to `/api/v1/agents/:id/models/profile`
- post-mutation refetch of launcher/models

**Step 4: Run test to verify it passes**

Run: `cd webui && bun test src/features/agents/AgentDetailPage.test.tsx && cd e2e && bunx playwright test tests/agent-detail.spec.ts --config playwright.config.ts --project=chromium --workers=1`
Expected: PASS

### Task 33: Turn Alias-Group Metadata Into Round-Robin/Fanout Selection

**Status:** done on 2026-03-13

**Files:**
- Modify: `daemon/server/managed_agent_proxy.go`
- Modify: `gateway/managed_instances.go`
- Modify: `gateway/agent_launcher_api.go`
- Modify: `gateway/orchestrator_observability_api.go`
- Modify: `daemon/server/server_handlers_coverage_test.go`
- Modify: `gateway/orchestrator_observability_api_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- repeated alias-only runs rotating across same alias group
- persisted selection cursor in managed instance store
- launcher/observability trace showing selected profile/ordinal/strategy

**Step 2: Run test to verify it fails**

Run: `go test ./daemon/server ./gateway -run 'TestResolveManagedZeroClawSelectedModel|TestOrchestratorMetricsSummary.*Fallback' -count=1`
Expected: FAIL because alias groups are informational only and selection always picks the first matching profile.

**Step 3: Write minimal implementation**

Add:
- deterministic round-robin selection for alias groups with size > 1
- persisted selection cursors in managed instance store
- runtime trace fields for selected profile / ordinal / strategy

Rules:
- explicit `model` still wins
- explicit `modelAlias + provider` constrains candidate group
- default profile remains first pick when no alias override is requested

**Step 4: Run test to verify it passes**

Run: `go test ./daemon/server ./gateway -run 'TestResolveManagedZeroClawSelectedModel|TestOrchestratorMetricsSummary.*Fallback' -count=1`
Expected: PASS

### Task 34: Improve Rich Outbound Media Render Depth

**Status:** done on 2026-03-13

Add richer image/document/audio render selection across transport and detail/evidence views.

### Task 35: Add Live Transcription Hard-Pass Provider Coverage

**Status:** done on 2026-03-13

Extend live-provider smoke so audio-capable providers can be required and recorded as hard pass instead of skip.

### Task 36: Add Skill Source/Health/Update Provenance

**Status:** done on 2026-03-13

Deepen skill lifecycle UI/runtime with source provenance, version drift, and update health.

### Task 37: Add MCP Attach/Detach Config Surface

**Status:** done on 2026-03-13

Move beyond enable/disable into attach/detach/config health detail.

### Task 38: Add Durable Job History And Runtime Session Surface

**Status:** done on 2026-03-13

Persist delegated jobs and expose recent/local interactive runtime state more fully.

### Task 39: Expand Agent Launcher Remediation UX

**Status:** done on 2026-03-13

Add clearer provider/model/runtime remediation actions in Agent Detail and launcher-style surfaces.
