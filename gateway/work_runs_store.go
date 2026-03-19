package gateway

import (
	"carrier/shared/work"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var workRunsStoreMu sync.Mutex

func listWorkRuns() ([]work.Run, error) {
	workRunsStoreMu.Lock()
	defer workRunsStoreMu.Unlock()

	roots, err := work.ResolveRoots()
	if err != nil {
		return nil, err
	}
	projectEntries, err := os.ReadDir(roots.Works)
	if err != nil {
		if os.IsNotExist(err) {
			return []work.Run{}, nil
		}
		return nil, fmt.Errorf("read works root: %w", err)
	}
	runs := make([]work.Run, 0)
	for _, entry := range projectEntries {
		if !entry.IsDir() {
			continue
		}
		runDir := filepath.Join(roots.Works, entry.Name(), "runs")
		fileEntries, readErr := os.ReadDir(runDir)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			return nil, fmt.Errorf("read work runs dir: %w", readErr)
		}
		for _, fileEntry := range fileEntries {
			if fileEntry.IsDir() || !strings.HasSuffix(fileEntry.Name(), ".json") {
				continue
			}
			run, err := loadWorkRunFromPath(filepath.Join(runDir, fileEntry.Name()))
			if err != nil {
				return nil, err
			}
			runs = append(runs, run)
		}
	}
	return runs, nil
}

func getWorkRun(runID string) (work.Run, bool, error) {
	workRunsStoreMu.Lock()
	defer workRunsStoreMu.Unlock()

	run, _, err := loadWorkRunByID(runID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return work.Run{}, false, nil
		}
		return work.Run{}, false, err
	}
	return run, true, nil
}

func setWorkRunPhase(runID string, phase work.RunPhase) (work.Run, error) {
	workRunsStoreMu.Lock()
	defer workRunsStoreMu.Unlock()
	run, path, err := loadWorkRunByID(runID)
	if err != nil {
		return work.Run{}, err
	}
	run.Phase = phase
	run.UpdatedAt = nowTimestamp()
	if err := saveWorkRunUnlocked(run); err != nil {
		return work.Run{}, err
	}
	run, err = loadWorkRunFromPath(path)
	if err != nil {
		return work.Run{}, err
	}
	if err := syncWorkExecutionFromRun(run); err != nil && !os.IsNotExist(err) {
		return work.Run{}, err
	}
	return run, nil
}

func startWorkRun(itemID string, backend work.RunBackend) (work.Run, error) {
	item, ok, err := getWorkItem(itemID)
	if err != nil {
		return work.Run{}, err
	}
	if !ok {
		return work.Run{}, os.ErrNotExist
	}
	if hasActive, err := hasActiveRunForWorkItem(item.ID); err != nil {
		return work.Run{}, err
	} else if hasActive {
		return work.Run{}, fmt.Errorf("work item %s already has an active run", item.ID)
	}
	project, ok, err := getWorkProject(item.ProjectID)
	if err != nil {
		return work.Run{}, err
	}
	if !ok {
		return work.Run{}, os.ErrNotExist
	}
	if project.State != work.ProjectStateReady {
		project, err = syncWorkProject(project.ID)
		if err != nil {
			return work.Run{}, err
		}
	}
	item, err = prepareWorkItemForRun(item)
	if err != nil {
		return work.Run{}, err
	}

	run, err := work.NormalizeRun(work.Run{
		ProjectID:          project.ID,
		WorkItemID:         item.ID,
		Backend:            backend,
		Phase:              work.RunPhasePreparing,
		LeaseOwner:         "carrier:local",
		VerificationStatus: work.VerificationStatusPending,
		PublishStatus:      work.PublishStatusPending,
		WorkflowDigest:     project.WorkflowDigest,
	})
	if err != nil {
		return work.Run{}, err
	}
	run.WorkspaceID = "ws_" + strings.TrimPrefix(run.ID, "run_")
	run.CreatedAt = nowTimestamp()
	run.UpdatedAt = run.CreatedAt

	paths, err := work.ResolveProjectPathsFromProject(project)
	if err != nil {
		return work.Run{}, err
	}
	workspacePath := filepath.Join(paths.Worktrees, run.ID)
	if err := createWorkRunWorkspace(paths.Repo, workspacePath, project.DefaultBranch); err != nil {
		return work.Run{}, err
	}

	execution := buildWorkExecution(project, item, run, workspacePath)
	savedExecution, err := upsertOrchestratorExecution(execution)
	if err != nil {
		return work.Run{}, err
	}
	run.ExecutionID = savedExecution.ID
	run.WorkspacePath = workspacePath
	run.Phase = work.RunPhaseExecuting
	run.UpdatedAt = nowTimestamp()
	if err := saveWorkRun(run); err != nil {
		return work.Run{}, err
	}
	if _, err := setWorkItemState(item.ID, work.WorkItemStateClaimed, run.ID); err != nil {
		return work.Run{}, err
	}
	if _, err := setWorkItemState(item.ID, work.WorkItemStateRunning, run.ID); err != nil {
		return work.Run{}, err
	}
	return run, nil
}

