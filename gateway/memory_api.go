package gateway

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

func handleMemory(w http.ResponseWriter, r *http.Request, requestID string, daemon *DaemonClient, cfg *GatewayConfig) {
	trimmed := strings.Trim(strings.TrimPrefix(strings.TrimSpace(r.URL.Path), "/api/v1/memory"), "/")
	if trimmed == "" {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		if _, ok := requireGatewayPermission(w, r, cfg, canViewExecutions, "E_RBAC_MEMORY_VIEW", "role cannot view memory"); !ok {
			return
		}
		path := "/api/v2/memory"
		if subject := strings.TrimSpace(r.URL.Query().Get("subject")); subject != "" {
			path += "?subject=" + url.QueryEscape(subject)
		}
		writeDaemonJSONProxy(w, r, daemon, requestID, http.MethodGet, path, nil, "webui:memory:list")
		return
	}

	switch trimmed {
	case "search":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		if _, ok := requireGatewayPermission(w, r, cfg, canViewExecutions, "E_RBAC_MEMORY_VIEW", "role cannot search memory"); !ok {
			return
		}
		proxyDaemonJSONBody(w, r, daemon, requestID, http.MethodPost, "/api/v2/memory/search", "webui:memory:search")
		return
	case "instance/attach":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		if _, ok := requireGatewayPermission(w, r, cfg, canLaunchExecutions, "E_RBAC_MEMORY_ATTACH", "role cannot attach memory to instances"); !ok {
			return
		}
		proxyDaemonJSONBody(w, r, daemon, requestID, http.MethodPost, "/api/v2/memory/instance/attach", "webui:memory:attach")
		return
	case "instance/detach":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		if _, ok := requireGatewayPermission(w, r, cfg, canLaunchExecutions, "E_RBAC_MEMORY_ATTACH", "role cannot detach memory from instances"); !ok {
			return
		}
		proxyDaemonJSONBody(w, r, daemon, requestID, http.MethodPost, "/api/v2/memory/instance/detach", "webui:memory:detach")
		return
	case "instance/distill":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, gatewayErrBody("E_METHOD_NOT_ALLOWED", "method not allowed"))
			return
		}
		if _, ok := requireGatewayPermission(w, r, cfg, canLaunchExecutions, "E_RBAC_MEMORY_DISTILL", "role cannot distill memory from instances"); !ok {
			return
		}
		proxyDaemonJSONBody(w, r, daemon, requestID, http.MethodPost, "/api/v2/memory/instance/distill", "webui:memory:distill")
		return
	default:
		writeJSON(w, http.StatusNotFound, gatewayErrBody("E_USAGE", "unsupported memory action"))
	}
}

func proxyDaemonJSONBody(w http.ResponseWriter, r *http.Request, daemon *DaemonClient, requestID, method, path, actor string) {
	body, err := readBodyWithLimit(r, defaultMaxCommandBodyBytes)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", err.Error()))
		return
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "request body must be valid JSON"))
		return
	}
	writeDaemonJSONProxy(w, r, daemon, requestID, method, path, json.RawMessage(body), actor)
}

func writeDaemonJSONProxy(w http.ResponseWriter, r *http.Request, daemon *DaemonClient, requestID, method, path string, body any, actor string) {
	if daemon == nil {
		writeJSON(w, http.StatusBadGateway, gatewayErrBody("E_COMMAND_FAILED", "daemon client is unavailable"))
		return
	}
	raw, err := daemon.request(r.Context(), method, path, body, actor, requestID)
	if err != nil {
		writeDaemonAPIError(w, err)
		return
	}
	var payload interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		writeInternalGatewayError(w, http.StatusBadGateway, "E_COMMAND_FAILED", "daemon returned invalid JSON", "decode daemon memory response", err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}
