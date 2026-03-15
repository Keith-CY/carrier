package baseagent

import (
	"context"
	"fmt"
	"strings"
)

// AgentLoop orchestrates tool routing, provider fallback, and session tracking.
type AgentLoop struct {
	svc               AgentService
	tools             *ToolRegistry
	providers         *ProviderManager
	sessions          *SessionManager
	bus               *MessageBus
	channels          *ChannelManager
	memory            ExtendedMemoryStore
	memorySubject     string
	executionTools    *ExecutionToolRegistry
	structuredTools   *structuredToolSurface
	subagentManager   SubagentManager
	maxToolIterations int
	skillsLoader      SkillsLoader
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
		svc:               svc,
		tools:             tools,
		providers:         providers,
		sessions:          sessions,
		bus:               bus,
		maxToolIterations: defaultMaxToolIterations,
	}
}

func (l *AgentLoop) SetChannelManager(cm *ChannelManager) {
	l.channels = cm
}

func (l *AgentLoop) SetSkillsLoader(loader SkillsLoader) {
	l.skillsLoader = loader
}

func (l *AgentLoop) SetMemoryStore(store MemoryStore, subject string) {
	if l == nil {
		return
	}
	extended, _ := store.(ExtendedMemoryStore)
	l.memory = extended
	l.memorySubject = strings.TrimSpace(subject)
	if l.memorySubject == "" {
		l.memorySubject = baseAgentVirtualID
	}
	if l.structuredTools != nil {
		l.structuredTools.SetMemoryStore(extended, l.memorySubject)
	}
}

func (l *AgentLoop) resolvedMemorySubject(override string) string {
	subject := strings.TrimSpace(override)
	if subject != "" {
		return subject
	}
	subject = strings.TrimSpace(l.memorySubject)
	if subject == "" {
		subject = baseAgentVirtualID
	}
	return subject
}

func (l *AgentLoop) structuredToolSurfaceForSubject(subject string) *structuredToolSurface {
	if l == nil || l.structuredTools == nil {
		return nil
	}
	resolved := l.resolvedMemorySubject(subject)
	if l.memory == nil || resolved == l.resolvedMemorySubject("") {
		return l.structuredTools
	}
	cloned := l.structuredTools.clone()
	cloned.SetMemoryStore(l.memory, resolved)
	return cloned
}

