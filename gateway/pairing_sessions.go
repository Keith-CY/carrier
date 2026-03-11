package gateway

import (
	"net/http"
	"strings"
)

type pairingSessionSummary struct {
	Provider        string `json:"provider"`
	ChatID          string `json:"chatId"`
	PairState       string `json:"pairState"`
	PairMethod      string `json:"pairMethod"`
	SupportsPairing bool   `json:"supportsPairing"`
	CreatedAt       string `json:"createdAt"`
	LastSeen        string `json:"lastSeenAt"`
}

func ChannelSupportsPairing(channel string) bool {
	desc, ok := LookupChannelDescriptor(channel)
	if !ok {
		return false
	}
	return desc.Capabilities.SupportsPairing
}

func (s *SessionStore) ListPairingStatus(provider string) []pairingSessionSummary {
	if s == nil {
		return nil
	}
	records := s.ListSessions(provider)
	items := make([]pairingSessionSummary, 0, len(records))
	for _, rec := range records {
		if rec == nil {
			continue
		}
		items = append(items, pairingSessionSummary{
			Provider:        rec.Provider,
			ChatID:          rec.ChatID,
			PairState:       rec.PairState,
			PairMethod:      rec.PairMethod,
			SupportsPairing: ChannelSupportsPairing(rec.Provider),
			CreatedAt:       rec.CreatedAt,
			LastSeen:        rec.LastSeenAt,
		})
	}
	return items
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

	items := sessions.ListPairingStatus(provider)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"provider":  provider,
		"sessions":  items,
	})
}
