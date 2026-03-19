package gateway

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
)

func listOrchestratorPolicies() ([]OrchestratorPolicyRule, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return nil, err
	}
	out := make([]OrchestratorPolicyRule, len(state.Policies))
	copy(out, state.Policies)
	return sortOrchestratorPolicyRules(out), nil
}

func listExecutionTriggers() ([]ExecutionTrigger, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return nil, err
	}
	out := make([]ExecutionTrigger, len(state.Triggers))
	copy(out, state.Triggers)
	return out, nil
}

func getExecutionTrigger(triggerID string) (ExecutionTrigger, bool, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return ExecutionTrigger{}, false, err
	}
	id := strings.TrimSpace(triggerID)
	for _, trigger := range state.Triggers {
		if strings.EqualFold(strings.TrimSpace(trigger.ID), id) {
			return trigger, true, nil
		}
	}
	return ExecutionTrigger{}, false, nil
}

func upsertExecutionTrigger(trigger ExecutionTrigger) (ExecutionTrigger, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return ExecutionTrigger{}, err
	}
	trigger, err = normalizeExecutionTriggerForStore(trigger)
	if err != nil {
		return ExecutionTrigger{}, err
	}
	now := nowTimestamp()
	trigger.UpdatedAt = now
	if trigger.CreatedAt == "" {
		trigger.CreatedAt = now
	}
	updated := false
	for i := range state.Triggers {
		if strings.EqualFold(strings.TrimSpace(state.Triggers[i].ID), trigger.ID) {
			trigger.CreatedAt = state.Triggers[i].CreatedAt
			state.Triggers[i] = trigger
			updated = true
			break
		}
	}
	if !updated {
		state.Triggers = append(state.Triggers, trigger)
	}
	if err := saveRemoteControlState(path, state); err != nil {
		return ExecutionTrigger{}, err
	}
	return trigger, nil
}

func deleteExecutionTrigger(triggerID string) error {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return err
	}
	id := strings.TrimSpace(triggerID)
	filtered := state.Triggers[:0]
	found := false
	for _, trigger := range state.Triggers {
		if strings.EqualFold(strings.TrimSpace(trigger.ID), id) {
			found = true
			continue
		}
		filtered = append(filtered, trigger)
	}
	if !found {
		return os.ErrNotExist
	}
	state.Triggers = append([]ExecutionTrigger(nil), filtered...)
	return saveRemoteControlState(path, state)
}

func upsertOrchestratorPolicy(rule OrchestratorPolicyRule) (OrchestratorPolicyRule, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return OrchestratorPolicyRule{}, err
	}
	normalized := normalizeOrchestratorPolicyRule(rule)
	if err := validateOrchestratorPolicyRule(normalized); err != nil {
		return OrchestratorPolicyRule{}, err
	}
	now := nowTimestamp()
	if normalized.ID == "" {
		normalized.ID = uuid.NewString()
	}
	if normalized.CreatedAt == "" {
		normalized.CreatedAt = now
	}
	normalized.UpdatedAt = now

	updated := false
	for i := range state.Policies {
		if strings.EqualFold(strings.TrimSpace(state.Policies[i].ID), normalized.ID) {
			normalized.CreatedAt = state.Policies[i].CreatedAt
			state.Policies[i] = normalized
			updated = true
			break
		}
	}
	if !updated {
		state.Policies = append(state.Policies, normalized)
	}
	state.Policies = sortOrchestratorPolicyRules(state.Policies)
	if err := saveRemoteControlState(path, state); err != nil {
		return OrchestratorPolicyRule{}, err
	}
	return normalized, nil
}

func deleteOrchestratorPolicy(policyID string) (bool, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return false, err
	}
	id := strings.TrimSpace(policyID)
	before := len(state.Policies)
	filtered := state.Policies[:0]
	for _, policy := range state.Policies {
		if strings.EqualFold(strings.TrimSpace(policy.ID), id) {
			continue
		}
		filtered = append(filtered, policy)
	}
	state.Policies = filtered
	if len(state.Policies) == before {
		return false, nil
	}
	if err := saveRemoteControlState(path, state); err != nil {
		return false, err
	}
	return true, nil
}

