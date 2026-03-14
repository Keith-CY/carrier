# PicoClaw Parity Next Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Continue single-agent parity after Tasks 1-16 by deepening provider/model direct-surface ergonomics, rich media/runtime behavior, and launcher-grade standalone UX without expanding into multi-channel parity.

**Architecture:** Add a stable managed-agent `models` control surface that can inspect and resync agent-local model configuration from persisted managed config files. Build later parity slices on top of that API instead of continuing to overload launcher summary. Then continue with richer media runtime behavior, skill/MCP lifecycle management, and launcher-focused UX.

**Tech Stack:** Go (`gateway`, `cmd/carrier`, `daemon/server`, `baseagent`), React WebUI, Playwright E2E, existing managed instance store/config renderers.

---

### Task 17: Add Managed Agent Model Surface Inspect And Sync APIs

Status: done

**Files:**
- Create: `gateway/managed_model_surface.go`
- Modify: `gateway/webui_agents.go`
- Modify: `gateway/agent_launcher_api_test.go`
- Modify: `cmd/carrier/main.go`
- Modify: `cmd/carrier/main_agent_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- `GET /api/v1/agents/:id/models`
- `POST /api/v1/agents/:id/models/sync`
- sync rebuilding model surface from managed config file for PicoClaw and ZeroClaw
- `carrier agent models <agent_id>`
- `carrier agent models sync <agent_id>`

**Step 2: Run test to verify it fails**

Run: `go test ./gateway ./cmd/carrier -run 'TestHandleAgentModels|TestParseAgentCommandArgs|TestRunAgentCommand' -count=1`
Expected: FAIL because no dedicated agent model API or CLI exists.

**Step 3: Write minimal implementation**

Add:
- gateway-side model surface parser for managed JSON/TOML configs
- `GET /api/v1/agents/:id/models`
- `POST /api/v1/agents/:id/models/sync`
- CLI `carrier agent models` and `carrier agent models sync`

Rules:
- PicoClaw/ZeroClaw configs must round-trip into the same normalized `managedAgentModelSurface`
- OpenClaw may return current stored surface unchanged if config lacks direct-surface fields
- sync updates managed instance store in place

**Step 4: Run test to verify it passes**

Run: `go test ./gateway ./cmd/carrier -run 'TestHandleAgentModels|TestParseAgentCommandArgs|TestRunAgentCommand' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/managed_model_surface.go gateway/webui_agents.go gateway/agent_launcher_api_test.go cmd/carrier/main.go cmd/carrier/main_agent_test.go
git commit -m "feat(picoclaw): add managed agent model surface sync"
```

### Task 18: Surface Agent Model Sync In Agent Detail

Status: done

**Files:**
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Modify: `webui/src/features/agents/AgentDetailPage.test.tsx`
- Modify: `webui/e2e/tests/agent-detail.spec.ts`

**Step 1: Write the failing tests**

Add coverage for:
- Agent Detail reading `/api/v1/agents/:id/models`
- `Sync models` action calling `/api/v1/agents/:id/models/sync`
- updated default profile/profile list after sync

**Step 2: Run test to verify it fails**

Run: `cd webui && bun test src/features/agents/AgentDetailPage.test.tsx && cd e2e && bunx playwright test tests/agent-detail.spec.ts --config playwright.config.ts --project=chromium --workers=1`
Expected: FAIL because Agent Detail only reads launcher summary.

**Step 3: Write minimal implementation**

Add:
- separate query for models API
- `Sync models` button
- optimistic status message + refetch launcher/models after sync

**Step 4: Run test to verify it passes**

Run: `cd webui && bun test src/features/agents/AgentDetailPage.test.tsx && cd e2e && bunx playwright test tests/agent-detail.spec.ts --config playwright.config.ts --project=chromium --workers=1`
Expected: PASS

**Step 5: Commit**

```bash
git add webui/src/features/agents/AgentDetailPage.tsx webui/src/features/agents/AgentDetailPage.test.tsx webui/e2e/tests/agent-detail.spec.ts
git commit -m "feat(picoclaw): add agent model sync controls"
```

### Task 19: Add Model Override/Fallback Hit Telemetry

Status: done

**Files:**
- Modify: `daemon/server/managed_agent_proxy.go`
- Modify: `gateway/agent_launcher_api.go`
- Modify: `gateway/orchestrator_observability_api.go`
- Test: `daemon/server/server_handlers_coverage_test.go`
- Test: `gateway/orchestrator_observability_api_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- selected `modelAlias/model` producing launcher-visible last-used metadata
- fallback/override hits appearing in observability summaries

**Step 2: Run test to verify it fails**

Run: `go test ./daemon/server ./gateway -run 'TestHandleAgentChat.*Model|TestOrchestratorMetricsSummary.*Fallback' -count=1`
Expected: FAIL because runtime does not persist model override hit metadata.

**Step 3: Write minimal implementation**

Persist:
- last requested alias/model
- last resolved model
- fallback hit boolean / group
- last run timestamp

**Step 4: Run test to verify it passes**

Run: `go test ./daemon/server ./gateway -run 'TestHandleAgentChat.*Model|TestOrchestratorMetricsSummary.*Fallback' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add daemon/server/managed_agent_proxy.go gateway/agent_launcher_api.go gateway/orchestrator_observability_api.go daemon/server/server_handlers_coverage_test.go gateway/orchestrator_observability_api_test.go
git commit -m "feat(picoclaw): record managed model override telemetry"
```

### Task 20: Add Rich Outbound Media Render Parity

Status: done

