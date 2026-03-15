package baseagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type stubSubagentSpawner struct {
	handle SubagentJobHandle
	err    error
	calls  []SubagentRequest
}

func (s *stubSubagentSpawner) Spawn(_ context.Context, req SubagentRequest) (SubagentJobHandle, error) {
	s.calls = append(s.calls, req)
	if s.err != nil {
		return SubagentJobHandle{}, s.err
	}
	return s.handle, nil
}

type scriptedToolAwareProvider struct {
	name       string
	replies    []StructuredToolReply
	requests   []StructuredToolRequest
	replyCalls int
	textCalls  int
}

func (p *scriptedToolAwareProvider) Name() string { return p.name }

func (p *scriptedToolAwareProvider) Reply(_ context.Context, _ ProviderRequest) (string, error) {
	p.textCalls++
	return "", nil
}

func (p *scriptedToolAwareProvider) ReplyWithTools(_ context.Context, req StructuredToolRequest) (StructuredToolReply, error) {
	p.requests = append(p.requests, req)
	p.replyCalls++
	reply := p.replies[0]
	p.replies = p.replies[1:]
	return reply, nil
}

type scriptedTextProvider struct {
	name     string
	replies  []string
	requests []ProviderRequest
}

type runtimeExtendedMemoryFake struct {
	*runtimeMemoryFake
	searchHits   []MemorySearchHit
	searchErr    error
	searchCalls  []runtimeMemorySearchCall
	observeID    string
	observeErr   error
	observeCalls []runtimeMemoryObserveCall
	records      map[string]MemoryRecord
	getRecordErr error
	grantID      string
	grantErr     error
	revokeErr    error
	audits       []MemoryAudit
}

type runtimeMemorySearchCall struct {
	subject    string
	query      string
	maxResults int
	minScore   float64
}

type runtimeMemoryObserveCall struct {
	subject       string
	toolName      string
	outputSnippet string
	scope         string
}

func newRuntimeExtendedMemoryFake() *runtimeExtendedMemoryFake {
	return &runtimeExtendedMemoryFake{
		runtimeMemoryFake: newRuntimeMemoryFake(),
		records:           map[string]MemoryRecord{},
	}
}

func (m *runtimeExtendedMemoryFake) Search(subject, query string, maxResults int, minScore float64) ([]MemorySearchHit, error) {
	m.searchCalls = append(m.searchCalls, runtimeMemorySearchCall{
		subject:    subject,
		query:      query,
		maxResults: maxResults,
		minScore:   minScore,
	})
	if m.searchErr != nil {
		return nil, m.searchErr
	}
	out := make([]MemorySearchHit, len(m.searchHits))
	copy(out, m.searchHits)
	return out, nil
}

func (m *runtimeExtendedMemoryFake) GetRecord(_ string, id string) (MemoryRecord, error) {
	if m.getRecordErr != nil {
		return MemoryRecord{}, m.getRecordErr
	}
	rec, ok := m.records[id]
	if !ok {
		return MemoryRecord{}, os.ErrNotExist
	}
	return rec, nil
}

func (m *runtimeExtendedMemoryFake) Observe(subject, toolName, outputSnippet, scope string) (string, error) {
	m.observeCalls = append(m.observeCalls, runtimeMemoryObserveCall{
		subject:       subject,
		toolName:      toolName,
		outputSnippet: outputSnippet,
		scope:         scope,
	})
	if m.observeErr != nil {
		return "", m.observeErr
	}
	if strings.TrimSpace(m.observeID) == "" {
		return "obs-runtime", nil
	}
	return m.observeID, nil
}

func (m *runtimeExtendedMemoryFake) Grant(_ string, _ string, _ string, _ string) (string, error) {
	if m.grantErr != nil {
		return "", m.grantErr
	}
	if strings.TrimSpace(m.grantID) == "" {
		return "grant-runtime", nil
	}
	return m.grantID, nil
}

func (m *runtimeExtendedMemoryFake) Revoke(_ string, _ string) error {
	return m.revokeErr
}

func (m *runtimeExtendedMemoryFake) ListAudits() []MemoryAudit {
	out := make([]MemoryAudit, len(m.audits))
	copy(out, m.audits)
	return out
}

func (p *scriptedTextProvider) Name() string { return p.name }

func (p *scriptedTextProvider) Reply(_ context.Context, req ProviderRequest) (string, error) {
	p.requests = append(p.requests, req)
	reply := p.replies[0]
	p.replies = p.replies[1:]
	return reply, nil
}

