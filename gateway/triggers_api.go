package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type executionTriggerPatchRequest struct {
	Name       *string                 `json:"name,omitempty"`
	Enabled    *bool                   `json:"enabled,omitempty"`
	TemplateID *string                 `json:"templateId,omitempty"`
	CreatedBy  *string                 `json:"createdBy,omitempty"`
	Config     *ExecutionTriggerConfig `json:"config,omitempty"`
}

func handleExecutionTriggers(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}

	trimmed := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/triggers"), "/")
	if trimmed == "" {
		switch r.Method {
		case http.MethodGet:
			if _, ok := requireGatewayPermission(w, r, cfg, canViewExecutions, "E_RBAC_EXECUTION_VIEW", "role cannot view execution triggers"); !ok {
				return
			}
			triggers, err := listExecutionTriggers()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", "failed to list execution triggers"))
				return
			}
			out := make([]ExecutionTrigger, 0, len(triggers))
			for _, trigger := range triggers {
				out = append(out, sanitizeExecutionTrigger(trigger))
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"requestId": requestID,
				"result":    "ok",
				"triggers":  out,
			})
			return
		case http.MethodPost:
			if _, ok := requireGatewayPermission(w, r, cfg, canManagePolicies, "E_RBAC_TRIGGER_MANAGE", "role cannot manage execution triggers"); !ok {
				return
			}
			var trigger ExecutionTrigger
			if err := json.NewDecoder(r.Body).Decode(&trigger); err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
				return
			}
			if !trigger.Enabled {
				trigger.Enabled = true
			}
			if prepared, err := prepareExecutionTriggerForSave(trigger, true); err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
				return
			} else {
				trigger = prepared
			}
			saved, err := upsertExecutionTrigger(trigger)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
				return
			}
			emitRemoteAuditEvent(requestID, "orchestrator_trigger_upsert", saved.ID, "success", map[string]interface{}{
				"type":       saved.Type,
				"templateId": saved.TemplateID,
				"createdBy":  saved.CreatedBy,
				"enabled":    saved.Enabled,
			})
			writeJSON(w, http.StatusCreated, map[string]interface{}{
				"requestId": requestID,
				"result":    "ok",
				"trigger":   sanitizeExecutionTrigger(saved),
			})
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
	}

	triggerID := strings.TrimSpace(trimmed)
	trigger, ok, err := getExecutionTrigger(triggerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", "failed to load execution trigger"))
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "execution trigger not found"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		if _, ok := requireGatewayPermission(w, r, cfg, canViewExecutions, "E_RBAC_EXECUTION_VIEW", "role cannot view execution triggers"); !ok {
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"trigger":   sanitizeExecutionTrigger(trigger),
		})
	case http.MethodPatch:
		if _, ok := requireGatewayPermission(w, r, cfg, canManagePolicies, "E_RBAC_TRIGGER_MANAGE", "role cannot manage execution triggers"); !ok {
			return
		}
		var patch executionTriggerPatchRequest
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
			return
		}
		merged := trigger
		if patch.Name != nil {
			merged.Name = strings.TrimSpace(*patch.Name)
		}
		if patch.Enabled != nil {
			merged.Enabled = *patch.Enabled
		}
		if patch.TemplateID != nil {
			merged.TemplateID = strings.TrimSpace(*patch.TemplateID)
		}
		if patch.CreatedBy != nil {
			merged.CreatedBy = strings.TrimSpace(*patch.CreatedBy)
		}
		if patch.Config != nil {
			nextConfig := *patch.Config
			if strings.TrimSpace(nextConfig.WebhookSecret) == "" {
				nextConfig.WebhookSecret = trigger.Config.WebhookSecret
			}
			if len(nextConfig.HostIDs) == 0 {
				nextConfig.HostIDs = append([]string(nil), trigger.Config.HostIDs...)
			}
			if len(nextConfig.HostLabels) == 0 {
				nextConfig.HostLabels = append([]string(nil), trigger.Config.HostLabels...)
			}
			if nextConfig.Provider == "" {
				nextConfig.Provider = trigger.Config.Provider
			}
			if nextConfig.MaxConcurrency == 0 {
				nextConfig.MaxConcurrency = trigger.Config.MaxConcurrency
			}
			if !nextConfig.PolicyApprove {
				nextConfig.PolicyApprove = trigger.Config.PolicyApprove
			}
			if len(nextConfig.Inputs) == 0 {
				nextConfig.Inputs = cloneTriggerInputs(trigger.Config.Inputs)
			}
			if nextConfig.GitHubCommand == "" {
				nextConfig.GitHubCommand = trigger.Config.GitHubCommand
			}
			if nextConfig.GitHubLabel == "" {
				nextConfig.GitHubLabel = trigger.Config.GitHubLabel
			}
			if nextConfig.GitHubRepository == "" {
				nextConfig.GitHubRepository = trigger.Config.GitHubRepository
			}
			if nextConfig.Cron == "" {
				nextConfig.Cron = trigger.Config.Cron
			}
			if nextConfig.Timezone == "" {
				nextConfig.Timezone = trigger.Config.Timezone
			}
			merged.Config = nextConfig
		}
		prepared, err := prepareExecutionTriggerForSave(merged, false)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
			return
		}
		saved, err := upsertExecutionTrigger(prepared)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
			return
		}
		emitRemoteAuditEvent(requestID, "orchestrator_trigger_upsert", saved.ID, "success", map[string]interface{}{
			"type":       saved.Type,
			"templateId": saved.TemplateID,
			"createdBy":  saved.CreatedBy,
			"enabled":    saved.Enabled,
		})
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"trigger":   sanitizeExecutionTrigger(saved),
		})
	case http.MethodDelete:
		if _, ok := requireGatewayPermission(w, r, cfg, canManagePolicies, "E_RBAC_TRIGGER_MANAGE", "role cannot manage execution triggers"); !ok {
			return
		}
		if err := deleteExecutionTrigger(trigger.ID); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "execution trigger not found"))
				return
			}
			writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", "failed to delete execution trigger"))
			return
		}
		emitRemoteAuditEvent(requestID, "orchestrator_trigger_delete", trigger.ID, "success", map[string]interface{}{
			"type":       trigger.Type,
			"templateId": trigger.TemplateID,
		})
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requestId": requestID,
			"result":    "ok",
			"deleted":   true,
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
	}
}

