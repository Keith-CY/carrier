package baseagent

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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

func TestToolRegistryListStructuredToolDescriptorsIncludesSchemaInOrder(t *testing.T) {
	reg := NewToolRegistry()
	alphaSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"target": map[string]any{"type": "string"},
		},
		"required": []string{"target"},
	}
	_ = reg.RegisterTool(ToolSpec{
		Name:        "alpha",
		Description: "first",
		Parameters:  alphaSchema,
		Match:       func(string) (ToolInvocation, bool) { return ToolInvocation{}, false },
		Run:         func(context.Context, ToolInvocation) (ChatResponse, error) { return ChatResponse{}, nil },
	})
	_ = reg.RegisterTool(ToolSpec{
		Name:        "beta",
		Description: "second",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"count": map[string]any{"type": "integer"},
			},
		},
		Match: func(string) (ToolInvocation, bool) { return ToolInvocation{}, false },
		Run:   func(context.Context, ToolInvocation) (ChatResponse, error) { return ChatResponse{}, nil },
	})

	descriptors := reg.ListStructuredToolDescriptors()
	if len(descriptors) != 2 {
		t.Fatalf("expected 2 structured descriptors, got %d", len(descriptors))
	}
	if descriptors[0].Name != "alpha" || descriptors[1].Name != "beta" {
		t.Fatalf("expected registration order to be preserved, got %+v", descriptors)
	}
	if !reflect.DeepEqual(descriptors[0].Parameters, alphaSchema) {
		t.Fatalf("unexpected schema export for alpha: %+v", descriptors[0].Parameters)
	}
}

