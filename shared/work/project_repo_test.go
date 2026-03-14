package work

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectRepoPathsResolveUnderProjectsRoot(t *testing.T) {
	roots := Roots{
		Root:     "/tmp/carrier",
		App:      "/tmp/carrier/app",
		Projects: "/tmp/carrier/projects",
		Works:    "/tmp/carrier/works",
	}

	paths, err := ResolveProjectPaths(roots, "proj_123")
	if err != nil {
		t.Fatalf("ResolveProjectPaths error: %v", err)
	}

	if paths.Root != "/tmp/carrier/projects/proj_123" {
		t.Fatalf("root=%q", paths.Root)
	}
	if paths.Repo != "/tmp/carrier/projects/proj_123/repo" {
		t.Fatalf("repo=%q", paths.Repo)
	}
	if paths.Worktrees != "/tmp/carrier/projects/proj_123/worktrees" {
		t.Fatalf("worktrees=%q", paths.Worktrees)
	}
	if paths.Workflow != "/tmp/carrier/projects/proj_123/workflow" {
		t.Fatalf("workflow=%q", paths.Workflow)
	}
}

func TestProjectRepoSyncCreatesCanonicalRepoAndWorkflowDigest(t *testing.T) {
	sourceRepo := createTestGitRepo(t)
	t.Setenv("CARRIER_ROOT", t.TempDir())
	t.Setenv("CARRIER_APP_ROOT", "")
	t.Setenv("CARRIER_PROJECTS_ROOT", "")
	t.Setenv("CARRIER_WORKS_ROOT", "")

	project, err := NormalizeProject(Project{
		ID:         "proj_123",
		Name:       "carrier",
		SourceType: SourceTypeLocal,
		SourceRef:  sourceRepo,
	})
	if err != nil {
		t.Fatalf("NormalizeProject error: %v", err)
	}

	synced, paths, err := SyncProjectRepo(project)
	if err != nil {
		t.Fatalf("SyncProjectRepo error: %v", err)
	}

	if synced.State != ProjectStateReady {
		t.Fatalf("state=%q", synced.State)
	}
	if synced.WorkflowDigest == "" {
		t.Fatal("expected workflow digest")
	}
	if _, err := os.Stat(filepath.Join(paths.Repo, ".git")); err != nil {
		t.Fatalf("expected repo clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.Workflow, "current.md")); err != nil {
		t.Fatalf("expected workflow snapshot: %v", err)
	}
}

func createTestGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "user.email", "tests@example.com")
	runGit(t, repo, "config", "user.name", "Carrier Tests")
	if err := os.WriteFile(filepath.Join(repo, "WORKFLOW.md"), []byte("# Workflow\n"), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	runGit(t, repo, "add", "WORKFLOW.md", "README.md")
	runGit(t, repo, "commit", "-m", "initial")
	return repo
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}
