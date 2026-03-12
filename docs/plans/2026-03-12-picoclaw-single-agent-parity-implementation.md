# PicoClaw Single-Agent Parity Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Close the highest-value single-agent parity gaps against upstream PicoClaw for runtime/tools, media/rich messages, provider/model direct surface, and launcher/cron/heartbeat UX, while keeping Carrier as the control-plane authority.

**Architecture:** Introduce an explicit agent-native surface under Carrier. Phase 1 adds structured runtime/media/provider contracts. Phase 2 adds launcher and heartbeat UX over those contracts. Phase 3 hardens cron and end-to-end parity through local-real and live-provider E2E. Multi-channel managed parity stays out of scope.

**Tech Stack:** Go (`baseagent`, `gateway`, `daemon`), Carrier WebUI React, Tauri shell, Playwright E2E, existing managed onboarding/renderers and execution evidence surfaces.

---

### Task 1: Add Rich Content And Attachment Contracts

**Files:**
- Modify: `baseagent/controlplane_bus.go`
- Modify: `baseagent/runtime.go`
- Modify: `baseagent/execution_tools.go`
- Modify: `baseagent/structured_tool_surface.go`
- Modify: `gateway/telegram_transport.go`
- Test: `baseagent/runtime_structured_loop_test.go`
- Test: `gateway/telegram_transport_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- inbound message envelopes carrying attachment metadata
- tool results returning attachment refs and structured content blocks
- outbound telegram rendering still sending text fallback when structured blocks are present

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestRuntimeChatStructuredLoop.*Attachment|TestTelegramTransport.*StructuredOutbound'`
Expected: FAIL because bus/tool/outbound contracts are text-only.

**Step 3: Write the minimal implementation**

Add:
- `ContentBlock`
- `AttachmentRef`
- `RichOutboundMessage`

Rules:
- `Content` string remains for backward compatibility
- all rich payloads must still provide plain-text fallback
- attachment refs should point to workspace path or artifact metadata, not opaque transport blobs

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestRuntimeChatStructuredLoop.*Attachment|TestTelegramTransport.*StructuredOutbound'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/controlplane_bus.go baseagent/runtime.go baseagent/execution_tools.go baseagent/structured_tool_surface.go gateway/telegram_transport.go baseagent/runtime_structured_loop_test.go gateway/telegram_transport_test.go
git commit -m "feat: add rich content and attachment contracts"
```

### Task 2: Expand Agent-Native Runtime Tools

**Files:**
- Modify: `baseagent/execution_tools.go`
- Modify: `baseagent/structured_tool_surface.go`
- Modify: `baseagent/runtime.go`
- Modify: `baseagent/runtime_options.go`
- Test: `baseagent/execution_tools_test.go`
- Test: `baseagent/runtime_structured_loop_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- `send_file` returning attachment refs
- `spawn_subagent` returning durable metadata
- `subagent_status` reading job status/result
- `memory_search` and web tools coexisting in the same structured loop

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestExecutionToolsSendFile|TestExecutionToolsSubagentStatus|TestStructuredLoop.*Subagent'`
Expected: FAIL because the runtime tool surface lacks durable subagent status and rich file semantics.

**Step 3: Write the minimal implementation**

Add:
- `subagent_status`
- richer `send_file`
- structured metadata for delegated job handles

Keep:
- policy tiers wired through existing structured policy model
- transport-neutral result payloads

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestExecutionToolsSendFile|TestExecutionToolsSubagentStatus|TestStructuredLoop.*Subagent'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/execution_tools.go baseagent/structured_tool_surface.go baseagent/runtime.go baseagent/runtime_options.go baseagent/execution_tools_test.go baseagent/runtime_structured_loop_test.go
git commit -m "feat: extend agent-native runtime tools"
```

### Task 3: Make Subagent Lifecycle Durable

**Files:**
- Modify: `baseagent/subagent_manager.go`
- Modify: `baseagent/runtime.go`
- Modify: `baseagent/agent_loop.go`
- Test: `baseagent/subagent_manager_test.go`
- Test: `baseagent/runtime_structured_loop_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- subagent jobs surviving multiple polling calls
- cancellation or terminal-state reporting
- job summary/result showing up in structured tool responses

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestSubagentManager.*Lifecycle|TestStructuredLoopDelegatesToSubagent'`
Expected: FAIL because current manager is in-memory and minimal.

**Step 3: Write the minimal implementation**

