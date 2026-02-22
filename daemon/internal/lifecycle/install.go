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

	opCtx, cancel := context.WithTimeout(ctx, s.commandTimeout)
	defer cancel()

	if err := s.checkRuntimePrerequisites(m); err != nil {
		repaired := s.triageInstallFailure(ctx, opCtx, agentID, err)
		if repaired {
			if retryPrereqErr := s.checkRuntimePrerequisites(m); retryPrereqErr != nil {
				err = fmt.Errorf("runtime prerequisites still failing after auto-repair: %w", retryPrereqErr)
			} else {
				err = nil
			}
		}
		if err != nil {
			return s.finalizeInstallFailure(agentID, err, "E_RUNTIME_PREREQUISITES")
		}
	}

	result, runErr := s.runner.Run(opCtx, m.Runtime.Install.Command)
	s.appendCommandLog(agentID, "install", m.Runtime.Install.Command, result, runErr)
	if runErr != nil {
		repaired := s.triageInstallFailure(ctx, opCtx, agentID, runErr)
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

		return s.finalizeInstallFailure(agentID, runErr, "E_INSTALL_FAILED")
	}

	s.markInstallSuccess(agentID)
	s.recordAudit("", "system", "install", agentID, AuditResultSuccess, "", "install completed")

	s.saveState()

	return nil
}

func (s *Service) triageInstallFailure(ctx, opCtx context.Context, agentID string, runErr error) bool {
	triage, triageErr := s.HandleFailure(ctx, agentID, runErr.Error())
	if triageErr != nil {
		s.appendLog(agentID, fmt.Sprintf("triage error: %v", triageErr))
		return false
	}
	if triage.Summary != "" {
		s.appendLog(agentID, fmt.Sprintf("triage summary: %s", triage.Summary))
	}

	repaired, repairErr := s.tryAutoRepairInstallFailure(opCtx, agentID, triage)
	if repairErr != nil {
		s.appendLog(agentID, fmt.Sprintf("auto-repair skipped: %v", repairErr))
		return false
	}
	return repaired
}

func (s *Service) finalizeInstallFailure(agentID string, runErr error, code string) error {
	s.updateStateOnInstallError(agentID, runErr)
	s.recordAudit("", "system", "install", agentID, AuditResultFailure, code, runErr.Error())

	diagnosePath, diagnoseErr := s.Diagnose(agentID)
	if diagnoseErr != nil {
		s.appendLog(agentID, fmt.Sprintf("diagnose generation failed: %v", diagnoseErr))
		s.saveState()
		return runErr
	}

	s.appendLog(agentID, fmt.Sprintf("diagnose artifact generated: %s", diagnosePath))
	s.saveState()
	return fmt.Errorf("%w (diagnose artifact: %s)", runErr, diagnosePath)
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
