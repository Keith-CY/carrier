package profilesync

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProfilesyncPathAndParsingHelpers(t *testing.T) {
	t.Setenv("CARRIER_PROFILESYNC_REPO", "/tmp/custom-profilesync")
	if got, err := profilesyncRepoRoot(); err != nil || got != "/tmp/custom-profilesync" {
		t.Fatalf("profilesyncRepoRoot env override mismatch: got=%q err=%v", got, err)
	}

	t.Setenv("CARRIER_PROFILESYNC_REPO", "")
	if got, err := profilesyncRepoRoot(); err != nil || !strings.Contains(filepath.ToSlash(got), "/.carrier/"+defaultProfilesyncRepoDirName) {
		t.Fatalf("profilesyncRepoRoot default mismatch: got=%q err=%v", got, err)
	}

	if got := sanitizeInstanceID(" a/b\\c:..d "); got != "a_b_c__d" {
		t.Fatalf("unexpected sanitizeInstanceID output: %q", got)
	}
	if got := sanitizeInstanceID("   "); got != "default" {
		t.Fatalf("expected default instance id for empty input, got %q", got)
	}
	if got := normalizeCommitReason("  "); got != "sync" {
		t.Fatalf("unexpected normalizeCommitReason fallback: %q", got)
	}
	if got := normalizeCommitReason("  RollBack "); got != "rollback" {
		t.Fatalf("unexpected normalizeCommitReason normalized value: %q", got)
	}

	parsed, err := parseProfileJSON(`{"k":"v"}`)
	if err != nil || strings.TrimSpace(parsed["k"].(string)) != "v" {
		t.Fatalf("parseProfileJSON valid payload mismatch parsed=%v err=%v", parsed, err)
	}
	emptyParsed, err := parseProfileJSON("   ")
	if err != nil || len(emptyParsed) != 0 {
		t.Fatalf("parseProfileJSON empty payload mismatch parsed=%v err=%v", emptyParsed, err)
	}
	if _, err := parseProfileJSON("{"); err == nil {
		t.Fatal("expected parseProfileJSON invalid payload error")
	}
}

func TestCommandAndInstallHelpers(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go executable not available")
	}

	if !commandExists("go") {
		t.Fatalf("expected commandExists(go)=true")
	}
	if commandExists("definitely-not-a-real-command") {
		t.Fatalf("expected commandExists false for invalid command")
	}

	if got := isAptGetCommand("apt-get", nil); !got {
		t.Fatalf("expected apt-get command detection")
	}
	if got := isAptGetCommand("sudo", []string{"-n", "apt-get", "install", "-y", "git"}); !got {
		t.Fatalf("expected sudo apt-get command detection")
	}
	if got := isAptGetCommand("sudo", []string{"-n", "dnf", "install", "-y", "git"}); got {
		t.Fatalf("expected non apt-get sudo command to be false")
	}

	if got := commandLine("git", nil); got != "git" {
		t.Fatalf("unexpected commandLine without args: %q", got)
	}
	if got := commandLine("git", []string{"status", "--short"}); got != "git status --short" {
		t.Fatalf("unexpected commandLine with args: %q", got)
	}

	short := strings.Repeat("x", 10)
	if got := truncateOutput(short); got != short {
		t.Fatalf("truncateOutput changed short output")
	}
	long := strings.Repeat("x", 900)
	if got := truncateOutput(long); !strings.HasSuffix(got, "...(truncated)") || len(got) >= len(long) {
		t.Fatalf("truncateOutput did not truncate long output")
	}

	out, err := runCommand("go", "version")
	if err != nil || !strings.Contains(strings.ToLower(out), "go version") {
		t.Fatalf("runCommand success mismatch out=%q err=%v", out, err)
	}
	if _, err := runCommand("go", "tool", "definitely-missing-subcommand"); err == nil {
		t.Fatal("expected runCommand failure for invalid go subcommand")
	}

	if err := runGitInstallStrategy(gitInstallStrategy{Name: "noop", InstallSteps: nil}); err != nil {
		t.Fatalf("expected empty install strategy to be no-op, got %v", err)
	}
	if err := runGitInstallStrategy(gitInstallStrategy{
		Name:         "failing",
		RequireRoot:  false,
		InstallSteps: [][]string{{"go", "tool", "definitely-missing-subcommand"}},
	}); err == nil {
		t.Fatal("expected failing install strategy error")
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", "")
	if err := installGit(); err == nil {
		t.Fatal("expected installGit failure without available package managers")
	}
	t.Setenv("PATH", originalPath)

	strategies := gitInstallStrategies()
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
		if len(strategies) == 0 {
			t.Fatalf("expected install strategies for supported os %q", runtime.GOOS)
		}
	default:
		if len(strategies) != 0 {
			t.Fatalf("expected no install strategies for unsupported os %q", runtime.GOOS)
		}
	}
}

