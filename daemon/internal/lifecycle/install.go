package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"carrier/daemon/internal/baseagent"
	"carrier/daemon/internal/manifest"
)

const maxInstallAutoRepairRounds = 3

func (s *Service) Install(ctx context.Context, agentID string) error {
	m, _, err := s.getManifestAndState(agentID)
	if err != nil {
		return err
	}
	installCommand, err := m.Runtime.Install.ResolveForCurrentOS()
	if err != nil {
		manifestErr := fmt.Errorf("resolve install command for %s: %w", agentID, err)
		return s.finalizeInstallFailure(agentID, manifestErr, "E_MANIFEST_INVALID")
	}

	opCtx, cancel := context.WithTimeout(ctx, s.commandTimeout)
	defer cancel()

	if err := s.checkRuntimePrerequisites(m); err != nil {
		if prereqErr := s.repairRuntimePrerequisitesLoop(ctx, opCtx, agentID, m, err); prereqErr != nil {
			return s.finalizeInstallFailure(agentID, prereqErr, "E_RUNTIME_PREREQUISITES")
		}
	}

	result, streamed, runErr := s.runCommandWithAgentLogs(opCtx, agentID, "install", installCommand)
	if streamed {
		s.appendCommandLogSummary(agentID, "install", installCommand, result, runErr)
	} else {
		s.appendCommandLog(agentID, "install", installCommand, result, runErr)
	}
	if runErr != nil {
		loopErr := s.repairAndRetryInstallLoop(ctx, opCtx, agentID, installCommand, runErr)
		if loopErr != nil {
			return s.finalizeInstallFailure(agentID, loopErr, "E_INSTALL_FAILED")
		}
	}

	s.markInstallSuccess(agentID)
	s.recordAudit("", "system", "install", agentID, AuditResultSuccess, "", "install completed")

	s.saveState()

	return nil
}

func (s *Service) repairRuntimePrerequisitesLoop(ctx, opCtx context.Context, agentID string, m manifest.Manifest, initialErr error) error {
	currentErr := initialErr
	for round := 1; round <= maxInstallAutoRepairRounds; round++ {
		s.appendLog(agentID, fmt.Sprintf("auto-repair round %d/%d for runtime prerequisites", round, maxInstallAutoRepairRounds))
		if repaired := s.triageInstallFailure(ctx, opCtx, agentID, currentErr); !repaired {
			return currentErr
		}
		if retryErr := s.checkRuntimePrerequisites(m); retryErr == nil {
			return nil
		} else {
			currentErr = fmt.Errorf("runtime prerequisites still failing after auto-repair round %d: %w", round, retryErr)
		}
	}
	return fmt.Errorf("runtime prerequisites unresolved after %d auto-repair rounds: %w", maxInstallAutoRepairRounds, currentErr)
}

func (s *Service) repairAndRetryInstallLoop(ctx, opCtx context.Context, agentID, installCommand string, initialErr error) error {
	currentErr := initialErr
	for round := 1; round <= maxInstallAutoRepairRounds; round++ {
		s.appendLog(agentID, fmt.Sprintf("auto-repair round %d/%d for install command", round, maxInstallAutoRepairRounds))
		if repaired := s.triageInstallFailure(ctx, opCtx, agentID, currentErr); !repaired {
			return currentErr
		}

		action := "install-retry-" + strconv.Itoa(round)
		retryResult, retryStreamed, retryErr := s.runCommandWithAgentLogs(opCtx, agentID, action, installCommand)
		if retryStreamed {
			s.appendCommandLogSummary(agentID, action, installCommand, retryResult, retryErr)
		} else {
			s.appendCommandLog(agentID, action, installCommand, retryResult, retryErr)
		}
		if retryErr == nil {
			return nil
		}
		currentErr = fmt.Errorf("install failed after auto-repair round %d: %w", round, retryErr)
	}
	return fmt.Errorf("install failed after %d auto-repair rounds: %w", maxInstallAutoRepairRounds, currentErr)
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

	result, streamed, repairErr := s.runCommandWithAgentLogs(ctx, agentID, "repair", repairCommand)
	if streamed {
		s.appendCommandLogSummary(agentID, "repair", repairCommand, result, repairErr)
	} else {
		s.appendCommandLog(agentID, "repair", repairCommand, result, repairErr)
	}
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
