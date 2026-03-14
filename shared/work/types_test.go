package work

import (
	"errors"
	"strings"
	"testing"
)

func TestWorkProjectNormalizeDefaults(t *testing.T) {
	project, err := NormalizeProject(Project{
		Name:       " Carrier ",
		SourceType: SourceTypeLocal,
		SourceRef:  " /tmp/carrier ",
	})
	if err != nil {
		t.Fatalf("NormalizeProject error: %v", err)
	}

	if !strings.HasPrefix(project.ID, "proj_") {
		t.Fatalf("id=%q", project.ID)
	}
	if project.Name != "Carrier" {
		t.Fatalf("name=%q", project.Name)
	}
	if project.SourceRef != "/tmp/carrier" {
		t.Fatalf("sourceRef=%q", project.SourceRef)
	}
	if project.DefaultBranch != "main" {
		t.Fatalf("defaultBranch=%q", project.DefaultBranch)
	}
	if project.WorkflowPath != "WORKFLOW.md" {
		t.Fatalf("workflowPath=%q", project.WorkflowPath)
	}
	if project.State != ProjectStateRegistered {
		t.Fatalf("state=%q", project.State)
	}
}

func TestWorkItemNormalizeDefaults(t *testing.T) {
	item, err := NormalizeWorkItem(WorkItem{
		ProjectID:   "proj_123",
		Title:       "  Add Work Queue  ",
		Description: "  Build queue primitives  ",
		Acceptance:  []string{" queue renders ", "queue renders", " actions work "},
		Labels:      []string{" backend ", "backend", " work "},
	})
	if err != nil {
		t.Fatalf("NormalizeWorkItem error: %v", err)
	}

	if !strings.HasPrefix(item.ID, "work_") {
		t.Fatalf("id=%q", item.ID)
	}
	if item.Title != "Add Work Queue" {
		t.Fatalf("title=%q", item.Title)
	}
	if item.Description != "Build queue primitives" {
		t.Fatalf("description=%q", item.Description)
	}
	if item.Priority != WorkPriorityNormal {
		t.Fatalf("priority=%q", item.Priority)
	}
	if item.Source != WorkSourceLocal {
		t.Fatalf("source=%q", item.Source)
	}
	if item.SourceRef != "local:manual" {
		t.Fatalf("sourceRef=%q", item.SourceRef)
	}
	if item.State != WorkItemStateNew {
		t.Fatalf("state=%q", item.State)
	}
	if got, want := len(item.Acceptance), 2; got != want {
		t.Fatalf("acceptance len=%d want=%d", got, want)
	}
	if got, want := len(item.Labels), 2; got != want {
		t.Fatalf("labels len=%d want=%d", got, want)
	}
}

func TestWorkRunNormalizeDefaults(t *testing.T) {
	run, err := NormalizeRun(Run{
		ProjectID:  "proj_123",
		WorkItemID: "work_123",
	})
	if err != nil {
		t.Fatalf("NormalizeRun error: %v", err)
	}

	if !strings.HasPrefix(run.ID, "run_") {
		t.Fatalf("id=%q", run.ID)
	}
	if run.Backend != RunBackendLocalSandboxed {
		t.Fatalf("backend=%q", run.Backend)
	}
	if run.Phase != RunPhaseCreated {
		t.Fatalf("phase=%q", run.Phase)
	}
	if run.VerificationStatus != VerificationStatusPending {
		t.Fatalf("verification=%q", run.VerificationStatus)
	}
	if run.PublishStatus != PublishStatusPending {
		t.Fatalf("publish=%q", run.PublishStatus)
	}
}

func TestWorkRunNormalizeRejectsUnknownBackend(t *testing.T) {
	_, err := NormalizeRun(Run{
		ProjectID:  "proj_123",
		WorkItemID: "work_123",
		Backend:    "weird",
	})
	if err == nil || !strings.Contains(err.Error(), "backend") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeProjectFailsWhenRandomIDGenerationFails(t *testing.T) {
	prev := randomIDRead
	randomIDRead = func([]byte) (int, error) {
		return 0, errors.New("entropy unavailable")
	}
	defer func() {
		randomIDRead = prev
	}()

	_, err := NormalizeProject(Project{
		Name:       "Carrier",
		SourceType: SourceTypeLocal,
		SourceRef:  "/tmp/carrier",
	})
	if err == nil || !strings.Contains(err.Error(), "failed to read random bytes") {
		t.Fatalf("unexpected error: %v", err)
	}
}
