package profilesync

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const gitInstallTimeout = 10 * time.Minute

var runtimeGOOS = runtime.GOOS

var (
	verifyGitAvailableFn    = verifyGitAvailable
	installGitFn            = installGit
	gitInstallStrategiesFn  = gitInstallStrategies
	commandExistsFn         = commandExists
	runGitInstallStrategyFn = runGitInstallStrategy
	osGeteuidFn             = os.Geteuid
)

func ensureGitAvailable() error {
	if err := verifyGitAvailableFn(); err == nil {
		return nil
	}
	if err := installGitFn(); err != nil {
		return fmt.Errorf("git is required for profile sync and auto-install failed: %w", err)
	}
	if err := verifyGitAvailableFn(); err != nil {
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
		return fmt.Errorf("git --version failed: %v: %s", err, strings.TrimSpace(string(raw)))
	}
	return nil
}

type gitInstallStrategy struct {
	Name         string
	RequireRoot  bool
	InstallSteps [][]string
}

func installGit() error {
	strategies := gitInstallStrategiesFn()
	if len(strategies) == 0 {
		return fmt.Errorf("unsupported operating system %q for automatic git installation", runtimeGOOS)
	}
	tried := 0
	failures := make([]string, 0, len(strategies))
	for _, strategy := range strategies {
		if len(strategy.InstallSteps) == 0 || len(strategy.InstallSteps[0]) == 0 {
			continue
		}
		manager := strategy.InstallSteps[0][0]
		if !commandExistsFn(manager) {
			continue
		}
		tried++
		if err := runGitInstallStrategyFn(strategy); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", strategy.Name, err))
			continue
		}
		if err := verifyGitAvailableFn(); err == nil {
			return nil
		}
		failures = append(failures, fmt.Sprintf("%s: install finished but git still unavailable", strategy.Name))
	}
	if tried == 0 {
		return fmt.Errorf("no supported package manager found to install git on %s", runtimeGOOS)
	}
	return fmt.Errorf("all git installation attempts failed (%s)", strings.Join(failures, "; "))
}

func gitInstallStrategies() []gitInstallStrategy {
	switch runtimeGOOS {
	case "darwin":
		return []gitInstallStrategy{{
			Name:         "homebrew",
			InstallSteps: [][]string{{"brew", "install", "git"}},
		}}
	case "linux":
		return []gitInstallStrategy{
			{Name: "apt-get", RequireRoot: true, InstallSteps: [][]string{{"apt-get", "install", "-y", "git"}}},
			{Name: "dnf", RequireRoot: true, InstallSteps: [][]string{{"dnf", "install", "-y", "git"}}},
			{Name: "yum", RequireRoot: true, InstallSteps: [][]string{{"yum", "install", "-y", "git"}}},
			{Name: "pacman", RequireRoot: true, InstallSteps: [][]string{{"pacman", "-Sy", "--noconfirm", "git"}}},
			{Name: "zypper", RequireRoot: true, InstallSteps: [][]string{{"zypper", "--non-interactive", "install", "git-core"}}},
			{Name: "apk", RequireRoot: true, InstallSteps: [][]string{{"apk", "add", "--no-cache", "git"}}},
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
			{Name: "chocolatey", InstallSteps: [][]string{{"choco", "install", "git", "-y"}}},
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
	if !requireRoot || runtimeGOOS == "windows" {
		return command, args, nil
	}
	if osGeteuidFn() == 0 {
		return command, args, nil
	}
	if !commandExistsFn("sudo") {
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
