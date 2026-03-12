package baseagent

import (
	"context"
	"strings"
	"testing"
)

func TestAgentLoopInjectsRelevantSkills(t *testing.T) {
	registry := NewSkillsRegistry(NewMemorySkillsStore(), testSkillCatalog())
	if _, err := registry.InstallSkill(context.Background(), "go-testing"); err != nil {
		t.Fatalf("install skill: %v", err)
	}

	provider := &scriptedTextProvider{
		name:    "skills-text",
		replies: []string{"done"},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithSkillsLoader(registry))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	_, _ = rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "skills",
		Message:  "run repository diagnostics and verify with go test",
	})

	if len(provider.requests) != 1 || !strings.Contains(provider.requests[0].SystemPrompt, "Use go test before claiming success.") {
		t.Fatalf("expected provider request to include installed relevant skill summary, got %+v", provider.requests)
	}
}

func TestInstallSkillAffectsFutureRequests(t *testing.T) {
	registry := NewSkillsRegistry(NewMemorySkillsStore(), testSkillCatalog())
	provider := &scriptedTextProvider{
		name:    "skills-install",
		replies: []string{"first", "second"},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithSkillsLoader(registry))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	_, _ = rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "skills-install",
		Message:  "run repository diagnostics and verify with go test",
	})
	if strings.Contains(provider.requests[0].SystemPrompt, "Use go test before claiming success.") {
		t.Fatalf("did not expect skill summary before install, got %+v", provider.requests[0])
	}

	if _, err := rt.InstallSkill(context.Background(), "go-testing"); err != nil {
		t.Fatalf("install skill through runtime: %v", err)
	}

	_, _ = rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "skills-install",
		Message:  "run repository diagnostics and verify with go test",
	})
	if len(provider.requests) != 2 || !strings.Contains(provider.requests[1].SystemPrompt, "Use go test before claiming success.") {
		t.Fatalf("expected installed skill to affect future requests, got %+v", provider.requests)
	}
}

func TestStructuredLoopCarriesSkillSummaryAcrossTurns(t *testing.T) {
	root := t.TempDir()
	registry := NewSkillsRegistry(NewMemorySkillsStore(), testSkillCatalog())
	if _, err := registry.InstallSkill(context.Background(), "workspace-inspection"); err != nil {
		t.Fatalf("install skill: %v", err)
	}

	provider := &scriptedToolAwareProvider{
		name: "skills-structured",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "list_dir",
						Arguments: map[string]any{
							"path": ".",
						},
					},
				},
			},
			{
				Content: "listed workspace",
			},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithWorkspaceRoot(root), WithMaxToolIterations(4), WithSkillsLoader(registry))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "skills-structured",
		Message:  "inspect workspace contents",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "listed workspace" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected two structured requests, got %d", len(provider.requests))
	}
	for _, req := range provider.requests {
		if !strings.Contains(req.SystemPrompt, "Prefer bounded workspace reads before edits.") {
			t.Fatalf("expected structured request to carry skills summary, got %+v", provider.requests)
		}
	}
}

func TestRuntimeCapabilitySummaryIncludesSkillsAndMCP(t *testing.T) {
	ctx := context.Background()
	registry := NewSkillsRegistry(NewMemorySkillsStore(), testSkillCatalog())
	if _, err := registry.InstallSkill(ctx, "go-testing"); err != nil {
		t.Fatalf("install skill: %v", err)
	}

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
		ServerHooks: map[string]MCPServerHooks{
			"repo": {
				Start: func(context.Context) error { return nil },
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
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("start manager: %v", err)
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithSkillsLoader(registry), WithMCPManager(manager))
	summary := rt.CapabilitySummary(ctx)

	if len(summary.Skills) != 1 || summary.Skills[0].Name != "go-testing" || !summary.Skills[0].Enabled {
		t.Fatalf("unexpected runtime skill summary: %+v", summary.Skills)
	}
	if len(summary.MCP.Servers) != 1 || summary.MCP.Servers[0].Name != "repo" {
		t.Fatalf("unexpected runtime mcp summary: %+v", summary.MCP)
	}
	if len(summary.MCP.VisibleTools) != 1 || summary.MCP.VisibleTools[0].Name != "repo_search" {
		t.Fatalf("unexpected runtime visible tools: %+v", summary.MCP.VisibleTools)
	}
}
