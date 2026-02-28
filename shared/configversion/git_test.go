package configversion

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

func commitCount(t *testing.T, dir string) int {
	t.Helper()
	out, err := runGit(dir, "rev-list", "--count", "HEAD")
	if err != nil {
		t.Fatalf("rev-list --count HEAD failed: %v", err)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		t.Fatalf("empty commit count")
	}
	var n int
	if _, err := fmt.Sscanf(out, "%d", &n); err != nil {
		t.Fatalf("parse commit count %q: %v", out, err)
	}
	return n
}

func TestInitRepoCreatesGitAndIgnore(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()

	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("expected .git to exist: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	content := string(raw)
	for _, want := range []string{"credentials.json", "carrier-secrets.json", "*.key", "*.pem", "tls/"} {
		if !strings.Contains(content, want) {
			t.Fatalf(".gitignore missing %q", want)
		}
	}
}

func TestInitRepoIsIdempotent(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()

	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo first call failed: %v", err)
	}
	firstCount := commitCount(t, dir)
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo second call failed: %v", err)
	}
	secondCount := commitCount(t, dir)
	if firstCount != secondCount {
		t.Fatalf("expected idempotent commit count, got %d then %d", firstCount, secondCount)
	}
}

func TestCommitChangeCreatesCommit(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}
	before := commitCount(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "instances.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write instances.json: %v", err)
	}
	CommitChange(dir, "add agent picoclaw")

	after := commitCount(t, dir)
	if after <= before {
		t.Fatalf("expected new commit, before=%d after=%d", before, after)
	}
}

func TestCommitChangeNoOpWhenNoChanges(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}
	before := commitCount(t, dir)
	CommitChange(dir, "no changes")
	after := commitCount(t, dir)
	if before != after {
		t.Fatalf("expected no new commit, before=%d after=%d", before, after)
	}
}

func TestCommitChangeNoOpWhenNotGitRepo(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	CommitChange(dir, "ignored")
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Fatalf("expected no .git directory, err=%v", err)
	}
}

func TestInitRepoEmptyDir(t *testing.T) {
	if err := InitRepo(""); err == nil {
		t.Fatal("expected error for empty dir")
	}
	if err := InitRepo("   "); err == nil {
		t.Fatal("expected error for whitespace dir")
	}
}

func TestCommitChangeEmptyDir(t *testing.T) {
	requireGit(t)
	// Should not panic
	CommitChange("", "test")
	CommitChange("   ", "test")
}

func TestCommitChangeEmptyMessage(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "test.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	CommitChange(dir, "")
	after := commitCount(t, dir)
	if after != 2 {
		t.Fatalf("expected 2 commits (init + empty-message), got %d", after)
	}
	// Verify default message was used
	out, err := runGit(dir, "log", "--format=%s", "-1")
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(out, "update config") {
		t.Fatalf("expected default commit message, got %q", out)
	}
}

func TestGitAvailable(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not available")
	}
}

func TestIsGitRepoFalseForNonRepo(t *testing.T) {
	if isGitRepo(t.TempDir()) {
		t.Fatal("expected false for non-repo")
	}
}

func TestEnsureGitIdentity(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	// Should be idempotent
	if err := ensureGitIdentity(dir); err != nil {
		t.Fatalf("ensureGitIdentity: %v", err)
	}
	name, _ := runGit(dir, "config", "--get", "user.name")
	if strings.TrimSpace(name) == "" {
		t.Fatal("expected user.name to be set")
	}
}

func TestRunGitErrors(t *testing.T) {
	requireGit(t)
	// Invalid git command
	_, err := runGit("", "not-a-real-command")
	if err == nil {
		t.Fatal("expected error for invalid git command")
	}
}

func TestInitRepoCommitExists(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	// Verify at least one commit exists
	n := commitCount(t, dir)
	if n < 1 {
		t.Fatalf("expected at least 1 commit, got %d", n)
	}
	// Verify commit message
	out, err := runGit(dir, "log", "--format=%s", "-1")
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(out, "init config versioning") {
		t.Fatalf("unexpected commit message: %q", out)
	}
}

func TestCommitChangeMultipleFiles(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	// Write multiple files
	for _, name := range []string{"a.json", "b.json", "c.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	CommitChange(dir, "add multiple files")
	// All should be tracked
	out, err := runGit(dir, "ls-files")
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	for _, name := range []string{"a.json", "b.json", "c.json"} {
		if !strings.Contains(out, name) {
			t.Fatalf("expected %s to be tracked", name)
		}
	}
}

func TestInitRepoMkdirFails(t *testing.T) {
	requireGit(t)
	origMkdir := mkdirAll
	mkdirAll = func(path string, perm os.FileMode) error {
		return fmt.Errorf("disk full")
	}
	defer func() { mkdirAll = origMkdir }()

	err := InitRepo("/tmp/test-carrier-init-fail")
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("expected disk full error, got %v", err)
	}
}

func TestInitRepoWriteFileFails(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	origWrite := writeFile
	writeFile = func(path string, data []byte, perm os.FileMode) error {
		if strings.HasSuffix(path, ".gitignore") {
			return fmt.Errorf("permission denied")
		}
		return os.WriteFile(path, data, perm)
	}
	defer func() { writeFile = origWrite }()

	err := InitRepo(dir)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestInitRepoGitUnavailable(t *testing.T) {
	origLook := lookPath
	lookPath = func(file string) (string, error) {
		return "", fmt.Errorf("not found")
	}
	defer func() { lookPath = origLook }()

	// Should return nil (silently skip)
	if err := InitRepo(t.TempDir()); err != nil {
		t.Fatalf("expected nil when git unavailable, got %v", err)
	}
}

func TestCommitChangeGitAddFails(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	origRun := execRunGit
	execRunGit = func(d string, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "add" {
			return "", fmt.Errorf("add failed")
		}
		return origRun(d, args...)
	}
	defer func() { execRunGit = origRun }()

	// Write a file so there's something to commit
	os.WriteFile(filepath.Join(dir, "x.json"), []byte("{}"), 0o600)
	// Should not panic
	CommitChange(dir, "test")
}

func TestCommitChangeGitCommitFails(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	origRun := execRunGit
	execRunGit = func(d string, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "commit" {
			return "", fmt.Errorf("commit failed")
		}
		return origRun(d, args...)
	}
	defer func() { execRunGit = origRun }()

	os.WriteFile(filepath.Join(dir, "y.json"), []byte("{}"), 0o600)
	CommitChange(dir, "test")
}

func TestEnsureGitIdentityFreshRepo(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	// Init a bare git repo without identity
	runGitImpl("", "init", dir)
	// Force local config to be empty
	runGitImpl(dir, "config", "--local", "--unset-all", "user.name")
	runGitImpl(dir, "config", "--local", "--unset-all", "user.email")
	if err := ensureGitIdentity(dir); err != nil {
		t.Fatalf("ensureGitIdentity: %v", err)
	}
}

func TestGitIgnoreExclusions(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	if err := InitRepo(dir); err != nil {
		t.Fatalf("InitRepo: %v", err)
	}
	// Write a file that should be ignored
	if err := os.WriteFile(filepath.Join(dir, "credentials.json"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	CommitChange(dir, "should ignore credentials")
	// Should still be 1 commit (init only)
	n := commitCount(t, dir)
	if n != 1 {
		t.Fatalf("expected 1 commit (credentials.json should be ignored), got %d", n)
	}
}