Extend the in-memory manager to support:
- terminal result polling
- optional cancel state
- richer summary/result metadata

Do not add distributed execution yet.

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestSubagentManager.*Lifecycle|TestStructuredLoopDelegatesToSubagent'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/subagent_manager.go baseagent/runtime.go baseagent/agent_loop.go baseagent/subagent_manager_test.go baseagent/runtime_structured_loop_test.go
git commit -m "feat: harden subagent lifecycle for agent-native runtime"
```

### Task 4: Upgrade Skills And MCP To Runtime Capabilities

**Files:**
- Modify: `baseagent/skills_registry.go`
- Modify: `baseagent/skills_store.go`
- Modify: `baseagent/mcp_manager.go`
- Modify: `baseagent/runtime.go`
- Modify: `baseagent/agent_loop.go`
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Test: `baseagent/skills_registry_test.go`
- Test: `baseagent/mcp_manager_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- runtime listing of installed skills and visible MCP tools
- launcher/detail surfaces receiving skill/MCP capability summaries
- enabling/disabling a skill reflected in subsequent requests

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestSkillsRegistry.*|TestManagedMCPManager.*Visible'`
Expected: FAIL on capability exposure shape or missing UI contract support.

**Step 3: Write the minimal implementation**

Add:
- runtime capability summary methods
- skill enablement state where needed
- MCP capability status for visibility and health

Do not build marketplace or remote skill install yet.

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestSkillsRegistry.*|TestManagedMCPManager.*Visible'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/skills_registry.go baseagent/skills_store.go baseagent/mcp_manager.go baseagent/runtime.go baseagent/agent_loop.go webui/src/features/agents/AgentDetailPage.tsx baseagent/skills_registry_test.go baseagent/mcp_manager_test.go
git commit -m "feat: expose skills and MCP as runtime capabilities"
```

### Task 5: Add Media / File / Voice Ingress And Egress

**Files:**
- Modify: `gateway/telegram_transport.go`
- Modify: `baseagent/controlplane_bus.go`
- Modify: `baseagent/agent_loop.go`
- Modify: `baseagent/runtime.go`
- Modify: `daemon/server/server.go`
- Test: `gateway/telegram_transport_test.go`
- Test: `daemon/server/server_handlers_coverage_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- inbound attachment normalization into `AttachmentRef`
- file send through outbound rich content
- audio attachment accepted and passed into runtime metadata
- unsupported rich blocks falling back to plain text

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestTelegram.*Attachment|TestHandleAgentChat.*Attachment'`
Expected: FAIL because transport/runtime boundaries are still text-only.

**Step 3: Write the minimal implementation**

Implement:
- inbound Telegram attachment extraction into normalized refs
- outbound file/image rich-block rendering where possible
- plain-text fallback everywhere else

Voice first cut:
- accept audio metadata
- do not require full TTS yet
- leave transcription behind an interface

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestTelegram.*Attachment|TestHandleAgentChat.*Attachment'`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/telegram_transport.go baseagent/controlplane_bus.go baseagent/agent_loop.go baseagent/runtime.go daemon/server/server.go gateway/telegram_transport_test.go daemon/server/server_handlers_coverage_test.go
git commit -m "feat: add media and file ingress for managed agent runtime"
```

### Task 6: Add Voice / Transcription Runtime Hooks

**Files:**
- Create: `baseagent/media_runtime.go`
- Create: `baseagent/media_runtime_test.go`
- Modify: `baseagent/runtime_options.go`
- Modify: `baseagent/runtime.go`
- Modify: `daemon/server/server.go`

**Step 1: Write the failing tests**

Add coverage for:
- voice/audio attachment invoking a media runtime hook
- transcription output re-entering normal chat flow
- policy or capability absence returning a bounded "unsupported" result instead of hard failure

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestMediaRuntime.*|TestRuntimeChat.*AudioAttachment'`
Expected: FAIL because no media runtime abstraction exists.

**Step 3: Write the minimal implementation**

Add a small `MediaRuntime` interface:
- `Transcribe`
- optional `Synthesize`

