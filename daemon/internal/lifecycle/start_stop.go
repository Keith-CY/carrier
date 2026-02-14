package lifecycle

import (
	"context"
	"fmt"

	"carrier/daemon/internal/runtimecheck"
)

func (s *Service) Start(ctx context.Context, agentID string) error {
	m, state, err := s.getManifestAndState(agentID)
	if err != nil {
		return err
	}
	if state.Install != InstallStateInstalled {
		return ErrNotInstalled
	}
	if state.Runtime == RuntimeStateRunning {
		return ErrAlreadyRunning
	}
	if err := s.blockIfCrashLoopCoolingDown(agentID, state); err != nil {
		return err
	}

	if pf, ok := s.checker.(runtimecheck.PreFlighter); ok {
		pfResult := pf.PreFlight(m)
		if !pfResult.Passed {
			failMsg := formatPreFlightFailures(pfResult)
			errCode := firstFailedCode(pfResult)
			pfErr := fmt.Errorf("pre-flight checks failed: %s", failMsg)
			s.updateStateOnStartError(agentID, pfErr)
			s.recordAudit("", "system", "start", agentID, AuditResultFailure, errCode, failMsg)
			return pfErr
		}
	} else {
		if err := s.checkRuntimePrerequisites(m); err != nil {
			s.updateStateOnStartError(agentID, err)
			s.recordAudit("", "system", "start", agentID, AuditResultFailure, "E_RUNTIME_PREREQUISITES", err.Error())
			return err
		}
		if err := s.validateRequiredEnv(m); err != nil {
			s.updateStateOnStartError(agentID, err)
			s.recordAudit("", "system", "start", agentID, AuditResultFailure, "E_ENV_MISSING", err.Error())
			return err
		}
		if err := s.ensurePortsAvailable(m.Network.Ports); err != nil {
			s.updateStateOnStartError(agentID, err)
			s.recordAudit("", "system", "start", agentID, AuditResultFailure, "E_PORT_CONFLICT", err.Error())
			return err
		}
	}

	// Start the process using ProcessManager
	pid, runErr := s.processManager.Start(agentID, "sh", []string{"-c", m.Runtime.Start.Command})
	if runErr != nil {
		triage, triageErr := s.HandleFailure(ctx, agentID, runErr.Error())
		if triageErr == nil {
			s.appendLog(agentID, fmt.Sprintf("triage summary: %s", triage.Summary))
		}
		s.updateStateOnStartError(agentID, runErr)
		s.recordAudit("", "system", "start", agentID, AuditResultFailure, "E_START_FAILED", runErr.Error())
		return runErr
	}

	s.appendLog(agentID, fmt.Sprintf("started process with PID %d", pid))

	// Auto-mount memories linked to this agent.
	s.autoMountMemories(agentID)

	s.mu.Lock()
	state = s.states[agentID]
	state.Runtime = RuntimeStateRunning
	state.Health = HealthStateHealthy
	state.LastError = ""
	state.LastTriageSummary = ""
	state.NeedsRemoteDiagnosis = false
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	delete(s.restarts, agentID)
	delete(s.cooldowns, agentID)
	s.mu.Unlock()
	s.recordAudit("", "system", "start", agentID, AuditResultSuccess, "", fmt.Sprintf("start completed (PID %d)", pid))

	// Monitor the process in background
	go s.monitorProcess(agentID)

	s.saveState()

	return nil
}

func (s *Service) Stop(ctx context.Context, agentID string) error {
	_, state, err := s.getManifestAndState(agentID)
	if err != nil {
		return err
	}
	if state.Runtime == RuntimeStateStopped {
		return ErrAlreadyStopped
	}

	// Stop the process using ProcessManager
	runErr := s.processManager.Stop(agentID)
	if runErr != nil {
		s.mu.Lock()
		state = s.states[agentID]
		state.LastError = runErr.Error()
		state.UpdatedAt = s.now()
		s.states[agentID] = state
		s.mu.Unlock()
		s.recordAudit("", "system", "stop", agentID, AuditResultFailure, "E_STOP_FAILED", runErr.Error())
		return runErr
	}

	s.appendLog(agentID, "process stopped successfully")

	// Auto-unmount all memories for this agent.
	s.autoUnmountMemories(agentID)

	s.mu.Lock()
	state = s.states[agentID]
	state.Runtime = RuntimeStateStopped
	state.Health = HealthStateUnknown
	state.LastError = ""
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	s.mu.Unlock()
	s.recordAudit("", "system", "stop", agentID, AuditResultSuccess, "", "stop completed")

	s.saveState()

	return nil
}

// monitorProcess watches a running agent process and updates state when it exits.
func (s *Service) monitorProcess(agentID string) {
	err := s.processManager.Wait(agentID)

	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.states[agentID]
	if !ok {
		return
	}

	// If the process exited unexpectedly (not stopped by user)
	if state.Runtime == RuntimeStateRunning {
		state.Runtime = RuntimeStateCrashing
		state.Health = HealthStateUnhealthy
		if err != nil {
			state.LastError = fmt.Sprintf("process exited: %v", err)
			s.appendLog(agentID, state.LastError)
		} else {
			state.LastError = "process exited unexpectedly"
			s.appendLog(agentID, state.LastError)
		}
		state.UpdatedAt = s.now()
		s.states[agentID] = state

		// Record crash for crash-loop detection
		s.recordRestart(agentID)
	}
}
