package baseagent

import (
	"carrier/shared/catalog"
	"carrier/shared/config"
	"context"
	"fmt"
	"path/filepath"
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

func mustRegisterProvider(pm *ProviderManager, provider Provider) {
	if pm == nil {
		panic("baseagent: provider manager is nil")
	}
	if err := pm.RegisterProvider(provider); err != nil {
		panic(fmt.Sprintf("baseagent: register built-in provider %q failed: %v", provider.Name(), err))
	}
}

type ChatRequest struct {
	Provider    string          `json:"provider"`
	ModelAlias  string          `json:"modelAlias,omitempty"`
	Model       string          `json:"model,omitempty"`
	ChatID      string          `json:"chatId"`
	RequestID   string          `json:"requestId"`
	Message     string          `json:"message"`
	Attachments []AttachmentRef `json:"attachments,omitempty"`
}

type ChatResponse struct {
	Message     string               `json:"message"`
	RichContent *RichOutboundMessage `json:"richContent,omitempty"`
	Action      string               `json:"action,omitempty"`
	SelfHealed  bool                 `json:"selfHealed,omitempty"`
	BackupRef   string               `json:"backupRef,omitempty"`
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
	cron      *CronService

	mu                           sync.Mutex
	initialized                  bool
	activeID                     string
	workspaceRoot                string
	maxToolIterations            int
	structuredToolPolicyOverride *StructuredToolPolicySpec
	mcpManager                   MCPManager
	skillsLoader                 SkillsLoader
	mediaRuntime                 MediaRuntime
	webBackend                   WebToolBackend
	subagentSpawner              SubagentSpawner
	subagentManager              SubagentManager
}

func NewRuntime(svc AgentService, memStore MemoryStore, opts ...RuntimeOption) *Runtime {
	r := &Runtime{
		svc:               svc,
		memory:            memStore,
		activeID:          baseAgentActiveMemoryV1ID,
		maxToolIterations: defaultMaxToolIterations,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	structuredToolPolicy := ActiveBoundarySpec().StructuredToolPolicy
	if r.structuredToolPolicyOverride != nil {
		structuredToolPolicy = *r.structuredToolPolicyOverride
	}
	r.bus = NewMessageBus(0, 0, 0)
	if r.workspaceRoot != "" {
		r.sessions = NewSessionManagerWithStorage(0, filepath.Join(r.workspaceRoot, ".baseagent", "sessions"))
	} else {
		r.sessions = NewSessionManager(0)
	}
	r.providers = NewProviderManager(nil)
	for _, provider := range catalog.ListProviders() {
		mustRegisterProvider(r.providers, NewLLMProviderAdapter(provider.ID, 8))
	}
	if configuredProvider := resolveConfiguredBaseAgentProviderID(); configuredProvider != "" {
		_ = r.providers.SetActiveProvider(configuredProvider)
	}
	r.tools = newBuiltinToolRegistry(r, r.providers, r.sessions)
	r.channels = NewChannelManager(r.bus)
	r.loop = NewAgentLoop(r.svc, r.tools, r.providers, r.sessions, r.bus)
	r.loop.SetChannelManager(r.channels)
	r.loop.SetSkillsLoader(r.skillsLoader)
	effectiveSubagentManager := r.subagentManager
	if effectiveSubagentManager == nil && r.subagentSpawner == nil {
		effectiveSubagentManager = NewInMemorySubagentManager(nil)
	}
	effectiveSubagentSpawner := r.subagentSpawner
	if effectiveSubagentManager != nil {
		effectiveSubagentSpawner = effectiveSubagentManager
	}
	r.subagentManager = effectiveSubagentManager
	executionToolOpts := []ExecutionToolRegistryOption{
		WithExecutionToolWebBackend(r.webBackend),
	}
	if effectiveSubagentSpawner != nil {
		executionToolOpts = append(executionToolOpts, WithExecutionToolSubagentSpawner(effectiveSubagentSpawner))
	}
	r.loop.SetExecutionTools(NewExecutionToolRegistry(
		r.workspaceRoot,
		executionToolOpts...,
	), r.maxToolIterations, structuredToolPolicy, r.mcpManager, effectiveSubagentManager)
	r.loop.SetMemoryStore(r.memory, baseAgentVirtualID)
	r.cron = NewCronService(func(ctx context.Context, job CronJob) error {
		_, err := r.Chat(ctx, cronChatRequestForSessionKey(job.SessionKey, job.Prompt))
		return err
	})
	return r
}

func resolveConfiguredBaseAgentProviderID() string {
	cfg, err := config.LoadCarrierDefaultModel()
	if err != nil || cfg == nil {
		return ""
	}
	providerID := strings.ToLower(strings.TrimSpace(cfg.ProviderID))
	if !catalog.IsSupportedProvider(providerID) {
		return ""
	}
	return providerID
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

func (r *Runtime) ListInstalledSkills(ctx context.Context) []SkillDefinition {
	if r == nil || r.skillsLoader == nil {
		return nil
	}
	return r.skillsLoader.ListInstalledSkills(ctx)
}

func (r *Runtime) SearchSkills(ctx context.Context, query string) []SkillDefinition {
	if r == nil || r.skillsLoader == nil {
		return nil
	}
	return r.skillsLoader.SearchSkills(ctx, query)
}

func (r *Runtime) InstallSkill(ctx context.Context, name string) (SkillDefinition, error) {
	if r == nil || r.skillsLoader == nil {
		return SkillDefinition{}, fmt.Errorf("skills loader is unavailable")
	}
	return r.skillsLoader.InstallSkill(ctx, name)
}

func (r *Runtime) StartMCP(ctx context.Context) error {
	if r == nil || r.mcpManager == nil {
		return nil
	}
	manager, ok := r.mcpManager.(interface{ Start(context.Context) error })
	if !ok {
		return nil
	}
	return manager.Start(ctx)
}

func (r *Runtime) StopMCP(ctx context.Context) error {
	if r == nil || r.mcpManager == nil {
		return nil
	}
	manager, ok := r.mcpManager.(interface{ Stop(context.Context) error })
	if !ok {
		return nil
	}
	return manager.Stop(ctx)
}

func (r *Runtime) SetMCPServerEnabled(ctx context.Context, name string, enabled bool) error {
	if r == nil || r.mcpManager == nil {
		return fmt.Errorf("mcp manager is unavailable")
	}
	if err := r.mcpManager.SetServerEnabled(ctx, name, enabled); err != nil {
		return err
	}
	if r.loop != nil {
		r.loop.SetExecutionTools(r.loop.executionTools, r.loop.maxToolIterations, r.effectiveStructuredToolPolicy(), r.mcpManager, r.subagentManager)
	}
	return nil
}

func (r *Runtime) effectiveStructuredToolPolicy() StructuredToolPolicySpec {
	if r == nil {
		return StructuredToolPolicySpec{}
	}
	if r.structuredToolPolicyOverride != nil {
		return *r.structuredToolPolicyOverride
	}
	return ActiveBoundarySpec().StructuredToolPolicy
}

func (r *Runtime) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	preparedReq, boundedResp, err := r.prepareMediaRequest(ctx, req)
	if err != nil {
		return ChatResponse{}, err
	}
	if boundedResp != nil {
		return *boundedResp, nil
	}
	req = preparedReq
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

func (r *Runtime) ScheduleJob(ctx context.Context, job CronJob) (CronJob, error) {
	if r == nil || r.cron == nil {
		return CronJob{}, fmt.Errorf("cron service is unavailable")
	}
	return r.cron.Schedule(ctx, job)
}

func (r *Runtime) ListCronJobs(_ context.Context, sessionKey string) ([]CronJob, error) {
	if r == nil || r.cron == nil {
		return nil, fmt.Errorf("cron service is unavailable")
	}
	return r.cron.List(sessionKey), nil
}

func (r *Runtime) CancelCronJob(_ context.Context, jobID string) (CronJob, error) {
	if r == nil || r.cron == nil {
		return CronJob{}, fmt.Errorf("cron service is unavailable")
	}
	return r.cron.Cancel(jobID)
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
	return "Base agent manages local agents: `list agents` or `/agents`, `uninstall <agent>`, `start <agent>`, `stop <agent>`, `status <agent>`, `logs <agent>`, `upgrade <agent>`, `diagnose <agent>`. Metadata commands: `/tools`, `/providers`, `/sessions`, `/boundaries`. When workspace tools are enabled, chat can use `read_file`, `write_file`, `append_file`, `edit_file`, `list_dir`, and bounded `exec` inside the current project workspace. For install/onboard, use Carrier CLI/TUI (`carrier install <agent>`, `carrier onboard`) or WebUI."
}

func baseAgentBoundariesText() string {
	return ActiveBoundarySpec().RenderSummary()
}

func (r *Runtime) renderToolSummary() string {
	if r == nil || r.tools == nil {
		return "No tools are registered."
	}

	lines := []string{r.tools.RenderToolSummary()}
	if r.loop == nil || r.loop.executionTools == nil {
		return strings.Join(lines, "\n")
	}

	descriptors := r.loop.executionTools.Descriptors()
	if len(descriptors) == 0 {
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "", fmt.Sprintf("Workspace tools (%d):", len(descriptors)))
	lines = append(lines, renderStructuredToolLines(descriptors)...)
	return strings.Join(lines, "\n")
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