func saveWorkRun(run work.Run) error {
	workRunsStoreMu.Lock()
	defer workRunsStoreMu.Unlock()
	if err := saveWorkRunUnlocked(run); err != nil {
		return err
	}
	if err := syncWorkExecutionFromRun(run); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func saveWorkRunUnlocked(run work.Run) error {
	normalized, err := work.NormalizeRun(run)
	if err != nil {
		return err
	}
	path, err := workRunPath(normalized.ProjectID, normalized.ID)
	if err != nil {
		return err
	}
	if normalized.CreatedAt == "" {
		normalized.CreatedAt = nowTimestamp()
	}
	if normalized.UpdatedAt == "" {
		normalized.UpdatedAt = normalized.CreatedAt
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create work run dir: %w", err)
	}
	raw, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal work run: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write work run: %w", err)
	}
	return nil
}

func workRunPath(projectID, runID string) (string, error) {
	roots, err := work.ResolveRoots()
	if err != nil {
		return "", err
	}
	projectID, err = work.NormalizeProjectID(projectID)
	if err != nil {
		return "", err
	}
	runID, err = work.NormalizeRunID(runID)
	if err != nil {
		return "", err
	}
	return filepath.Join(roots.Works, projectID, "runs", runID+".json"), nil
}

func loadWorkRunFromPath(path string) (work.Run, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return work.Run{}, fmt.Errorf("read work run: %w", err)
	}
	var run work.Run
	if err := json.Unmarshal(raw, &run); err != nil {
		return work.Run{}, fmt.Errorf("parse work run: %w", err)
	}
	return work.NormalizeRun(run)
}

func loadWorkRunByID(runID string) (work.Run, string, error) {
	roots, err := work.ResolveRoots()
	if err != nil {
		return work.Run{}, "", err
	}
	projectEntries, err := os.ReadDir(roots.Works)
	if err != nil {
		if os.IsNotExist(err) {
			return work.Run{}, "", os.ErrNotExist
		}
		return work.Run{}, "", fmt.Errorf("read works root: %w", err)
	}
	id, err := work.NormalizeRunID(runID)
	if err != nil {
		return work.Run{}, "", err
	}
	for _, entry := range projectEntries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(roots.Works, entry.Name(), "runs", id+".json")
		if _, err := os.Stat(path); err == nil {
			run, err := loadWorkRunFromPath(path)
			return run, path, err
		}
	}
	return work.Run{}, "", os.ErrNotExist
}

func hasActiveRunForWorkItem(itemID string) (bool, error) {
	runs, err := listWorkRuns()
	if err != nil {
		return false, err
	}
	for _, run := range runs {
		if !strings.EqualFold(strings.TrimSpace(run.WorkItemID), strings.TrimSpace(itemID)) {
			continue
		}
		if isActiveRunPhase(run.Phase) {
			return true, nil
		}
	}
	return false, nil
}

