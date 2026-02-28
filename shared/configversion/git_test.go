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
