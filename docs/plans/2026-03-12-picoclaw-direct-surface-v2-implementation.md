# PicoClaw Direct-Surface V2 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Expose a first-class managed-agent model surface, then allow CLI/runtime selection by model alias or concrete model id.

**Architecture:** Persist a normalized model surface inside managed instance metadata at install/onboard time. Reuse that surface in launcher summary, CLI rendering, and WebUI agent detail. Then thread optional `modelAlias` / `model` fields through gateway and daemon chat entrypoints for managed agents.

**Tech Stack:** Go (`gateway`, `cmd/carrier`, `daemon/server`), React WebUI (`AgentDetailPage`), existing managed onboarding/config renderers, Vitest, Go tests.

---

### Task 13: Persist Managed Model Surface And Expose It In Launcher Summary

**Files:**
- Modify: `gateway/managed_instances.go`
- Modify: `gateway/managed_onboard.go`
- Modify: `gateway/agent_launcher_api.go`
- Test: `gateway/agent_launcher_api_test.go`

**Step 1: Write the failing test**

Add launcher summary coverage for:
- `modelSurface.defaultProfile`
- `modelSurface.profiles[]`
- primary profile carrying alias/model/provider/protocol fields

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestHandleAgentLauncherReturnsModelSurface' -count=1`
Expected: FAIL because launcher summary does not expose managed model metadata.

**Step 3: Write minimal implementation**

Add:
- `managedAgentModelSurface`
- `managedAgentModelProfile`
- `ModelSurface *managedAgentModelSurface` on `managedAgentInstance`

Populate the surface from managed onboarding profile generation and include it in launcher summary.

**Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestHandleAgentLauncherReturnsModelSurface' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/managed_instances.go gateway/managed_onboard.go gateway/agent_launcher_api.go gateway/agent_launcher_api_test.go
git commit -m "feat(picoclaw): expose managed model surface in launcher"
```

### Task 14: Add CLI Model Selection Flags And Thread Them Into Agent Chat

**Files:**
- Modify: `cmd/carrier/main.go`
- Modify: `cmd/carrier/main_agent_test.go`
- Modify: `gateway/webui_agents.go`
- Modify: `gateway/daemonclient.go`
- Modify: `gateway/daemonclient_test.go`
- Modify: `gateway/agent_chat_api_test.go`
- Modify: `daemon/server/server.go`
- Modify: `daemon/server/server_handlers_coverage_test.go`
- Modify: `daemon/server/managed_agent_proxy.go`

**Step 1: Write the failing tests**

Add coverage for:
- `carrier agent run ... --model-alias flash`
- `carrier agent run ... --model google/gemini-2.0-flash-001`
- daemon client forwarding `modelAlias` / `model`
- daemon handler decoding these fields
- managed zero/pico agent proxy preferring selected alias/model where supported

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/carrier ./gateway ./daemon/server -run 'TestParseAgentCommandArgs|TestRunAgentCommand|TestDaemonClient_ChatAgent|TestHandleWebUIAgentChatPassesThroughToDaemon|TestHandleAgentChat' -count=1`
Expected: FAIL because CLI and chat payloads do not support model selection.

**Step 3: Write minimal implementation**

Add optional fields:
- `modelAlias`
- `model`

Rules:
- prefer alias when both alias and model are present only if user explicitly chose alias; otherwise pass both transparently
- `shell` inherits selected alias/model across turns
- managed zero/pico runtime can ignore unsupported exact model selection only if it returns a bounded error or falls back cleanly

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/carrier ./gateway ./daemon/server -run 'TestParseAgentCommandArgs|TestRunAgentCommand|TestDaemonClient_ChatAgent|TestHandleWebUIAgentChatPassesThroughToDaemon|TestHandleAgentChat' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/carrier/main.go cmd/carrier/main_agent_test.go gateway/webui_agents.go gateway/daemonclient.go gateway/daemonclient_test.go gateway/agent_chat_api_test.go daemon/server/server.go daemon/server/server_handlers_coverage_test.go daemon/server/managed_agent_proxy.go
git commit -m "feat(picoclaw): add model selection to managed agent run surface"
```

### Task 15: Show Model Surface In CLI Launcher Output And WebUI Agent Detail

**Files:**
- Modify: `cmd/carrier/main.go`
- Modify: `cmd/carrier/main_agent_test.go`
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Modify: `webui/src/features/agents/AgentDetailPage.test.tsx`

**Step 1: Write the failing tests**

Add coverage for:
- launcher CLI rendering default model alias/model id
- Agent Detail rendering model surface cards/list

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/carrier -run 'TestRunAgentCommand' -count=1 && cd webui && bun test src/features/agents/AgentDetailPage.test.tsx`
Expected: FAIL because model surface is not rendered.

**Step 3: Write minimal implementation**

Render:
- `default=<alias or profileName> -> <model id>`
- profile list with provider / protocol family / primary marker

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/carrier -run 'TestRunAgentCommand' -count=1 && cd webui && bun test src/features/agents/AgentDetailPage.test.tsx`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/carrier/main.go cmd/carrier/main_agent_test.go webui/src/features/agents/AgentDetailPage.tsx webui/src/features/agents/AgentDetailPage.test.tsx
git commit -m "feat(picoclaw): surface model profiles in launcher ui"
```

### Task 16: Add Fallback/Fanout Metadata To Managed Model Surface

**Files:**
- Modify: `gateway/managed_instances.go`
- Modify: `gateway/managed_onboard.go`
- Modify: `gateway/agent_launcher_api.go`
- Modify: `gateway/orchestrator_observability_api.go`
- Test: `gateway/agent_launcher_api_test.go`
- Test: `gateway/orchestrator_observability_api_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- multiple profiles sharing the same alias
- launcher summary clearly marking primary and fallback entries
- observability exposing fallback candidate count or alias grouping metadata

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestHandleAgentLauncher.*Fallback|TestOrchestratorMetricsSummary.*ModelAlias' -count=1`
Expected: FAIL because managed model surface does not distinguish primary vs fallback semantics.

**Step 3: Write minimal implementation**

Add:
- `Primary bool`
- `FallbackGroup string`
- `AliasGroupSize int`

Populate them from the ordered managed profile list.

**Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestHandleAgentLauncher.*Fallback|TestOrchestratorMetricsSummary.*ModelAlias' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/managed_instances.go gateway/managed_onboard.go gateway/agent_launcher_api.go gateway/orchestrator_observability_api.go gateway/agent_launcher_api_test.go gateway/orchestrator_observability_api_test.go
git commit -m "feat(picoclaw): mark managed model fallback groups"
```