func TestBaseagentMemoryQueryUsesCarrierStore(t *testing.T) {
	mem := newRuntimeExtendedMemoryFake()
	mem.searchHits = []MemorySearchHit{
		{ID: "rec-1", Scope: "public", Score: 0.91, Snippet: "timezone preference: JST", Provenance: "truth/public/preferences.md"},
	}
	provider := &scriptedToolAwareProvider{
		name: "memory-aware",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "memory_search",
						Arguments: map[string]any{
							"query": "timezone",
						},
					},
				},
			},
			{
				Content: "memory search completed",
			},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, mem, WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "memory-search",
		Message:  "find memory about timezone",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "memory search completed" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(mem.searchCalls) != 1 {
		t.Fatalf("expected 1 memory search call, got %d", len(mem.searchCalls))
	}
	if mem.searchCalls[0].subject != baseAgentVirtualID {
		t.Fatalf("expected baseagent subject, got %+v", mem.searchCalls[0])
	}
	if mem.searchCalls[0].query != "timezone" {
		t.Fatalf("unexpected search query: %+v", mem.searchCalls[0])
	}
	last := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
	if last.Role != "tool" || last.ToolCallID != "call-1" {
		t.Fatalf("unexpected memory tool callback: %+v", last)
	}
	if !strings.Contains(last.Content, "timezone preference: JST") {
		t.Fatalf("expected search hit snippet in callback, got %+v", last)
	}
}

func TestBaseagentMemoryQueryUsesRequestMemorySubject(t *testing.T) {
	mem := newRuntimeExtendedMemoryFake()
	mem.searchHits = []MemorySearchHit{
		{ID: "rec-1", Scope: "shared:snapshot-child-1", Score: 0.91, Snippet: "delegated timezone: JST", Provenance: "snapshot:child-1"},
	}
	provider := &scriptedToolAwareProvider{
		name: "memory-aware-override",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "memory_search",
						Arguments: map[string]any{
							"query": "timezone",
						},
					},
				},
			},
			{
				Content: "memory search completed",
			},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, mem, WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider:      "cli",
		ChatID:        "memory-search-override",
		Message:       "find delegated memory about timezone",
		MemorySubject: "child-1",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "memory search completed" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(mem.searchCalls) != 1 {
		t.Fatalf("expected 1 memory search call, got %d", len(mem.searchCalls))
	}
	if mem.searchCalls[0].subject != "child-1" {
		t.Fatalf("expected request memory subject, got %+v", mem.searchCalls[0])
	}
}

func TestStructuredLoopObserveMemory(t *testing.T) {
	mem := newRuntimeExtendedMemoryFake()
	provider := &scriptedToolAwareProvider{
		name: "observe-aware",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "list_agents",
					},
				},
			},
			{
				Content: "listed agents",
			},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{
		agents: []AgentState{{ID: "openclaw", Install: "installed", Runtime: "running", Health: "ok"}},
	}, mem, WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "memory-observe",
		Message:  "check the current fleet posture",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "listed agents" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(mem.observeCalls) != 1 {
		t.Fatalf("expected 1 memory observe call, got %d", len(mem.observeCalls))
	}
	if mem.observeCalls[0].subject != baseAgentVirtualID {
		t.Fatalf("unexpected observe subject: %+v", mem.observeCalls[0])
	}
	if mem.observeCalls[0].toolName != "list_agents" {
		t.Fatalf("unexpected observe tool name: %+v", mem.observeCalls[0])
	}
	if !strings.Contains(mem.observeCalls[0].outputSnippet, "openclaw") {
		t.Fatalf("expected tool output in observe call, got %+v", mem.observeCalls[0])
	}
}

func TestStructuredLoopObserveMemoryUsesRequestMemorySubject(t *testing.T) {
	mem := newRuntimeExtendedMemoryFake()
	provider := &scriptedToolAwareProvider{
		name: "observe-aware-override",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "list_agents",
					},
				},
			},
			{
				Content: "listed agents",
			},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{
		agents: []AgentState{{ID: "openclaw", Install: "installed", Runtime: "running", Health: "ok"}},
	}, mem, WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider:      "cli",
		ChatID:        "memory-observe-override",
		Message:       "check the current fleet posture",
		MemorySubject: "child-1",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "listed agents" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(mem.observeCalls) != 1 {
		t.Fatalf("expected 1 memory observe call, got %d", len(mem.observeCalls))
	}
	if mem.observeCalls[0].subject != "child-1" {
		t.Fatalf("expected request memory subject, got %+v", mem.observeCalls[0])
	}
}

