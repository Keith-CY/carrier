# PicoClaw Parity V2 Follow-up Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Continue single-agent parity after Tasks 17-24 by making the managed model surface mutable, then deepening runtime policy metadata and agent-native operator controls.

**Architecture:** Treat managed-agent `modelSurface` as the primary direct-surface contract for single-agent operation. First add explicit default-profile mutation so launcher/CLI/WebUI can change agent-local model behavior without re-onboarding. Then layer richer profile policy metadata, runtime trace visibility, skill update/version control, and MCP detail controls on top of the same surface.

**Tech Stack:** Go (`gateway`, `cmd/carrier`, `daemon/server`), React WebUI (`AgentDetailPage`), managed instance store/config sync helpers, Vitest, Playwright.

**Progress Update (2026-03-13):**
- Task 25 completed
- Task 26 completed
- Task 27 completed

---

### Task 25: Add Managed Agent Model Default Update API

**Files:**
- Modify: `gateway/webui_agents.go`
- Modify: `gateway/managed_model_surface.go`
- Modify: `gateway/agent_launcher_api_test.go`
- Modify: `cmd/carrier/main.go`
- Modify: `cmd/carrier/main_agent_test.go`

**Step 1: Write the failing test**

Add coverage for:
- `POST /api/v1/agents/:id/models/default` with `{profileName}`
- store update changing `model_surface.default_profile`
- CLI `carrier agent models default <agent_id> <profile_name>`

**Step 2: Run test to verify it fails**

Run: `go test ./gateway ./cmd/carrier -run 'TestHandleAgentModels|TestParseAgentCommandArgs|TestRunAgentCommand' -count=1`
Expected: FAIL because model surface is inspect/sync-only.

**Step 3: Write minimal implementation**

Add:
- gateway default-profile mutation endpoint
- validation that requested profile exists in `model_surface.profiles`
- managed instance store update in place
- CLI mutation surface

**Step 4: Run test to verify it passes**

Run: `go test ./gateway ./cmd/carrier -run 'TestHandleAgentModels|TestParseAgentCommandArgs|TestRunAgentCommand' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/webui_agents.go gateway/managed_model_surface.go gateway/agent_launcher_api_test.go cmd/carrier/main.go cmd/carrier/main_agent_test.go
git commit -m "feat(picoclaw): add managed model default controls"
```

### Task 26: Surface Default Profile Switching In Agent Detail

**Files:**
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Modify: `webui/src/features/agents/AgentDetailPage.test.tsx`
- Modify: `webui/e2e/tests/agent-detail.spec.ts`

**Step 1: Write the failing tests**

Add coverage for:
- selecting a different profile as default
- success message + models/launcher refetch

**Step 2: Run test to verify it fails**

Run: `cd webui && bun test src/features/agents/AgentDetailPage.test.tsx && cd e2e && bunx playwright test tests/agent-detail.spec.ts --config playwright.config.ts --project=chromium --workers=1`
Expected: FAIL because model surface is read-only.

**Step 3: Write minimal implementation**

Add:
- `Set default` action per non-default profile
- mutation to `/api/v1/agents/:id/models/default`
- post-mutation refetch of launcher/models

**Step 4: Run test to verify it passes**

Run: `cd webui && bun test src/features/agents/AgentDetailPage.test.tsx && cd e2e && bunx playwright test tests/agent-detail.spec.ts --config playwright.config.ts --project=chromium --workers=1`
Expected: PASS

**Step 5: Commit**

```bash
git add webui/src/features/agents/AgentDetailPage.tsx webui/src/features/agents/AgentDetailPage.test.tsx webui/e2e/tests/agent-detail.spec.ts
git commit -m "feat(picoclaw): add model default switch ui"
```

### Task 27: Add Managed Model Policy Metadata

**Files:**
- Modify: `gateway/managed_instances.go`
- Modify: `gateway/managed_onboard.go`
- Modify: `gateway/agent_launcher_api.go`
- Modify: `cmd/carrier/main.go`
- Test: `gateway/agent_launcher_api_test.go`

**Step 1: Write the failing test**

