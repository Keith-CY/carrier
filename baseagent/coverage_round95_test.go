package baseagent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBoundaryFallbackAndBudgetHelpers(t *testing.T) {
	fallback := fallbackBoundarySpec()
	if strings.TrimSpace(fallback.SchemaVersion) == "" {
		t.Fatalf("expected fallback schema version")
	}
	if fallback.RepairRoundBudget() <= 0 {
		t.Fatalf("expected positive repair round budget")
	}
	if InstallAutoRepairRoundBudget() <= 0 {
		t.Fatalf("expected positive install auto-repair budget")
	}
	if !strings.Contains(fallback.RenderSummary(), "BaseAgent boundaries") {
		t.Fatalf("expected boundary summary header")
	}
}

func TestMessageBusCloseAndChannelManagerRegistryPaths(t *testing.T) {
	bus := NewMessageBus(1, 1, 1)
	bus.PublishInbound(InboundEnvelope{Content: "x"})
	bus.PublishOutbound(OutboundEnvelope{Content: "y"})
	bus.PublishEvent(LoopEvent{Name: "z"})
	bus.Close()
	bus.Close()

	manager := NewChannelManager(bus)
	telegram := NewTelegramChannel(func(_ context.Context, _ OutboundEnvelope) error { return nil })
	discord := NewDiscordChannel(func(_ context.Context, _ OutboundEnvelope) error { return nil })
	feishu := NewFeishuChannel(func(_ context.Context, _ OutboundEnvelope) error { return nil })

	if telegram.Name() != "telegram" || discord.Name() != "discord" || feishu.Name() != "feishu" {
		t.Fatalf("unexpected channel names: tg=%q dc=%q fs=%q", telegram.Name(), discord.Name(), feishu.Name())
	}
	if err := manager.RegisterChannel("telegram", telegram); err != nil {
		t.Fatalf("register telegram: %v", err)
	}
	if err := manager.RegisterChannel("discord", discord); err != nil {
		t.Fatalf("register discord: %v", err)
	}
	names := manager.ListChannels()
	if len(names) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(names))
	}
	manager.UnregisterChannel("discord")
	names = manager.ListChannels()
	if len(names) != 1 || names[0] != "telegram" {
		t.Fatalf("unexpected channels after unregister: %v", names)
	}
}

func TestStaticProviderAndSessionManagerHelpers(t *testing.T) {
	static := NewStaticProvider("static-provider", "hello")
	if static.Name() != "static-provider" {
		t.Fatalf("unexpected static provider name: %q", static.Name())
	}
	reply, err := static.Reply(context.Background(), ProviderRequest{})
	if err != nil || reply != "hello" {
		t.Fatalf("unexpected static provider reply=%q err=%v", reply, err)
	}
	empty := NewStaticProvider("empty", "")
	if _, err := empty.Reply(context.Background(), ProviderRequest{}); err == nil {
		t.Fatalf("expected error for empty static provider reply")
	}

	sessions := NewSessionManager(4)
	sessions.SetSummary("k1", strings.Repeat("a", maxSessionSummaryBytes+100))
	if got := sessions.Summary("k1"); !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated summary suffix, got %q", got)
	}
	stats := sessions.ListStats(1)
	if len(stats) != 1 || stats[0].Key != "k1" {
		t.Fatalf("unexpected stats output: %#v", stats)
	}

	reg := NewToolRegistry()
	_ = reg.RegisterTool(ToolSpec{
		Name:  "zeta",
		Match: func(string) (ToolInvocation, bool) { return ToolInvocation{}, false },
		Run:   func(context.Context, ToolInvocation) (ChatResponse, error) { return ChatResponse{}, nil },
	})
	_ = reg.RegisterTool(ToolSpec{
		Name:  "alpha",
		Match: func(string) (ToolInvocation, bool) { return ToolInvocation{}, false },
		Run:   func(context.Context, ToolInvocation) (ChatResponse, error) { return ChatResponse{}, nil },
	})
	if got := reg.ListToolNames(); len(got) != 2 {
		t.Fatalf("expected list tool names length 2, got %d", len(got))
	}
	sorted := reg.SortedToolNames()
	if len(sorted) != 2 || sorted[0] != "alpha" || sorted[1] != "zeta" {
		t.Fatalf("unexpected sorted tool names: %v", sorted)
	}
}

func TestRequestCompletionWrapper(t *testing.T) {
	server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	writeDefaultModelConfig(t, "openai", "openai/gpt-5.3", "OPENAI_API_KEY")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("CARRIER_OPENAI_BASE_URL", server.URL)

	got, err := RequestCompletion(context.Background(), "system", "hello")
	if err != nil {
		t.Fatalf("RequestCompletion error: %v", err)
	}
	if strings.TrimSpace(got) != "ok" {
		t.Fatalf("unexpected RequestCompletion output: %q", got)
	}
}

func TestLLMProviderNameAndHistoryRenderingBranches(t *testing.T) {
	provider := NewLLMProviderAdapter("", 0)
	if provider.Name() != "llm" {
		t.Fatalf("expected default llm provider name, got %q", provider.Name())
	}
	history := []ConversationMessage{
		{Role: "user", Content: strings.Repeat("x", maxProviderHistoryMessageLength+20), Timestamp: time.Now().UTC()},
	}
	rendered := renderProviderHistory(history, 1)
	if !strings.Contains(rendered, "user:") || !strings.Contains(rendered, "...") {
		t.Fatalf("unexpected rendered history: %q", rendered)
	}
}

func TestLLMProviderAdapterIncludesStructuredToolSchemasInPrompt(t *testing.T) {
	var gotUserPrompt string

	server := newLocalhostServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		if len(payload.Messages) < 2 {
			t.Fatalf("expected at least 2 messages, got %+v", payload.Messages)
		}
		gotUserPrompt = payload.Messages[1].Content
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	writeDefaultModelConfig(t, "openai", "openai/gpt-5.3", "OPENAI_API_KEY")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("CARRIER_OPENAI_BASE_URL", server.URL)

	provider := NewLLMProviderAdapter("openai", 0)
	_, err := provider.Reply(context.Background(), ProviderRequest{
		SystemPrompt: "system",
		UserMessage:  "hello",
		StructuredTools: []StructuredToolDescriptor{
			{
				Name:        "agent_action",
				Description: "Run an agent lifecycle action.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"agent_id": map[string]any{"type": "string"},
						"action":   map[string]any{"type": "string"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("provider reply failed: %v", err)
	}
	if !strings.Contains(gotUserPrompt, "Available built-in tools") {
		t.Fatalf("expected built-in tools section, got %q", gotUserPrompt)
	}
	if !strings.Contains(gotUserPrompt, "agent_action") || !strings.Contains(gotUserPrompt, `parameters={"properties":{"action":{"type":"string"},"agent_id":{"type":"string"}},"type":"object"}`) {
		t.Fatalf("expected structured tool schema in prompt, got %q", gotUserPrompt)
	}
}