func TestMemoryPolicyBlocksUnauthorizedScope(t *testing.T) {
	mem := newRuntimeExtendedMemoryFake()
	provider := &scriptedToolAwareProvider{
		name: "memory-policy",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "memory_search",
						Arguments: map[string]any{
							"query": "private",
							"scope": "agent:other-agent",
						},
					},
				},
			},
			{
				Content: "blocked",
			},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, mem, WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "memory-scope",
		Message:  "search forbidden memory",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "blocked" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(mem.searchCalls) != 0 {
		t.Fatalf("unauthorized scope should not reach memory store: %+v", mem.searchCalls)
	}
	last := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
	if last.ToolResultStatus != ExecutionToolResultStatusError {
		t.Fatalf("expected memory scope failure, got %+v", last)
	}
	if !strings.Contains(strings.ToLower(last.Content), "unauthorized memory scope") {
		t.Fatalf("unexpected memory scope failure message: %+v", last)
	}
}

func TestRuntimeChatUsesToolAwareProviderLoop(t *testing.T) {
	root := t.TempDir()
	provider := &scriptedToolAwareProvider{
		name: "scripted",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "write_file",
						Arguments: map[string]any{
							"path":    "generated.txt",
							"content": "hello from structured loop",
						},
					},
				},
			},
			{
				Content: "created generated.txt",
			},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithWorkspaceRoot(root), WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "structured",
		Message:  "create generated.txt",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "created generated.txt" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}

	raw, err := os.ReadFile(filepath.Join(root, "generated.txt"))
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	if string(raw) != "hello from structured loop" {
		t.Fatalf("unexpected file content: %q", string(raw))
	}

	if len(provider.requests) != 2 {
		t.Fatalf("expected 2 provider requests, got %d", len(provider.requests))
	}
	if len(provider.requests[1].Messages) == 0 {
		t.Fatalf("expected tool result in second provider request")
	}
	last := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
	if last.Role != "tool" || last.ToolCallID != "call-1" {
		t.Fatalf("unexpected final tool message: %+v", last)
	}
}

func TestRuntimeChatStructuredLoopCarriesAttachmentMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "artifact.log"), []byte("hello attachment"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	policy := ActiveBoundarySpec().StructuredToolPolicy
	policy.HighRiskDecision = string(structuredToolDecisionAllow)
	provider := &scriptedToolAwareProvider{
		name: "attachment-aware",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "send_file",
						Arguments: map[string]any{
							"path": "artifact.log",
						},
					},
				},
			},
			{
				Content: "attachment prepared",
			},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithWorkspaceRoot(root), WithMaxToolIterations(4), WithStructuredToolPolicy(policy))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "attachment-loop",
		Message:  "send artifact.log",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "attachment prepared" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}

	last := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
	if last.Role != "tool" || last.ToolCallID != "call-1" {
		t.Fatalf("unexpected final tool message: %+v", last)
	}
	if len(last.Attachments) != 1 {
		t.Fatalf("expected first-class attachment metadata, got %+v", last)
	}
	if last.Attachments[0].Name != "artifact.log" {
		t.Fatalf("unexpected attachment refs: %+v", last.Attachments)
	}
	if len(last.ContentBlocks) != 1 || last.ContentBlocks[0].Type != "file" {
		t.Fatalf("unexpected content blocks: %+v", last.ContentBlocks)
	}
	if last.ContentBlocks[0].AttachmentID != last.Attachments[0].ID {
		t.Fatalf("expected file block to point at attachment id, block=%+v attachments=%+v", last.ContentBlocks[0], last.Attachments)
	}
}

func TestRuntimeChatUsesTextProviderStructuredFallback(t *testing.T) {
	root := t.TempDir()
	provider := &scriptedTextProvider{
		name: "scripted-text",
		replies: []string{
			`{"tool_calls":[{"name":"write_file","arguments":{"path":"fallback.txt","content":"created through text provider"}}]}`,
			`{"content":"created fallback.txt","tool_calls":[]}`,
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithWorkspaceRoot(root), WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "text-fallback",
		Message:  "create fallback.txt",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "created fallback.txt" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}

	raw, err := os.ReadFile(filepath.Join(root, "fallback.txt"))
	if err != nil {
		t.Fatalf("read fallback file: %v", err)
	}
	if string(raw) != "created through text provider" {
		t.Fatalf("unexpected file content: %q", string(raw))
	}

	if len(provider.requests) != 2 {
		t.Fatalf("expected 2 provider requests, got %d", len(provider.requests))
	}
	if !strings.Contains(provider.requests[0].UserMessage, `"tool_calls"`) {
		t.Fatalf("expected structured prompt in text-provider fallback, got %q", provider.requests[0].UserMessage)
	}
}

func TestRuntimeChatStructuredLoopRecoversFromToolError(t *testing.T) {
	root := t.TempDir()
	provider := &scriptedToolAwareProvider{
		name: "scripted-recover",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "read_file",
						Arguments: map[string]any{
							"path": "missing.txt",
						},
					},
				},
			},
			{
				Content: "missing file handled",
			},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithWorkspaceRoot(root), WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "recover",
		Message:  "read missing.txt",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "missing file handled" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}

	last := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
	if last.Role != "tool" || !strings.Contains(strings.ToLower(last.Content), "read file") {
		t.Fatalf("expected tool error to be fed back into provider, got %+v", last)
	}
}

