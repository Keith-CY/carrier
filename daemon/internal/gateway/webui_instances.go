package gateway

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

func handleWebUIInstances(w http.ResponseWriter, r *http.Request, requestID string, daemon *DaemonClient) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	instances, path, err := loadManagedInstances()
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load managed instances", "load managed instances", err)
		return
	}
	persistDirty := false
	if len(instances) > 0 {
		updated := mergeManagedRuntimeState(r, daemon, instances, requestID)
		if updated {
			persistDirty = true
		}
	}
	instances, updated, backfillErr := backfillManagedInstancesFromDaemon(r, daemon, instances, requestID)
	if backfillErr != nil {
		log.Printf("[gateway] managed instance backfill skipped: %s", RedactErrorMessage(backfillErr.Error()))
	} else if updated {
		persistDirty = true
	}
	if updatePairingStateFromLogs(r, daemon, instances, requestID) {
		persistDirty = true
	}
	if persistDirty {
		if saveErr := saveManagedInstances(path, instances); saveErr != nil {
			log.Printf("[gateway] managed instances sync persist skipped: %s", RedactErrorMessage(saveErr.Error()))
		}
	}

	sort.Slice(instances, func(i, j int) bool {
		ti := parseManagedTimestamp(instances[i].UpdatedAt)
		tj := parseManagedTimestamp(instances[j].UpdatedAt)
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return strings.ToLower(strings.TrimSpace(instances[i].ID)) < strings.ToLower(strings.TrimSpace(instances[j].ID))
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"instances": instances,
	})
}

func updatePairingStateFromLogs(r *http.Request, daemon *DaemonClient, instances []managedAgentInstance, requestID string) bool {
	if daemon == nil || len(instances) == 0 {
		return false
	}
	changedAny := false
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range instances {
		inst := &instances[i]
		agentID := strings.TrimSpace(inst.AgentID)
		channel := strings.TrimSpace(inst.Channel)
		if agentID == "" {
			agentID = strings.TrimSpace(inst.Type)
		}
		if !strings.EqualFold(agentID, "picoclaw") || !strings.EqualFold(channel, "telegram") {
			continue
		}
		if !inst.PairRequired && strings.TrimSpace(inst.PairedChatID) != "" {
			continue
		}

		logs, err := daemon.GetLogs(r.Context(), "picoclaw", 120, "webui:instances:pair-state", requestID)
		if err != nil || logs == nil {
			continue
		}
		changed := false
		if pairCode := extractPairCode(logs.Lines); pairCode != "" && pairCode != strings.TrimSpace(inst.PairCode) {
			inst.PairCode = pairCode
			changed = true
		}
		if pairedChatID := extractPairedTelegramChatID(logs.Lines); pairedChatID != "" {
			if pairedChatID != strings.TrimSpace(inst.PairedChatID) {
				inst.PairedChatID = pairedChatID
				changed = true
			}
			if inst.PairRequired {
				inst.PairRequired = false
				changed = true
			}
			if strings.EqualFold(strings.TrimSpace(inst.RuntimeState), "pending_pair") {
				inst.RuntimeState = "running"
				changed = true
			}
		} else {
			if !inst.PairRequired {
				inst.PairRequired = true
				changed = true
			}
			if strings.EqualFold(strings.TrimSpace(inst.RuntimeState), "running") || strings.TrimSpace(inst.RuntimeState) == "" {
				inst.RuntimeState = "pending_pair"
				changed = true
			}
		}
		if changed {
			inst.UpdatedAt = now
			changedAny = true
		}
	}
	return changedAny
}

