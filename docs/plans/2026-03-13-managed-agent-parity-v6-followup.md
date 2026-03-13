# Managed Agent Parity V6 Follow-up Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Finish the remaining managed-agent parity deep-water by strengthening media delivery verification, skill/MCP operator depth, runtime traceability, and launcher remediation flows.

**Architecture:** Keep the existing managed instance store, daemon runtime endpoints, launcher summary API, and execution evidence bundle as the canonical state surfaces. New work should extend these surfaces rather than adding sidecar state. Media verification should assert transport-native delivery where supported; runtime/operator work should expose durable detail and direct remediation actions without bypassing existing gateway/daemon contracts.

**Tech Stack:** Go (`baseagent`, `daemon/server`, `gateway`, `cmd/carrier`), React WebUI (`AgentDetailPage`, execution detail), shell-based live smoke scripts, Vitest, Playwright, GitHub Actions.

---

### Task 51: Add Media Delivery Outcome And Preview Metadata

**Files:**
- Modify: `baseagent/controlplane_bus.go`
- Modify: `gateway/providers.go`
- Modify: `gateway/telegram_transport.go`
- Modify: `gateway/orchestrator_evidence_api.go`
- Modify: `gateway/orchestrator_types.go`
- Modify: `webui/src/features/executions/components/ExecutionOutcomeBlock.tsx`
- Test: `gateway/providers_test.go`
- Test: `gateway/telegram_transport_test.go`
- Test: `gateway/orchestrator_evidence_api_test.go`
- Test: `webui/src/features/executions/ExecutionDetailContent.test.tsx`

**Step 1: Write the failing tests**

Add coverage for:
- execution evidence `mediaOutputs` entries carrying `deliveryMethod`, `transport`, and `previewText`
- Telegram rich outbound render recording `sendPhoto/sendDocument/sendAudio/sendVoice/sendVideo`
- execution detail rendering the delivery method and preview text

**Step 2: Run test to verify it fails**

Run:
- `go test ./gateway -run 'TestHandleOrchestratorExecutionEvidenceJSONAndAuditExport|TestTelegramTransportSendRichOutboundMessageAudioAndVideoPreferNativeRender|TestRenderTelegramWebhookResponse_PrefersNativeMediaMethods' -count=1`
- `cd webui && bun run test src/features/executions/ExecutionDetailContent.test.tsx`

Expected: FAIL because delivery metadata and preview text are not persisted/rendered end-to-end.

**Step 3: Write minimal implementation**

Add:
- richer evidence/media output records with explicit delivery metadata
- transport render hooks that record the native method chosen
- a compact preview text derived from the first rich block or attachment fallback
- execution detail UI that shows `delivery=...` and `preview=...`

Rules:
- do not regress plain-text fallback rendering
- preview text should be bounded and deterministic
- evidence must keep working for executions without media output

**Step 4: Run test to verify it passes**

Run:
- `go test ./gateway -run 'TestHandleOrchestratorExecutionEvidenceJSONAndAuditExport|TestTelegramTransportSendRichOutboundMessageAudioAndVideoPreferNativeRender|TestRenderTelegramWebhookResponse_PrefersNativeMediaMethods' -count=1`
- `cd webui && bun run test src/features/executions/ExecutionDetailContent.test.tsx`

**Step 5: Commit**

```bash
git add baseagent/controlplane_bus.go gateway/providers.go gateway/telegram_transport.go gateway/orchestrator_evidence_api.go gateway/orchestrator_types.go webui/src/features/executions/components/ExecutionOutcomeBlock.tsx gateway/providers_test.go gateway/telegram_transport_test.go gateway/orchestrator_evidence_api_test.go webui/src/features/executions/ExecutionDetailContent.test.tsx
git commit -m "feat(managed-agent): persist media delivery metadata"
```

### Task 52: Tighten Live Media Verification To Assert Native Delivery

