# PicoClaw Parity V4 Follow-up Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Continue single-agent parity after Tasks 34-39 by deepening provider/model operator surfaces, richer voice/media pass criteria, and standalone runtime remediation.

**Architecture:** Keep `managedAgentModelSurface` as the canonical agent-local model contract, but add read-only discovery/drift inspection instead of mutating sync-by-default. New provider/model operator surfaces must show both stored state and config-discovered state. Media/voice improvements should tighten live verification without coupling carrier orchestration to a single provider. Runtime/launcher UX should continue to consume the same managed instance store and daemon runtime endpoints.

**Tech Stack:** Go (`gateway`, `daemon/server`, `cmd/carrier`, `baseagent`), React WebUI (`AgentDetailPage`), live-provider shell scripts, Vitest, Playwright.

---

### Task 40: Add Managed Model Discovery / Drift Inspect Surface

**Status:** completed on 2026-03-13

**Files:**
- Modify: `gateway/managed_model_surface.go`
- Modify: `gateway/webui_agents.go`
- Modify: `gateway/agent_launcher_api_test.go`
- Modify: `cmd/carrier/main.go`
- Modify: `cmd/carrier/main_agent_test.go`
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Modify: `webui/src/features/agents/AgentDetailPage.test.tsx`
- Modify: `webui/e2e/tests/agent-detail.spec.ts`

**Step 1: Write the failing tests**

Add coverage for:
- `GET /api/v1/agents/:id/models/discover`
- drift state when stored surface and config-discovered surface differ
- CLI `carrier agent models discover <agent_id>`
- Agent Detail on-demand discovery/drift panel

**Step 2: Run test to verify it fails**

Run: `go test ./gateway ./cmd/carrier -run 'TestHandleAgentModels|TestParseAgentCommandArgs|TestRunAgentCommand' -count=1`
Expected: FAIL because models only support stored view, sync, default, and update-profile.

**Step 3: Write minimal implementation**

Add:
- managed config discovery summary (stored + discovered + drift state/reason)
- gateway `GET /api/v1/agents/:id/models/discover`
- CLI `carrier agent models discover`
- Agent Detail button to fetch and render drift summary

Rules:
- discovery must be read-only
- drift detection must use normalized model surface comparison, not raw file text
- do not mutate managed instance store during discover

**Step 4: Run test to verify it passes**

Run:
- `go test ./gateway ./cmd/carrier -run 'TestHandleAgentModels|TestParseAgentCommandArgs|TestRunAgentCommand' -count=1`
- `cd webui && bun test src/features/agents/AgentDetailPage.test.tsx`
- `cd webui/e2e && bunx playwright test tests/agent-detail.spec.ts --config playwright.config.ts --project=chromium --workers=1`

### Task 41: Tighten Provider-Specific Live Transcription Matrix

**Status:** in progress on 2026-03-13

Expand live-provider smoke so OpenAI-capable environments hard-pass transcription by default while OpenRouter remains soft-fail unless explicitly required.

### Task 42: Add Voice Output / Media Result Metadata

Extend rich outbound media contracts so execution/agent detail/evidence can distinguish generated file/image/audio outputs from fallback text-only responses.

### Task 43: Add Runtime Remediation Drill-Down

Extend Agent Detail / launcher surfaces with richer last-failure detail for provider/model, media runtime, MCP, and cron remediation paths.
