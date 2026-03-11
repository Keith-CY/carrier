package baseagent

import (
	"context"
	"testing"
)

func containsStructuredTool(tools []StructuredToolDescriptor, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func TestMCPToolsAppearInStructuredSurfaceUnderPolicy(t *testing.T) {
	mgr := NewStaticMCPManager()
	mgr.RegisterTool("repo_search", StructuredToolDescriptor{
		Name:        "repo_search",
		Description: "Search the repository index.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
			"required": []string{"query"},
		},
	}, func(context.Context, map[string]any) ExecutionToolResult {
		return ExecutionToolResult{Output: "found"}
	})

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithMCPManager(mgr))
	got := rt.loop.structuredTools.Descriptors()

	if !containsStructuredTool(got, "repo_search") {
		t.Fatalf("expected MCP tool descriptor, got %+v", got)
	}
}

func TestMCPToolExecutionRespectsPolicyDecision(t *testing.T) {
	mgr := NewStaticMCPManager()
	mgr.RegisterTool("repo_write", StructuredToolDescriptor{
		Name:        "repo_write",
		Description: "Write to the repository index.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
			"required": []string{"path"},
		},
	}, func(context.Context, map[string]any) ExecutionToolResult {
		return ExecutionToolResult{Output: "wrote"}
	})

	policy := ActiveBoundarySpec().StructuredToolPolicy
	policy.HighRiskDecision = string(structuredToolDecisionDeny)

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithMCPManager(mgr), WithStructuredToolPolicy(policy))
	result := rt.loop.structuredTools.Execute(context.Background(), "repo_write", map[string]any{"path": "README.md"})
	if result.Status != ExecutionToolResultStatusDeny {
		t.Fatalf("expected MCP tool execution to respect policy deny, got %+v", result)
	}
}
