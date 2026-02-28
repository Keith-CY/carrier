package gateway

import (
	"net/http"
	"testing"
)

func TestMapDaemonErrorToExternal(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{"empty_defaults", " ", http.StatusBadGateway, "E_COMMAND_FAILED", "daemon command failed"},
		{"not_found", "E_AGENT_NOT_FOUND", http.StatusNotFound, "E_AGENT_NOT_FOUND", "agent not found"},
		{"usage", "E_USAGE", http.StatusBadRequest, "E_USAGE", "invalid daemon request"},
		{"pair_invalid", "E_PAIR_CODE_INVALID", http.StatusBadRequest, "E_PAIR_CODE_INVALID", "pair code is invalid or expired; request a new PAIR_CODE and retry /pair"},
		{"session_required", "E_SESSION_REQUIRED", http.StatusUnauthorized, "E_SESSION_REQUIRED", "daemon request unauthorized"},
		{"not_installed", "E_NOT_INSTALLED", http.StatusBadRequest, "E_NOT_INSTALLED", "agent is not installed"},
		{"already_running", "E_ALREADY_RUNNING", http.StatusConflict, "E_ALREADY_RUNNING", "agent is already running"},
		{"already_stopped", "E_ALREADY_STOPPED", http.StatusConflict, "E_ALREADY_STOPPED", "agent is already stopped"},
		{"upgrade_not_supported", "E_UPGRADE_NOT_SUPPORTED", http.StatusBadRequest, "E_UPGRADE_NOT_SUPPORTED", "agent upgrade is not supported"},
		{"upgrade_strategy_unsupported", "E_UPGRADE_STRATEGY_UNSUPPORTED", http.StatusBadRequest, "E_UPGRADE_STRATEGY_UNSUPPORTED", "agent upgrade strategy is unsupported"},
		{"remote_diag_not_needed", "E_REMOTE_DIAG_NOT_NEEDED", http.StatusBadRequest, "E_REMOTE_DIAG_NOT_NEEDED", "remote diagnosis is not needed"},
		{"isolation_unavailable", "E_ISOLATION_UNAVAILABLE", http.StatusUnprocessableEntity, "E_ISOLATION_UNAVAILABLE", "isolation backend is unavailable"},
		{"isolation_start_failed", "E_ISOLATION_START_FAILED", http.StatusBadGateway, "E_ISOLATION_START_FAILED", "isolation runtime start failed"},
		{"unknown", "E_ANYTHING", http.StatusBadGateway, "E_ANYTHING", "daemon command failed"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, code, message := mapDaemonErrorToExternal(tc.input)
			if status != tc.wantStatus || code != tc.wantCode || message != tc.wantMsg {
				t.Fatalf("mapDaemonErrorToExternal(%q)=(%d,%q,%q), want (%d,%q,%q)", tc.input, status, code, message, tc.wantStatus, tc.wantCode, tc.wantMsg)
			}
		})
	}
}
