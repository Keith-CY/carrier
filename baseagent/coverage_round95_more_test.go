package baseagent

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type listOnlyService struct {
	agents []AgentState
}

func (s *listOnlyService) ListAgents() []AgentState { return s.agents }
func (s *listOnlyService) Install(_ context.Context, _ string) error {
	return nil
}
func (s *listOnlyService) Uninstall(_ context.Context, _ string) error {
	return nil
}
func (s *listOnlyService) Start(_ context.Context, _ string) error {
	return nil
}
func (s *listOnlyService) Stop(_ context.Context, _ string) error {
	return nil
}
func (s *listOnlyService) Status(agentID string) (AgentState, error) {
	return AgentState{ID: agentID}, nil
}
func (s *listOnlyService) Logs(_ string, _ int) ([]string, error) {
	return nil, nil
}
func (s *listOnlyService) Upgrade(_ context.Context, agentID string) (UpgradeResult, error) {
	return UpgradeResult{AgentID: agentID}, nil
}
func (s *listOnlyService) Diagnose(agentID string) (string, error) {
	return agentID, nil
}

func TestNewAgentLoopAppliesDefaultDependencies(t *testing.T) {
	loop := NewAgentLoop(nil, nil, nil, nil, nil)
	if loop.tools == nil || loop.providers == nil || loop.sessions == nil || loop.bus == nil {
		t.Fatalf("expected defaults to be initialized: %+v", loop)
	}
}

func TestAgentLoopBestEffortAgentStatusBranches(t *testing.T) {
	loop := NewAgentLoop(nil, NewToolRegistry(), NewProviderManager(nil), NewSessionManager(8), NewMessageBus(8, 8, 8))
	if _, ok := loop.bestEffortAgentStatus(context.Background(), "status openclaw"); ok {
		t.Fatal("expected nil service to skip best-effort status")
	}

	regErr := NewToolRegistry()
	_ = regErr.RegisterTool(ToolSpec{
		Name: "agent_action",
		Match: func(string) (ToolInvocation, bool) {
			return ToolInvocation{}, false
		},
		Run: func(context.Context, ToolInvocation) (ChatResponse, error) {
			return ChatResponse{}, errors.New("boom")
		},
	})
	loopErr := NewAgentLoop(
		&listOnlyService{agents: []AgentState{{ID: "openclaw"}}},
		regErr,
		NewProviderManager(NewStaticProvider("default", "ok")),
		NewSessionManager(8),
		NewMessageBus(8, 8, 8),
	)
	if _, ok := loopErr.bestEffortAgentStatus(context.Background(), "show openclaw status"); ok {
		t.Fatal("expected tool execution error branch to return not handled")
	}

	regOK := NewToolRegistry()
	_ = regOK.RegisterTool(ToolSpec{
		Name: "agent_action",
		Match: func(string) (ToolInvocation, bool) {
			return ToolInvocation{}, false
		},
		Run: func(_ context.Context, call ToolInvocation) (ChatResponse, error) {
			if call.Args["agent_id"] != "openclaw" || call.Args["action"] != "status" {
				t.Fatalf("unexpected tool args: %+v", call.Args)
			}
			return ChatResponse{Message: "running"}, nil
		},
	})
	loopOK := NewAgentLoop(
		&listOnlyService{agents: []AgentState{{ID: "openclaw"}}},
		regOK,
		NewProviderManager(NewStaticProvider("default", "ok")),
		NewSessionManager(8),
		NewMessageBus(8, 8, 8),
	)
	resp, ok := loopOK.bestEffortAgentStatus(context.Background(), "OPENCLAW now")
	if !ok {
		t.Fatal("expected known agent mention to trigger best-effort status")
	}
	if resp.Action != "status" {
		t.Fatalf("expected default status action, got %q", resp.Action)
	}
}

