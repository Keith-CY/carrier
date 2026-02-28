package gateway

import (
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
	case "install", "start", "stop", "uninstall":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var err error
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
		if syncErr := syncManagedInstanceByAgentAction(r, agentID, action); syncErr != nil {
			writeStatePersistenceError(w, requestID, action, agentID, "", syncErr)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"agentId":   agentID,
			"action":    action,
		})
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
