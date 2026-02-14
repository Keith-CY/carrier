package api

import (
	"errors"
	"net/http"

	"carrier/daemon/internal/lifecycle"
)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}

func mapLifecycleError(err error) (status int, code, message string) {
	switch {
	case errors.Is(err, lifecycle.ErrAgentNotFound):
		return http.StatusNotFound, "E_AGENT_NOT_FOUND", err.Error()
	case errors.Is(err, lifecycle.ErrNotInstalled):
		return http.StatusConflict, "E_NOT_INSTALLED", err.Error()
	case errors.Is(err, lifecycle.ErrAlreadyRunning):
		return http.StatusConflict, "E_ALREADY_RUNNING", err.Error()
	case errors.Is(err, lifecycle.ErrAlreadyStopped):
		return http.StatusConflict, "E_ALREADY_STOPPED", err.Error()
	case errors.Is(err, lifecycle.ErrAgentRunning):
		return http.StatusConflict, "E_AGENT_RUNNING", err.Error()
	case errors.Is(err, lifecycle.ErrUpgradeNotSupported):
		return http.StatusBadRequest, "E_UPGRADE_NOT_SUPPORTED", err.Error()
	case errors.Is(err, lifecycle.ErrRuntimePrerequisites):
		return http.StatusUnprocessableEntity, "E_RUNTIME_PREREQUISITES", err.Error()
	case errors.Is(err, lifecycle.ErrMissingRequiredEnv):
		return http.StatusUnprocessableEntity, "E_MISSING_REQUIRED_ENV", err.Error()
	case errors.Is(err, lifecycle.ErrPortConflict):
		return http.StatusUnprocessableEntity, "E_PORT_CONFLICT", err.Error()
	case errors.Is(err, lifecycle.ErrUpgradeFailed):
		return http.StatusInternalServerError, "E_UPGRADE_FAILED", err.Error()
	case errors.Is(err, lifecycle.ErrUpgradeStrategyUnsupported):
		return http.StatusBadRequest, "E_UPGRADE_STRATEGY_UNSUPPORTED", err.Error()
	case errors.Is(err, lifecycle.ErrRemoteDiagnosisNotNeeded):
		return http.StatusConflict, "E_REMOTE_DIAG_NOT_NEEDED", err.Error()
	default:
		return http.StatusInternalServerError, "E_INTERNAL", err.Error()
	}
}
