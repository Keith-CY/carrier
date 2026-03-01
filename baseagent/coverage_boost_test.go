package baseagent

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ---------- AgentLoop ----------

func TestNewAgentLoopAllNils(t *testing.T) {
	loop := NewAgentLoop(nil, nil, nil, nil, nil)
	if loop.tools == nil || loop.providers == nil || loop.sessions == nil || loop.bus == nil {
		t.Fatal("expected all defaults to be set")
	}
}

// ---------- Channel lifecycle ----------

func TestNewDiscordChannel(t *testing.T) {
	ch := NewDiscordChannel(func(_ context.Context, _ OutboundEnvelope) error { return nil })
	if ch.Name() != "discord" {
		t.Fatalf("expected 'discord', got %q", ch.Name())
	}
}

func TestNewFeishuChannel(t *testing.T) {
	ch := NewFeishuChannel(func(_ context.Context, _ OutboundEnvelope) error { return nil })
	if ch.Name() != "feishu" {
		t.Fatalf("expected 'feishu', got %q", ch.Name())
	}
}

func TestCallbackChannelSendNotRunning(t *testing.T) {
	ch := NewCallbackChannel("test", func(_ context.Context, _ OutboundEnvelope) error { return nil })
	err := ch.Send(context.Background(), OutboundEnvelope{})
	if err == nil {
		t.Fatal("expected error when not running")
	}
}

// ---------- ChannelManager ----------

func TestChannelManagerUnregister(t *testing.T) {
	bus := NewMessageBus(1, 1, 1)
	cm := NewChannelManager(bus)
	ch := NewCallbackChannel("test", func(_ context.Context, _ OutboundEnvelope) error { return nil })
	cm.RegisterChannel("test", ch)
	if _, ok := cm.GetChannel("test"); !ok {
		t.Fatal("expected channel to be registered")
	}
	cm.UnregisterChannel("test")
	if _, ok := cm.GetChannel("test"); ok {
		t.Fatal("expected channel to be unregistered")
	}
}

func TestChannelManagerUnregisterEmpty(t *testing.T) {
	bus := NewMessageBus(1, 1, 1)
	cm := NewChannelManager(bus)
	cm.UnregisterChannel("") // should not panic
}

func TestChannelManagerListChannels(t *testing.T) {
	bus := NewMessageBus(1, 1, 1)
	cm := NewChannelManager(bus)
	ch1 := NewCallbackChannel("alpha", func(_ context.Context, _ OutboundEnvelope) error { return nil })
	ch2 := NewCallbackChannel("beta", func(_ context.Context, _ OutboundEnvelope) error { return nil })
	cm.RegisterChannel("alpha", ch1)
	cm.RegisterChannel("beta", ch2)
	list := cm.ListChannels()
	if len(list) != 2 || list[0] != "alpha" || list[1] != "beta" {
		t.Fatalf("expected [alpha beta], got %v", list)
	}
}

// ---------- MessageBus Close ----------

func TestMessageBusClose(t *testing.T) {
	bus := NewMessageBus(1, 1, 1)
	bus.Close()
	// Publishing after close should not panic
	bus.PublishInbound(InboundEnvelope{Content: "test"})
}

// ---------- StaticProvider ----------