func (l *AgentLoop) ProcessChat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return ChatResponse{Message: baseAgentHelpText()}, nil
	}

	channel := strings.TrimSpace(req.Provider)
	chatID := strings.TrimSpace(req.ChatID)
	requestID := strings.TrimSpace(req.RequestID)
	sessionKey := resolveSessionKey(channel, chatID)

	l.bus.PublishInbound(InboundEnvelope{
		Channel:     channel,
		ChatID:      chatID,
		Content:     message,
		Attachments: cloneAttachmentRefs(req.Attachments),
		SessionKey:  sessionKey,
		Metadata: map[string]string{
			"request_id": requestID,
		},
	})
	l.sessions.AddMessage(sessionKey, "user", message)
	skillSummary := ""
	if l.skillsLoader != nil {
		skillSummary = strings.TrimSpace(l.skillsLoader.RelevantSkillsSummary(ctx, message))
	}
	memorySubject := l.resolvedMemorySubject(req.MemorySubject)

	if resp, handled, err := l.handlePendingApprovalInput(ctx, sessionKey, message); handled {
		if err != nil {
			l.bus.PublishEvent(LoopEvent{
				Type:    EventError,
				Name:    "pending_approval_failed",
				Message: err.Error(),
				Metadata: map[string]string{
					"request_id": requestID,
				},
			})
			return ChatResponse{}, err
		}
		return l.finalizeResponse(sessionKey, channel, chatID, requestID, resp), nil
	}

	if resp, handled, err := l.tools.RouteMessage(ctx, message); handled {
		if err != nil {
			l.bus.PublishEvent(LoopEvent{
				Type:    EventError,
				Name:    "tool_route_failed",
				Message: err.Error(),
				Metadata: map[string]string{
					"request_id": requestID,
				},
			})
			return ChatResponse{}, err
		}
		return l.finalizeResponse(sessionKey, channel, chatID, requestID, resp), nil
	}

	// Best-effort fallback: if user mentioned a known agent ID, return status.
	if resp, ok := l.bestEffortAgentStatus(ctx, message); ok {
		return l.finalizeResponse(sessionKey, channel, chatID, requestID, resp), nil
	}

	if resp, handled, err := l.processStructuredChat(ctx, sessionKey, l.sessions.History(sessionKey), skillSummary, memorySubject); handled {
		if err != nil {
			l.bus.PublishEvent(LoopEvent{
				Type:    EventError,
				Name:    "structured_tool_loop_failed",
				Message: err.Error(),
				Metadata: map[string]string{
					"request_id": requestID,
				},
			})
			return ChatResponse{}, err
		}
		return l.finalizeResponse(sessionKey, channel, chatID, requestID, resp), nil
	}

	reply, err := l.providers.Reply(ctx, ProviderRequest{
		SystemPrompt:    composeSkillAwareSystemPrompt(baseAgentSystemPrompt, skillSummary),
		UserMessage:     message,
		History:         l.sessions.History(sessionKey),
		Tools:           l.tools.ListToolDescriptors(),
		StructuredTools: l.tools.ListStructuredToolDescriptors(),
	})
	if err != nil {
		l.bus.PublishEvent(LoopEvent{
			Type:    EventError,
			Name:    "provider_reply_failed",
			Message: err.Error(),
			Metadata: map[string]string{
				"request_id": requestID,
			},
		})
		return ChatResponse{}, err
	}
	return l.finalizeResponse(sessionKey, channel, chatID, requestID, ChatResponse{
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

func (l *AgentLoop) handlePendingApprovalInput(ctx context.Context, sessionKey, message string) (ChatResponse, bool, error) {
	if l == nil || l.sessions == nil {
		return ChatResponse{}, false, nil
	}
	pending := l.sessions.PendingApproval(sessionKey)
	if pending == nil {
		return ChatResponse{}, false, nil
	}

	switch {
	case isApprovalRejection(message):
		resp, err := l.RespondPendingApproval(ctx, sessionKey, pending.ID, ApprovalDecisionReject)
		return resp, true, err
	case !isApprovalConfirmation(message):
		return ChatResponse{}, false, nil
	default:
		resp, err := l.RespondPendingApproval(ctx, sessionKey, pending.ID, ApprovalDecisionConfirm)
		return resp, true, err
	}
}

func isApprovalConfirmation(message string) bool {
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "confirm", "approve", "approved", "yes", "yes please", "proceed":
		return true
	default:
		return false
	}
}

func isApprovalRejection(message string) bool {
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "cancel", "reject", "decline", "no", "no thanks":
		return true
	default:
		return false
	}
}

func (l *AgentLoop) finalizeResponse(sessionKey, channel, chatID, requestID string, resp ChatResponse) ChatResponse {
	if resp.RichContent != nil {
		if strings.TrimSpace(resp.RichContent.Text) == "" {
			resp.RichContent.Text = strings.TrimSpace(resp.Message)
		}
		if strings.TrimSpace(resp.Message) == "" {
			resp.Message = strings.TrimSpace(resp.RichContent.PlainTextFallback())
		}
	}
	if strings.TrimSpace(resp.Message) == "" {
		resp.Message = "Done."
	}
	l.sessions.AddMessage(sessionKey, "assistant", resp.Message)

	if !isInternalChannelName(channel) {
		l.bus.PublishOutbound(OutboundEnvelope{
			Channel:     channel,
			ChatID:      chatID,
			Content:     resp.Message,
			RichContent: resp.RichContent,
			Metadata: map[string]string{
				"request_id": requestID,
				"action":     strings.TrimSpace(resp.Action),
			},
		})
	}
	l.bus.PublishEvent(LoopEvent{
		Type:    EventOutbound,
		Name:    "chat_response",
		Message: fmt.Sprintf("action=%s", strings.TrimSpace(resp.Action)),
		Metadata: map[string]string{
			"request_id": requestID,
			"session":    sessionKey,
		},
	})
	return resp
}

func ResolveSessionKey(provider, chatID string) string {
	return resolveSessionKey(provider, chatID)
}

func resolveSessionKey(provider, chatID string) string {
	channel := strings.ToLower(strings.TrimSpace(provider))
	if channel == "" {
		channel = "chat"
	}
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		chatID = "default"
	}
	return channel + ":" + chatID
}
