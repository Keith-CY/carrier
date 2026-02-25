package gateway

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

type telegramTransportStatus struct {
	RequestedMode string `json:"requested_mode"`
	SelectedMode  string `json:"selected_mode"`
	ReasonCode    string `json:"reason_code,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Hint          string `json:"hint,omitempty"`
	UpdatedAt     string `json:"updated_at"`
}

var telegramTransportState = struct {
	mu     sync.RWMutex
	status telegramTransportStatus
}{
	status: telegramTransportStatus{
		RequestedMode: telegramTransportAuto,
		SelectedMode:  "unknown",
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	},
}

func setTelegramTransportStatus(requestedMode, selectedMode, reasonCode, reason, hint string) {
	telegramTransportState.mu.Lock()
	defer telegramTransportState.mu.Unlock()
	telegramTransportState.status = telegramTransportStatus{
		RequestedMode: strings.TrimSpace(requestedMode),
		SelectedMode:  strings.TrimSpace(selectedMode),
		ReasonCode:    strings.TrimSpace(reasonCode),
		Reason:        strings.TrimSpace(reason),
		Hint:          strings.TrimSpace(hint),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	if telegramTransportState.status.RequestedMode == "" {
		telegramTransportState.status.RequestedMode = telegramTransportAuto
	}
	if telegramTransportState.status.SelectedMode == "" {
		telegramTransportState.status.SelectedMode = "unknown"
	}
}

func snapshotTelegramTransportStatus() telegramTransportStatus {
	telegramTransportState.mu.RLock()
	defer telegramTransportState.mu.RUnlock()
	return telegramTransportState.status
}

func handleTelegramTransportStatus(w http.ResponseWriter, _ *http.Request, requestID string) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"transport": snapshotTelegramTransportStatus(),
	})
}
