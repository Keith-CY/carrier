package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	defaultOrchestratorWorkerIdleTTL  = 10 * time.Minute
	defaultOrchestratorMaxConcurrency = 8
	orchestratorLocalHostID           = "local"
)

var orchestratorExecutionRunState = struct {
	mu      sync.Mutex
	running map[string]bool
}{
	running: map[string]bool{},
}

type orchestratorExecutionControl struct {
	cancel         context.CancelFunc
	mu             sync.Mutex
	pauseRequested bool
	paused         bool
	resumeCh       chan struct{}
}

var (
	orchestratorListLeasesByExecution = listOrchestratorWorkerLeasesByExecution
	orchestratorInstallAgent          = remoteInstallAgent
	orchestratorLocalDaemonClientFn   = newOrchestratorLocalDaemonClientFromEnv
	orchestratorLaunchExecutionFn     = startOrchestratorExecutionAsync
)

var orchestratorExecutionControlState = struct {
	mu       sync.Mutex
	controls map[string]*orchestratorExecutionControl
}{
	controls: map[string]*orchestratorExecutionControl{},
}

func newOrchestratorLocalDaemonClientFromEnv() *DaemonClient {
	baseURL := strings.TrimSpace(os.Getenv("CARRIER_DAEMON_BASE_URL"))
	if baseURL == "" {
		baseURL = defaultDaemonBaseURL
	}
	token := strings.TrimSpace(os.Getenv("CARRIER_SERVER_API_TOKEN"))
	timeout := defaultDaemonTimeout
	if raw := strings.TrimSpace(os.Getenv("CARRIER_DAEMON_TIMEOUT_MS")); raw != "" {
		if millis, err := strconv.Atoi(raw); err == nil && millis > 0 {
			timeout = time.Duration(millis) * time.Millisecond
		}
	}
	return NewDaemonClient(baseURL, token, timeout)
}

func logOrchestratorPersistError(context string, err error) {
	if err == nil {
		return
	}
	log.Printf("[gateway] %s: %s", strings.TrimSpace(context), RedactErrorMessage(err.Error()))
}

type orchestratorWorkerPool struct {
	key string
	ch  chan OrchestratorWorkerLease
}

func isOrchestratorExecutionTerminal(status OrchestratorExecutionStatus) bool {
	switch status {
	case OrchestratorExecutionStatusCompleted,
		OrchestratorExecutionStatusPartialCompleted,
		OrchestratorExecutionStatusFailed,
		OrchestratorExecutionStatusRetryableFailed,
		OrchestratorExecutionStatusCancelled,
		OrchestratorExecutionStatusDeclined:
		return true
	default:
		return false
	}
}

func registerOrchestratorExecutionCancel(executionID string, cancel context.CancelFunc) {
	id := strings.TrimSpace(executionID)
	if id == "" || cancel == nil {
		return
	}
	orchestratorExecutionControlState.mu.Lock()
	defer orchestratorExecutionControlState.mu.Unlock()
	orchestratorExecutionControlState.controls[id] = &orchestratorExecutionControl{
		cancel:   cancel,
		resumeCh: make(chan struct{}),
	}
}

func unregisterOrchestratorExecutionCancel(executionID string) {
	id := strings.TrimSpace(executionID)
	if id == "" {
		return
	}
	orchestratorExecutionControlState.mu.Lock()
	defer orchestratorExecutionControlState.mu.Unlock()
	delete(orchestratorExecutionControlState.controls, id)
}

func cancelOrchestratorExecutionRun(executionID string) bool {
	id := strings.TrimSpace(executionID)
	if id == "" {
		return false
	}
	orchestratorExecutionControlState.mu.Lock()
	control, ok := orchestratorExecutionControlState.controls[id]
	orchestratorExecutionControlState.mu.Unlock()
	if !ok || control == nil {
		return false
	}
	cancel := control.cancel
	if ok && cancel != nil {
		cancel()
	}
	return ok
}

func getOrchestratorExecutionControl(executionID string) *orchestratorExecutionControl {
	orchestratorExecutionControlState.mu.Lock()
	defer orchestratorExecutionControlState.mu.Unlock()
	return orchestratorExecutionControlState.controls[strings.TrimSpace(executionID)]
}

