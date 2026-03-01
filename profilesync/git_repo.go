package profilesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultProfilesyncRepoDirName = "profiles-repo"

const gitInstallTimeout = 10 * time.Minute

func profilesyncRepoRoot() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("CARRIER_PROFILESYNC_REPO")); custom != "" {
		return custom, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir for profilesync repo: %w", err)
	}
	return filepath.Join(home, ".carrier", defaultProfilesyncRepoDirName), nil
}

func SaveInstanceProfile(instanceID, hostID, agentID string, profile map[string]interface{}, reason string) (string, bool, error) {
	repoRoot, err := profilesyncRepoRoot()
	if err != nil {
		return "", false, err
	}
	if err := ensureGitRepo(repoRoot); err != nil {
		return "", false, err
	}
	normalizedProfile := deepCopyMap(profile)
	instancePath := filepath.Join(repoRoot, "instances", sanitizeInstanceID(instanceID), "openclaw.json")
	if err := os.MkdirAll(filepath.Dir(instancePath), 0o700); err != nil {
		return "", false, fmt.Errorf("create instance profile dir: %w", err)
	}
	raw, err := json.MarshalIndent(normalizedProfile, "", "  ")
	if err != nil {
		return "", false, fmt.Errorf("marshal profile: %w", err)
	}
	if err := os.WriteFile(instancePath, append(raw, '\n'), 0o600); err != nil {
		return "", false, fmt.Errorf("write profile file: %w", err)
	}

	metadataPath := filepath.Join(repoRoot, "instances", sanitizeInstanceID(instanceID), "metadata.json")
	metadata := map[string]interface{}{
		"instanceId": instanceID,
		"hostId":     hostID,
		"agentId":    agentID,
	}
	metadataRaw, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return "", false, fmt.Errorf("marshal profile metadata: %w", err)
	}
	if err := os.WriteFile(metadataPath, append(metadataRaw, '\n'), 0o600); err != nil {
		return "", false, fmt.Errorf("write profile metadata: %w", err)
	}

	relativeProfilePath := filepath.ToSlash(filepath.Join("instances", sanitizeInstanceID(instanceID), "openclaw.json"))
	relativeMetadataPath := filepath.ToSlash(filepath.Join("instances", sanitizeInstanceID(instanceID), "metadata.json"))
	if _, err := runGit(repoRoot, "add", "--", relativeProfilePath, relativeMetadataPath); err != nil {
		return "", false, err
	}
	changed, err := gitHasStagedChanges(repoRoot)
	if err != nil {
		return "", false, err
	}
	if !changed {
		head, err := gitHead(repoRoot)
		return head, false, err
	}
	message := fmt.Sprintf("profile(%s): %s @ %s", normalizeCommitReason(reason), strings.TrimSpace(instanceID), strings.TrimSpace(hostID))
	if _, err := runGit(repoRoot, "commit", "-m", message); err != nil {
		return "", false, err
	}
	head, err := gitHead(repoRoot)
	if err != nil {
		return "", false, err
	}
	return head, true, nil
}

func LoadInstanceProfileAtCommit(instanceID, commitHash string) (map[string]interface{}, error) {
	repoRoot, err := profilesyncRepoRoot()
	if err != nil {
		return nil, err
	}
	if err := ensureGitRepo(repoRoot); err != nil {
		return nil, err
	}
	commit := strings.TrimSpace(commitHash)
	if commit == "" {
		return nil, errors.New("commit hash is required")
	}
	relativeProfilePath := filepath.ToSlash(filepath.Join("instances", sanitizeInstanceID(instanceID), "openclaw.json"))
	out, err := runGit(repoRoot, "show", fmt.Sprintf("%s:%s", commit, relativeProfilePath))
	if err != nil {
		return nil, err
	}
	return parseProfileJSON(out)
}

