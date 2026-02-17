package lifecycle

import (
	"context"
	"fmt"
	"time"

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

	// Check exponential backoff policy
	s.mu.Lock()
	backoffState := s.backoffStates[agentID]
	shouldRetry, backoffMsg := s.backoffPolicy.ShouldRetry(backoffState, s.now())
	s.mu.Unlock()

	if !shouldRetry {
		s.recordAudit("", "system", "start", agentID, AuditResultFailure, "E_BACKOFF_COOLDOWN", backoffMsg)
		return fmt.Errorf("%w: %s", ErrCrashLoop, backoffMsg)
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

	// Detect immediate process exit (e.g., command not found) by probing
	// multiple times instead of relying on a fixed sleep duration.
	if err := s.waitForStableStart(agentID); err != nil {
		s.updateStateOnStartError(agentID, err)
		s.recordAudit("", "system", "start", agentID, AuditResultFailure, "E_START_FAILED", err.Error())
		return err
	}

	// Auto-mount memories linked to this agent.
	s.autoMountMemories(agentID)

	s.mu.Lock()
	state = s.states[agentID]
	now := s.now()
	state.Runtime = RuntimeStateRunning
	state.Health = HealthStateHealthy
	state.LastError = ""
	state.LastTriageSummary = ""
	state.NeedsRemoteDiagnosis = false
	if state.StartedAt != nil {
		// Only count as a restart if the agent was previously started
		state.RestartCount = state.RestartCount + 1
	}
	state.StartedAt = &now
	// Populate ports from manifest
	if m, ok := s.manifests[agentID]; ok {
		ports := make([]int, 0, len(m.Network.Ports))
		for _, p := range m.Network.Ports {
			ports = append(ports, p.Port)
		}
		state.Ports = ports
	}
	state.UpdatedAt = now
	s.states[agentID] = state
	delete(s.restarts, agentID)
	delete(s.cooldowns, agentID)

	// Record successful start in backoff state
	backoffState = s.backoffStates[agentID]
	backoffState = s.backoffPolicy.RecordStart(backoffState, s.now())
	s.backoffStates[agentID] = backoffState
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

	// Mark as stopped before killing the process so that monitorProcess
	// (which races on the same lock) won't overwrite the state to Crashing.
	s.mu.Lock()
	state = s.states[agentID]
	state.Runtime = RuntimeStateStopped
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	s.mu.Unlock()

	// Stop the process using ProcessManager
	runErr := s.processManager.Stop(agentID)
	if runErr != nil {
		s.mu.Lock()
		state = s.states[agentID]
		state.Runtime = RuntimeStateRunning // revert — process may still be alive
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
	state.Health = HealthStateUnknown
	state.LastError = ""
	state.StartedAt = nil
	state.Ports = []int{}
	state.UpdatedAt = s.now()
	s.states[agentID] = state
	s.mu.Unlock()
	s.recordAudit("", "system", "stop", agentID, AuditResultSuccess, "", "stop completed")

	s.saveState()

	return nil
}

// stableStartProbes is the number of consecutive alive-checks required to
// consider a process stably started.
const stableStartProbes = 3

// stableStartInterval is the delay between successive alive-checks.
const stableStartInterval = 10 * time.Millisecond

// waitForStableStart probes the process multiple times to confirm it has not
// exited immediately after being started. This replaces the previous fixed
// sleep with a deterministic check: the process must be alive on every probe.
func (s *Service) waitForStableStart(agentID string) error {
	for i := 0; i < stableStartProbes; i++ {
		time.Sleep(stableStartInterval)
		if !s.processManager.IsRunning(agentID) {
			waitErr := s.processManager.Wait(agentID)
			if waitErr == nil {
				waitErr = fmt.Errorf("process exited immediately")
			}
			return waitErr
		}
	}
	return nil
}

// monitorProcess watches a running agent process and updates state when it exits.
func (s *Service) monitorProcess(agentID string) {
	err := s.processManager.Wait(agentID)

	var logLine string
	var shouldTriage bool
	var errorMsg string

	s.mu.Lock()
	state, ok := s.states[agentID]
	if !ok {
		s.mu.Unlock()
		return
	}

	// If the process exited unexpectedly (not stopped by user)
	if state.Runtime == RuntimeStateRunning {
		state.Runtime = RuntimeStateCrashing
		state.Health = HealthStateUnhealthy
		if err != nil {
			state.LastError = fmt.Sprintf("process exited: %v", err)
		} else {
			state.LastError = "process exited unexpectedly"
		}
		logLine = state.LastError
		errorMsg = state.LastError
		state.UpdatedAt = s.now()
		s.states[agentID] = state

		// Record crash for crash-loop detection (legacy)
		s.recordRestart(agentID)

		// Record crash for exponential backoff
		backoffState := s.backoffStates[agentID]
		backoffState = s.backoffPolicy.RecordCrash(backoffState, s.now())
		s.backoffStates[agentID] = backoffState

		// Update error message if crash looping
		if backoffState.CrashLooping {
			state.LastError = fmt.Sprintf("crash-loop: exceeded max retry attempts (%d); %s",
				s.backoffPolicy.MaxAttempts, state.LastError)
			s.states[agentID] = state
		}

		shouldTriage = true
	}
	s.mu.Unlock()

	// Persist crash state to disk
	if shouldTriage {
		s.saveState()
	}

	if logLine != "" {
		s.appendLog(agentID, logLine)
	}

	// Trigger failure handling for unexpected exits
	if shouldTriage {
		if _, triageErr := s.HandleFailure(context.Background(), agentID, errorMsg); triageErr != nil {
			s.appendLog(agentID, fmt.Sprintf("triage error: %v", triageErr))
		}
	}
}
