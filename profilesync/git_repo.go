package profilesync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultProfilesyncRepoDirName = "profiles-repo"

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
	return ensureProfilesRepoGitIgnore(repoRoot)
}

func ensureProfilesRepoGitIgnore(repoRoot string) error {
	requiredLines := []string{
		"/credentials.json",
		"/carrier-secrets.json",
		"instances/*/carrier-secrets.json",
		"instances/*/logs/",
		"instances/*/.cache/",
		"instances/*/tmp/",
		"*secret*.json",
		"*credential*.json",
		"*.key",
		"*.pem",
	}

	const carrierBlock = `# Carrier secrets (never commit)
/credentials.json
/carrier-secrets.json
instances/*/carrier-secrets.json

# Runtime artifacts
instances/*/logs/
instances/*/.cache/
instances/*/tmp/

# Defense in depth
*secret*.json
*credential*.json
*.key
*.pem`

	path := filepath.Join(repoRoot, ".gitignore")
	current, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read profilesync .gitignore: %w", err)
	}

	existingContent := ""
	if err == nil {
		existingContent = string(current)
	}

	existingLines := make(map[string]bool)
	for _, line := range strings.Split(existingContent, "\n") {
		existingLines[strings.TrimSpace(line)] = true
	}

	var missingLines []string
	for _, required := range requiredLines {
		if !existingLines[required] {
			missingLines = append(missingLines, required)
		}
	}
	if len(missingLines) == 0 {
		return nil
	}

	content := strings.TrimSpace(existingContent)
	if content != "" {
		content += "\n\n"
	}
	if len(missingLines) < len(requiredLines) {
		content += strings.Join(missingLines, "\n")
	} else {
		content += carrierBlock
	}
	content += "\n"

	if err := writeFileAtomic(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write profilesync .gitignore: %w", err)
	}
	return nil
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