func handleExecutionTriggerWebhook(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	triggerID := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/triggers/webhook/"))
	if triggerID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "trigger id is required"))
		return
	}
	trigger, ok, err := getExecutionTrigger(triggerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", "failed to load execution trigger"))
		return
	}
	if !ok || !trigger.Enabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "execution trigger not found"))
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "failed to read webhook body"))
		return
	}
	result, apiErr := launchExecutionTriggerFromWebhook(requestID, cfg, trigger, r, body)
	if apiErr != nil {
		writeJSON(w, apiErr.Status, apiErr.Body)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func prepareExecutionTriggerForSave(trigger ExecutionTrigger, isCreate bool) (ExecutionTrigger, error) {
	trigger, err := normalizeExecutionTriggerForStore(trigger)
	if err != nil {
		return ExecutionTrigger{}, err
	}
	if isCreate && trigger.CreatedBy == "" {
		trigger.CreatedBy = "admin"
	}
	if trigger.Type == ExecutionTriggerTypeSchedule {
		nextRunAt, err := nextExecutionTriggerRunAt(time.Now().UTC(), trigger.Config.Cron)
		if err != nil {
			return ExecutionTrigger{}, err
		}
		if trigger.Enabled {
			trigger.NextRunAt = nextRunAt.Format(time.RFC3339)
		}
	}
	if !trigger.Enabled {
		trigger.NextRunAt = ""
	}
	return trigger, nil
}

func cloneTriggerInputs(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
