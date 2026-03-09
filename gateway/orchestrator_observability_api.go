package gateway

import (
	"net/http"
	"strings"
	"time"
)

type orchestratorExecutionMetricsSnapshot struct {
	Total                int   `json:"total"`
	PendingAuthorization int   `json:"pendingAuthorization"`
	Provisioning         int   `json:"provisioning"`
	Running              int   `json:"running"`
	Completed            int   `json:"completed"`
	PartialCompleted     int   `json:"partialCompleted"`
	Failed               int   `json:"failed"`
	RetryableFailed      int   `json:"retryableFailed"`
	Cancelled            int   `json:"cancelled"`
	Declined             int   `json:"declined"`
	RetryCount           int   `json:"retryCount"`
	AvgLatencyMs         int64 `json:"avgLatencyMs"`
}

type orchestratorWorkerMetricsSnapshot struct {
	Total        int `json:"total"`
	Provisioning int `json:"provisioning"`
	Ready        int `json:"ready"`
	Busy         int `json:"busy"`
	Reclaiming   int `json:"reclaiming"`
	Reclaimed    int `json:"reclaimed"`
	Error        int `json:"error"`
	Stale        int `json:"stale"`
}

type orchestratorProviderMetricsSnapshot struct {
	RequestedFailures map[string]int `json:"requestedFailures"`
	ResolvedFailures  map[string]int `json:"resolvedFailures"`
}

type orchestratorPolicyMetricsSnapshot struct {
	Allow int `json:"allow"`
	Ask   int `json:"ask"`
	Deny  int `json:"deny"`
}

type orchestratorMetricsSnapshot struct {
	Timestamp  string                               `json:"timestamp"`
	Executions orchestratorExecutionMetricsSnapshot `json:"executions"`
	Workers    orchestratorWorkerMetricsSnapshot    `json:"workers"`
	Providers  orchestratorProviderMetricsSnapshot  `json:"providers"`
	Policies   orchestratorPolicyMetricsSnapshot    `json:"policies"`
	Queue      OrchestratorWorkerQueueSummary       `json:"queue"`
}

func handleOrchestratorMetrics(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	if _, ok := requireGatewayPermission(w, r, cfg, canViewExecutions, "E_RBAC_EXECUTION_VIEW", "role cannot view orchestrator metrics"); !ok {
		return
	}

	executions, err := listOrchestratorExecutions()
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to list orchestrator executions", "list orchestrator executions for metrics", err)
		return
	}
	leases, err := listOrchestratorWorkerLeases()
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to list orchestrator worker leases", "list orchestrator worker leases for metrics", err)
		return
	}

	now := time.Now().UTC()
	metrics := buildOrchestratorMetricsSnapshot(executions, leases, cfg, now)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"metrics":   metrics,
	})
}

