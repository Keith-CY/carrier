package gateway

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func writeDaemonAPIError(w http.ResponseWriter, err error) {
	if de, ok := err.(*DaemonClientError); ok {
		status, code, message := mapDaemonErrorToExternal(de.Code)
		detail := strings.TrimSpace(RedactErrorMessage(de.Message))
		log.Printf("[gateway] daemon API error code=%s detail=%s", code, detail)
		if message == "daemon command failed" && detail != "" {
			message = fmt.Sprintf("%s: %s", message, detail)
		}
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
			log.Printf("[gateway] cleanup managed instance files failed (instance=%s): %s", instances[idx].ID, RedactErrorMessage(err.Error()))
		}
		instances = append(instances[:idx], instances[idx+1:]...)
		return saveManagedInstances(path, instances)
	default:
		return nil
	}
}
