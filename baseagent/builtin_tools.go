package baseagent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var slashAgentActionPattern = regexp.MustCompile(`(?i)^/(uninstall|start|stop|status|logs|upgrade|diagnose)\s+([a-zA-Z0-9][a-zA-Z0-9._-]*)\s*$`)
var agentActionPattern = regexp.MustCompile(`(?i)\b(uninstall|start|stop|status|logs|upgrade|diagnose)\s+([a-zA-Z0-9][a-zA-Z0-9._-]*)\b`)

func mustRegisterTool(registry *ToolRegistry, spec ToolSpec) {
	if registry == nil {
		panic("baseagent: tool registry is nil")
	}
	if err := registry.RegisterTool(spec); err != nil {
		panic(fmt.Sprintf("baseagent: register built-in tool %q failed: %v", strings.TrimSpace(spec.Name), err))
	}
}

func newBuiltinToolRegistry(rt *Runtime, providers *ProviderManager, sessions *SessionManager) *ToolRegistry {
	registry := NewToolRegistry()

	mustRegisterTool(registry, ToolSpec{
		Name:        "list_agents",
		Description: "List currently registered agents and health/runtime status.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Match: func(input string) (ToolInvocation, bool) {
			lower := strings.ToLower(strings.TrimSpace(input))
			if lower == "/agents" || lower == "/list agents" || lower == "/agents list" {
				return ToolInvocation{Name: "list_agents"}, true
			}
			if wantsListAgents(lower) {
				return ToolInvocation{Name: "list_agents"}, true
			}
			return ToolInvocation{}, false
		},
		Run: func(_ context.Context, _ ToolInvocation) (ChatResponse, error) {
			if rt == nil || rt.svc == nil {
				return ChatResponse{Message: "Agent service is unavailable.", Action: "list_agents"}, nil
			}
			return ChatResponse{
				Message: renderAgentList(rt.svc.ListAgents()),
				Action:  "list_agents",
			}, nil
		},
	})

	mustRegisterTool(registry, ToolSpec{
		Name:        "agent_action",
		Description: "Run an agent lifecycle action (start/stop/status/logs/upgrade/diagnose/uninstall).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type": "string",
					"enum": []string{"start", "stop", "status", "logs", "upgrade", "diagnose", "uninstall"},
				},
				"agent_id": map[string]any{
					"type": "string",
				},
			},
			"required": []string{"action", "agent_id"},
		},
		Match: func(input string) (ToolInvocation, bool) {
			return matchAgentActionInvocation(input)
		},
		Run: func(ctx context.Context, call ToolInvocation) (ChatResponse, error) {
			action := strings.ToLower(strings.TrimSpace(call.Args["action"]))
			agentID := strings.TrimSpace(call.Args["agent_id"])
			if action == "" || agentID == "" {
				return ChatResponse{}, fmt.Errorf("missing action or agent id")
			}
			if rt == nil {
				return ChatResponse{Message: fmt.Sprintf("%s %s failed: runtime unavailable", action, agentID), Action: action}, nil
			}
			resp, err := rt.executeAgentAction(ctx, action, agentID)
			if err != nil {
				return ChatResponse{
					Message: fmt.Sprintf("%s %s failed: %v", action, agentID, err),
					Action:  action,
				}, nil
			}
			return resp, nil
		},
	})

	mustRegisterTool(registry, ToolSpec{
		Name:        "help",
		Description: "Show base-agent usage commands.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Match: func(input string) (ToolInvocation, bool) {
			lower := strings.ToLower(strings.TrimSpace(input))
			switch lower {
			case "/help", "/?", "help", "what can you do", "what can you do?", "show help":
				return ToolInvocation{Name: "help"}, true
			}
			return ToolInvocation{}, false
		},
		Run: func(_ context.Context, _ ToolInvocation) (ChatResponse, error) {
			return ChatResponse{Message: baseAgentHelpText(), Action: "help"}, nil
		},
	})

	mustRegisterTool(registry, ToolSpec{
		Name:        "list_tools",
		Description: "List internal tool capabilities available to base-agent.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Match: func(input string) (ToolInvocation, bool) {
			lower := strings.ToLower(strings.TrimSpace(input))
			if lower == "/tools" || lower == "list tools" || lower == "show tools" {
				return ToolInvocation{Name: "list_tools"}, true
			}
			return ToolInvocation{}, false
		},
		Run: func(_ context.Context, _ ToolInvocation) (ChatResponse, error) {
			summary := registry.RenderToolSummary()
			if rt != nil {
				summary = rt.renderToolSummary()
			}
			return ChatResponse{
				Message: summary,
				Action:  "tools",
			}, nil
		},
	})

	mustRegisterTool(registry, ToolSpec{
		Name:        "list_providers",
		Description: "List configured provider backends and current active provider.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Match: func(input string) (ToolInvocation, bool) {
			lower := strings.ToLower(strings.TrimSpace(input))
			if lower == "/providers" || lower == "list providers" || lower == "show providers" {
				return ToolInvocation{Name: "list_providers"}, true
			}
			return ToolInvocation{}, false
		},
		Run: func(_ context.Context, _ ToolInvocation) (ChatResponse, error) {
			if providers == nil {
				return ChatResponse{Message: "No providers are configured.", Action: "providers"}, nil
			}
			names := providers.ListProviders()
			if len(names) == 0 {
				return ChatResponse{Message: "No providers are configured.", Action: "providers"}, nil
			}
			active := providers.ActiveProviderName()
			lines := make([]string, 0, len(names)+2)
			lines = append(lines, fmt.Sprintf("Active provider: %s", active))
			lines = append(lines, "Registered providers:")
			for _, name := range names {
				if name == active {
					lines = append(lines, "- "+name+" (active)")
					continue
				}
				lines = append(lines, "- "+name)
			}
			return ChatResponse{Message: strings.Join(lines, "\n"), Action: "providers"}, nil
		},
	})

	mustRegisterTool(registry, ToolSpec{
		Name:        "list_sessions",
		Description: "Show recent chat sessions and compaction state.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Match: func(input string) (ToolInvocation, bool) {
			lower := strings.ToLower(strings.TrimSpace(input))
			if lower == "/sessions" || lower == "list sessions" || lower == "show sessions" {
				return ToolInvocation{Name: "list_sessions"}, true
			}
			return ToolInvocation{}, false
		},
		Run: func(_ context.Context, _ ToolInvocation) (ChatResponse, error) {
			if sessions == nil {
				return ChatResponse{Message: "No session manager is configured.", Action: "sessions"}, nil
			}
			stats := sessions.ListStats(8)
			if len(stats) == 0 {
				return ChatResponse{Message: "No sessions recorded yet.", Action: "sessions"}, nil
			}
			lines := make([]string, 0, len(stats)+1)
			lines = append(lines, fmt.Sprintf("Recent sessions (%d):", len(stats)))
			for _, s := range stats {
				lines = append(lines, fmt.Sprintf(
					"- %s: messages=%d summary=%d updated=%s",
					s.Key,
					s.MessageCount,
					s.SummaryLength,
					s.UpdatedAt.UTC().Format(time.RFC3339),
				))
			}
			return ChatResponse{Message: strings.Join(lines, "\n"), Action: "sessions"}, nil
		},
	})

	mustRegisterTool(registry, ToolSpec{
		Name:        "show_boundaries",
		Description: "Explain base-agent capability boundaries, sources, and design rationale.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Match: func(input string) (ToolInvocation, bool) {
			lower := strings.ToLower(strings.TrimSpace(input))
			if lower == "/boundaries" || lower == "show boundaries" || lower == "list boundaries" || lower == "what are your boundaries" {
				return ToolInvocation{Name: "show_boundaries"}, true
			}
			return ToolInvocation{}, false
		},
		Run: func(_ context.Context, _ ToolInvocation) (ChatResponse, error) {
			return ChatResponse{
				Message: baseAgentBoundariesText(),
				Action:  "boundaries",
			}, nil
		},
	})

	return registry
}

func matchAgentActionInvocation(input string) (ToolInvocation, bool) {
	trimmed := strings.TrimSpace(input)
	if matches := slashAgentActionPattern.FindStringSubmatch(trimmed); len(matches) == 3 {
		return ToolInvocation{
			Name: "agent_action",
			Args: map[string]string{
				"action":   strings.ToLower(strings.TrimSpace(matches[1])),
				"agent_id": strings.TrimSpace(matches[2]),
			},
		}, true
	}
	if matches := agentActionPattern.FindStringSubmatch(trimmed); len(matches) == 3 {
		return ToolInvocation{
			Name: "agent_action",
			Args: map[string]string{
				"action":   strings.ToLower(strings.TrimSpace(matches[1])),
				"agent_id": strings.TrimSpace(matches[2]),
			},
		}, true
	}
	return ToolInvocation{}, false
}