Wire only transcription in V1.

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestMediaRuntime.*|TestRuntimeChat.*AudioAttachment'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/media_runtime.go baseagent/media_runtime_test.go baseagent/runtime_options.go baseagent/runtime.go daemon/server/server.go
git commit -m "feat: add voice and transcription runtime hooks"
```

### Task 7: Refactor Provider / Model Surface To Protocol Profiles

**Files:**
- Modify: `shared/catalog/catalog.go`
- Modify: `shared/config/default_model.go`
- Modify: `gateway/managed_onboard.go`
- Modify: `gateway/picoclaw_onboard.go`
- Modify: `cmd/carrier/main.go`
- Test: `shared/config/default_model_test.go`
- Test: `gateway/picoclaw_onboard_test.go`
- Test: `gateway/zeroclaw_onboard_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- protocol-family resolution (`openai-compatible`, `ollama`, `oauth-openai`)
- same provider supporting multiple managed model profiles
- renderer emitting direct-surface model/profile config instead of a single collapsed provider

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'Test.*ManagedOnboard.*Profile|TestLoadCarrierModel.*Protocol'`
Expected: FAIL because direct-surface protocol/profile abstraction does not exist yet.

**Step 3: Write the minimal implementation**

Add:
- protocol-family metadata to provider catalog
- managed model profile structure
- config renderers that can emit per-agent model lists or direct-surface profile sections

Keep Carrier governance resolution as the source of truth.

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'Test.*ManagedOnboard.*Profile|TestLoadCarrierModel.*Protocol'`
Expected: PASS

**Step 5: Commit**

```bash
git add shared/catalog/catalog.go shared/config/default_model.go gateway/managed_onboard.go gateway/picoclaw_onboard.go cmd/carrier/main.go shared/config/default_model_test.go gateway/picoclaw_onboard_test.go gateway/zeroclaw_onboard_test.go
git commit -m "feat: add protocol-based provider and model profiles"
```

### Task 8: Add Managed Model Fanout / Fallback Semantics

**Files:**
- Modify: `baseagent/llm.go`
- Modify: `baseagent/runtime.go`
- Modify: `gateway/provider_governance.go`
- Modify: `gateway/orchestrator_observability_api.go`
- Test: `baseagent/llm_test.go`
- Test: `gateway/orchestrator_observability_api_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- multiple managed profiles with same logical model alias
- fallback on provider/model failure
- observability still attributing actual resolved provider/model/cost

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestRequestLLM.*Fallback|TestOrchestratorMetrics.*Provider'`
Expected: FAIL because runtime/provider governance do not support same-model fanout/fallback.

**Step 3: Write the minimal implementation**

Implement:
- logical model alias to candidate profile list
- sequential fallback in V1
- resolved provider/model still captured in governance snapshot

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestRequestLLM.*Fallback|TestOrchestratorMetrics.*Provider'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/llm.go baseagent/runtime.go gateway/provider_governance.go gateway/orchestrator_observability_api.go baseagent/llm_test.go gateway/orchestrator_observability_api_test.go
git commit -m "feat: add managed model fallback and fanout semantics"
```

### Task 9: Add Launcher And Heartbeat Contracts

**Files:**
- Create: `gateway/agent_launcher_api.go`
- Create: `gateway/agent_launcher_api_test.go`
- Modify: `daemon/internal/lifecycle/types.go`
- Modify: `daemon/server/server.go`
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Modify: `webui/src/features/hosts/components/HostManagePanel.tsx`

**Step 1: Write the failing tests**

Add coverage for:
- launcher API returning runtime summary, provider readiness, memory attachments, skills/MCP, heartbeat state
- heartbeat age and last activity rendering in WebUI

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestHandleAgentLauncher.*|TestHandleAgentStatus.*Heartbeat'`
Expected: FAIL because no launcher API or heartbeat contract exists.

**Step 3: Write the minimal implementation**

Add:
- `AgentHeartbeat`
- `LauncherSessionSummary`
- read-only launcher API first

Do not add a separate daemon process; use existing daemon/gateway state.

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestHandleAgentLauncher.*|TestHandleAgentStatus.*Heartbeat'`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/agent_launcher_api.go gateway/agent_launcher_api_test.go daemon/internal/lifecycle/types.go daemon/server/server.go webui/src/features/agents/AgentDetailPage.tsx webui/src/features/hosts/components/HostManagePanel.tsx
git commit -m "feat: add launcher and heartbeat contracts for managed agents"
```

### Task 10: Add Standalone CLI Surface For Managed PicoClaw

**Files:**
- Modify: `cmd/carrier/main.go`
- Modify: `docs/carrier-cli.md`
- Test: `cmd/carrier/main_templates_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- one-shot prompt command
- local interactive managed-agent shell command parsing
- launcher status command parsing

