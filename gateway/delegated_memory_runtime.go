package gateway

import (
	"context"
	"fmt"
	"strings"
)

const (
	delegatedDistillTargetPerAgent      = "per_agent"
	delegatedCleanupPolicyDeleteDistill = "delete_after_distill"
	delegatedPerAgentMemoryVersion      = "v1"
)

func provisionDelegatedChild(
	ctx context.Context,
	daemon *DaemonClient,
	execution *OrchestratorExecution,
	task OrchestratorTaskUnit,
	lease OrchestratorWorkerLease,
	attempt int,
) (managedAgentInstance, error) {
	if daemon == nil {
		return managedAgentInstance{}, fmt.Errorf("daemon client is not available")
	}
	if execution == nil {
		return managedAgentInstance{}, fmt.Errorf("execution is required")
	}

	childID, err := generateManagedInstanceID(strings.TrimSpace(lease.AgentID) + "-delegated")
	if err != nil {
		return managedAgentInstance{}, fmt.Errorf("allocate delegated child instance id: %w", err)
	}
	perAgentMemoryID := buildDelegatedPerAgentMemoryID(childID)
	parentSubjectID := resolveDelegatedParentSubjectID(strings.TrimSpace(lease.AgentID))
	requestID := "orchestrator-" + strings.TrimSpace(execution.ID)
	actor := "gateway:orchestrator:delegated"

	if _, err := daemon.CreateMemoryEntry(
		ctx,
		perAgentMemoryID,
		"Delegated Child Memory",
		delegatedPerAgentMemoryVersion,
		"per_agent",
		childID,
		actor,
		requestID,
	); err != nil {
		return managedAgentInstance{}, fmt.Errorf("create delegated child per-agent memory: %w", err)
	}

	var snapshotID string
	var snapshotDigest string
	sourceScopes := normalizeStringSelectorList(execution.SourceScopes, true)
	if len(sourceScopes) > 0 {
		snapshot, err := daemon.CreateInstanceSnapshot(
			ctx,
			parentSubjectID,
			sourceScopes,
			childID,
			fmt.Sprintf("delegated task %s for execution %s", strings.TrimSpace(task.ID), strings.TrimSpace(execution.ID)),
			actor,
			requestID,
		)
		if err != nil {
			return managedAgentInstance{}, fmt.Errorf("create delegated child snapshot: %w", err)
		}
		snapshotID = strings.TrimSpace(snapshot.ID)
		snapshotDigest = strings.TrimSpace(snapshot.Digest)
		if snapshotID != "" {
			if err := daemon.MountInstanceSnapshot(ctx, childID, snapshotID, actor, requestID); err != nil {
				return managedAgentInstance{}, fmt.Errorf("mount delegated child snapshot: %w", err)
			}
		}
	}

	now := nowTimestamp()
	child := managedAgentInstance{
		ID:                 childID,
		Type:               strings.TrimSpace(lease.AgentID),
		AgentID:            strings.TrimSpace(lease.AgentID),
		RuntimeState:       "delegated",
		AgentLifecycleMode: orchestratorAgentLifecycleMode,
		MemoryBindingMode:  orchestratorMemoryBindingMode,
		PerAgentMemoryID:   perAgentMemoryID,
		ParentAgentID:      parentSubjectID,
		ParentExecutionID:  strings.TrimSpace(execution.ID),
		TaskID:             strings.TrimSpace(task.ID),
		SnapshotID:         snapshotID,
		SnapshotDigest:     snapshotDigest,
		DistillTarget:      delegatedDistillTargetPerAgent,
		CleanupPolicy:      delegatedCleanupPolicyDeleteDistill,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := upsertManagedInstance(child); err != nil {
		return managedAgentInstance{}, fmt.Errorf("persist delegated child instance: %w", err)
	}

	execution.ChildAgentID = child.ID
	execution.ChildPerAgentMemoryID = child.PerAgentMemoryID
	execution.SnapshotID = child.SnapshotID
	execution.SnapshotDigest = child.SnapshotDigest
	if _, err := upsertOrchestratorExecution(*execution); err != nil {
		return managedAgentInstance{}, fmt.Errorf("persist delegated execution state: %w", err)
	}
	return child, nil
}

func resolveDelegatedParentSubjectID(agentID string) string {
	instances, _, err := loadManagedInstances()
	if err == nil {
		target := strings.ToLower(strings.TrimSpace(agentID))
		bestIdx := -1
		for i, inst := range instances {
			if !strings.EqualFold(strings.TrimSpace(inst.AgentID), target) && !strings.EqualFold(strings.TrimSpace(inst.Type), target) {
				continue
			}
			if normalizeManagedAgentLifecycleMode(inst.AgentLifecycleMode) != managedAgentLifecyclePersistent {
				continue
			}
			if bestIdx < 0 {
				bestIdx = i
				continue
			}
			currentUpdated, currentHasTime := parseManagedInstanceTimestamp(inst.UpdatedAt)
			bestUpdated, bestHasTime := parseManagedInstanceTimestamp(instances[bestIdx].UpdatedAt)
			if currentHasTime && (!bestHasTime || currentUpdated.After(bestUpdated) || currentUpdated.Equal(bestUpdated)) {
				bestIdx = i
			}
		}
		if bestIdx >= 0 {
			if trimmed := strings.TrimSpace(instances[bestIdx].ID); trimmed != "" {
				return trimmed
			}
		}
	}
	return strings.TrimSpace(agentID)
}

func buildDelegatedPerAgentMemoryID(childID string) string {
	trimmed := strings.TrimSpace(childID)
	if trimmed == "" {
		return "delegated-child-memory"
	}
	return "per-agent-" + trimmed
}
