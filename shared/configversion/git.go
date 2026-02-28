package configversion

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	initCommitMessage = "carrier: init config versioning"
)

var gitIgnoreContent = strings.Join([]string{
	"credentials.json",
	"carrier-secrets.json",
	"*.key",
	"*.pem",
	"tls/",
	"",
}, "\n")

// InitRepo ensures ~/.carrier/ is a git repo.
// If already initialized, this is a no-op.
func InitRepo(carrierDir string) error {
	carrierDir = strings.TrimSpace(carrierDir)
	if carrierDir == "" {
		return errors.New("carrier directory is empty")
	}
	if !gitAvailable() {
		return nil
	}
	if isGitRepo(carrierDir) {
		return nil
	}
	if err := os.MkdirAll(carrierDir, 0o700); err != nil {
		return fmt.Errorf("create carrier dir: %w", err)
	}
	if _, err := runGit("", "init", carrierDir); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	if err := os.WriteFile(filepath.Join(carrierDir, ".gitignore"), []byte(gitIgnoreContent), 0o600); err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}
	if err := ensureGitIdentity(carrierDir); err != nil {
		return fmt.Errorf("configure git identity: %w", err)
	}
	if _, err := runGit(carrierDir, "add", ".gitignore"); err != nil {
		return fmt.Errorf("git add .gitignore: %w", err)
	}
	if _, err := runGit(carrierDir, "commit", "-m", initCommitMessage); err != nil {
		return fmt.Errorf("git commit init: %w", err)
	}
	return nil
}

// CommitChange stages all changes and commits with a descriptive message.
// This is best-effort: if git is not available or not initialized, it silently returns.
func CommitChange(carrierDir string, message string) {
	carrierDir = strings.TrimSpace(carrierDir)
	if carrierDir == "" || !gitAvailable() || !isGitRepo(carrierDir) {
		return
	}
	if err := ensureGitIdentity(carrierDir); err != nil {
		log.Printf("[configversion] ensure git identity: %v", err)
	}
	if _, err := runGit(carrierDir, "add", "-A"); err != nil {
		log.Printf("[configversion] git add -A: %v", err)
		return
	}
	if _, err := runGit(carrierDir, "diff", "--cached", "--quiet"); err == nil {
		return
	} else {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			log.Printf("[configversion] git diff --cached --quiet: %v", err)
			return
		}
	}
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		trimmed = "update config"
	}
	if _, err := runGit(carrierDir, "commit", "-m", "carrier: "+trimmed); err != nil {
		log.Printf("[configversion] git commit: %v", err)
	}
}

func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func isGitRepo(carrierDir string) bool {
	info, err := os.Stat(filepath.Join(carrierDir, ".git"))
	return err == nil && info.IsDir()
}

func ensureGitIdentity(carrierDir string) error {
	name, _ := runGit(carrierDir, "config", "--get", "user.name")
	if strings.TrimSpace(name) == "" {
		if _, err := runGit(carrierDir, "config", "user.name", "carrier"); err != nil {
			return err
		}
	}
	email, _ := runGit(carrierDir, "config", "--get", "user.email")
	if strings.TrimSpace(email) == "" {
		if _, err := runGit(carrierDir, "config", "user.email", "carrier@localhost"); err != nil {
			return err
		}
	}
	return nil
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	output := strings.TrimSpace(stdout.String())
	if err == nil {
		return output, nil
	}
	if serr := strings.TrimSpace(stderr.String()); serr != "" {
		return output, fmt.Errorf("%w: %s", err, serr)
	}
	if output != "" {
		return output, fmt.Errorf("%w: %s", err, output)
	}
	return output, err
}
