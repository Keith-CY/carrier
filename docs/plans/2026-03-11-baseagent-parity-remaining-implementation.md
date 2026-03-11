# Baseagent Parity Remaining Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete the remaining baseagent parity work for approval flow, policy externalization, argument-level controls, approval lifecycle/audit state, MCP, cron, and skills integration.

**Architecture:** Finish the control-plane hardening first so high-risk structured tools have a durable approval lifecycle and machine-readable policy outcomes. Then externalize policy and add argument-level enforcement before expanding the surface area to MCP, cron, and skills, so new tool sources enter a single policy model instead of creating parallel exceptions.

**Tech Stack:** Go, JSON boundary spec, baseagent runtime/session/provider loop, daemon HTTP server, gateway forwarding, persisted session JSON under `.baseagent/sessions`.

---

### Task 1: Approval API And Token Consume Path

**Files:**
- Create: `baseagent/approval_flow.go`
- Create: `baseagent/approval_flow_test.go`
- Modify: `baseagent/agent_loop.go`
- Modify: `baseagent/controlplane_sessions.go`
- Modify: `baseagent/runtime.go`
- Modify: `daemon/server/server.go`
- Test: `baseagent/runtime_structured_loop_test.go`
- Test: `daemon/server/server_test.go`

**Step 1: Write the failing test**

```go
func TestRuntimeRespondPendingApprovalConfirmsSpecificApproval(t *testing.T) {
    rt := newApprovalRuntimeForTest(t)
    _, _ = rt.Chat(context.Background(), ChatRequest{
        Provider: "cli",
        ChatID:   "approval-api",
        Message:  "perform maintenance planning",
    })

    pending := rt.sessions.PendingApprovals("cli:approval-api")
    resp, err := rt.RespondPendingApproval(context.Background(), "cli:approval-api", pending[0].ID, ApprovalDecisionConfirm)

    if err != nil || resp.Action != "approval_confirm" {
        t.Fatalf("expected approval confirm path, got resp=%+v err=%v", resp, err)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestRuntimeRespondPendingApprovalConfirmsSpecificApproval|TestDaemonServerApprovalEndpoint'`
Expected: FAIL with missing runtime approval API / missing daemon route.

**Step 3: Write minimal implementation**

```go
type ApprovalDecision string

const (
    ApprovalDecisionConfirm ApprovalDecision = "confirm"
    ApprovalDecisionReject  ApprovalDecision = "reject"
)

func (r *Runtime) RespondPendingApproval(ctx context.Context, sessionKey, approvalID string, decision ApprovalDecision) (ChatResponse, error) {
    return r.loop.RespondPendingApproval(ctx, sessionKey, approvalID, decision)
}
```

Add one daemon endpoint around the runtime method. Keep the first version narrow:
- confirm or reject one approval by ID
- return the resumed `ChatResponse`
- keep natural-language `confirm/cancel` as backward-compatible fallback

**Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestRuntimeRespondPendingApprovalConfirmsSpecificApproval|TestDaemonServerApprovalEndpoint'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/approval_flow.go baseagent/approval_flow_test.go baseagent/agent_loop.go baseagent/controlplane_sessions.go baseagent/runtime.go daemon/server/server.go baseagent/runtime_structured_loop_test.go daemon/server/server_test.go
git commit -m "feat: add explicit approval consume path"
```

### Task 2: Persist Full Tool Metadata In Session History

**Files:**
- Modify: `baseagent/controlplane_sessions.go`
- Modify: `baseagent/structured_provider.go`
- Modify: `baseagent/structured_loop.go`
- Create: `baseagent/session_metadata_test.go`
- Test: `baseagent/structured_provider_test.go`

**Step 1: Write the failing test**

```go
func TestSessionManagerPersistsStructuredToolMetadata(t *testing.T) {
    sm := NewSessionManagerWithStorage(8, t.TempDir())
    sm.AddStructuredToolMessage("cli:meta", StructuredToolMessage{
        Role:             "tool",
        Content:          "needs confirmation",
        ToolCallID:       "call-1",
        ToolName:         "exec",
        ToolResultStatus: ExecutionToolResultStatusAsk,
    })

    got := NewSessionManagerWithStorage(8, sm.storageDir).StructuredHistory("cli:meta")
    if len(got) != 1 || got[0].ToolName != "exec" || got[0].ToolResultStatus != ExecutionToolResultStatusAsk {
        t.Fatalf("unexpected structured history: %+v", got)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestSessionManagerPersistsStructuredToolMetadata|TestBuildStructuredToolPromptIncludesStructuredHistoryMetadata'`
Expected: FAIL because session storage still only persists flat `ConversationMessage`.

**Step 3: Write minimal implementation**

```go
type ConversationEvent struct {
    Role             string                    `json:"role"`
    Content          string                    `json:"content"`
    ToolCallID       string                    `json:"tool_call_id,omitempty"`
    ToolName         string                    `json:"tool_name,omitempty"`
    ToolResultStatus ExecutionToolResultStatus `json:"tool_result_status,omitempty"`
}
```

Use `ConversationEvent` as the persisted session record while keeping the old `History()` helper for compatibility. `StructuredHistory()` should become the source of truth for `processStructuredChat()`.

**Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestSessionManagerPersistsStructuredToolMetadata|TestBuildStructuredToolPromptIncludesStructuredHistoryMetadata'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/controlplane_sessions.go baseagent/structured_provider.go baseagent/structured_loop.go baseagent/session_metadata_test.go baseagent/structured_provider_test.go
git commit -m "feat: persist structured tool metadata in sessions"
```

### Task 3: Externalize Structured Tool Policy Into Boundary Spec And Runtime Config

**Files:**
- Modify: `baseagent/spec/baseagent-boundary.v1.json`
- Modify: `baseagent/boundary_spec.go`
- Modify: `baseagent/boundary_spec_test.go`
- Modify: `baseagent/runtime_options.go`
- Modify: `baseagent/runtime.go`
- Modify: `baseagent/structured_tool_surface.go`
- Test: `baseagent/runtime_structured_loop_test.go`

**Step 1: Write the failing test**

```go
func TestStructuredToolSurfaceReadsPolicyFromBoundarySpec(t *testing.T) {
    spec := ActiveBoundarySpec()
    spec.StructuredToolPolicy.HighRiskDecision = "deny"
    surface := newStructuredToolSurfaceWithPolicy(NewToolRegistry(), NewExecutionToolRegistry(t.TempDir()), spec.StructuredToolPolicy)

    result := surface.Execute(context.Background(), "exec", map[string]any{"command": "go test ./..."})
    if result.Status != ExecutionToolResultStatusDeny {
        t.Fatalf("expected boundary policy to drive deny, got %+v", result)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestStructuredToolSurfaceReadsPolicyFromBoundarySpec|TestBoundarySpecRendersStructuredToolPolicy'`
Expected: FAIL because policy is still hardcoded in `defaultStructuredToolPolicy`.

**Step 3: Write minimal implementation**

```go
type StructuredToolPolicySpec struct {
    MetadataReadDecision      string `json:"metadata_read_decision"`
    OperationalReadDecision   string `json:"operational_read_decision"`
    WorkspaceReadDecision     string `json:"workspace_read_decision"`
    WorkspaceMutationDecision string `json:"workspace_mutation_decision"`
    HighRiskDecision          string `json:"high_risk_decision"`
}
```

Wire the active boundary spec into runtime construction. Keep runtime options able to override the spec for tests.

**Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestStructuredToolSurfaceReadsPolicyFromBoundarySpec|TestBoundarySpecRendersStructuredToolPolicy'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/spec/baseagent-boundary.v1.json baseagent/boundary_spec.go baseagent/boundary_spec_test.go baseagent/runtime_options.go baseagent/runtime.go baseagent/structured_tool_surface.go baseagent/runtime_structured_loop_test.go
git commit -m "feat: externalize structured tool policy"
```

### Task 4: Add Argument-Level Policy And Audit Reasons

**Files:**
- Create: `baseagent/structured_policy.go`
- Create: `baseagent/structured_policy_test.go`
- Modify: `baseagent/structured_tool_surface.go`
- Modify: `baseagent/execution_tools.go`
- Modify: `baseagent/policy.go`
- Modify: `baseagent/structured_provider.go`

**Step 1: Write the failing test**

```go
func TestStructuredPolicyClassifiesExecCommandsDifferently(t *testing.T) {
    allow := evaluateStructuredToolPolicy("exec", map[string]any{"command": "go test ./..."})
    deny := evaluateStructuredToolPolicy("exec", map[string]any{"command": "rm -rf /"})

    if allow.Decision != structuredToolDecisionAsk {
        t.Fatalf("expected bounded test command to require confirmation, got %+v", allow)
    }
    if deny.Decision != structuredToolDecisionDeny || deny.RuleID == "" {
        t.Fatalf("expected dangerous command deny with rule id, got %+v", deny)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestStructuredPolicyClassifiesExecCommandsDifferently|TestStructuredToolPromptIncludesPolicyReasonMetadata'`
Expected: FAIL because policy still only keys on tool name/tier.

**Step 3: Write minimal implementation**

```go
type StructuredPolicyDecision struct {
    Decision structuredToolDecision
    Reason   string
    RuleID   string
}
```

Add argument-sensitive policy hooks for:
- `exec.command`
- `agent_start/stop/upgrade/uninstall/diagnose`
- future MCP/cron routed tools

Expose `reason` and `rule_id` in structured tool result metadata so providers and logs can explain why a call was allowed, asked, or denied.

**Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestStructuredPolicyClassifiesExecCommandsDifferently|TestStructuredToolPromptIncludesPolicyReasonMetadata'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/structured_policy.go baseagent/structured_policy_test.go baseagent/structured_tool_surface.go baseagent/execution_tools.go baseagent/policy.go baseagent/structured_provider.go
git commit -m "feat: add argument-level structured tool policy"
```

### Task 5: Expand Approval Lifecycle To Multiple Pending Items, Expiry, And Audit State

**Files:**
- Modify: `baseagent/controlplane_sessions.go`
- Modify: `baseagent/approval_flow.go`
- Modify: `baseagent/agent_loop.go`
- Create: `baseagent/approval_store_test.go`
- Test: `baseagent/execution_tools_test.go`
- Test: `baseagent/runtime_structured_loop_test.go`

**Step 1: Write the failing test**

```go
func TestSessionManagerStoresMultiplePendingApprovalsWithExpiry(t *testing.T) {
    sm := NewSessionManager(8)
    sm.SetPendingApprovals("cli:multi", []*PendingToolApproval{
        {ID: "a1", ToolName: "exec"},
        {ID: "a2", ToolName: "agent_start"},
    })

    got := sm.PendingApprovals("cli:multi")
    if len(got) != 2 {
        t.Fatalf("expected 2 pending approvals, got %+v", got)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestSessionManagerStoresMultiplePendingApprovalsWithExpiry|TestRuntimeRejectsExpiredApproval'`
Expected: FAIL because sessions still store only one pending approval and no expiry.

**Step 3: Write minimal implementation**

```go
type PendingToolApproval struct {
    ID          string
    ToolName    string
    Arguments   map[string]any
    RequestedAt time.Time
    ExpiresAt   time.Time
    Decision    string
    Reason      string
    RuleID      string
}
```

Keep lookup by `approval_id`. Add cleanup of expired items during read/write. Preserve resolved audit state long enough for session replay.

**Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestSessionManagerStoresMultiplePendingApprovalsWithExpiry|TestRuntimeRejectsExpiredApproval'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/controlplane_sessions.go baseagent/approval_flow.go baseagent/agent_loop.go baseagent/approval_store_test.go baseagent/execution_tools_test.go baseagent/runtime_structured_loop_test.go
git commit -m "feat: expand approval lifecycle state"
```

### Task 6: Add MCP Manager Integration Under The Structured Policy Model

**Files:**
- Create: `baseagent/mcp_manager.go`
- Create: `baseagent/mcp_manager_test.go`
- Modify: `baseagent/runtime.go`
- Modify: `baseagent/structured_tool_surface.go`
- Modify: `baseagent/structured_provider.go`
- Modify: `daemon/server/server.go`

**Step 1: Write the failing test**

```go
func TestMCPToolsAppearInStructuredSurfaceUnderPolicy(t *testing.T) {
    mgr := newFakeMCPManager()
    mgr.RegisterTool("repo_search", fakeMCPToolSpec())

    rt := NewRuntime(&runtimeServiceFake{}, nil, WithMCPManager(mgr))
    got := rt.loop.structuredTools.Descriptors()

    if !containsStructuredTool(got, "repo_search") {
        t.Fatalf("expected MCP tool descriptor, got %+v", got)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestMCPToolsAppearInStructuredSurfaceUnderPolicy|TestMCPToolExecutionRespectsPolicyDecision'`
Expected: FAIL because no MCP manager exists in baseagent runtime.

**Step 3: Write minimal implementation**

```go
type MCPManager interface {
    ListStructuredTools() []StructuredToolDescriptor
    ExecuteTool(ctx context.Context, name string, args map[string]any) ExecutionToolResult
}
```

Register MCP tools as another policy-governed source in `structuredToolSurface`. Do not bypass policy for MCP.

**Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestMCPToolsAppearInStructuredSurfaceUnderPolicy|TestMCPToolExecutionRespectsPolicyDecision'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/mcp_manager.go baseagent/mcp_manager_test.go baseagent/runtime.go baseagent/structured_tool_surface.go baseagent/structured_provider.go daemon/server/server.go
git commit -m "feat: add mcp tool integration"
```

### Task 7: Add Cron/Scheduled Re-entry Into The Structured Loop

**Files:**
- Create: `baseagent/cron_service.go`
- Create: `baseagent/cron_service_test.go`
- Modify: `baseagent/runtime.go`
- Modify: `baseagent/agent_loop.go`
- Modify: `baseagent/controlplane_sessions.go`
- Modify: `daemon/server/server.go`

**Step 1: Write the failing test**

```go
func TestCronJobReentersStructuredLoopWithSessionContext(t *testing.T) {
    rt := newCronRuntimeForTest(t)
    _, err := rt.ScheduleJob(CronJob{
        SessionKey: "cron:check",
        Prompt:     "perform maintenance planning",
    })
    if err != nil {
        t.Fatalf("schedule job: %v", err)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestCronJobReentersStructuredLoopWithSessionContext|TestCronJobRespectsPendingApprovalPolicy'`
Expected: FAIL because no cron service exists.

**Step 3: Write minimal implementation**

```go
type CronJob struct {
    ID         string
    SessionKey string
    Prompt     string
    NextRunAt  time.Time
}
```

Use a simple in-process scheduler first. The key rule: cron jobs must re-enter `ProcessChat()` or the same structured loop path so approvals and policy are enforced identically to interactive requests.

**Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestCronJobReentersStructuredLoopWithSessionContext|TestCronJobRespectsPendingApprovalPolicy'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/cron_service.go baseagent/cron_service_test.go baseagent/runtime.go baseagent/agent_loop.go baseagent/controlplane_sessions.go daemon/server/server.go
git commit -m "feat: add cron re-entry for baseagent"
```

### Task 8: Add Skills Loading And Provider Context Integration

**Files:**
- Create: `baseagent/skills_loader.go`
- Create: `baseagent/skills_loader_test.go`
- Modify: `baseagent/runtime.go`
- Modify: `baseagent/structured_provider.go`
- Modify: `baseagent/agent_loop.go`

**Step 1: Write the failing test**

```go
func TestProviderRequestIncludesRelevantSkillsSummary(t *testing.T) {
    loader := newFakeSkillsLoader("go-testing", "Use go test before claiming success.")
    rt := NewRuntime(&runtimeServiceFake{}, nil, WithSkillsLoader(loader))

    _ = rt.Chat(context.Background(), ChatRequest{
        Provider: "cli",
        ChatID:   "skills",
        Message:  "run repository diagnostics",
    })

    if !loader.WasConsulted() {
        t.Fatal("expected skills loader to contribute provider context")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestProviderRequestIncludesRelevantSkillsSummary|TestStructuredLoopCarriesSkillSummaryAcrossTurns'`
Expected: FAIL because runtime does not yet know about skills.

**Step 3: Write minimal implementation**

```go
type SkillsLoader interface {
    RelevantSkills(query string) []SkillSummary
}
```

Inject compact skill summaries into:
- provider prompt construction
- structured loop system/context
- optionally `/tools` or `/boundaries` diagnostics

Keep the first version summary-only. Do not build auto-install/mutation behavior here.

**Step 4: Run test to verify it passes**

Run: `go test ./... -run 'TestProviderRequestIncludesRelevantSkillsSummary|TestStructuredLoopCarriesSkillSummaryAcrossTurns'`
Expected: PASS

**Step 5: Commit**

```bash
git add baseagent/skills_loader.go baseagent/skills_loader_test.go baseagent/runtime.go baseagent/structured_provider.go baseagent/agent_loop.go
git commit -m "feat: add skills context integration"
```

### Task 9: Final Hardening, End-To-End Verification, And Docs

**Files:**
- Modify: `baseagent/spec/baseagent-boundary.v1.json`
- Modify: `baseagent/boundary_spec.go`
- Modify: `baseagent/runtime_structured_loop_test.go`
- Modify: `baseagent/controlplane_test.go`
- Modify: `daemon/server/server_test.go`
- Modify: `docs/plans/2026-03-11-baseagent-parity-remaining-implementation.md`

**Step 1: Write the failing test**

```go
func TestEndToEndApprovalMCPAndCronShareOnePolicyModel(t *testing.T) {
    t.Fatal("write this integration test before the final pass")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./... -run 'TestEndToEndApprovalMCPAndCronShareOnePolicyModel'`
Expected: FAIL by design until the whole stack is integrated.

**Step 3: Write minimal implementation**

The final implementation pass should only do cleanup:
- remove duplicated policy helpers
- ensure `/boundaries` explains approval/API behavior
- ensure daemon routes and runtime methods align
- ensure structured provider metadata remains stable

**Step 4: Run test to verify it passes**

Run:

```bash
go test ./...                               # in baseagent
go test ./...                               # in gateway
go test ./server                            # in daemon
```

Expected: PASS

**Step 5: Commit**

```bash
git add baseagent daemon/server docs/plans/2026-03-11-baseagent-parity-remaining-implementation.md
git commit -m "feat: finish remaining baseagent parity work"
```
