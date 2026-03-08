package gateway

import (
	"net/http"
	"sort"
	"strings"
	"time"
)

func handleOrchestratorWorkers(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig, daemon *DaemonClient) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}

	workers, summary, warnings, err := listOrchestratorWorkerInventory(r, daemon, requestID, cfg)
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to list orchestrator workers", "list orchestrator worker inventory", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"workers":   workers,
		"summary":   summary,
		"warnings":  warnings,
	})
}

func handleOrchestratorWorkersQueue(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}

	executions, err := listOrchestratorExecutions()
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to list orchestrator executions", "list orchestrator executions for queue summary", err)
		return
	}
	leases, err := listOrchestratorWorkerLeases()
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to list orchestrator worker leases", "list orchestrator worker leases for queue summary", err)
		return
	}
	summary := buildOrchestratorWorkerQueueSummary(executions, leases, cfg, time.Now().UTC())
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"summary":   summary,
	})
}

func handleOrchestratorWorkersReclaimStale(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	summary, err := reclaimOrchestratorStaleWorkers(r.Context(), cfg)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_RECLAIM_FAILED", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"reclaim":   summary,
	})
}

func listOrchestratorWorkerInventory(r *http.Request, daemon *DaemonClient, requestID string, cfg *GatewayConfig) ([]OrchestratorWorkerInventoryItem, OrchestratorWorkerInventorySummary, []string, error) {
	hosts, err := listRemoteHosts()
	if err != nil {
		return nil, OrchestratorWorkerInventorySummary{}, nil, err
	}
	syncStatuses, err := listRemoteInstanceSyncStatuses()
	if err != nil {
		return nil, OrchestratorWorkerInventorySummary{}, nil, err
	}
	leases, err := listOrchestratorWorkerLeases()
	if err != nil {
		return nil, OrchestratorWorkerInventorySummary{}, nil, err
	}
	executions, err := listOrchestratorExecutions()
	if err != nil {
		return nil, OrchestratorWorkerInventorySummary{}, nil, err
	}
	executionsByID := buildOrchestratorExecutionIndex(executions)
	now := time.Now().UTC()
	leases = markStaleWorkerLeases(leases, executionsByID, now, cfg)

	hostByID := make(map[string]RemoteHost, len(hosts))
	for _, host := range hosts {
		hostByID[strings.TrimSpace(host.ID)] = host
	}

	workers := make([]OrchestratorWorkerInventoryItem, 0, len(syncStatuses)+len(leases)+4)
	for _, lease := range leases {
		workers = append(workers, workerInventoryItemFromLease(lease, hostByID[strings.TrimSpace(lease.HostID)], now))
	}
	for _, syncStatus := range syncStatuses {
		workers = append(workers, workerInventoryItemFromSync(syncStatus, hostByID[strings.TrimSpace(syncStatus.HostID)]))
	}

	warnings := make([]string, 0)
	if daemon != nil {
		agents, daemonErr := daemon.ListAgents(r.Context(), "orchestrator:workers:list", requestID)
		if daemonErr != nil {
			warnings = append(warnings, "local daemon inventory unavailable: "+daemonErr.Error())
		} else {
			for _, agent := range agents {
				if !shouldIncludeLocalWorkerAgent(agent) {
					continue
				}
				workers = append(workers, workerInventoryItemFromLocalAgent(agent))
			}
		}
	}

	sort.SliceStable(workers, func(i, j int) bool {
		left := workers[i]
		right := workers[j]
		lp := workerInventorySortPriority(left.State, left.Source)
		rp := workerInventorySortPriority(right.State, right.Source)
		if lp != rp {
			return lp < rp
		}
		lt := parseRFC3339OrNow(left.UpdatedAt)
		rt := parseRFC3339OrNow(right.UpdatedAt)
		if !lt.Equal(rt) {
			return lt.After(rt)
		}
		if !strings.EqualFold(left.HostID, right.HostID) {
			return strings.ToLower(left.HostID) < strings.ToLower(right.HostID)
		}
		if !strings.EqualFold(left.AgentID, right.AgentID) {
			return strings.ToLower(left.AgentID) < strings.ToLower(right.AgentID)
		}
		return strings.ToLower(left.ID) < strings.ToLower(right.ID)
	})

	return workers, summarizeWorkerInventory(workers), warnings, nil
}

func shouldIncludeLocalWorkerAgent(agent AgentState) bool {
	install := strings.ToLower(strings.TrimSpace(agent.InstallState))
	runtime := strings.ToLower(strings.TrimSpace(agent.Runtime))
	health := strings.ToLower(strings.TrimSpace(agent.Health))
	if install == "installed" {
		return true
	}
	switch runtime {
	case "running", "starting", "healthy", "error":
		return true
	}
	return health == "healthy" || health == "unhealthy"
}

func workerInventoryItemFromLocalAgent(agent AgentState) OrchestratorWorkerInventoryItem {
	lastError := ""
	if agent.LastError != nil {
		lastError = strings.TrimSpace(*agent.LastError)
	}
	return OrchestratorWorkerInventoryItem{
		ID:           "local:" + strings.ToLower(strings.TrimSpace(agent.ID)),
		Source:       "local",
		HostID:       orchestratorLocalHostID,
		HostName:     "Local",
		AgentID:      strings.ToLower(strings.TrimSpace(agent.ID)),
		State:        deriveLocalWorkerInventoryState(agent),
		RuntimeState: strings.TrimSpace(agent.Runtime),
		Health:       strings.TrimSpace(agent.Health),
		LastError:    lastError,
		UpdatedAt:    strings.TrimSpace(agent.UpdatedAt),
	}
}

