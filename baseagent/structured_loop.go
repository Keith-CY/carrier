package baseagent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func buildStructuredSystemPrompt(skillSummary string) string {
	return composeSkillAwareSystemPrompt(baseAgentExecutionPrompt, skillSummary)
}

func structuredToolMessagesFromHistory(summary string, history []StructuredToolMessage) []StructuredToolMessage {
	out := make([]StructuredToolMessage, 0, len(history)+1)
	if trimmed := strings.TrimSpace(summary); trimmed != "" {
		out = append(out, StructuredToolMessage{
			Role:    "system",
			Content: "Session summary:\n" + trimmed,
		})
	}
	for _, msg := range history {
		msg = normalizeStructuredToolMessage(msg)
		if msg.Role == "" {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func renderStructuredToolCallSummary(calls []StructuredToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "Tool call requested"
	}
	return "Tool call requested: " + strings.Join(names, ", ")
}

func normalizeExecutionToolResultStatus(result ExecutionToolResult) ExecutionToolResultStatus {
	status := result.Status
	if status != "" {
		return status
	}
	if result.IsError {
		return ExecutionToolResultStatusError
	}
	return ExecutionToolResultStatusOK
}

func renderStructuredToolResultOutput(toolName string, result ExecutionToolResult) string {
	toolOutput := strings.TrimSpace(result.Output)
	if toolOutput == "" {
		toolOutput = strings.TrimSpace(result.Stdout)
	}
	if toolOutput == "" && strings.TrimSpace(result.Stderr) != "" {
		toolOutput = strings.TrimSpace(result.Stderr)
	}
	if toolOutput == "" {
		toolOutput = fmt.Sprintf("tool %s completed", strings.TrimSpace(toolName))
	}
	return toolOutput
}

func formatPendingApprovalToolOutput(base string, pending *PendingToolApproval) string {
	base = strings.TrimSpace(base)
	if pending == nil {
		return base
	}
	line := fmt.Sprintf("Reply with 'confirm' to continue or 'cancel' to decline. approval_id=%s", strings.TrimSpace(pending.ID))
	if base == "" {
		return line
	}
	return base + "\n" + line
}

func shouldObserveStructuredToolResult(toolName string, status ExecutionToolResultStatus) bool {
	if status != ExecutionToolResultStatusOK {
		return false
	}
	switch strings.TrimSpace(toolName) {
	case "", "memory_search":
		return false
	default:
		return true
	}
}

func (l *AgentLoop) observeStructuredToolResult(toolName string, result ExecutionToolResult) {
	if l == nil || l.memory == nil {
		return
	}
	status := normalizeExecutionToolResultStatus(result)
	if !shouldObserveStructuredToolResult(toolName, status) {
		return
	}
	output := strings.TrimSpace(renderStructuredToolResultOutput(toolName, result))
	if output == "" {
		return
	}
	if _, err := observeMemoryStore(l.memory, l.memorySubject, strings.TrimSpace(toolName), output, ""); err != nil {
		return
	}
}

func (l *AgentLoop) SetExecutionTools(registry *ExecutionToolRegistry, maxIterations int, policySpec StructuredToolPolicySpec, mcpManager MCPManager, subagentManager SubagentManager) {
	l.executionTools = registry
	l.subagentManager = subagentManager
	l.structuredTools = newStructuredToolSurfaceWithPolicy(l.tools, registry, mcpManager, subagentManager, policySpec)
	if l.memory != nil && l.structuredTools != nil {
		l.structuredTools.SetMemoryStore(l.memory, l.memorySubject)
	}
	if maxIterations <= 0 {
		maxIterations = 6
	}
	l.maxToolIterations = maxIterations
}

func (l *AgentLoop) processStructuredChat(
	ctx context.Context,
	sessionKey string,
	legacyHistory []ConversationMessage,
	skillSummary string,
) (ChatResponse, bool, error) {
	if l == nil || l.structuredTools == nil || l.providers == nil {
		return ChatResponse{}, false, nil
	}

	summary := ""
	var structuredHistory []StructuredToolMessage
	if l.sessions != nil {
		summary = l.sessions.Summary(sessionKey)
		structuredHistory = l.sessions.StructuredHistory(sessionKey)
	}
	if len(structuredHistory) == 0 {
		structuredHistory = structuredMessagesFromConversationHistory(legacyHistory)
	}
	messages := structuredToolMessagesFromHistory(summary, structuredHistory)
	turnLimit := l.maxToolIterations
	if turnLimit <= 0 {
		turnLimit = 6
	}

	for turn := 0; turn < turnLimit; turn++ {
		reply, err := l.providers.ReplyWithTools(ctx, StructuredToolRequest{
			SystemPrompt: buildStructuredSystemPrompt(skillSummary),
			Messages:     messages,
			Tools:        l.structuredTools.Descriptors(),
		})
		if err != nil {
			return ChatResponse{}, true, err
		}

		if len(reply.ToolCalls) == 0 {
			return ChatResponse{
				Message: strings.TrimSpace(reply.Content),
				Action:  "chat",
			}, true, nil
		}

		assistantContent := strings.TrimSpace(reply.Content)
		if assistantContent == "" {
			assistantContent = renderStructuredToolCallSummary(reply.ToolCalls)
		}
		messages = append(messages, StructuredToolMessage{
			Role:      "assistant",
			Content:   assistantContent,
			ToolCalls: reply.ToolCalls,
		})
		if l.sessions != nil {
			l.sessions.AddStructuredToolMessage(sessionKey, StructuredToolMessage{
				Role:      "assistant",
				Content:   assistantContent,
				ToolCalls: reply.ToolCalls,
			})
		}

		for _, call := range reply.ToolCalls {
			result := l.structuredTools.Execute(ctx, call.Name, call.Arguments)
			status := normalizeExecutionToolResultStatus(result)
			toolOutput := renderStructuredToolResultOutput(call.Name, result)
			l.observeStructuredToolResult(call.Name, result)
			if status == ExecutionToolResultStatusAsk && l.sessions != nil {
				requestedAt := time.Now().UTC()
				pending := &PendingToolApproval{
					ID:          fmt.Sprintf("approval-%d", requestedAt.UnixNano()),
					ToolName:    strings.TrimSpace(call.Name),
					Arguments:   cloneToolSchema(call.Arguments),
					RequestedAt: requestedAt,
					ExpiresAt:   requestedAt.Add(defaultPendingApprovalTTL),
					Reason:      strings.TrimSpace(result.PolicyReason),
					RuleID:      strings.TrimSpace(result.PolicyRuleID),
				}
				l.sessions.SetPendingApproval(sessionKey, pending)
				toolOutput = formatPendingApprovalToolOutput(toolOutput, pending)
			}
			messages = append(messages, StructuredToolMessage{
				Role:             "tool",
				Content:          toolOutput,
				Attachments:      cloneAttachmentRefs(result.Attachments),
				ContentBlocks:    cloneContentBlocks(result.ContentBlocks),
				ToolCallID:       call.ID,
				ToolName:         strings.TrimSpace(call.Name),
				ToolResultStatus: status,
				ToolPolicyReason: strings.TrimSpace(result.PolicyReason),
				ToolPolicyRuleID: strings.TrimSpace(result.PolicyRuleID),
			})
			if l.sessions != nil {
				l.sessions.AddStructuredToolMessage(sessionKey, StructuredToolMessage{
					Role:             "tool",
					Content:          toolOutput,
					Attachments:      cloneAttachmentRefs(result.Attachments),
					ContentBlocks:    cloneContentBlocks(result.ContentBlocks),
					ToolCallID:       call.ID,
					ToolName:         strings.TrimSpace(call.Name),
					ToolResultStatus: status,
					ToolPolicyReason: strings.TrimSpace(result.PolicyReason),
					ToolPolicyRuleID: strings.TrimSpace(result.PolicyRuleID),
				})
			}
		}
	}

	return ChatResponse{}, true, fmt.Errorf("structured tool loop exceeded max iterations")
}
