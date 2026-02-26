package baseagent

import (
	"context"
	"fmt"
	"strings"
)

// AgentLoop orchestrates tool routing, provider fallback, and session tracking.
type AgentLoop struct {
	svc       AgentService
	tools     *ToolRegistry
	providers *ProviderManager
	sessions  *SessionManager
	bus       *MessageBus
	channels  *ChannelManager
}

func NewAgentLoop(
	svc AgentService,
	tools *ToolRegistry,
	providers *ProviderManager,
	sessions *SessionManager,
	bus *MessageBus,
) *AgentLoop {
	if tools == nil {
		tools = NewToolRegistry()
	}
	if providers == nil {
		providers = NewProviderManager(nil)
	}
	if sessions == nil {
		sessions = NewSessionManager(0)
	}
	if bus == nil {
		bus = NewMessageBus(0, 0, 0)
	}
	return &AgentLoop{
		svc:       svc,
		tools:     tools,
		providers: providers,
		sessions:  sessions,
		bus:       bus,
	}
}

func (l *AgentLoop) SetChannelManager(cm *ChannelManager) {
	l.channels = cm
}

func (l *AgentLoop) ProcessChat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return ChatResponse{Message: baseAgentHelpText()}, nil
	}

	sessionKey := resolveSessionKey(req)
	channel := strings.TrimSpace(req.Provider)

	l.bus.PublishInbound(InboundEnvelope{
		Channel:    channel,
		ChatID:     strings.TrimSpace(req.ChatID),
		Content:    message,
		SessionKey: sessionKey,
		Metadata: map[string]string{
			"request_id": strings.TrimSpace(req.RequestID),
		},
	})
	l.sessions.AddMessage(sessionKey, "user", message)

	if resp, handled, err := l.tools.RouteMessage(ctx, message); handled {
		if err != nil {
			l.bus.PublishEvent(LoopEvent{
				Type:    EventError,
				Name:    "tool_route_failed",
				Message: err.Error(),
				Metadata: map[string]string{
					"request_id": strings.TrimSpace(req.RequestID),
				},
			})
			return ChatResponse{}, err
		}
		return l.finalizeResponse(sessionKey, req, resp), nil
	}

	// Best-effort fallback: if user mentioned a known agent ID, return status.
	if resp, ok := l.bestEffortAgentStatus(ctx, message); ok {
		return l.finalizeResponse(sessionKey, req, resp), nil
	}

	reply, err := l.providers.Reply(ctx, ProviderRequest{
		SystemPrompt: baseAgentSystemPrompt,
		UserMessage:  message,
		History:      l.sessions.History(sessionKey),
		Tools:        l.tools.ListToolDescriptors(),
	})
	if err != nil {
		l.bus.PublishEvent(LoopEvent{
			Type:    EventError,
			Name:    "provider_reply_failed",
			Message: err.Error(),
			Metadata: map[string]string{
				"request_id": strings.TrimSpace(req.RequestID),
			},
		})
		return ChatResponse{}, err
	}
	return l.finalizeResponse(sessionKey, req, ChatResponse{
		Message: strings.TrimSpace(reply),
		Action:  "chat",
	}), nil
}

func (l *AgentLoop) bestEffortAgentStatus(ctx context.Context, rawMessage string) (ChatResponse, bool) {
	if l.svc == nil {
		return ChatResponse{}, false
	}
	lower := strings.ToLower(rawMessage)
	for _, s := range l.svc.ListAgents() {
		if !strings.Contains(lower, strings.ToLower(s.ID)) {
			continue
		}
		resp, err := l.tools.ExecuteTool(ctx, "agent_action", map[string]string{
			"action":   "status",
			"agent_id": s.ID,
		})
		if err != nil {
			return ChatResponse{}, false
		}
		if strings.TrimSpace(resp.Action) == "" {
			resp.Action = "status"
		}
		return resp, true
	}
	return ChatResponse{}, false
}

func (l *AgentLoop) finalizeResponse(sessionKey string, req ChatRequest, resp ChatResponse) ChatResponse {
	if strings.TrimSpace(resp.Message) == "" {
		resp.Message = "Done."
	}
	l.sessions.AddMessage(sessionKey, "assistant", resp.Message)

	channel := strings.TrimSpace(req.Provider)
	if !isInternalChannelName(channel) {
		l.bus.PublishOutbound(OutboundEnvelope{
			Channel: channel,
			ChatID:  strings.TrimSpace(req.ChatID),
			Content: resp.Message,
			Metadata: map[string]string{
				"request_id": strings.TrimSpace(req.RequestID),
				"action":     strings.TrimSpace(resp.Action),
			},
		})
	}
	l.bus.PublishEvent(LoopEvent{
		Type:    EventOutbound,
		Name:    "chat_response",
		Message: fmt.Sprintf("action=%s", strings.TrimSpace(resp.Action)),
		Metadata: map[string]string{
			"request_id": strings.TrimSpace(req.RequestID),
			"session":    sessionKey,
		},
	})
	return resp
}

func resolveSessionKey(req ChatRequest) string {
	channel := strings.ToLower(strings.TrimSpace(req.Provider))
	if channel == "" {
		channel = "chat"
	}
	chatID := strings.TrimSpace(req.ChatID)
	if chatID == "" {
		chatID = "default"
	}
	return channel + ":" + chatID
}
