package gateway

import (
	"carrier/shared/work"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
)

func handleWorkProjects(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	trimmed := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/work/projects"), "/")
	if trimmed == "" {
		switch r.Method {
		case http.MethodGet:
			if _, ok := requireGatewayPermission(w, r, cfg, canViewExecutions, "E_RBAC_EXECUTION_VIEW", "role cannot view work projects"); !ok {
				return
			}
			projects, err := listWorkProjects()
			if err != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to list work projects", "list work projects", err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID, "result": "ok", "projects": projects})
			return
		case http.MethodPost:
			if _, ok := requireGatewayPermission(w, r, cfg, canLaunchExecutions, "E_RBAC_EXECUTION_LAUNCH", "role cannot create work projects"); !ok {
				return
			}
			var project work.Project
			if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
				return
			}
			saved, err := upsertWorkProject(project)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"requestId": requestID, "result": "ok", "project": saved})
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
	}

	projectID := strings.TrimSpace(trimmed)
	if strings.HasSuffix(projectID, "/sync") {
		projectID = strings.TrimSuffix(projectID, "/sync")
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		projectID = strings.Trim(projectID, "/")
		project, err := syncWorkProject(projectID)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, os.ErrNotExist) {
				status = http.StatusNotFound
			}
			writeJSON(w, status, gatewayErrBody("E_INTERNAL", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID, "result": "ok", "project": project})
		return
	}
	if strings.HasSuffix(projectID, "/archive") {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		projectID = strings.Trim(strings.TrimSuffix(projectID, "/archive"), "/")
		project, ok, err := getWorkProject(projectID)
		if err != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load work project", "get work project", err)
			return
		}
		if !ok {
			writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "work project not found"))
			return
		}
		project.State = work.ProjectStateArchived
		saved, err := upsertWorkProject(project)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID, "result": "ok", "project": saved})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	project, ok, err := getWorkProject(projectID)
	if err != nil {
		writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load work project", "get work project", err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "work project not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID, "result": "ok", "project": project})
}

func handleWorkItems(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	trimmed := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/work/items"), "/")
	if trimmed == "" {
		switch r.Method {
		case http.MethodGet:
			items, err := listWorkItems()
			if err != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to list work items", "list work items", err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID, "result": "ok", "items": items})
			return
		case http.MethodPost:
			var item work.WorkItem
			if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
				return
			}
			saved, err := upsertWorkItem(item)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"requestId": requestID, "result": "ok", "item": saved})
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
	}

	parts := strings.Split(trimmed, "/")
	itemID := strings.TrimSpace(parts[0])
	if itemID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "work item id is required"))
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			item, ok, err := getWorkItem(itemID)
			if err != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load work item", "get work item", err)
				return
			}
			if !ok {
				writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "work item not found"))
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID, "result": "ok", "item": item})
		case http.MethodPatch:
			item, ok, err := getWorkItem(itemID)
			if err != nil {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load work item", "get work item", err)
				return
			}
			if !ok {
				writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "work item not found"))
				return
			}
			var patch struct {
				Title       *string            `json:"title,omitempty"`
				Description *string            `json:"description,omitempty"`
				Acceptance  []string           `json:"acceptance,omitempty"`
				Priority    *work.WorkPriority `json:"priority,omitempty"`
				Labels      []string           `json:"labels,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
				return
			}
			if patch.Title != nil {
				item.Title = strings.TrimSpace(*patch.Title)
			}
			if patch.Description != nil {
				item.Description = strings.TrimSpace(*patch.Description)
			}
			if patch.Acceptance != nil {
				item.Acceptance = append([]string(nil), patch.Acceptance...)
			}
			if patch.Priority != nil {
				item.Priority = *patch.Priority
			}
			if patch.Labels != nil {
				item.Labels = append([]string(nil), patch.Labels...)
			}
			saved, err := upsertWorkItem(item)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID, "result": "ok", "item": saved})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		}
		return
	}

	action := strings.TrimSpace(parts[1])
	switch action {
	case "claim":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var body struct {
			RunID string `json:"runId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
			return
		}
		item, err := setWorkItemState(itemID, work.WorkItemStateClaimed, body.RunID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID, "result": "ok", "item": item})
	case "cancel":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		item, ok, err := getWorkItem(itemID)
		if err != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load work item", "get work item", err)
			return
		}
		if !ok {
			writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "work item not found"))
			return
		}
		if strings.TrimSpace(item.LatestRunID) != "" {
			if _, err := cancelWorkRun(item.LatestRunID); err != nil && !errors.Is(err, os.ErrNotExist) {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to cancel work run", "cancel work run", err)
				return
			}
		}
		item, err = setWorkItemState(itemID, work.WorkItemStateCancelled, item.LatestRunID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID, "result": "ok", "item": item})
	case "complete":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		item, ok, err := getWorkItem(itemID)
		if err != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load work item", "get work item", err)
			return
		}
		if !ok {
			writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "work item not found"))
			return
		}
		if strings.TrimSpace(item.LatestRunID) != "" {
			if _, err := completeWorkRun(item.LatestRunID); err != nil && !errors.Is(err, os.ErrNotExist) {
				writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to complete work run", "complete work run", err)
				return
			}
		}
		item, err = setWorkItemState(itemID, work.WorkItemStateDone, item.LatestRunID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID, "result": "ok", "item": item})
	case "runs":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		var body struct {
			Backend work.RunBackend `json:"backend,omitempty"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		run, err := startWorkRun(itemID, body.Backend)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"requestId": requestID, "result": "ok", "run": run})
	default:
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported work item action"))
	}
}

func handleWorkRuns(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	trimmed := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/work/runs"), "/")
	if trimmed == "" {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		runs, err := listWorkRuns()
		if err != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to list work runs", "list work runs", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID, "result": "ok", "runs": runs})
		return
	}
	parts := strings.Split(trimmed, "/")
	runID := strings.TrimSpace(parts[0])
	if runID == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "run id is required"))
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		run, ok, err := getWorkRun(runID)
		if err != nil {
			writeInternalGatewayError(w, http.StatusInternalServerError, "E_INTERNAL", "failed to load work run", "get work run", err)
			return
		}
		if !ok {
			writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "work run not found"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID, "result": "ok", "run": run})
		return
	}
	action := strings.TrimSpace(parts[1])
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
		return
	}
	var (
		run work.Run
		err error
	)
	switch action {
	case "resume":
		run, err = resumeWorkRun(runID)
	case "cancel":
		run, err = cancelWorkRun(runID)
	case "reclaim":
		run, err = reclaimWorkRun(runID)
	case "cleanup":
		if err = cleanupWorkRun(runID); err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID, "result": "ok", "cleaned": true})
			return
		}
	default:
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported work run action"))
		return
	}
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		} else {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, gatewayErrBody("E_USAGE", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID, "result": "ok", "run": run})
}
