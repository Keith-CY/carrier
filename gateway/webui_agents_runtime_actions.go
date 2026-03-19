package gateway

import (
	"carrier/baseagent"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func handleWebUIAgentSessionsAction(w http.ResponseWriter, r *http.Request, requestID, agentID string, daemon *DaemonClient) {
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
}

func handleWebUIAgentSubagentsAction(w http.ResponseWriter, r *http.Request, requestID, agentID string, parts []string, daemon *DaemonClient) {
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
}

func handleWebUIAgentChatAction(w http.ResponseWriter, r *http.Request, requestID, agentID string, daemon *DaemonClient) {
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
}

func handleWebUIAgentMediaAction(w http.ResponseWriter, r *http.Request, requestID, agentID string, parts []string, daemon *DaemonClient) {
	if len(parts) != 3 || !strings.EqualFold(strings.TrimSpace(parts[2]), "speak") {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "unsupported media action"))
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
		Text   string `json:"text"`
		Voice  string `json:"voice,omitempty"`
		Format string `json:"format,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid JSON body"))
		return
	}
	result, err := daemon.SpeakAgentMedia(r.Context(), agentID, strings.TrimSpace(body.Text), strings.TrimSpace(body.Voice), strings.TrimSpace(body.Format), "webui:agents:media:speak", requestID)
	if err != nil {
		writeDaemonAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func handleWebUIAgentCronAction(w http.ResponseWriter, r *http.Request, requestID, agentID string, parts []string, daemon *DaemonClient) {
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
	case len(parts) == 4 && r.Method == http.MethodPost:
		handleWebUIAgentCronJobAction(w, r, requestID, parts, daemon)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
	}
}

func handleWebUIAgentCronJobAction(w http.ResponseWriter, r *http.Request, requestID string, parts []string, daemon *DaemonClient) {
	jobID := strings.TrimSpace(parts[2])
	if jobID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "cron job id is required"))
		return
	}
	action := strings.ToLower(strings.TrimSpace(parts[3]))
	var (
		job any
		err error
	)
	switch action {
	case "cancel":
		job, err = daemon.CancelCronJob(r.Context(), jobID, "webui:agents:cron:cancel", requestID)
	case "pause":
		job, err = daemon.PauseCronJob(r.Context(), jobID, "webui:agents:cron:pause", requestID)
	case "resume":
		job, err = daemon.ResumeCronJob(r.Context(), jobID, "webui:agents:cron:resume", requestID)
	case "run":
		job, err = daemon.RunCronJob(r.Context(), jobID, "webui:agents:cron:run", requestID)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	if err != nil {
		writeDaemonAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func handleWebUIAgentLifecycleAction(w http.ResponseWriter, r *http.Request, requestID, agentID, action string, daemon *DaemonClient) {
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
	payload := map[string]any{
		"requestId": requestID,
		"result":    "ok",
		"agentId":   agentID,
		"action":    action,
	}
	if warning != "" {
		payload["warning"] = warning
	}
	writeJSON(w, http.StatusOK, payload)
}