func TestRuntimeChatStructuredLoopInjectsSessionSummary(t *testing.T) {
	root := t.TempDir()
	provider := &scriptedToolAwareProvider{
		name: "summary-aware",
		replies: []StructuredToolReply{
			{Content: "summary seen"},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithWorkspaceRoot(root), WithMaxToolIterations(2))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	rt.sessions.SetSummary("cli:summary", "important prior context")
	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "summary",
		Message:  "continue",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "summary seen" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(provider.requests) != 1 || len(provider.requests[0].Messages) == 0 {
		t.Fatalf("expected initial structured request, got %+v", provider.requests)
	}
	first := provider.requests[0].Messages[0]
	if first.Role != "system" || !strings.Contains(first.Content, "important prior context") {
		t.Fatalf("expected session summary injection, got %+v", first)
	}
}

func TestRuntimeChatStructuredLoopHandlesMultipleToolCalls(t *testing.T) {
	root := t.TempDir()
	provider := &scriptedToolAwareProvider{
		name: "multi-call",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "write_file",
						Arguments: map[string]any{
							"path":    "one.txt",
							"content": "one",
						},
					},
					{
						ID:   "call-2",
						Name: "write_file",
						Arguments: map[string]any{
							"path":    "two.txt",
							"content": "two",
						},
					},
				},
			},
			{Content: "both files created"},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithWorkspaceRoot(root), WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "multi",
		Message:  "create two files",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "both files created" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	for _, file := range []string{"one.txt", "two.txt"} {
		if _, err := os.Stat(filepath.Join(root, file)); err != nil {
			t.Fatalf("expected %s to be created: %v", file, err)
		}
	}
	secondReq := provider.requests[1]
	if got := len(secondReq.Messages); got < 4 {
		t.Fatalf("expected assistant plus two tool messages in follow-up request, got %+v", secondReq.Messages)
	}
	lastTwo := secondReq.Messages[len(secondReq.Messages)-2:]
	if lastTwo[0].Role != "tool" || lastTwo[0].ToolCallID != "call-1" {
		t.Fatalf("unexpected first tool callback message: %+v", lastTwo[0])
	}
	if lastTwo[1].Role != "tool" || lastTwo[1].ToolCallID != "call-2" {
		t.Fatalf("unexpected second tool callback message: %+v", lastTwo[1])
	}
}

func TestRuntimeChatStructuredLoopIncludesBuiltInAndWorkspaceToolDescriptors(t *testing.T) {
	root := t.TempDir()
	provider := &scriptedToolAwareProvider{
		name: "descriptor-aware",
		replies: []StructuredToolReply{
			{Content: "done"},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithWorkspaceRoot(root), WithMaxToolIterations(2))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "descriptor-mix",
		Message:  "show me the available actions",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "done" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("expected 1 structured request, got %d", len(provider.requests))
	}

	seen := map[string]StructuredToolDescriptor{}
	for _, descriptor := range provider.requests[0].Tools {
		seen[descriptor.Name] = descriptor
	}
	if _, ok := seen["agent_status"]; !ok {
		t.Fatalf("expected split built-in tool descriptor in structured request, got %+v", provider.requests[0].Tools)
	}
	if _, ok := seen["agent_action"]; ok {
		t.Fatalf("did not expect generic agent_action descriptor in structured request, got %+v", provider.requests[0].Tools)
	}
	if _, ok := seen["write_file"]; !ok {
		t.Fatalf("expected workspace tool descriptor in structured request, got %+v", provider.requests[0].Tools)
	}
	if _, ok := seen["exec"]; ok {
		t.Fatalf("did not expect high-risk exec descriptor in structured request, got %+v", provider.requests[0].Tools)
	}
	if seen["agent_status"].Parameters == nil {
		t.Fatalf("expected built-in descriptor schema in structured request, got %+v", seen["agent_status"])
	}
}