func buildOrchestratorMetricsSnapshot(executions []OrchestratorExecution, leases []OrchestratorWorkerLease, cfg *GatewayConfig, now time.Time) orchestratorMetricsSnapshot {
	executionsByID := buildOrchestratorExecutionIndex(executions)
	markedLeases := markStaleWorkerLeases(leases, executionsByID, now, cfg)

	executionMetrics := orchestratorExecutionMetricsSnapshot{}
	workerMetrics := orchestratorWorkerMetricsSnapshot{}
	providerMetrics := orchestratorProviderMetricsSnapshot{
		RequestedFailures: map[string]int{},
		ResolvedFailures:  map[string]int{},
	}
	policyMetrics := orchestratorPolicyMetricsSnapshot{}

	var latencyTotal int64
	var latencyCount int64
	for _, execution := range executions {
		executionMetrics.Total++
		switch execution.Status {
		case OrchestratorExecutionStatusPendingAuthorization:
			executionMetrics.PendingAuthorization++
		case OrchestratorExecutionStatusProvisioning:
			executionMetrics.Provisioning++
		case OrchestratorExecutionStatusRunning:
			executionMetrics.Running++
		case OrchestratorExecutionStatusCompleted:
			executionMetrics.Completed++
		case OrchestratorExecutionStatusPartialCompleted:
			executionMetrics.PartialCompleted++
		case OrchestratorExecutionStatusFailed:
			executionMetrics.Failed++
		case OrchestratorExecutionStatusRetryableFailed:
			executionMetrics.RetryableFailed++
		case OrchestratorExecutionStatusCancelled:
			executionMetrics.Cancelled++
		case OrchestratorExecutionStatusDeclined:
			executionMetrics.Declined++
		}

		for _, result := range execution.Results {
			if result.Attempts > 1 {
				executionMetrics.RetryCount += result.Attempts - 1
			}
		}

		if latencyMs, ok := orchestratorExecutionLatencyMs(execution); ok {
			latencyTotal += latencyMs
			latencyCount++
		}

		switch strings.ToLower(strings.TrimSpace(execution.Policy.Decision)) {
		case "allow":
			policyMetrics.Allow++
		case "ask":
			policyMetrics.Ask++
		case "deny":
			policyMetrics.Deny++
		}

		if !executionHasProviderFailure(execution) {
			continue
		}
		requestedProvider := strings.ToLower(strings.TrimSpace(execution.RequestedProvider))
		if requestedProvider != "" {
			providerMetrics.RequestedFailures[requestedProvider]++
		}
		resolved := map[string]struct{}{}
		for _, resolution := range execution.Governance.ProviderResolutions {
			provider := strings.ToLower(strings.TrimSpace(resolution.Provider))
			if provider == "" {
				continue
			}
			if _, seen := resolved[provider]; seen {
				continue
			}
			resolved[provider] = struct{}{}
			providerMetrics.ResolvedFailures[provider]++
		}
	}
	if latencyCount > 0 {
		executionMetrics.AvgLatencyMs = latencyTotal / latencyCount
	}

	for _, lease := range markedLeases {
		workerMetrics.Total++
		switch lease.State {
		case OrchestratorWorkerStateProvisioning:
			workerMetrics.Provisioning++
		case OrchestratorWorkerStateReady:
			workerMetrics.Ready++
		case OrchestratorWorkerStateBusy:
			workerMetrics.Busy++
		case OrchestratorWorkerStateReclaiming:
			workerMetrics.Reclaiming++
		case OrchestratorWorkerStateReclaimed:
			workerMetrics.Reclaimed++
		case OrchestratorWorkerStateError:
			workerMetrics.Error++
		}
		if lease.Stale && isActiveOrchestratorWorkerState(lease.State) {
			workerMetrics.Stale++
		}
	}

	return orchestratorMetricsSnapshot{
		Timestamp:  now.UTC().Format(time.RFC3339Nano),
		Executions: executionMetrics,
		Workers:    workerMetrics,
		Providers:  providerMetrics,
		Policies:   policyMetrics,
		Queue:      buildOrchestratorWorkerQueueSummary(executions, markedLeases, cfg, now),
	}
}

func isActiveOrchestratorWorkerState(state OrchestratorWorkerState) bool {
	switch state {
	case OrchestratorWorkerStateProvisioning,
		OrchestratorWorkerStateBusy,
		OrchestratorWorkerStateReclaiming:
		return true
	default:
		return false
	}
}

func orchestratorExecutionLatencyMs(execution OrchestratorExecution) (int64, bool) {
	startedAt := strings.TrimSpace(execution.StartedAt)
	completedAt := strings.TrimSpace(execution.CompletedAt)
	if startedAt == "" || completedAt == "" {
		return 0, false
	}
	start, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		start, err = time.Parse(time.RFC3339Nano, startedAt)
		if err != nil {
			return 0, false
		}
	}
	end, err := time.Parse(time.RFC3339, completedAt)
	if err != nil {
		end, err = time.Parse(time.RFC3339Nano, completedAt)
		if err != nil {
			return 0, false
		}
	}
	if end.Before(start) {
		return 0, false
	}
	return end.Sub(start).Milliseconds(), true
}

func executionHasProviderFailure(execution OrchestratorExecution) bool {
	if strings.Contains(strings.ToLower(strings.TrimSpace(execution.Outcome.FailureCategory)), "provider") {
		return true
	}
	for _, result := range execution.Results {
		if strings.Contains(strings.ToLower(strings.TrimSpace(result.FailureCategory)), "provider") {
			return true
		}
	}
	return false
}