Add coverage for profile metadata:
- `timeoutMs`
- `retryBudget`
- `fallbackStrategy`

**Step 2: Run test to verify it fails**

Run: `go test ./gateway ./cmd/carrier -run 'TestHandleAgentLauncher.*ModelSurface|TestRunAgentCommand' -count=1`
Expected: FAIL because model surface only carries identity metadata.

**Step 3: Write minimal implementation**

Persist and render optional profile policy metadata without yet changing daemon runtime behavior.

**Step 4: Run test to verify it passes**

Run: `go test ./gateway ./cmd/carrier -run 'TestHandleAgentLauncher.*ModelSurface|TestRunAgentCommand' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/managed_instances.go gateway/managed_onboard.go gateway/agent_launcher_api.go cmd/carrier/main.go gateway/agent_launcher_api_test.go
git commit -m "feat(picoclaw): add model policy metadata"
```

### Task 28: Surface Model Runtime Trace In Launcher UX

**Files:**
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Modify: `webui/src/features/agents/AgentDetailPage.test.tsx`
- Modify: `webui/e2e/tests/agent-detail.spec.ts`
- Modify: `cmd/carrier/main.go`

**Step 1: Write the failing test**

Add coverage for:
- last resolved model
- override hit
- fallback hit
- last run timestamp

**Step 2: Run test to verify it fails**

Run: targeted CLI/WebUI agent-detail tests
Expected: FAIL because runtime trace is not rendered in a dedicated way.

**Step 3: Write minimal implementation**

Add dedicated launcher trace block to CLI/WebUI.

**Step 4: Run test to verify it passes**

Run: targeted CLI/WebUI tests
Expected: PASS

**Step 5: Commit**

```bash
git add webui/src/features/agents/AgentDetailPage.tsx webui/src/features/agents/AgentDetailPage.test.tsx webui/e2e/tests/agent-detail.spec.ts cmd/carrier/main.go
git commit -m "feat(picoclaw): surface model runtime trace"
```

### Task 29: Add Skill Update And Version Pin Controls

**Files:**
- Modify: `daemon/server/server.go`
- Modify: `gateway/daemonclient.go`
- Modify: `gateway/webui_agents.go`
- Modify: `cmd/carrier/main.go`
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`

**Step 1: Write the failing test**

Add coverage for:
- `skills update`
- optional version pin input
- UI rendering of installed version target

**Step 2: Run test to verify it fails**

Run: targeted daemon/gateway/CLI/WebUI skill lifecycle tests
Expected: FAIL because skill lifecycle stops at install/uninstall/enable.

**Step 3: Write minimal implementation**

Add:
- update endpoint
- optional version pin field
- CLI + WebUI controls

**Step 4: Run test to verify it passes**

Run: targeted daemon/gateway/CLI/WebUI tests
Expected: PASS

**Step 5: Commit**

```bash
git add daemon/server/server.go gateway/daemonclient.go gateway/webui_agents.go cmd/carrier/main.go webui/src/features/agents/AgentDetailPage.tsx
git commit -m "feat(picoclaw): add skill update controls"
```

### Task 30: Add MCP Detail And Health Controls

**Files:**
- Modify: `daemon/server/server.go`
- Modify: `gateway/daemonclient.go`
- Modify: `gateway/webui_agents.go`
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Test: daemon/gateway/WebUI agent detail MCP tests

**Step 1: Write the failing test**

Add coverage for:
- MCP detail endpoint
- server health detail
- visible/hidden tool counts and remediation hints

**Step 2: Run test to verify it fails**

Run: targeted daemon/gateway/WebUI MCP tests
Expected: FAIL because MCP UI only exposes enable/disable and a shallow summary.

**Step 3: Write minimal implementation**

Add:
- MCP detail endpoint
- launcher-facing health detail block
- WebUI detail rendering

**Step 4: Run test to verify it passes**

Run: targeted daemon/gateway/WebUI tests
Expected: PASS

**Step 5: Commit**

```bash
git add daemon/server/server.go gateway/daemonclient.go gateway/webui_agents.go webui/src/features/agents/AgentDetailPage.tsx
git commit -m "feat(picoclaw): add managed mcp detail controls"
```