func TestPrivilegeAndRemoteHelperValidation(t *testing.T) {
	cmd, args, err := applyPrivilege("apt-get", []string{"install", "git"}, false)
	if err != nil || cmd != "apt-get" || len(args) != 2 {
		t.Fatalf("applyPrivilege no-root mismatch cmd=%q args=%v err=%v", cmd, args, err)
	}

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		cmd, args, err = applyPrivilege("apt-get", []string{"install", "git"}, true)
		if err != nil || cmd != "apt-get" || len(args) != 2 {
			t.Fatalf("applyPrivilege privileged mismatch cmd=%q args=%v err=%v", cmd, args, err)
		}
	} else {
		originalPath := os.Getenv("PATH")
		t.Setenv("PATH", "")
		if _, _, err := applyPrivilege("apt-get", []string{"install", "git"}, true); err == nil {
			t.Fatal("expected applyPrivilege to fail when sudo is unavailable")
		}
		t.Setenv("PATH", originalPath)
	}

	if err := ensureGitRemote("/tmp/repo", "", "https://example.com/repo.git"); err == nil {
		t.Fatal("expected ensureGitRemote validation error")
	}
	if err := checkoutBranch("/tmp/repo", " "); err == nil {
		t.Fatal("expected checkoutBranch validation error")
	}
	if err := pullRemoteBranch("/tmp/repo", "", "main"); err == nil {
		t.Fatal("expected pullRemoteBranch validation error")
	}
	if err := pushRemoteBranch("/tmp/repo", "origin", " "); err == nil {
		t.Fatal("expected pushRemoteBranch validation error")
	}
	if err := rebaseRemoteBranch("/tmp/repo", "", "main"); err == nil {
		t.Fatal("expected rebaseRemoteBranch validation error")
	}

	if isNonFastForwardPushError(nil) {
		t.Fatalf("expected nil error not to be classified as non-fast-forward")
	}
	if !isNonFastForwardPushError(errors.New("rejected non-fast-forward")) {
		t.Fatalf("expected non-fast-forward classification to be true")
	}
	if !isNonFastForwardPushError(errors.New("fetch first")) {
		t.Fatalf("expected fetch-first classification to be true")
	}
	if !isNonFastForwardPushError(errors.New("rejected")) {
		t.Fatalf("expected rejected classification to be true")
	}
	if isNonFastForwardPushError(errors.New("network timeout")) {
		t.Fatalf("expected unrelated error not to be classified as non-fast-forward")
	}
}

