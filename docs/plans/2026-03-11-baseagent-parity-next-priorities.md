# Baseagent Next Parity Priorities Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Close the next highest-value parity gaps against picoclaw by prioritizing new tool capabilities and a real skills subsystem, then hardening MCP, while integrating memory through Carrier's own design instead of copying picoclaw's storage model.

**Architecture:** Keep extending the existing structured tool surface rather than adding side paths. Phase 1 grows end-user capability with new structured tools and skills orchestration. Phase 2 upgrades MCP into a real managed integration surface. Phase 3 connects baseagent to Carrier memory contracts and daemon memory policy. Channel/media parity stays out of scope for now and is tracked as TODO only.

**Tech Stack:** Go, baseagent runtime/session/provider loop, daemon HTTP server, structured tool policy/boundary spec, Carrier daemon memory packages under `daemon/internal/memory`, planning docs under `docs/plans`.

---

## Priority Order

1. **P1:** `web/search/send_file/subagent` tool family
2. **P1:** full skills subsystem
3. **P2:** managed MCP integration
4. **P3:** memory integration using Carrier design, not picoclaw storage
5. **TODO:** channels/routing/auth parity
6. **TODO:** media/voice parity

---

### Task 1: Add Missing High-Value Structured Tools

**Files:**
- Modify: `baseagent/execution_tools.go`
- Modify: `baseagent/structured_tool_surface.go`
- Modify: `baseagent/structured_policy.go`
- Modify: `baseagent/runtime.go`
- Modify: `baseagent/runtime_options.go`
- Test: `baseagent/execution_tools_test.go`
- Test: `baseagent/runtime_structured_loop_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- `web_fetch` returns fetched page text in a bounded result envelope
- `web_search` returns compact search hits
- `send_file` returns a structured attachment result for a workspace file
- `spawn_subagent` creates a delegated job/result handle instead of failing as unknown tool

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestExecutionToolsWebFetch|TestExecutionToolsWebSearch|TestExecutionToolsSendFile|TestStructuredLoopSpawnSubagent'`
Expected: FAIL because the tools do not exist yet.

**Step 3: Write the minimal implementation**

Add new structured tools with narrow first versions:
- `web_fetch`
- `web_search`
- `send_file`
- `spawn_subagent`

Implementation rules:
- keep them behind the existing structured policy surface
- classify `web_fetch` and `web_search` as read-tier tools
- classify `send_file` and `spawn_subagent` as ask-tier tools by default
- return machine-readable tool metadata using the existing `ExecutionToolResult`

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestExecutionToolsWebFetch|TestExecutionToolsWebSearch|TestExecutionToolsSendFile|TestStructuredLoopSpawnSubagent'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/execution_tools.go baseagent/structured_tool_surface.go baseagent/structured_policy.go baseagent/runtime.go baseagent/runtime_options.go baseagent/execution_tools_test.go baseagent/runtime_structured_loop_test.go
git commit -m "feat: add web and delegation structured tools"
```

### Task 2: Make Subagent Delegation Real

**Files:**
- Create: `baseagent/subagent_manager.go`
- Create: `baseagent/subagent_manager_test.go`
- Modify: `baseagent/agent_loop.go`
- Modify: `baseagent/runtime.go`
- Modify: `baseagent/structured_tool_surface.go`
- Test: `baseagent/runtime_structured_loop_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- spawning a subagent returns a stable job ID
- polling/follow-up on the delegated result works
- policy denial/ask is preserved for risky delegated requests

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestSubagentManagerSpawn|TestStructuredLoopDelegatesToSubagent'`
Expected: FAIL because there is no subagent manager/runtime bridge.

**Step 3: Write the minimal implementation**

Add a baseagent-owned subagent manager abstraction that:
- accepts a bounded task payload
- records status/result
- exposes a small execution API to the structured tool surface

Keep V1 simple:
- in-process manager
- no distributed queue
- no external worker requirement

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestSubagentManagerSpawn|TestStructuredLoopDelegatesToSubagent'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/subagent_manager.go baseagent/subagent_manager_test.go baseagent/agent_loop.go baseagent/runtime.go baseagent/structured_tool_surface.go baseagent/runtime_structured_loop_test.go
git commit -m "feat: add baseagent subagent delegation manager"
```

### Task 3: Build A Real Skills Subsystem

**Files:**
- Replace: `baseagent/skills_loader.go`
- Create: `baseagent/skills_registry.go`
- Create: `baseagent/skills_registry_test.go`
- Create: `baseagent/skills_store.go`
- Modify: `baseagent/agent_loop.go`
- Modify: `baseagent/structured_loop.go`
- Modify: `baseagent/runtime.go`
- Test: `baseagent/skills_loader_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- listing installed skills
- searching skills by keyword
- loading relevant skills into prompt context
- installing/enabling a skill updates future requests

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestSkillsRegistryList|TestSkillsRegistrySearch|TestAgentLoopInjectsRelevantSkills|TestInstallSkillAffectsFutureRequests'`
Expected: FAIL because skills are still only a summary callback.

