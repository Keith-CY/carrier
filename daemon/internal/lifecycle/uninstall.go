package lifecycle

import (
	"context"
	"fmt"
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

	var isolationCleanupErr error
	instanceName := strings.TrimSpace(state.LimaInstanceName)
	if instanceName != "" && strings.EqualFold(strings.TrimSpace(isolationRuntimeGOOS), "darwin") {
		backend, err := resolveIsolationBackend(isolationBackendOptions{InstanceName: instanceName})
		if err != nil {
			isolationCleanupErr = err
		} else if err := backend.Cleanup(); err != nil {
			isolationCleanupErr = err
		}
		if isolationCleanupErr != nil {
			s.appendLog(agentID, fmt.Sprintf("lima cleanup failed: %v", isolationCleanupErr))
		} else {
			s.appendLog(agentID, fmt.Sprintf("lima cleanup succeeded for %s", instanceName))
		}
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
	state.Isolated = false
	state.LimaInstanceName = ""
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

	if isolationCleanupErr != nil {
		return fmt.Errorf("isolation cleanup failed: %w", isolationCleanupErr)
	}
	return nil
}
