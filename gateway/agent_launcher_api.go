package gateway

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type agentLauncherSummary struct {
	AgentID           string                     `json:"agentId"`
	Status            AgentState                 `json:"status"`
	Heartbeat         *AgentHeartbeat            `json:"heartbeat,omitempty"`
	Memory            *AgentMemoryState          `json:"memory,omitempty"`
	Capabilities      AgentCapabilitySummary     `json:"capabilities"`
	ProviderReadiness agentProviderReadiness     `json:"providerReadiness"`
	ModelSurface      *agentLauncherModelSurface `json:"modelSurface,omitempty"`
	Cron              *agentLauncherCronSummary  `json:"cron,omitempty"`
	Session           *agentLauncherSession      `json:"session,omitempty"`
}

type agentLauncherModelSurface struct {
	DefaultProfile string                             `json:"defaultProfile,omitempty"`
	Profiles       []agentLauncherModelSurfaceProfile `json:"profiles,omitempty"`
}

type agentLauncherModelSurfaceProfile struct {
	ProfileName    string `json:"profileName,omitempty"`
	ModelAlias     string `json:"modelAlias,omitempty"`
	ModelID        string `json:"modelId,omitempty"`
	ProviderID     string `json:"providerId,omitempty"`
	ProviderKey    string `json:"providerKey,omitempty"`
	ProtocolFamily string `json:"protocolFamily,omitempty"`
	BaseURL        string `json:"baseUrl,omitempty"`
	AuthMethod     string `json:"authMethod,omitempty"`
	FallbackGroup  string `json:"fallbackGroup,omitempty"`
	AliasGroupSize int    `json:"aliasGroupSize,omitempty"`
	Primary        bool   `json:"primary,omitempty"`
}

type agentLauncherCronSummary struct {
	Count      int       `json:"count"`
	NextRunAt  string    `json:"nextRunAt,omitempty"`
	LastRunAt  string    `json:"lastRunAt,omitempty"`
	LastResult string    `json:"lastResult,omitempty"`
	Jobs       []CronJob `json:"jobs,omitempty"`
}

type agentProviderReadiness struct {
	Provider             string `json:"provider,omitempty"`
	AuthMode             string `json:"authMode,omitempty"`
	CredentialBackend    string `json:"credentialBackend,omitempty"`
	CredentialConfigured bool   `json:"credentialConfigured"`
	Ready                bool   `json:"ready"`
}

type agentLauncherSession struct {
	InstanceID   string `json:"instanceId"`
	Type         string `json:"type,omitempty"`
	Channel      string `json:"channel,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Isolation    bool   `json:"isolation,omitempty"`
	RuntimeState string `json:"runtimeState,omitempty"`
	GatewayURL   string `json:"gatewayUrl,omitempty"`
	PairRequired bool   `json:"pairRequired,omitempty"`
	PairedChatID string `json:"pairedChatId,omitempty"`
	Workspace    string `json:"workspace,omitempty"`
	ConfigPath   string `json:"configPath,omitempty"`
	RecordPath   string `json:"recordPath,omitempty"`
	CreatedAt    string `json:"createdAt,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
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
	if inst, ok := latestManagedInstanceForAgent(agentID); ok {
		session = &agentLauncherSession{
			InstanceID:   strings.TrimSpace(inst.ID),
			Type:         strings.TrimSpace(inst.Type),
			Channel:      strings.TrimSpace(inst.Channel),
			Provider:     strings.TrimSpace(inst.Provider),
			Isolation:    inst.Isolation,
			RuntimeState: strings.TrimSpace(inst.RuntimeState),
			GatewayURL:   strings.TrimSpace(inst.GatewayURL),
			PairRequired: inst.PairRequired,
			PairedChatID: strings.TrimSpace(inst.PairedChatID),
			Workspace:    strings.TrimSpace(inst.Workspace),
			ConfigPath:   strings.TrimSpace(inst.ConfigPath),
			RecordPath:   strings.TrimSpace(inst.RecordPath),
			CreatedAt:    strings.TrimSpace(inst.CreatedAt),
			UpdatedAt:    strings.TrimSpace(inst.UpdatedAt),
		}
		readiness = buildAgentProviderReadiness(inst.Provider)
		modelSurface = buildAgentLauncherModelSurface(inst.ModelSurface)
	}
	var cronSummary *agentLauncherCronSummary
	if jobs, err := daemon.ListCronJobs(r.Context(), agentID, "", "webui:agents:launcher", requestID); err == nil && len(jobs) > 0 {
		cronSummary = buildAgentCronSummary(jobs)
	}

	status := statuses[0]
	if readiness.Provider == "" {
		readiness = buildAgentProviderReadiness(agentProviderFromStatus(status))
	}

	writeJSON(w, http.StatusOK, agentLauncherSummary{
		AgentID:           agentID,
		Status:            status,
		Heartbeat:         status.Heartbeat,
		Memory:            status.Memory,
		Capabilities:      capabilities,
		ProviderReadiness: readiness,
		ModelSurface:      modelSurface,
		Cron:              cronSummary,
		Session:           session,
	})
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
			ProfileName:    strings.TrimSpace(profile.ProfileName),
			ModelAlias:     strings.TrimSpace(profile.ModelAlias),
			ModelID:        strings.TrimSpace(profile.ModelID),
			ProviderID:     strings.TrimSpace(profile.ProviderID),
			ProviderKey:    strings.TrimSpace(profile.ProviderKey),
			ProtocolFamily: strings.TrimSpace(profile.ProtocolFamily),
			BaseURL:        strings.TrimSpace(profile.BaseURL),
			AuthMethod:     strings.TrimSpace(profile.AuthMethod),
			FallbackGroup:  group,
			AliasGroupSize: aliasGroupSize,
			Primary:        primary,
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