func deriveLocalWorkerInventoryState(agent AgentState) string {
	runtime := strings.ToLower(strings.TrimSpace(agent.Runtime))
	health := strings.ToLower(strings.TrimSpace(agent.Health))
	switch runtime {
	case "running", "starting", "healthy":
		return "available"
	case "stopped":
		if strings.EqualFold(strings.TrimSpace(agent.InstallState), "installed") {
			return "stopped"
		}
	case "error":
		return "error"
	}
	if health == "healthy" {
		return "available"
	}
	if health == "unhealthy" {
		return "error"
	}
	return "unknown"
}

func workerInventoryItemFromSync(status RemoteInstanceSyncStatus, host RemoteHost) OrchestratorWorkerInventoryItem {
	hostID := strings.TrimSpace(status.HostID)
	hostName := strings.TrimSpace(host.Name)
	if hostID == orchestratorLocalHostID {
		hostName = "Local"
	}
	if hostName == "" {
		hostName = hostID
	}
	lastError := strings.TrimSpace(status.MemoryLastSyncError)
	if lastError == "" {
		lastError = strings.TrimSpace(host.LastError)
	}
	return OrchestratorWorkerInventoryItem{
		ID:             "sync:" + strings.ToLower(hostID) + ":" + strings.ToLower(strings.TrimSpace(status.AgentID)),
		Source:         "remote_sync",
		HostID:         hostID,
		HostName:       hostName,
		AgentID:        strings.ToLower(strings.TrimSpace(status.AgentID)),
		State:          deriveRemoteSyncWorkerState(status),
		RuntimeMode:    strings.TrimSpace(string(host.RuntimeMode)),
		Health:         strings.TrimSpace(string(host.LastHealth)),
		DriftState:     strings.TrimSpace(status.DriftState),
		LastSyncStatus: strings.TrimSpace(status.LastSyncStatus),
		LastError:      lastError,
		UpdatedAt:      strings.TrimSpace(status.UpdatedAt),
	}
}

func deriveRemoteSyncWorkerState(status RemoteInstanceSyncStatus) string {
	syncState := strings.ToLower(strings.TrimSpace(status.LastSyncStatus))
	driftState := strings.ToLower(strings.TrimSpace(status.DriftState))
	if strings.Contains(syncState, "fail") || strings.Contains(syncState, "error") || strings.Contains(driftState, "error") {
		return "error"
	}
	return "managed"
}

func workerInventoryItemFromLease(lease OrchestratorWorkerLease, host RemoteHost, now time.Time) OrchestratorWorkerInventoryItem {
	hostID := strings.TrimSpace(lease.HostID)
	hostName := strings.TrimSpace(host.Name)
	if hostID == orchestratorLocalHostID {
		hostName = "Local"
	}
	if hostName == "" {
		hostName = hostID
	}
	return OrchestratorWorkerInventoryItem{
		ID:              "lease:" + strings.TrimSpace(lease.ID),
		Source:          "lease",
		HostID:          hostID,
		HostName:        hostName,
		AgentID:         strings.ToLower(strings.TrimSpace(lease.AgentID)),
		State:           string(lease.State),
		LeaseState:      strings.TrimSpace(lease.LeaseState),
		ExecutionID:     strings.TrimSpace(lease.ExecutionID),
		TaskCount:       lease.TaskCount,
		QueuePosition:   lease.QueuePosition,
		Ephemeral:       lease.Ephemeral,
		InstalledByRun:  lease.InstalledByRun,
		RuntimeMode:     strings.TrimSpace(string(host.RuntimeMode)),
		Health:          strings.TrimSpace(string(host.LastHealth)),
		LastError:       strings.TrimSpace(lease.LastError),
		Stale:           lease.Stale,
		StaleReason:     strings.TrimSpace(lease.StaleReason),
		LeaseAgeSec:     orchestratorLeaseAgeSec(lease, now),
		HeartbeatAgeSec: orchestratorHeartbeatAgeSec(lease, now),
		LeaseExpireAt:   strings.TrimSpace(lease.LeaseExpireAt),
		LastHeartbeatAt: strings.TrimSpace(lease.LastHeartbeatAt),
		HeartbeatAt:     strings.TrimSpace(lease.HeartbeatAt),
		UpdatedAt:       strings.TrimSpace(lease.UpdatedAt),
	}
}

func workerInventorySortPriority(state, source string) int {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "busy":
		return 0
	case "stale":
		return 1
	case "provisioning":
		return 2
	case "reclaiming":
		return 3
	case "available":
		return 4
	case "managed":
		return 5
	case "stopped":
		return 6
	case "reclaimed":
		return 7
	case "error":
		return 8
	default:
		if strings.EqualFold(strings.TrimSpace(source), "lease") {
			return 3
		}
		return 9
	}
}

func summarizeWorkerInventory(workers []OrchestratorWorkerInventoryItem) OrchestratorWorkerInventorySummary {
	summary := OrchestratorWorkerInventorySummary{Total: len(workers)}
	for _, worker := range workers {
		state := strings.ToLower(strings.TrimSpace(worker.State))
		if strings.TrimSpace(worker.HostID) == orchestratorLocalHostID {
			summary.Local++
		} else {
			summary.Remote++
		}
		if state == "busy" {
			summary.Busy++
		}
		if state == "busy" || state == "provisioning" || state == "reclaiming" || state == "ready" {
			summary.Active++
		}
		if state == "error" {
			summary.Error++
		}
		if worker.Stale {
			summary.Stale++
		}
	}
	return summary
}