func isActiveRunPhase(phase work.RunPhase) bool {
	switch phase {
	case work.RunPhaseCreated,
		work.RunPhasePreparing,
		work.RunPhaseReady,
		work.RunPhaseExecuting,
		work.RunPhaseVerifying,
		work.RunPhasePublishing,
		work.RunPhaseStale:
		return true
	default:
		return false
	}
}

func completeWorkRun(runID string) (work.Run, error) {
	run, err := setWorkRunPhase(runID, work.RunPhaseCompleted)
	if err != nil {
		return work.Run{}, err
	}
	if _, err := setWorkItemState(run.WorkItemID, work.WorkItemStateAwaitingReview, run.ID); err != nil && !os.IsNotExist(err) {
		return work.Run{}, err
	}
	if strings.TrimSpace(run.ExecutionID) != "" {
		if err := updateWorkExecutionStatus(run.ExecutionID, OrchestratorExecutionStatusCompleted, "work run completed"); err != nil {
			return work.Run{}, err
		}
	}
	return run, nil
}

func cancelWorkRun(runID string) (work.Run, error) {
	run, err := setWorkRunPhase(runID, work.RunPhaseCancelled)
	if err != nil {
		return work.Run{}, err
	}
	if _, err := setWorkItemState(run.WorkItemID, work.WorkItemStateCancelled, run.ID); err != nil && !os.IsNotExist(err) {
		return work.Run{}, err
	}
	if strings.TrimSpace(run.ExecutionID) != "" {
		if err := updateWorkExecutionStatus(run.ExecutionID, OrchestratorExecutionStatusCancelled, "work run cancelled"); err != nil {
			return work.Run{}, err
		}
	}
	return run, nil
}

func resumeWorkRun(runID string) (work.Run, error) {
	run, err := setWorkRunPhase(runID, work.RunPhaseExecuting)
	if err != nil {
		return work.Run{}, err
	}
	if _, err := setWorkItemState(run.WorkItemID, work.WorkItemStateRunning, run.ID); err != nil && !os.IsNotExist(err) {
		return work.Run{}, err
	}
	if strings.TrimSpace(run.ExecutionID) != "" {
		if err := updateWorkExecutionStatus(run.ExecutionID, OrchestratorExecutionStatusRunning, "work run resumed"); err != nil {
			return work.Run{}, err
		}
	}
	return run, nil
}

func reclaimWorkRun(runID string) (work.Run, error) {
	run, err := setWorkRunPhase(runID, work.RunPhasePreparing)
	if err != nil {
		return work.Run{}, err
	}
	run, err = setWorkRunPhase(runID, work.RunPhaseExecuting)
	if err != nil {
		return work.Run{}, err
	}
	if _, err := setWorkItemState(run.WorkItemID, work.WorkItemStateRunning, run.ID); err != nil && !os.IsNotExist(err) {
		return work.Run{}, err
	}
	if strings.TrimSpace(run.ExecutionID) != "" {
		if err := updateWorkExecutionStatus(run.ExecutionID, OrchestratorExecutionStatusRunning, "work run reclaimed"); err != nil {
			return work.Run{}, err
		}
	}
	return run, nil
}

func cleanupWorkRun(runID string) error {
	workRunsStoreMu.Lock()
	run, _, err := loadWorkRunByID(runID)
	workRunsStoreMu.Unlock()
	if err != nil {
		return err
	}
	if strings.TrimSpace(run.WorkspacePath) == "" {
		return nil
	}
	roots, err := work.ResolveRoots()
	if err != nil {
		return err
	}
	cleaned := filepath.Clean(run.WorkspacePath)
	prefix := filepath.Clean(roots.Projects) + string(filepath.Separator)
	if !strings.HasPrefix(cleaned, prefix) {
		return fmt.Errorf("workspace path %s is outside project root", cleaned)
	}
	return os.RemoveAll(cleaned)
}