**Files:**
- Modify: `scripts/e2e-control-plane-live-provider.sh`
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/control-plane-live-provider-schedule.yml`
- Test: `scripts/testdata/transcription-smoke.wav`

**Step 1: Write the failing verification**

Strengthen the live script so it fails when:
- a provider marked `hard_required` returns only plain-text fallback for a media-capable flow
- evidence lacks `mediaOutputs` for the media smoke execution

Keep `openrouter` soft-optional and `openai` hard-required.

**Step 2: Run syntax and workflow validation**

Run:
- `bash -n scripts/e2e-control-plane-live-provider.sh`
- `ruby -e 'require \"psych\"; [\".github/workflows/ci.yml\", \".github/workflows/control-plane-live-provider-schedule.yml\"].each { |p| Psych.load_file(p) }; puts \"yaml-ok\"'`

Expected: PASS for syntax, but current live verification logic is still too weak to enforce native media delivery.

**Step 3: Write minimal implementation**

Add:
- explicit `media_delivery_required` policy in the script
- evidence inspection for `mediaOutputs[].deliveryMethod`
- provider matrix comments and env handling documenting `hard_required` vs `soft_optional`

Rules:
- do not make OpenRouter hard-fail on audio balance limits
- keep artifacts split by provider
- summary output must clearly show whether native media delivery was observed

**Step 4: Run validation again**

Run:
- `bash -n scripts/e2e-control-plane-live-provider.sh`
- `ruby -e 'require \"psych\"; [\".github/workflows/ci.yml\", \".github/workflows/control-plane-live-provider-schedule.yml\"].each { |p| Psych.load_file(p) }; puts \"yaml-ok\"'`

**Step 5: Commit**

```bash
git add scripts/e2e-control-plane-live-provider.sh .github/workflows/ci.yml .github/workflows/control-plane-live-provider-schedule.yml
git commit -m "ci(managed-agent): require native media evidence in live smoke"
```

### Task 53: Add Skill Operator Actions And Provenance Controls

**Files:**
- Modify: `baseagent/skills_loader.go`
- Modify: `baseagent/skills_store.go`
- Modify: `baseagent/skills_registry.go`
- Modify: `baseagent/runtime_capabilities.go`
- Modify: `daemon/server/server.go`
- Modify: `gateway/daemonclient.go`
- Modify: `gateway/webui_agents.go`
- Modify: `cmd/carrier/main.go`
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Test: `baseagent/skills_registry_test.go`
- Test: `daemon/server/server_test.go`
- Test: `gateway/daemonclient_test.go`
- Test: `gateway/webui_agents_coverage_test.go`
- Test: `cmd/carrier/main_agent_test.go`
- Test: `webui/src/features/agents/AgentDetailPage.test.tsx`

**Step 1: Write the failing tests**

Add coverage for:
- `skills reinstall` and `skills disable/enable` API/CLI flows
- provenance/source/version pin visibility after reinstall
- launcher remediation suggesting reinstall when a skill health check is degraded

**Step 2: Run test to verify it fails**

Run:
- `go test ./baseagent -run 'TestSkillsRegistryRuntimeCapabilitiesExposeProvenanceAndTimestamps|TestSkillsRegistryReinstallAndToggleLifecycle' -count=1`
- `go test ./server -run 'TestHandleAgentSkillsSearchInstallUpdateUninstall|TestHandleAgentSkillsReinstallAndToggle' -count=1`
- `go test ./gateway -run 'TestHandleWebUIAgentSkillActions|TestHandleWebUIAgentSkillReinstallAndToggle' -count=1`
- `go test ./cmd/carrier -run 'TestParseAgentCommandArgs|TestRunAgentCommand' -count=1`

Expected: FAIL because reinstall/toggle flows do not exist end-to-end.

**Step 3: Write minimal implementation**

Add:
- daemon endpoints for `skills/reinstall`, `skills/enable`, `skills/disable`
- gateway client/web UI proxy/CLI verbs for the same
- launcher summary remediation for degraded or disabled critical skills

Rules:
- do not create a second skill state source
- reinstall should preserve provenance history while updating timestamps
- enable/disable must update runtime capability visibility

**Step 4: Run test to verify it passes**

Run the same commands from Step 2, plus:
- `cd webui && bun run test src/features/agents/AgentDetailPage.test.tsx`

**Step 5: Commit**

```bash
git add baseagent/skills_loader.go baseagent/skills_store.go baseagent/skills_registry.go baseagent/runtime_capabilities.go daemon/server/server.go gateway/daemonclient.go gateway/webui_agents.go cmd/carrier/main.go webui/src/features/agents/AgentDetailPage.tsx baseagent/skills_registry_test.go daemon/server/server_test.go gateway/daemonclient_test.go gateway/webui_agents_coverage_test.go cmd/carrier/main_agent_test.go webui/src/features/agents/AgentDetailPage.test.tsx
git commit -m "feat(managed-agent): add skill operator actions"
```

### Task 54: Add MCP Runtime Refresh And Config Drill-Down

**Files:**
- Modify: `baseagent/mcp_manager.go`
- Modify: `daemon/server/server.go`
- Modify: `gateway/daemonclient.go`
- Modify: `gateway/webui_agents.go`
- Modify: `cmd/carrier/main.go`
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Test: `baseagent/mcp_manager_test.go`
- Test: `daemon/server/server_test.go`
- Test: `gateway/daemonclient_test.go`
- Test: `gateway/webui_agents_coverage_test.go`
- Test: `cmd/carrier/main_agent_test.go`
- Test: `webui/src/features/agents/AgentDetailPage.test.tsx`

**Step 1: Write the failing tests**

Add coverage for:
- explicit `mcp refresh` action that recomputes health/visible tools
- `mcp config` detail returned to gateway/UI
- remediation action that links degraded server state to refresh/attach/detail flows

**Step 2: Run test to verify it fails**

Run:
- `go test ./baseagent -run 'TestMCPManager.*Refresh' -count=1`
- `go test ./server -run 'TestHandleAgentMCPDetailAndAttachDetach|TestHandleAgentMCPRefresh' -count=1`
- `go test ./gateway -run 'TestHandleWebUIAgentMCPAttachDetachAndDetail|TestHandleWebUIAgentMCPRefresh' -count=1`
- `go test ./cmd/carrier -run 'TestParseAgentCommandArgs|TestRunAgentCommand' -count=1`

Expected: FAIL because refresh/config drill-down are not exposed as first-class operator surfaces.

**Step 3: Write minimal implementation**

Add:
- daemon refresh endpoint and config payload
- gateway proxy and CLI command
- Agent Detail buttons for `Refresh MCP` and `Inspect config`

Rules:
- health refresh must not mutate config
- config drill-down should redact secrets
- keep existing enable/disable/attach/detach flows intact

**Step 4: Run test to verify it passes**

Run the same commands from Step 2, plus:
- `cd webui && bun run test src/features/agents/AgentDetailPage.test.tsx`

**Step 5: Commit**

```bash
git add baseagent/mcp_manager.go daemon/server/server.go gateway/daemonclient.go gateway/webui_agents.go cmd/carrier/main.go webui/src/features/agents/AgentDetailPage.tsx baseagent/mcp_manager_test.go daemon/server/server_test.go gateway/daemonclient_test.go gateway/webui_agents_coverage_test.go cmd/carrier/main_agent_test.go webui/src/features/agents/AgentDetailPage.test.tsx
git commit -m "feat(managed-agent): add mcp refresh and config detail"
```

### Task 55: Deepen Model Runtime Trace And Operator Editing

**Files:**
- Modify: `gateway/managed_model_surface.go`
- Modify: `gateway/managed_instances.go`
- Modify: `gateway/managed_onboard.go`
- Modify: `gateway/agent_launcher_api.go`
- Modify: `cmd/carrier/main.go`
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Test: `gateway/agent_launcher_api_test.go`
- Test: `cmd/carrier/main_agent_test.go`
- Test: `webui/src/features/agents/AgentDetailPage.test.tsx`

**Step 1: Write the failing tests**

Add coverage for:
- per-profile timeout/retry/fallback policy editing
- launcher showing last resolved profile + fallback path + ordinal
- CLI rendering runtime trace for the last request

**Step 2: Run test to verify it fails**

Run:
- `go test ./gateway -run 'TestHandleAgentLauncherReturnsLastModelRuntime|TestHandleAgentModelsUpdateProfile' -count=1`
- `go test ./cmd/carrier -run 'TestParseAgentCommandArgs|TestRunAgentCommand' -count=1`
- `cd webui && bun run test src/features/agents/AgentDetailPage.test.tsx`

Expected: FAIL because runtime trace depth and editable policy metadata are incomplete.

**Step 3: Write minimal implementation**

Add:
- editable timeout/retry/fallback fields per profile
- `lastResolvedProfile`, `lastFallbackPath`, `lastSelectionOrdinal` on launcher summary
- Agent Detail editor and CLI output for those fields

Rules:
- preserve existing default-profile update flow
- keep alias-group selection deterministic
- avoid mixing runtime trace with long-lived config unless explicitly edited

**Step 4: Run test to verify it passes**

Run the same commands from Step 2.

**Step 5: Commit**

```bash
git add gateway/managed_model_surface.go gateway/managed_instances.go gateway/managed_onboard.go gateway/agent_launcher_api.go cmd/carrier/main.go webui/src/features/agents/AgentDetailPage.tsx gateway/agent_launcher_api_test.go cmd/carrier/main_agent_test.go webui/src/features/agents/AgentDetailPage.test.tsx
git commit -m "feat(managed-agent): deepen model runtime trace"
```

### Task 56: Deepen Launcher Operator Console And Durable Runtime Drill-Down

**Files:**
- Modify: `baseagent/subagent_manager.go`
- Modify: `daemon/server/server.go`
- Modify: `gateway/agent_launcher_api.go`
- Modify: `gateway/daemonclient.go`
- Modify: `gateway/webui_agents.go`
- Modify: `cmd/carrier/main.go`
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Test: `baseagent/subagent_manager_test.go`
- Test: `daemon/server/server_test.go`
- Test: `gateway/agent_launcher_api_test.go`
- Test: `gateway/daemonclient_test.go`
- Test: `cmd/carrier/main_agent_test.go`
- Test: `webui/src/features/agents/AgentDetailPage.test.tsx`
- Test: `webui/e2e/tests/agent-detail.spec.ts`
- Test: `webui/e2e/tests/fullstack-agent-launcher.spec.ts`

**Step 1: Write the failing tests**

Add coverage for:
- launcher history showing recent restarts and runtime failures
- delegated job detail surviving restart via durable store reload
- operator actions `run-now cron`, `restart runtime`, `inspect delegation` reflected in refreshed launcher state

**Step 2: Run test to verify it fails**

Run:
- `go test ./baseagent -run 'TestSubagentManager.*Persistence' -count=1`
- `go test ./server -run 'TestHandleAgentSubagents|TestHandleAgentCronRunNow' -count=1`
- `go test ./gateway -run 'TestHandleAgentLauncherReturnsStructuredRemediations|TestHandleWebUIAgent_SubagentListAndDetail' -count=1`
- `go test ./cmd/carrier -run 'TestParseAgentCommandArgs|TestRunAgentCommand' -count=1`
- `cd webui/e2e && bunx playwright test tests/agent-detail.spec.ts tests/fullstack-agent-launcher.spec.ts --config playwright.fullstack.config.ts --project=chromium --workers=1`

Expected: FAIL because launcher drill-down does not yet expose enough durable runtime/operator history.

**Step 3: Write minimal implementation**

Add:
- durable runtime history fields exposed through launcher summary
- explicit run-now/restart/delegation-inspect action confirmations in UI
- CLI surfaces for launcher history and recent delegation failures

Rules:
- keep launcher as a summary view backed by existing daemon/runtime state
- do not introduce separate cron history storage if existing runtime state can be extended
- full-stack tests must hit real gateway/daemon, not mocked route fulfillments

**Step 4: Run test to verify it passes**

Run the same commands from Step 2, plus:
- `cd webui && bun run test src/features/agents/AgentDetailPage.test.tsx`
- `bash scripts/build-webui.sh`

**Step 5: Commit**

```bash
git add baseagent/subagent_manager.go daemon/server/server.go gateway/agent_launcher_api.go gateway/daemonclient.go gateway/webui_agents.go cmd/carrier/main.go webui/src/features/agents/AgentDetailPage.tsx baseagent/subagent_manager_test.go daemon/server/server_test.go gateway/agent_launcher_api_test.go gateway/daemonclient_test.go cmd/carrier/main_agent_test.go webui/src/features/agents/AgentDetailPage.test.tsx webui/e2e/tests/agent-detail.spec.ts webui/e2e/tests/fullstack-agent-launcher.spec.ts
git commit -m "feat(managed-agent): deepen launcher operator drill-down"
```
