package gateway

import (
	"net/http"
	"strings"
)

type ChannelStatus struct {
	ID                    string `json:"id"`
	DisplayName           string `json:"displayName"`
	SupportsWebhook       bool   `json:"supportsWebhook"`
	SupportsPolling       bool   `json:"supportsPolling"`
	SupportsPairing       bool   `json:"supportsPairing"`
	RequiresBotToken      bool   `json:"requiresBotToken"`
	RequiresWebhookSecret bool   `json:"requiresWebhookSecret"`
	SupportsWebUI         bool   `json:"supportsWebUI"`
	SupportsGatewayCmd    bool   `json:"supportsGatewayCmd"`
	SupportsProviderSetup bool   `json:"supportsProviderSetup"`
	Configured            bool   `json:"configured"`
	ConfiguredAt          string `json:"configuredAt,omitempty"`
}

func BuildChannelStatuses(setup *SetupStore) []ChannelStatus {
	descriptors := SupportedChannelDescriptors()
	statuses := make([]ChannelStatus, 0, len(descriptors))

	var configuredProvider string
	var configuredAt string
	if setup != nil {
		if redacted := setup.GetRedacted(); redacted != nil {
			configuredProvider = strings.TrimSpace(string(redacted.Provider))
			configuredAt = strings.TrimSpace(redacted.ConfiguredAt)
		}
	}

	for _, desc := range descriptors {
		status := ChannelStatus{
			ID:                    string(desc.ID),
			DisplayName:           desc.DisplayName,
			SupportsWebhook:       desc.Capabilities.SupportsWebhook,
			SupportsPolling:       desc.Capabilities.SupportsPolling,
			SupportsPairing:       desc.Capabilities.SupportsPairing,
			RequiresBotToken:      desc.Capabilities.RequiresBotToken,
			RequiresWebhookSecret: desc.Capabilities.RequiresWebhookSecret,
			SupportsWebUI:         desc.Capabilities.SupportsWebUI,
			SupportsGatewayCmd:    desc.Capabilities.SupportsGatewayCmd,
			SupportsProviderSetup: desc.Capabilities.SupportsProviderSetup,
			Configured:            configuredProvider == string(desc.ID),
		}
		if status.Configured {
			status.ConfiguredAt = configuredAt
		}
		statuses = append(statuses, status)
	}
	return statuses
}

func BuildChannelStatus(channelID string, configured bool, configuredAt string) ChannelStatus {
	desc, ok := LookupChannelDescriptor(channelID)
	if !ok {
		return ChannelStatus{}
	}
	status := ChannelStatus{
		ID:                    string(desc.ID),
		DisplayName:           desc.DisplayName,
		SupportsWebhook:       desc.Capabilities.SupportsWebhook,
		SupportsPolling:       desc.Capabilities.SupportsPolling,
		SupportsPairing:       desc.Capabilities.SupportsPairing,
		RequiresBotToken:      desc.Capabilities.RequiresBotToken,
		RequiresWebhookSecret: desc.Capabilities.RequiresWebhookSecret,
		SupportsWebUI:         desc.Capabilities.SupportsWebUI,
		SupportsGatewayCmd:    desc.Capabilities.SupportsGatewayCmd,
		SupportsProviderSetup: desc.Capabilities.SupportsProviderSetup,
		Configured:            configured,
		ConfiguredAt:          strings.TrimSpace(configuredAt),
	}
	return status
}

func handleProviderAuthStatusAPI(w http.ResponseWriter, r *http.Request, requestID string) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"providers": ListProviderAuthStatuses(),
	})
}

func handleChannelStatusAPI(w http.ResponseWriter, r *http.Request, requestID string, setup *SetupStore) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId": requestID,
		"result":    "ok",
		"channels":  BuildChannelStatuses(setup),
	})
}
