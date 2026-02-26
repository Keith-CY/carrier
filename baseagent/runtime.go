package baseagent

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	baseAgentVirtualID          = "carrier.base.internal"
	baseAgentPublicMemoryID     = "carrier.base.public.v1"
	baseAgentActiveMemoryV1ID   = "carrier.base.active.v1"
	baseAgentActiveMemoryPrefix = "carrier.base.active."
)

var agentActionPattern = regexp.MustCompile(`(?i)\b(uninstall|start|stop|status|logs|upgrade|diagnose)\s+([a-zA-Z0-9][a-zA-Z0-9._-]*)\b`)

type ChatRequest struct {
	Provider  string `json:"provider"`
	ChatID    string `json:"chatId"`
	RequestID string `json:"requestId"`
	Message   string `json:"message"`
}

type ChatResponse struct {
	Message    string `json:"message"`
	Action     string `json:"action,omitempty"`
	SelfHealed bool   `json:"selfHealed,omitempty"`
	BackupRef  string `json:"backupRef,omitempty"`
}

type AgentState struct {
	ID           string
	Install      string
	Runtime      string
	Health       string
	RestartCount int
}

type UpgradeResult struct {
	AgentID     string
	FromVersion string
	ToVersion   string
}

type AgentService interface {
	ListAgents() []AgentState
	Install(ctx context.Context, agentID string) error
	Uninstall(ctx context.Context, agentID string) error
	Start(ctx context.Context, agentID string) error
	Stop(ctx context.Context, agentID string) error
	Status(agentID string) (AgentState, error)
	Logs(agentID string, tail int) ([]string, error)
	Upgrade(ctx context.Context, agentID string) (UpgradeResult, error)
	Diagnose(agentID string) (string, error)
}

type Runtime struct {
	svc    AgentService
	memory MemoryStore

	bus       *MessageBus
	channels  *ChannelManager
	tools     *ToolRegistry
	providers *ProviderManager
	sessions  *SessionManager
	loop      *AgentLoop

	mu          sync.Mutex
	initialized bool
	activeID    string
}

func NewRuntime(svc AgentService, memStore MemoryStore) *Runtime {
	llmProvider := NewLLMProviderAdapter("llm", 8)
	localFallback := NewStaticProvider("local-fallback", "I'm currently running in local fallback mode. Try `list agents`, `status <agent>`, `logs <agent>`, or `help`.")

	r := &Runtime{
		svc:      svc,
		memory:   memStore,
		activeID: baseAgentActiveMemoryV1ID,
	}
	r.bus = NewMessageBus(0, 0, 0)
	r.sessions = NewSessionManager(0)
	r.providers = NewProviderManager(llmProvider)
	_ = r.providers.RegisterProvider(localFallback)
	_ = r.providers.RegisterProvider(NewChainProvider("llm-with-fallback", llmProvider, localFallback))
	r.tools = newBuiltinToolRegistry(r, r.providers, r.sessions)
	r.channels = NewChannelManager(r.bus)
	r.loop = NewAgentLoop(r.svc, r.tools, r.providers, r.sessions, r.bus)
	r.loop.SetChannelManager(r.channels)
	return r
}

// RegisterExternalChannel registers a concrete channel transport (e.g. telegram, discord, feishu).
func (r *Runtime) RegisterExternalChannel(name string, sender ChannelSender) error {
	if r == nil || r.channels == nil {
		return fmt.Errorf("channel manager is unavailable")
	}
	return r.channels.RegisterChannel(name, NewCallbackChannel(name, sender))
}

// StartChannels starts all registered channels.
func (r *Runtime) StartChannels(ctx context.Context) error {
	if r == nil || r.channels == nil {
		return fmt.Errorf("channel manager is unavailable")
	}
	return r.channels.StartAll(ctx)
}

// StopChannels stops all registered channels.
func (r *Runtime) StopChannels(ctx context.Context) error {
	if r == nil || r.channels == nil {
		return fmt.Errorf("channel manager is unavailable")
	}
	return r.channels.StopAll(ctx)
}

