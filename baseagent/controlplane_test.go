package baseagent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMessageBusPublishAndConsume(t *testing.T) {
	bus := NewMessageBus(2, 2, 2)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	bus.PublishInbound(InboundEnvelope{Channel: "cli", ChatID: "c1", Content: "hello"})
	in, ok := bus.ConsumeInbound(ctx)
	if !ok {
		t.Fatal("expected inbound message")
	}
	if in.Content != "hello" {
		t.Fatalf("unexpected inbound content: %q", in.Content)
	}

	bus.PublishOutbound(OutboundEnvelope{Channel: "cli", ChatID: "c1", Content: "world"})
	out, ok := bus.ConsumeOutbound(ctx)
	if !ok {
		t.Fatal("expected outbound message")
	}
	if out.Content != "world" {
		t.Fatalf("unexpected outbound content: %q", out.Content)
	}

	bus.PublishEvent(LoopEvent{Type: EventTool, Name: "unit"})
	evt, ok := bus.ConsumeEvent(ctx)
	if !ok {
		t.Fatal("expected event")
	}
	if evt.Name != "unit" || evt.Type != EventTool {
		t.Fatalf("unexpected event: %+v", evt)
	}
}

func TestSessionManagerCompaction(t *testing.T) {
	sessions := NewSessionManager(3)
	key := "cli:default"
	sessions.AddMessage(key, "user", "one")
	sessions.AddMessage(key, "assistant", "two")
	sessions.AddMessage(key, "user", "three")
	sessions.AddMessage(key, "assistant", "four")
	sessions.AddMessage(key, "user", "five")

	history := sessions.History(key)
	if len(history) != 3 {
		t.Fatalf("expected compacted history of 3, got %d", len(history))
	}
	summary := sessions.Summary(key)
	if !strings.Contains(summary, "Compacted") {
		t.Fatalf("expected compaction summary, got %q", summary)
	}
}

func TestToolRegistryRoutesFirstMatch(t *testing.T) {
	reg := NewToolRegistry()
	_ = reg.RegisterTool(ToolSpec{
		Name:        "noop",
		Description: "noop",
		Match: func(input string) (ToolInvocation, bool) {
			return ToolInvocation{}, false
		},
		Run: func(ctx context.Context, call ToolInvocation) (ChatResponse, error) {
			return ChatResponse{Message: "noop"}, nil
		},
	})
	_ = reg.RegisterTool(ToolSpec{
		Name:        "echo",
		Description: "echo",
		Match: func(input string) (ToolInvocation, bool) {
			if strings.HasPrefix(input, "echo ") {
				return ToolInvocation{Name: "echo", Args: map[string]string{"text": strings.TrimPrefix(input, "echo ")}}, true
			}
			return ToolInvocation{}, false
		},
		Run: func(ctx context.Context, call ToolInvocation) (ChatResponse, error) {
			return ChatResponse{Message: call.Args["text"], Action: "echo"}, nil
		},
	})

	resp, handled, err := reg.RouteMessage(context.Background(), "echo hi")
	if err != nil {
		t.Fatalf("route error: %v", err)
	}
	if !handled || resp.Action != "echo" || resp.Message != "hi" {
		t.Fatalf("unexpected routed response: handled=%v resp=%+v", handled, resp)
	}
}

type providerFake struct {
	name string
	out  string
}

func (p providerFake) Name() string { return p.name }
func (p providerFake) Reply(_ context.Context, _ ProviderRequest) (string, error) {
	return p.out, nil
}

func TestProviderManagerRouting(t *testing.T) {
	pm := NewProviderManager(providerFake{name: "a", out: "A"})
	if err := pm.RegisterProvider(providerFake{name: "b", out: "B"}); err != nil {
		t.Fatalf("register provider b: %v", err)
	}
	if err := pm.SetActiveProvider("b"); err != nil {
		t.Fatalf("set active provider: %v", err)
	}
	reply, err := pm.Reply(context.Background(), ProviderRequest{UserMessage: "x"})
	if err != nil {
		t.Fatalf("provider reply error: %v", err)
	}
	if reply != "B" {
		t.Fatalf("unexpected provider reply: %q", reply)
	}
}

type providerErrFake struct {
	name string
	err  error
}