func TestRuntimeChatStructuredLoopExecutesBuiltInToolCalls(t *testing.T) {
	root := t.TempDir()
	provider := &scriptedToolAwareProvider{
		name: "builtin-tool-aware",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "agent_status",
						Arguments: map[string]any{
							"agent_id": "openclaw",
						},
					},
				},
			},
			{
				Content: "got agent status",
			},
		},
	}

	svc := &runtimeServiceFake{
		agents: []AgentState{
			{ID: "openclaw", Install: "installed", Runtime: "running", Health: "healthy"},
		},
		statuses: map[string]AgentState{
			"openclaw": {ID: "openclaw", Install: "installed", Runtime: "running", Health: "healthy"},
		},
	}
	rt := NewRuntime(svc, nil, WithWorkspaceRoot(root), WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "builtin-tool",
		Message:  "check agent status",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "got agent status" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected tool follow-up request, got %d", len(provider.requests))
	}
	last := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
	if last.Role != "tool" || last.ToolCallID != "call-1" {
		t.Fatalf("unexpected tool callback message: %+v", last)
	}
	if !strings.Contains(last.Content, "openclaw") || !strings.Contains(last.Content, "runtime=running") {
		t.Fatalf("expected built-in tool result to include agent status, got %+v", last)
	}
}

func TestRuntimeChatStructuredLoopExecutesBuiltInToolCallsWithoutWorkspaceRoot(t *testing.T) {
	provider := &scriptedToolAwareProvider{
		name: "builtin-only",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:        "call-1",
						Name:      "list_agents",
						Arguments: map[string]any{},
					},
				},
			},
			{
				Content: "listed agents without workspace",
			},
		},
	}

	svc := &runtimeServiceFake{
		agents: []AgentState{
			{ID: "openclaw", Install: "installed", Runtime: "running", Health: "healthy"},
		},
	}
	rt := NewRuntime(svc, nil, WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "builtin-noworkspace",
		Message:  "check agent inventory",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "listed agents without workspace" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected structured loop follow-up request, got %d", len(provider.requests))
	}

	seen := map[string]StructuredToolDescriptor{}
	for _, descriptor := range provider.requests[0].Tools {
		seen[descriptor.Name] = descriptor
	}
	if _, ok := seen["list_agents"]; !ok {
		t.Fatalf("expected built-in tool descriptor without workspace, got %+v", provider.requests[0].Tools)
	}
	if _, ok := seen["web_fetch"]; !ok {
		t.Fatalf("expected web_fetch descriptor without workspace root, got %+v", provider.requests[0].Tools)
	}
	if _, ok := seen["web_search"]; !ok {
		t.Fatalf("expected web_search descriptor without workspace root, got %+v", provider.requests[0].Tools)
	}
	if _, ok := seen["write_file"]; ok {
		t.Fatalf("did not expect workspace tool descriptor without workspace root, got %+v", provider.requests[0].Tools)
	}

	last := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
	if last.Role != "tool" || last.ToolCallID != "call-1" {
		t.Fatalf("unexpected tool callback message: %+v", last)
	}
	if !strings.Contains(last.Content, "openclaw") {
		t.Fatalf("expected built-in tool result to include agent listing, got %+v", last)
	}
}

func TestRuntimeChatStructuredLoopAsksForHighRiskBuiltInToolCalls(t *testing.T) {
	provider := &scriptedToolAwareProvider{
		name: "ask-high-risk-builtin",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "agent_start",
						Arguments: map[string]any{
							"agent_id": "openclaw",
						},
					},
				},
			},
			{
				Content: "high risk needs confirmation",
			},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "ask-high-risk-builtin",
		Message:  "perform maintenance planning",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "high risk needs confirmation" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected ask callback follow-up request, got %d", len(provider.requests))
	}
	last := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
	if last.Role != "tool" || last.ToolCallID != "call-1" {
		t.Fatalf("unexpected tool callback message: %+v", last)
	}
	if last.ToolName != "agent_start" || last.ToolResultStatus != ExecutionToolResultStatusAsk {
		t.Fatalf("expected structured ask metadata, got %+v", last)
	}
	if !strings.Contains(strings.ToLower(last.Content), "confirmation") {
		t.Fatalf("expected high-risk ask message, got %+v", last)
	}
}

