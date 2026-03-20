package baseagent

import (
	"context"
	"strings"
	"testing"
)

func TestDecomposeGoalWithOptionsCarriesSharedInstructionsAndRuntimeContext(t *testing.T) {
	original := decomposeRequestLLMCompletion
	t.Cleanup(func() {
		decomposeRequestLLMCompletion = original
	})

	var capturedSystemPrompt string
	var capturedRuntimeContext any
	decomposeRequestLLMCompletion = func(ctx context.Context, providerID, systemPrompt, userPrompt string) (string, error) {
		capturedSystemPrompt = systemPrompt
		capturedRuntimeContext, _ = RuntimeContextValue(ctx, "workspace.path")
		return `[{"id":"task-1","input":"inspect workspace"}]`, nil
	}

	tasks, err := DecomposeGoalWithOptions(context.Background(), "openrouter", "plan the work", DecomposeOptions{
		SharedInstructions: []SharedInstruction{{
			Title:   "Execution Contract",
			Content: "Do not expose hidden runtime state in the prompt.",
		}},
		RuntimeContext: []RuntimeContextEntry{{
			Key:   "workspace.path",
			Value: "/tmp/runtime-context",
			Class: "workspace",
		}},
	})
	if err != nil {
		t.Fatalf("DecomposeGoalWithOptions failed: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Input != "inspect workspace" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}
	if !strings.Contains(capturedSystemPrompt, "Do not expose hidden runtime state in the prompt.") {
		t.Fatalf("expected shared instruction in system prompt, got %q", capturedSystemPrompt)
	}
	if capturedRuntimeContext != "/tmp/runtime-context" {
		t.Fatalf("expected runtime context in decompose ctx, got %#v", capturedRuntimeContext)
	}
}

func TestRuntimeChatKeepsRuntimeContextOutOfPromptAndMakesItAvailableToTools(t *testing.T) {
	ctx := context.Background()
	sawRuntimeContext := false

	manager, err := NewManagedMCPManager(MCPConfig{
		Servers: []MCPServerConfig{{
			Name: "runtime",
			Tools: []MCPToolConfig{{
				Name:        "read_runtime_context",
				Description: "Inspect hidden runtime context state.",
			}},
		}},
	}, ManagedMCPManagerOptions{
		ServerHooks: map[string]MCPServerHooks{
			"runtime": {
				Start: func(context.Context) error { return nil },
			},
		},
		ToolRunners: map[string]MCPToolRunner{
			"read_runtime_context": func(ctx context.Context, _ map[string]any) ExecutionToolResult {
				value, ok := RuntimeContextValue(ctx, "workflow.run_id")
				sawRuntimeContext = ok && value == "run-123"
				return ExecutionToolResult{Output: "runtime context inspected"}
			},
		},
	})
	if err != nil {
		t.Fatalf("NewManagedMCPManager failed: %v", err)
	}
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("manager.Start failed: %v", err)
	}

	provider := &scriptedToolAwareProvider{
		name: "runtime-context-mcp",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{{
					ID:   "call-1",
					Name: "read_runtime_context",
					Arguments: map[string]any{
						"scope": "workflow",
					},
				}},
			},
			{
				Content: "done",
			},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithMCPManager(manager), WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(ctx, ChatRequest{
		Provider: "cli",
		ChatID:   "runtime-context-mcp",
		Message:  "inspect hidden workflow state",
		RuntimeContext: []RuntimeContextEntry{{
			Key:           "workflow.run_id",
			Value:         "run-123",
			Source:        "orchestrator",
			Class:         "workflow",
			RedactionMode: "hidden",
		}},
	})
	if err != nil {
		t.Fatalf("runtime chat failed: %v", err)
	}
	if resp.Message != "done" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if !sawRuntimeContext {
		t.Fatal("expected MCP tool runner to access runtime context")
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected two structured requests, got %d", len(provider.requests))
	}
	for _, req := range provider.requests {
		if strings.Contains(req.SystemPrompt, "run-123") {
			t.Fatalf("runtime context leaked into system prompt: %+v", provider.requests)
		}
		for _, msg := range req.Messages {
			if strings.Contains(msg.Content, "run-123") {
				t.Fatalf("runtime context leaked into provider messages: %+v", provider.requests)
			}
		}
	}
}

func TestRuntimeCapabilitySummaryIncludesRuntimeMetadata(t *testing.T) {
	ctx := context.Background()
	manager, err := NewManagedMCPManager(MCPConfig{
		Servers: []MCPServerConfig{{
			Name: "repo",
			Tools: []MCPToolConfig{{
				Name:        "read_repo_index",
				Description: "Read the repository index.",
			}},
		}},
	}, ManagedMCPManagerOptions{
		ServerHooks: map[string]MCPServerHooks{
			"repo": {
				Start: func(context.Context) error { return nil },
			},
		},
		ToolRunners: map[string]MCPToolRunner{
			"read_repo_index": func(context.Context, map[string]any) ExecutionToolResult {
				return ExecutionToolResult{Output: "ok"}
			},
		},
	})
	if err != nil {
		t.Fatalf("NewManagedMCPManager failed: %v", err)
	}
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("manager.Start failed: %v", err)
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithWorkspaceRoot(t.TempDir()), WithMCPManager(manager))
	summary := rt.CapabilitySummary(ctx)

	if !summary.Metadata.SharedInstructionsSupported || !summary.Metadata.RuntimeContextSupported || !summary.Metadata.GuardrailsSupported {
		t.Fatalf("expected runtime metadata support flags, got %+v", summary.Metadata)
	}
	if len(summary.Metadata.Tools) == 0 {
		t.Fatalf("expected runtime metadata tools, got %+v", summary.Metadata)
	}

	var execTool RuntimeToolMetadata
	var builtinTool RuntimeToolMetadata
	var mcpTool RuntimeToolMetadata
	for _, tool := range summary.Metadata.Tools {
		switch tool.Name {
		case "exec":
			execTool = tool
		case "help":
			builtinTool = tool
		case "read_repo_index":
			mcpTool = tool
		}
	}
	if execTool.Name != "exec" || execTool.Source != "workspace" || execTool.PolicyDecision != "ask" {
		t.Fatalf("unexpected exec runtime metadata: %+v", execTool)
	}
	if builtinTool.Name != "help" || builtinTool.Source != "builtin" || builtinTool.PolicyDecision != "allow" {
		t.Fatalf("unexpected builtin runtime metadata: %+v", builtinTool)
	}
	if mcpTool.Name != "read_repo_index" || mcpTool.Source != "mcp" {
		t.Fatalf("unexpected MCP runtime metadata: %+v", mcpTool)
	}
}

