# Base Agent Core Refactor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor `baseagent` into a modular, production-grade agent core with channel/tool/provider/session architecture while keeping existing daemon/gateway compatibility.

**Architecture:** Introduce a control plane with explicit managers (`MessageBus`, `ChannelManager`, `ToolRegistry`, `ProviderManager`, `SessionManager`) and a single `AgentLoop` orchestrator. Keep `Runtime.Chat` and existing action APIs stable, but route behavior through the new loop. Preserve current memory self-heal and lifecycle operation semantics.

**Tech Stack:** Go 1.23, existing `baseagent` package, existing daemon adapters, existing LLM request path in `llm.go`.

---

### Task 1: Add Modular Core Primitives

**Files:**
- Create: `baseagent/controlplane_bus.go`
- Create: `baseagent/controlplane_channels.go`
- Create: `baseagent/controlplane_sessions.go`
- Create: `baseagent/controlplane_providers.go`
- Create: `baseagent/controlplane_tools.go`

**Step 1: Write failing tests for core primitives**
- Add focused tests for bus publish/consume, channel dispatch, provider registration, session compaction, and tool routing.

**Step 2: Run test to verify failures**
- Run: `go test ./baseagent -run 'Test(ControlPlane|MessageBus|ChannelManager|ProviderManager|SessionManager|ToolRegistry)' -v`

**Step 3: Implement primitives minimally**
- Implement concurrency-safe managers and small interfaces.
- Ensure primitives avoid external dependencies and are deterministic for unit tests.

**Step 4: Re-run tests to verify pass**
- Run: `go test ./baseagent -run 'Test(ControlPlane|MessageBus|ChannelManager|ProviderManager|SessionManager|ToolRegistry)' -v`

### Task 2: Add Agent Loop Orchestrator

**Files:**
- Create: `baseagent/agent_loop.go`
- Modify: `baseagent/runtime.go`

**Step 1: Write failing tests for loop behavior**
- Add tests for direct command/tool dispatch, help/list/status/action paths, LLM fallback path, and session history updates.

**Step 2: Run test to verify failures**
- Run: `go test ./baseagent -run 'Test(AgentLoop|RuntimeChat)' -v`

**Step 3: Implement loop and runtime wiring**
- Build `AgentLoop` to process inbound chat through tools, known-agent fallback, and provider fallback.
- Keep current output contract (`ChatResponse`, action strings, help text, error phrasing).

**Step 4: Re-run tests**
- Run: `go test ./baseagent -run 'Test(AgentLoop|RuntimeChat)' -v`

### Task 3: Register Built-in Tools and Provider Adapter

**Files:**
- Modify: `baseagent/runtime.go`
- Create: `baseagent/builtin_tools.go`

**Step 1: Add failing tests for built-in registry**
- Validate tool list, slash command aliases, and natural-language command matching.

**Step 2: Implement registry and adapter**
- Register lifecycle tools (`list`, `start`, `stop`, `status`, `logs`, `upgrade`, `diagnose`, `help`) and metadata tools (`tools`, `providers`).
- Add default provider adapter that calls existing `requestLLMCompletion` path.

**Step 3: Verify branch compatibility**
- Re-run existing runtime coverage tests to ensure no behavior regression.

### Task 4: Keep Memory Self-Heal and Action Semantics Stable

**Files:**
- Modify: `baseagent/runtime.go`
- Modify: `baseagent/runtime_coverage_test.go` (only if needed for new non-breaking fields)

**Step 1: Validate memory note behavior under new loop**
- Ensure `withMemoryNote` behavior remains unchanged for success, self-heal, and safe-mode.

**Step 2: Validate action error wrapping**
- Ensure failed actions still produce `<action> <agent> failed: <err>` with matching `Action`.

### Task 5: Verification and Cleanup

**Files:**
- Modify: `baseagent/doc.go` (if package summary needs update)

**Step 1: Run package tests**
- Run: `go test ./baseagent/...`

**Step 2: Run broader compile/test smoke**
- Run: `go test ./daemon/... ./gateway/...`

**Step 3: Confirm no API breakage**
- Verify `daemon/server` still compiles with `baseagent.NewRuntime` and `baseagent.ChatRequest/ChatResponse`.