func TestRuntimeChatStructuredLoopAsksForHighRiskWorkspaceExec(t *testing.T) {
	root := t.TempDir()
	provider := &scriptedToolAwareProvider{
		name: "ask-high-risk-exec",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "exec",
						Arguments: map[string]any{
							"command": "go test ./...",
						},
					},
				},
			},
			{
				Content: "exec needs confirmation",
			},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithWorkspaceRoot(root), WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "ask-high-risk-exec",
		Message:  "run repository diagnostics",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "exec needs confirmation" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected ask callback follow-up request, got %d", len(provider.requests))
	}
	last := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
	if last.Role != "tool" || last.ToolCallID != "call-1" {
		t.Fatalf("unexpected tool callback message: %+v", last)
	}
	if last.ToolName != "exec" || last.ToolResultStatus != ExecutionToolResultStatusAsk {
		t.Fatalf("expected structured ask metadata, got %+v", last)
	}
	if !strings.Contains(strings.ToLower(last.Content), "confirmation") {
		t.Fatalf("expected high-risk exec ask message, got %+v", last)
	}
}

func TestRuntimeChatStructuredLoopUsesStructuredToolPolicyOverride(t *testing.T) {
	root := t.TempDir()
	provider := &scriptedToolAwareProvider{
		name: "deny-high-risk-exec",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "exec",
						Arguments: map[string]any{
							"command": "go test ./...",
						},
					},
				},
			},
			{
				Content: "exec denied by override",
			},
		},
	}

	policy := ActiveBoundarySpec().StructuredToolPolicy
	policy.HighRiskDecision = string(structuredToolDecisionDeny)

	rt := NewRuntime(
		&runtimeServiceFake{},
		nil,
		WithWorkspaceRoot(root),
		WithMaxToolIterations(4),
		WithStructuredToolPolicy(policy),
	)
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "deny-high-risk-exec",
		Message:  "run repository diagnostics",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "exec denied by override" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected deny callback follow-up request, got %d", len(provider.requests))
	}
	last := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
	if last.ToolName != "exec" || last.ToolResultStatus != ExecutionToolResultStatusDeny {
		t.Fatalf("expected deny metadata after runtime override, got %+v", last)
	}
}

func TestRuntimeChatStructuredLoopDeniesBlockedExecCommandByArgumentPolicy(t *testing.T) {
	root := t.TempDir()
	provider := &scriptedToolAwareProvider{
		name: "deny-blocked-exec",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "exec",
						Arguments: map[string]any{
							"command": "rm -rf /",
						},
					},
				},
			},
			{
				Content: "blocked exec denied",
			},
		},
	}

	rt := NewRuntime(&runtimeServiceFake{}, nil, WithWorkspaceRoot(root), WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "deny-blocked-exec",
		Message:  "run destructive command",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "blocked exec denied" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected deny callback follow-up request, got %d", len(provider.requests))
	}
	last := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
	if last.ToolName != "exec" || last.ToolResultStatus != ExecutionToolResultStatusDeny {
		t.Fatalf("expected argument-level deny metadata, got %+v", last)
	}
	if last.ToolPolicyRuleID != "exec.blocked_command" {
		t.Fatalf("expected blocked command rule metadata, got %+v", last)
	}
}

func TestRuntimeChatStructuredLoopConfirmExecutesPendingApproval(t *testing.T) {
	provider := &scriptedToolAwareProvider{
		name: "confirm-pending",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "agent_start",
						Arguments: map[string]any{
							"agent_id": "openclaw",
						},
					},
				},
			},
			{
				Content: "please confirm the pending start",
			},
			{
				Content: "confirmed and started openclaw",
			},
		},
	}

	svc := &runtimeServiceFake{}
	rt := NewRuntime(svc, nil, WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	firstResp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "confirm-pending",
		Message:  "perform maintenance planning",
	})
	if err != nil {
		t.Fatalf("first runtime chat: %v", err)
	}
	if firstResp.Message != "please confirm the pending start" {
		t.Fatalf("unexpected first response: %+v", firstResp)
	}
	pending := rt.sessions.PendingApproval("cli:confirm-pending")
	if pending == nil || pending.ToolName != "agent_start" {
		t.Fatalf("expected pending approval for agent_start, got %+v", pending)
	}

	secondResp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "confirm-pending",
		Message:  "confirm",
	})
	if err != nil {
		t.Fatalf("second runtime chat: %v", err)
	}
	if secondResp.Message != "confirmed and started openclaw" {
		t.Fatalf("unexpected confirmation response: %+v", secondResp)
	}
	if pending := rt.sessions.PendingApproval("cli:confirm-pending"); pending != nil {
		t.Fatalf("expected pending approval to be cleared after confirm, got %+v", pending)
	}
	if len(svc.callLog) != 1 || svc.callLog[0] != "start:openclaw" {
		t.Fatalf("expected confirmed tool execution, got %+v", svc.callLog)
	}
	if len(provider.requests) != 3 {
		t.Fatalf("expected confirmation to resume provider loop, got %d requests", len(provider.requests))
	}
}