func TestToolRegistryListStructuredToolDescriptorsReturnsDeepCopy(t *testing.T) {
	reg := NewToolRegistry()
	_ = reg.RegisterTool(ToolSpec{
		Name:        "alpha",
		Description: "first",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{"type": "string"},
			},
		},
		Match: func(string) (ToolInvocation, bool) { return ToolInvocation{}, false },
		Run:   func(context.Context, ToolInvocation) (ChatResponse, error) { return ChatResponse{}, nil },
	})

	exported := reg.ListStructuredToolDescriptors()
	exported[0].Parameters["type"] = "mutated"
	exportedProps := exported[0].Parameters["properties"].(map[string]any)
	exportedProps["target"] = map[string]any{"type": "integer"}

	fresh := reg.ListStructuredToolDescriptors()
	if fresh[0].Parameters["type"] != "object" {
		t.Fatalf("expected registry schema to remain unchanged, got %+v", fresh[0].Parameters)
	}
	props := fresh[0].Parameters["properties"].(map[string]any)
	target := props["target"].(map[string]any)
	if target["type"] != "string" {
		t.Fatalf("expected nested schema to remain unchanged, got %+v", fresh[0].Parameters)
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

type capturingProviderFake struct {
	name     string
	out      string
	requests []ProviderRequest
}

func (p *capturingProviderFake) Name() string { return p.name }

func (p *capturingProviderFake) Reply(_ context.Context, req ProviderRequest) (string, error) {
	p.requests = append(p.requests, req)
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

func TestAgentLoopPassesStructuredToolSchemasToProvider(t *testing.T) {
	reg := NewToolRegistry()
	if err := reg.RegisterTool(ToolSpec{
		Name:        "custom_tool",
		Description: "custom tool",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{"type": "string"},
			},
		},
		Match: func(string) (ToolInvocation, bool) { return ToolInvocation{}, false },
		Run:   func(context.Context, ToolInvocation) (ChatResponse, error) { return ChatResponse{}, nil },
	}); err != nil {
		t.Fatalf("register custom tool: %v", err)
	}

	provider := &capturingProviderFake{name: "capture", out: "provider reply"}
	loop := NewAgentLoop(nil, reg, NewProviderManager(provider), NewSessionManager(8), NewMessageBus(8, 8, 8))

	resp, err := loop.ProcessChat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "unit",
		Message:  "hello",
	})
	if err != nil {
		t.Fatalf("ProcessChat failed: %v", err)
	}
	if resp.Message != "provider reply" {
		t.Fatalf("unexpected provider reply: %+v", resp)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("expected 1 provider request, got %d", len(provider.requests))
	}
	if len(provider.requests[0].StructuredTools) != 1 {
		t.Fatalf("expected structured tool descriptors to be passed through, got %+v", provider.requests[0].StructuredTools)
	}
	if provider.requests[0].StructuredTools[0].Name != "custom_tool" {
		t.Fatalf("unexpected structured tool descriptor: %+v", provider.requests[0].StructuredTools[0])
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

type providerToolAwareFake struct {
	name       string
	reply      StructuredToolReply
	replyCalls int
	textCalls  int
}

func (p *providerToolAwareFake) Name() string { return p.name }

func (p *providerToolAwareFake) Reply(_ context.Context, _ ProviderRequest) (string, error) {
	p.textCalls++
	return "", nil
}

func (p *providerToolAwareFake) ReplyWithTools(_ context.Context, _ StructuredToolRequest) (StructuredToolReply, error) {
	p.replyCalls++
	return p.reply, nil
}

type providerToolAwareSequenceFake struct {
	name       string
	results    []providerToolAwareSequenceResult
	replyCalls int
	textCalls  int
}

type providerToolAwareSequenceResult struct {
	reply StructuredToolReply
	err   error
}

func (p *providerToolAwareSequenceFake) Name() string { return p.name }

func (p *providerToolAwareSequenceFake) Reply(_ context.Context, _ ProviderRequest) (string, error) {
	p.textCalls++
	return "", fmt.Errorf("unexpected text reply for %s", p.name)
}

func (p *providerToolAwareSequenceFake) ReplyWithTools(_ context.Context, _ StructuredToolRequest) (StructuredToolReply, error) {
	idx := p.replyCalls
	p.replyCalls++
	if idx >= len(p.results) {
		return StructuredToolReply{}, fmt.Errorf("no scripted tool result for %s call %d", p.name, idx)
	}
	return p.results[idx].reply, p.results[idx].err
}

type providerSequenceFake struct {
	name    string
	results []providerSequenceResult
	calls   int
}

type providerSequenceResult struct {
	reply string
	err   error
}

func (p *providerSequenceFake) Name() string { return p.name }

func (p *providerSequenceFake) Reply(_ context.Context, _ ProviderRequest) (string, error) {
	idx := p.calls
	p.calls++
	if idx >= len(p.results) {
		return "", fmt.Errorf("no scripted result for %s call %d", p.name, idx)
	}
	return p.results[idx].reply, p.results[idx].err
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

func TestProviderManagerReplyWithToolsPrefersToolAwareProvider(t *testing.T) {
	provider := &providerToolAwareFake{
		name: "tool-aware",
		reply: StructuredToolReply{
			Content: "tool aware response",
		},
	}

	pm := NewProviderManager(provider)
	reply, err := pm.ReplyWithTools(context.Background(), StructuredToolRequest{
		SystemPrompt: "system",
		Messages: []StructuredToolMessage{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("ReplyWithTools failed: %v", err)
	}
	if reply.Content != "tool aware response" {
		t.Fatalf("unexpected tool-aware response: %+v", reply)
	}
	if provider.replyCalls != 1 || provider.textCalls != 0 {
		t.Fatalf("expected tool-aware path only, got replyCalls=%d textCalls=%d", provider.replyCalls, provider.textCalls)
	}
}

func TestProviderManagerReplyFallsBackAfterProviderError(t *testing.T) {
	primary := &providerSequenceFake{
		name: "alpha",
		results: []providerSequenceResult{
			{err: errors.New("rate limit")},
		},
	}
	fallback := &providerSequenceFake{
		name: "beta",
		results: []providerSequenceResult{
			{reply: "fallback reply"},
		},
	}

	pm := NewProviderManager(primary)
	pm.cooldown = time.Minute
	pm.now = func() time.Time { return time.Unix(100, 0) }
	if err := pm.RegisterProvider(fallback); err != nil {
		t.Fatalf("register fallback: %v", err)
	}

	reply, err := pm.Reply(context.Background(), ProviderRequest{UserMessage: "hi"})
	if err != nil {
		t.Fatalf("Reply failed: %v", err)
	}
	if reply != "fallback reply" {
		t.Fatalf("unexpected fallback reply: %q", reply)
	}
	if pm.ActiveProviderName() != "beta" {
		t.Fatalf("expected active provider to switch to fallback, got %q", pm.ActiveProviderName())
	}
	if primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf("unexpected provider call counts: primary=%d fallback=%d", primary.calls, fallback.calls)
	}
}

func TestProviderManagerReplySkipsCooldownProvider(t *testing.T) {
	primary := &providerSequenceFake{
		name: "alpha",
		results: []providerSequenceResult{
			{err: errors.New("rate limit")},
			{reply: "should not be used during cooldown"},
		},
	}
	fallback := &providerSequenceFake{
		name: "beta",
		results: []providerSequenceResult{
			{reply: "fallback-1"},
			{reply: "fallback-2"},
		},
	}

	pm := NewProviderManager(primary)
	pm.cooldown = time.Minute
	now := time.Unix(200, 0)
	pm.now = func() time.Time { return now }
	if err := pm.RegisterProvider(fallback); err != nil {
		t.Fatalf("register fallback: %v", err)
	}

	reply, err := pm.Reply(context.Background(), ProviderRequest{UserMessage: "first"})
	if err != nil || reply != "fallback-1" {
		t.Fatalf("unexpected first fallback reply: %q err=%v", reply, err)
	}

	if err := pm.SetActiveProvider("alpha"); err != nil {
		t.Fatalf("reset active provider: %v", err)
	}
	reply, err = pm.Reply(context.Background(), ProviderRequest{UserMessage: "second"})
	if err != nil || reply != "fallback-2" {
		t.Fatalf("unexpected second fallback reply: %q err=%v", reply, err)
	}
	if primary.calls != 1 {
		t.Fatalf("expected cooldown to skip primary on second call, got %d calls", primary.calls)
	}
}

func TestProviderManagerReplyWithToolsFallsBackAfterProviderError(t *testing.T) {
	primary := &providerToolAwareSequenceFake{
		name: "alpha",
		results: []providerToolAwareSequenceResult{
			{err: errors.New("rate limit")},
		},
	}
	fallback := &providerToolAwareSequenceFake{
		name: "beta",
		results: []providerToolAwareSequenceResult{
			{reply: StructuredToolReply{Content: "fallback tool reply"}},
		},
	}

	pm := NewProviderManager(primary)
	pm.cooldown = time.Minute
	pm.now = func() time.Time { return time.Unix(300, 0) }
	if err := pm.RegisterProvider(fallback); err != nil {
		t.Fatalf("register fallback: %v", err)
	}

	reply, err := pm.ReplyWithTools(context.Background(), StructuredToolRequest{
		Messages: []StructuredToolMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ReplyWithTools failed: %v", err)
	}
	if reply.Content != "fallback tool reply" {
		t.Fatalf("unexpected fallback tool reply: %+v", reply)
	}
	if pm.ActiveProviderName() != "beta" {
		t.Fatalf("expected active provider to switch to fallback, got %q", pm.ActiveProviderName())
	}
	if primary.replyCalls != 1 || fallback.replyCalls != 1 {
		t.Fatalf("unexpected provider call counts: primary=%d fallback=%d", primary.replyCalls, fallback.replyCalls)
	}
}

func TestProviderManagerReplyWithToolsSkipsCooldownProvider(t *testing.T) {
	primary := &providerToolAwareSequenceFake{
		name: "alpha",
		results: []providerToolAwareSequenceResult{
			{err: errors.New("rate limit")},
			{reply: StructuredToolReply{Content: "should not be used during cooldown"}},
		},
	}
	fallback := &providerToolAwareSequenceFake{
		name: "beta",
		results: []providerToolAwareSequenceResult{
			{reply: StructuredToolReply{Content: "fallback-1"}},
			{reply: StructuredToolReply{Content: "fallback-2"}},
		},
	}

	pm := NewProviderManager(primary)
	pm.cooldown = time.Minute
	now := time.Unix(400, 0)
	pm.now = func() time.Time { return now }
	if err := pm.RegisterProvider(fallback); err != nil {
		t.Fatalf("register fallback: %v", err)
	}

	reply, err := pm.ReplyWithTools(context.Background(), StructuredToolRequest{
		Messages: []StructuredToolMessage{{Role: "user", Content: "first"}},
	})
	if err != nil || reply.Content != "fallback-1" {
		t.Fatalf("unexpected first fallback reply: %+v err=%v", reply, err)
	}

	if err := pm.SetActiveProvider("alpha"); err != nil {
		t.Fatalf("reset active provider: %v", err)
	}
	reply, err = pm.ReplyWithTools(context.Background(), StructuredToolRequest{
		Messages: []StructuredToolMessage{{Role: "user", Content: "second"}},
	})
	if err != nil || reply.Content != "fallback-2" {
		t.Fatalf("unexpected second fallback reply: %+v err=%v", reply, err)
	}
	if primary.replyCalls != 1 {
		t.Fatalf("expected cooldown to skip primary on second call, got %d calls", primary.replyCalls)
	}
}

func TestProviderManagerReplyWithToolsFallsBackFromMalformedTextProvider(t *testing.T) {
	primary := &providerSequenceFake{
		name: "alpha",
		results: []providerSequenceResult{
			{reply: `{"content":"broken"`},
		},
	}
	fallback := &providerSequenceFake{
		name: "beta",
		results: []providerSequenceResult{
			{reply: `{"content":"fallback from text","tool_calls":[]}`},
		},
	}

	pm := NewProviderManager(primary)
	pm.cooldown = time.Minute
	pm.now = func() time.Time { return time.Unix(450, 0) }
	if err := pm.RegisterProvider(fallback); err != nil {
		t.Fatalf("register fallback: %v", err)
	}

	reply, err := pm.ReplyWithTools(context.Background(), StructuredToolRequest{
		Messages: []StructuredToolMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ReplyWithTools failed: %v", err)
	}
	if reply.Content != "fallback from text" {
		t.Fatalf("unexpected fallback reply: %+v", reply)
	}
	if pm.ActiveProviderName() != "beta" {
		t.Fatalf("expected active provider to switch to fallback, got %q", pm.ActiveProviderName())
	}
	if primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf("unexpected provider call counts: primary=%d fallback=%d", primary.calls, fallback.calls)
	}
}

func TestProviderManagerReplyWithToolsRetriesTextProviderAfterCooldownExpires(t *testing.T) {
	primary := &providerSequenceFake{
		name: "alpha",
		results: []providerSequenceResult{
			{reply: `{"content":"broken"`},
			{reply: `{"content":"primary recovered","tool_calls":[]}`},
		},
	}
	fallback := &providerSequenceFake{
		name: "beta",
		results: []providerSequenceResult{
			{reply: `{"content":"fallback-1","tool_calls":[]}`},
		},
	}

	pm := NewProviderManager(primary)
	pm.cooldown = time.Minute
	now := time.Unix(500, 0)
	pm.now = func() time.Time { return now }
	if err := pm.RegisterProvider(fallback); err != nil {
		t.Fatalf("register fallback: %v", err)
	}

	reply, err := pm.ReplyWithTools(context.Background(), StructuredToolRequest{
		Messages: []StructuredToolMessage{{Role: "user", Content: "first"}},
	})
	if err != nil || reply.Content != "fallback-1" {
		t.Fatalf("unexpected first fallback reply: %+v err=%v", reply, err)
	}

	if err := pm.SetActiveProvider("alpha"); err != nil {
		t.Fatalf("reset active provider: %v", err)
	}
	now = now.Add(2 * time.Minute)

	reply, err = pm.ReplyWithTools(context.Background(), StructuredToolRequest{
		Messages: []StructuredToolMessage{{Role: "user", Content: "second"}},
	})
	if err != nil {
		t.Fatalf("ReplyWithTools after cooldown failed: %v", err)
	}
	if reply.Content != "primary recovered" {
		t.Fatalf("unexpected recovered reply: %+v", reply)
	}
	if primary.calls != 2 {
		t.Fatalf("expected primary to be retried after cooldown expiry, got %d calls", primary.calls)
	}
	if fallback.calls != 1 {
		t.Fatalf("expected fallback to be used only once, got %d calls", fallback.calls)
	}
	if pm.ActiveProviderName() != "alpha" {
		t.Fatalf("expected active provider to switch back to primary, got %q", pm.ActiveProviderName())
	}
}

func TestProviderManagerReplyDoesNotCooldownNonRetriableErrors(t *testing.T) {
	primary := &providerSequenceFake{
		name: "alpha",
		results: []providerSequenceResult{
			{err: NonRetriableProviderError(errors.New("invalid request"))},
			{reply: "primary recovered"},
		},
	}
	fallback := &providerSequenceFake{
		name: "beta",
		results: []providerSequenceResult{
			{reply: "fallback-1"},
		},
	}

	pm := NewProviderManager(primary)
	pm.cooldown = time.Minute
	now := time.Unix(550, 0)
	pm.now = func() time.Time { return now }
	if err := pm.RegisterProvider(fallback); err != nil {
		t.Fatalf("register fallback: %v", err)
	}

	reply, err := pm.Reply(context.Background(), ProviderRequest{UserMessage: "first"})
	if err != nil || reply != "fallback-1" {
		t.Fatalf("unexpected first fallback reply: %q err=%v", reply, err)
	}

	if err := pm.SetActiveProvider("alpha"); err != nil {
		t.Fatalf("reset active provider: %v", err)
	}
	reply, err = pm.Reply(context.Background(), ProviderRequest{UserMessage: "second"})
	if err != nil {
		t.Fatalf("second reply failed: %v", err)
	}
	if reply != "primary recovered" {
		t.Fatalf("expected primary to be retried after non-retriable error, got %q", reply)
	}
	if primary.calls != 2 {
		t.Fatalf("expected primary to be called twice, got %d", primary.calls)
	}
	if fallback.calls != 1 {
		t.Fatalf("expected fallback to be used once, got %d", fallback.calls)
	}
}

func TestProviderManagerReplyWithToolsDoesNotCooldownNonRetriableErrors(t *testing.T) {
	primary := &providerToolAwareSequenceFake{
		name: "alpha",
		results: []providerToolAwareSequenceResult{
			{err: NonRetriableProviderError(errors.New("invalid tool schema"))},
			{reply: StructuredToolReply{Content: "primary recovered"}},
		},
	}
	fallback := &providerToolAwareSequenceFake{
		name: "beta",
		results: []providerToolAwareSequenceResult{
			{reply: StructuredToolReply{Content: "fallback-1"}},
		},
	}

	pm := NewProviderManager(primary)
	pm.cooldown = time.Minute
	now := time.Unix(600, 0)
	pm.now = func() time.Time { return now }
	if err := pm.RegisterProvider(fallback); err != nil {
		t.Fatalf("register fallback: %v", err)
	}

	reply, err := pm.ReplyWithTools(context.Background(), StructuredToolRequest{
		Messages: []StructuredToolMessage{{Role: "user", Content: "first"}},
	})
	if err != nil || reply.Content != "fallback-1" {
		t.Fatalf("unexpected first fallback reply: %+v err=%v", reply, err)
	}

	if err := pm.SetActiveProvider("alpha"); err != nil {
		t.Fatalf("reset active provider: %v", err)
	}
	reply, err = pm.ReplyWithTools(context.Background(), StructuredToolRequest{
		Messages: []StructuredToolMessage{{Role: "user", Content: "second"}},
	})
	if err != nil {
		t.Fatalf("second tool reply failed: %v", err)
	}
	if reply.Content != "primary recovered" {
		t.Fatalf("expected primary tool provider to be retried after non-retriable error, got %+v", reply)
	}
	if primary.replyCalls != 2 {
		t.Fatalf("expected primary tool provider to be called twice, got %d", primary.replyCalls)
	}
	if fallback.replyCalls != 1 {
		t.Fatalf("expected fallback tool provider to be used once, got %d", fallback.replyCalls)
	}
}

func TestProviderManagerReplyHonorsCanceledContextBeforeCallingProviders(t *testing.T) {
	primary := &providerSequenceFake{name: "alpha", results: []providerSequenceResult{{reply: "should not run"}}}
	fallback := &providerSequenceFake{name: "beta", results: []providerSequenceResult{{reply: "should not run"}}}

	pm := NewProviderManager(primary)
	if err := pm.RegisterProvider(fallback); err != nil {
		t.Fatalf("register fallback: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := pm.Reply(ctx, ProviderRequest{UserMessage: "stop"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if primary.calls != 0 || fallback.calls != 0 {
		t.Fatalf("expected canceled context to skip providers, got primary=%d fallback=%d", primary.calls, fallback.calls)
	}
}

func TestProviderManagerReplyWithToolsHonorsCanceledContextBeforeCallingProviders(t *testing.T) {
	primary := &providerToolAwareSequenceFake{name: "alpha", results: []providerToolAwareSequenceResult{{reply: StructuredToolReply{Content: "should not run"}}}}
	fallback := &providerToolAwareSequenceFake{name: "beta", results: []providerToolAwareSequenceResult{{reply: StructuredToolReply{Content: "should not run"}}}}

	pm := NewProviderManager(primary)
	if err := pm.RegisterProvider(fallback); err != nil {
		t.Fatalf("register fallback: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := pm.ReplyWithTools(ctx, StructuredToolRequest{
		Messages: []StructuredToolMessage{{Role: "user", Content: "stop"}},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if primary.replyCalls != 0 || fallback.replyCalls != 0 {
		t.Fatalf("expected canceled context to skip providers, got primary=%d fallback=%d", primary.replyCalls, fallback.replyCalls)
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

func TestRuntimeMetadataCommandsIncludeWorkspaceTools(t *testing.T) {
	rt := NewRuntime(&runtimeServiceFake{}, nil, WithWorkspaceRoot(t.TempDir()))

	resp, err := rt.Chat(context.Background(), ChatRequest{Provider: "cli", ChatID: "unit", Message: "/tools"})
	if err != nil {
		t.Fatalf("tools command failed: %v", err)
	}
	if !strings.Contains(resp.Message, "Workspace tools") {
		t.Fatalf("expected workspace tools section, got: %q", resp.Message)
	}
	if !strings.Contains(resp.Message, "append_file") || !strings.Contains(resp.Message, "exec") {
		t.Fatalf("expected execution tool descriptors in summary, got: %q", resp.Message)
	}
}

func TestRuntimeMetadataCommandsIncludeBuiltInToolSchemas(t *testing.T) {
	rt := NewRuntime(&runtimeServiceFake{}, nil)

	resp, err := rt.Chat(context.Background(), ChatRequest{Provider: "cli", ChatID: "unit", Message: "/tools"})
	if err != nil {
		t.Fatalf("tools command failed: %v", err)
	}
	if !strings.Contains(resp.Message, "agent_action") {
		t.Fatalf("expected built-in tool name in summary, got: %q", resp.Message)
	}
	if !strings.Contains(resp.Message, "parameters=") || !strings.Contains(resp.Message, `"agent_id":{"type":"string"}`) {
		t.Fatalf("expected built-in tool schema in summary, got: %q", resp.Message)
	}
}