func (p providerErrFake) Name() string { return p.name }
func (p providerErrFake) Reply(_ context.Context, _ ProviderRequest) (string, error) {
	return "", p.err
}

func TestChainProviderFallback(t *testing.T) {
	chain := NewChainProvider(
		"chain",
		providerErrFake{name: "broken", err: errors.New("primary failed")},
		providerFake{name: "fallback", out: "fallback reply"},
	)
	reply, err := chain.Reply(context.Background(), ProviderRequest{UserMessage: "hi"})
	if err != nil {
		t.Fatalf("chain reply error: %v", err)
	}
	if reply != "fallback reply" {
		t.Fatalf("unexpected chain reply: %q", reply)
	}
}

func TestCallbackChannelLifecycleAndSend(t *testing.T) {
	sent := ""
	channel := NewTelegramChannel(func(_ context.Context, msg OutboundEnvelope) error {
		sent = msg.Content
		return nil
	})
	if channel.IsRunning() {
		t.Fatal("new channel should be stopped")
	}
	if err := channel.Start(context.Background()); err != nil {
		t.Fatalf("start channel: %v", err)
	}
	if !channel.IsRunning() {
		t.Fatal("channel should be running")
	}
	if err := channel.Send(context.Background(), OutboundEnvelope{Channel: "telegram", ChatID: "c1", Content: "hello"}); err != nil {
		t.Fatalf("send channel message: %v", err)
	}
	if sent != "hello" {
		t.Fatalf("unexpected callback payload: %q", sent)
	}
	if err := channel.Stop(context.Background()); err != nil {
		t.Fatalf("stop channel: %v", err)
	}
}

func TestChannelManagerDispatchPublishesSendErrorEvent(t *testing.T) {
	bus := NewMessageBus(0, 0, 0)
	manager := NewChannelManager(bus)
	if err := manager.RegisterChannel("telegram", NewTelegramChannel(func(_ context.Context, _ OutboundEnvelope) error {
		return errors.New("send failed")
	})); err != nil {
		t.Fatalf("register channel: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.StartAll(ctx); err != nil {
		t.Fatalf("start channels: %v", err)
	}
	defer manager.StopAll(context.Background())

	bus.PublishOutbound(OutboundEnvelope{Channel: "telegram", ChatID: "c1", Content: "hello"})

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	evt, ok := bus.ConsumeEvent(waitCtx)
	if !ok {
		t.Fatal("expected send failure event")
	}
	if evt.Name != "channel_send_failed" {
		t.Fatalf("event name = %q, want channel_send_failed", evt.Name)
	}
	if evt.Type != EventError {
		t.Fatalf("event type = %q, want %q", evt.Type, EventError)
	}
}

func TestRuntimeMetadataCommands(t *testing.T) {
	rt := NewRuntime(&runtimeServiceFake{}, nil)

	resp, err := rt.Chat(context.Background(), ChatRequest{Provider: "cli", ChatID: "unit", Message: "/tools"})
	if err != nil {
		t.Fatalf("tools command failed: %v", err)
	}
	if resp.Action != "tools" || !strings.Contains(resp.Message, "Available tools") {
		t.Fatalf("unexpected tools response: %+v", resp)
	}

	resp, err = rt.Chat(context.Background(), ChatRequest{Provider: "cli", ChatID: "unit", Message: "/providers"})
	if err != nil {
		t.Fatalf("providers command failed: %v", err)
	}
	if resp.Action != "providers" || !strings.Contains(resp.Message, "Active provider") {
		t.Fatalf("unexpected providers response: %+v", resp)
	}

	resp, err = rt.Chat(context.Background(), ChatRequest{Provider: "cli", ChatID: "unit", Message: "/sessions"})
	if err != nil {
		t.Fatalf("sessions command failed: %v", err)
	}
	if resp.Action != "sessions" || !strings.Contains(resp.Message, "Recent sessions") {
		t.Fatalf("unexpected sessions response: %+v", resp)
	}

	resp, err = rt.Chat(context.Background(), ChatRequest{Provider: "cli", ChatID: "unit", Message: "/boundaries"})
	if err != nil {
		t.Fatalf("boundaries command failed: %v", err)
	}
	if resp.Action != "boundaries" || !strings.Contains(resp.Message, "BaseAgent boundaries") {
		t.Fatalf("unexpected boundaries response: %+v", resp)
	}
}