func TestStaticProvider(t *testing.T) {
	sp := NewStaticProvider("static-test", "Hello, World!")
	if sp.Name() != "static-test" {
		t.Fatalf("expected 'static-test', got %q", sp.Name())
	}
	resp, err := sp.Reply(context.Background(), ProviderRequest{UserMessage: "anything"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "Hello, World!" {
		t.Fatalf("expected 'Hello, World!', got %q", resp)
	}
}

func TestStaticProviderEmpty(t *testing.T) {
	sp := NewStaticProvider("empty", "")
	_, err := sp.Reply(context.Background(), ProviderRequest{})
	if err == nil {
		t.Fatal("expected error for empty static provider")
	}
}

func TestStaticProviderDefaultName(t *testing.T) {
	sp := NewStaticProvider("", "hello")
	if sp.Name() != "static" {
		t.Fatalf("expected 'static', got %q", sp.Name())
	}
}

// ---------- ChainProvider ----------

func TestChainProviderName(t *testing.T) {
	p := NewChainProvider("chain", nil)
	if p.Name() != "chain" {
		t.Fatalf("expected 'chain', got %q", p.Name())
	}
}

func TestChainProviderReplyFallthrough(t *testing.T) {
	failProvider := &mockProvider{err: errors.New("fail")}
	successProvider := &mockProvider{response: "success"}

	p := NewChainProvider("chain", failProvider, successProvider)
	resp, err := p.Reply(context.Background(), ProviderRequest{UserMessage: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "success" {
		t.Fatalf("expected 'success', got %q", resp)
	}
}

func TestChainProviderReplyAllFail(t *testing.T) {
	p1 := &mockProvider{err: errors.New("fail1")}
	p2 := &mockProvider{err: errors.New("fail2")}

	p := NewChainProvider("chain", p1, p2)
	_, err := p.Reply(context.Background(), ProviderRequest{UserMessage: "test"})
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestChainProviderReplyEmpty(t *testing.T) {
	p := NewChainProvider("chain")
	_, err := p.Reply(context.Background(), ProviderRequest{UserMessage: "test"})
	if err == nil {
		t.Fatal("expected error for empty chain")
	}
}

type mockProvider struct {
	response string
	err      error
}

func (p *mockProvider) Name() string { return "mock" }
func (p *mockProvider) Reply(_ context.Context, _ ProviderRequest) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	return p.response, nil
}

// ---------- SessionManager ----------

func TestSessionManagerSetSummary(t *testing.T) {
	sm := NewSessionManager(10)
	sm.AddMessage("test-key", "user", "hello")
	sm.SetSummary("test-key", "Custom summary")
	summary := sm.Summary("test-key")
	if summary != "Custom summary" {
		t.Fatalf("expected 'Custom summary', got %q", summary)
	}
}

func TestSessionManagerListStats(t *testing.T) {
	sm := NewSessionManager(10)
	sm.AddMessage("key1", "user", "msg1")
	sm.AddMessage("key2", "user", "msg2")
	stats := sm.ListStats(10)
	if len(stats) != 2 {
		t.Fatalf("expected 2 stats, got %d", len(stats))
	}
}

func TestTruncateSummary(t *testing.T) {
	short := "short summary"
	if got := truncateSummary(short); got != short {
		t.Fatalf("expected unchanged, got %q", got)
	}
}

// ---------- ToolRegistry ----------

func TestToolRegistryListToolNames(t *testing.T) {
	reg := NewToolRegistry()
	_ = reg.RegisterTool(ToolSpec{
		Name:        "alpha",
		Description: "alpha tool",
		Match:       func(_ string) (ToolInvocation, bool) { return ToolInvocation{}, false },
		Run:         func(_ context.Context, _ ToolInvocation) (ChatResponse, error) { return ChatResponse{}, nil },
	})
	_ = reg.RegisterTool(ToolSpec{
		Name:        "beta",
		Description: "beta tool",
		Match:       func(_ string) (ToolInvocation, bool) { return ToolInvocation{}, false },
		Run:         func(_ context.Context, _ ToolInvocation) (ChatResponse, error) { return ChatResponse{}, nil },
	})
	names := reg.ListToolNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
}

func TestToolRegistrySortedToolNames(t *testing.T) {
	reg := NewToolRegistry()
	_ = reg.RegisterTool(ToolSpec{
		Name:        "zeta",
		Description: "z",
		Match:       func(_ string) (ToolInvocation, bool) { return ToolInvocation{}, false },
		Run:         func(_ context.Context, _ ToolInvocation) (ChatResponse, error) { return ChatResponse{}, nil },
	})
	_ = reg.RegisterTool(ToolSpec{
		Name:        "alpha",
		Description: "a",
		Match:       func(_ string) (ToolInvocation, bool) { return ToolInvocation{}, false },
		Run:         func(_ context.Context, _ ToolInvocation) (ChatResponse, error) { return ChatResponse{}, nil },
	})
	sorted := reg.SortedToolNames()
	if sorted[0] != "alpha" || sorted[1] != "zeta" {
		t.Fatalf("expected sorted order, got %v", sorted)
	}
}

// ---------- mustRegisterTool ----------

func TestMustRegisterToolPanicsOnNilRegistry(t *testing.T) {
	spec := ToolSpec{
		Name:        "test",
		Description: "test",
		Match:       func(_ string) (ToolInvocation, bool) { return ToolInvocation{}, false },
		Run:         func(_ context.Context, _ ToolInvocation) (ChatResponse, error) { return ChatResponse{}, nil },
	}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on nil registry")
		}
	}()
	mustRegisterTool(nil, spec)
}

// ---------- BoundarySpec ----------

func TestFallbackBoundarySpec(t *testing.T) {
	spec := fallbackBoundarySpec()
	if spec.SchemaVersion == "" || spec.AssistantRole == "" {
		t.Fatalf("expected non-zero fallback spec, got %+v", spec)
	}
}

func TestInstallAutoRepairRoundBudget(t *testing.T) {
	budget := InstallAutoRepairRoundBudget()
	if budget <= 0 {
		t.Fatalf("expected positive budget, got %d", budget)
	}
}

func TestBoundarySpecRepairRoundBudget(t *testing.T) {
	spec := BoundarySpec{}
	budget := spec.RepairRoundBudget()
	if budget != defaultInstallAutoRepairRoundBudget {
		t.Fatalf("expected default budget %d, got %d", defaultInstallAutoRepairRoundBudget, budget)
	}
}

func TestBoundarySpecRepairRoundBudgetCustom(t *testing.T) {
	spec := BoundarySpec{
		RepairPolicy: RepairPolicy{MaxAutoRepairRounds: 7},
	}
	if spec.RepairRoundBudget() != 7 {
		t.Fatalf("expected 7, got %d", spec.RepairRoundBudget())
	}
}

func TestValidateBoundarySpecEmpty(t *testing.T) {
	spec := BoundarySpec{}
	err := ValidateBoundarySpec(spec)
	if err == nil {
		t.Fatal("expected error for empty boundary spec")
	}
}

func TestValidateBoundarySpecMissingRole(t *testing.T) {
	spec := BoundarySpec{SchemaVersion: boundarySpecSchemaV1}
	err := ValidateBoundarySpec(spec)
	if err == nil {
		t.Fatal("expected error for missing role")
	}
}

func TestValidateBoundarySpecMissingInScope(t *testing.T) {
	spec := BoundarySpec{
		SchemaVersion: boundarySpecSchemaV1,
		AssistantRole: "test",
	}
	err := ValidateBoundarySpec(spec)
	if err == nil {
		t.Fatal("expected error for empty in_scope")
	}
}

// ---------- prefixLines ----------

func TestPrefixLinesNil(t *testing.T) {
	result := prefixLines(nil)
	if len(result) != 1 || result[0] != "- (none)" {
		t.Fatalf("expected ['- (none)'], got %v", result)
	}
}

func TestPrefixLinesAllEmpty(t *testing.T) {
	result := prefixLines([]string{"", "  "})
	if len(result) != 1 || result[0] != "- (none)" {
		t.Fatalf("expected ['- (none)'], got %v", result)
	}
}

func TestPrefixLinesSingle(t *testing.T) {
	result := prefixLines([]string{"hello"})
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d", len(result))
	}
}

func TestPrefixLinesMultiple(t *testing.T) {
	result := prefixLines([]string{"a", "b", "c"})
	if len(result) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(result))
	}
}