func backfillManagedInstancesFromDaemon(r *http.Request, daemon *DaemonClient, instances []managedAgentInstance, requestID string) ([]managedAgentInstance, bool, error) {
	if daemon == nil {
		return instances, false, nil
	}
	agents, err := daemon.ListAgents(r.Context(), "webui:instances:backfill", requestID)
	if err != nil {
		return instances, false, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	changed := false
	for _, agent := range agents {
		agentID := strings.TrimSpace(agent.ID)
		if agentID == "" {
			continue
		}
		installState := strings.ToLower(strings.TrimSpace(agent.InstallState))
		runtimeState := strings.TrimSpace(agent.Runtime)
		shouldTrack := installState == "installed" || strings.EqualFold(runtimeState, "running") || strings.EqualFold(runtimeState, "healthy")
		if !shouldTrack {
			continue
		}
		idx := findManagedInstanceIndexByAgentID(instances, agentID)
		if idx >= 0 {
			updated := false
			if strings.TrimSpace(instances[idx].Type) == "" {
				instances[idx].Type = agentID
				updated = true
			}
			if strings.TrimSpace(instances[idx].GatewayURL) == "" {
				instances[idx].GatewayURL = gatewayURLFromRequest(r)
				updated = true
			}
			if runtimeState != "" && strings.TrimSpace(instances[idx].RuntimeState) != runtimeState {
				instances[idx].RuntimeState = runtimeState
				updated = true
			}
			if updated {
				instances[idx].UpdatedAt = now
				changed = true
			}
			continue
		}

		instanceID := agentID + "-default"
		if findManagedInstanceIndex(instances, instanceID) >= 0 {
			generatedID, genErr := generateManagedInstanceID(agentID)
			if genErr != nil {
				return instances, changed, genErr
			}
			instanceID = generatedID
		}
		instances = append(instances, managedAgentInstance{
			ID:           instanceID,
			Type:         agentID,
			AgentID:      agentID,
			GatewayURL:   gatewayURLFromRequest(r),
			RuntimeState: defaultRuntimeState(runtimeState, installState),
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		changed = true
	}
	return instances, changed, nil
}

func defaultRuntimeState(runtimeState, installState string) string {
	rs := strings.TrimSpace(runtimeState)
	if rs != "" {
		return rs
	}
	if strings.EqualFold(strings.TrimSpace(installState), "installed") {
		return "stopped"
	}
	return "unknown"
}

func handleWebUIInstance(w http.ResponseWriter, r *http.Request, requestID string, daemon *DaemonClient) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/instances/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "instance path is required"))
		return
	}
	parts := strings.Split(trimmed, "/")
	instanceID := strings.TrimSpace(parts[0])
	if instanceID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "instance id is required"))
		return
	}

	instances, path, err := loadManagedInstances()
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load managed instances", "load managed instance by id", err)
		return
	}
	idx := findManagedInstanceIndex(instances, instanceID)
	if idx < 0 {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_INSTANCE_NOT_FOUND", fmt.Sprintf("instance %s not found", instanceID)))
		return
	}
	inst := instances[idx]

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"instance":  inst,
		})
		return
	}

	action := strings.TrimSpace(parts[1])
	switch action {
	case "logs":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		tail := parsePositiveInt(r.URL.Query().Get("tail"), 200)
		logs, err := daemon.GetLogs(r.Context(), inst.AgentID, tail, "webui:instances:logs", requestID)
		if err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, logs)
		return
	case "start", "stop", "uninstall":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
	default:
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported instance action"))
		return
	}

	actor := "webui:instances:" + action
	warning := ""
	switch action {
	case "start":
		if err := daemon.StartAgent(r.Context(), inst.AgentID, actor, requestID); err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		instances[idx].RuntimeState = "running"
		instances[idx].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(path, instances); err != nil {
			writeStatePersistenceError(w, requestID, action, inst.AgentID, inst.ID, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"action":    action,
			"instance":  instances[idx],
		})
		return
	case "stop":
		if err := daemon.StopAgent(r.Context(), inst.AgentID, actor, requestID); err != nil {
			writeDaemonAPIError(w, err)
			return
		}
		instances[idx].RuntimeState = "stopped"
		instances[idx].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveManagedInstances(path, instances); err != nil {
			writeStatePersistenceError(w, requestID, action, inst.AgentID, inst.ID, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"action":    action,
			"instance":  instances[idx],
		})
		return
	case "uninstall":
		_ = daemon.StopAgent(r.Context(), inst.AgentID, actor, requestID)
		if err := daemon.UninstallAgent(r.Context(), inst.AgentID, actor, requestID); err != nil && !isDaemonAgentNotFound(err) {
			writeDaemonAPIError(w, err)
			return
		}
		if err := cleanupManagedInstanceFiles(inst); err != nil {
			warning = err.Error()
		}
		instances = append(instances[:idx], instances[idx+1:]...)
		if err := saveManagedInstances(path, instances); err != nil {
			writeStatePersistenceError(w, requestID, action, inst.AgentID, inst.ID, err)
			return
		}

		payload := map[string]interface{}{
			"requestId":  requestID,
			"result":     "ok",
			"action":     action,
			"instanceId": inst.ID,
		}
		if warning != "" {
			payload["warning"] = warning
		}
		writeJSON(w, http.StatusOK, payload)
		return
	}
}

func isDaemonAgentNotFound(err error) bool {
	de, ok := err.(*DaemonClientError)
	return ok && strings.EqualFold(strings.TrimSpace(de.Code), "E_AGENT_NOT_FOUND")
}

func mergeManagedRuntimeState(r *http.Request, daemon *DaemonClient, instances []managedAgentInstance, requestID string) bool {
	if daemon == nil || len(instances) == 0 {
		return false
	}
	agentStates := map[string]string{}
	agentIDs := make([]string, 0, len(instances))
	seen := map[string]struct{}{}
	for _, inst := range instances {
		agentID := strings.TrimSpace(inst.AgentID)
		if agentID == "" {
			continue
		}
		key := strings.ToLower(agentID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		agentIDs = append(agentIDs, agentID)
	}
	if len(agentIDs) == 0 {
		return false
	}
	actor := "webui:instances:list"
	for _, agentID := range agentIDs {
		statuses, err := daemon.GetStatus(r.Context(), agentID, actor, requestID)
		if err != nil || len(statuses) == 0 {
			continue
		}
		state := strings.TrimSpace(statuses[0].Runtime)
		if state == "" {
			continue
		}
		agentStates[strings.ToLower(agentID)] = state
	}
	if len(agentStates) == 0 {
		return false
	}

	changed := false
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range instances {
		key := strings.ToLower(strings.TrimSpace(instances[i].AgentID))
		state, ok := agentStates[key]
		if !ok {
			continue
		}
		if strings.TrimSpace(instances[i].RuntimeState) != state {
			instances[i].RuntimeState = state
			instances[i].UpdatedAt = now
			changed = true
		}
	}
	return changed
}

func parseManagedTimestamp(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return ts
}
