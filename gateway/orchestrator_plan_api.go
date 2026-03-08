package gateway

import (
	"carrier/shared/orchestration"
	"encoding/json"
	"net/http"
	"strings"
)

type orchestratorPlanRequest struct {
	Goal           string   `json:"goal"`
	Provider       string   `json:"provider,omitempty"`
	HostIDs        []string `json:"hostIds,omitempty"`
	HostLabels     []string `json:"hostLabels,omitempty"`
	MaxConcurrency int      `json:"maxConcurrency,omitempty"`
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

	var req orchestratorPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
		return
	}
	req.Goal = strings.TrimSpace(req.Goal)
	req.Provider = strings.TrimSpace(req.Provider)
	if req.Goal == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "goal is required"))
		return
	}
	if daemon == nil {
		writeInternalGatewayError(w, http.StatusBadGateway, "E_UPSTREAM", "daemon client unavailable", "create daemon client for orchestration planning", nil)
		return
	}

	tasks, err := daemon.DecomposeBaseAgentWithProvider(r.Context(), req.Goal, req.Provider, "webui", requestID)
	if err != nil {
		writeInternalGatewayError(w, http.StatusBadGateway, "E_UPSTREAM", "failed to decompose goal", "decompose base agent goal", err)
		return
	}

	plannerTasks := make([]orchestration.DecomposeTask, 0, len(tasks))
	for _, task := range tasks {
		plannerTasks = append(plannerTasks, orchestration.DecomposeTask{
			ID:      task.ID,
			Input:   task.Input,
			AgentID: task.AgentID,
		})
	}

	plan, err := orchestration.BuildPlan(orchestration.BuildPlanInput{
		Goal:           req.Goal,
		Provider:       req.Provider,
		HostIDs:        req.HostIDs,
		HostLabels:     req.HostLabels,
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
