package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"carrier/daemon/internal/baseagent"
)

func (s *Service) Install(ctx context.Context, agentID string) error {
	m, _, err := s.getManifestAndState(agentID)
	if err != nil {
		return err
	}

	if err := s.checkRuntimePrerequisites(m); err != nil {
		s.updateStateOnInstallError(agentID, err)
		s.recordAudit("", "system", "install", agentID, AuditResultFailure, "E_RUNTIME_PREREQUISITES", err.Error())
		return err
	}

	opCtx, cancel := context.WithTimeout(ctx, s.commandTimeout)
	defer cancel()

	result, runErr := s.runner.Run(opCtx, m.Runtime.Install.Command)
	s.appendCommandLog(agentID, "install", m.Runtime.Install.Command, result, runErr)
	if runErr != nil {
		triage, triageErr := s.HandleFailure(ctx, agentID, runErr.Error())
		if triageErr != nil {
			s.appendLog(agentID, fmt.Sprintf("triage error: %v", triageErr))
		} else if triage.Summary != "" {
			s.appendLog(agentID, fmt.Sprintf("triage summary: %s", triage.Summary))
		}

		repaired := false
		if triageErr == nil {
			repaired, err = s.tryAutoRepairInstallFailure(opCtx, agentID, triage)
			if err != nil {
				s.appendLog(agentID, fmt.Sprintf("auto-repair skipped: %v", err))
			}
		}
		if repaired {
			retryResult, retryErr := s.runner.Run(opCtx, m.Runtime.Install.Command)
			s.appendCommandLog(agentID, "install-retry", m.Runtime.Install.Command, retryResult, retryErr)
			if retryErr == nil {
				s.markInstallSuccess(agentID)
				s.recordAudit("", "system", "install", agentID, AuditResultSuccess, "", "install completed after auto-repair")
				s.saveState()
				return nil
			}
			runErr = fmt.Errorf("install failed after auto-repair: %w", retryErr)
		}

		s.updateStateOnInstallError(agentID, runErr)
		s.recordAudit("", "system", "install", agentID, AuditResultFailure, "E_INSTALL_FAILED", runErr.Error())
		return runErr
	}

	s.markInstallSuccess(agentID)
	s.recordAudit("", "system", "install", agentID, AuditResultSuccess, "", "install completed")

	s.saveState()

	return nil
}

func (s *Service) markInstallSuccess(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.states[agentID]
	if !ok {
		return
	}
	state.Install = InstallStateInstalled
	state.Runtime = RuntimeStateStopped
	state.Health = HealthStateUnknown
	state.LastError = ""
	state.LastTriageSummary = ""
	state.NeedsRemoteDiagnosis = false
	state.UpdatedAt = s.now()
	s.states[agentID] = state
}

func (s *Service) tryAutoRepairInstallFailure(ctx context.Context, agentID string, triage baseagent.TriageResult) (bool, error) {
	if !strings.EqualFold(agentID, "openclaw") {
		return false, nil
	}
	if triage.RepairAction == nil {
		return false, nil
	}

	action, err := baseagent.ValidateRepairAction(*triage.RepairAction, false)
	if err != nil {
		if errors.Is(err, baseagent.ErrRepairActionNeedsConfirmation) || errors.Is(err, baseagent.ErrRepairActionNotAllowed) {
			s.recordAudit("", "base-agent", "repair", agentID, AuditResultFailure, "E_REPAIR_POLICY", err.Error())
		}
		return false, err
	}

	repairCommand := strings.TrimSpace(action.Command)
	targetPath := strings.TrimSpace(action.TargetPath)
	if repairCommand == "" {
		return false, nil
	}
	if targetPath != "" {
		repairCommand = fmt.Sprintf("cd %s && %s", shellSingleQuote(targetPath), repairCommand)
	}

	result, repairErr := s.runner.Run(ctx, repairCommand)
	s.appendCommandLog(agentID, "repair", repairCommand, result, repairErr)
	if repairErr != nil {
		s.recordAudit("", "base-agent", "repair", agentID, AuditResultFailure, "E_REPAIR_FAILED", repairErr.Error())
		return false, repairErr
	}
	s.recordAudit("", "base-agent", "repair", agentID, AuditResultSuccess, "", fmt.Sprintf("auto-repair command succeeded: %s", action.Command))
	return true, nil
}

func shellSingleQuote(input string) string {
	if input == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(input, "'", `'"'"'`) + "'"
}
