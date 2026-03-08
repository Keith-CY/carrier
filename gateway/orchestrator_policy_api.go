package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func handleOrchestratorPolicies(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}

	trimmed := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/orchestrator/policies"), "/")
	if trimmed == "" {
		switch r.Method {
		case http.MethodGet:
			policies, err := listOrchestratorPolicies()
			if err != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to list orchestrator policies", "list orchestrator policies", err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"requestId": requestID,
				"result":    "ok",
				"policies":  policies,
			})
			return
		case http.MethodPost:
			var req OrchestratorPolicyRule
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
				return
			}
			if strings.TrimSpace(req.ID) == "" && !req.Enabled {
				req.Enabled = true
			}
			saved, err := upsertOrchestratorPolicy(req)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
				return
			}
			emitRemoteAuditEvent(requestID, "orchestrator_policy_upsert", saved.ID, "success", map[string]interface{}{
				"name":     saved.Name,
				"action":   saved.Action,
				"enabled":  saved.Enabled,
				"priority": saved.Priority,
			})
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"requestId": requestID,
				"result":    "ok",
				"policy":    saved,
			})
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
	}

	policyID := strings.TrimSpace(trimmed)
	if policyID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "policy id is required"))
		return
	}
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	deleted, err := deleteOrchestratorPolicy(policyID)
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to delete orchestrator policy", "delete orchestrator policy", err)
		return
	}
	if !deleted {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", fmt.Sprintf("orchestrator policy %s not found", policyID)))
		return
	}
	emitRemoteAuditEvent(requestID, "orchestrator_policy_delete", policyID, "success", nil)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"deleted":   true,
	})
}
