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

func TestManagedMCPManagerVisibleCapabilitySummary(t *testing.T) {
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
		ServerHooks: map[string]MCPServerHooks{
			"repo": {
				Start: func(context.Context) error { return nil },
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

	summary := manager.CapabilitySummary()
	if len(summary.Servers) != 1 {
		t.Fatalf("expected 1 server in capability summary, got %+v", summary)
	}
	if summary.Servers[0].Name != "repo" || summary.Servers[0].Health != "healthy" {
		t.Fatalf("unexpected server summary: %+v", summary.Servers[0])
	}
	if summary.Servers[0].VisibleToolCount != 1 || summary.Servers[0].HiddenToolCount != 1 {
		t.Fatalf("unexpected tool counts: %+v", summary.Servers[0])
	}
	if len(summary.VisibleTools) != 1 || summary.VisibleTools[0].Name != "repo_search" {
		t.Fatalf("unexpected visible tool summary: %+v", summary.VisibleTools)
	}
}

func TestManagedMCPManagerServerDetail(t *testing.T) {
	cfg := MCPConfig{
		Servers: []MCPServerConfig{
			{
				Name: "repo",
				Tools: []MCPToolConfig{
					{Name: "repo_search", Description: "Search the repository index."},
					{Name: "repo_write", Description: "Write to the repository index.", Hidden: true},
				},
			},
		},
	}

	manager, err := NewManagedMCPManager(cfg, ManagedMCPManagerOptions{})
	if err != nil {
		t.Fatalf("new managed mcp manager: %v", err)
	}

	detail, err := manager.ServerDetail("repo")
	if err != nil {
		t.Fatalf("server detail: %v", err)
	}
	if detail.Name != "repo" || detail.Health != "healthy" || !detail.Enabled {
		t.Fatalf("unexpected mcp detail: %+v", detail)
	}
	if detail.VisibleToolCount != 1 || detail.HiddenToolCount != 1 {
		t.Fatalf("unexpected tool counts: %+v", detail)
	}
	if len(detail.VisibleTools) != 1 || detail.VisibleTools[0].Name != "repo_search" {
		t.Fatalf("unexpected visible tools: %+v", detail.VisibleTools)
	}
	if len(detail.HiddenTools) != 1 || detail.HiddenTools[0].Name != "repo_write" {
		t.Fatalf("unexpected hidden tools: %+v", detail.HiddenTools)
	}
	if detail.HealthDetail == "" || detail.RemediationHint == "" {
		t.Fatalf("expected detail metadata, got %+v", detail)
	}
}

func TestManagedMCPManagerSetServerEnabled(t *testing.T) {
	cfg := MCPConfig{
		Servers: []MCPServerConfig{
			{
				Name: "repo",
				Tools: []MCPToolConfig{
					{
						Name:        "repo_search",
						Description: "Search the repository index.",
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
		},
	})
	if err != nil {
		t.Fatalf("new managed mcp manager: %v", err)
	}

	initial := manager.CapabilitySummary()
	if len(initial.Servers) != 1 || !initial.Servers[0].Enabled {
		t.Fatalf("expected server enabled by default, got %+v", initial.Servers)
	}
	if len(initial.VisibleTools) != 1 || initial.VisibleTools[0].Name != "repo_search" {
		t.Fatalf("expected visible tools while enabled, got %+v", initial.VisibleTools)
	}

	if err := manager.SetServerEnabled(context.Background(), "repo", false); err != nil {
		t.Fatalf("disable server: %v", err)
	}
	if stopped != 1 {
		t.Fatalf("expected one stop call, got %d", stopped)
	}
	disabled := manager.CapabilitySummary()
	if disabled.Servers[0].Enabled || disabled.Servers[0].Health != "stopped" {
		t.Fatalf("unexpected disabled server summary: %+v", disabled.Servers[0])
	}
	if len(disabled.VisibleTools) != 0 {
		t.Fatalf("expected no visible tools after disable, got %+v", disabled.VisibleTools)
	}
	if result := manager.ExecuteTool(context.Background(), "repo_search", map[string]any{}); !result.IsError || result.Output == "" {
		t.Fatalf("expected disabled server to block tool execution, got %+v", result)
	}

	if err := manager.SetServerEnabled(context.Background(), "repo", true); err != nil {
		t.Fatalf("enable server: %v", err)
	}
	if started != 1 {
		t.Fatalf("expected one start call, got %d", started)
	}
	enabled := manager.CapabilitySummary()
	if !enabled.Servers[0].Enabled || enabled.Servers[0].Health != "healthy" {
		t.Fatalf("unexpected enabled server summary: %+v", enabled.Servers[0])
	}
	if len(enabled.VisibleTools) != 1 || enabled.VisibleTools[0].Name != "repo_search" {
		t.Fatalf("unexpected visible tools after enable: %+v", enabled.VisibleTools)
	}

	if result := manager.ExecuteTool(context.Background(), "repo_search", map[string]any{}); result.IsError || result.Output != "found" {
		t.Fatalf("expected tool execution to work while enabled, got %+v", result)
	}
}

func TestRuntimeSetMCPServerEnabledRefreshesStructuredTools(t *testing.T) {
	cfg := MCPConfig{
		Servers: []MCPServerConfig{
			{
				Name: "repo",
				Tools: []MCPToolConfig{
					{
						Name:        "repo_search",
						Description: "Search the repository index.",
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
		},
	})
	if err != nil {
		t.Fatalf("new managed mcp manager: %v", err)
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithMCPManager(manager))
	if !containsStructuredTool(rt.loop.structuredTools.Descriptors(), "repo_search") {
		t.Fatalf("expected runtime to expose repo_search before disable, got %+v", rt.loop.structuredTools.Descriptors())
	}

	if err := rt.SetMCPServerEnabled(context.Background(), "repo", false); err != nil {
		t.Fatalf("disable server through runtime: %v", err)
	}
	if containsStructuredTool(rt.loop.structuredTools.Descriptors(), "repo_search") {
		t.Fatalf("expected runtime to hide repo_search after disable, got %+v", rt.loop.structuredTools.Descriptors())
	}

	if err := rt.SetMCPServerEnabled(context.Background(), "repo", true); err != nil {
		t.Fatalf("enable server through runtime: %v", err)
	}
	if !containsStructuredTool(rt.loop.structuredTools.Descriptors(), "repo_search") {
		t.Fatalf("expected runtime to expose repo_search after re-enable, got %+v", rt.loop.structuredTools.Descriptors())
	}
}
