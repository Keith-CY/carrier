package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

func handleWebUIAgentSkillsAction(w http.ResponseWriter, r *http.Request, requestID, agentID string, parts []string, daemon *DaemonClient) {
	if len(parts) != 3 {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "skill action or name is required"))
		return
	}
	skillAction := strings.TrimSpace(parts[2])
	switch {
	case strings.EqualFold(skillAction, "search"):
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		if daemon == nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_COMMAND_FAILED", "daemon client is unavailable"))
			return
		}
		skills, err := daemon.SearchAgentSkills(r.Context(), agentID, strings.TrimSpace(r.URL.Query().Get("q")), "webui:agents:skills:search", requestID)
		if err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"skills": skills})
		return
	case strings.EqualFold(skillAction, "install"):
		handleWebUIAgentNamedSkillMutation(w, r, requestID, agentID, daemon, "install")
		return
	case strings.EqualFold(skillAction, "reinstall"):
		handleWebUIAgentNamedSkillMutation(w, r, requestID, agentID, daemon, "reinstall")
		return
	case strings.EqualFold(skillAction, "update"):
		handleWebUIAgentSkillUpdate(w, r, requestID, agentID, daemon)
		return
	case strings.EqualFold(skillAction, "uninstall"):
		handleWebUIAgentNamedSkillMutation(w, r, requestID, agentID, daemon, "uninstall")
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	if daemon == nil {
		writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_COMMAND_FAILED", "daemon client is unavailable"))
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid JSON body"))
		return
	}
	summary, err := daemon.SetAgentSkillEnabled(r.Context(), agentID, skillAction, body.Enabled, "webui:agents:skills", requestID)
	if err != nil {
		writeDaemonAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func handleWebUIAgentNamedSkillMutation(w http.ResponseWriter, r *http.Request, requestID, agentID string, daemon *DaemonClient, action string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	if daemon == nil {
		writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_COMMAND_FAILED", "daemon client is unavailable"))
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid JSON body"))
		return
	}
	name := strings.TrimSpace(body.Name)
	var (
		out any
		err error
	)
	switch action {
	case "install":
		out, err = daemon.InstallAgentSkill(r.Context(), agentID, name, "webui:agents:skills:install", requestID)
	case "reinstall":
		out, err = daemon.ReinstallAgentSkill(r.Context(), agentID, name, "webui:agents:skills:reinstall", requestID)
	case "uninstall":
		out, err = daemon.UninstallAgentSkill(r.Context(), agentID, name, "webui:agents:skills:uninstall", requestID)
	}
	if err != nil {
		writeDaemonAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func handleWebUIAgentSkillUpdate(w http.ResponseWriter, r *http.Request, requestID, agentID string, daemon *DaemonClient) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	if daemon == nil {
		writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_COMMAND_FAILED", "daemon client is unavailable"))
		return
	}
	var body struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid JSON body"))
		return
	}
	updated, err := daemon.UpdateAgentSkill(r.Context(), agentID, strings.TrimSpace(body.Name), strings.TrimSpace(body.Version), "webui:agents:skills:update", requestID)
	if err != nil {
		writeDaemonAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func handleWebUIAgentModelsAction(w http.ResponseWriter, r *http.Request, requestID, agentID string, parts []string) {
	switch {
	case len(parts) == 2 && r.Method == http.MethodGet:
		summary, err := currentManagedAgentModelsSummary(agentID)
		if err != nil {
			writeManagedAgentModelsError(w, agentID, err, "failed to load managed agent models", "load managed agent models")
			return
		}
		writeJSON(w, http.StatusOK, summary)
	case len(parts) == 3 && strings.EqualFold(strings.TrimSpace(parts[2]), "sync") && r.Method == http.MethodPost:
		summary, err := syncManagedAgentModelsSummary(agentID)
		if err != nil {
			writeManagedAgentModelsError(w, agentID, err, "failed to sync managed agent models", "sync managed agent models")
			return
		}
		writeJSON(w, http.StatusOK, summary)
	case len(parts) == 3 && strings.EqualFold(strings.TrimSpace(parts[2]), "discover") && r.Method == http.MethodGet:
		summary, err := discoverManagedAgentModelsSummary(agentID)
		if err != nil {
			writeManagedAgentModelsError(w, agentID, err, "failed to discover managed agent models", "discover managed agent models")
			return
		}
		writeJSON(w, http.StatusOK, summary)
	case len(parts) == 3 && strings.EqualFold(strings.TrimSpace(parts[2]), "default") && r.Method == http.MethodPost:
		var body struct {
			ProfileName string `json:"profileName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid JSON body"))
			return
		}
		summary, err := updateManagedAgentModelsDefaultSummary(agentID, strings.TrimSpace(body.ProfileName))
		if err != nil {
			writeManagedAgentModelsError(w, agentID, err, "failed to update managed agent default model profile", "update managed agent default model profile")
			return
		}
		writeJSON(w, http.StatusOK, summary)
	case len(parts) == 3 && strings.EqualFold(strings.TrimSpace(parts[2]), "profile") && r.Method == http.MethodPost:
		var body struct {
			ProfileName      string `json:"profileName"`
			ModelAlias       string `json:"modelAlias,omitempty"`
			ModelID          string `json:"modelId,omitempty"`
			ProviderID       string `json:"providerId,omitempty"`
			BaseURL          string `json:"baseUrl,omitempty"`
			AuthMethod       string `json:"authMethod,omitempty"`
			TimeoutMs        int    `json:"timeoutMs,omitempty"`
			RetryBudget      int    `json:"retryBudget,omitempty"`
			FallbackStrategy string `json:"fallbackStrategy,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid JSON body"))
			return
		}
		summary, err := updateManagedAgentModelProfileSummary(agentID, managedAgentModelProfileUpdate{
			ProfileName:      strings.TrimSpace(body.ProfileName),
			ModelAlias:       strings.TrimSpace(body.ModelAlias),
			ModelID:          strings.TrimSpace(body.ModelID),
			ProviderID:       strings.TrimSpace(body.ProviderID),
			BaseURL:          strings.TrimSpace(body.BaseURL),
			AuthMethod:       strings.TrimSpace(body.AuthMethod),
			TimeoutMs:        body.TimeoutMs,
			RetryBudget:      body.RetryBudget,
			FallbackStrategy: strings.TrimSpace(body.FallbackStrategy),
		})
		if err != nil {
			writeManagedAgentModelsError(w, agentID, err, "failed to update managed agent model profile", "update managed agent model profile")
			return
		}
		writeJSON(w, http.StatusOK, summary)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
	}
}

func writeManagedAgentModelsError(w http.ResponseWriter, agentID string, err error, publicMessage, logMessage string) {
	if errors.Is(err, errManagedAgentInstanceNotFound) {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_AGENT_NOT_FOUND", fmt.Sprintf("managed agent %s not found", agentID)))
		return
	}
	errText := strings.ToLower(err.Error())
	if strings.Contains(errText, "not found") || strings.Contains(errText, "required") || strings.Contains(errText, "unavailable") {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", err.Error()))
		return
	}
	writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", publicMessage, logMessage, err)
}

func handleWebUIAgentMCPAction(w http.ResponseWriter, r *http.Request, requestID, agentID string, parts []string, daemon *DaemonClient) {
	if len(parts) < 3 {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "mcp server name is required"))
		return
	}
	if daemon == nil {
		writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_COMMAND_FAILED", "daemon client is unavailable"))
		return
	}
	serverName := strings.TrimSpace(parts[2])
	subaction := ""
	if len(parts) > 3 {
		subaction = strings.ToLower(strings.TrimSpace(parts[3]))
	}
	if subaction != "" {
		handleWebUIAgentMCPSubaction(w, r, requestID, agentID, serverName, subaction, daemon)
		return
	}

	switch r.Method {
	case http.MethodGet:
		detail, err := daemon.GetAgentMCPServerDetail(r.Context(), agentID, serverName, "webui:agents:mcp:detail", requestID)
		if err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	case http.MethodPost:
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid JSON body"))
			return
		}
		summary, err := daemon.SetAgentMCPServerEnabled(r.Context(), agentID, serverName, body.Enabled, "webui:agents:mcp", requestID)
		if err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		if persistErr := persistManagedAgentMCPSummary(agentID, summary.MCP); persistErr != nil {
			writeStatePersistenceError(w, requestID, "mcp_toggle", agentID, "", persistErr)
			return
		}
		writeJSON(w, http.StatusOK, summary)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
	}
}

func handleWebUIAgentMCPSubaction(w http.ResponseWriter, r *http.Request, requestID, agentID, serverName, subaction string, daemon *DaemonClient) {
	switch subaction {
	case "attach", "detach":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		detail, err := daemon.SetAgentMCPServerAttached(r.Context(), agentID, serverName, subaction == "attach", "webui:agents:mcp:attach", requestID)
		if err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		if persistErr := persistManagedAgentMCPServerDetail(agentID, detail); persistErr != nil {
			writeStatePersistenceError(w, requestID, "mcp_"+subaction, agentID, "", persistErr)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	case "config":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var body struct {
			Config string `json:"config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid JSON body"))
			return
		}
		detail, err := daemon.UpdateAgentMCPServerConfig(r.Context(), agentID, serverName, body.Config, "webui:agents:mcp:config", requestID)
		if err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		if persistErr := persistManagedAgentMCPServerDetail(agentID, detail); persistErr != nil {
			writeStatePersistenceError(w, requestID, "mcp_config", agentID, "", persistErr)
			return
		}
		writeJSON(w, http.StatusOK, detail)
	default:
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "mcp action not found"))
	}
}
