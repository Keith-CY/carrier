package profilesync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runGit(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
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
