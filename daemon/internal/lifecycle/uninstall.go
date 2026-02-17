package lifecycle

import (
	"context"
	"fmt"
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