func TestRebaseRemoteBranchMissingRefAndWriteFileAtomic(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}

	repoRoot := t.TempDir()
	remoteBare := filepath.Join(t.TempDir(), "remote.git")

	if out, err := exec.Command("git", "init", "--bare", remoteBare).CombinedOutput(); err != nil {
		t.Fatalf("init bare repo failed err=%v out=%s", err, out)
	}
	if out, err := exec.Command("git", "init", "-b", "main", repoRoot).CombinedOutput(); err != nil {
		t.Fatalf("init work repo failed err=%v out=%s", err, out)
	}
	if out, err := exec.Command("git", "-C", repoRoot, "remote", "add", "origin", remoteBare).CombinedOutput(); err != nil {
		t.Fatalf("add remote failed err=%v out=%s", err, out)
	}
	if err := rebaseRemoteBranch(repoRoot, "origin", "main"); err != nil {
		t.Fatalf("expected missing remote ref to be treated as non-fatal, got %v", err)
	}

	path := filepath.Join(t.TempDir(), "state.json")
	if err := writeFileAtomic(path, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatalf("writeFileAtomic success path error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}

	badPath := filepath.Join(t.TempDir(), "missing", "state.json")
	if err := writeFileAtomic(badPath, []byte(`{"ok":true}`), 0o600); err == nil {
		t.Fatal("expected writeFileAtomic to fail when target dir is missing")
	}
}

func TestEnsureGitAvailableAndRepoBranches(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}

	if err := ensureGitAvailable(); err != nil {
		t.Fatalf("expected ensureGitAvailable success with git installed: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", "")
	if err := ensureGitAvailable(); err == nil {
		t.Fatal("expected ensureGitAvailable failure without PATH tools")
	}
	t.Setenv("PATH", originalPath)

	if err := ensureGitRepo(""); err == nil {
		t.Fatal("expected ensureGitRepo empty-root validation error")
	}

	repoRoot := t.TempDir()
	if err := ensureGitRepo(repoRoot); err != nil {
		t.Fatalf("ensureGitRepo init failed: %v", err)
	}
}

func TestGitHeadAndLoadLatestEmptyRepoBranches(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}

	repoRoot := t.TempDir()
	t.Setenv("CARRIER_PROFILESYNC_REPO", repoRoot)
	if err := ensureGitRepo(repoRoot); err != nil {
		t.Fatalf("ensureGitRepo failed: %v", err)
	}

	head, err := gitHead(repoRoot)
	if err != nil {
		t.Fatalf("gitHead returned unexpected error: %v", err)
	}
	if strings.TrimSpace(head) != "" {
		t.Fatalf("expected empty head in fresh repo, got %q", head)
	}

	latest, latestHead, err := LoadLatestInstanceProfile("instance-a")
	if err != nil {
		t.Fatalf("LoadLatestInstanceProfile failed: %v", err)
	}
	if len(latest) != 0 || strings.TrimSpace(latestHead) != "" {
		t.Fatalf("expected empty latest profile/head for fresh repo, profile=%v head=%q", latest, latestHead)
	}

	if _, err := LoadInstanceProfileAtCommit("instance-a", " "); err == nil {
		t.Fatal("expected LoadInstanceProfileAtCommit to require commit hash")
	}
	if _, _, err := RollbackInstanceProfile("instance-a", "host-a", "main", " "); err == nil {
		t.Fatal("expected RollbackInstanceProfile to require target commit")
	}
}

func TestRemoteBranchHelpersHappyPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}

	repoRoot := t.TempDir()
	remoteBare := filepath.Join(t.TempDir(), "remote.git")
	t.Setenv("CARRIER_PROFILESYNC_REPO", repoRoot)

	if _, err := exec.Command("git", "init", "--bare", remoteBare).CombinedOutput(); err != nil {
		t.Fatalf("init bare remote failed: %v", err)
	}
	if err := ensureGitRepo(repoRoot); err != nil {
		t.Fatalf("ensureGitRepo failed: %v", err)
	}

	if _, changed, err := SaveInstanceProfile("host-a_main", "host-a", "main", map[string]interface{}{"k": "v"}, "seed"); err != nil || !changed {
		t.Fatalf("seed SaveInstanceProfile failed changed=%v err=%v", changed, err)
	}

	if err := ensureGitRemote(repoRoot, "origin", remoteBare); err != nil {
		t.Fatalf("ensureGitRemote add failed: %v", err)
	}
	if err := ensureGitRemote(repoRoot, "origin", remoteBare); err != nil {
		t.Fatalf("ensureGitRemote set-url failed: %v", err)
	}

	if err := checkoutBranch(repoRoot, "main"); err != nil {
		t.Fatalf("checkoutBranch failed: %v", err)
	}
	if err := pushRemoteBranch(repoRoot, "origin", "main"); err != nil {
		t.Fatalf("pushRemoteBranch failed: %v", err)
	}
	if err := pullRemoteBranch(repoRoot, "origin", "main"); err != nil {
		t.Fatalf("pullRemoteBranch failed: %v", err)
	}
	if err := pullRemoteBranch(repoRoot, "origin", "missing-branch-for-tests"); err != nil {
		t.Fatalf("pullRemoteBranch should tolerate missing remote branch: %v", err)
	}
}