func LoadLatestInstanceProfile(instanceID string) (map[string]interface{}, string, error) {
	repoRoot, err := profilesyncRepoRoot()
	if err != nil {
		return nil, "", err
	}
	if err := ensureGitRepo(repoRoot); err != nil {
		return nil, "", err
	}
	head, err := gitHead(repoRoot)
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(head) == "" {
		return map[string]interface{}{}, "", nil
	}
	profile, err := LoadInstanceProfileAtCommit(instanceID, head)
	if err != nil {
		return nil, "", err
	}
	return profile, head, nil
}

func RollbackInstanceProfile(instanceID, hostID, agentID, targetCommit string) (string, map[string]interface{}, error) {
	target := strings.TrimSpace(targetCommit)
	if target == "" {
		return "", nil, errors.New("target commit is required")
	}
	profile, err := LoadInstanceProfileAtCommit(instanceID, target)
	if err != nil {
		return "", nil, err
	}
	commit, _, err := SaveInstanceProfile(instanceID, hostID, agentID, profile, "rollback")
	if err != nil {
		return "", nil, err
	}
	return commit, profile, nil
}

func ensureGitRepo(repoRoot string) error {
	if err := ensureGitAvailable(); err != nil {
		return err
	}
	if strings.TrimSpace(repoRoot) == "" {
		return errors.New("profilesync repo root is empty")
	}
	if err := os.MkdirAll(repoRoot, 0o700); err != nil {
		return fmt.Errorf("create profilesync repo root: %w", err)
	}
	gitDir := filepath.Join(repoRoot, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if _, err := runGit(repoRoot, "init", "-b", "main"); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("stat profilesync repo git dir: %w", err)
		}
	}
	if _, err := runGit(repoRoot, "config", "user.name", "carrier-profilesync"); err != nil {
		return err
	}
	if _, err := runGit(repoRoot, "config", "user.email", "carrier-profilesync@localhost"); err != nil {
		return err
	}
	if err := ensureProfilesRepoGitIgnore(repoRoot); err != nil {
		return err
	}
	return nil
}

func ensureProfilesRepoGitIgnore(repoRoot string) error {
	requiredEntries := []string{
		"# Carrier secrets (never commit)",
		"/credentials.json",
		"/carrier-secrets.json",
		"instances/*/carrier-secrets.json",
		"",
		"# Runtime artifacts",
		"instances/*/logs/",
		"instances/*/.cache/",
		"instances/*/tmp/",
		"",
		"# Defense in depth",
		"*secret*.json",
		"*credential*.json",
		"*.key",
		"*.pem",
	}
	path := filepath.Join(repoRoot, ".gitignore")
	current, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read profilesync .gitignore: %w", err)
	}

	existingLines := make(map[string]bool)
	var lines []string
	if err == nil {
		lines = strings.Split(string(current), "\n")
		for _, l := range lines {
			existingLines[l] = true
		}
	}

	// Append any missing required entries, preserving existing content.
	changed := false
	for _, entry := range requiredEntries {
		if !existingLines[entry] {
			lines = append(lines, entry)
			existingLines[entry] = true
			changed = true
		}
	}

	if !changed {
		return nil
	}

	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := writeFileAtomic(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write profilesync .gitignore: %w", err)
	}
	return nil
}

func ensureGitAvailable() error {
	if err := verifyGitAvailable(); err == nil {
		return nil
	}
	if err := installGit(); err != nil {
		return fmt.Errorf("git is required for profile sync and auto-install failed: %w", err)
	}
	if err := verifyGitAvailable(); err != nil {
		return fmt.Errorf("git remains unavailable after auto-install: %w", err)
	}
	return nil
}

func verifyGitAvailable() error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git executable not found in PATH: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--version")
	raw, err := cmd.CombinedOutput()
	if err != nil {
		out := strings.TrimSpace(string(raw))
		return fmt.Errorf("git --version failed: %v: %s", err, out)
	}
	return nil
}

type gitInstallStrategy struct {
	Name         string
	RequireRoot  bool
	InstallSteps [][]string
}

