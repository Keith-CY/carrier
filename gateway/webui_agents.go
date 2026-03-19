package gateway

import (
	"fmt"
	"net/http"
	"strings"
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
		handleWebUIAgentSkillsAction(w, r, requestID, agentID, parts, daemon)
		return
	case "models":
		handleWebUIAgentModelsAction(w, r, requestID, agentID, parts)
		return
	case "mcp":
		handleWebUIAgentMCPAction(w, r, requestID, agentID, parts, daemon)
		return
	case "sessions":
		handleWebUIAgentSessionsAction(w, r, requestID, agentID, daemon)
		return
	case "subagents":
		handleWebUIAgentSubagentsAction(w, r, requestID, agentID, parts, daemon)
		return
	case "chat":
		handleWebUIAgentChatAction(w, r, requestID, agentID, daemon)
		return
	case "media":
		handleWebUIAgentMediaAction(w, r, requestID, agentID, parts, daemon)
		return
	case "cron":
		handleWebUIAgentCronAction(w, r, requestID, agentID, parts, daemon)
		return
	case "launcher":
		handleAgentLauncher(w, r, requestID, agentID, daemon)
		return
	case "install", "start", "stop", "uninstall":
		handleWebUIAgentLifecycleAction(w, r, requestID, agentID, action, daemon)
		return
	default:
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported agent action"))
		return
	}
}
