package baseagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// LLMProviderAdapter wraps baseagent requestLLMCompletion as a provider.
type LLMProviderAdapter struct {
	name          string
	historyWindow int
}

func NewLLMProviderAdapter(name string, historyWindow int) *LLMProviderAdapter {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "llm"
	}
	if historyWindow <= 0 {
		historyWindow = 8
	}
	return &LLMProviderAdapter{name: name, historyWindow: historyWindow}
}

func (p *LLMProviderAdapter) Name() string {
	return p.name
}

func (p *LLMProviderAdapter) Reply(ctx context.Context, req ProviderRequest) (string, error) {
	user := strings.TrimSpace(req.UserMessage)
	if user == "" {
		return "", fmt.Errorf("empty user message")
	}

	prompt := user
	if history := renderProviderHistory(req.History, p.historyWindow); history != "" {
		prompt = "Conversation context:\n" + history + "\n\nCurrent user message:\n" + user
	}
	toolLines := renderStructuredToolLines(req.StructuredTools)
	if len(toolLines) == 0 {
		toolLines = renderCompactToolLines(req.Tools)
	}
	if len(toolLines) > 0 {
		prompt += "\n\nAvailable built-in tools:\n" + strings.Join(toolLines, "\n")
	}

	return requestLLMCompletionForProvider(ctx, p.name, req.SystemPrompt, prompt)
}

func renderCompactToolLines(tools []ToolDescriptor) []string {
	lines := make([]string, 0, len(tools))
	for _, t := range tools {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		desc := strings.TrimSpace(t.Description)
		if desc == "" {
			lines = append(lines, "- "+name)
		} else {
			lines = append(lines, "- "+name+": "+desc)
		}
	}
	return lines
}

func renderStructuredToolLines(tools []StructuredToolDescriptor) []string {
	lines := make([]string, 0, len(tools))
	for _, t := range tools {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		line := "- " + name
		desc := strings.TrimSpace(t.Description)
		if desc != "" {
			line += ": " + desc
		}
		if len(t.Parameters) > 0 {
			if raw, err := json.Marshal(t.Parameters); err == nil {
				line += " | parameters=" + string(raw)
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func renderProviderHistory(history []ConversationMessage, window int) string {
	if len(history) == 0 {
		return ""
	}
	if window <= 0 {
		window = len(history)
	}
	start := len(history) - window
	if start < 0 {
		start = 0
	}
	lines := make([]string, 0, len(history[start:]))
	for _, msg := range history[start:] {
		role := strings.TrimSpace(msg.Role)
		content := strings.TrimSpace(msg.Content)
		if role == "" || content == "" {
			continue
		}
		if len(content) > maxProviderHistoryMessageLength {
			content = content[:maxProviderHistoryMessageLength] + "..."
		}
		lines = append(lines, role+": "+content)
	}
	return strings.Join(lines, "\n")
}