func remoteInstanceSyncKey(hostID, agentID string) string {
	return strings.ToLower(strings.TrimSpace(hostID) + ":" + strings.TrimSpace(agentID))
}

func getRemoteInstanceSyncStatus(hostID, agentID string) (RemoteInstanceSyncStatus, bool, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return RemoteInstanceSyncStatus{}, false, err
	}
	key := remoteInstanceSyncKey(hostID, agentID)
	status, ok := state.InstanceSyncs[key]
	if !ok {
		return RemoteInstanceSyncStatus{}, false, nil
	}
	return status, true, nil
}

func listRemoteInstanceSyncStatuses() ([]RemoteInstanceSyncStatus, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return nil, err
	}
	out := make([]RemoteInstanceSyncStatus, 0, len(state.InstanceSyncs))
	for _, status := range state.InstanceSyncs {
		out = append(out, status)
	}
	return out, nil
}

func upsertRemoteInstanceSyncStatus(status RemoteInstanceSyncStatus) (RemoteInstanceSyncStatus, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return RemoteInstanceSyncStatus{}, err
	}
	hostID := strings.TrimSpace(status.HostID)
	agentID := strings.TrimSpace(status.AgentID)
	if hostID == "" || agentID == "" {
		return RemoteInstanceSyncStatus{}, fmt.Errorf("hostId and agentId are required")
	}
	status.HostID = hostID
	status.AgentID = agentID
	status.SyncMode = normalizeProviderBindingSyncMode(status.SyncMode)
	if err := validateProviderBindingSyncMode(status.SyncMode); err != nil {
		return RemoteInstanceSyncStatus{}, err
	}
	if strings.TrimSpace(status.DriftState) == "" {
		status.DriftState = "unknown"
	}
	if strings.TrimSpace(status.LastSyncStatus) == "" {
		status.LastSyncStatus = "unknown"
	}
	if strings.TrimSpace(status.MemoryLastSyncStatus) == "" {
		status.MemoryLastSyncStatus = "unknown"
	}
	status.MemoryGit = normalizeRemoteMemoryGitConfig(status.MemoryGit)
	status.UpdatedAt = nowTimestamp()
	key := remoteInstanceSyncKey(hostID, agentID)
	state.InstanceSyncs[key] = status
	if err := saveRemoteControlState(path, state); err != nil {
		return RemoteInstanceSyncStatus{}, err
	}
	return status, nil
}

func deleteRemoteInstanceSyncStatus(hostID, agentID string) error {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return err
	}
	delete(state.InstanceSyncs, remoteInstanceSyncKey(hostID, agentID))
	return saveRemoteControlState(path, state)
}

func listOrchestratorExecutions() ([]OrchestratorExecution, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return nil, err
	}
	out := make([]OrchestratorExecution, len(state.Executions))
	copy(out, state.Executions)
	return out, nil
}

func getOrchestratorExecution(executionID string) (OrchestratorExecution, bool, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return OrchestratorExecution{}, false, err
	}
	id := strings.TrimSpace(executionID)
	for _, execution := range state.Executions {
		if strings.EqualFold(strings.TrimSpace(execution.ID), id) {
			return execution, true, nil
		}
	}
	return OrchestratorExecution{}, false, nil
}

func findOrchestratorExecutionByIdempotencyKey(idempotencyKey string) (OrchestratorExecution, bool, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return OrchestratorExecution{}, false, err
	}
	key := strings.TrimSpace(idempotencyKey)
	if key == "" {
		return OrchestratorExecution{}, false, nil
	}
	for _, execution := range state.Executions {
		if strings.EqualFold(strings.TrimSpace(execution.IdempotencyKey), key) {
			return execution, true, nil
		}
	}
	return OrchestratorExecution{}, false, nil
}

