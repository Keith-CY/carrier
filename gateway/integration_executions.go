package gateway

import (
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
