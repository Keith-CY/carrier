package gateway

import (
	"carrier/shared/orchestration"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type orchestratorPlanRequest struct {
	Goal           string            `json:"goal"`
	TemplateID     string            `json:"templateId,omitempty"`
	Inputs         map[string]string `json:"inputs,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	HostIDs        []string          `json:"hostIds,omitempty"`
	HostLabels     []string          `json:"hostLabels,omitempty"`
	RequiredMemory []string          `json:"requiredMemory,omitempty"`
	DistillOutputs []string          `json:"distillOutputs,omitempty"`
	MaxConcurrency int               `json:"maxConcurrency,omitempty"`
}

func handleOrchestratorPlans(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig, daemon *DaemonClient) {
	flags := effectiveGatewayFeatureFlags(cfg)
	if !flags.RemoteControlPlaneEnabled {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "remote control plane is disabled"))
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	if _, ok := requireGatewayPermission(w, r, cfg, canLaunchExecutions, "E_RBAC_EXECUTION_LAUNCH", "role cannot preview orchestrator execution plans"); !ok {
		return
	}

	var req orchestratorPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
		return
	}
	req.TemplateID = strings.TrimSpace(req.TemplateID)
	req.Provider = strings.TrimSpace(req.Provider)
	req.Goal = strings.TrimSpace(req.Goal)
	plannerTasks, goal, err := buildPlannerTasksForPlanRequest(r.Context(), daemon, req.Goal, req.TemplateID, req.Inputs, req.Provider, requestID)
	if err != nil {
		if req.TemplateID != "" || strings.TrimSpace(req.Goal) == "" {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
			return
		}
		writeInternalGatewayError(w, http.StatusBadGateway, "E_UPSTREAM", "failed to decompose goal", "decompose base agent goal", err)
		return
	}

	plan, err := orchestration.BuildPlan(orchestration.BuildPlanInput{
		Goal:           goal,
		TemplateID:     req.TemplateID,
		Provider:       req.Provider,
		HostIDs:        req.HostIDs,
		HostLabels:     req.HostLabels,
		RequiredMemory: req.RequiredMemory,
		DistillOutputs: req.DistillOutputs,
		MaxConcurrency: req.MaxConcurrency,
		Tasks:          plannerTasks,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"plan":      plan,
	})
}

func buildPlannerTasksForPlanRequest(ctx context.Context, daemon *DaemonClient, goal, templateID string, inputs map[string]string, provider, requestID string) ([]orchestration.DecomposeTask, string, error) {
	trimmedTemplateID := strings.TrimSpace(templateID)
	if trimmedTemplateID != "" {
		resolved, err := orchestration.ResolveExecutionTemplate(trimmedTemplateID, inputs)
		if err != nil {
			return nil, "", err
		}
		return resolved.Tasks, resolved.Goal, nil
	}
	trimmedGoal := strings.TrimSpace(goal)
	if trimmedGoal == "" {
		return nil, "", errors.New("goal is required")
	}
	if daemon == nil {
		return nil, "", errors.New("daemon client unavailable")
	}
	tasks, err := daemon.DecomposeBaseAgentWithProvider(ctx, trimmedGoal, provider, "webui", requestID)
	if err != nil {
		return nil, "", err
	}
	plannerTasks := make([]orchestration.DecomposeTask, 0, len(tasks))
	for _, task := range tasks {
		plannerTasks = append(plannerTasks, orchestration.DecomposeTask{
			ID:      task.ID,
			Input:   task.Input,
			AgentID: task.AgentID,
		})
	}
	return plannerTasks, trimmedGoal, nil
}
