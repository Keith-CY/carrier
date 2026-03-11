package baseagent

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestParseStructuredToolReplyRejectsMalformedJSON(t *testing.T) {
	_, err := parseStructuredToolReply(`{"tool_calls":[{"name":"write_file","arguments":{"path":"x.txt"}}`)
	if err == nil {
		t.Fatal("expected malformed structured reply to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "parse structured tool reply") {
		t.Fatalf("unexpected malformed structured reply error: %v", err)
	}
}

func TestParseStructuredToolReplyHandlesFencedJSONAndDefaultCallID(t *testing.T) {
	reply, err := parseStructuredToolReply("```json\n{\"tool_calls\":[{\"name\":\"write_file\",\"arguments\":{\"path\":\"x.txt\",\"content\":\"hello\"}}]}\n```")
	if err != nil {
		t.Fatalf("parse structured tool reply: %v", err)
	}
	if len(reply.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %+v", reply)
	}
	if reply.ToolCalls[0].ID != "call-1" {
		t.Fatalf("expected default call id, got %+v", reply.ToolCalls[0])
	}
}

func TestToolRegistryConcurrentAccess(t *testing.T) {
	reg := NewToolRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "tool-" + string(rune('a'+i%8))
			_ = reg.RegisterTool(ToolSpec{
				Name:        name,
				Description: "concurrent test tool",
				Match:       func(string) (ToolInvocation, bool) { return ToolInvocation{}, false },
				Run: func(context.Context, ToolInvocation) (ChatResponse, error) {
					return ChatResponse{Message: "ok"}, nil
				},
			})
			_ = reg.ListToolNames()
			_ = reg.ListToolDescriptors()
			_ = reg.RenderToolSummary()
			_, _ = reg.ExecuteTool(context.Background(), name, map[string]string{})
		}(i)
	}

	wg.Wait()
	if len(reg.ListToolNames()) == 0 {
		t.Fatal("expected tools to remain registered after concurrent access")
	}
}

func TestBuildStructuredToolPromptIncludesToolResultMetadata(t *testing.T) {
	prompt := buildStructuredToolPrompt(StructuredToolRequest{
		SystemPrompt: "system",
		Messages: []StructuredToolMessage{
			{
				Role:             "tool",
				Content:          "tool exec requires confirmation before automatic structured execution",
				ToolCallID:       "call-1",
				ToolName:         "exec",
				ToolResultStatus: ExecutionToolResultStatusAsk,
			},
		},
	})

	if !strings.Contains(prompt, "tool(call-1)[ask][exec]: tool exec requires confirmation before automatic structured execution") {
		t.Fatalf("expected prompt to include structured tool result metadata, got %q", prompt)
	}
}

func TestBuildStructuredToolPromptIncludesStructuredHistoryMetadata(t *testing.T) {
	sessions := NewSessionManager(8)
	sessionKey := "cli:structured-prompt"

	sessions.AddMessage(sessionKey, "user", "run the tests")
	sessions.AddStructuredToolMessage(sessionKey, StructuredToolMessage{
		Role:             "tool",
		Content:          "tool exec requires confirmation before automatic structured execution",
		ToolCallID:       "call-1",
		ToolName:         "exec",
		ToolResultStatus: ExecutionToolResultStatusAsk,
	})

	prompt := buildStructuredToolPrompt(StructuredToolRequest{
		SystemPrompt: "system",
		Messages:     sessions.StructuredHistory(sessionKey),
	})

	if !strings.Contains(prompt, "tool(call-1)[ask][exec]: tool exec requires confirmation before automatic structured execution") {
		t.Fatalf("expected prompt to include structured history metadata, got %q", prompt)
	}
}

func TestStructuredToolPromptIncludesPolicyReasonMetadata(t *testing.T) {
	prompt := buildStructuredToolPrompt(StructuredToolRequest{
		SystemPrompt: "system",
		Messages: []StructuredToolMessage{
			{
				Role:             "tool",
				Content:          "command denied by execution safety policy",
				ToolCallID:       "call-1",
				ToolName:         "exec",
				ToolResultStatus: ExecutionToolResultStatusDeny,
				ToolPolicyRuleID: "exec.blocked_command",
				ToolPolicyReason: "Command matches blocked execution policy.",
			},
		},
	})

	if !strings.Contains(prompt, "[rule=exec.blocked_command]") {
		t.Fatalf("expected prompt to include rule id metadata, got %q", prompt)
	}
	if !strings.Contains(prompt, "[reason=Command matches blocked execution policy.]") {
		t.Fatalf("expected prompt to include policy reason metadata, got %q", prompt)
	}
}
