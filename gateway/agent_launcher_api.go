package gateway

import (
	"fmt"
	"net/http"
	"strings"
)

type agentLauncherSummary struct {
	AgentID           string                 `json:"agentId"`
	Status            AgentState             `json:"status"`
	Heartbeat         *AgentHeartbeat        `json:"heartbeat,omitempty"`
	Memory            *AgentMemoryState      `json:"memory,omitempty"`
	Capabilities      AgentCapabilitySummary `json:"capabilities"`
	ProviderReadiness agentProviderReadiness `json:"providerReadiness"`
	Session           *agentLauncherSession  `json:"session,omitempty"`
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
		Session:           session,
	})
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
