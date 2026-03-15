package gateway

import (
	"carrier/shared/orchestration"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type executionTemplateLaunchRequest struct {
	Inputs         map[string]string `json:"inputs,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	HostIDs        []string          `json:"hostIds,omitempty"`
	HostLabels     []string          `json:"hostLabels,omitempty"`
	RequiredMemory []string          `json:"requiredMemory,omitempty"`
	DistillOutputs []string          `json:"distillOutputs,omitempty"`
	MaxConcurrency int               `json:"maxConcurrency,omitempty"`
	PolicyApprove  bool              `json:"policyApprove,omitempty"`
	IdempotencyKey string            `json:"idempotencyKey,omitempty"`
	Actor          string            `json:"actor,omitempty"`
}

type executionLaunchMetadata struct {
	TriggerSource        string
	TriggerID            string
	TriggerEvent         string
	TriggerPayloadDigest string
	Initiator            string
}

type executionTemplateLaunchOptions struct {
	TemplateID     string
	Inputs         map[string]string
	Provider       string
	HostIDs        []string
	HostLabels     []string
	RequiredMemory []string
	DistillOutputs []string
	MaxConcurrency int
	PolicyApprove  bool
	IdempotencyKey string
	Actor          string
	Metadata       executionLaunchMetadata
}

func handleExecutionTemplates(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}

	trimmed := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/templates"), "/")
	if trimmed == "" {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		if _, ok := requireGatewayPermission(w, r, cfg, canViewExecutions, "E_RBAC_EXECUTION_VIEW", "role cannot view execution templates"); !ok {
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"templates": orchestration.ListExecutionTemplates(),
		})
		return
	}

	parts := strings.Split(trimmed, "/")
	templateID := strings.TrimSpace(parts[0])
	if templateID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "template id is required"))
		return
	}
	template, ok := orchestration.GetExecutionTemplate(templateID)
	if !ok {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "template not found"))
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		if _, ok := requireGatewayPermission(w, r, cfg, canViewExecutions, "E_RBAC_EXECUTION_VIEW", "role cannot view execution templates"); !ok {
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"template":  template,
		})
		return
	}

	if len(parts) != 2 || strings.ToLower(strings.TrimSpace(parts[1])) != "launch" {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported template action"))
		return
	}
	if _, ok := requireGatewayPermission(w, r, cfg, canLaunchExecutions, "E_RBAC_EXECUTION_LAUNCH", "role cannot launch execution templates"); !ok {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}

	var req executionTemplateLaunchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
		return
	}
	req.Provider = strings.TrimSpace(req.Provider)
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.Actor = strings.TrimSpace(req.Actor)

	authorized, apiErr := launchExecutionTemplate(requestID, cfg, executionTemplateLaunchOptions{
		TemplateID:     template.ID,
		Inputs:         req.Inputs,
		Provider:       req.Provider,
		HostIDs:        req.HostIDs,
		HostLabels:     req.HostLabels,
		RequiredMemory: req.RequiredMemory,
		DistillOutputs: req.DistillOutputs,
		MaxConcurrency: req.MaxConcurrency,
		PolicyApprove:  req.PolicyApprove,
		IdempotencyKey: req.IdempotencyKey,
		Actor:          req.Actor,
	})
	if apiErr != nil {
		writeJSON(w, apiErr.Status, apiErr.Body)
		return
	}
	emitRemoteAuditEvent(requestID, "orchestrator_template_launch", authorized.ID, "success", map[string]interface{}{
		"templateId":           template.ID,
		"goal":                 authorized.Goal,
		"requestedProvider":    authorized.RequestedProvider,
		"effectiveConcurrency": authorized.Policy.EffectiveMaxConcurrency,
	})
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"template":  template,
		"execution": authorized,
	})
}

type gatewayAPIResponseError struct {
	Status int
	Body   map[string]interface{}
}

func launchExecutionTemplate(requestID string, cfg *GatewayConfig, opts executionTemplateLaunchOptions) (OrchestratorExecution, *gatewayAPIResponseError) {
	templateID := strings.TrimSpace(opts.TemplateID)
	if templateID == "" {
		return OrchestratorExecution{}, &gatewayAPIResponseError{Status: http.StatusBadRequest, Body: gatewayErrBody("E_USAGE", "template id is required")}
	}
	resolved, err := orchestration.ResolveExecutionTemplate(templateID, opts.Inputs)
	if err != nil {
		return OrchestratorExecution{}, &gatewayAPIResponseError{Status: http.StatusBadRequest, Body: gatewayErrBody("E_USAGE", err.Error())}
	}
	plan, err := orchestration.BuildPlan(orchestration.BuildPlanInput{
		Goal:           resolved.Goal,
		TemplateID:     templateID,
		Provider:       strings.TrimSpace(opts.Provider),
		HostIDs:        opts.HostIDs,
		HostLabels:     opts.HostLabels,
		RequiredMemory: opts.RequiredMemory,
		DistillOutputs: opts.DistillOutputs,
		MaxConcurrency: opts.MaxConcurrency,
		Tasks:          resolved.Tasks,
	})
	if err != nil {
		return OrchestratorExecution{}, &gatewayAPIResponseError{Status: http.StatusBadRequest, Body: gatewayErrBody("E_USAGE", err.Error())}
	}
	created, apiErr := createTemplateExecutionRecord(requestID, cfg, plan, opts.IdempotencyKey, opts.Metadata)
	if apiErr != nil {
		return OrchestratorExecution{}, apiErr
	}
	return authorizeTemplateExecutionRecord(requestID, cfg, created, opts.Actor, opts.PolicyApprove, opts.MaxConcurrency)
}

func createTemplateExecutionRecord(requestID string, cfg *GatewayConfig, plan orchestration.Plan, idempotencyKey string, metadata executionLaunchMetadata) (OrchestratorExecution, *gatewayAPIResponseError) {
	req := OrchestratorExecution{
		Goal:                 strings.TrimSpace(plan.Goal),
		TemplateID:           strings.TrimSpace(plan.TemplateID),
		TriggerSource:        strings.TrimSpace(metadata.TriggerSource),
		TriggerID:            strings.TrimSpace(metadata.TriggerID),
		TriggerEvent:         strings.TrimSpace(metadata.TriggerEvent),
		TriggerPayloadDigest: strings.TrimSpace(metadata.TriggerPayloadDigest),
		Initiator:            strings.TrimSpace(metadata.Initiator),
		RequestedProvider:    strings.TrimSpace(plan.Provider),
		RequiredMemory:       append([]string(nil), plan.RequiredMemory...),
		MemoryContractDigest: buildMemoryContractDigest(plan.RequiredMemory),
		MemoryProvenance:     append([]string(nil), plan.RequiredMemory...),
		AgentLifecycleMode:   orchestratorAgentLifecycleMode,
		MemoryBindingMode:    orchestratorMemoryBindingMode,
		SourceScopes:         append([]string(nil), plan.RequiredMemory...),
		SnapshotDigest:       buildMemoryContractDigest(plan.RequiredMemory),
		DistillOutputs:       append([]string(nil), plan.DistillOutputs...),
		IdempotencyKey:       strings.TrimSpace(idempotencyKey),
		ApprovalScope:        strings.TrimSpace(plan.ApprovalScope),
		RequiredWorkers:      make([]OrchestratorRequiredWorker, 0, len(plan.RequiredWorkers)),
		TaskUnits:            make([]OrchestratorTaskUnit, 0, len(plan.TaskUnits)),
		MaxConcurrency:       plan.MaxConcurrency,
	}
	for _, worker := range plan.RequiredWorkers {
		req.RequiredWorkers = append(req.RequiredWorkers, OrchestratorRequiredWorker{
			HostID:     worker.HostID,
			HostLabels: append([]string(nil), worker.HostLabels...),
			AgentID:    worker.AgentID,
			Count:      worker.Count,
		})
	}
	for _, task := range plan.TaskUnits {
		req.TaskUnits = append(req.TaskUnits, OrchestratorTaskUnit{
			ID:          task.ID,
			Input:       task.Input,
			TimeoutMs:   task.TimeoutMs,
			RetryBudget: task.RetryBudget,
			HostID:      task.HostID,
			HostLabels:  append([]string(nil), task.HostLabels...),
			AgentID:     task.AgentID,
		})
	}
	normalized, err := normalizeOrchestratorExecution(req)
	if err != nil {
		return OrchestratorExecution{}, &gatewayAPIResponseError{Status: http.StatusBadRequest, Body: gatewayErrBody("E_USAGE", err.Error())}
	}
	normalized = resetOrchestratorDelegatedMemoryProgress(normalized)
	if normalized.IdempotencyKey != "" {
		existing, ok, findErr := findOrchestratorExecutionByIdempotencyKey(normalized.IdempotencyKey)
		if findErr != nil {
			return OrchestratorExecution{}, &gatewayAPIResponseError{Status: http.StatusInternalServerError, Body: gatewayErrBody("E_INTERNAL", "failed to lookup idempotency key")}
		}
		if ok {
			return existing, nil
		}
	}
	now := nowTimestamp()
	normalized.ID = uuid.NewString()
	normalized.Status = OrchestratorExecutionStatusPendingAuthorization
	normalized.Authorization = OrchestratorAuthorization{}
	normalized.Results = []OrchestratorTaskResult{}
	normalized.CreatedAt = now
	normalized.UpdatedAt = now
	normalized.Error = ""
	if effectiveGatewayFeatureFlags(cfg).ProviderBindingEnabled {
		resolutions, resolveErr := resolveProviderGovernanceForWorkers(normalized.RequiredWorkers)
		if resolveErr != nil {
			return OrchestratorExecution{}, &gatewayAPIResponseError{Status: http.StatusInternalServerError, Body: gatewayErrBody("E_INTERNAL", "failed to resolve execution governance")}
		}
		normalized.Governance = OrchestratorExecutionGovernance{ProviderResolutions: resolutions}
	}
	policyRules, policyErr := listOrchestratorPolicies()
	if policyErr != nil {
		return OrchestratorExecution{}, &gatewayAPIResponseError{Status: http.StatusInternalServerError, Body: gatewayErrBody("E_INTERNAL", "failed to evaluate execution policy")}
	}
	remoteHosts, remoteHostsErr := listRemoteHosts()
	if remoteHostsErr != nil {
		return OrchestratorExecution{}, &gatewayAPIResponseError{Status: http.StatusInternalServerError, Body: gatewayErrBody("E_INTERNAL", "failed to load remote hosts for policy evaluation")}
	}
	normalized = applyOrchestratorExecutionPolicy(normalized, policyRules, remoteHosts)
	if normalized.Policy.Decision == orchestratorPolicyDecisionDeny {
		return OrchestratorExecution{}, &gatewayAPIResponseError{
			Status: http.StatusForbidden,
			Body: map[string]interface{}{
				"requestId": requestID,
				"result":    "error",
				"error": map[string]interface{}{
					"code":    "E_POLICY_DENY",
					"message": firstNonEmptyPolicyValue(normalized.Policy.Reason, "execution denied by policy"),
				},
				"policy": normalized.Policy,
			},
		}
	}
	saved, saveErr := upsertOrchestratorExecution(normalized)
	if saveErr != nil {
		return OrchestratorExecution{}, &gatewayAPIResponseError{Status: http.StatusInternalServerError, Body: gatewayErrBody("E_INTERNAL", "failed to save orchestrator execution")}
	}
	emitRemoteAuditEvent(requestID, "orchestrator_execution_create", saved.ID, "success", map[string]interface{}{
		"goal":                    saved.Goal,
		"templateId":              saved.TemplateID,
		"requestedProvider":       saved.RequestedProvider,
		"workerCount":             len(saved.RequiredWorkers),
		"resolutionCount":         len(saved.Governance.ProviderResolutions),
		"policyDecision":          saved.Policy.Decision,
		"toolMode":                saved.Policy.ToolPolicy.Mode,
		"effectiveMaxConcurrency": saved.Policy.EffectiveMaxConcurrency,
	})
	return saved, nil
}

func authorizeTemplateExecutionRecord(requestID string, cfg *GatewayConfig, execution OrchestratorExecution, actor string, policyApprove bool, maxConcurrency int) (OrchestratorExecution, *gatewayAPIResponseError) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "operator"
	}
	if maxConcurrency > 0 {
		execution.MaxConcurrency = maxConcurrency
	}
	if execution.MaxConcurrency <= 0 {
		execution.MaxConcurrency = defaultOrchestratorMaxConcurrency
	}
	if execution.MaxConcurrency > 64 {
		execution.MaxConcurrency = 64
	}
	if execution.Policy.Decision == orchestratorPolicyDecisionDeny {
		return OrchestratorExecution{}, &gatewayAPIResponseError{
			Status: http.StatusForbidden,
			Body: map[string]interface{}{
				"requestId": requestID,
				"result":    "error",
				"execution": execution,
				"error": map[string]interface{}{
					"code":    "E_POLICY_DENY",
					"message": firstNonEmptyPolicyValue(execution.Policy.Reason, "execution denied by policy"),
				},
			},
		}
	}
	if execution.Policy.Decision == orchestratorPolicyDecisionAsk && !policyApprove {
		return OrchestratorExecution{}, &gatewayAPIResponseError{
			Status: http.StatusConflict,
			Body: map[string]interface{}{
				"requestId": requestID,
				"result":    "error",
				"execution": execution,
				"error": map[string]interface{}{
					"code":    "E_POLICY_APPROVAL_REQUIRED",
					"message": firstNonEmptyPolicyValue(execution.Policy.Reason, "policy approval required before execution can run"),
				},
			},
		}
	}
	if isOrchestratorExecutionTerminal(execution.Status) {
		return execution, nil
	}
	execution.Authorization = OrchestratorAuthorization{
		InfrastructureApproved: true,
		ApprovedBy:             actor,
		ApprovedAt:             nowTimestamp(),
	}
	if execution.Policy.Decision == orchestratorPolicyDecisionAsk && policyApprove {
		execution.Policy.ApprovedBy = actor
		execution.Policy.ApprovedAt = nowTimestamp()
	}
	if execution.StartedAt == "" {
		execution.StartedAt = nowTimestamp()
	}
	execution.Status = OrchestratorExecutionStatusProvisioning
	execution.UpdatedAt = nowTimestamp()
	execution.Error = ""
	updated, saveErr := upsertOrchestratorExecution(execution)
	if saveErr != nil {
		return OrchestratorExecution{}, &gatewayAPIResponseError{Status: http.StatusInternalServerError, Body: gatewayErrBody("E_INTERNAL", "failed to update orchestrator execution")}
	}
	emitRemoteAuditEvent(requestID, "orchestrator_execution_authorize", updated.ID, "success", map[string]interface{}{
		"actor":                   actor,
		"templateId":              updated.TemplateID,
		"maxConcurrency":          updated.MaxConcurrency,
		"policyDecision":          updated.Policy.Decision,
		"effectiveMaxConcurrency": updated.Policy.EffectiveMaxConcurrency,
		"policyApproved":          policyApprove,
	})
	orchestratorLaunchExecutionFn(updated.ID)
	return updated, nil
}