// ---------- isRetryableModelRequestError ----------

func TestIsRetryableModelRequestErrorNil(t *testing.T) {
	if isRetryableModelRequestError(nil) {
		t.Fatal("expected false for nil error")
	}
}

func TestIsRetryableModelRequestErrorCanceled(t *testing.T) {
	if isRetryableModelRequestError(context.Canceled) {
		t.Fatal("expected false for context.Canceled")
	}
	if isRetryableModelRequestError(context.DeadlineExceeded) {
		t.Fatal("expected false for DeadlineExceeded")
	}
}

func TestIsRetryableModelRequestErrorRegular(t *testing.T) {
	if isRetryableModelRequestError(errors.New("something broke")) {
		t.Fatal("expected false for regular error")
	}
}

// ---------- MessageBus full channels ----------

func TestMessageBusInboundFull(t *testing.T) {
	bus := NewMessageBus(1, 1, 1)
	bus.PublishInbound(InboundEnvelope{Content: "first"})
	bus.PublishInbound(InboundEnvelope{Content: "second"}) // should drop or succeed without panic
}

func TestMessageBusOutboundFull(t *testing.T) {
	bus := NewMessageBus(1, 1, 1)
	bus.PublishOutbound(OutboundEnvelope{Content: "first"})
	bus.PublishOutbound(OutboundEnvelope{Content: "second"})
}