func installGit() error {
	strategies := gitInstallStrategies()
	if len(strategies) == 0 {
		return fmt.Errorf("unsupported operating system %q for automatic git installation", runtime.GOOS)
	}
	tried := 0
	failures := make([]string, 0, len(strategies))
	for _, strategy := range strategies {
		if len(strategy.InstallSteps) == 0 || len(strategy.InstallSteps[0]) == 0 {
			continue
		}
		manager := strategy.InstallSteps[0][0]
		if !commandExists(manager) {
			continue
		}
		tried++
		if err := runGitInstallStrategy(strategy); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", strategy.Name, err))
			continue
		}
		if err := verifyGitAvailable(); err == nil {
			return nil
		}
		failures = append(failures, fmt.Sprintf("%s: install finished but git still unavailable", strategy.Name))
	}
	if tried == 0 {
		return fmt.Errorf("no supported package manager found to install git on %s", runtime.GOOS)
	}
	return fmt.Errorf("all git installation attempts failed (%s)", strings.Join(failures, "; "))
}

func gitInstallStrategies() []gitInstallStrategy {
	switch runtime.GOOS {
	case "darwin":
		return []gitInstallStrategy{
			{
				Name:         "homebrew",
				InstallSteps: [][]string{{"brew", "install", "git"}},
			},
		}
	case "linux":
		return []gitInstallStrategy{
			{
				Name:         "apt-get",
				RequireRoot:  true,
				InstallSteps: [][]string{{"apt-get", "install", "-y", "git"}},
			},
			{
				Name:         "dnf",
				RequireRoot:  true,
				InstallSteps: [][]string{{"dnf", "install", "-y", "git"}},
			},
			{
				Name:         "yum",
				RequireRoot:  true,
				InstallSteps: [][]string{{"yum", "install", "-y", "git"}},
			},
			{
				Name:         "pacman",
				RequireRoot:  true,
				InstallSteps: [][]string{{"pacman", "-Sy", "--noconfirm", "git"}},
			},
			{
				Name:         "zypper",
				RequireRoot:  true,
				InstallSteps: [][]string{{"zypper", "--non-interactive", "install", "git-core"}},
			},
			{
				Name:         "apk",
				RequireRoot:  true,
				InstallSteps: [][]string{{"apk", "add", "--no-cache", "git"}},
			},
		}
	case "windows":
		return []gitInstallStrategy{
			{
				Name: "winget",
				InstallSteps: [][]string{{
					"winget", "install", "--id", "Git.Git", "-e", "--source", "winget",
					"--accept-package-agreements", "--accept-source-agreements",
				}},
			},
			{
				Name:         "chocolatey",
				InstallSteps: [][]string{{"choco", "install", "git", "-y"}},
			},
		}
	default:
		return nil
	}
}

func runGitInstallStrategy(strategy gitInstallStrategy) error {
	for _, step := range strategy.InstallSteps {
		if len(step) == 0 {
			continue
		}
		command, args, err := applyPrivilege(step[0], step[1:], strategy.RequireRoot)
		if err != nil {
			return err
		}
		if _, err := runCommand(command, args...); err != nil {
			return err
		}
	}
	return nil
}

func applyPrivilege(command string, args []string, requireRoot bool) (string, []string, error) {
	if !requireRoot || runtime.GOOS == "windows" {
		return command, args, nil
	}
	if os.Geteuid() == 0 {
		return command, args, nil
	}
	if !commandExists("sudo") {
		return "", nil, fmt.Errorf("%s requires root privileges but sudo is unavailable", command)
	}
	wrapped := make([]string, 0, len(args)+2)
	wrapped = append(wrapped, "-n", command)
	wrapped = append(wrapped, args...)
	return "sudo", wrapped, nil
}

