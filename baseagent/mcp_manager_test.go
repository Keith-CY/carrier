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

func TestMCPManagerLoadsConfig(t *testing.T) {
	cfg := MCPConfig{
		Servers: []MCPServerConfig{
			{
				Name: "repo",
				Tools: []MCPToolConfig{
					{
						Name:        "repo_search",
						Description: "Search the repository index.",
						Parameters: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"query": map[string]any{"type": "string"},
							},
						},
					},
					{
						Name:        "repo_write",
						Description: "Write to the repository index.",
						Hidden:      true,
					},
				},
			},
		},
	}

	started := 0
	stopped := 0
	manager, err := NewManagedMCPManager(cfg, ManagedMCPManagerOptions{
		ServerHooks: map[string]MCPServerHooks{
			"repo": {
				Start: func(context.Context) error {
					started++
					return nil
				},
				Stop: func(context.Context) error {
					stopped++
					return nil
				},
			},
		},
		ToolRunners: map[string]MCPToolRunner{
			"repo_search": func(context.Context, map[string]any) ExecutionToolResult {
				return ExecutionToolResult{Output: "found"}
			},
			"repo_write": func(context.Context, map[string]any) ExecutionToolResult {
				return ExecutionToolResult{Output: "wrote"}
			},
		},
	})
	if err != nil {
		t.Fatalf("new managed mcp manager: %v", err)
	}

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatalf("stop manager: %v", err)
	}
	if started != 1 || stopped != 1 {
		t.Fatalf("expected lifecycle hooks once each, got started=%d stopped=%d", started, stopped)
	}

	tools := manager.ListStructuredTools()
	if !containsStructuredTool(tools, "repo_search") {
		t.Fatalf("expected visible tool in structured list, got %+v", tools)
	}
	if containsStructuredTool(tools, "repo_write") {
		t.Fatalf("did not expect hidden tool in structured list, got %+v", tools)
	}

	result := manager.ExecuteTool(context.Background(), "repo_write", map[string]any{"path": "README.md"})
	if result.IsError || result.Output != "wrote" {
		t.Fatalf("expected hidden tool to remain executable, got %+v", result)
	}
}

func TestMCPManagerAliases(t *testing.T) {
	cfg := MCPConfig{
		Servers: []MCPServerConfig{
			{
				Name: "repo",
				Tools: []MCPToolConfig{
					{
						Name:        "repo_search",
						Description: "Search the repository index.",
						Aliases:     []string{"repo_query"},
					},
				},
			},
		},
	}

	manager, err := NewManagedMCPManager(cfg, ManagedMCPManagerOptions{
		ToolRunners: map[string]MCPToolRunner{
			"repo_search": func(_ context.Context, args map[string]any) ExecutionToolResult {
				return ExecutionToolResult{Output: stringifyStructuredToolArg(args["query"])}
			},
		},
	})
	if err != nil {
		t.Fatalf("new managed mcp manager: %v", err)
	}

	result := manager.ExecuteTool(context.Background(), "repo_query", map[string]any{"query": "openclaw"})
	if result.IsError || result.Output != "openclaw" {
		t.Fatalf("expected alias to resolve to canonical tool, got %+v", result)
	}
}

func TestStructuredSurfaceRegistersVisibleMCPTools(t *testing.T) {
	cfg := MCPConfig{
		Servers: []MCPServerConfig{
			{
				Name: "repo",
				Tools: []MCPToolConfig{
					{
						Name:        "repo_search",
						Description: "Search the repository index.",
					},
					{
						Name:        "repo_write",
						Description: "Write to the repository index.",
						Hidden:      true,
					},
				},
			},
		},
	}

	manager, err := NewManagedMCPManager(cfg, ManagedMCPManagerOptions{
		ToolRunners: map[string]MCPToolRunner{
			"repo_search": func(context.Context, map[string]any) ExecutionToolResult {
				return ExecutionToolResult{Output: "found"}
			},
			"repo_write": func(context.Context, map[string]any) ExecutionToolResult {
				return ExecutionToolResult{Output: "wrote"}
			},
		},
	})
	if err != nil {
		t.Fatalf("new managed mcp manager: %v", err)
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithMCPManager(manager))
	got := rt.loop.structuredTools.Descriptors()

	if !containsStructuredTool(got, "repo_search") {
		t.Fatalf("expected visible MCP tool descriptor, got %+v", got)
	}
	if containsStructuredTool(got, "repo_write") {
		t.Fatalf("did not expect hidden MCP tool descriptor, got %+v", got)
	}
}