// RegisterProvider registers a provider implementation into the provider manager.
func (r *Runtime) RegisterProvider(provider Provider) error {
	if r == nil || r.providers == nil {
		return fmt.Errorf("provider manager is unavailable")
	}
	return r.providers.RegisterProvider(provider)
}

// SetActiveProvider sets the active provider by name.
func (r *Runtime) SetActiveProvider(name string) error {
	if r == nil || r.providers == nil {
		return fmt.Errorf("provider manager is unavailable")
	}
	return r.providers.SetActiveProvider(name)
}

func (r *Runtime) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		return ChatResponse{Message: baseAgentHelpText()}, nil
	}

	healNote, healed, backupRef, err := r.ensureMemoryReady()
	if err != nil {
		healNote = "Base-agent memory check failed; continuing with safe mode."
	}

	resp, err := r.loop.ProcessChat(ctx, req)
	if err != nil {
		return withMemoryNote(ChatResponse{
			Message: fmt.Sprintf("%s\n%s", explainLLMUnavailable(err), baseAgentHelpText()),
			Action:  "help",
		}, healNote, healed, backupRef), nil
	}
	return withMemoryNote(resp, healNote, healed, backupRef), nil
}

func wantsListAgents(lower string) bool {
	lower = strings.TrimSpace(lower)
	if strings.HasPrefix(lower, "/agents") || lower == "/list agents" {
		return true
	}
	return strings.Contains(lower, "list agents") ||
		strings.Contains(lower, "show agents") ||
		strings.Contains(lower, "agents") &&
			!strings.Contains(lower, "install ") &&
			!strings.Contains(lower, "add ")
}

func renderAgentList(states []AgentState) string {
	if len(states) == 0 {
		return "No agents are registered."
	}
	lines := make([]string, 0, len(states)+1)
	lines = append(lines, fmt.Sprintf("Found %d agents:", len(states)))
	for _, s := range states {
		lines = append(lines, fmt.Sprintf("- %s: install=%s runtime=%s health=%s", s.ID, s.Install, s.Runtime, s.Health))
	}
	return strings.Join(lines, "\n")
}

func (r *Runtime) executeAgentAction(ctx context.Context, action, agentID string) (ChatResponse, error) {
	if r.svc == nil {
		return ChatResponse{}, fmt.Errorf("agent service is unavailable")
	}
	switch action {
	case "uninstall":
		if err := r.svc.Uninstall(ctx, agentID); err != nil {
			return ChatResponse{}, err
		}
		return ChatResponse{Message: fmt.Sprintf("Uninstalled %s.", agentID), Action: "uninstall"}, nil
	case "start":
		if err := r.svc.Start(ctx, agentID); err != nil {
			return ChatResponse{}, err
		}
		return ChatResponse{Message: fmt.Sprintf("Started %s.", agentID), Action: "start"}, nil
	case "stop":
		if err := r.svc.Stop(ctx, agentID); err != nil {
			return ChatResponse{}, err
		}
		return ChatResponse{Message: fmt.Sprintf("Stopped %s.", agentID), Action: "stop"}, nil
	case "status":
		state, err := r.svc.Status(agentID)
		if err != nil {
			return ChatResponse{}, err
		}
		return ChatResponse{
			Message: fmt.Sprintf("%s: install=%s runtime=%s health=%s restarts=%d", state.ID, state.Install, state.Runtime, state.Health, state.RestartCount),
			Action:  "status",
		}, nil
	case "logs":
		lines, err := r.svc.Logs(agentID, 50)
		if err != nil {
			return ChatResponse{}, err
		}
		if len(lines) == 0 {
			return ChatResponse{Message: fmt.Sprintf("No logs for %s.", agentID), Action: "logs"}, nil
		}
		return ChatResponse{Message: strings.Join(lines, "\n"), Action: "logs"}, nil
	case "upgrade":
		result, err := r.svc.Upgrade(ctx, agentID)
		if err != nil {
			return ChatResponse{}, err
		}
		return ChatResponse{
			Message: fmt.Sprintf("Upgraded %s from %s to %s.", result.AgentID, result.FromVersion, result.ToVersion),
			Action:  "upgrade",
		}, nil
	case "diagnose":
		ref, err := r.svc.Diagnose(agentID)
		if err != nil {
			return ChatResponse{}, err
		}
		return ChatResponse{
			Message: fmt.Sprintf("Created diagnose artifact for %s: %s", agentID, ref),
			Action:  "diagnose",
		}, nil
	default:
		return ChatResponse{}, fmt.Errorf("unsupported action: %s", action)
	}
}

