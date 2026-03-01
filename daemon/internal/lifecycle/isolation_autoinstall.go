package lifecycle

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type PackageManager struct {
	Name       string
	InstallCmd []string
	UpdateCmd  []string
}

type BwrapAutoInstallResult struct {
	Installed      bool
	PackageManager string
	UsedSudo       bool
	SudoOutput     string
	DirectOutput   string
	VerifyPath     string
}

var ErrAutoInstallFailed = errors.New("bwrap auto-install failed")

var (
	autoInstallLookPath = exec.LookPath
	autoInstallExecCmd  = func(name string, args ...string) *exec.Cmd {
		return exec.Command(name, args...)
	}
)

func attemptAutoInstallBwrap() (*BwrapAutoInstallResult, error) {
	pm, err := detectPackageManager()
	if err != nil {
		return nil, err
	}
	result := &BwrapAutoInstallResult{PackageManager: pm.Name}
	hasSudo, passwordless := detectSudoAvailability()
	sudoTried := false

	if len(pm.UpdateCmd) > 0 {
		if hasSudo && passwordless {
			sudoTried = true
			updateOutput, updateErr := runInstallCommand("sudo", append([]string{"-n"}, pm.UpdateCmd...)...)
			result.SudoOutput = strings.TrimSpace(updateOutput)
			if updateErr != nil {
				directOutput, directErr := runInstallCommand(pm.UpdateCmd[0], pm.UpdateCmd[1:]...)
				result.DirectOutput = strings.TrimSpace(directOutput)
				if directErr != nil {
					return result, fmt.Errorf("%w: failed running update for %s (sudo: %s; direct: %s)", ErrAutoInstallFailed, pm.Name, result.SudoOutput, result.DirectOutput)
				}
			}
		} else {
			directOutput, directErr := runInstallCommand(pm.UpdateCmd[0], pm.UpdateCmd[1:]...)
			result.DirectOutput = strings.TrimSpace(directOutput)
			if directErr != nil {
				return result, fmt.Errorf("%w: failed running update for %s (%s)", ErrAutoInstallFailed, pm.Name, result.DirectOutput)
			}
		}
	}

	if hasSudo && passwordless {
		sudoTried = true
		output, sudoErr := runInstallCommand("sudo", append([]string{"-n"}, pm.InstallCmd...)...)
		result.SudoOutput = strings.TrimSpace(joinOutput(result.SudoOutput, output))
		if sudoErr == nil {
			result.UsedSudo = true
		} else {
			output, directErr := runInstallCommand(pm.InstallCmd[0], pm.InstallCmd[1:]...)
			result.DirectOutput = strings.TrimSpace(joinOutput(result.DirectOutput, output))
			if directErr != nil {
				return result, fmt.Errorf("%w: %s (sudo: %s; direct: %s)", ErrAutoInstallFailed, pm.Name, result.SudoOutput, result.DirectOutput)
			}
		}
	} else {
		output, directErr := runInstallCommand(pm.InstallCmd[0], pm.InstallCmd[1:]...)
		result.DirectOutput = strings.TrimSpace(joinOutput(result.DirectOutput, output))
		if directErr != nil {
			if sudoTried {
				return result, fmt.Errorf("%w: %s (sudo: %s; direct: %s)", ErrAutoInstallFailed, pm.Name, result.SudoOutput, result.DirectOutput)
			}
			return result, fmt.Errorf("%w: %s (%s)", ErrAutoInstallFailed, pm.Name, result.DirectOutput)
		}
	}

	bwrapPath, lookupErr := autoInstallLookPath("bwrap")
	if lookupErr != nil || strings.TrimSpace(bwrapPath) == "" {
		return result, fmt.Errorf("%w: bubblewrap not found after install attempt (pkg_manager=%s)", ErrAutoInstallFailed, pm.Name)
	}

	result.Installed = true
	result.VerifyPath = strings.TrimSpace(bwrapPath)
	return result, nil
}

func detectPackageManager() (*PackageManager, error) {
	candidates := []PackageManager{
		{Name: "apt-get", InstallCmd: []string{"apt-get", "install", "-y", "bubblewrap"}, UpdateCmd: []string{"apt-get", "update"}},
		{Name: "dnf", InstallCmd: []string{"dnf", "install", "-y", "bubblewrap"}},
		{Name: "yum", InstallCmd: []string{"yum", "install", "-y", "bubblewrap"}},
		{Name: "pacman", InstallCmd: []string{"pacman", "-Sy", "--noconfirm", "bubblewrap"}},
		{Name: "apk", InstallCmd: []string{"apk", "add", "bubblewrap"}},
		{Name: "zypper", InstallCmd: []string{"zypper", "--non-interactive", "install", "bubblewrap"}},
	}
	for _, candidate := range candidates {
		if _, err := autoInstallLookPath(candidate.Name); err == nil {
			pm := candidate
			return &pm, nil
		}
	}
	return nil, fmt.Errorf("%w: no supported package manager found", ErrAutoInstallFailed)
}

func detectSudoAvailability() (hasSudo bool, passwordless bool) {
	if _, err := autoInstallLookPath("sudo"); err != nil {
		return false, false
	}
	cmd := autoInstallExecCmd("sudo", "-n", "true")
	if err := cmd.Run(); err != nil {
		return true, false
	}
	return true, true
}

func runInstallCommand(name string, args ...string) (string, error) {
	cmd := autoInstallExecCmd(name, args...)
	raw, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(raw)), err
}

func joinOutput(existing, latest string) string {
	existing = strings.TrimSpace(existing)
	latest = strings.TrimSpace(latest)
	if existing == "" {
		return latest
	}
	if latest == "" {
		return existing
	}
	return existing + "\n" + latest
}