func TestFinalizeResponseOutboundAndInternalChannelBranches(t *testing.T) {
	bus := NewMessageBus(8, 8, 8)
	sessions := NewSessionManager(8)
	loop := NewAgentLoop(nil, NewToolRegistry(), NewProviderManager(NewStaticProvider("default", "ok")), sessions, bus)

	resp := loop.finalizeResponse("telegram:chat1", "telegram", "chat1", "req-1", ChatResponse{
		Message: "  ",
		Action:  " chat ",
	})
	if resp.Message != "Done." {
		t.Fatalf("expected default fallback message, got %q", resp.Message)
	}

	out, ok := bus.ConsumeOutbound(context.Background())
	if !ok {
		t.Fatal("expected outbound message for non-internal channel")
	}
	if out.Content != "Done." || out.Metadata["request_id"] != "req-1" {
		t.Fatalf("unexpected outbound envelope: %+v", out)
	}

	evt, ok := bus.ConsumeEvent(context.Background())
	if !ok || evt.Name != "chat_response" || !strings.Contains(evt.Message, "action=chat") {
		t.Fatalf("unexpected event: ok=%v evt=%+v", ok, evt)
	}

	_ = loop.finalizeResponse("cli:chat2", "cli", "chat2", "req-2", ChatResponse{
		Message: "already set",
		Action:  "status",
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := bus.ConsumeOutbound(ctx); ok {
		t.Fatal("expected no outbound for internal channel")
	}
}

func TestMessageBusConsumeCanceledContextAndCloseBranches(t *testing.T) {
	bus := NewMessageBus(2, 2, 2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, ok := bus.ConsumeInbound(ctx); ok {
		t.Fatal("expected canceled inbound consume to return false")
	}
	if _, ok := bus.ConsumeOutbound(ctx); ok {
		t.Fatal("expected canceled outbound consume to return false")
	}
	if _, ok := bus.ConsumeEvent(ctx); ok {
		t.Fatal("expected canceled event consume to return false")
	}

	bus.Close()
	bus.Close()
	bus.PublishInbound(InboundEnvelope{Content: "drop"})
	bus.PublishOutbound(OutboundEnvelope{Content: "drop"})
	bus.PublishEvent(LoopEvent{Name: "drop"})
}

func TestProviderManagerAndProvidersErrorBranches(t *testing.T) {
	var nilPM *ProviderManager
	if err := nilPM.RegisterProvider(providerFake{name: "x", out: "x"}); err == nil {
		t.Fatal("expected nil manager register to fail")
	}
	if err := nilPM.SetActiveProvider("x"); err == nil {
		t.Fatal("expected nil manager set active to fail")
	}
	if _, err := nilPM.Reply(context.Background(), ProviderRequest{UserMessage: "hi"}); err == nil {
		t.Fatal("expected nil manager reply to fail")
	}
	if names := nilPM.ListProviders(); names != nil {
		t.Fatalf("expected nil manager list to be nil, got %+v", names)
	}
	if active := nilPM.ActiveProviderName(); active != "" {
		t.Fatalf("expected empty active provider for nil manager, got %q", active)
	}

	pm := NewProviderManager(nil)
	if _, err := pm.Reply(context.Background(), ProviderRequest{UserMessage: "hi"}); err == nil {
		t.Fatal("expected no-active-provider error")
	}
	if err := pm.RegisterProvider(nil); err == nil {
		t.Fatal("expected nil provider registration error")
	}
	if err := pm.RegisterProvider(providerFake{name: "", out: "x"}); err == nil {
		t.Fatal("expected empty-name provider registration error")
	}
	if err := pm.SetActiveProvider("missing"); err == nil {
		t.Fatal("expected missing provider set-active error")
	}

	chain := NewChainProvider("", nil)
	if chain.Name() != "chain" {
		t.Fatalf("expected default chain name, got %q", chain.Name())
	}
	if _, err := chain.Reply(context.Background(), ProviderRequest{UserMessage: "hi"}); err == nil {
		t.Fatal("expected empty chain to fail")
	}

	chainNoContent := NewChainProvider("x", providerFake{name: "blank", out: "   "})
	if _, err := chainNoContent.Reply(context.Background(), ProviderRequest{UserMessage: "hi"}); err == nil {
		t.Fatal("expected no-content chain error")
	}

	sp := NewStaticProvider("", "  ")
	if sp.Name() != "static" {
		t.Fatalf("expected default static provider name, got %q", sp.Name())
	}
	if _, err := sp.Reply(context.Background(), ProviderRequest{}); err == nil {
		t.Fatal("expected empty static provider message error")
	}
}
