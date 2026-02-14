package lifecycle

import (
	"context"
)

func (s *Service) Install(ctx context.Context, agentID string) error {
	m, state, err := s.getManifestAndState(agentID)
	if err != nil {
		return err
	}

	if err := s.checkRuntimePrerequisites(m); err != nil {
		s.updateStateOnInstallError(agentID, err)
		s.recordAudit("", "system", "install", agentID, AuditResultFailure, "E_RUNTIME_PREREQUISITES", err.Error())
		return err
	}

	result, runErr := s.runner.Run(ctx, m.Runtime.Install.Command)
	s.appendCommandLog(agentID, "install", m.Runtime.Install.Command, result, runErr)
	if runErr != nil {
		s.updateStateOnInstallError(agentID, runErr)
		s.recordAudit("", "system", "install", agentID, AuditResultFailure, "E_INSTALL_FAILED", runErr.Error())
		return runErr
	}

	s.mu.Lock()
	state = s.states[agentID]
	state.Install = InstallStateInstalled
	state.Runtime = RuntimeStateStopped
	state.Health = HealthStateUnknown
	state.LastError = ""
	state.LastTriageSummary = ""
	state.NeedsRemoteDiagnosis = false
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	s.mu.Unlock()
	s.recordAudit("", "system", "install", agentID, AuditResultSuccess, "", "install completed")

	s.saveState()

	return nil
}