**Step 2: Run the targeted tests**

Run: `go test ./cmd/carrier -run 'TestParse.*Picoclaw.*|TestRun.*Launcher.*'`
Expected: FAIL because no standalone managed PicoClaw CLI surface exists.

**Step 3: Write the minimal implementation**

Add a small CLI surface, for example:
- `carrier agent run picoclaw -m "..."`
- `carrier agent launcher picoclaw`
- `carrier agent heartbeat picoclaw`

Keep it thin and gateway-backed.

**Step 4: Run the targeted tests again**

Run: `go test ./cmd/carrier -run 'TestParse.*Picoclaw.*|TestRun.*Launcher.*'`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/carrier/main.go docs/carrier-cli.md cmd/carrier/main_templates_test.go
git commit -m "feat: add standalone managed-agent cli surface"
```

### Task 11: Add Cron UX For Managed Agents

**Files:**
- Modify: `baseagent/cron_service.go`
- Modify: `daemon/server/server.go`
- Modify: `cmd/carrier/main.go`
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Test: `baseagent/cron_service_test.go`
- Test: `daemon/server/server_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- list scheduled jobs
- cancel scheduled jobs
- next run / last run state
- launcher surface showing cron summary

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestCron.*List|TestCron.*Cancel|TestDaemonServerCron.*'`
Expected: FAIL because cron currently only exposes schedule and immediate reentry semantics.

**Step 3: Write the minimal implementation**

Add:
- list/cancel APIs
- execution history metadata for cron jobs
- minimal WebUI/CLI surfacing

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestCron.*List|TestCron.*Cancel|TestDaemonServerCron.*'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/cron_service.go daemon/server/server.go cmd/carrier/main.go webui/src/features/agents/AgentDetailPage.tsx baseagent/cron_service_test.go daemon/server/server_test.go
git commit -m "feat: add cron management ux for managed agents"
```

### Task 12: Add End-To-End Parity Validation

**Files:**
- Modify: `scripts/e2e-control-plane-live-provider.sh`
- Modify: `scripts/e2e-control-plane-local.sh`
- Modify: `.github/workflows/ci.yml`
- Create: `webui/e2e/tests/fullstack-agent-launcher.spec.ts`
- Create: `webui/e2e/tests/fullstack-agent-media.spec.ts`
- Create: `webui/e2e/tests/fullstack-agent-cron.spec.ts`

**Step 1: Write the failing tests**

Add full-stack coverage for:
- managed PicoClaw one-shot launcher run
- file/media input/output
- heartbeat visibility
- cron create/list/cancel
- provider/model direct-surface resolution

**Step 2: Run the targeted tests**

Run: `bash scripts/e2e-control-plane-local.sh --project=chromium`
Expected: FAIL because new launcher/media/cron surfaces do not exist yet.

**Step 3: Write the minimal implementation**

Wire the new flows into existing local-real and live-provider harnesses.

Rules:
- PR-required path stays local-real only
- live-provider covers media/provider/runtime completion

**Step 4: Run the targeted tests again**

Run:
- `bash scripts/e2e-control-plane-local.sh --project=chromium`
- `bash scripts/e2e-control-plane-live-provider.sh`

Expected: PASS

**Step 5: Commit**

```bash
git add scripts/e2e-control-plane-live-provider.sh scripts/e2e-control-plane-local.sh .github/workflows/ci.yml webui/e2e/tests/fullstack-agent-launcher.spec.ts webui/e2e/tests/fullstack-agent-media.spec.ts webui/e2e/tests/fullstack-agent-cron.spec.ts
git commit -m "test: add pico single-agent parity e2e coverage"
```

## Recommended Execution Order

1. Tasks 1-3: runtime content/tool/subagent foundation
2. Task 4: skills and MCP runtime visibility
3. Tasks 5-6: media and voice contracts
4. Tasks 7-8: provider/model direct surface
5. Tasks 9-11: launcher, heartbeat, standalone UX, cron
6. Task 12: end-to-end hardening

## Notes For Implementation

- Do not broaden channel support during this plan.
- Preserve existing Carrier evidence/audit/governance fields when adding richer runtime surfaces.
- Prefer introducing additive contracts and compatibility shims rather than breaking current text-only flows.
- Keep all managed-agent direct-surface features backed by Carrier policy, memory, and evidence.
