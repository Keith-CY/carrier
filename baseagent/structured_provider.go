package baseagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type StructuredToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type StructuredToolMessage struct {
	Role             string                    `json:"role"`
	Content          string                    `json:"content,omitempty"`
	Attachments      []AttachmentRef           `json:"attachments,omitempty"`
	ContentBlocks    []ContentBlock            `json:"contentBlocks,omitempty"`
	ToolCallID       string                    `json:"toolCallId,omitempty"`
	ToolName         string                    `json:"toolName,omitempty"`
	ToolResultStatus ExecutionToolResultStatus `json:"toolResultStatus,omitempty"`
	ToolPolicyReason string                    `json:"toolPolicyReason,omitempty"`
	ToolPolicyRuleID string                    `json:"toolPolicyRuleId,omitempty"`
	ToolCalls        []StructuredToolCall      `json:"toolCalls,omitempty"`
}

type StructuredToolRequest struct {
	SystemPrompt string                     `json:"systemPrompt"`
	Messages     []StructuredToolMessage    `json:"messages"`
	Tools        []StructuredToolDescriptor `json:"tools"`
}

type StructuredToolReply struct {
	Content   string               `json:"content"`
	ToolCalls []StructuredToolCall `json:"tool_calls,omitempty"`
}

type ToolAwareProvider interface {
	Provider
	ReplyWithTools(ctx context.Context, req StructuredToolRequest) (StructuredToolReply, error)
}

func replyWithToolsViaTextProvider(ctx context.Context, provider Provider, req StructuredToolRequest) (StructuredToolReply, error) {
	if provider == nil {
		return StructuredToolReply{}, fmt.Errorf("provider is required")
	}

	raw, err := provider.Reply(ctx, ProviderRequest{
		SystemPrompt: req.SystemPrompt,
		UserMessage:  buildStructuredToolPrompt(req),
	})
	if err != nil {
		return StructuredToolReply{}, err
	}
	return parseStructuredToolReply(raw)
}

func buildStructuredToolPrompt(req StructuredToolRequest) string {
	lines := []string{
		"Return JSON only.",
		`Use this exact shape: {"content":"assistant text","tool_calls":[{"id":"call-1","name":"tool_name","arguments":{}}]}`,
		"If no tool is needed, return an empty tool_calls array.",
		"Do not wrap JSON in markdown fences.",
	}
	if len(req.Tools) > 0 {
		lines = append(lines, "", "Available tools:")
		for _, tool := range req.Tools {
			params := "{}"
			if len(tool.Parameters) > 0 {
				if raw, err := json.Marshal(tool.Parameters); err == nil {
					params = string(raw)
				}
			}
			lines = append(lines, fmt.Sprintf("- %s: %s | parameters=%s", tool.Name, tool.Description, params))
		}
	}
	lines = append(lines, "", "Conversation:")
	for _, msg := range req.Messages {
		if msg.ToolCallID != "" {
			lines = append(lines, renderStructuredToolPromptMessage(msg))
			continue
		}
		if len(msg.ToolCalls) > 0 {
			raw, _ := json.Marshal(msg.ToolCalls)
			lines = append(lines, fmt.Sprintf("%s: %s | tool_calls=%s", msg.Role, msg.Content, string(raw)))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", msg.Role, msg.Content))
	}
	return strings.Join(lines, "\n")
}

func renderStructuredToolPromptMessage(msg StructuredToolMessage) string {
	suffix := ""
	if status := strings.TrimSpace(string(msg.ToolResultStatus)); status != "" {
		suffix += fmt.Sprintf("[%s]", status)
	}
	if name := strings.TrimSpace(msg.ToolName); name != "" {
		suffix += fmt.Sprintf("[%s]", name)
	}
	if ruleID := strings.TrimSpace(msg.ToolPolicyRuleID); ruleID != "" {
		suffix += fmt.Sprintf("[rule=%s]", ruleID)
	}
	if reason := strings.TrimSpace(msg.ToolPolicyReason); reason != "" {
		suffix += fmt.Sprintf("[reason=%s]", reason)
	}
	return fmt.Sprintf("%s(%s)%s: %s", msg.Role, msg.ToolCallID, suffix, msg.Content)
}

func parseStructuredToolReply(raw string) (StructuredToolReply, error) {
	trimmed := strings.TrimSpace(raw)
	candidate := strings.TrimSpace(extractJSONCandidate(raw))
	if candidate == "" {
		if looksLikeStructuredReply(trimmed) {
			return StructuredToolReply{}, fmt.Errorf("parse structured tool reply: invalid json payload")
		}
		return StructuredToolReply{Content: trimmed}, nil
	}

	var reply StructuredToolReply
	if err := json.Unmarshal([]byte(candidate), &reply); err != nil {
		return StructuredToolReply{}, fmt.Errorf("parse structured tool reply: %w", err)
	}
	reply.Content = strings.TrimSpace(reply.Content)
	for i := range reply.ToolCalls {
		call := &reply.ToolCalls[i]
		call.ID = strings.TrimSpace(call.ID)
		call.Name = strings.TrimSpace(call.Name)
		if call.Arguments == nil {
			call.Arguments = map[string]any{}
		}
		if call.ID == "" {
			call.ID = fmt.Sprintf("call-%d", i+1)
		}
	}
	return reply, nil
}

func looksLikeStructuredReply(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	return strings.HasPrefix(raw, "{") ||
		strings.HasPrefix(raw, "[") ||
		strings.HasPrefix(raw, "```") ||
		strings.Contains(raw, `"tool_calls"`)
}
