package gateway

import (
	"carrier/shared/work"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

func prepareWorkItemForRun(item work.WorkItem) (work.WorkItem, error) {
	current := item
	var err error
	switch current.State {
	case work.WorkItemStateNew:
		current, err = setWorkItemState(current.ID, work.WorkItemStateTriaged, "")
		if err != nil {
			return work.WorkItem{}, err
		}
		fallthrough
	case work.WorkItemStateTriaged, work.WorkItemStateBlocked:
		current, err = setWorkItemState(current.ID, work.WorkItemStateQueued, "")
		if err != nil {
			return work.WorkItem{}, err
		}
	case work.WorkItemStateQueued:
		return current, nil
	case work.WorkItemStateClaimed, work.WorkItemStateRunning:
		return work.WorkItem{}, fmt.Errorf("work item %s already has an active run", current.ID)
	case work.WorkItemStateAwaitingReview, work.WorkItemStateDone, work.WorkItemStateCancelled:
		return work.WorkItem{}, fmt.Errorf("work item %s is terminal", current.ID)
	}
	return current, nil
}

func buildWorkExecution(project work.Project, item work.WorkItem, run work.Run, workspacePath string) OrchestratorExecution {
	goal := strings.TrimSpace(item.Title)
	if description := strings.TrimSpace(item.Description); description != "" {
		goal += "\n\n" + description
	}
	now := nowTimestamp()
	return OrchestratorExecution{
		ID:   uuid.NewString(),
		Mode: OrchestratorExecutionModeWork,
		Work: OrchestratorExecutionWorkContext{
			ProjectID:          project.ID,
			WorkItemID:         item.ID,
			RunID:              run.ID,
			WorkspaceID:        run.WorkspaceID,
			WorkspacePath:      workspacePath,
			Backend:            string(run.Backend),
			WorkflowDigest:     project.WorkflowDigest,
			Phase:              string(work.RunPhaseExecuting),
			VerificationStatus: string(run.VerificationStatus),
			PublishStatus:      string(run.PublishStatus),
		},
		Goal:          goal,
		Project:       project.Name,
		Initiator:     string(item.Source),
		Status:        OrchestratorExecutionStatusRunning,
		ApprovalScope: "infrastructure_only",
		ToolPolicy:    normalizeOrchestratorToolPolicy(OrchestratorToolPolicy{}),
		Outcome: OrchestratorExecutionOutcome{
			Summary: fmt.Sprintf("work run %s bound to workspace %s", run.ID, workspacePath),
		},
		Results:   []OrchestratorTaskResult{},
		CreatedAt: now,
		StartedAt: now,
		UpdatedAt: now,
	}
}

func createWorkRunWorkspace(repoPath, workspacePath, branch string) error {
	if err := os.MkdirAll(filepath.Dir(workspacePath), 0o700); err != nil {
		return fmt.Errorf("create worktree parent: %w", err)
	}
	return work.RunGitForWorktree(repoPath, workspacePath, branch)
}

func updateWorkExecutionStatus(executionID string, status OrchestratorExecutionStatus, summary string) error {
	execution, ok, err := getOrchestratorExecution(executionID)
	if err != nil {
		return err
	}
	if !ok {
		return os.ErrNotExist
	}
	execution.Status = status
	execution.UpdatedAt = nowTimestamp()
	switch status {
	case OrchestratorExecutionStatusRunning:
		if execution.StartedAt == "" {
			execution.StartedAt = nowTimestamp()
		}
	case OrchestratorExecutionStatusCompleted, OrchestratorExecutionStatusCancelled:
		if execution.CompletedAt == "" {
			execution.CompletedAt = nowTimestamp()
		}
	}
	if strings.TrimSpace(summary) != "" {
		execution.Outcome.Summary = strings.TrimSpace(summary)
	}
	_, err = upsertOrchestratorExecution(execution)
	return err
}

func syncWorkExecutionFromRun(run work.Run) error {
	if strings.TrimSpace(run.ExecutionID) == "" {
		return nil
	}
	execution, ok, err := getOrchestratorExecution(run.ExecutionID)
	if err != nil {
		return err
	}
	if !ok {
		return os.ErrNotExist
	}
	execution.Work.ProjectID = strings.TrimSpace(run.ProjectID)
	execution.Work.WorkItemID = strings.TrimSpace(run.WorkItemID)
	execution.Work.RunID = strings.TrimSpace(run.ID)
	execution.Work.WorkspaceID = strings.TrimSpace(run.WorkspaceID)
	execution.Work.WorkspacePath = strings.TrimSpace(run.WorkspacePath)
	execution.Work.Backend = strings.TrimSpace(string(run.Backend))
	execution.Work.WorkflowDigest = strings.TrimSpace(run.WorkflowDigest)
	execution.Work.Phase = strings.TrimSpace(string(run.Phase))
	execution.Work.VerificationStatus = strings.TrimSpace(string(run.VerificationStatus))
	execution.Work.PublishStatus = strings.TrimSpace(string(run.PublishStatus))
	execution.UpdatedAt = nowTimestamp()
	_, err = upsertOrchestratorExecution(execution)
	return err
}
