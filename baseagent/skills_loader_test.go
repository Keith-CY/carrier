package baseagent

import (
	"context"
	"strings"
	"testing"
)

type fakeSkillsLoader struct {
	summary string
	calls   []string
}

func (f *fakeSkillsLoader) RelevantSkillsSummary(_ context.Context, message string) string {
	f.calls = append(f.calls, message)
	return f.summary
}

func (f *fakeSkillsLoader) WasConsulted() bool {
	return len(f.calls) > 0
}

func TestProviderRequestIncludesRelevantSkillsSummary(t *testing.T) {
	loader := &fakeSkillsLoader{summary: "Use go test before claiming success."}
	provider := &scriptedTextProvider{
		name:    "skills-text",
		replies: []string{"done"},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithSkillsLoader(loader))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	_, _ = rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "skills",
		Message:  "run repository diagnostics",
	})

	if !loader.WasConsulted() {
		t.Fatal("expected skills loader to contribute provider context")
	}
	if len(provider.requests) != 1 || !strings.Contains(provider.requests[0].SystemPrompt, loader.summary) {
		t.Fatalf("expected provider request to include skills summary, got %+v", provider.requests)
	}
}

func TestStructuredLoopCarriesSkillSummaryAcrossTurns(t *testing.T) {
	root := t.TempDir()
	loader := &fakeSkillsLoader{summary: "Prefer bounded workspace reads before edits."}
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

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithWorkspaceRoot(root), WithMaxToolIterations(4), WithSkillsLoader(loader))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "skills-structured",
		Message:  "inspect the workspace",
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
		if !strings.Contains(req.SystemPrompt, loader.summary) {
			t.Fatalf("expected structured request to carry skills summary, got %+v", provider.requests)
		}
	}
}
