package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"carrier/shared/integration"
)

var integrationVerifyBindingTarget = defaultIntegrationVerifyBindingTarget

func handleOneTokIntegration(w http.ResponseWriter, r *http.Request, requestID string) {
	switch {
	case r.URL.Path == "/api/v1/integrations/one-tok/bindings/verify":
		handleOneTokVerifyBinding(w, r, requestID)
	case r.URL.Path == "/api/v1/integrations/one-tok/executions":
		handleOneTokExecutions(w, r, requestID)
	case strings.HasPrefix(r.URL.Path, "/api/v1/integrations/one-tok/executions/"):
		handleOneTokExecution(w, r, requestID)
	default:
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "integration endpoint not found"))
	}
}

func handleOneTokVerifyBinding(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	binding, _, ok, authErr := authenticateIntegrationToken(gatewayTokenFromRequest(r), "one-tok")
	if authErr != nil {
		writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", authErr.Error()))
		return
	}
	if !ok {
		writeJSON(w, http.StatusUnauthorized, gatewayErrBody("E_INTEGRATION_AUTH_INVALID", "invalid integration binding token"))
		return
	}
	var req integration.VerifyBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid JSON body"))
		return
	}
	normalized, err := integration.NormalizeVerifyBindingRequest(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
		return
	}
	verify, err := integrationVerifyBindingTarget(binding, normalized)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_INTEGRATION_VERIFY_FAILED", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"verify":    verify,
	})
}

func handleOneTokExecutions(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	binding, _, ok, authErr := authenticateIntegrationToken(gatewayTokenFromRequest(r), "one-tok")
	if authErr != nil {
		writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", authErr.Error()))
		return
	}
	if !ok {
		writeJSON(w, http.StatusUnauthorized, gatewayErrBody("E_INTEGRATION_AUTH_INVALID", "invalid integration binding token"))
		return
	}
	var req integration.CreateExecutionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid JSON body"))
		return
	}
	result, err := createIntegrationExecution(binding, req)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "policy denied") {
			status = http.StatusForbidden
		}
		writeJSON(w, status, gatewayErrBody("E_INTEGRATION_EXECUTION_CREATE_FAILED", err.Error()))
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"existing":  result.Existing,
		"execution": result.Execution,
		"attempt":   result.Attempt,
	})
}

func handleOneTokExecution(w http.ResponseWriter, r *http.Request, requestID string) {
	binding, _, ok, authErr := authenticateIntegrationToken(gatewayTokenFromRequest(r), "one-tok")
	if authErr != nil {
		writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", authErr.Error()))
		return
	}
	if !ok {
		writeJSON(w, http.StatusUnauthorized, gatewayErrBody("E_INTEGRATION_AUTH_INVALID", "invalid integration binding token"))
		return
	}
	trimmed := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/integrations/one-tok/executions/"), "/")
	if trimmed == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "execution id is required"))
		return
	}
	parts := strings.Split(trimmed, "/")
	execID := strings.TrimSpace(parts[0])
	exec, attempt, found, err := getIntegrationExecutionByID(execID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", err.Error()))
		return
	}
	if !found || !strings.EqualFold(exec.BindingID, binding.ID) {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "integration execution not found"))
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		events, err := listIntegrationEventsByExecutionID(exec.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", err.Error()))
			return
		}
		proofs, err := listIntegrationUsageProofsByExecutionID(exec.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", err.Error()))
			return
		}
		artifacts, err := listIntegrationArtifactRefsByExecutionID(exec.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", err.Error()))
			return
		}
		deliveries, err := listIntegrationCallbackDeliveriesByExecutionID(exec.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId":          requestID,
			"result":             "ok",
			"execution":          exec,
			"attempt":            attempt,
			"events":             events,
			"usageProofs":        proofs,
			"artifactRefs":       artifacts,
			"callbackDeliveries": deliveries,
		})
		return
	}
	if len(parts) == 3 && parts[1] == "callbacks" && parts[2] == "replay" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var req struct {
			FromSequence int64  `json:"fromSequence,omitempty"`
			EventID      string `json:"eventId,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid JSON body"))
			return
		}
		replayed, replayErr := replayIntegrationCallbackDeliveries(execID, req.FromSequence, req.EventID)
		if replayErr != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_INTEGRATION_CALLBACK_REPLAY_FAILED", replayErr.Error()))
			return
		}
		deliveries, err := listIntegrationCallbackDeliveriesByExecutionID(exec.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", err.Error()))
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"requestId":          requestID,
			"result":             "ok",
			"execution":          exec,
			"replayed":           replayed,
			"callbackDeliveries": deliveries,
		})
		return
	}
	if len(parts) == 2 && parts[1] == "actions" {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var req integration.ActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_BAD_REQUEST", "invalid JSON body"))
			return
		}
		result, status, actionErr := applyIntegrationAction(execID, req)
		if actionErr != nil {
			code := "E_INTEGRATION_ACTION_FAILED"
			if status == httpStatusConflict {
				code = "E_INTEGRATION_ACTION_UNSUPPORTED"
			}
			writeJSON(w, status, map[string]interface{}{
				"requestId": requestID,
				"result":    "error",
				"execution": result.Execution,
				"action":    result.Action,
				"errorCode": code,
				"message":   strings.TrimSpace(actionErr.Error()),
			})
			return
		}
		writeJSON(w, status, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"execution": result.Execution,
			"action":    result.Action,
		})
		return
	}
	writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "integration execution path not found"))
}

func defaultIntegrationVerifyBindingTarget(binding integration.Binding, req integration.VerifyBindingRequest) (integration.VerifyBindingResult, error) {
	if !strings.EqualFold(binding.Target.HostID, req.HostID) ||
		!strings.EqualFold(binding.Target.AgentID, req.AgentID) ||
		!strings.EqualFold(binding.Target.Backend, req.Backend) ||
		!strings.EqualFold(binding.Target.WorkspaceRoot, req.WorkspaceRoot) {
		return integration.VerifyBindingResult{}, fmt.Errorf("requested target does not match binding scope")
	}
	host, ok, err := getRemoteHost(req.HostID)
	if err != nil {
		return integration.VerifyBindingResult{}, err
	}
	if !ok {
		return integration.VerifyBindingResult{}, fmt.Errorf("remote host %s not found", req.HostID)
	}
	return integration.VerifyBindingResult{
		Verified: true,
		Health: integration.BindingHealth{
			Healthy:       true,
			WorkspaceRoot: req.WorkspaceRoot,
			Detail:        strings.TrimSpace(host.Host),
		},
		ResolvedHostID:   req.HostID,
		ResolvedAgentID:  req.AgentID,
		ResolvedBackend:  req.Backend,
		VersionValue:     "",
		Capabilities:     req.SupportedCapabilities,
		BindingID:        binding.ID,
		BindingAccountID: binding.Account,
	}, nil
}
