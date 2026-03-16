package gateway

import (
	"carrier/baseagent"
	"carrier/shared/catalog"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type agentLauncherSummary struct {
	AgentID           string                          `json:"agentId"`
	Status            AgentState                      `json:"status"`
	Heartbeat         *AgentHeartbeat                 `json:"heartbeat,omitempty"`
	Memory            *AgentMemoryState               `json:"memory,omitempty"`
	Capabilities      AgentCapabilitySummary          `json:"capabilities"`
	ProviderReadiness agentProviderReadiness          `json:"providerReadiness"`
	MediaRuntime      *agentLauncherMediaRuntime      `json:"mediaRuntime,omitempty"`
	Remediations      []agentLauncherRemediation      `json:"remediations,omitempty"`
	ModelSurface      *agentLauncherModelSurface      `json:"modelSurface,omitempty"`
	LastModelRun      *agentLauncherModelRuntime      `json:"lastModelRun,omitempty"`
	Cron              *agentLauncherCronSummary       `json:"cron,omitempty"`
	Delegation        *agentLauncherDelegationSummary `json:"delegation,omitempty"`
	Sessions          *agentLauncherSessionsSummary   `json:"sessions,omitempty"`
	Session           *agentLauncherSession           `json:"session,omitempty"`
}

type agentLauncherMediaRuntime struct {
	Provider        string `json:"provider,omitempty"`
	Status          string `json:"status,omitempty"`
	Detail          string `json:"detail,omitempty"`
	RemediationHint string `json:"remediationHint,omitempty"`
}

type agentLauncherRemediation struct {
	Category        string                          `json:"category,omitempty"`
	Summary         string                          `json:"summary,omitempty"`
	Detail          string                          `json:"detail,omitempty"`
	RemediationHint string                          `json:"remediationHint,omitempty"`
	Action          *agentLauncherRemediationAction `json:"action,omitempty"`
}

type agentLauncherRemediationAction struct {
	Kind   string `json:"kind,omitempty"`
	Label  string `json:"label,omitempty"`
	Target string `json:"target,omitempty"`
}

type agentLauncherModelSurface struct {
	DefaultProfile string                             `json:"defaultProfile,omitempty"`
	Profiles       []agentLauncherModelSurfaceProfile `json:"profiles,omitempty"`
}

type agentLauncherModelSurfaceProfile struct {
	ProfileName      string `json:"profileName,omitempty"`
	ModelAlias       string `json:"modelAlias,omitempty"`
	ModelID          string `json:"modelId,omitempty"`
	ProviderID       string `json:"providerId,omitempty"`
	ProviderKey      string `json:"providerKey,omitempty"`
	ProtocolFamily   string `json:"protocolFamily,omitempty"`
	BaseURL          string `json:"baseUrl,omitempty"`
	AuthMethod       string `json:"authMethod,omitempty"`
	TimeoutMs        int    `json:"timeoutMs,omitempty"`
	RetryBudget      int    `json:"retryBudget,omitempty"`
	FallbackStrategy string `json:"fallbackStrategy,omitempty"`
	FallbackGroup    string `json:"fallbackGroup,omitempty"`
	AliasGroupSize   int    `json:"aliasGroupSize,omitempty"`
	Primary          bool   `json:"primary,omitempty"`
}

type agentLauncherModelRuntime struct {
	RequestedAlias    string `json:"requestedAlias,omitempty"`
	RequestedModel    string `json:"requestedModel,omitempty"`
	ResolvedModel     string `json:"resolvedModel,omitempty"`
	ResolvedProfile   string `json:"resolvedProfile,omitempty"`
	FallbackGroup     string `json:"fallbackGroup,omitempty"`
	SelectionStrategy string `json:"selectionStrategy,omitempty"`
	SelectionOrdinal  int    `json:"selectionOrdinal,omitempty"`
	OverrideHit       bool   `json:"overrideHit,omitempty"`
	FallbackHit       bool   `json:"fallbackHit,omitempty"`
	LastRunAt         string `json:"lastRunAt,omitempty"`
}

type agentLauncherCronSummary struct {
	Count      int       `json:"count"`
	NextRunAt  string    `json:"nextRunAt,omitempty"`
	LastRunAt  string    `json:"lastRunAt,omitempty"`
	LastResult string    `json:"lastResult,omitempty"`
	Jobs       []CronJob `json:"jobs,omitempty"`
}

type agentLauncherDelegationSummary struct {
	Count int                     `json:"count"`
	Jobs  []baseagent.SubagentJob `json:"jobs,omitempty"`
}

type agentLauncherSessionsSummary struct {
	Count    int                      `json:"count"`
	Sessions []baseagent.SessionStats `json:"sessions,omitempty"`
}

type agentProviderReadiness struct {
	Provider             string `json:"provider,omitempty"`
	AuthMode             string `json:"authMode,omitempty"`
	CredentialBackend    string `json:"credentialBackend,omitempty"`
	CredentialConfigured bool   `json:"credentialConfigured"`
	Ready                bool   `json:"ready"`
}

type agentLauncherSession struct {
	InstanceID          string   `json:"instanceId"`
	Type                string   `json:"type,omitempty"`
	Channel             string   `json:"channel,omitempty"`
	Provider            string   `json:"provider,omitempty"`
	Isolation           bool     `json:"isolation,omitempty"`
	RuntimeState        string   `json:"runtimeState,omitempty"`
	GatewayURL          string   `json:"gatewayUrl,omitempty"`
	PairRequired        bool     `json:"pairRequired,omitempty"`
	PairedChatID        string   `json:"pairedChatId,omitempty"`
	Workspace           string   `json:"workspace,omitempty"`
	ConfigPath          string   `json:"configPath,omitempty"`
	RecordPath          string   `json:"recordPath,omitempty"`
	AgentLifecycleMode  string   `json:"agentLifecycleMode,omitempty"`
	MemoryBindingMode   string   `json:"memoryBindingMode,omitempty"`
	PublicScopes        []string `json:"publicScopes,omitempty"`
	SharedScopes        []string `json:"sharedScopes,omitempty"`
	PerAgentMemoryID    string   `json:"perAgentMemoryId,omitempty"`
	MemoryRefreshPolicy string   `json:"memoryRefreshPolicy,omitempty"`
	ParentAgentID       string   `json:"parentAgentId,omitempty"`
	ParentExecutionID   string   `json:"parentExecutionId,omitempty"`
	TaskID              string   `json:"taskId,omitempty"`
	SnapshotID          string   `json:"snapshotId,omitempty"`
	SnapshotDigest      string   `json:"snapshotDigest,omitempty"`
	DistillTarget       string   `json:"distillTarget,omitempty"`
	CleanupPolicy       string   `json:"cleanupPolicy,omitempty"`
	CreatedAt           string   `json:"createdAt,omitempty"`
	UpdatedAt           string   `json:"updatedAt,omitempty"`
}

func handleAgentLauncher(w http.ResponseWriter, r *http.Request, requestID, agentID string, daemon *DaemonClient) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	if daemon == nil {
		writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_COMMAND_FAILED", "daemon client is unavailable"))
		return
	}

	statuses, err := daemon.GetStatus(r.Context(), agentID, "webui:agents:launcher", requestID)
	if err != nil {
		writeDaemonAPIError(w, err)
		return
	}
	if len(statuses) == 0 {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_AGENT_NOT_FOUND", fmt.Sprintf("agent %s not found", agentID)))
		return
	}
	capabilities, err := daemon.GetAgentCapabilities(r.Context(), agentID, "webui:agents:launcher", requestID)
	if err != nil {
		writeDaemonAPIError(w, err)
		return
	}

	var session *agentLauncherSession
	var readiness agentProviderReadiness
	var modelSurface *agentLauncherModelSurface
	var lastModelRun *agentLauncherModelRuntime
	if inst, ok := latestManagedInstanceForAgent(agentID); ok {
		session = &agentLauncherSession{
			InstanceID:          strings.TrimSpace(inst.ID),
			Type:                strings.TrimSpace(inst.Type),
			Channel:             strings.TrimSpace(inst.Channel),
			Provider:            strings.TrimSpace(inst.Provider),
			Isolation:           inst.Isolation,
			RuntimeState:        strings.TrimSpace(inst.RuntimeState),
			GatewayURL:          strings.TrimSpace(inst.GatewayURL),
			PairRequired:        inst.PairRequired,
			PairedChatID:        strings.TrimSpace(inst.PairedChatID),
			Workspace:           strings.TrimSpace(inst.Workspace),
			ConfigPath:          strings.TrimSpace(inst.ConfigPath),
			RecordPath:          strings.TrimSpace(inst.RecordPath),
			AgentLifecycleMode:  strings.TrimSpace(inst.AgentLifecycleMode),
			MemoryBindingMode:   strings.TrimSpace(inst.MemoryBindingMode),
			PublicScopes:        append([]string(nil), inst.PublicScopes...),
			SharedScopes:        append([]string(nil), inst.SharedScopes...),
			PerAgentMemoryID:    strings.TrimSpace(inst.PerAgentMemoryID),
			MemoryRefreshPolicy: strings.TrimSpace(inst.MemoryRefreshPolicy),
			ParentAgentID:       strings.TrimSpace(inst.ParentAgentID),
			ParentExecutionID:   strings.TrimSpace(inst.ParentExecutionID),
			TaskID:              strings.TrimSpace(inst.TaskID),
			SnapshotID:          strings.TrimSpace(inst.SnapshotID),
			SnapshotDigest:      strings.TrimSpace(inst.SnapshotDigest),
			DistillTarget:       strings.TrimSpace(inst.DistillTarget),
			CleanupPolicy:       strings.TrimSpace(inst.CleanupPolicy),
			CreatedAt:           strings.TrimSpace(inst.CreatedAt),
			UpdatedAt:           strings.TrimSpace(inst.UpdatedAt),
		}
		readiness = buildAgentProviderReadiness(inst.Provider)
		modelSurface = buildAgentLauncherModelSurface(inst.ModelSurface)
		lastModelRun = buildAgentLauncherModelRuntime(inst.ModelRuntime)
	}
	var cronSummary *agentLauncherCronSummary
	if jobs, err := daemon.ListCronJobs(r.Context(), agentID, "", "webui:agents:launcher", requestID); err == nil && len(jobs) > 0 {
		cronSummary = buildAgentCronSummary(jobs)
	}
	var delegationSummary *agentLauncherDelegationSummary
	if jobs, err := daemon.GetAgentSubagentJobs(r.Context(), agentID, 5, "webui:agents:launcher", requestID); err == nil && len(jobs) > 0 {
		delegationSummary = &agentLauncherDelegationSummary{Count: len(jobs), Jobs: jobs}
	}
	var sessionsSummary *agentLauncherSessionsSummary
	if sessions, err := daemon.GetAgentSessions(r.Context(), agentID, 5, "webui:agents:launcher", requestID); err == nil && len(sessions) > 0 {
		sessionsSummary = &agentLauncherSessionsSummary{Count: len(sessions), Sessions: sessions}
	}

	status := statuses[0]
	if readiness.Provider == "" {
		readiness = buildAgentProviderReadiness(agentProviderFromStatus(status))
	}
	mediaRuntime := buildAgentMediaRuntime(firstNonEmptyAgentProvider(strings.TrimSpace(os.Getenv("CARRIER_TRANSCRIPTION_PROVIDER")), readiness.Provider, agentProviderFromStatus(status)))
	remediations := buildAgentLauncherRemediations(readiness, status.Heartbeat, cronSummary, delegationSummary, capabilities, session, mediaRuntime)

	writeJSON(w, http.StatusOK, agentLauncherSummary{
		AgentID:           agentID,
		Status:            status,
		Heartbeat:         status.Heartbeat,
		Memory:            status.Memory,
		Capabilities:      capabilities,
		ProviderReadiness: readiness,
		MediaRuntime:      mediaRuntime,
		Remediations:      remediations,
		ModelSurface:      modelSurface,
		LastModelRun:      lastModelRun,
		Cron:              cronSummary,
		Delegation:        delegationSummary,
		Sessions:          sessionsSummary,
		Session:           session,
	})
}