func upsertOrchestratorExecution(execution OrchestratorExecution) (OrchestratorExecution, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return OrchestratorExecution{}, err
	}
	execution = normalizeOrchestratorExecutionForStore(execution)
	id := execution.ID
	if id == "" {
		return OrchestratorExecution{}, fmt.Errorf("execution id is required")
	}
	execution.UpdatedAt = nowTimestamp()
	if execution.CreatedAt == "" {
		execution.CreatedAt = execution.UpdatedAt
	}

	updated := false
	for i := range state.Executions {
		if strings.EqualFold(strings.TrimSpace(state.Executions[i].ID), id) {
			execution.CreatedAt = state.Executions[i].CreatedAt
			state.Executions[i] = execution
			updated = true
			break
		}
	}
	if !updated {
		state.Executions = append(state.Executions, execution)
	}
	if err := saveRemoteControlState(path, state); err != nil {
		return OrchestratorExecution{}, err
	}
	return execution, nil
}

func listOrchestratorWorkerLeases() ([]OrchestratorWorkerLease, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return nil, err
	}
	out := make([]OrchestratorWorkerLease, len(state.WorkerLeases))
	copy(out, state.WorkerLeases)
	return out, nil
}

func listOrchestratorWorkerLeasesByExecution(executionID string) ([]OrchestratorWorkerLease, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, _, err := loadRemoteControlState()
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(executionID)
	out := make([]OrchestratorWorkerLease, 0)
	for _, lease := range state.WorkerLeases {
		if strings.EqualFold(strings.TrimSpace(lease.ExecutionID), id) {
			out = append(out, lease)
		}
	}
	return out, nil
}

func upsertOrchestratorWorkerLease(lease OrchestratorWorkerLease) (OrchestratorWorkerLease, error) {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return OrchestratorWorkerLease{}, err
	}
	lease = normalizeOrchestratorWorkerLeaseForStore(lease)
	id := lease.ID
	if id == "" {
		return OrchestratorWorkerLease{}, fmt.Errorf("worker lease id is required")
	}
	if lease.ExecutionID == "" {
		return OrchestratorWorkerLease{}, fmt.Errorf("executionId is required")
	}
	if lease.HostID == "" {
		return OrchestratorWorkerLease{}, fmt.Errorf("hostId is required")
	}
	if lease.AgentID == "" {
		return OrchestratorWorkerLease{}, fmt.Errorf("agentId is required")
	}
	lease.UpdatedAt = nowTimestamp()
	if lease.CreatedAt == "" {
		lease.CreatedAt = lease.UpdatedAt
	}
	if lease.HeartbeatAt == "" {
		lease.HeartbeatAt = lease.UpdatedAt
	}
	if lease.LastHeartbeatAt == "" {
		lease.LastHeartbeatAt = lease.HeartbeatAt
	}
	if lease.LeaseExpireAt == "" {
		lease.LeaseExpireAt = lease.UpdatedAt
	}

	updated := false
	for i := range state.WorkerLeases {
		if strings.EqualFold(strings.TrimSpace(state.WorkerLeases[i].ID), id) {
			lease.CreatedAt = state.WorkerLeases[i].CreatedAt
			state.WorkerLeases[i] = lease
			updated = true
			break
		}
	}
	if !updated {
		state.WorkerLeases = append(state.WorkerLeases, lease)
	}
	if err := saveRemoteControlState(path, state); err != nil {
		return OrchestratorWorkerLease{}, err
	}
	return lease, nil
}

func deleteOrchestratorWorkerLease(leaseID string) error {
	remoteControlStoreMu.Lock()
	defer remoteControlStoreMu.Unlock()

	state, path, err := loadRemoteControlState()
	if err != nil {
		return err
	}
	id := strings.TrimSpace(leaseID)
	filtered := state.WorkerLeases[:0]
	for _, lease := range state.WorkerLeases {
		if strings.EqualFold(strings.TrimSpace(lease.ID), id) {
			continue
		}
		filtered = append(filtered, lease)
	}
	state.WorkerLeases = filtered
	return saveRemoteControlState(path, state)
}