func TestRuntimeChatStructuredLoopCancelClearsPendingApproval(t *testing.T) {
	provider := &scriptedToolAwareProvider{
		name: "cancel-pending",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "agent_start",
						Arguments: map[string]any{
							"agent_id": "openclaw",
						},
					},
				},
			},
			{
				Content: "please confirm the pending start",
			},
		},
	}

	svc := &runtimeServiceFake{}
	rt := NewRuntime(svc, nil, WithMaxToolIterations(4))
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	if _, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "cancel-pending",
		Message:  "perform maintenance planning",
	}); err != nil {
		t.Fatalf("first runtime chat: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "cancel-pending",
		Message:  "cancel",
	})
	if err != nil {
		t.Fatalf("cancel runtime chat: %v", err)
	}
	if !strings.Contains(strings.ToLower(resp.Message), "canceled pending approval") {
		t.Fatalf("unexpected cancel response: %+v", resp)
	}
	if pending := rt.sessions.PendingApproval("cli:cancel-pending"); pending != nil {
		t.Fatalf("expected pending approval to be cleared after cancel, got %+v", pending)
	}
	if len(svc.callLog) != 0 {
		t.Fatalf("expected cancel to skip tool execution, got %+v", svc.callLog)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected cancel to skip provider resume, got %d requests", len(provider.requests))
	}
}

func TestAgentLoopStructuredLoopStopsAtMaxIterations(t *testing.T) {
	provider := &scriptedToolAwareProvider{
		name: "infinite-tools",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{ID: "call-1", Name: "list_dir", Arguments: map[string]any{"path": "."}},
				},
			},
			{
				ToolCalls: []StructuredToolCall{
					{ID: "call-2", Name: "list_dir", Arguments: map[string]any{"path": "."}},
				},
			},
			{
				ToolCalls: []StructuredToolCall{
					{ID: "call-3", Name: "list_dir", Arguments: map[string]any{"path": "."}},
				},
			},
		},
	}

	pm := NewProviderManager(provider)
	sessions := NewSessionManager(8)
	loop := NewAgentLoop(&runtimeServiceFake{}, NewToolRegistry(), pm, sessions, NewMessageBus(0, 0, 0))
	loop.SetExecutionTools(NewExecutionToolRegistry(t.TempDir()), 2, ActiveBoundarySpec().StructuredToolPolicy, nil, nil)

	_, handled, err := loop.processStructuredChat(context.Background(), "cli:max-iterations", []ConversationMessage{
		{Role: "user", Content: "loop forever"},
	}, "", "")
	if !handled {
		t.Fatal("expected structured loop to handle request")
	}
	if err == nil || !strings.Contains(err.Error(), "max iterations") {
		t.Fatalf("expected max iteration failure, got %v", err)
	}
}

func TestStructuredLoopSpawnSubagent(t *testing.T) {
	policy := ActiveBoundarySpec().StructuredToolPolicy
	policy.HighRiskDecision = string(structuredToolDecisionAllow)

	spawner := &stubSubagentSpawner{
		handle: SubagentJobHandle{
			JobID:   "job-42",
			Status:  "queued",
			Summary: "collect dependency graph",
		},
	}
	provider := &scriptedToolAwareProvider{
		name: "spawn-subagent",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-1",
						Name: "spawn_subagent",
						Arguments: map[string]any{
							"task": "collect dependency graph",
						},
					},
				},
			},
			{
				Content: "delegated work queued",
			},
		},
	}

	rt := NewRuntime(
		&runtimeServiceFake{},
		nil,
		WithWorkspaceRoot(t.TempDir()),
		WithMaxToolIterations(4),
		WithStructuredToolPolicy(policy),
		WithSubagentSpawner(spawner),
	)
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "spawn-subagent",
		Message:  "delegate dependency graph analysis",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "delegated work queued" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(spawner.calls) != 1 || spawner.calls[0].Task != "collect dependency graph" {
		t.Fatalf("expected subagent spawn call, got %+v", spawner.calls)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected tool follow-up request, got %d", len(provider.requests))
	}
	last := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
	if last.Role != "tool" || last.ToolName != "spawn_subagent" {
		t.Fatalf("unexpected tool callback message: %+v", last)
	}
	if !strings.Contains(last.Content, "job-42") || !strings.Contains(strings.ToLower(last.Content), "queued") {
		t.Fatalf("expected delegated job handle in tool output, got %+v", last)
	}
}

