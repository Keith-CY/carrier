package gateway

import (
	"carrier/baseagent"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func handleWebUIAgents(w http.ResponseWriter, r *http.Request, requestID string, daemon *DaemonClient) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}

	agents, err := daemon.ListAgents(r.Context(), "webui:agents:list", requestID)
	if err != nil {
		writeDaemonAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

func handleWebUIAgent(w http.ResponseWriter, r *http.Request, requestID string, daemon *DaemonClient) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/agents/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "agent path is required"))
		return
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "agent action path is required"))
		return
	}

	agentID := strings.TrimSpace(parts[0])
	action := strings.TrimSpace(parts[1])
	if agentID == "" || action == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "agent id and action are required"))
		return
	}

	switch action {
	case "status":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		statuses, err := daemon.GetStatus(r.Context(), agentID, "webui:agents:status", requestID)
		if err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		if len(statuses) == 0 {
			writeJSON(w, http.StatusNotFound, gatewayErrBody("E_AGENT_NOT_FOUND", fmt.Sprintf("agent %s not found", agentID)))
			return
		}
		writeJSON(w, http.StatusOK, statuses[0])
		return
	case "logs":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		tail := parsePositiveInt(r.URL.Query().Get("tail"), 200)
		logs, err := daemon.GetLogs(r.Context(), agentID, tail, "webui:agents:logs", requestID)
		if err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, logs)
		return
	case "capabilities":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		summary, err := daemon.GetAgentCapabilities(r.Context(), agentID, "webui:agents:capabilities", requestID)
		if err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, summary)
		return
	case "skills":
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
			installed, err := daemon.InstallAgentSkill(r.Context(), agentID, strings.TrimSpace(body.Name), "webui:agents:skills:install", requestID)
			if err != nil {
				writeDaemonAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, installed)
			return
		case strings.EqualFold(skillAction, "update"):
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
			return
		case strings.EqualFold(skillAction, "uninstall"):
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
			removed, err := daemon.UninstallAgentSkill(r.Context(), agentID, strings.TrimSpace(body.Name), "webui:agents:skills:uninstall", requestID)
			if err != nil {
				writeDaemonAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, removed)
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
		return
	case "models":
		switch {
		case len(parts) == 2 && r.Method == http.MethodGet:
			summary, err := currentManagedAgentModelsSummary(agentID)
			if err != nil {
				if errors.Is(err, errManagedAgentInstanceNotFound) {
					writeJSON(w, http.StatusNotFound, gatewayErrBody("E_AGENT_NOT_FOUND", fmt.Sprintf("managed agent %s not found", agentID)))
					return
				}
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load managed agent models", "load managed agent models", err)
				return
			}
			writeJSON(w, http.StatusOK, summary)
			return
		case len(parts) == 3 && strings.EqualFold(strings.TrimSpace(parts[2]), "sync") && r.Method == http.MethodPost:
			summary, err := syncManagedAgentModelsSummary(agentID)
			if err != nil {
				if errors.Is(err, errManagedAgentInstanceNotFound) {
					writeJSON(w, http.StatusNotFound, gatewayErrBody("E_AGENT_NOT_FOUND", fmt.Sprintf("managed agent %s not found", agentID)))
					return
				}
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to sync managed agent models", "sync managed agent models", err)
				return
			}
			writeJSON(w, http.StatusOK, summary)
			return
		case len(parts) == 3 && strings.EqualFold(strings.TrimSpace(parts[2]), "discover") && r.Method == http.MethodGet:
			summary, err := discoverManagedAgentModelsSummary(agentID)
			if err != nil {
				if errors.Is(err, errManagedAgentInstanceNotFound) {
					writeJSON(w, http.StatusNotFound, gatewayErrBody("E_AGENT_NOT_FOUND", fmt.Sprintf("managed agent %s not found", agentID)))
					return
				}
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to discover managed agent models", "discover managed agent models", err)
				return
			}
			writeJSON(w, http.StatusOK, summary)
			return
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
				if errors.Is(err, errManagedAgentInstanceNotFound) {
					writeJSON(w, http.StatusNotFound, gatewayErrBody("E_AGENT_NOT_FOUND", fmt.Sprintf("managed agent %s not found", agentID)))
					return
				}
				if strings.Contains(strings.ToLower(err.Error()), "not found") || strings.Contains(strings.ToLower(err.Error()), "required") || strings.Contains(strings.ToLower(err.Error()), "unavailable") {
					writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", err.Error()))
					return
				}
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to update managed agent default model profile", "update managed agent default model profile", err)
				return
			}
			writeJSON(w, http.StatusOK, summary)
			return
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
				if errors.Is(err, errManagedAgentInstanceNotFound) {
					writeJSON(w, http.StatusNotFound, gatewayErrBody("E_AGENT_NOT_FOUND", fmt.Sprintf("managed agent %s not found", agentID)))
					return
				}
				if strings.Contains(strings.ToLower(err.Error()), "not found") || strings.Contains(strings.ToLower(err.Error()), "required") || strings.Contains(strings.ToLower(err.Error()), "unavailable") {
					writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", err.Error()))
					return
				}
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to update managed agent model profile", "update managed agent model profile", err)
				return
			}
			writeJSON(w, http.StatusOK, summary)
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
	case "mcp":
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
				return
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
				return
			default:
				writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "mcp action not found"))
				return
			}
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
		return
	case "sessions":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		if daemon == nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_COMMAND_FAILED", "daemon client is unavailable"))
			return
		}
		sessions, err := daemon.GetAgentSessions(r.Context(), agentID, 10, "webui:agents:sessions", requestID)
		if err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
		return
	case "subagents":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		if daemon == nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_COMMAND_FAILED", "daemon client is unavailable"))
			return
		}
		if len(parts) == 2 {
			jobs, err := daemon.GetAgentSubagentJobs(r.Context(), agentID, parsePositiveInt(r.URL.Query().Get("limit"), 10), "webui:agents:subagents:list", requestID)
			if err != nil {
				writeDaemonAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
			return
		}
		if len(parts) == 3 {
			jobID := strings.TrimSpace(parts[2])
			if jobID == "" {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "delegation job id is required"))
				return
			}
			job, err := daemon.GetAgentSubagentJob(r.Context(), agentID, jobID, "webui:agents:subagents:detail", requestID)
			if err != nil {
				writeDaemonAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, job)
			return
		}
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "unsupported delegation job path"))
		return
	case "chat":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		if daemon == nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_COMMAND_FAILED", "daemon client is unavailable"))
			return
		}
		var body struct {
			Provider   string `json:"provider"`
			ModelAlias string `json:"modelAlias,omitempty"`
			Model      string `json:"model,omitempty"`
			Message    string `json:"message"`
			SessionID  string `json:"sessionId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid JSON body"))
			return
		}
		message := strings.TrimSpace(body.Message)
		if message == "" {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "message is required"))
			return
		}
		result, err := daemon.ChatAgent(r.Context(), agentID, strings.TrimSpace(body.Provider), message, strings.TrimSpace(body.SessionID), strings.TrimSpace(body.ModelAlias), strings.TrimSpace(body.Model), "webui:agents:chat", requestID)
		if err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	case "cron":
		if daemon == nil {
			writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_COMMAND_FAILED", "daemon client is unavailable"))
			return
		}
		switch {
		case len(parts) == 2 && r.Method == http.MethodGet:
			jobs, err := daemon.ListCronJobs(r.Context(), agentID, "", "webui:agents:cron:list", requestID)
			if err != nil {
				writeDaemonAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
			return
		case len(parts) == 2 && r.Method == http.MethodPost:
			var body struct {
				Provider  string    `json:"provider,omitempty"`
				SessionID string    `json:"sessionId,omitempty"`
				Message   string    `json:"message"`
				NextRunAt time.Time `json:"nextRunAt,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid JSON body"))
				return
			}
			if strings.TrimSpace(body.Message) == "" {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "message is required"))
				return
			}
			job, err := daemon.ScheduleCronJob(r.Context(), baseagent.CronJob{
				AgentID:    agentID,
				SessionKey: baseagent.ResolveSessionKey(firstNonEmpty(strings.TrimSpace(body.Provider), "managed-agent"), firstNonEmpty(strings.TrimSpace(body.SessionID), agentID+"-cron")),
				Prompt:     strings.TrimSpace(body.Message),
				NextRunAt:  body.NextRunAt,
			}, "webui:agents:cron:schedule", requestID)
			if err != nil {
				writeDaemonAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, job)
			return
		case len(parts) == 4 && strings.EqualFold(strings.TrimSpace(parts[3]), "cancel") && r.Method == http.MethodPost:
			jobID := strings.TrimSpace(parts[2])
			if jobID == "" {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "cron job id is required"))
				return
			}
			job, err := daemon.CancelCronJob(r.Context(), jobID, "webui:agents:cron:cancel", requestID)
			if err != nil {
				writeDaemonAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, job)
			return
		case len(parts) == 4 && strings.EqualFold(strings.TrimSpace(parts[3]), "pause") && r.Method == http.MethodPost:
			jobID := strings.TrimSpace(parts[2])
			if jobID == "" {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "cron job id is required"))
				return
			}
			job, err := daemon.PauseCronJob(r.Context(), jobID, "webui:agents:cron:pause", requestID)
			if err != nil {
				writeDaemonAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, job)
			return
		case len(parts) == 4 && strings.EqualFold(strings.TrimSpace(parts[3]), "resume") && r.Method == http.MethodPost:
			jobID := strings.TrimSpace(parts[2])
			if jobID == "" {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "cron job id is required"))
				return
			}
			job, err := daemon.ResumeCronJob(r.Context(), jobID, "webui:agents:cron:resume", requestID)
			if err != nil {
				writeDaemonAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, job)
			return
		case len(parts) == 4 && strings.EqualFold(strings.TrimSpace(parts[3]), "run") && r.Method == http.MethodPost:
			jobID := strings.TrimSpace(parts[2])
			if jobID == "" {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "cron job id is required"))
				return
			}
			job, err := daemon.RunCronJob(r.Context(), jobID, "webui:agents:cron:run", requestID)
			if err != nil {
				writeDaemonAPIError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, job)
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
	case "launcher":
		handleAgentLauncher(w, r, requestID, agentID, daemon)
		return
	case "install", "start", "stop", "uninstall":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var err error
		warning := ""
		actor := "webui:agents:" + action
		switch action {
		case "install":
			installOpts := InstallAgentOptions{}
			if inst, ok := latestManagedInstanceForAgent(agentID); ok {
				installOpts.Isolation = inst.Isolation
			}
			err = daemon.InstallAgentWithOptions(r.Context(), agentID, installOpts, actor, requestID)
		case "start":
			startOpts := StartAgentOptions{}
			if inst, ok := latestManagedInstanceForAgent(agentID); ok {
				startOpts.Isolation = inst.Isolation
			}
			err = daemon.StartAgentWithOptions(r.Context(), agentID, startOpts, actor, requestID)
		case "stop":
			err = daemon.StopAgent(r.Context(), agentID, actor, requestID)
		case "uninstall":
			err = daemon.UninstallAgent(r.Context(), agentID, actor, requestID)
		}
		if err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		if action == "start" {
			warning = managedAgentMCPReconcileWarning(reconcileManagedAgentMCPState(r.Context(), daemon, agentID, requestID))
		}
		if syncErr := syncManagedInstanceByAgentAction(r, agentID, action); syncErr != nil {
			writeStatePersistenceError(w, requestID, action, agentID, "", syncErr)
			return
		}
		payload := map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"agentId":   agentID,
			"action":    action,
		}
		if warning != "" {
			payload["warning"] = warning
		}
		writeJSON(w, http.StatusOK, payload)
		return
	default:
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported agent action"))
		return
	}
}

