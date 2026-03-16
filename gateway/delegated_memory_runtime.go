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
	delegatedCleanupStatusCompleted     = "completed"
	delegatedCleanupStatusPending       = "cleanup_pending"
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
	parentSubjectID := strings.TrimSpace(lease.AgentID)
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

func buildDelegatedPerAgentMemoryID(childID string) string {
	trimmed := strings.TrimSpace(childID)
	if trimmed == "" {
		return "delegated-child-memory"
	}
	return "per-agent-" + trimmed
}

func finalizeDelegatedChild(
	ctx context.Context,
	daemon *DaemonClient,
	execution *OrchestratorExecution,
	task OrchestratorTaskUnit,
	child managedAgentInstance,
	result OrchestratorTaskResult,
) (OrchestratorTaskResult, error) {
	if daemon == nil {
		return result, fmt.Errorf("daemon client is not available")
	}
	if execution == nil {
		return result, fmt.Errorf("execution is required")
	}

	result = normalizeOrchestratorTaskResult(result)
	state := &OrchestratorDelegatedTaskMemoryState{
		ChildAgentID:          strings.TrimSpace(child.ID),
		ChildPerAgentMemoryID: strings.TrimSpace(child.PerAgentMemoryID),
		SnapshotID:            strings.TrimSpace(child.SnapshotID),
		SnapshotDigest:        strings.TrimSpace(child.SnapshotDigest),
	}
	result.DelegatedMemory = state

	requestID := "orchestrator-" + strings.TrimSpace(execution.ID)
	actor := "gateway:orchestrator:delegated"
	childScope := "agent:" + strings.TrimSpace(child.ID)

	distillResult, err := daemon.DistillInstanceMemory(
		ctx,
		strings.TrimSpace(child.ID),
		childScope,
		true,
		actor,
		requestID,
		fmt.Sprintf("finalize delegated task %s for execution %s", strings.TrimSpace(task.ID), strings.TrimSpace(execution.ID)),
	)
	if err != nil {
		return result, fmt.Errorf("distill delegated child memory: %w", err)
	}
	state.DistillRunID = strings.TrimSpace(distillResult.RunID)

	summaries := make([]string, 0, len(distillResult.OutputIDs))
	for _, outputID := range normalizeStringSelectorList(distillResult.OutputIDs, true) {
		record, err := daemon.GetMemoryRecord(ctx, strings.TrimSpace(child.ID), outputID, actor, requestID)
		if err != nil {
			return result, fmt.Errorf("read distilled child record %s: %w", outputID, err)
		}
		if summary := strings.TrimSpace(record.ContentSummary); summary != "" {
			summaries = append(summaries, summary)
		}
	}

	writebackSummary := buildDelegatedWritebackSummary(task, result, summaries)
	parentRecord, err := daemon.UpsertMemoryRecord(
		ctx,
		strings.TrimSpace(child.ParentAgentID),
		"agent:"+strings.TrimSpace(child.ParentAgentID),
		"note",
		writebackSummary,
		buildDelegatedWritebackProvenance(execution, task, child, state.DistillRunID),
		actor,
		requestID,
	)
	if err != nil {
		return result, fmt.Errorf("write back delegated child summary: %w", err)
	}
	if parentRecord != nil && strings.TrimSpace(parentRecord.ID) != "" {
		state.ParentRecordIDs = []string{strings.TrimSpace(parentRecord.ID)}
	}

	cleanupErrors := make([]string, 0, 4)
	if _, err := daemon.PurgeInstanceScope(ctx, strings.TrimSpace(child.ID), childScope, actor, requestID); err != nil {
		cleanupErrors = append(cleanupErrors, err.Error())
	}
	if snapshotID := strings.TrimSpace(child.SnapshotID); snapshotID != "" {
		if err := daemon.DeleteInstanceSnapshot(ctx, snapshotID, actor, requestID); err != nil {
			cleanupErrors = append(cleanupErrors, err.Error())
		}
	}
	if memoryID := strings.TrimSpace(child.PerAgentMemoryID); memoryID != "" {
		if err := daemon.ArchiveMemoryEntry(ctx, memoryID, actor, requestID); err != nil {
			cleanupErrors = append(cleanupErrors, err.Error())
		}
	}
	if err := deleteManagedInstance(child.ID); err != nil {
		cleanupErrors = append(cleanupErrors, err.Error())
	}
	if len(cleanupErrors) == 0 {
		state.CleanupStatus = delegatedCleanupStatusCompleted
	} else {
		state.CleanupStatus = delegatedCleanupStatusPending
	}

	execution.ChildAgentID = child.ID
	execution.ChildPerAgentMemoryID = child.PerAgentMemoryID
	execution.SnapshotID = child.SnapshotID
	execution.SnapshotDigest = child.SnapshotDigest
	execution.DistillRunID = state.DistillRunID
	execution.CleanupStatus = state.CleanupStatus
	if _, err := upsertOrchestratorExecution(*execution); err != nil {
		return result, fmt.Errorf("persist delegated finalize state: %w", err)
	}

	return result, nil
}

func buildDelegatedWritebackSummary(task OrchestratorTaskUnit, result OrchestratorTaskResult, distilledSummaries []string) string {
	lines := []string{fmt.Sprintf("Delegated task %s summary", strings.TrimSpace(task.ID))}
	if output := strings.TrimSpace(result.Output); output != "" {
		lines = append(lines, output)
	}
	for _, summary := range distilledSummaries {
		if trimmed := strings.TrimSpace(summary); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return truncateDelegateText(strings.Join(lines, "\n"), 1200)
}

func buildDelegatedWritebackProvenance(
	execution *OrchestratorExecution,
	task OrchestratorTaskUnit,
	child managedAgentInstance,
	distillRunID string,
) string {
	return strings.TrimSpace(fmt.Sprintf(
		"delegated:execution=%s task=%s child=%s snapshot=%s distill=%s",
		strings.TrimSpace(execution.ID),
		strings.TrimSpace(task.ID),
		strings.TrimSpace(child.ID),
		strings.TrimSpace(child.SnapshotDigest),
		strings.TrimSpace(distillRunID),
	))
}