func requestOrchestratorExecutionPause(executionID string) bool {
	control := getOrchestratorExecutionControl(executionID)
	if control == nil {
		return false
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	if control.pauseRequested {
		return true
	}
	control.pauseRequested = true
	control.resumeCh = make(chan struct{})
	return true
}

func resumeOrchestratorExecutionRun(executionID string) bool {
	control := getOrchestratorExecutionControl(executionID)
	if control == nil {
		return false
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	if !control.pauseRequested && !control.paused {
		return false
	}
	control.pauseRequested = false
	control.paused = false
	select {
	case <-control.resumeCh:
	default:
		close(control.resumeCh)
	}
	control.resumeCh = make(chan struct{})
	return true
}

func waitIfOrchestratorExecutionPaused(ctx context.Context, executionID string) error {
	control := getOrchestratorExecutionControl(executionID)
	if control == nil {
		return nil
	}
	for {
		control.mu.Lock()
		if !control.pauseRequested {
			control.mu.Unlock()
			return nil
		}
		resumeCh := control.resumeCh
		shouldMarkPaused := !control.paused
		if shouldMarkPaused {
			control.paused = true
		}
		control.mu.Unlock()

		if shouldMarkPaused {
			if err := markOrchestratorExecutionPaused(executionID); err != nil {
				logOrchestratorPersistError("mark orchestrator execution paused", err)
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-resumeCh:
			if err := markOrchestratorExecutionResumed(executionID); err != nil {
				logOrchestratorPersistError("mark orchestrator execution resumed", err)
			}
		}
	}
}

func handleOrchestratorExecutions(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	trimmed := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/orchestrator/executions"), "/")
	if trimmed == "" {
		switch r.Method {
		case http.MethodGet:
			if _, ok := requireGatewayPermission(w, r, cfg, canViewExecutions, "E_RBAC_EXECUTION_VIEW", "role cannot view orchestrator executions"); !ok {
				return
			}
			executions, err := listOrchestratorExecutions()
			if err != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to list orchestrator executions", "list orchestrator executions", err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"requestId":  requestID,
				"result":     "ok",
				"executions": executions,
			})
			return
		case http.MethodPost:
			if _, ok := requireGatewayPermission(w, r, cfg, canLaunchExecutions, "E_RBAC_EXECUTION_LAUNCH", "role cannot launch orchestrator executions"); !ok {
				return
			}
			var req OrchestratorExecution
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
				return
			}
			normalized, err := normalizeOrchestratorExecution(req)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
				return
			}
			if normalized.IdempotencyKey != "" {
				existing, ok, findErr := findOrchestratorExecutionByIdempotencyKey(normalized.IdempotencyKey)
				if findErr != nil {
					writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to lookup idempotency key", "find orchestrator execution by idempotency key", findErr)
					return
				}
				if ok {
					writeJSON(w, http.StatusOK, map[string]interface{}{
						"requestId": requestID,
						"result":    "ok",
						"execution": existing,
					})
					return
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
					writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to resolve execution governance", "resolve execution provider governance", resolveErr)
					return
				}
				normalized.Governance = OrchestratorExecutionGovernance{
					ProviderResolutions: resolutions,
				}
			}
			policyRules, policyErr := listOrchestratorPolicies()
			if policyErr != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to evaluate execution policy", "list orchestrator policies", policyErr)
				return
			}
			remoteHosts, remoteHostsErr := listRemoteHosts()
			if remoteHostsErr != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load remote hosts for policy evaluation", "list remote hosts", remoteHostsErr)
				return
			}
			normalized = applyOrchestratorExecutionPolicy(normalized, policyRules, remoteHosts)
			if normalized.Policy.Decision == orchestratorPolicyDecisionDeny {
				emitRemoteAuditEvent(requestID, "orchestrator_execution_policy_deny", normalized.ID, "blocked", map[string]interface{}{
					"goal":              normalized.Goal,
					"requestedProvider": normalized.RequestedProvider,
					"policyReason":      normalized.Policy.Reason,
					"matchedRuleId":     normalized.Policy.MatchedRuleID,
					"matchedRuleName":   normalized.Policy.MatchedRuleName,
				})
				writeJSON(w, http.StatusForbidden, map[string]interface{}{
					"requestId": requestID,
					"result":    "error",
					"error": map[string]interface{}{
						"code":    "E_POLICY_DENY",
						"message": firstNonEmptyPolicyValue(normalized.Policy.Reason, "execution denied by policy"),
					},
					"policy": normalized.Policy,
				})
				return
			}

			saved, saveErr := upsertOrchestratorExecution(normalized)
			if saveErr != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to save orchestrator execution", "upsert orchestrator execution", saveErr)
				return
			}
			emitRemoteAuditEvent(requestID, "orchestrator_execution_create", saved.ID, "success", map[string]interface{}{
				"goal":                    saved.Goal,
				"requestedProvider":       saved.RequestedProvider,
				"workerCount":             len(saved.RequiredWorkers),
				"resolutionCount":         len(saved.Governance.ProviderResolutions),
				"policyDecision":          saved.Policy.Decision,
				"toolMode":                saved.Policy.ToolPolicy.Mode,
				"effectiveMaxConcurrency": saved.Policy.EffectiveMaxConcurrency,
			})
			writeJSON(w, http.StatusCreated, map[string]interface{}{
				"requestId": requestID,
				"result":    "ok",
				"execution": saved,
			})
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
	}

	parts := strings.Split(trimmed, "/")
	executionID := strings.TrimSpace(parts[0])
	if executionID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "execution id is required"))
		return
	}
	execution, ok, err := getOrchestratorExecution(executionID)
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load orchestrator execution", "get orchestrator execution", err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", fmt.Sprintf("orchestrator execution %s not found", executionID)))
		return
	}

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		if _, ok := requireGatewayPermission(w, r, cfg, canViewExecutions, "E_RBAC_EXECUTION_VIEW", "role cannot view orchestrator executions"); !ok {
			return
		}
		leases, leaseErr := orchestratorListLeasesByExecution(executionID)
		if leaseErr != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load orchestrator worker leases", "list orchestrator worker leases by execution", leaseErr)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"execution": execution,
			"workers":   leases,
		})
		return
	}

	action := strings.ToLower(strings.TrimSpace(parts[1]))
	if action == "artifacts" {
		if _, ok := requireGatewayPermission(w, r, cfg, canViewExecutions, "E_RBAC_EXECUTION_VIEW", "role cannot view orchestrator execution artifacts"); !ok {
			return
		}
		handleOrchestratorExecutionArtifacts(w, r, requestID, cfg, execution, parts)
		return
	}
	if action == "evidence" {
		if _, ok := requireGatewayPermission(w, r, cfg, canViewExecutions, "E_RBAC_EXECUTION_VIEW", "role cannot export orchestrator execution evidence"); !ok {
			return
		}
		handleOrchestratorExecutionEvidence(w, r, requestID, cfg, execution, parts)
		return
	}
	switch action {
	case "authorize":
		if _, ok := requireGatewayPermission(w, r, cfg, canApproveExecutions, "E_RBAC_EXECUTION_APPROVE", "role cannot approve orchestrator executions"); !ok {
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var req struct {
			Approved       bool   `json:"approved"`
			Actor          string `json:"actor"`
			MaxConcurrency int    `json:"maxConcurrency,omitempty"`
			PolicyApproved bool   `json:"policyApproved,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
			return
		}
		actor := strings.TrimSpace(req.Actor)
		if actor == "" {
			actor = "operator"
		}
		if !req.Approved {
			execution.Authorization = OrchestratorAuthorization{
				InfrastructureApproved: false,
				ApprovedBy:             actor,
				ApprovedAt:             nowTimestamp(),
			}
			execution.Status = OrchestratorExecutionStatusDeclined
			execution.CompletedAt = nowTimestamp()
			execution.UpdatedAt = nowTimestamp()
			execution.Error = "infrastructure authorization declined"
			updated, saveErr := upsertOrchestratorExecution(execution)
			if saveErr != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to update orchestrator execution", "upsert orchestrator execution declined", saveErr)
				return
			}
			emitRemoteAuditEvent(requestID, "orchestrator_execution_authorize", updated.ID, "declined", map[string]interface{}{
				"actor": actor,
			})
			publishOrchestratorExecutionEvent(updated)
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"requestId": requestID,
				"result":    "ok",
				"execution": updated,
			})
			return
		}

		if req.MaxConcurrency > 0 {
			execution.MaxConcurrency = req.MaxConcurrency
		}
		if execution.MaxConcurrency <= 0 {
			execution.MaxConcurrency = defaultOrchestratorMaxConcurrency
		}
		if execution.MaxConcurrency > 64 {
			execution.MaxConcurrency = 64
		}
		if execution.Policy.Decision == orchestratorPolicyDecisionDeny {
			writeJSON(w, http.StatusForbidden, map[string]interface{}{
				"requestId": requestID,
				"result":    "error",
				"execution": execution,
				"error": map[string]interface{}{
					"code":    "E_POLICY_DENY",
					"message": firstNonEmptyPolicyValue(execution.Policy.Reason, "execution denied by policy"),
				},
			})
			return
		}
		if execution.Policy.Decision == orchestratorPolicyDecisionAsk && !req.PolicyApproved {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"requestId": requestID,
				"result":    "error",
				"execution": execution,
				"error": map[string]interface{}{
					"code":    "E_POLICY_APPROVAL_REQUIRED",
					"message": firstNonEmptyPolicyValue(execution.Policy.Reason, "policy approval required before execution can run"),
				},
			})
			return
		}
		if isOrchestratorExecutionTerminal(execution.Status) {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"requestId": requestID,
				"result":    "ok",
				"execution": execution,
			})
			return
		}
		execution.Authorization = OrchestratorAuthorization{
			InfrastructureApproved: true,
			ApprovedBy:             actor,
			ApprovedAt:             nowTimestamp(),
		}
		if execution.Policy.Decision == orchestratorPolicyDecisionAsk && req.PolicyApproved {
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
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to update orchestrator execution", "upsert orchestrator execution authorize", saveErr)
			return
		}
		emitRemoteAuditEvent(requestID, "orchestrator_execution_authorize", updated.ID, "success", map[string]interface{}{
			"actor":                   actor,
			"maxConcurrency":          updated.MaxConcurrency,
			"policyDecision":          updated.Policy.Decision,
			"effectiveMaxConcurrency": updated.Policy.EffectiveMaxConcurrency,
			"policyApproved":          req.PolicyApproved,
		})
		orchestratorLaunchExecutionFn(updated.ID)
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"execution": updated,
		})
		return
	case "pause":
		if _, ok := requireGatewayPermission(w, r, cfg, canLaunchExecutions, "E_RBAC_EXECUTION_LAUNCH", "role cannot pause orchestrator executions"); !ok {
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var req struct {
			Actor string `json:"actor"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
			return
		}
		updated, pauseErr := pauseOrchestratorExecution(execution.ID, req.Actor)
		if pauseErr != nil {
			writeInternalGatewayError(w, http.StatusConflict, "E_ORCHESTRATOR_PAUSE_FAILED", pauseErr.Error(), "pause orchestrator execution", pauseErr)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"execution": updated,
		})
		return
	case "resume":
		if _, ok := requireGatewayPermission(w, r, cfg, canLaunchExecutions, "E_RBAC_EXECUTION_LAUNCH", "role cannot resume orchestrator executions"); !ok {
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var req struct {
			Actor string `json:"actor"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
			return
		}
		updated, resumeErr := resumeOrchestratorExecution(execution.ID, req.Actor)
		if resumeErr != nil {
			writeInternalGatewayError(w, http.StatusConflict, "E_ORCHESTRATOR_RESUME_FAILED", resumeErr.Error(), "resume orchestrator execution", resumeErr)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"execution": updated,
		})
		return
	case "cancel":
		if _, ok := requireGatewayPermission(w, r, cfg, canLaunchExecutions, "E_RBAC_EXECUTION_LAUNCH", "role cannot cancel orchestrator executions"); !ok {
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var req struct {
			Actor string `json:"actor"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
			return
		}
		if isOrchestratorExecutionTerminal(execution.Status) {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"requestId": requestID,
				"result":    "ok",
				"execution": execution,
			})
			return
		}
		updated, cancelErr := cancelOrchestratorExecution(execution.ID, req.Actor)
		if cancelErr != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to cancel orchestrator execution", "cancel orchestrator execution", cancelErr)
			return
		}
		emitRemoteAuditEvent(requestID, "orchestrator_execution_cancel", updated.ID, "success", map[string]interface{}{
			"actor": strings.TrimSpace(req.Actor),
		})
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"execution": updated,
		})
		return
	case "retry", "rerun", "clone":
		if _, ok := requireGatewayPermission(w, r, cfg, canLaunchExecutions, "E_RBAC_EXECUTION_LAUNCH", "role cannot relaunch orchestrator executions"); !ok {
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var (
			derived  OrchestratorExecution
			buildErr error
		)
		switch action {
		case "retry":
			derived, buildErr = buildRetryExecution(execution)
		case "rerun":
			derived = buildRerunExecution(execution)
		default:
			derived = buildCloneExecution(execution)
		}
		if buildErr != nil {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"requestId": requestID,
				"result":    "error",
				"error": map[string]interface{}{
					"code":    "E_ORCHESTRATOR_RETRY_NOTHING",
					"message": buildErr.Error(),
				},
			})
			return
		}
		normalized, err := normalizeOrchestratorExecution(derived)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
			return
		}
		now := nowTimestamp()
		normalized.ID = uuid.NewString()
		normalized.Status = OrchestratorExecutionStatusPendingAuthorization
		normalized.Authorization = OrchestratorAuthorization{}
		normalized.Results = []OrchestratorTaskResult{}
		normalized.Outcome = OrchestratorExecutionOutcome{}
		normalized.CreatedAt = now
		normalized.StartedAt = ""
		normalized.CompletedAt = ""
		normalized.UpdatedAt = now
		normalized.Error = ""
		saved, saveErr := upsertOrchestratorExecution(normalized)
		if saveErr != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to save derived orchestrator execution", "upsert derived orchestrator execution", saveErr)
			return
		}
		emitRemoteAuditEvent(requestID, "orchestrator_execution_"+action, saved.ID, "success", map[string]interface{}{
			"sourceExecutionId": execution.ID,
			"launchReason":      saved.LaunchReason,
			"taskCount":         len(saved.TaskUnits),
		})
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"execution": saved,
		})
		return
	default:
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported orchestrator execution action"))
		return
	}
}

