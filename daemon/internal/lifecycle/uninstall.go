package lifecycle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Uninstall stops a running agent (if necessary) and removes its installed artifacts,
// resetting its state to not_installed/stopped.
func (s *Service) Uninstall(ctx context.Context, agentID string) error {
	_, state, err := s.getManifestAndState(agentID)
	if err != nil {
		return err
	}

	// If the agent is running, stop it first.
	if state.Runtime == RuntimeStateRunning || state.Runtime == RuntimeStateStarting {
		if stopErr := s.Stop(ctx, agentID); stopErr != nil {
			s.recordAudit("", "system", "uninstall", agentID, AuditResultFailure, "E_STOP_FAILED", stopErr.Error())
			return fmt.Errorf("failed to stop agent before uninstall: %w", stopErr)
		}
	}
	if cleanupErr := s.cleanupIsolationVM(agentID); cleanupErr != nil {
		s.appendLog(agentID, fmt.Sprintf("isolation cleanup failed (best-effort): %v", cleanupErr))
	}

	s.mu.Lock()
	state = s.states[agentID]
	state.Install = InstallStateNotInstalled
	state.Runtime = RuntimeStateStopped
	state.Health = HealthStateUnknown
	state.LastError = ""
	state.LastTriageSummary = ""
	state.NeedsRemoteDiagnosis = false
	state.Ports = []int{}
	state.RestartCount = 0
	state.StartedAt = nil
	state.UpdatedAt = s.now()
	s.states[agentID] = state

	// Reset crash-loop state
	delete(s.restarts, agentID)
	delete(s.cooldowns, agentID)
	s.backoffStates[agentID] = BackoffState{}
	s.mu.Unlock()

	s.appendLog(agentID, "agent uninstalled")
	s.recordAudit("", "system", "uninstall", agentID, AuditResultSuccess, "", "uninstall completed")
	s.saveState()

	return nil
}

var (
	isolationCleanupRunCommand = func(name string, args ...string) error {
		return exec.Command(name, args...).Run()
	}
	isolationCleanupRemoveFile = os.Remove
)

func (s *Service) cleanupIsolationVM(agentID string) error {
	hostGOOS := strings.ToLower(strings.TrimSpace(isolationRuntimeGOOS))
	instance := strings.TrimSpace(isolationEnvLookup(defaultLimaInstanceEnvKey))
	if instance == "" {
		instance = defaultLimaInstanceName
	}

	var errorsSeen []string
	switch hostGOOS {
	case "darwin":
		s.appendLog(agentID, fmt.Sprintf("isolation cleanup: stopping Lima instance %s", instance))
		if err := isolationCleanupRunCommand("limactl", "stop", instance); err != nil {
			errorsSeen = append(errorsSeen, fmt.Sprintf("limactl stop %s: %v", instance, err))
		}
		s.appendLog(agentID, fmt.Sprintf("isolation cleanup: deleting Lima instance %s", instance))
		if err := isolationCleanupRunCommand("limactl", "delete", instance); err != nil {
			errorsSeen = append(errorsSeen, fmt.Sprintf("limactl delete %s: %v", instance, err))
		}
		if err := cleanupLimaTemplateFile(instance); err != nil {
			errorsSeen = append(errorsSeen, err.Error())
		}
	case "windows":
		distro := strings.TrimSpace(isolationEnvLookup(defaultWSLDistroEnvKey))
		if distro == "" {
			distro = instance
		}
		s.appendLog(agentID, fmt.Sprintf("isolation cleanup: terminating WSL distro %s", distro))
		if err := isolationCleanupRunCommand("wsl", "--terminate", distro); err != nil {
			errorsSeen = append(errorsSeen, fmt.Sprintf("wsl --terminate %s: %v", distro, err))
		}
		s.appendLog(agentID, fmt.Sprintf("isolation cleanup: unregistering WSL distro %s", distro))
		if err := isolationCleanupRunCommand("wsl", "--unregister", distro); err != nil {
			errorsSeen = append(errorsSeen, fmt.Sprintf("wsl --unregister %s: %v", distro, err))
		}
	case "linux":
		s.appendLog(agentID, "isolation cleanup: no VM cleanup needed on linux")
	default:
		s.appendLog(agentID, fmt.Sprintf("isolation cleanup: unsupported host OS %s", hostGOOS))
	}

	if len(errorsSeen) > 0 {
		return fmt.Errorf(strings.Join(errorsSeen, "; "))
	}
	return nil
}

func cleanupLimaTemplateFile(instance string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home directory for Lima template cleanup: %w", err)
	}

	templatePath := filepath.Join(homeDir, ".carrier", "lima", strings.TrimSpace(instance)+".yaml")
	if err := isolationCleanupRemoveFile(templatePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove lima template %s: %w", templatePath, err)
	}
	return nil
}