func firstNonEmptyAgentProvider(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func buildAgentLauncherModelRuntime(runtime *managedAgentModelRuntime) *agentLauncherModelRuntime {
	if runtime == nil {
		return nil
	}
	return &agentLauncherModelRuntime{
		RequestedAlias:    strings.TrimSpace(runtime.RequestedAlias),
		RequestedModel:    strings.TrimSpace(runtime.RequestedModel),
		ResolvedModel:     strings.TrimSpace(runtime.ResolvedModel),
		ResolvedProfile:   strings.TrimSpace(runtime.ResolvedProfile),
		FallbackGroup:     strings.TrimSpace(runtime.FallbackGroup),
		SelectionStrategy: strings.TrimSpace(runtime.SelectionStrategy),
		SelectionOrdinal:  runtime.SelectionOrdinal,
		OverrideHit:       runtime.OverrideHit,
		FallbackHit:       runtime.FallbackHit,
		LastRunAt:         strings.TrimSpace(runtime.LastRunAt),
	}
}

func buildAgentLauncherModelSurface(surface *managedAgentModelSurface) *agentLauncherModelSurface {
	if surface == nil {
		return nil
	}
	groupSizes := map[string]int{}
	groupPrimaries := map[string]bool{}
	for _, profile := range surface.Profiles {
		group := strings.TrimSpace(profile.FallbackGroup)
		if group == "" {
			group = strings.TrimSpace(managedAgentModelSurfaceFallbackGroup(profile))
		}
		if group == "" {
			continue
		}
		groupSizes[group]++
	}
	result := &agentLauncherModelSurface{
		DefaultProfile: strings.TrimSpace(surface.DefaultProfile),
		Profiles:       make([]agentLauncherModelSurfaceProfile, 0, len(surface.Profiles)),
	}
	for index, profile := range surface.Profiles {
		group := strings.TrimSpace(profile.FallbackGroup)
		if group == "" {
			group = strings.TrimSpace(managedAgentModelSurfaceFallbackGroup(profile))
		}
		aliasGroupSize := profile.AliasGroupSize
		if aliasGroupSize == 0 {
			aliasGroupSize = groupSizes[group]
		}
		primary := profile.Primary
		if !primary && group != "" && !groupPrimaries[group] {
			primary = true
		}
		if group != "" && primary {
			groupPrimaries[group] = true
		}
		if group == "" && index == 0 && !primary {
			primary = true
		}
		result.Profiles = append(result.Profiles, agentLauncherModelSurfaceProfile{
			ProfileName:      strings.TrimSpace(profile.ProfileName),
			ModelAlias:       strings.TrimSpace(profile.ModelAlias),
			ModelID:          strings.TrimSpace(profile.ModelID),
			ProviderID:       strings.TrimSpace(profile.ProviderID),
			ProviderKey:      strings.TrimSpace(profile.ProviderKey),
			ProtocolFamily:   strings.TrimSpace(profile.ProtocolFamily),
			BaseURL:          strings.TrimSpace(profile.BaseURL),
			AuthMethod:       strings.TrimSpace(profile.AuthMethod),
			TimeoutMs:        profile.TimeoutMs,
			RetryBudget:      profile.RetryBudget,
			FallbackStrategy: strings.TrimSpace(profile.FallbackStrategy),
			FallbackGroup:    group,
			AliasGroupSize:   aliasGroupSize,
			Primary:          primary,
		})
	}
	return result
}

func managedAgentModelSurfaceFallbackGroup(profile managedAgentModelProfile) string {
	alias := strings.ToLower(strings.TrimSpace(profile.ModelAlias))
	if alias == "" {
		return ""
	}
	provider := strings.ToLower(strings.TrimSpace(profile.ProviderID))
	if provider == "" {
		provider = strings.ToLower(strings.TrimSpace(profile.ProviderKey))
	}
	if provider == "" {
		return alias
	}
	return provider + ":" + alias
}

func buildAgentProviderReadiness(providerID string) agentProviderReadiness {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return agentProviderReadiness{}
	}
	readiness := agentProviderReadiness{Provider: providerID}
	if provider := GetLLMProvider(providerID); provider != nil {
		readiness.AuthMode = string(provider.AuthMode)
		if provider.AuthMode == AuthModeNone {
			readiness.Ready = true
			return readiness
		}
		if strings.TrimSpace(provider.EnvVar) != "" && strings.TrimSpace(os.Getenv(provider.EnvVar)) != "" {
			readiness.CredentialConfigured = true
			readiness.CredentialBackend = "env"
			readiness.Ready = true
		}
	}
	backend, ok, err := loadProviderCredentialStatus(providerID)
	if err == nil && ok {
		readiness.CredentialConfigured = true
		readiness.CredentialBackend = backend
		readiness.Ready = true
	}
	return readiness
}

