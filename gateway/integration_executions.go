package gateway

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"carrier/shared/integration"

	"github.com/google/uuid"
)

type integrationCreateResult struct {
	Execution integration.Execution
	Attempt   integration.Attempt
	Existing  bool
}

type integrationActionResult struct {
	Execution integration.Execution
	Action    integration.Action
}

func createIntegrationExecution(binding integration.Binding, req integration.CreateExecutionRequest) (integrationCreateResult, error) {
	normalizedReq, err := integration.NormalizeCreateExecutionRequest(req)
	if err != nil {
		return integrationCreateResult{}, err
	}

	integrationStoreMu.Lock()
	existing, attempt, found, err := loadIntegrationExecutionByBindingAndIdempotencyLocked(binding.ID, normalizedReq.IdempotencyKey)
	if err != nil {
		integrationStoreMu.Unlock()
		return integrationCreateResult{}, err
	}
	if found {
		integrationStoreMu.Unlock()
		materialized, materializeErr := materializeIntegrationExecutionLocked(existing)
		if materializeErr != nil {
			return integrationCreateResult{}, materializeErr
		}
		return integrationCreateResult{Execution: materialized, Attempt: attempt, Existing: true}, nil
	}

	internal, err := buildIntegrationOrchestratorExecution(binding, normalizedReq)
	if err != nil {
		integrationStoreMu.Unlock()
		return integrationCreateResult{}, err
	}
	now := nowTimestamp()
	internal.ID = uuid.NewString()
	internal.Status = OrchestratorExecutionStatusProvisioning
	internal.Authorization = OrchestratorAuthorization{
		InfrastructureApproved: true,
		ApprovedBy:             "integration:" + binding.Adapter,
		ApprovedAt:             now,
	}
	internal.CreatedAt = now
	internal.StartedAt = now
	internal.UpdatedAt = now
	internal.CompletedAt = ""
	internal.Error = ""
	internal.Results = []OrchestratorTaskResult{}
	internal.Outcome = OrchestratorExecutionOutcome{}

	policyRules, policyErr := listOrchestratorPolicies()
	if policyErr != nil {
		integrationStoreMu.Unlock()
		return integrationCreateResult{}, fmt.Errorf("list orchestrator policies: %w", policyErr)
	}
	remoteHosts, remoteHostsErr := listRemoteHosts()
	if remoteHostsErr != nil {
		integrationStoreMu.Unlock()
		return integrationCreateResult{}, fmt.Errorf("list remote hosts: %w", remoteHostsErr)
	}
	internal = applyOrchestratorExecutionPolicy(internal, policyRules, remoteHosts)
	if internal.Policy.Decision == orchestratorPolicyDecisionDeny {
		integrationStoreMu.Unlock()
		return integrationCreateResult{}, fmt.Errorf("policy denied integration execution: %s", firstNonEmptyPolicyValue(internal.Policy.Reason, "execution denied"))
	}

	savedOrchestrator, err := upsertOrchestratorExecution(internal)
	if err != nil {
		integrationStoreMu.Unlock()
		return integrationCreateResult{}, fmt.Errorf("save orchestrator execution: %w", err)
	}

	execution := integration.Execution{
		ID:                      "cexec_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		BindingID:               binding.ID,
		Adapter:                 binding.Adapter,
		Account:                 binding.Account,
		ExternalExecutionID:     normalizedReq.ExternalExecutionID,
		OrchestratorExecutionID: savedOrchestrator.ID,
		IdempotencyKey:          normalizedReq.IdempotencyKey,
		State:                   integration.ExecutionStateAccepted,
		Goal:                    normalizedReq.Goal,
		Input:                   normalizedReq.Input,
		RequestedProvider:       normalizedReq.RequestedProvider,
		CreatedAt:               now,
		StartedAt:               now,
		UpdatedAt:               now,
	}
	attempt = integration.Attempt{
		ID:          "attempt_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		ExecutionID: execution.ID,
		Number:      1,
		CreatedAt:   now,
		StartedAt:   now,
		UpdatedAt:   now,
	}

	db, err := openIntegrationDB()
	if err != nil {
		integrationStoreMu.Unlock()
		return integrationCreateResult{}, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		integrationStoreMu.Unlock()
		return integrationCreateResult{}, fmt.Errorf("begin integration execution tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertIntegrationExecutionTx(tx, execution); err != nil {
		integrationStoreMu.Unlock()
		return integrationCreateResult{}, err
	}
	if err := insertIntegrationAttemptTx(tx, attempt); err != nil {
		integrationStoreMu.Unlock()
		return integrationCreateResult{}, err
	}
	if _, err := appendIntegrationEventTx(tx, execution.ID, attempt.ID, "execution.accepted", map[string]interface{}{
		"carrierExecutionId": execution.ID,
		"attemptId":          attempt.ID,
		"bindingId":          binding.ID,
		"orchestratorId":     savedOrchestrator.ID,
	}); err != nil {
		integrationStoreMu.Unlock()
		return integrationCreateResult{}, err
	}
	if err := tx.Commit(); err != nil {
		integrationStoreMu.Unlock()
		return integrationCreateResult{}, fmt.Errorf("commit integration execution tx: %w", err)
	}

	integrationStoreMu.Unlock()
	orchestratorLaunchExecutionFn(savedOrchestrator.ID)
	return integrationCreateResult{Execution: execution, Attempt: attempt}, nil
}

func getIntegrationExecutionByID(executionID string) (integration.Execution, integration.Attempt, bool, error) {
	exec, attempt, found, err := loadIntegrationExecutionByIDLocked(executionID)
	if err != nil || !found {
		return exec, attempt, found, err
	}
	materialized, err := materializeIntegrationExecutionLocked(exec)
	if err != nil {
		return integration.Execution{}, integration.Attempt{}, false, err
	}
	return materialized, attempt, true, nil
}

func applyIntegrationAction(executionID string, req integration.ActionRequest) (integrationActionResult, int, error) {
	normalizedReq, err := integration.NormalizeActionRequest(req)
	if err != nil {
		return integrationActionResult{}, httpStatusBadRequest, err
	}
	exec, _, found, err := loadIntegrationExecutionByIDLocked(executionID)
	if err != nil {
		return integrationActionResult{}, httpStatusInternalServerError, err
	}
	if !found {
		return integrationActionResult{}, httpStatusNotFound, fmt.Errorf("integration execution %s not found", strings.TrimSpace(executionID))
	}
	if normalizedReq.IdempotencyKey != "" {
		if existing, foundAction, findErr := findIntegrationActionByExecutionAndKeyLocked(exec.ID, normalizedReq.IdempotencyKey); findErr != nil {
			return integrationActionResult{}, httpStatusInternalServerError, findErr
		} else if foundAction {
			materialized, materializeErr := materializeIntegrationExecutionLocked(exec)
			if materializeErr != nil {
				return integrationActionResult{}, httpStatusInternalServerError, materializeErr
			}
			return integrationActionResult{Execution: materialized, Action: existing}, httpStatusAccepted, nil
		}
	}
	switch normalizedReq.Type {
	case integration.ActionTypePause:
		paused, pauseErr := pauseOrchestratorExecution(exec.OrchestratorExecutionID, "integration:"+exec.Adapter)
		if pauseErr != nil {
			action, actionErr := buildRejectedIntegrationAction(exec.ID, normalizedReq, pauseErr.Error())
			if actionErr != nil {
				return integrationActionResult{}, httpStatusInternalServerError, actionErr
			}
			if saveErr := saveIntegrationAction(action); saveErr != nil {
				return integrationActionResult{}, httpStatusInternalServerError, saveErr
			}
			return integrationActionResult{Execution: exec, Action: action}, httpStatusConflict, pauseErr
		}
		action, err := buildAppliedIntegrationAction(exec.ID, normalizedReq)
		if err != nil {
			return integrationActionResult{}, httpStatusInternalServerError, err
		}
		materialized, err := rematerializeIntegrationExecutionLocked(exec, paused)
		if err != nil {
			return integrationActionResult{}, httpStatusInternalServerError, err
		}
		if err := saveIntegrationAction(action); err != nil {
			return integrationActionResult{}, httpStatusInternalServerError, err
		}
		if _, err := appendIntegrationEvent(exec.ID, materialized.CurrentAttemptID, "execution.pause_requested", map[string]interface{}{"reason": normalizedReq.Reason}); err != nil {
			return integrationActionResult{}, httpStatusInternalServerError, err
		}
		return integrationActionResult{Execution: materialized, Action: action}, httpStatusAccepted, nil
	case integration.ActionTypeResume:
		resumed, resumeErr := resumeOrchestratorExecution(exec.OrchestratorExecutionID, "integration:"+exec.Adapter)
		if resumeErr != nil {
			action, actionErr := buildRejectedIntegrationAction(exec.ID, normalizedReq, resumeErr.Error())
			if actionErr != nil {
				return integrationActionResult{}, httpStatusInternalServerError, actionErr
			}
			if saveErr := saveIntegrationAction(action); saveErr != nil {
				return integrationActionResult{}, httpStatusInternalServerError, saveErr
			}
			return integrationActionResult{Execution: exec, Action: action}, httpStatusConflict, resumeErr
		}
		action, err := buildAppliedIntegrationAction(exec.ID, normalizedReq)
		if err != nil {
			return integrationActionResult{}, httpStatusInternalServerError, err
		}
		materialized, err := rematerializeIntegrationExecutionLocked(exec, resumed)
		if err != nil {
			return integrationActionResult{}, httpStatusInternalServerError, err
		}
		if err := saveIntegrationAction(action); err != nil {
			return integrationActionResult{}, httpStatusInternalServerError, err
		}
		if _, err := appendIntegrationEvent(exec.ID, materialized.CurrentAttemptID, "execution.resumed", map[string]interface{}{"reason": normalizedReq.Reason}); err != nil {
			return integrationActionResult{}, httpStatusInternalServerError, err
		}
		return integrationActionResult{Execution: materialized, Action: action}, httpStatusAccepted, nil
	case integration.ActionTypeCancel:
	default:
		action, actionErr := buildRejectedIntegrationAction(exec.ID, normalizedReq, "action is not implemented yet")
		if actionErr != nil {
			return integrationActionResult{}, httpStatusInternalServerError, actionErr
		}
		if saveErr := saveIntegrationAction(action); saveErr != nil {
			return integrationActionResult{}, httpStatusInternalServerError, saveErr
		}
		return integrationActionResult{Execution: exec, Action: action}, httpStatusConflict, fmt.Errorf("action %s is not implemented yet", normalizedReq.Type)
	}
	cancelled, cancelErr := cancelOrchestratorExecution(exec.OrchestratorExecutionID, "integration:"+exec.Adapter)
	if cancelErr != nil {
		return integrationActionResult{}, httpStatusInternalServerError, cancelErr
	}
	action, err := buildAppliedIntegrationAction(exec.ID, normalizedReq)
	if err != nil {
		return integrationActionResult{}, httpStatusInternalServerError, err
	}
	materialized, err := rematerializeIntegrationExecutionLocked(exec, cancelled)
	if err != nil {
		return integrationActionResult{}, httpStatusInternalServerError, err
	}
	if err := saveIntegrationAction(action); err != nil {
		return integrationActionResult{}, httpStatusInternalServerError, err
	}
	return integrationActionResult{Execution: materialized, Action: action}, httpStatusAccepted, nil
}

const (
	httpStatusAccepted            = 202
	httpStatusBadRequest          = 400
	httpStatusNotFound            = 404
	httpStatusConflict            = 409
	httpStatusInternalServerError = 500
)

func buildIntegrationOrchestratorExecution(binding integration.Binding, req integration.CreateExecutionRequest) (OrchestratorExecution, error) {
	execution, err := normalizeOrchestratorExecution(OrchestratorExecution{
		Goal:              req.Goal,
		RequestedProvider: req.RequestedProvider,
		RequiredMemory:    req.RequiredMemory,
		DistillOutputs:    req.DistillOutputs,
		IdempotencyKey:    binding.ID + ":" + req.IdempotencyKey,
		ApprovalScope:     "infrastructure_only",
		MaxConcurrency:    req.MaxConcurrency,
		RequiredWorkers: []OrchestratorRequiredWorker{{
			HostID:  binding.Target.HostID,
			AgentID: binding.Target.AgentID,
			Count:   1,
		}},
		TaskUnits: []OrchestratorTaskUnit{{
			ID:      "task-1",
			Input:   req.Input,
			HostID:  binding.Target.HostID,
			AgentID: binding.Target.AgentID,
		}},
	})
	if err != nil {
		return OrchestratorExecution{}, err
	}
	return execution, nil
}

func insertIntegrationExecutionTx(tx *sql.Tx, execution integration.Execution) error {
	_, err := tx.Exec(`
		INSERT INTO integration_executions (
			id, binding_id, adapter, account, external_execution_id, orchestrator_execution_id, idempotency_key,
			state, goal, input, requested_provider, failure_category, failure_code, current_attempt_id,
			created_at, started_at, completed_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, execution.ID, execution.BindingID, execution.Adapter, execution.Account, execution.ExternalExecutionID, execution.OrchestratorExecutionID,
		execution.IdempotencyKey, execution.State, execution.Goal, execution.Input, execution.RequestedProvider, execution.FailureCategory,
		execution.FailureCode, execution.CurrentAttemptID, execution.CreatedAt, execution.StartedAt, execution.CompletedAt, execution.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert integration execution: %w", err)
	}
	return nil
}

func insertIntegrationAttemptTx(tx *sql.Tx, attempt integration.Attempt) error {
	_, err := tx.Exec(`
		INSERT INTO integration_attempts (id, execution_id, attempt_number, created_at, started_at, completed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, attempt.ID, attempt.ExecutionID, attempt.Number, attempt.CreatedAt, attempt.StartedAt, attempt.CompletedAt, attempt.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert integration attempt: %w", err)
	}
	if _, err := tx.Exec(`UPDATE integration_executions SET current_attempt_id = ?, updated_at = ? WHERE id = ?`, attempt.ID, attempt.UpdatedAt, attempt.ExecutionID); err != nil {
		return fmt.Errorf("link integration current attempt: %w", err)
	}
	return nil
}

func loadIntegrationExecutionByBindingAndIdempotencyLocked(bindingID, idempotencyKey string) (integration.Execution, integration.Attempt, bool, error) {
	db, err := openIntegrationDB()
	if err != nil {
		return integration.Execution{}, integration.Attempt{}, false, err
	}
	defer db.Close()
	row := db.QueryRow(`
		SELECT id, binding_id, adapter, account, external_execution_id, orchestrator_execution_id, idempotency_key,
			state, goal, input, requested_provider, failure_category, failure_code, current_attempt_id,
			created_at, started_at, completed_at, updated_at
		FROM integration_executions
		WHERE binding_id = ? AND idempotency_key = ?
	`, strings.TrimSpace(bindingID), strings.TrimSpace(idempotencyKey))
	exec, found, err := scanIntegrationExecution(row)
	if err != nil || !found {
		return integration.Execution{}, integration.Attempt{}, found, err
	}
	attempt, _, attemptErr := loadCurrentAttemptByExecutionID(db, exec.ID)
	if attemptErr != nil {
		return integration.Execution{}, integration.Attempt{}, false, attemptErr
	}
	return exec, attempt, true, nil
}

func loadIntegrationExecutionByIDLocked(executionID string) (integration.Execution, integration.Attempt, bool, error) {
	db, err := openIntegrationDB()
	if err != nil {
		return integration.Execution{}, integration.Attempt{}, false, err
	}
	defer db.Close()
	row := db.QueryRow(`
		SELECT id, binding_id, adapter, account, external_execution_id, orchestrator_execution_id, idempotency_key,
			state, goal, input, requested_provider, failure_category, failure_code, current_attempt_id,
			created_at, started_at, completed_at, updated_at
		FROM integration_executions
		WHERE id = ?
	`, strings.TrimSpace(executionID))
	exec, found, err := scanIntegrationExecution(row)
	if err != nil || !found {
		return integration.Execution{}, integration.Attempt{}, found, err
	}
	attempt, _, attemptErr := loadCurrentAttemptByExecutionID(db, exec.ID)
	if attemptErr != nil {
		return integration.Execution{}, integration.Attempt{}, false, attemptErr
	}
	return exec, attempt, true, nil
}

func loadCurrentAttemptByExecutionID(db *sql.DB, executionID string) (integration.Attempt, bool, error) {
	row := db.QueryRow(`
		SELECT id, execution_id, attempt_number, created_at, started_at, completed_at, updated_at
		FROM integration_attempts
		WHERE execution_id = ?
		ORDER BY attempt_number DESC
		LIMIT 1
	`, strings.TrimSpace(executionID))
	var attempt integration.Attempt
	err := row.Scan(&attempt.ID, &attempt.ExecutionID, &attempt.Number, &attempt.CreatedAt, &attempt.StartedAt, &attempt.CompletedAt, &attempt.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return integration.Attempt{}, false, nil
	}
	if err != nil {
		return integration.Attempt{}, false, fmt.Errorf("scan integration attempt: %w", err)
	}
	return attempt, true, nil
}

func materializeIntegrationExecutionLocked(exec integration.Execution) (integration.Execution, error) {
	orchestrator, ok, err := getOrchestratorExecution(exec.OrchestratorExecutionID)
	if err != nil {
		return integration.Execution{}, err
	}
	if !ok {
		return exec, nil
	}
	if err := syncIntegrationUsageProofsForExecution(exec, orchestrator); err != nil {
		return integration.Execution{}, err
	}
	if err := syncIntegrationArtifactRefsForExecution(exec, orchestrator); err != nil {
		return integration.Execution{}, err
	}
	return rematerializeIntegrationExecutionLocked(exec, orchestrator)
}

func rematerializeIntegrationExecutionLocked(exec integration.Execution, orchestrator OrchestratorExecution) (integration.Execution, error) {
	exec.State = mapOrchestratorExecutionState(orchestrator.Status)
	exec.FailureCategory = mapFailureCategory(orchestrator)
	exec.FailureCode = strings.TrimSpace(orchestrator.Error)
	exec.UpdatedAt = nowTimestamp()
	if orchestrator.StartedAt != "" {
		exec.StartedAt = orchestrator.StartedAt
	}
	if orchestrator.CompletedAt != "" {
		exec.CompletedAt = orchestrator.CompletedAt
	}
	db, err := openIntegrationDB()
	if err != nil {
		return integration.Execution{}, err
	}
	defer db.Close()
	if _, err := db.Exec(`
		UPDATE integration_executions
		SET state = ?, failure_category = ?, failure_code = ?, started_at = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`, exec.State, exec.FailureCategory, exec.FailureCode, exec.StartedAt, exec.CompletedAt, exec.UpdatedAt, exec.ID); err != nil {
		return integration.Execution{}, fmt.Errorf("update materialized integration execution: %w", err)
	}
	return exec, nil
}

func scanIntegrationExecution(scanner interface{ Scan(dest ...any) error }) (integration.Execution, bool, error) {
	var exec integration.Execution
	err := scanner.Scan(
		&exec.ID,
		&exec.BindingID,
		&exec.Adapter,
		&exec.Account,
		&exec.ExternalExecutionID,
		&exec.OrchestratorExecutionID,
		&exec.IdempotencyKey,
		&exec.State,
		&exec.Goal,
		&exec.Input,
		&exec.RequestedProvider,
		&exec.FailureCategory,
		&exec.FailureCode,
		&exec.CurrentAttemptID,
		&exec.CreatedAt,
		&exec.StartedAt,
		&exec.CompletedAt,
		&exec.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return integration.Execution{}, false, nil
	}
	if err != nil {
		return integration.Execution{}, false, fmt.Errorf("scan integration execution: %w", err)
	}
	return exec, true, nil
}

func loadIntegrationExecutionByOrchestratorID(orchestratorExecutionID string) (integration.Execution, bool, error) {
	integrationStoreMu.Lock()
	defer integrationStoreMu.Unlock()

	db, err := openIntegrationDB()
	if err != nil {
		return integration.Execution{}, false, err
	}
	defer db.Close()
	row := db.QueryRow(`
		SELECT id, binding_id, adapter, account, external_execution_id, orchestrator_execution_id, idempotency_key,
			state, goal, input, requested_provider, failure_category, failure_code, current_attempt_id,
			created_at, started_at, completed_at, updated_at
		FROM integration_executions
		WHERE orchestrator_execution_id = ?
	`, strings.TrimSpace(orchestratorExecutionID))
	return scanIntegrationExecution(row)
}

func buildAppliedIntegrationAction(executionID string, req integration.ActionRequest) (integration.Action, error) {
	now := nowTimestamp()
	return integration.Action{
		ID:             "action_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		ExecutionID:    executionID,
		Type:           req.Type,
		IdempotencyKey: req.IdempotencyKey,
		Reason:         req.Reason,
		State:          integration.ActionStateApplied,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func buildRejectedIntegrationAction(executionID string, req integration.ActionRequest, reason string) (integration.Action, error) {
	now := nowTimestamp()
	return integration.Action{
		ID:             "action_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		ExecutionID:    executionID,
		Type:           req.Type,
		IdempotencyKey: req.IdempotencyKey,
		Reason:         firstNonEmpty(reason, req.Reason),
		State:          integration.ActionStateRejected,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

func saveIntegrationAction(action integration.Action) error {
	db, err := openIntegrationDB()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`
		INSERT INTO integration_actions (id, execution_id, action_type, idempotency_key, reason, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, action.ID, action.ExecutionID, action.Type, action.IdempotencyKey, action.Reason, action.State, action.CreatedAt, action.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert integration action: %w", err)
	}
	return nil
}

func findIntegrationActionByExecutionAndKeyLocked(executionID, idempotencyKey string) (integration.Action, bool, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return integration.Action{}, false, nil
	}
	db, err := openIntegrationDB()
	if err != nil {
		return integration.Action{}, false, err
	}
	defer db.Close()
	row := db.QueryRow(`
		SELECT id, execution_id, action_type, idempotency_key, reason, state, created_at, updated_at
		FROM integration_actions
		WHERE execution_id = ? AND idempotency_key = ?
	`, strings.TrimSpace(executionID), strings.TrimSpace(idempotencyKey))
	var action integration.Action
	err = row.Scan(&action.ID, &action.ExecutionID, &action.Type, &action.IdempotencyKey, &action.Reason, &action.State, &action.CreatedAt, &action.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return integration.Action{}, false, nil
	}
	if err != nil {
		return integration.Action{}, false, fmt.Errorf("scan integration action: %w", err)
	}
	return action, true, nil
}

func mapOrchestratorExecutionState(status OrchestratorExecutionStatus) integration.ExecutionState {
	switch status {
	case OrchestratorExecutionStatusRunning:
		return integration.ExecutionStateRunning
	case OrchestratorExecutionStatusPauseRequested:
		return integration.ExecutionStatePauseRequested
	case OrchestratorExecutionStatusPaused:
		return integration.ExecutionStatePaused
	case OrchestratorExecutionStatusCompleted, OrchestratorExecutionStatusPartialCompleted:
		return integration.ExecutionStateCompleted
	case OrchestratorExecutionStatusCancelled:
		return integration.ExecutionStateCancelled
	case OrchestratorExecutionStatusFailed, OrchestratorExecutionStatusRetryableFailed, OrchestratorExecutionStatusDeclined:
		return integration.ExecutionStateFailed
	default:
		return integration.ExecutionStateAccepted
	}
}

func mapFailureCategory(execution OrchestratorExecution) string {
	category := strings.TrimSpace(execution.Outcome.FailureCategory)
	if category != "" {
		return category
	}
	if execution.Status == OrchestratorExecutionStatusDeclined {
		return "policy_denied"
	}
	if execution.Status == OrchestratorExecutionStatusFailed || execution.Status == OrchestratorExecutionStatusRetryableFailed {
		return "unknown"
	}
	return ""
}