func TestStructuredLoopPersistsGuardrailEventsInStructuredHistory(t *testing.T) {
	root := t.TempDir()
	provider := &scriptedToolAwareProvider{
		name: "guardrail-history",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{{
					ID:   "call-1",
					Name: "exec",
					Arguments: map[string]any{
						"command": "go test ./...",
					},
				}},
			},
			{
				Content: "exec needs confirmation",
			},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithWorkspaceRoot(root), WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	if _, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "guardrail-history",
		Message:  "run diagnostics",
	}); err != nil {
		t.Fatalf("runtime chat failed: %v", err)
	}

	history := rt.sessions.StructuredHistory("cli:guardrail-history")
	if len(history) < 2 {
		t.Fatalf("expected structured history entries, got %+v", history)
	}
	var toolMessage StructuredToolMessage
	for _, item := range history {
		if item.Role == "tool" && item.ToolName == "exec" {
			toolMessage = item
		}
	}
	if len(toolMessage.GuardrailEvents) != 1 {
		t.Fatalf("expected one guardrail event, got %+v", history)
	}
	if toolMessage.GuardrailEvents[0].Scope != GuardrailScopeToolCall || toolMessage.GuardrailEvents[0].Decision != GuardrailDecisionAsk || toolMessage.GuardrailEvents[0].ToolName != "exec" {
		t.Fatalf("unexpected guardrail event: %+v", toolMessage.GuardrailEvents[0])
	}
}
