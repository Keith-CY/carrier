package gateway

import (
	"carrier/shared/work"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkRunStartCreatesExecutionBindingAndWorkspace(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	project, err := upsertWorkProject(work.Project{
		ID:         "proj_123",
		Name:       "carrier",
		SourceType: work.SourceTypeLocal,
		SourceRef:  createGatewayTestGitRepo(t),
	})
	if err != nil {
		t.Fatalf("upsertWorkProject error: %v", err)
	}
	if _, err := syncWorkProject(project.ID); err != nil {
		t.Fatalf("syncWorkProject error: %v", err)
	}
	item, err := upsertWorkItem(work.WorkItem{
		ID:        "work_123",
		ProjectID: project.ID,
		Title:     "Add queue",
	})
	if err != nil {
		t.Fatalf("upsertWorkItem error: %v", err)
	}

	run, err := startWorkRun(item.ID, work.RunBackendLocalSandboxed)
	if err != nil {
		t.Fatalf("startWorkRun error: %v", err)
	}
	if run.ExecutionID == "" {
		t.Fatal("expected execution binding")
	}
	if run.WorkspacePath == "" {
		t.Fatal("expected workspace path")
	}
	if _, err := os.Stat(run.WorkspacePath); err != nil {
		t.Fatalf("expected workspace path: %v", err)
	}
	if !strings.Contains(run.WorkspacePath, filepath.Join("worktrees", run.ID)) {
		t.Fatalf("workspacePath=%q", run.WorkspacePath)
	}

	execution, ok, err := getOrchestratorExecution(run.ExecutionID)
	if err != nil {
		t.Fatalf("getOrchestratorExecution error: %v", err)
	}
	if !ok {
		t.Fatal("expected orchestrator execution")
	}
	if execution.Mode != OrchestratorExecutionModeWork {
		t.Fatalf("mode=%q", execution.Mode)
	}
	if execution.Work.WorkItemID != item.ID {
		t.Fatalf("workItemId=%q", execution.Work.WorkItemID)
	}
}

func TestWorkRunStartRejectsConcurrentActiveRun(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	project, err := upsertWorkProject(work.Project{
		ID:         "proj_123",
		Name:       "carrier",
		SourceType: work.SourceTypeLocal,
		SourceRef:  createGatewayTestGitRepo(t),
	})
	if err != nil {
		t.Fatalf("upsertWorkProject error: %v", err)
	}
	if _, err := syncWorkProject(project.ID); err != nil {
		t.Fatalf("syncWorkProject error: %v", err)
	}
	item, err := upsertWorkItem(work.WorkItem{
		ID:        "work_123",
		ProjectID: project.ID,
		Title:     "Add queue",
	})
	if err != nil {
		t.Fatalf("upsertWorkItem error: %v", err)
	}
	if _, err := startWorkRun(item.ID, work.RunBackendLocalSandboxed); err != nil {
		t.Fatalf("startWorkRun error: %v", err)
	}
	if _, err := startWorkRun(item.ID, work.RunBackendLocalSandboxed); err == nil {
		t.Fatal("expected concurrent active run rejection")
	}
}

func TestWorkRunActionsSyncWorkItemAndExecutionContext(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	project, err := upsertWorkProject(work.Project{
		ID:         "proj_123",
		Name:       "carrier",
		SourceType: work.SourceTypeLocal,
		SourceRef:  createGatewayTestGitRepo(t),
	})
	if err != nil {
		t.Fatalf("upsertWorkProject error: %v", err)
	}
	if _, err := syncWorkProject(project.ID); err != nil {
		t.Fatalf("syncWorkProject error: %v", err)
	}
	item, err := upsertWorkItem(work.WorkItem{
		ID:        "work_123",
		ProjectID: project.ID,
		Title:     "Add queue",
	})
	if err != nil {
		t.Fatalf("upsertWorkItem error: %v", err)
	}

	run, err := startWorkRun(item.ID, work.RunBackendManaged)
	if err != nil {
		t.Fatalf("startWorkRun error: %v", err)
	}
	if _, err := reclaimWorkRun(run.ID); err != nil {
		t.Fatalf("reclaimWorkRun error: %v", err)
	}
	run, ok, err := getWorkRun(run.ID)
	if err != nil || !ok {
		t.Fatalf("getWorkRun after reclaim error=%v ok=%v", err, ok)
	}
	execution, ok, err := getOrchestratorExecution(run.ExecutionID)
	if err != nil || !ok {
		t.Fatalf("getOrchestratorExecution after reclaim error=%v ok=%v", err, ok)
	}
	if execution.Work.Backend != string(work.RunBackendManaged) {
		t.Fatalf("execution.Work.Backend=%q want %q", execution.Work.Backend, work.RunBackendManaged)
	}
	if execution.Work.Phase != string(work.RunPhaseExecuting) {
		t.Fatalf("execution.Work.Phase=%q want executing", execution.Work.Phase)
	}

	cancelled, err := cancelWorkRun(run.ID)
	if err != nil {
		t.Fatalf("cancelWorkRun error: %v", err)
	}
	currentItem, ok, err := getWorkItem(item.ID)
	if err != nil || !ok {
		t.Fatalf("getWorkItem after cancel error=%v ok=%v", err, ok)
	}
	if currentItem.State != work.WorkItemStateCancelled {
		t.Fatalf("item.State=%q want %q", currentItem.State, work.WorkItemStateCancelled)
	}
	if cancelled.Phase != work.RunPhaseCancelled {
		t.Fatalf("cancelled.Phase=%q want %q", cancelled.Phase, work.RunPhaseCancelled)
	}
	execution, ok, err = getOrchestratorExecution(run.ExecutionID)
	if err != nil || !ok {
		t.Fatalf("getOrchestratorExecution after cancel error=%v ok=%v", err, ok)
	}
	if execution.Work.PublishStatus != string(cancelled.PublishStatus) {
		t.Fatalf("execution.Work.PublishStatus=%q want %q", execution.Work.PublishStatus, cancelled.PublishStatus)
	}
	if execution.Status != OrchestratorExecutionStatusCancelled {
		t.Fatalf("execution.Status=%q want %q", execution.Status, OrchestratorExecutionStatusCancelled)
	}
}
