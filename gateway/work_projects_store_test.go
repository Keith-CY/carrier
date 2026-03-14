package gateway

import (
	"carrier/shared/work"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkProjectStoreRegisterAndList(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	project, err := upsertWorkProject(work.Project{
		Name:       "carrier",
		SourceType: work.SourceTypeLocal,
		SourceRef:  createGatewayTestGitRepo(t),
	})
	if err != nil {
		t.Fatalf("upsertWorkProject error: %v", err)
	}

	if project.State != work.ProjectStateRegistered {
		t.Fatalf("state=%q", project.State)
	}

	projects, err := listWorkProjects()
	if err != nil {
		t.Fatalf("listWorkProjects error: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("projects len=%d", len(projects))
	}
}

func TestWorkProjectStoreSyncTransitionsToReady(t *testing.T) {
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	project, err := upsertWorkProject(work.Project{
		ID:         "proj_sync",
		Name:       "carrier",
		SourceType: work.SourceTypeLocal,
		SourceRef:  createGatewayTestGitRepo(t),
	})
	if err != nil {
		t.Fatalf("upsertWorkProject error: %v", err)
	}

	synced, err := syncWorkProject(project.ID)
	if err != nil {
		t.Fatalf("syncWorkProject error: %v", err)
	}
	if synced.State != work.ProjectStateReady {
		t.Fatalf("state=%q", synced.State)
	}
	if synced.WorkflowDigest == "" {
		t.Fatal("expected workflow digest")
	}
}

func createGatewayTestGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGatewayGit(t, repo, "init", "-b", "main")
	runGatewayGit(t, repo, "config", "user.email", "tests@example.com")
	runGatewayGit(t, repo, "config", "user.name", "Carrier Tests")
	if err := os.WriteFile(filepath.Join(repo, "WORKFLOW.md"), []byte("# Workflow\n"), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	runGatewayGit(t, repo, "add", "WORKFLOW.md", "README.md")
	runGatewayGit(t, repo, "commit", "-m", "initial")
	return repo
}

func runGatewayGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}
