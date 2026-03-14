package work

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ProjectPaths struct {
	Root      string
	Repo      string
	Worktrees string
	Workflow  string
}

func ResolveProjectPaths(roots Roots, projectID string) (ProjectPaths, error) {
	id, err := NormalizeProjectID(projectID)
	if err != nil {
		return ProjectPaths{}, err
	}
	root := filepath.Join(roots.Projects, id)
	return ProjectPaths{
		Root:      filepath.Clean(root),
		Repo:      filepath.Join(root, "repo"),
		Worktrees: filepath.Join(root, "worktrees"),
		Workflow:  filepath.Join(root, "workflow"),
	}, nil
}

func ResolveProjectPathsFromProject(project Project) (ProjectPaths, error) {
	roots, err := ResolveRoots()
	if err != nil {
		return ProjectPaths{}, err
	}
	return ResolveProjectPaths(roots, project.ID)
}

func SyncProjectRepo(project Project) (Project, ProjectPaths, error) {
	normalized, err := NormalizeProject(project)
	if err != nil {
		return Project{}, ProjectPaths{}, err
	}
	roots, err := ResolveRoots()
	if err != nil {
		return Project{}, ProjectPaths{}, err
	}
	paths, err := ResolveProjectPaths(roots, normalized.ID)
	if err != nil {
		return Project{}, ProjectPaths{}, err
	}
	if err := ensureProjectLayout(paths); err != nil {
		return Project{}, ProjectPaths{}, err
	}
	if err := syncGitRepository(normalized, paths); err != nil {
		return Project{}, ProjectPaths{}, err
	}
	digest, err := snapshotWorkflow(normalized, paths)
	if err != nil {
		return Project{}, ProjectPaths{}, err
	}
	normalized.State = ProjectStateReady
	normalized.WorkflowDigest = digest
	normalized.LastSyncAt = time.Now().UTC().Format(time.RFC3339)
	normalized.LastSyncError = ""
	return normalized, paths, nil
}

func ensureProjectLayout(paths ProjectPaths) error {
	for _, dir := range []string{paths.Root, paths.Worktrees, paths.Workflow} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create project layout %s: %w", dir, err)
		}
	}
	return nil
}

func syncGitRepository(project Project, paths ProjectPaths) error {
	gitDir := filepath.Join(paths.Repo, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		if err := runGitCommand(paths.Repo, "fetch", "--quiet", "origin"); err != nil {
			return err
		}
		if err := runGitCommand(paths.Repo, "checkout", "--quiet", project.DefaultBranch); err != nil {
			return err
		}
		if err := runGitCommand(paths.Repo, "pull", "--quiet", "--ff-only", "origin", project.DefaultBranch); err != nil {
			return err
		}
		return nil
	}
	if _, err := os.Stat(paths.Repo); err == nil {
		entries, readErr := os.ReadDir(paths.Repo)
		if readErr != nil {
			return fmt.Errorf("read canonical repo dir: %w", readErr)
		}
		if len(entries) > 0 {
			return fmt.Errorf("canonical repo dir %s exists but is not a git checkout", paths.Repo)
		}
	}
	parent := filepath.Dir(paths.Repo)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create canonical repo parent: %w", err)
	}
	return runGitCommand("", "clone", "--quiet", "--branch", project.DefaultBranch, "--single-branch", project.SourceRef, paths.Repo)
}

func snapshotWorkflow(project Project, paths ProjectPaths) (string, error) {
	workflowPath := filepath.Join(paths.Repo, project.WorkflowPath)
	raw, err := os.ReadFile(workflowPath)
	if err != nil {
		return "", fmt.Errorf("read workflow contract: %w", err)
	}
	sum := sha256.Sum256(raw)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(paths.Workflow, "current.md"), raw, 0o600); err != nil {
		return "", fmt.Errorf("write workflow snapshot: %w", err)
	}
	if err := os.WriteFile(filepath.Join(paths.Workflow, "digest"), []byte(digest+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write workflow digest: %w", err)
	}
	return digest, nil
}

func runGitCommand(repo string, args ...string) error {
	cmdArgs := make([]string, 0, len(args)+2)
	if strings.TrimSpace(repo) != "" {
		cmdArgs = append(cmdArgs, "-C", repo)
	}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command("git", cmdArgs...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func RunGitForWorktree(repoPath, workspacePath, branch string) error {
	return runGitCommand(repoPath, "worktree", "add", "--force", "--detach", workspacePath, strings.TrimSpace(branch))
}