**Step 3: Write the minimal implementation**

Replace the summary-only loader with:
- skill registry
- local skill store
- relevant-skill selector
- runtime wiring for list/search/install/use

V1 scope:
- local filesystem registry
- deterministic keyword/tag matching
- prompt summary injection remains the last-mile integration

Do not copy picoclaw's exact storage layout. Reuse Carrier conventions and keep the public surface minimal.

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestSkillsRegistryList|TestSkillsRegistrySearch|TestAgentLoopInjectsRelevantSkills|TestInstallSkillAffectsFutureRequests'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/skills_loader.go baseagent/skills_registry.go baseagent/skills_registry_test.go baseagent/skills_store.go baseagent/agent_loop.go baseagent/structured_loop.go baseagent/runtime.go baseagent/skills_loader_test.go
git commit -m "feat: add baseagent skills subsystem"
```

### Task 4: Upgrade MCP From Static Registry To Managed Integration

**Files:**
- Replace: `baseagent/mcp_manager.go`
- Create: `baseagent/mcp_config.go`
- Create: `baseagent/mcp_config_test.go`
- Modify: `baseagent/runtime.go`
- Modify: `baseagent/structured_tool_surface.go`
- Test: `baseagent/mcp_manager_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- loading MCP server config
- registering visible and hidden tools
- alias resolution
- lifecycle start/stop for MCP servers
- policy classification across MCP tools

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestMCPManagerLoadsConfig|TestMCPManagerAliases|TestStructuredSurfaceRegistersVisibleMCPTools'`
Expected: FAIL because MCP is still a static in-memory registry.

**Step 3: Write the minimal implementation**

Upgrade MCP manager to:
- parse config
- maintain server/tool registry
- expose visible structured tools
- execute tools through a managed abstraction
- preserve existing policy enforcement

Keep V1 pragmatic:
- server lifecycle in-process
- hidden tools supported
- no broad discovery UX yet

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestMCPManagerLoadsConfig|TestMCPManagerAliases|TestStructuredSurfaceRegistersVisibleMCPTools'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/mcp_manager.go baseagent/mcp_config.go baseagent/mcp_config_test.go baseagent/runtime.go baseagent/structured_tool_surface.go baseagent/mcp_manager_test.go
git commit -m "feat: add managed MCP integration for baseagent"
```

### Task 5: Integrate Memory Using Carrier's Design

**Files:**
- Modify: `baseagent/memory_store.go`
- Modify: `baseagent/runtime.go`
- Modify: `baseagent/agent_loop.go`
- Modify: `baseagent/structured_tool_surface.go`
- Reference: `daemon/internal/memory/types.go`
- Reference: `daemon/internal/memory/store.go`
- Reference: `daemon/internal/memory/policy.go`
- Reference: `docs/current-architecture.md`
- Test: `baseagent/controlplane_test.go`
- Test: `baseagent/runtime_structured_loop_test.go`

**Step 1: Write the failing tests**

Add coverage for:
- querying attached memory through Carrier memory contracts
- observing tool output into memory
- enforcing Carrier memory scope/policy before memory access

**Step 2: Run the targeted tests**

Run: `go test ./... -run 'TestBaseagentMemoryQueryUsesCarrierStore|TestStructuredLoopObserveMemory|TestMemoryPolicyBlocksUnauthorizedScope'`
Expected: FAIL because memory is still only an interface seam.

**Step 3: Write the minimal implementation**

Integrate baseagent with Carrier memory primitives:
- keep `MemoryStore` as the seam
- map baseagent memory operations onto Carrier daemon memory types/policy
- do not reproduce picoclaw's JSONL/journal store inside baseagent

Required constraints:
- follow Carrier scope/policy rules
- preserve auditability
- keep memory access under the same structured policy model when surfaced as tools

**Step 4: Run the targeted tests again**

Run: `go test ./... -run 'TestBaseagentMemoryQueryUsesCarrierStore|TestStructuredLoopObserveMemory|TestMemoryPolicyBlocksUnauthorizedScope'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/memory_store.go baseagent/runtime.go baseagent/agent_loop.go baseagent/structured_tool_surface.go baseagent/controlplane_test.go baseagent/runtime_structured_loop_test.go
git commit -m "feat: integrate baseagent with carrier memory design"
```

## Deferred TODO

These are intentionally not part of the next execution batch:

- channel/routing/auth parity with picoclaw
- media/voice parity with picoclaw

Do not start these until Tasks 1-5 are complete and re-prioritized.
