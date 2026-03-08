package gateway

import (
	"context"
	"strings"
	"time"
)

const (
	defaultOrchestratorWorkerStaleAfter   = 10 * time.Minute
	defaultOrchestratorWorkerHeartbeatTTL = 2 * time.Minute
)

func effectiveOrchestratorWorkerStaleAfter(cfg *GatewayConfig) time.Duration {
	if cfg != nil && cfg.WorkerLeaseStaleAfter > 0 {
		return cfg.WorkerLeaseStaleAfter
	}
	return defaultOrchestratorWorkerStaleAfter
}

func effectiveOrchestratorWorkerHeartbeatTimeout(cfg *GatewayConfig) time.Duration {
	if cfg != nil && cfg.WorkerHeartbeatTimeout > 0 {
		return cfg.WorkerHeartbeatTimeout
	}
	return defaultOrchestratorWorkerHeartbeatTTL
}

func buildOrchestratorExecutionIndex(executions []OrchestratorExecution) map[string]OrchestratorExecution {
	index := make(map[string]OrchestratorExecution, len(executions))
	for _, execution := range executions {
		id := strings.TrimSpace(execution.ID)
		if id == "" {
			continue
		}
		index[id] = execution
	}
	return index
}

func isWorkerLeaseStale(lease OrchestratorWorkerLease, executionsByID map[string]OrchestratorExecution, now time.Time, cfg *GatewayConfig) (bool, string) {
	state := lease.State
	if state == OrchestratorWorkerStateReclaimed {
		return false, ""
	}
	executionID := strings.TrimSpace(lease.ExecutionID)
	if execution, ok := executionsByID[executionID]; ok && isOrchestratorExecutionTerminal(execution.Status) {
		return true, "execution_terminal"
	}

	leaseExpireAt := strings.TrimSpace(lease.LeaseExpireAt)
	if leaseExpireAt != "" {
		if expireTime, err := time.Parse(time.RFC3339Nano, leaseExpireAt); err == nil && !expireTime.After(now) {
			return true, "lease_expired"
		} else if err != nil {
			if expireTime, errRFC3339 := time.Parse(time.RFC3339, leaseExpireAt); errRFC3339 == nil && !expireTime.After(now) {
				return true, "lease_expired"
			}
		}
	}

	heartbeatRaw := strings.TrimSpace(lease.LastHeartbeatAt)
	if heartbeatRaw == "" {
		heartbeatRaw = strings.TrimSpace(lease.HeartbeatAt)
	}
	if heartbeatRaw != "" && (state == OrchestratorWorkerStateBusy || state == OrchestratorWorkerStateProvisioning || state == OrchestratorWorkerStateReady || state == OrchestratorWorkerStateReclaiming) {
		heartbeatAt := parseRFC3339OrNow(heartbeatRaw)
		if now.Sub(heartbeatAt) >= effectiveOrchestratorWorkerHeartbeatTimeout(cfg) {
			return true, "heartbeat_timeout"
		}
	}

	updatedAt := strings.TrimSpace(lease.UpdatedAt)
	if updatedAt != "" && state != OrchestratorWorkerStateReady {
		if now.Sub(parseRFC3339OrNow(updatedAt)) >= effectiveOrchestratorWorkerStaleAfter(cfg) {
			return true, "lease_stale"
		}
	}
	return false, ""
}

func markStaleWorkerLeases(leases []OrchestratorWorkerLease, executionsByID map[string]OrchestratorExecution, now time.Time, cfg *GatewayConfig) []OrchestratorWorkerLease {
	marked := make([]OrchestratorWorkerLease, len(leases))
	for i, lease := range leases {
		out := lease
		out.LeaseState = string(out.State)
		if strings.TrimSpace(out.LastHeartbeatAt) == "" {
			out.LastHeartbeatAt = strings.TrimSpace(out.HeartbeatAt)
		}
		stale, reason := isWorkerLeaseStale(out, executionsByID, now, cfg)
		out.Stale = stale
		out.StaleReason = reason
		marked[i] = out
	}
	return marked
}

func orchestratorLeaseAgeSec(lease OrchestratorWorkerLease, now time.Time) int64 {
	raw := strings.TrimSpace(lease.CreatedAt)
	if raw == "" {
		return 0
	}
	age := now.Sub(parseRFC3339OrNow(raw))
	if age < 0 {
		return 0
	}
	return int64(age / time.Second)
}

func orchestratorHeartbeatAgeSec(lease OrchestratorWorkerLease, now time.Time) int64 {
	raw := strings.TrimSpace(lease.LastHeartbeatAt)
	if raw == "" {
		raw = strings.TrimSpace(lease.HeartbeatAt)
	}
	if raw == "" {
		return 0
	}
	age := now.Sub(parseRFC3339OrNow(raw))
	if age < 0 {
		return 0
	}
	return int64(age / time.Second)
}

func buildOrchestratorWorkerQueueSummary(executions []OrchestratorExecution, leases []OrchestratorWorkerLease, cfg *GatewayConfig, now time.Time) OrchestratorWorkerQueueSummary {
	executionsByID := buildOrchestratorExecutionIndex(executions)
	markedLeases := markStaleWorkerLeases(leases, executionsByID, now, cfg)

	activeExecutions := 0
	queuedTasks := 0
	for _, execution := range executions {
		status := execution.Status
		if status != OrchestratorExecutionStatusProvisioning && status != OrchestratorExecutionStatusRunning {
			continue
		}
		activeExecutions++
		completedTasks := map[string]struct{}{}
		for _, result := range execution.Results {
			taskID := strings.TrimSpace(result.TaskID)
			if taskID == "" {
				continue
			}
			if result.Status == OrchestratorTaskStatusCompleted {
				completedTasks[taskID] = struct{}{}
			}
		}
		remaining := 0
		for _, task := range execution.TaskUnits {
			if _, done := completedTasks[strings.TrimSpace(task.ID)]; !done {
				remaining++
			}
		}
		busyWorkers := 0
		for _, lease := range markedLeases {
			if strings.EqualFold(strings.TrimSpace(lease.ExecutionID), strings.TrimSpace(execution.ID)) &&
				(lease.State == OrchestratorWorkerStateBusy || lease.State == OrchestratorWorkerStateProvisioning) &&
				!lease.Stale {
				busyWorkers++
			}
		}
		if remaining > busyWorkers {
			queuedTasks += remaining - busyWorkers
		}
	}

	staleLeases := 0
	for _, lease := range markedLeases {
		if lease.Stale {
			staleLeases++
		}
	}

	return OrchestratorWorkerQueueSummary{
		ActiveExecutions:   activeExecutions,
		QueuedTasks:        queuedTasks,
		StaleLeases:        staleLeases,
		ReclaimableWorkers: staleLeases,
		UpdatedAt:          now.UTC().Format(time.RFC3339Nano),
	}
}

func reclaimOrchestratorStaleWorkers(ctx context.Context, cfg *GatewayConfig) (map[string]interface{}, error) {
	leases, err := listOrchestratorWorkerLeases()
	if err != nil {
		return nil, err
	}
	executions, err := listOrchestratorExecutions()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	marked := markStaleWorkerLeases(leases, buildOrchestratorExecutionIndex(executions), now, cfg)
	staleOnly := make([]OrchestratorWorkerLease, 0)
	for _, lease := range marked {
		if lease.Stale {
			staleOnly = append(staleOnly, lease)
		}
	}
	return reclaimOrchestratorLeaseSet(ctx, staleOnly, true, 0)
}