**Files:**
- Modify: `gateway/telegram_transport.go`
- Modify: `baseagent/controlplane_bus.go`
- Modify: `gateway/orchestrator_evidence_api.go`
- Test: `gateway/telegram_transport_test.go`
- Test: `gateway/orchestrator_evidence_api_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- outbound image/document blocks preferring richer Telegram render paths
- evidence bundle including richer media manifest linkage

**Step 2: Run test to verify it fails**

Run: `go test ./gateway -run 'TestTelegramTransport.*Rich|TestHandleOrchestratorExecutionEvidence.*Media' -count=1`
Expected: FAIL because rich outbound render remains mostly text fallback.

**Step 3: Write minimal implementation**

Add:
- richer Telegram outbound branch selection for image/document blocks
- evidence linkage back to attachment/artifact metadata

**Step 4: Run test to verify it passes**

Run: `go test ./gateway -run 'TestTelegramTransport.*Rich|TestHandleOrchestratorExecutionEvidence.*Media' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add gateway/telegram_transport.go baseagent/controlplane_bus.go gateway/orchestrator_evidence_api.go gateway/telegram_transport_test.go gateway/orchestrator_evidence_api_test.go
git commit -m "feat(picoclaw): improve rich media render parity"
```

### Task 21: Add Live Transcription Hard-Pass Coverage

Status: done

**Files:**
- Modify: `scripts/e2e-control-plane-live-provider.sh`
- Modify: `.github/workflows/control-plane-live-provider-schedule.yml`

**Step 1: Write the failing test/check**

Add a script assertion for hard-required transcription when:
- `CARRIER_LIVE_REQUIRE_TRANSCRIPTION=1`
- provider advertises an audio-capable path

**Step 2: Run check to verify it fails**

Run: `bash -n scripts/e2e-control-plane-live-provider.sh`
Expected: FAIL only after adding incompatible assertions; adjust until syntax is valid and logic is enforced.

**Step 3: Write minimal implementation**

Make live smoke:
- hard-fail on missing transcription when explicitly required
- continue skipping only for known account/provider limitations

**Step 4: Run verification**

Run: `bash -n scripts/e2e-control-plane-live-provider.sh`
Expected: PASS

**Step 5: Commit**

```bash
git add scripts/e2e-control-plane-live-provider.sh .github/workflows/control-plane-live-provider-schedule.yml
git commit -m "test(picoclaw): support required transcription live smoke"
```

### Task 22: Add Skill Lifecycle Management Beyond Install

Status: done

**Files:**
- Modify: `daemon/server/server.go`
- Modify: `gateway/daemonclient.go`
- Modify: `gateway/webui_agents.go`
- Modify: `cmd/carrier/main.go`
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Test: daemon/gateway/CLI/WebUI agent skill tests

**Step 1: Write the failing tests**

Add coverage for:
- `skills uninstall`
- skill source/version metadata
- agent detail rendering of source/version/install state

**Step 2: Run test to verify it fails**

Run: targeted daemon/gateway/CLI/WebUI agent skill tests
Expected: FAIL because only search/install/enable currently exist.

**Step 3: Write minimal implementation**

Add:
- uninstall API
- optional source/version fields in skill payloads
- CLI/WebUI rendering for lifecycle state

**Step 4: Run tests to verify it passes**

Run: targeted daemon/gateway/CLI/WebUI tests
Expected: PASS

**Step 5: Commit**

```bash
git add daemon/server/server.go gateway/daemonclient.go gateway/webui_agents.go cmd/carrier/main.go webui/src/features/agents/AgentDetailPage.tsx
git commit -m "feat(picoclaw): expand managed skill lifecycle controls"
```

### Task 23: Add Durable Delegated Job History

Status: done

**Files:**
- Modify: `baseagent/subagent_manager.go`
- Modify: `baseagent/runtime.go`
- Modify: `daemon/server/server.go`
- Test: `baseagent/subagent_manager_test.go`
- Test: `daemon/server/server_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- completed/cancelled delegated jobs remaining queryable
- recent job history surfaced to runtime/server

**Step 2: Run test to verify it fails**

Run: `go test ./baseagent ./daemon/server -run 'TestSubagentManager.*History|TestHandleAgent.*Subagent' -count=1`
Expected: FAIL because current lifecycle is only current-state oriented.

**Step 3: Write minimal implementation**

Add:
- bounded recent-history retention
- query path for recent delegated jobs

**Step 4: Run test to verify it passes**

Run: `go test ./baseagent ./daemon/server -run 'TestSubagentManager.*History|TestHandleAgent.*Subagent' -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/subagent_manager.go baseagent/runtime.go daemon/server/server.go baseagent/subagent_manager_test.go daemon/server/server_test.go
git commit -m "feat(picoclaw): retain delegated job history"
```

### Task 24: Add Launcher Remediation And Cron History UX

Status: done

**Files:**
- Modify: `webui/src/features/agents/AgentDetailPage.tsx`
- Modify: `webui/src/features/agents/AgentDetailPage.test.tsx`
- Modify: `webui/e2e/tests/agent-detail.spec.ts`

**Step 1: Write the failing tests**

Add coverage for:
- cron history rendering
- remediation callouts for provider-not-ready / stale heartbeat / paused cron

**Step 2: Run test to verify it fails**

Run: targeted WebUI unit + Playwright agent-detail tests
Expected: FAIL because launcher page only shows raw summaries.

**Step 3: Write minimal implementation**

Add:
- remediation callouts
- cron history block
- launcher troubleshooting hints

**Step 4: Run test to verify it passes**

Run: targeted WebUI unit + Playwright agent-detail tests
Expected: PASS

**Step 5: Commit**

```bash
git add webui/src/features/agents/AgentDetailPage.tsx webui/src/features/agents/AgentDetailPage.test.tsx webui/e2e/tests/agent-detail.spec.ts
git commit -m "feat(picoclaw): add launcher remediation and cron history ui"
```