func baseAgentHelpText() string {
	return "Base agent manages local agents: `list agents` or `/agents`, `uninstall <agent>`, `start <agent>`, `stop <agent>`, `status <agent>`, `logs <agent>`, `upgrade <agent>`, `diagnose <agent>`. Metadata commands: `/tools`, `/providers`, `/sessions`. For install/onboard, open Carrier GUI."
}

func withMemoryNote(resp ChatResponse, note string, healed bool, backupRef string) ChatResponse {
	if strings.TrimSpace(note) != "" {
		resp.Message = note + "\n" + resp.Message
	}
	resp.SelfHealed = healed
	resp.BackupRef = backupRef
	return resp
}

func explainLLMUnavailable(err error) string {
	if err == nil {
		return "LLM chat is currently unavailable."
	}
	msg := strings.TrimSpace(strings.ReplaceAll(err.Error(), "\n", " "))
	if len(msg) > 220 {
		msg = msg[:220] + "..."
	}
	return fmt.Sprintf("LLM chat is currently unavailable (%s).", msg)
}

func (r *Runtime) ensureMemoryReady() (note string, healed bool, backupRef string, err error) {
	if r.memory == nil {
		return "", false, "", nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.initialized {
		if err := r.ensureMemoryEntry(baseAgentPublicMemoryID, "Carrier Base Public Memory", "v1", MemoryTypePublic, ""); err != nil {
			return "", false, "", err
		}
		r.activeID = r.pickActiveMemoryID()
		if err := r.ensureMemoryEntry(r.activeID, "Carrier Base Active Memory", "v1", MemoryTypePerAgent, baseAgentVirtualID); err != nil {
			return "", false, "", err
		}
		r.initialized = true
	}

	if err := r.memory.SetAttachmentsFromLinks(baseAgentVirtualID, []string{baseAgentPublicMemoryID, r.activeID}); err != nil {
		return "", false, "", err
	}
	if err := r.memory.PrepareAgentMemory(baseAgentVirtualID); err == nil {
		return "", false, "", nil
	}

	backupRef, _ = r.memory.ExportMemory(r.activeID, ExportOptions{
		Actor:     "carrier-base",
		RequestID: fmt.Sprintf("base-self-heal-%d", time.Now().UTC().UnixNano()),
	})
	_ = r.memory.Archive(r.activeID)

	r.activeID = fmt.Sprintf("%s%d", baseAgentActiveMemoryPrefix, time.Now().UTC().UnixNano())
	if err := r.ensureMemoryEntry(r.activeID, "Carrier Base Active Memory", "v2", MemoryTypePerAgent, baseAgentVirtualID); err != nil {
		return "", false, backupRef, err
	}
	if err := r.memory.SetAttachmentsFromLinks(baseAgentVirtualID, []string{baseAgentPublicMemoryID, r.activeID}); err != nil {
		return "", false, backupRef, err
	}
	if err := r.memory.PrepareAgentMemory(baseAgentVirtualID); err != nil {
		return "", false, backupRef, err
	}
	msg := "Base-agent memory self-heal completed: active memory was reset from base public memory."
	if backupRef != "" {
		msg += " Backup: " + backupRef
	}
	return msg, true, backupRef, nil
}

func (r *Runtime) ensureMemoryEntry(id, name, version string, t MemoryType, owner string) error {
	if err := r.memory.Get(id); err == nil {
		return nil
	}
	return r.memory.Create(id, name, version, t, owner)
}

func (r *Runtime) pickActiveMemoryID() string {
	entries := r.memory.List()
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasPrefix(e.ID, baseAgentActiveMemoryPrefix) && e.State != MemoryStateArchived {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 {
		return baseAgentActiveMemoryV1ID
	}
	sort.Strings(ids)
	return ids[len(ids)-1]
}
