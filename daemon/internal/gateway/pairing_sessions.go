package gateway

import (
	"net/http"
	"strings"
)

type pairingSessionSummary struct {
	Provider  string `json:"provider"`
	ChatID    string `json:"chatId"`
	CreatedAt string `json:"createdAt"`
	LastSeen  string `json:"lastSeenAt"`
}

func handlePairingSessions(w http.ResponseWriter, r *http.Request, requestID string, sessions *SessionStore) {
	if sessions == nil {
		writeJSON(w, http.StatusInternalServerError, gatewayErrBody("E_INTERNAL", "session store is not initialized"))
		return
	}
	provider := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("provider")))
	if provider == "" {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "provider query parameter is required"))
		return
	}
	if !IsValidProviderType(provider) {
		writeJSON(w, http.StatusBadRequest, gatewayErrBody("E_USAGE", "unsupported provider"))
		return
	}

	records := sessions.ListSessions(provider)
	items := make([]pairingSessionSummary, 0, len(records))
	for _, rec := range records {
		if rec == nil {
			continue
		}
		items = append(items, pairingSessionSummary{
			Provider:  rec.Provider,
			ChatID:    rec.ChatID,
			CreatedAt: rec.CreatedAt,
			LastSeen:  rec.LastSeenAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"provider":  provider,
		"sessions":  items,
	})
}