func buildAgentMediaRuntime(providerID string) *agentLauncherMediaRuntime {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil
	}
	summary := &agentLauncherMediaRuntime{
		Provider: providerID,
	}
	protocolFamily := strings.TrimSpace(catalog.ProtocolFamilyForProvider(providerID))
	if !strings.EqualFold(providerID, "openrouter") && !strings.EqualFold(protocolFamily, "openai-compatible") {
		summary.Status = "unsupported"
		summary.Detail = fmt.Sprintf("provider=%s runtime unsupported", providerID)
		summary.RemediationHint = "Switch transcription to an OpenAI-compatible provider."
		return summary
	}
	readiness := buildAgentProviderReadiness(providerID)
	if readiness.Ready {
		summary.Status = "ready"
		summary.Detail = fmt.Sprintf("provider=%s runtime configured", providerID)
		return summary
	}
	summary.Status = "unavailable"
	summary.Detail = fmt.Sprintf("provider=%s runtime unavailable", providerID)
	summary.RemediationHint = "Configure transcription credentials or switch providers."
	return summary
}

func buildAgentLauncherRemediations(
	readiness agentProviderReadiness,
	heartbeat *AgentHeartbeat,
	cronSummary *agentLauncherCronSummary,
	delegationSummary *agentLauncherDelegationSummary,
	capabilities AgentCapabilitySummary,
	session *agentLauncherSession,
	mediaRuntime *agentLauncherMediaRuntime,
) []agentLauncherRemediation {
	var remediations []agentLauncherRemediation
	if readiness.Provider != "" && !readiness.Ready {
		remediations = append(remediations, agentLauncherRemediation{
			Category: "provider",
			Summary:  "Provider authentication is not ready. Reconfigure credentials or switch to a ready profile.",
			Detail:   fmt.Sprintf("provider=%s auth=%s", readiness.Provider, strings.TrimSpace(readiness.AuthMode)),
			Action: &agentLauncherRemediationAction{
				Kind:  "sync-model-surface",
				Label: "Sync model surface",
			},
		})
	}
	if heartbeat != nil && (strings.EqualFold(strings.TrimSpace(heartbeat.State), "stale") || strings.EqualFold(strings.TrimSpace(heartbeat.State), "expired")) {
		action := &agentLauncherRemediationAction{
			Kind:  "start-runtime",
			Label: "Start runtime",
		}
		if session != nil && strings.EqualFold(strings.TrimSpace(session.RuntimeState), "running") {
			action = &agentLauncherRemediationAction{
				Kind:  "restart-runtime",
				Label: "Restart runtime",
			}
		}
		remediations = append(remediations, agentLauncherRemediation{
			Category: "heartbeat",
			Summary:  "Launcher heartbeat is stale. Restart the agent or inspect the managed runtime.",
			Detail:   fmt.Sprintf("state=%s age=%ds", strings.TrimSpace(heartbeat.State), heartbeat.AgeSeconds),
			Action:   action,
		})
	}
	if cronSummary != nil {
		for _, job := range cronSummary.Jobs {
			if !job.Paused {
				continue
			}
			remediations = append(remediations, agentLauncherRemediation{
				Category: "cron",
				Summary:  "One or more cron jobs are paused. Resume or cancel them to restore scheduled automation.",
				Detail:   fmt.Sprintf("job=%s last=%s", strings.TrimSpace(job.ID), strings.TrimSpace(job.LastResult)),
				Action: &agentLauncherRemediationAction{
					Kind:   "resume-cron",
					Label:  fmt.Sprintf("Resume %s", strings.TrimSpace(job.ID)),
					Target: strings.TrimSpace(job.ID),
				},
			})
			break
		}
	}
	if delegationSummary != nil {
		for _, job := range delegationSummary.Jobs {
			status := strings.TrimSpace(string(job.Status))
			if !strings.EqualFold(status, string(baseagent.SubagentJobStatusFailed)) && !strings.EqualFold(status, string(baseagent.SubagentJobStatusCancelled)) {
				continue
			}
			remediations = append(remediations, agentLauncherRemediation{
				Category:        "delegation",
				Summary:         "One or more delegated jobs ended unsuccessfully. Inspect delegation detail before trusting downstream results.",
				Detail:          fmt.Sprintf("job=%s status=%s", strings.TrimSpace(job.JobID), status),
				RemediationHint: firstNonEmpty(strings.TrimSpace(job.Error), strings.TrimSpace(job.Summary), "Inspect the recent delegation jobs to review failure detail."),
				Action: &agentLauncherRemediationAction{
					Kind:   "inspect-delegation",
					Label:  fmt.Sprintf("Inspect %s", strings.TrimSpace(job.JobID)),
					Target: strings.TrimSpace(job.JobID),
				},
			})
			break
		}
	}
	for _, server := range capabilities.MCP.Servers {
		health := strings.TrimSpace(server.Health)
		switch {
		case !server.Attached:
			remediations = append(remediations, agentLauncherRemediation{
				Category:        "mcp",
				Summary:         "One or more MCP servers are detached. Re-attach them before expecting tools to appear.",
				Detail:          fmt.Sprintf("server=%s health=%s", strings.TrimSpace(server.Name), health),
				RemediationHint: firstNonEmpty(strings.TrimSpace(server.RemediationHint), "Attach the MCP server to restore its tool surface."),
				Action: &agentLauncherRemediationAction{
					Kind:   "attach-mcp",
					Label:  fmt.Sprintf("Attach %s MCP", strings.TrimSpace(server.Name)),
					Target: strings.TrimSpace(server.Name),
				},
			})
			goto runtimeRemediations
		case strings.EqualFold(health, "degraded") || strings.EqualFold(health, "error"):
			remediations = append(remediations, agentLauncherRemediation{
				Category:        "mcp",
				Summary:         "One or more MCP servers are unhealthy. Inspect server detail and refresh its config before expecting tools to appear.",
				Detail:          fmt.Sprintf("server=%s health=%s", strings.TrimSpace(server.Name), health),
				RemediationHint: firstNonEmpty(strings.TrimSpace(server.RemediationHint), "Inspect MCP detail to refresh configuration or disable the server."),
				Action: &agentLauncherRemediationAction{
					Kind:   "inspect-mcp",
					Label:  fmt.Sprintf("Inspect %s MCP", strings.TrimSpace(server.Name)),
					Target: strings.TrimSpace(server.Name),
				},
			})
			goto runtimeRemediations
		}
	}
runtimeRemediations:
	for _, skill := range capabilities.Skills {
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			continue
		}
		if !skill.Enabled {
			remediations = append(remediations, agentLauncherRemediation{
				Category:        "skills",
				Summary:         "One or more installed skills are disabled. Enable them to restore runtime guidance and tools.",
				Detail:          fmt.Sprintf("skill=%s state=disabled", name),
				RemediationHint: firstNonEmpty(strings.TrimSpace(skill.RemediationHint), "Enable the skill to expose its runtime guidance and tools."),
				Action: &agentLauncherRemediationAction{
					Kind:   "enable-skill",
					Label:  fmt.Sprintf("Enable %s", name),
					Target: name,
				},
			})
			break
		}
		if strings.EqualFold(strings.TrimSpace(skill.Health), "degraded") || skill.UpdateAvailable {
			remediations = append(remediations, agentLauncherRemediation{
				Category:        "skills",
				Summary:         "One or more installed skills are degraded. Reinstall them to restore a healthy runtime surface.",
				Detail:          fmt.Sprintf("skill=%s health=%s", name, firstNonEmpty(strings.TrimSpace(skill.Health), "unknown")),
				RemediationHint: firstNonEmpty(strings.TrimSpace(skill.RemediationHint), "Reinstall the skill to restore a healthy runtime surface."),
				Action: &agentLauncherRemediationAction{
					Kind:   "reinstall-skill",
					Label:  fmt.Sprintf("Reinstall %s", name),
					Target: name,
				},
			})
			break
		}
	}
	if session != nil {
		runtimeState := strings.TrimSpace(session.RuntimeState)
		if runtimeState != "" && runtimeState != "running" {
			remediations = append(remediations, agentLauncherRemediation{
				Category: "runtime",
				Summary:  "Managed runtime is not running. Start the agent or inspect the launcher session.",
				Detail:   fmt.Sprintf("state=%s", runtimeState),
				Action: &agentLauncherRemediationAction{
					Kind:  "start-runtime",
					Label: "Start runtime",
				},
			})
		}
	}
	if mediaRuntime != nil && mediaRuntime.Status == "unavailable" {
		remediations = append(remediations, agentLauncherRemediation{
			Category:        "media",
			Summary:         "Media runtime is unavailable. Configure transcription credentials or switch providers.",
			Detail:          strings.TrimSpace(mediaRuntime.Detail),
			RemediationHint: strings.TrimSpace(mediaRuntime.RemediationHint),
		})
	}
	return remediations
}

func agentProviderFromStatus(status AgentState) string {
	if status.Memory == nil {
		return ""
	}
	return ""
}

func buildAgentCronSummary(jobs []CronJob) *agentLauncherCronSummary {
	if len(jobs) == 0 {
		return nil
	}
	summary := &agentLauncherCronSummary{
		Count: len(jobs),
		Jobs:  append([]CronJob(nil), jobs...),
	}
	var lastRunAt time.Time
	var nextRunAt time.Time
	for _, job := range jobs {
		if !job.NextRunAt.IsZero() && (nextRunAt.IsZero() || job.NextRunAt.Before(nextRunAt)) {
			nextRunAt = job.NextRunAt
		}
		if job.LastRunAt != nil && (lastRunAt.IsZero() || job.LastRunAt.After(lastRunAt)) {
			lastRunAt = *job.LastRunAt
			summary.LastResult = strings.TrimSpace(job.LastResult)
		}
	}
	if !nextRunAt.IsZero() {
		summary.NextRunAt = nextRunAt.UTC().Format(time.RFC3339Nano)
	}
	if !lastRunAt.IsZero() {
		summary.LastRunAt = lastRunAt.UTC().Format(time.RFC3339Nano)
	}
	return summary
}