func writeDaemonAPIError(w http.ResponseWriter, err error) {
	if de, ok := err.(*DaemonClientError); ok {
		status, code, message := mapDaemonErrorToExternal(de.Code)
		log.Printf("[gateway] daemon API error code=%s detail=%s", code, RedactErrorMessage(de.Message))
		writeJSON(w, status, gatewayErrBody(code, message))
		return
	}
	writeInternalGatewayError(w, http.StatusBadGateway, "E_COMMAND_FAILED", "daemon command failed", "daemon API request failed", err)
}

func syncManagedInstanceByAgentAction(r *http.Request, agentID, action string) error {
	instances, path, err := loadManagedInstances()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	idx := findManagedInstanceIndexByAgentID(instances, agentID)

	switch action {
	case "install":
		if idx >= 0 {
			if strings.TrimSpace(instances[idx].RuntimeState) == "" {
				instances[idx].RuntimeState = "stopped"
			}
			instances[idx].UpdatedAt = now
			return saveManagedInstances(path, instances)
		}
		instanceID := agentID + "-default"
		if findManagedInstanceIndex(instances, instanceID) >= 0 {
			generatedID, genErr := generateManagedInstanceID(agentID)
			if genErr != nil {
				return genErr
			}
			instanceID = generatedID
		}
		instances = append(instances, managedAgentInstance{
			ID:           instanceID,
			Type:         agentID,
			AgentID:      agentID,
			GatewayURL:   gatewayURLFromRequest(r),
			RuntimeState: "stopped",
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		return saveManagedInstances(path, instances)
	case "start", "stop":
		targetState := "stopped"
		if action == "start" {
			targetState = "running"
		}
		if idx >= 0 {
			instances[idx].RuntimeState = targetState
			instances[idx].UpdatedAt = now
			return saveManagedInstances(path, instances)
		}
		instanceID := agentID + "-default"
		if findManagedInstanceIndex(instances, instanceID) >= 0 {
			generatedID, genErr := generateManagedInstanceID(agentID)
			if genErr != nil {
				return genErr
			}
			instanceID = generatedID
		}
		instances = append(instances, managedAgentInstance{
			ID:           instanceID,
			Type:         agentID,
			AgentID:      agentID,
			GatewayURL:   gatewayURLFromRequest(r),
			RuntimeState: targetState,
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		return saveManagedInstances(path, instances)
	case "uninstall":
		if idx < 0 {
			return nil
		}
		if err := cleanupManagedInstanceFiles(instances[idx]); err != nil {
			// Non-critical: daemon uninstall already succeeded; keep instance-state cleanup best-effort.
			log.Printf("[gateway] cleanup managed instance files failed (instance=%s): %s", instances[idx].ID, RedactErrorMessage(err.Error()))
		}
		instances = append(instances[:idx], instances[idx+1:]...)
		return saveManagedInstances(path, instances)
	default:
		return nil
	}
}
