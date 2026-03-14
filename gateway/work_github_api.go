package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
)

func handleWorkGitHubAdapter(w http.ResponseWriter, r *http.Request, requestID string, cfg *GatewayConfig) {
	trimmed := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/work/adapters/github"), "/")
	if trimmed == "" {
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "adapter action is required"))
		return
	}
	switch trimmed {
	case "import":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		if _, ok := requireGatewayPermission(w, r, cfg, canLaunchExecutions, "E_RBAC_EXECUTION_LAUNCH", "role cannot import github work items"); !ok {
			return
		}
		var req workGitHubImportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
			return
		}
		item, err := importGitHubWorkItem(req)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, os.ErrNotExist) {
				status = http.StatusNotFound
			}
			writeJSON(w, status, gatewayErrBody("E_USAGE", err.Error()))
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"requestId": requestID, "result": "ok", "item": item})
	case "publish":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		if _, ok := requireGatewayPermission(w, r, cfg, canLaunchExecutions, "E_RBAC_EXECUTION_LAUNCH", "role cannot publish github work updates"); !ok {
			return
		}
		var req workGitHubPublishRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
			return
		}
		record, err := publishGitHubRun(req)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, os.ErrNotExist) {
				status = http.StatusNotFound
			}
			writeJSON(w, status, gatewayErrBody("E_USAGE", err.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"requestId": requestID, "result": "ok", "record": record})
	default:
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_NOT_FOUND", "unsupported github adapter action"))
	}
}
