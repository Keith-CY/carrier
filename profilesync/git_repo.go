package profilesync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultProfilesyncRepoDirName = "profiles-repo"
	fallbackRepoMarkerFileName    = ".profilesync-fallback"
)

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
	if isFallbackRepoEnabled(repoRoot) {
		return saveInstanceProfileFallback(repoRoot, instanceID, hostID, agentID, profile, reason)
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
	if isFallbackRepoEnabled(repoRoot) {
		return loadInstanceProfileAtCommitFallback(repoRoot, instanceID, commitHash)
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
	if isFallbackRepoEnabled(repoRoot) {
		return loadLatestInstanceProfileFallback(repoRoot, instanceID)
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
	if strings.TrimSpace(repoRoot) == "" {
		return errors.New("profilesync repo root is empty")
	}
	if err := os.MkdirAll(repoRoot, 0o700); err != nil {
		return fmt.Errorf("create profilesync repo root: %w", err)
	}
	gitDir := filepath.Join(repoRoot, ".git")
	markerPath := filepath.Join(repoRoot, fallbackRepoMarkerFileName)
	if _, err := os.Stat(markerPath); err == nil {
		return nil
	}
	if _, err := os.Stat(gitDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if _, err := runGit(repoRoot, "init", "-b", "main"); err != nil {
				if shouldEnableFallbackRepo(err) {
					return enableFallbackRepo(repoRoot)
				}
				return err
			}
		} else {
			return fmt.Errorf("stat profilesync repo git dir: %w", err)
		}
	}
	if _, err := runGit(repoRoot, "config", "user.name", "carrier-profilesync"); err != nil {
		if shouldEnableFallbackRepo(err) {
			return enableFallbackRepo(repoRoot)
		}
		return err
	}
	if _, err := runGit(repoRoot, "config", "user.email", "carrier-profilesync@localhost"); err != nil {
		if shouldEnableFallbackRepo(err) {
			return enableFallbackRepo(repoRoot)
		}
		return err
	}
	return nil
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

func shouldEnableFallbackRepo(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "xcode license") ||
		strings.Contains(text, "executable file not found") ||
		strings.Contains(text, "no such file or directory")
}

func enableFallbackRepo(repoRoot string) error {
	markerPath := filepath.Join(repoRoot, fallbackRepoMarkerFileName)
	return os.WriteFile(markerPath, []byte("fallback"), 0o600)
}

func isFallbackRepoEnabled(repoRoot string) bool {
	markerPath := filepath.Join(repoRoot, fallbackRepoMarkerFileName)
	_, err := os.Stat(markerPath)
	return err == nil
}

func saveInstanceProfileFallback(repoRoot, instanceID, hostID, agentID string, profile map[string]interface{}, reason string) (string, bool, error) {
	instanceDir := filepath.Join(repoRoot, "instances", sanitizeInstanceID(instanceID))
	historyDir := filepath.Join(instanceDir, "history")
	if err := os.MkdirAll(historyDir, 0o700); err != nil {
		return "", false, fmt.Errorf("create fallback history dir: %w", err)
	}
	currentPath := filepath.Join(instanceDir, "openclaw.json")
	normalizedProfile := deepCopyMap(profile)
	raw, err := json.MarshalIndent(normalizedProfile, "", "  ")
	if err != nil {
		return "", false, fmt.Errorf("marshal profile: %w", err)
	}
	raw = append(raw, '\n')
	existingRaw, _ := os.ReadFile(currentPath)
	if strings.TrimSpace(string(existingRaw)) == strings.TrimSpace(string(raw)) {
		head, _ := os.ReadFile(filepath.Join(instanceDir, "HEAD"))
		return strings.TrimSpace(string(head)), false, nil
	}
	if err := os.WriteFile(currentPath, raw, 0o600); err != nil {
		return "", false, fmt.Errorf("write fallback current profile: %w", err)
	}

	metadataPath := filepath.Join(instanceDir, "metadata.json")
	metadata := map[string]interface{}{
		"instanceId": instanceID,
		"hostId":     hostID,
		"agentId":    agentID,
		"reason":     normalizeCommitReason(reason),
	}
	metadataRaw, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return "", false, fmt.Errorf("marshal fallback metadata: %w", err)
	}
	if err := os.WriteFile(metadataPath, append(metadataRaw, '\n'), 0o600); err != nil {
		return "", false, fmt.Errorf("write fallback metadata: %w", err)
	}

	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%s", time.Now().UTC().Format(time.RFC3339Nano), instanceID, hostID, agentID, string(raw))))
	commit := hex.EncodeToString(sum[:16])
	historyPath := filepath.Join(historyDir, commit+".json")
	if err := os.WriteFile(historyPath, raw, 0o600); err != nil {
		return "", false, fmt.Errorf("write fallback history profile: %w", err)
	}
	if err := os.WriteFile(filepath.Join(instanceDir, "HEAD"), []byte(commit+"\n"), 0o600); err != nil {
		return "", false, fmt.Errorf("write fallback head: %w", err)
	}
	return commit, true, nil
}

func loadInstanceProfileAtCommitFallback(repoRoot, instanceID, commitHash string) (map[string]interface{}, error) {
	commit := strings.TrimSpace(commitHash)
	if commit == "" {
		return nil, errors.New("commit hash is required")
	}
	historyPath := filepath.Join(repoRoot, "instances", sanitizeInstanceID(instanceID), "history", commit+".json")
	raw, err := os.ReadFile(historyPath)
	if err != nil {
		return nil, fmt.Errorf("read fallback history profile: %w", err)
	}
	return parseProfileJSON(string(raw))
}

func loadLatestInstanceProfileFallback(repoRoot, instanceID string) (map[string]interface{}, string, error) {
	instanceDir := filepath.Join(repoRoot, "instances", sanitizeInstanceID(instanceID))
	headRaw, err := os.ReadFile(filepath.Join(instanceDir, "HEAD"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]interface{}{}, "", nil
		}
		return nil, "", fmt.Errorf("read fallback head: %w", err)
	}
	head := strings.TrimSpace(string(headRaw))
	if head == "" {
		return map[string]interface{}{}, "", nil
	}
	profile, err := loadInstanceProfileAtCommitFallback(repoRoot, instanceID, head)
	if err != nil {
		return nil, "", err
	}
	return profile, head, nil
}