func runCommand(command string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitInstallTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	if isAptGetCommand(command, args) {
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	}
	raw, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(raw))
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("%s timed out after %s", commandLine(command, args), gitInstallTimeout)
	}
	if err != nil {
		return "", fmt.Errorf("%s failed: %v: %s", commandLine(command, args), err, truncateOutput(out))
	}
	return out, nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func isAptGetCommand(command string, args []string) bool {
	if command == "apt-get" {
		return true
	}
	return command == "sudo" && len(args) >= 2 && args[1] == "apt-get"
}

func commandLine(command string, args []string) string {
	if len(args) == 0 {
		return command
	}
	return command + " " + strings.Join(args, " ")
}

func truncateOutput(out string) string {
	const maxLen = 800
	if len(out) <= maxLen {
		return out
	}
	return out[:maxLen] + "...(truncated)"
}

func runGit(repoRoot string, args ...string) (string, error) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoRoot
	raw, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(raw))
	if err != nil {
		return "", fmt.Errorf("git %s failed: %v: %s", strings.Join(args, " "), err, out)
	}
	return out, nil
}

func gitHead(repoRoot string) (string, error) {
	out, err := runGit(repoRoot, "rev-parse", "HEAD")
	if err != nil {
		if strings.Contains(err.Error(), "unknown revision") || strings.Contains(err.Error(), "Needed a single revision") {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func gitHasStagedChanges(repoRoot string) (bool, error) {
	out, err := runGit(repoRoot, "diff", "--cached", "--name-only")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func parseProfileJSON(raw string) (map[string]interface{}, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return map[string]interface{}{}, nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, fmt.Errorf("parse profile json: %w", err)
	}
	return out, nil
}

func sanitizeInstanceID(instanceID string) string {
	trimmed := strings.TrimSpace(instanceID)
	if trimmed == "" {
		return "default"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "..", "_")
	return replacer.Replace(trimmed)
}

func normalizeCommitReason(reason string) string {
	trimmed := strings.TrimSpace(strings.ToLower(reason))
	if trimmed == "" {
		return "sync"
	}
	return trimmed
}

func SyncInstanceMemoryContract(instanceID string, contract map[string]interface{}, repoURL, branch, reason string) (string, bool, error) {
	repoRoot, err := profilesyncRepoRoot()
	if err != nil {
		return "", false, err
	}
	if err := ensureGitRepo(repoRoot); err != nil {
		return "", false, err
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	repoURL = strings.TrimSpace(repoURL)
	if repoURL != "" {
		if err := ensureGitRemote(repoRoot, "origin", repoURL); err != nil {
			return "", false, err
		}
		if err := checkoutBranch(repoRoot, branch); err != nil {
			return "", false, err
		}
		if err := pullRemoteBranch(repoRoot, "origin", branch); err != nil {
			return "", false, err
		}
	}

	normalized := deepCopyMap(contract)
	memoryPath := filepath.Join(repoRoot, "instances", sanitizeInstanceID(instanceID), "memory-contract.json")
	if err := os.MkdirAll(filepath.Dir(memoryPath), 0o700); err != nil {
		return "", false, fmt.Errorf("create memory contract dir: %w", err)
	}
	raw, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return "", false, fmt.Errorf("marshal memory contract: %w", err)
	}
	if err := writeFileAtomic(memoryPath, append(raw, '\n'), 0o600); err != nil {
		return "", false, fmt.Errorf("write memory contract file: %w", err)
	}

	relativePath := filepath.ToSlash(filepath.Join("instances", sanitizeInstanceID(instanceID), "memory-contract.json"))
	if _, err := runGit(repoRoot, "add", "--", relativePath); err != nil {
		return "", false, err
	}
	changed, err := gitHasStagedChanges(repoRoot)
	if err != nil {
		return "", false, err
	}
	if !changed {
		head, err := gitHead(repoRoot)
		return head, false, err
	}

	message := fmt.Sprintf("memory(%s): %s", normalizeCommitReason(reason), strings.TrimSpace(instanceID))
	if _, err := runGit(repoRoot, "commit", "-m", message); err != nil {
		return "", false, err
	}
	head, err := gitHead(repoRoot)
	if err != nil {
		return "", false, err
	}
	if repoURL != "" {
		if err := checkoutBranch(repoRoot, branch); err != nil {
			return "", false, err
		}
		if err := pushRemoteBranch(repoRoot, "origin", branch); err != nil {
			return "", false, err
		}
	}
	return head, true, nil
}

func ensureGitRemote(repoRoot, remoteName, remoteURL string) error {
	remoteName = strings.TrimSpace(remoteName)
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteName == "" || remoteURL == "" {
		return errors.New("remote name and remote url are required")
	}
	if _, err := runGit(repoRoot, "remote", "get-url", remoteName); err != nil {
		if _, addErr := runGit(repoRoot, "remote", "add", remoteName, remoteURL); addErr != nil {
			return addErr
		}
		return nil
	}
	_, err := runGit(repoRoot, "remote", "set-url", remoteName, remoteURL)
	return err
}

func checkoutBranch(repoRoot, branch string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return errors.New("branch is required")
	}
	_, err := runGit(repoRoot, "checkout", "-B", branch)
	return err
}

func pullRemoteBranch(repoRoot, remoteName, branch string) error {
	remoteName = strings.TrimSpace(remoteName)
	branch = strings.TrimSpace(branch)
	if remoteName == "" || branch == "" {
		return errors.New("remote and branch are required")
	}
	if _, err := runGit(repoRoot, "fetch", remoteName, branch); err != nil {
		// Brand-new remotes may not have the target branch yet.
		errText := strings.ToLower(err.Error())
		if strings.Contains(errText, "couldn't find remote ref") || strings.Contains(errText, "remote ref does not exist") {
			return nil
		}
		return err
	}
	if _, err := runGit(repoRoot, "merge", "--ff-only", "FETCH_HEAD"); err != nil {
		errText := strings.ToLower(err.Error())
		if strings.Contains(errText, "not possible to fast-forward") || strings.Contains(errText, "unrelated histories") {
			return err
		}
	}
	return nil
}

func pushRemoteBranch(repoRoot, remoteName, branch string) error {
	remoteName = strings.TrimSpace(remoteName)
	branch = strings.TrimSpace(branch)
	if remoteName == "" || branch == "" {
		return errors.New("remote and branch are required")
	}
	if _, err := runGit(repoRoot, "push", remoteName, branch); err != nil {
		if !isNonFastForwardPushError(err) {
			return err
		}
		// Another writer updated remote branch after our last pull; rebase and retry once.
		if err := rebaseRemoteBranch(repoRoot, remoteName, branch); err != nil {
			return err
		}
		if _, err := runGit(repoRoot, "push", remoteName, branch); err != nil {
			return err
		}
	}
	return nil
}

func rebaseRemoteBranch(repoRoot, remoteName, branch string) error {
	remoteName = strings.TrimSpace(remoteName)
	branch = strings.TrimSpace(branch)
	if remoteName == "" || branch == "" {
		return errors.New("remote and branch are required")
	}
	if _, err := runGit(repoRoot, "fetch", remoteName, branch); err != nil {
		errText := strings.ToLower(err.Error())
		if strings.Contains(errText, "couldn't find remote ref") || strings.Contains(errText, "remote ref does not exist") {
			return nil
		}
		return err
	}
	if _, err := runGit(repoRoot, "rebase", "FETCH_HEAD"); err != nil {
		_, _ = runGit(repoRoot, "rebase", "--abort")
		return err
	}
	return nil
}

func isNonFastForwardPushError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "non-fast-forward") ||
		strings.Contains(msg, "fetch first") ||
		strings.Contains(msg, "rejected")
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmpPath := filepath.Join(dir, "."+filepath.Base(path)+".tmp")
	if err := os.WriteFile(tmpPath, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
