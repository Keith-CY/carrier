package gateway

import (
	"log"
	"net/http"
	"strings"
)

const errCodeStatePersistence = "E_STATE_PERSISTENCE"

func mapDaemonErrorToExternal(code string) (status int, normalizedCode, message string) {
	normalizedCode = strings.TrimSpace(code)
	if normalizedCode == "" {
		normalizedCode = "E_COMMAND_FAILED"
	}

	switch normalizedCode {
	case "E_AGENT_NOT_FOUND":
		return http.StatusNotFound, normalizedCode, "agent not found"
	case "E_USAGE":
		return http.StatusBadRequest, normalizedCode, "invalid daemon request"
	case "E_PAIR_CODE_INVALID":
		return http.StatusBadRequest, normalizedCode, "pair code is invalid or expired; request a new PAIR_CODE and retry /pair"
	case "E_SESSION_REQUIRED":
		return http.StatusUnauthorized, normalizedCode, "daemon request unauthorized"
	case "E_NOT_INSTALLED":
		return http.StatusBadRequest, normalizedCode, "agent is not installed"
	case "E_ALREADY_RUNNING":
		return http.StatusConflict, normalizedCode, "agent is already running"
	case "E_ALREADY_STOPPED":
		return http.StatusConflict, normalizedCode, "agent is already stopped"
	case "E_UPGRADE_NOT_SUPPORTED":
		return http.StatusBadRequest, normalizedCode, "agent upgrade is not supported"
	case "E_UPGRADE_STRATEGY_UNSUPPORTED":
		return http.StatusBadRequest, normalizedCode, "agent upgrade strategy is unsupported"
	case "E_REMOTE_DIAG_NOT_NEEDED":
		return http.StatusBadRequest, normalizedCode, "remote diagnosis is not needed"
	case "E_ISOLATION_UNAVAILABLE":
		return http.StatusUnprocessableEntity, normalizedCode, "isolation backend is unavailable"
	case "E_ISOLATION_START_FAILED":
		return http.StatusBadGateway, normalizedCode, "isolation runtime start failed"
	default:
		return http.StatusBadGateway, normalizedCode, "daemon command failed"
	}
}

func writeInternalGatewayError(w http.ResponseWriter, status int, code, publicMessage, context string, err error) {
	if err != nil {
		log.Printf("[gateway] %s: %s", context, RedactErrorMessage(err.Error()))
	}
	writeJSON(w, status, gatewayErrBody(code, publicMessage))
}

func writeStatePersistenceError(w http.ResponseWriter, requestID, action, agentID, instanceID string, err error) {
	if err != nil {
		log.Printf("[gateway] managed state persistence failed: %s", RedactErrorMessage(err.Error()))
	}

	// Preserve partial-failure semantics for clients that can recover.
	payload := gatewayErrBody(errCodeStatePersistence, "operation succeeded but failed to persist managed state")
	payload["requestId"] = requestID
	payload["partialSuccess"] = true
	if action = strings.TrimSpace(action); action != "" {
		payload["action"] = action
	}
	if agentID = strings.TrimSpace(agentID); agentID != "" {
		payload["agentId"] = agentID
	}
	if instanceID = strings.TrimSpace(instanceID); instanceID != "" {
		payload["instanceId"] = instanceID
	}
	writeJSON(w, http.StatusInternalServerError, payload)
}