func handleOrchestratorWorkersReclaim(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	if _, ok := requireGatewayPermission(w, r, cfg, canManageHosts, "E_RBAC_HOST_MANAGE", "role cannot reclaim orchestrator workers"); !ok {
		return
	}
	var req struct {
		Force      bool `json:"force"`
		IdleTTLSec int  `json:"idleTtlSec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
		return
	}
	idleTTL := defaultOrchestratorWorkerIdleTTL
	if req.IdleTTLSec > 0 {
		idleTTL = time.Duration(req.IdleTTLSec) * time.Second
	}
	summary, err := reclaimOrchestratorWorkers(context.Background(), req.Force, idleTTL)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_REMOTE_RECLAIM_FAILED", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"reclaim":   summary,
	})
}

func startOrchestratorExecutionAsync(executionID string) {
	id := strings.TrimSpace(executionID)
	if id == "" {
		return
	}
	orchestratorExecutionRunState.mu.Lock()
	if orchestratorExecutionRunState.running[id] {
		orchestratorExecutionRunState.mu.Unlock()
		return
	}
	orchestratorExecutionRunState.running[id] = true
	orchestratorExecutionRunState.mu.Unlock()

	go func() {
		defer func() {
			orchestratorExecutionRunState.mu.Lock()
			delete(orchestratorExecutionRunState.running, id)
			orchestratorExecutionRunState.mu.Unlock()
		}()
		runOrchestratorExecution(id)
	}()
}

func cancelOrchestratorExecution(executionID string, actor string) (OrchestratorExecution, error) {
	execution, ok, err := getOrchestratorExecution(executionID)
	if err != nil {
		return OrchestratorExecution{}, err
	}
	if !ok {
		return OrchestratorExecution{}, fmt.Errorf("orchestrator execution %s not found", strings.TrimSpace(executionID))
	}
	if isOrchestratorExecutionTerminal(execution.Status) {
		return execution, nil
	}

	cancelActor := strings.TrimSpace(actor)
	if cancelActor == "" {
		cancelActor = "operator"
	}
	cancelOrchestratorExecutionRun(execution.ID)

	execution.Status = OrchestratorExecutionStatusCancelled
	if execution.StartedAt == "" {
		execution.StartedAt = nowTimestamp()
	}
	if execution.CompletedAt == "" {
		execution.CompletedAt = nowTimestamp()
	}
	execution.UpdatedAt = nowTimestamp()
	execution.Error = "execution cancelled by " + cancelActor
	updated, err := upsertOrchestratorExecution(execution)
	if err != nil {
		return OrchestratorExecution{}, err
	}
	publishOrchestratorExecutionEvent(updated)
	if _, reclaimErr := reclaimExecutionLeases(context.Background(), updated.ID, true); reclaimErr != nil {
		logOrchestratorPersistError("reclaim execution leases on cancel", reclaimErr)
	}
	return updated, nil
}

func pauseOrchestratorExecution(executionID string, actor string) (OrchestratorExecution, error) {
	execution, ok, err := getOrchestratorExecution(executionID)
	if err != nil {
		return OrchestratorExecution{}, err
	}
	if !ok {
		return OrchestratorExecution{}, fmt.Errorf("orchestrator execution %s not found", strings.TrimSpace(executionID))
	}
	if isOrchestratorExecutionTerminal(execution.Status) {
		return execution, nil
	}
	switch execution.Status {
	case OrchestratorExecutionStatusPaused, OrchestratorExecutionStatusPauseRequested:
		return execution, nil
	}
	if !requestOrchestratorExecutionPause(execution.ID) {
		return OrchestratorExecution{}, fmt.Errorf("orchestrator execution %s is not actively running on this gateway", strings.TrimSpace(executionID))
	}
	execution.Status = OrchestratorExecutionStatusPauseRequested
	execution.UpdatedAt = nowTimestamp()
	if strings.TrimSpace(actor) != "" {
		execution.Error = "pause requested by " + strings.TrimSpace(actor)
	}
	updated, err := upsertOrchestratorExecution(execution)
	if err != nil {
		return OrchestratorExecution{}, err
	}
	return updated, nil
}

func resumeOrchestratorExecution(executionID string, actor string) (OrchestratorExecution, error) {
	execution, ok, err := getOrchestratorExecution(executionID)
	if err != nil {
		return OrchestratorExecution{}, err
	}
	if !ok {
		return OrchestratorExecution{}, fmt.Errorf("orchestrator execution %s not found", strings.TrimSpace(executionID))
	}
	if isOrchestratorExecutionTerminal(execution.Status) {
		return execution, nil
	}
	if !resumeOrchestratorExecutionRun(execution.ID) {
		return OrchestratorExecution{}, fmt.Errorf("orchestrator execution %s is not paused on this gateway", strings.TrimSpace(executionID))
	}
	execution.Status = OrchestratorExecutionStatusRunning
	execution.UpdatedAt = nowTimestamp()
	execution.Error = ""
	updated, err := upsertOrchestratorExecution(execution)
	if err != nil {
		return OrchestratorExecution{}, err
	}
	return updated, nil
}

func markOrchestratorExecutionPaused(executionID string) error {
	execution, ok, err := getOrchestratorExecution(executionID)
	if err != nil {
		return err
	}
	if !ok || isOrchestratorExecutionTerminal(execution.Status) || execution.Status == OrchestratorExecutionStatusPaused {
		return nil
	}
	execution.Status = OrchestratorExecutionStatusPaused
	execution.UpdatedAt = nowTimestamp()
	if _, err = upsertOrchestratorExecution(execution); err != nil {
		return err
	}
	return appendIntegrationEventByOrchestratorExecution(executionID, "execution.paused", map[string]interface{}{
		"status": execution.Status,
	})
}

func markOrchestratorExecutionResumed(executionID string) error {
	execution, ok, err := getOrchestratorExecution(executionID)
	if err != nil {
		return err
	}
	if !ok || isOrchestratorExecutionTerminal(execution.Status) {
		return nil
	}
	if execution.Status != OrchestratorExecutionStatusPaused && execution.Status != OrchestratorExecutionStatusPauseRequested {
		return nil
	}
	execution.Status = OrchestratorExecutionStatusRunning
	execution.UpdatedAt = nowTimestamp()
	execution.Error = ""
	_, err = upsertOrchestratorExecution(execution)
	return err
}

func buildRetryExecution(source OrchestratorExecution) (OrchestratorExecution, error) {
	failedTaskIDs := map[string]struct{}{}
	for _, result := range source.Results {
		if result.Status == OrchestratorTaskStatusFailed {
			failedTaskIDs[strings.TrimSpace(result.TaskID)] = struct{}{}
		}
	}
	if len(failedTaskIDs) == 0 {
		return OrchestratorExecution{}, fmt.Errorf("execution has no failed tasks to retry")
	}
	tasks := make([]OrchestratorTaskUnit, 0, len(source.TaskUnits))
	for _, task := range source.TaskUnits {
		if _, ok := failedTaskIDs[strings.TrimSpace(task.ID)]; ok {
			tasks = append(tasks, task)
		}
	}
	if len(tasks) == 0 {
		return OrchestratorExecution{}, fmt.Errorf("execution has no failed task units to retry")
	}
	derived := buildDerivedExecution(source, "retry_failed_tasks")
	derived.TaskUnits = tasks
	derived.RequiredWorkers = selectRequiredWorkersForTasks(source.RequiredWorkers, tasks)
	return derived, nil
}

func buildRerunExecution(source OrchestratorExecution) OrchestratorExecution {
	return buildDerivedExecution(source, "rerun_execution")
}

func buildCloneExecution(source OrchestratorExecution) OrchestratorExecution {
	return buildDerivedExecution(source, "clone_execution")
}

func buildDerivedExecution(source OrchestratorExecution, launchReason string) OrchestratorExecution {
	derived := source
	derived.ID = ""
	derived.IdempotencyKey = ""
	derived.ParentExecutionID = strings.TrimSpace(source.ID)
	derived.SourceExecutionID = sourceExecutionID(source)
	derived.LaunchReason = strings.TrimSpace(launchReason)
	derived.Authorization = OrchestratorAuthorization{}
	derived.Policy = resetDerivedExecutionPolicyApproval(source.Policy)
	derived.Results = []OrchestratorTaskResult{}
	derived.Outcome = OrchestratorExecutionOutcome{}
	derived.Error = ""
	derived.Status = OrchestratorExecutionStatusPendingAuthorization
	derived.CreatedAt = ""
	derived.StartedAt = ""
	derived.CompletedAt = ""
	derived.UpdatedAt = ""
	derived.RequiredWorkers = append([]OrchestratorRequiredWorker(nil), source.RequiredWorkers...)
	derived.TaskUnits = append([]OrchestratorTaskUnit(nil), source.TaskUnits...)
	return derived
}

func sourceExecutionID(source OrchestratorExecution) string {
	if existing := strings.TrimSpace(source.SourceExecutionID); existing != "" {
		return existing
	}
	return strings.TrimSpace(source.ID)
}

func resetDerivedExecutionPolicyApproval(policy OrchestratorExecutionPolicySnapshot) OrchestratorExecutionPolicySnapshot {
	out := policy
	out.ApprovedBy = ""
	out.ApprovedAt = ""
	return out
}

func selectRequiredWorkersForTasks(workers []OrchestratorRequiredWorker, tasks []OrchestratorTaskUnit) []OrchestratorRequiredWorker {
	targets := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		targets[orchestratorTargetKey(task.HostID, task.HostLabels, task.AgentID)] = struct{}{}
	}
	selected := make([]OrchestratorRequiredWorker, 0, len(workers))
	for _, worker := range workers {
		if _, ok := targets[orchestratorTargetKey(worker.HostID, worker.HostLabels, worker.AgentID)]; ok {
			selected = append(selected, worker)
		}
	}
	if len(selected) == 0 {
		return append([]OrchestratorRequiredWorker(nil), workers...)
	}
	return selected
}

func orchestratorTargetKey(hostID string, hostLabels []string, agentID string) string {
	return strings.ToLower(strings.TrimSpace(hostID) + "|" + strings.Join(normalizeStringSelectorList(hostLabels, true), ",") + "|" + strings.TrimSpace(agentID))
}

func runOrchestratorExecution(executionID string) {
	execution, ok, err := getOrchestratorExecution(executionID)
	if err != nil || !ok {
		return
	}
	if !execution.Authorization.InfrastructureApproved {
		return
	}
	if isOrchestratorExecutionTerminal(execution.Status) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	registerOrchestratorExecutionCancel(execution.ID, cancel)
	defer cancel()
	defer unregisterOrchestratorExecutionCancel(execution.ID)

	leases, provisionErr := provisionOrchestratorWorkers(ctx, execution)
	if provisionErr != nil {
		markOrchestratorExecutionFailed(execution, provisionErr, nil)
		return
	}

	if waitErr := waitIfOrchestratorExecutionPaused(ctx, execution.ID); waitErr != nil {
		markOrchestratorExecutionFailed(execution, waitErr, nil)
		return
	}

	execution.Status = OrchestratorExecutionStatusRunning
	execution.UpdatedAt = nowTimestamp()
	updated, saveErr := upsertOrchestratorExecution(execution)
	if saveErr != nil {
		markOrchestratorExecutionFailed(execution, saveErr, nil)
		return
	}
	if err := appendIntegrationEventByOrchestratorExecution(updated.ID, "execution.started", map[string]interface{}{
		"status": updated.Status,
	}); err != nil {
		logOrchestratorPersistError("append integration started event", err)
	}

	results, runErr := runOrchestratorTasks(ctx, updated, leases)
	if runErr != nil {
		markOrchestratorExecutionFailed(updated, runErr, results)
		if _, err := reclaimExecutionLeases(context.Background(), updated.ID, true); err != nil {
			logOrchestratorPersistError("reclaim execution leases on failure", err)
		}
		return
	}

	if _, err := reclaimExecutionLeases(context.Background(), updated.ID, true); err != nil {
		logOrchestratorPersistError("reclaim execution leases on success", err)
	}
	latest, found, latestErr := getOrchestratorExecution(updated.ID)
	if latestErr == nil && found && latest.Status == OrchestratorExecutionStatusCancelled {
		if len(results) > 0 {
			latest.Results = results
			latest.UpdatedAt = nowTimestamp()
			if _, err := upsertOrchestratorExecution(latest); err != nil {
				logOrchestratorPersistError("upsert orchestrator execution cancelled results", err)
			}
		}
		return
	}
	updated.Results = results
	updated.Status = OrchestratorExecutionStatusCompleted
	updated.CompletedAt = nowTimestamp()
	updated.UpdatedAt = updated.CompletedAt
	updated.Error = ""
	if persisted, err := upsertOrchestratorExecution(updated); err != nil {
		logOrchestratorPersistError("upsert orchestrator execution completed", err)
	} else {
		updated = persisted
	}
	publishOrchestratorExecutionEvent(updated)
}

func markOrchestratorExecutionFailed(execution OrchestratorExecution, runErr error, results []OrchestratorTaskResult) {
	latest, found, err := getOrchestratorExecution(execution.ID)
	if err == nil && found {
		execution = latest
	}
	if results != nil {
		execution.Results = results
	}
	if execution.Status == OrchestratorExecutionStatusCancelled || errors.Is(runErr, context.Canceled) {
		execution.Status = OrchestratorExecutionStatusCancelled
		if execution.Error == "" {
			execution.Error = "execution cancelled"
		}
		if execution.CompletedAt == "" {
			execution.CompletedAt = nowTimestamp()
		}
		execution.UpdatedAt = nowTimestamp()
		if _, err := upsertOrchestratorExecution(execution); err != nil {
			logOrchestratorPersistError("upsert orchestrator execution cancelled", err)
		}
		publishOrchestratorExecutionEvent(execution)
		return
	}
	execution.Status = OrchestratorExecutionStatusFailed
	execution.Error = strings.TrimSpace(runErr.Error())
	execution.UpdatedAt = nowTimestamp()
	execution.CompletedAt = execution.UpdatedAt
	if results != nil {
		execution.Results = results
	}
	if _, err := upsertOrchestratorExecution(execution); err != nil {
		logOrchestratorPersistError("upsert orchestrator execution failed", err)
	}
	publishOrchestratorExecutionEvent(execution)
}

func provisionOrchestratorWorkers(ctx context.Context, execution OrchestratorExecution) ([]OrchestratorWorkerLease, error) {
	leases := make([]OrchestratorWorkerLease, 0)
	installedByRun := map[string]bool{}

	for _, req := range execution.RequiredWorkers {
		resolvedReq := req
		resolvedHostID := strings.TrimSpace(req.HostID)
		if resolvedHostID == "" && len(req.HostLabels) > 0 {
			host, resolveErr := selectOrchestratorRemoteHostByLabels(req.HostLabels)
			if resolveErr != nil {
				return nil, resolveErr
			}
			resolvedHostID = host.ID
			resolvedReq.HostID = host.ID
		}
		if strings.EqualFold(resolvedHostID, orchestratorLocalHostID) {
			localLeases, localErr := provisionOrchestratorLocalWorkers(ctx, execution, resolvedReq)
			if localErr != nil {
				return nil, localErr
			}
			leases = append(leases, localLeases...)
			continue
		}
		host, found, err := getRemoteHost(resolvedHostID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("remote host %s not found", resolvedHostID)
		}
		key := strings.ToLower(strings.TrimSpace(resolvedHostID) + ":" + strings.TrimSpace(resolvedReq.AgentID))
		for i := 0; i < resolvedReq.Count; i++ {
			lease := OrchestratorWorkerLease{
				ID:              uuid.NewString(),
				ExecutionID:     execution.ID,
				HostID:          resolvedHostID,
				AgentID:         resolvedReq.AgentID,
				State:           OrchestratorWorkerStateProvisioning,
				LeaseState:      string(OrchestratorWorkerStateProvisioning),
				CreatedAt:       nowTimestamp(),
				UpdatedAt:       nowTimestamp(),
				HeartbeatAt:     nowTimestamp(),
				LastHeartbeatAt: nowTimestamp(),
				LeaseExpireAt: time.Now().UTC().Add(defaultOrchestratorWorkerIdleTTL).
					Format(time.RFC3339Nano),
			}
			if _, saveErr := upsertOrchestratorWorkerLease(lease); saveErr != nil {
				return nil, saveErr
			}

			exists, existsErr := remoteAgentConfigExists(ctx, host, resolvedReq.AgentID)
			if existsErr != nil {
				lease.State = OrchestratorWorkerStateError
				lease.LeaseState = string(OrchestratorWorkerStateError)
				lease.LastError = existsErr.Error()
				lease.UpdatedAt = nowTimestamp()
				if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
					logOrchestratorPersistError("upsert orchestrator worker lease config-check error", err)
				}
				return nil, existsErr
			}

			ephemeral := !exists || installedByRun[key]
			if !exists && !installedByRun[key] {
				installResult, installErr := orchestratorInstallAgent(ctx, host, resolvedHostID, resolvedReq.AgentID, false)
				if installErr != nil {
					lease.State = OrchestratorWorkerStateError
					lease.LeaseState = string(OrchestratorWorkerStateError)
					lease.LastError = installErr.Error()
					lease.UpdatedAt = nowTimestamp()
					if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
						logOrchestratorPersistError("upsert orchestrator worker lease install error", err)
					}
					return nil, installErr
				}
				if installResult == nil || !installResult.Installed {
					return nil, fmt.Errorf("worker install did not complete for %s:%s", resolvedHostID, resolvedReq.AgentID)
				}
				installedByRun[key] = true
				ephemeral = true
			}

			lease.Ephemeral = ephemeral
			lease.InstalledByRun = installedByRun[key]
			lease.State = OrchestratorWorkerStateReady
			lease.LeaseState = string(OrchestratorWorkerStateReady)
			lease.LastError = ""
			lease.HeartbeatAt = nowTimestamp()
			lease.LastHeartbeatAt = lease.HeartbeatAt
			lease.Stale = false
			lease.StaleReason = ""
			lease.UpdatedAt = nowTimestamp()
			savedLease, saveErr := upsertOrchestratorWorkerLease(lease)
			if saveErr != nil {
				return nil, saveErr
			}
			leases = append(leases, savedLease)
		}
	}
	return leases, nil
}

func provisionOrchestratorLocalWorkers(ctx context.Context, execution OrchestratorExecution, req OrchestratorRequiredWorker) ([]OrchestratorWorkerLease, error) {
	daemon := orchestratorLocalDaemonClientFn
	if daemon == nil {
		return nil, fmt.Errorf("local daemon client factory is not configured")
	}
	client := daemon()
	if client == nil {
		return nil, fmt.Errorf("local daemon client is not available")
	}
	agentID := strings.ToLower(strings.TrimSpace(req.AgentID))
	if agentID == "" {
		agentID = "zeroclaw"
	}
	if err := ensureOrchestratorLocalAgentReady(ctx, client, execution.ID, agentID); err != nil {
		return nil, err
	}

	leases := make([]OrchestratorWorkerLease, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		lease := OrchestratorWorkerLease{
			ID:              uuid.NewString(),
			ExecutionID:     execution.ID,
			HostID:          orchestratorLocalHostID,
			AgentID:         agentID,
			State:           OrchestratorWorkerStateReady,
			LeaseState:      string(OrchestratorWorkerStateReady),
			Ephemeral:       false,
			InstalledByRun:  false,
			CreatedAt:       nowTimestamp(),
			UpdatedAt:       nowTimestamp(),
			HeartbeatAt:     nowTimestamp(),
			LastHeartbeatAt: nowTimestamp(),
			LeaseExpireAt: time.Now().UTC().Add(defaultOrchestratorWorkerIdleTTL).
				Format(time.RFC3339Nano),
		}
		saved, err := upsertOrchestratorWorkerLease(lease)
		if err != nil {
			return nil, err
		}
		leases = append(leases, saved)
	}
	return leases, nil
}

func ensureOrchestratorLocalAgentReady(ctx context.Context, daemon *DaemonClient, executionID, agentID string) error {
	if daemon == nil {
		return fmt.Errorf("daemon client is not configured")
	}
	actor := "gateway:orchestrator:local"
	requestID := "orchestrator-local-" + strings.TrimSpace(executionID)
	agents, err := daemon.ListAgents(ctx, actor, requestID)
	if err != nil {
		return fmt.Errorf("list local agents failed: %w", err)
	}
	var matched *AgentState
	for idx := range agents {
		if strings.EqualFold(strings.TrimSpace(agents[idx].ID), agentID) {
			matched = &agents[idx]
			break
		}
	}
	if matched == nil {
		return fmt.Errorf("local worker agent %s not found", agentID)
	}
	if !strings.EqualFold(strings.TrimSpace(matched.InstallState), "installed") {
		return fmt.Errorf("local worker agent %s is not installed", agentID)
	}
	if strings.EqualFold(strings.TrimSpace(matched.Runtime), "running") {
		return nil
	}
	if err := daemon.StartAgentWithOptions(ctx, agentID, StartAgentOptions{Isolation: matched.Isolated}, actor, requestID); err != nil {
		return fmt.Errorf("start local worker agent %s failed: %w", agentID, err)
	}
	statuses, err := daemon.GetStatus(ctx, agentID, actor, requestID)
	if err != nil {
		return fmt.Errorf("verify local worker agent %s status failed: %w", agentID, err)
	}
	if len(statuses) == 0 {
		return fmt.Errorf("local worker agent %s status is unavailable", agentID)
	}
	runtimeState := strings.ToLower(strings.TrimSpace(statuses[0].Runtime))
	if runtimeState != "running" && runtimeState != "starting" {
		if runtimeState == "" {
			runtimeState = "unknown"
		}
		return fmt.Errorf("local worker agent %s runtime state is %s", agentID, runtimeState)
	}
	return nil
}

func runOrchestratorTasks(ctx context.Context, execution OrchestratorExecution, leases []OrchestratorWorkerLease) ([]OrchestratorTaskResult, error) {
	if len(execution.TaskUnits) == 0 {
		return []OrchestratorTaskResult{}, nil
	}

	pools := map[string]orchestratorWorkerPool{}
	firstPoolKey := ""
	for _, lease := range leases {
		key := workerPoolKey(lease.HostID, lease.AgentID)
		pool, ok := pools[key]
		if !ok {
			pool = orchestratorWorkerPool{
				key: key,
				ch:  make(chan OrchestratorWorkerLease, len(leases)),
			}
			pools[key] = pool
			if firstPoolKey == "" {
				firstPoolKey = key
			}
		}
		pool.ch <- lease
	}
	if len(pools) == 0 {
		return nil, fmt.Errorf("no available workers")
	}

	maxConcurrency := execution.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = defaultOrchestratorMaxConcurrency
	}
	if maxConcurrency > len(execution.TaskUnits) {
		maxConcurrency = len(execution.TaskUnits)
	}
	type taskOutcome struct {
		index  int
		result OrchestratorTaskResult
		err    error
	}
	jobs := make(chan int)
	outcomes := make(chan taskOutcome, len(execution.TaskUnits))
	var wg sync.WaitGroup
	for i := 0; i < maxConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if err := waitIfOrchestratorExecutionPaused(ctx, execution.ID); err != nil {
					outcomes <- taskOutcome{
						index: idx,
						result: OrchestratorTaskResult{
							TaskID: execution.TaskUnits[idx].ID,
							Status: OrchestratorTaskStatusFailed,
							Error:  err.Error(),
						},
						err: err,
					}
					continue
				}
				task := execution.TaskUnits[idx]
				result, err := runTaskWithRetries(ctx, execution, task, pools, firstPoolKey)
				outcomes <- taskOutcome{
					index:  idx,
					result: result,
					err:    err,
				}
			}
		}()
	}

	for i := range execution.TaskUnits {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	close(outcomes)

	results := make([]OrchestratorTaskResult, len(execution.TaskUnits))
	var firstErr error
	for outcome := range outcomes {
		results[outcome.index] = outcome.result
		if outcome.err != nil && firstErr == nil {
			firstErr = outcome.err
		}
	}
	if firstErr != nil {
		return results, firstErr
	}
	return results, nil
}

func runTaskWithRetries(
	ctx context.Context,
	execution OrchestratorExecution,
	task OrchestratorTaskUnit,
	pools map[string]orchestratorWorkerPool,
	firstPoolKey string,
) (OrchestratorTaskResult, error) {
	retries := task.RetryBudget
	var lastErr error
	var lastResult OrchestratorTaskResult
	for attempt := 1; attempt <= retries+1; attempt++ {
		if err := waitIfOrchestratorExecutionPaused(ctx, execution.ID); err != nil {
			lastErr = err
			lastResult = OrchestratorTaskResult{
				TaskID:   task.ID,
				Status:   OrchestratorTaskStatusFailed,
				Attempts: attempt,
				Error:    err.Error(),
			}
			return lastResult, err
		}
		lease, key, err := acquireWorkerForTask(ctx, task, pools, firstPoolKey)
		if err != nil {
			lastErr = err
			lastResult = OrchestratorTaskResult{
				TaskID:   task.ID,
				Status:   OrchestratorTaskStatusFailed,
				Attempts: attempt,
				Error:    err.Error(),
			}
			continue
		}

		lease.State = OrchestratorWorkerStateBusy
		lease.LeaseState = string(OrchestratorWorkerStateBusy)
		lease.HeartbeatAt = nowTimestamp()
		lease.LastHeartbeatAt = lease.HeartbeatAt
		lease.Stale = false
		lease.StaleReason = ""
		lease.UpdatedAt = nowTimestamp()
		if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
			logOrchestratorPersistError("upsert orchestrator worker lease busy", err)
		}

		start := time.Now()
		result, execErr := runOrchestratorTaskAttempt(ctx, execution, task, lease, attempt)
		lease.HeartbeatAt = nowTimestamp()
		lease.LastHeartbeatAt = lease.HeartbeatAt
		lease.UpdatedAt = nowTimestamp()
		if execErr != nil {
			lease.State = OrchestratorWorkerStateReady
			lease.LeaseState = string(OrchestratorWorkerStateReady)
			lease.LastError = execErr.Error()
			lease.Stale = false
			lease.StaleReason = ""
			if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
				logOrchestratorPersistError("upsert orchestrator worker lease ready-on-error", err)
			}
			releaseWorkerToPool(lease, pools[key])

			lastErr = execErr
			lastResult = result
			lastResult.LatencyMs = time.Since(start).Milliseconds()
			if attempt > retries {
				return lastResult, execErr
			}
			continue
		}

		lease.State = OrchestratorWorkerStateReady
		lease.LeaseState = string(OrchestratorWorkerStateReady)
		lease.LastError = ""
		lease.TaskCount++
		lease.Stale = false
		lease.StaleReason = ""
		if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
			logOrchestratorPersistError("upsert orchestrator worker lease ready", err)
		}
		releaseWorkerToPool(lease, pools[key])

		result.Status = OrchestratorTaskStatusCompleted
		result.Attempts = attempt
		result.LatencyMs = time.Since(start).Milliseconds()
		return result, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("task failed with unknown error")
	}
	return lastResult, lastErr
}

func runOrchestratorTaskAttempt(
	ctx context.Context,
	execution OrchestratorExecution,
	task OrchestratorTaskUnit,
	lease OrchestratorWorkerLease,
	attempt int,
) (OrchestratorTaskResult, error) {
	if strings.EqualFold(strings.TrimSpace(lease.HostID), orchestratorLocalHostID) {
		clientFactory := orchestratorLocalDaemonClientFn
		if clientFactory == nil {
			runErr := fmt.Errorf("local daemon client factory is not configured")
			return OrchestratorTaskResult{
				TaskID:   task.ID,
				Status:   OrchestratorTaskStatusFailed,
				WorkerID: lease.ID,
				HostID:   lease.HostID,
				AgentID:  lease.AgentID,
				Attempts: attempt,
				Error:    runErr.Error(),
			}, runErr
		}
		client := clientFactory()
		if client == nil {
			runErr := fmt.Errorf("local daemon client is not available")
			return OrchestratorTaskResult{
				TaskID:   task.ID,
				Status:   OrchestratorTaskStatusFailed,
				WorkerID: lease.ID,
				HostID:   lease.HostID,
				AgentID:  lease.AgentID,
				Attempts: attempt,
				Error:    runErr.Error(),
			}, runErr
		}

		timeout := time.Duration(task.TimeoutMs) * time.Millisecond
		if timeout <= 0 {
			timeout = 60 * time.Second
		}
		if timeout > 5*time.Minute {
			timeout = 5 * time.Minute
		}
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		sessionID := strings.TrimSpace(task.SessionID)
		if sessionID == "" {
			sessionID = fmt.Sprintf("%s-%s-%d", execution.ID, task.ID, attempt)
		}

		start := time.Now()
		requestID := "orchestrator-" + strings.TrimSpace(execution.ID)
		chatResult, runErr := client.ChatAgent(
			runCtx,
			strings.TrimSpace(lease.AgentID),
			strings.TrimSpace(execution.RequestedProvider),
			strings.TrimSpace(task.Input),
			sessionID,
			"",
			"",
			"gateway:orchestrator:local",
			requestID,
		)
		if runErr != nil {
			return OrchestratorTaskResult{
				TaskID:      task.ID,
				Status:      OrchestratorTaskStatusFailed,
				WorkerID:    lease.ID,
				HostID:      lease.HostID,
				AgentID:     lease.AgentID,
				Attempts:    attempt,
				Error:       runErr.Error(),
				StartedAt:   start.UTC().Format(time.RFC3339Nano),
				CompletedAt: time.Now().UTC().Format(time.RFC3339Nano),
				LatencyMs:   time.Since(start).Milliseconds(),
			}, runErr
		}
		output := ""
		if chatResult != nil {
			output = strings.TrimSpace(chatResult.Message)
		}
		return OrchestratorTaskResult{
			TaskID:      task.ID,
			Status:      OrchestratorTaskStatusCompleted,
			WorkerID:    lease.ID,
			HostID:      lease.HostID,
			AgentID:     lease.AgentID,
			Attempts:    attempt,
			Output:      output,
			StartedAt:   start.UTC().Format(time.RFC3339Nano),
			CompletedAt: time.Now().UTC().Format(time.RFC3339Nano),
			LatencyMs:   time.Since(start).Milliseconds(),
		}, nil
	}

	host, found, err := getRemoteHost(lease.HostID)
	if err != nil {
		return OrchestratorTaskResult{
			TaskID:   task.ID,
			Status:   OrchestratorTaskStatusFailed,
			WorkerID: lease.ID,
			HostID:   lease.HostID,
			AgentID:  lease.AgentID,
			Attempts: attempt,
			Error:    err.Error(),
		}, err
	}
	if !found {
		runErr := fmt.Errorf("remote host %s not found", lease.HostID)
		return OrchestratorTaskResult{
			TaskID:   task.ID,
			Status:   OrchestratorTaskStatusFailed,
			WorkerID: lease.ID,
			HostID:   lease.HostID,
			AgentID:  lease.AgentID,
			Attempts: attempt,
			Error:    runErr.Error(),
		}, runErr
	}

	timeout := time.Duration(task.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if timeout > 5*time.Minute {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sessionID := strings.TrimSpace(task.SessionID)
	if sessionID == "" {
		sessionID = fmt.Sprintf("%s-%s-%d", execution.ID, task.ID, attempt)
	}

	start := time.Now()
	runResult, _, runErr := remoteRunTaskViaAgent(runCtx, host, lease.HostID, lease.AgentID, task.Input, sessionID)
	if runErr != nil {
		return OrchestratorTaskResult{
			TaskID:      task.ID,
			Status:      OrchestratorTaskStatusFailed,
			WorkerID:    lease.ID,
			HostID:      lease.HostID,
			AgentID:     lease.AgentID,
			Attempts:    attempt,
			Error:       runErr.Error(),
			StartedAt:   start.UTC().Format(time.RFC3339Nano),
			CompletedAt: time.Now().UTC().Format(time.RFC3339Nano),
			LatencyMs:   time.Since(start).Milliseconds(),
		}, runErr
	}
	return OrchestratorTaskResult{
		TaskID:      task.ID,
		Status:      OrchestratorTaskStatusCompleted,
		WorkerID:    lease.ID,
		HostID:      lease.HostID,
		AgentID:     lease.AgentID,
		Attempts:    attempt,
		Output:      strings.TrimSpace(runResult.Output),
		StartedAt:   start.UTC().Format(time.RFC3339Nano),
		CompletedAt: time.Now().UTC().Format(time.RFC3339Nano),
		LatencyMs:   time.Since(start).Milliseconds(),
	}, nil
}

func workerPoolKey(hostID, agentID string) string {
	return strings.ToLower(strings.TrimSpace(hostID) + ":" + strings.TrimSpace(agentID))
}

func acquireWorkerForTask(
	ctx context.Context,
	task OrchestratorTaskUnit,
	pools map[string]orchestratorWorkerPool,
	firstPoolKey string,
) (OrchestratorWorkerLease, string, error) {
	hostID := strings.TrimSpace(task.HostID)
	hostLabels := normalizeStringSelectorList(task.HostLabels, true)
	agentID := strings.TrimSpace(task.AgentID)
	key := ""
	if hostID != "" {
		if agentID == "" {
			agentID = "zeroclaw"
		}
		key = workerPoolKey(hostID, agentID)
		pool, ok := pools[key]
		if !ok {
			return OrchestratorWorkerLease{}, "", fmt.Errorf("no worker pool available for key %s", key)
		}
		select {
		case <-ctx.Done():
			return OrchestratorWorkerLease{}, "", ctx.Err()
		case lease := <-pool.ch:
			return lease, key, nil
		}
	}

	if agentID != "" {
		matching, matchErr := matchingOrchestratorWorkerPools(agentID, hostLabels, pools)
		if matchErr != nil {
			return OrchestratorWorkerLease{}, "", matchErr
		}
		if len(matching) > 0 {
			cases := make([]reflect.SelectCase, 0, len(matching)+1)
			poolKeys := make([]string, 0, len(matching))
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(ctx.Done()),
			})
			for _, pool := range matching {
				cases = append(cases, reflect.SelectCase{
					Dir:  reflect.SelectRecv,
					Chan: reflect.ValueOf(pool.ch),
				})
				poolKeys = append(poolKeys, pool.key)
			}
			chosen, recv, recvOK := reflect.Select(cases)
			if chosen == 0 {
				return OrchestratorWorkerLease{}, "", ctx.Err()
			}
			if !recvOK {
				return OrchestratorWorkerLease{}, "", fmt.Errorf("worker pool closed for key %s", poolKeys[chosen-1])
			}
			lease, _ := recv.Interface().(OrchestratorWorkerLease)
			return lease, poolKeys[chosen-1], nil
		}
	}

	key = firstPoolKey
	pool, ok := pools[key]
	if !ok {
		return OrchestratorWorkerLease{}, "", fmt.Errorf("no worker pool available for key %s", key)
	}
	select {
	case <-ctx.Done():
		return OrchestratorWorkerLease{}, "", ctx.Err()
	case lease := <-pool.ch:
		return lease, key, nil
	}
}

func matchingOrchestratorWorkerPools(agentID string, hostLabels []string, pools map[string]orchestratorWorkerPool) ([]orchestratorWorkerPool, error) {
	normalized := strings.ToLower(strings.TrimSpace(agentID))
	if normalized == "" {
		return nil, nil
	}
	keys := make([]string, 0, len(pools))
	for key, pool := range pools {
		poolAgentID := pool.key
		if idx := strings.LastIndex(poolAgentID, ":"); idx >= 0 && idx+1 < len(poolAgentID) {
			poolAgentID = poolAgentID[idx+1:]
		}
		if !strings.EqualFold(strings.TrimSpace(poolAgentID), normalized) {
			continue
		}
		if len(hostLabels) > 0 {
			poolHostID := pool.key
			if idx := strings.LastIndex(poolHostID, ":"); idx > 0 {
				poolHostID = poolHostID[:idx]
			}
			matches, err := orchestratorHostMatchesLabels(poolHostID, hostLabels)
			if err != nil {
				return nil, err
			}
			if !matches {
				continue
			}
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	matching := make([]orchestratorWorkerPool, 0, len(keys))
	for _, key := range keys {
		matching = append(matching, pools[key])
	}
	return matching, nil
}

func selectOrchestratorRemoteHostByLabels(hostLabels []string) (RemoteHost, error) {
	required := normalizeStringSelectorList(hostLabels, true)
	if len(required) == 0 {
		return RemoteHost{}, fmt.Errorf("hostLabels are required to resolve a remote host")
	}
	hosts, err := listRemoteHosts()
	if err != nil {
		return RemoteHost{}, err
	}
	matching := make([]RemoteHost, 0, len(hosts))
	for _, host := range hosts {
		if stringSliceContainsAllFold(host.Labels, required) {
			matching = append(matching, host)
		}
	}
	if len(matching) == 0 {
		return RemoteHost{}, fmt.Errorf("no remote host matches labels %s", strings.Join(required, ","))
	}
	sort.SliceStable(matching, func(i, j int) bool {
		leftHealth := orchestratorRemoteHealthRank(matching[i].LastHealth)
		rightHealth := orchestratorRemoteHealthRank(matching[j].LastHealth)
		if leftHealth != rightHealth {
			return leftHealth < rightHealth
		}
		leftName := strings.ToLower(strings.TrimSpace(firstNonEmptyPolicyValue(matching[i].Name, matching[i].ID)))
		rightName := strings.ToLower(strings.TrimSpace(firstNonEmptyPolicyValue(matching[j].Name, matching[j].ID)))
		return leftName < rightName
	})
	return matching[0], nil
}

func orchestratorHostMatchesLabels(hostID string, hostLabels []string) (bool, error) {
	if strings.TrimSpace(hostID) == "" || strings.EqualFold(strings.TrimSpace(hostID), orchestratorLocalHostID) {
		return false, nil
	}
	host, found, err := getRemoteHost(hostID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	return stringSliceContainsAllFold(host.Labels, hostLabels), nil
}

func orchestratorRemoteHealthRank(health RemoteHealth) int {
	switch health {
	case RemoteHealthHealthy:
		return 0
	case RemoteHealthUnknown:
		return 1
	default:
		return 2
	}
}

func releaseWorkerToPool(lease OrchestratorWorkerLease, pool orchestratorWorkerPool) {
	if lease.State == OrchestratorWorkerStateReclaimed {
		return
	}
	select {
	case pool.ch <- lease:
	default:
	}
}

func reclaimExecutionLeases(ctx context.Context, executionID string, force bool) (map[string]interface{}, error) {
	leases, err := orchestratorListLeasesByExecution(executionID)
	if err != nil {
		return nil, err
	}
	return reclaimOrchestratorLeaseSet(ctx, leases, force, 0)
}

func reclaimOrchestratorWorkers(ctx context.Context, force bool, idleTTL time.Duration) (map[string]interface{}, error) {
	leases, err := listOrchestratorWorkerLeases()
	if err != nil {
		return nil, err
	}
	return reclaimOrchestratorLeaseSet(ctx, leases, force, idleTTL)
}

func reclaimOrchestratorLeaseSet(ctx context.Context, leases []OrchestratorWorkerLease, force bool, idleTTL time.Duration) (map[string]interface{}, error) {
	now := time.Now().UTC()
	reclaimed := 0
	skipped := 0
	failed := 0
	failures := make([]string, 0)

	recordPersistFailure := func(leaseID, step string, err error) {
		if err == nil {
			return
		}
		failed++
		msg := fmt.Sprintf("%s: persist %s failed: %v", strings.TrimSpace(leaseID), strings.TrimSpace(step), err)
		failures = append(failures, msg)
		logOrchestratorPersistError("upsert orchestrator worker lease "+strings.TrimSpace(step), err)
	}

	for _, lease := range leases {
		if lease.State == OrchestratorWorkerStateReclaimed {
			skipped++
			continue
		}
		if !force && idleTTL > 0 {
			heartbeatRaw := strings.TrimSpace(lease.LastHeartbeatAt)
			if heartbeatRaw == "" {
				heartbeatRaw = strings.TrimSpace(lease.HeartbeatAt)
			}
			heartbeat := parseRFC3339OrNow(heartbeatRaw)
			if now.Sub(heartbeat) < idleTTL {
				skipped++
				continue
			}
		}
		if !lease.Ephemeral && !force {
			skipped++
			continue
		}

		lease.State = OrchestratorWorkerStateReclaiming
		lease.LeaseState = string(OrchestratorWorkerStateReclaiming)
		lease.UpdatedAt = nowTimestamp()
		if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
			recordPersistFailure(lease.ID, "reclaiming", err)
		}

		if lease.Ephemeral {
			host, found, hostErr := getRemoteHost(lease.HostID)
			if hostErr != nil {
				failed++
				failures = append(failures, fmt.Sprintf("%s: %v", lease.ID, hostErr))
				lease.State = OrchestratorWorkerStateError
				lease.LeaseState = string(OrchestratorWorkerStateError)
				lease.LastError = hostErr.Error()
				lease.UpdatedAt = nowTimestamp()
				if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
					recordPersistFailure(lease.ID, "host-error", err)
				}
				continue
			}
			if found {
				_, uninstallErr := remoteUninstallAgent(ctx, host, lease.HostID, lease.AgentID)
				if uninstallErr != nil {
					failed++
					failures = append(failures, fmt.Sprintf("%s: %v", lease.ID, uninstallErr))
					lease.State = OrchestratorWorkerStateError
					lease.LeaseState = string(OrchestratorWorkerStateError)
					lease.LastError = uninstallErr.Error()
					lease.UpdatedAt = nowTimestamp()
					if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
						recordPersistFailure(lease.ID, "uninstall-error", err)
					}
					continue
				}
			}
		}

		lease.State = OrchestratorWorkerStateReclaimed
		lease.LeaseState = string(OrchestratorWorkerStateReclaimed)
		lease.LastError = ""
		lease.HeartbeatAt = nowTimestamp()
		lease.LastHeartbeatAt = lease.HeartbeatAt
		lease.Stale = false
		lease.StaleReason = ""
		lease.UpdatedAt = nowTimestamp()
		if _, err := upsertOrchestratorWorkerLease(lease); err != nil {
			recordPersistFailure(lease.ID, "reclaimed", err)
		}
		reclaimed++
	}

	summary := map[string]interface{}{
		"reclaimed": reclaimed,
		"skipped":   skipped,
		"failed":    failed,
		"failures":  failures,
	}
	if failed > 0 {
		return summary, fmt.Errorf("%d worker reclaim operations failed", failed)
	}
	return summary, nil
}

func parseRFC3339OrNow(raw string) time.Time {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Now().UTC()
	}
	parsed, err := time.Parse(time.RFC3339Nano, trimmed)
	if err != nil {
		return time.Now().UTC()
	}
	return parsed.UTC()
}

func remoteAgentConfigExists(ctx context.Context, host RemoteHost, agentID string) (bool, error) {
	path := remoteOpenClawConfigPath
	switch normalizeRemoteInstallAgentID(agentID) {
	case "picoclaw":
		path = remotePicoClawConfigPath
	case "zeroclaw":
		path = remoteZeroClawConfigPath
	}
	exists, _, err := remoteConfigFileExists(ctx, host, path)
	return exists, err
}

func remoteRunTaskViaAgent(ctx context.Context, host RemoteHost, hostID, agentID, message, sessionID string) (*remoteRunResult, []remoteExecResult, error) {
	switch normalizeRemoteInstallAgentID(agentID) {
	case "zeroclaw":
		return remoteRunViaZeroClaw(ctx, host, hostID, agentID, message, sessionID)
	default:
		return remoteRunViaOpenClaw(ctx, host, hostID, agentID, message, sessionID)
	}
}

func remoteRunViaZeroClaw(ctx context.Context, host RemoteHost, hostID, agentID, message, sessionID string) (*remoteRunResult, []remoteExecResult, error) {
	startedAt := time.Now()
	if err := validateAgentIdentifier(agentID); err != nil {
		return nil, nil, err
	}
	trimmedMessage := strings.TrimSpace(message)
	if trimmedMessage == "" {
		return nil, nil, fmt.Errorf("message is required")
	}

	contract := buildRemoteMemoryRuntimeContract(agentID)
	candidates := []string{
		fmt.Sprintf("zeroclaw task run --message %s --json --no-color", shellSingleQuote(trimmedMessage)),
		fmt.Sprintf("zeroclaw run --message %s --json --no-color", shellSingleQuote(trimmedMessage)),
		fmt.Sprintf("zeroclaw agent --message %s --json --no-color", shellSingleQuote(trimmedMessage)),
	}
	if strings.TrimSpace(sessionID) != "" {
		for i := range candidates {
			candidates[i] += " --session-id " + shellSingleQuote(strings.TrimSpace(sessionID))
		}
	}

	steps := make([]remoteExecResult, 0, len(candidates)+1)
	for _, cmd := range candidates {
		res, err := runRemoteCommand(ctx, host, wrapRemoteCommandWithMemoryContract(cmd, contract))
		steps = append(steps, res)
		if err != nil {
			continue
		}
		if res.ExitCode != 0 {
			continue
		}
		payload := parseRemoteRunPayload(res.Stdout, contract)
		return &remoteRunResult{
			HostID:     strings.TrimSpace(hostID),
			AgentID:    strings.TrimSpace(agentID),
			SessionID:  strings.TrimSpace(anyToString(payload["sessionId"])),
			Output:     extractChatResponseText(payload),
			LatencyMs:  time.Since(startedAt).Milliseconds(),
			Memory:     parseRemoteMemoryStatus(payload),
			RawPayload: payload,
		}, steps, nil
	}

	fallback, fallbackSteps, fallbackErr := remoteRunViaOpenClaw(ctx, host, hostID, agentID, message, sessionID)
	steps = append(steps, fallbackSteps...)
	if fallbackErr == nil {
		return fallback, steps, nil
	}
	return nil, steps, fmt.Errorf("zeroclaw direct run failed and openclaw fallback failed: %w", fallbackErr)
}

func parseRemoteRunPayload(stdout string, contract remoteMemoryRuntimeContract) map[string]interface{} {
	payload := strings.TrimSpace(extractJSONObjectOrArray(stdout))
	if payload == "" {
		return defaultRemoteRunPayload(stdout, contract)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &out); err != nil {
		return defaultRemoteRunPayload(stdout, contract)
	}
	if _, ok := out["memory"]; !ok {
		out["memory"] = defaultRemoteRunMemoryPayload(contract)
	}
	return out
}

func defaultRemoteRunPayload(stdout string, contract remoteMemoryRuntimeContract) map[string]interface{} {
	return map[string]interface{}{
		"message": strings.TrimSpace(stdout),
		"memory":  defaultRemoteRunMemoryPayload(contract),
	}
}

func defaultRemoteRunMemoryPayload(contract remoteMemoryRuntimeContract) map[string]interface{} {
	return map[string]interface{}{
		"contractId":     contract.ContractID,
		"contractDigest": contract.ContractDigest,
		"syncState":      "ready",
		"syncedAt":       nowTimestamp(),
	}
}