func TestMessageBusEventFull(t *testing.T) {
	bus := NewMessageBus(1, 1, 1)
	bus.PublishEvent(LoopEvent{Name: "first"})
	bus.PublishEvent(LoopEvent{Name: "second"})
}

func TestMessageBusConsumeTimeout(t *testing.T) {
	bus := NewMessageBus(1, 1, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, ok := bus.ConsumeInbound(ctx)
	if ok {
		t.Fatal("expected consume to return not-ok on timeout")
	}
	_, ok = bus.ConsumeEvent(ctx)
	if ok {
		t.Fatal("expected event consume to return not-ok on timeout")
	}
}

// ---------- ProviderManager ----------

func TestProviderManagerActiveProviderEmpty(t *testing.T) {
	pm := NewProviderManager(nil)
	name := pm.ActiveProviderName()
	if name != "" {
		t.Fatalf("expected empty provider name, got %q", name)
	}
}

func TestProviderManagerSetActive(t *testing.T) {
	pm := NewProviderManager(nil)
	sp := NewStaticProvider("test-p", "hello")
	pm.RegisterProvider(sp)
	pm.SetActiveProvider("test-p")
	if pm.ActiveProviderName() != "test-p" {
		t.Fatalf("expected 'test-p', got %q", pm.ActiveProviderName())
	}
}

func TestProviderManagerReplyNoActive(t *testing.T) {
	pm := NewProviderManager(nil)
	_, err := pm.Reply(context.Background(), ProviderRequest{UserMessage: "test"})
	if err == nil {
		t.Fatal("expected error with no active provider")
	}
}

func TestProviderManagerListProviders(t *testing.T) {
	pm := NewProviderManager(nil)
	pm.RegisterProvider(NewStaticProvider("p1", "a"))
	pm.RegisterProvider(NewStaticProvider("p2", "b"))
	list := pm.ListProviders()
	if len(list) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(list))
	}
}

// ---------- NewChannelManager nil bus ----------

func TestNewChannelManagerNilBus(t *testing.T) {
	cm := NewChannelManager(nil)
	if cm.bus == nil {
		t.Fatal("expected default bus to be created")
	}
}

// ---------- LLMProviderAdapter nil provider ----------

func TestNewLLMProviderAdapterZeroHistory(t *testing.T) {
	adapter := NewLLMProviderAdapter("test", 0)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
}

func TestNewLLMProviderAdapterNegativeHistory(t *testing.T) {
	adapter := NewLLMProviderAdapter("test", -1)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
}