func TestStructuredLoopReadsSubagentResult(t *testing.T) {
	policy := ActiveBoundarySpec().StructuredToolPolicy
	policy.HighRiskDecision = string(structuredToolDecisionAllow)

	manager := NewInMemorySubagentManager(func(_ context.Context, req SubagentRequest) (string, error) {
		return "result: " + req.Task, nil
	})
	handle, err := manager.Spawn(context.Background(), SubagentRequest{Task: "collect dependency graph"})
	if err != nil {
		t.Fatalf("spawn delegated job: %v", err)
	}
	job := waitForSubagentJobState(t, manager, handle.JobID, SubagentJobStatusCompleted)

	provider := &scriptedToolAwareProvider{
		name: "delegate-followup",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-2",
						Name: "subagent_result",
						Arguments: map[string]any{
							"job_id": handle.JobID,
						},
					},
				},
			},
			{
				Content: "delegated result collected",
			},
		},
	}

	rt := NewRuntime(
		&runtimeServiceFake{},
		nil,
		WithWorkspaceRoot(t.TempDir()),
		WithMaxToolIterations(5),
		WithStructuredToolPolicy(policy),
		WithSubagentManager(manager),
	)
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "delegate-followup",
		Message:  "delegate dependency graph analysis and report back",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "delegated result collected" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected result follow-up requests, got %d", len(provider.requests))
	}

	if job.Status != SubagentJobStatusCompleted || job.Result != "result: collect dependency graph" {
		t.Fatalf("unexpected delegated job state: %+v", job)
	}

	last := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
	if last.Role != "tool" || last.ToolName != "subagent_result" {
		t.Fatalf("unexpected final tool callback message: %+v", last)
	}
	if !strings.Contains(last.Content, handle.JobID) || !strings.Contains(last.Content, "result: collect dependency graph") {
		t.Fatalf("expected delegated result in tool output, got %+v", last)
	}
}

func TestStructuredLoopReadsSubagentStatus(t *testing.T) {
	policy := ActiveBoundarySpec().StructuredToolPolicy
	policy.HighRiskDecision = string(structuredToolDecisionAllow)

	manager := NewInMemorySubagentManager(func(_ context.Context, req SubagentRequest) (string, error) {
		return "result: " + req.Task, nil
	})
	handle, err := manager.Spawn(context.Background(), SubagentRequest{Task: "collect dependency graph"})
	if err != nil {
		t.Fatalf("spawn delegated job: %v", err)
	}
	_ = waitForSubagentJobState(t, manager, handle.JobID, SubagentJobStatusCompleted)

	provider := &scriptedToolAwareProvider{
		name: "delegate-status",
		replies: []StructuredToolReply{
			{
				ToolCalls: []StructuredToolCall{
					{
						ID:   "call-2",
						Name: "subagent_status",
						Arguments: map[string]any{
							"job_id": handle.JobID,
						},
					},
				},
			},
			{
				Content: "delegated status collected",
			},
		},
	}

	rt := NewRuntime(
		&runtimeServiceFake{},
		nil,
		WithWorkspaceRoot(t.TempDir()),
		WithMaxToolIterations(5),
		WithStructuredToolPolicy(policy),
		WithSubagentManager(manager),
	)
	if err := rt.RegisterProvider(provider); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := rt.SetActiveProvider(provider.Name()); err != nil {
		t.Fatalf("set active provider: %v", err)
	}

	resp, err := rt.Chat(context.Background(), ChatRequest{
		Provider: "cli",
		ChatID:   "delegate-status",
		Message:  "check delegated dependency graph status",
	})
	if err != nil {
		t.Fatalf("runtime chat: %v", err)
	}
	if resp.Message != "delegated status collected" {
		t.Fatalf("unexpected chat response: %+v", resp)
	}

	last := provider.requests[1].Messages[len(provider.requests[1].Messages)-1]
	if last.Role != "tool" || last.ToolName != "subagent_status" {
		t.Fatalf("unexpected final tool callback message: %+v", last)
	}
	if !strings.Contains(last.Content, handle.JobID) || !strings.Contains(last.Content, "result: collect dependency graph") {
		t.Fatalf("expected delegated status in tool output, got %+v", last)
	}
}
