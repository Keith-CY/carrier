package gateway

import (
	"fmt"
	"strings"
)

// ChannelID is a canonical gateway channel identifier.
type ChannelID string

const (
	ChannelTelegram ChannelID = "telegram"
	ChannelDiscord  ChannelID = "discord"
	ChannelFeishu   ChannelID = "feishu"
	ChannelWebUI    ChannelID = "webui"
)

// ChannelCapabilities captures channel transport and auth behaviors.
type ChannelCapabilities struct {
	SupportsWebhook       bool
	SupportsPolling       bool
	SupportsPairing       bool
	RequiresBotToken      bool
	RequiresWebhookSecret bool
	SupportsWebUI         bool
	SupportsGatewayCmd    bool
	SupportsProviderSetup bool
}

// ChannelDescriptor is the canonical metadata record for a channel.
type ChannelDescriptor struct {
	ID           ChannelID
	DisplayName  string
	Capabilities ChannelCapabilities
}

var channelDescriptors = map[ChannelID]ChannelDescriptor{
	ChannelTelegram: {
		ID:          ChannelTelegram,
		DisplayName: "Telegram",
		Capabilities: ChannelCapabilities{
			SupportsWebhook:       true,
			SupportsPolling:       true,
			SupportsPairing:       true,
			RequiresBotToken:      true,
			RequiresWebhookSecret: false,
			SupportsWebUI:         true,
			SupportsGatewayCmd:    true,
			SupportsProviderSetup: true,
		},
	},
	ChannelDiscord: {
		ID:          ChannelDiscord,
		DisplayName: "Discord",
		Capabilities: ChannelCapabilities{
			SupportsWebhook:       true,
			SupportsPolling:       false,
			SupportsPairing:       false,
			RequiresBotToken:      true,
			RequiresWebhookSecret: true,
			SupportsWebUI:         true,
			SupportsGatewayCmd:    true,
			SupportsProviderSetup: true,
		},
	},
	ChannelFeishu: {
		ID:          ChannelFeishu,
		DisplayName: "Feishu",
		Capabilities: ChannelCapabilities{
			SupportsWebhook:       true,
			SupportsPolling:       false,
			SupportsPairing:       false,
			RequiresBotToken:      true,
			RequiresWebhookSecret: true,
			SupportsWebUI:         true,
			SupportsGatewayCmd:    true,
			SupportsProviderSetup: true,
		},
	},
	ChannelWebUI: {
		ID:          ChannelWebUI,
		DisplayName: "WebUI",
		Capabilities: ChannelCapabilities{
			SupportsWebhook:       false,
			SupportsPolling:       false,
			SupportsPairing:       false,
			RequiresBotToken:      false,
			RequiresWebhookSecret: false,
			SupportsWebUI:         true,
			SupportsGatewayCmd:    false,
			SupportsProviderSetup: false,
		},
	},
}

var channelDescriptorOrder = []ChannelID{
	ChannelTelegram,
	ChannelDiscord,
	ChannelFeishu,
	ChannelWebUI,
}

// SupportedChannelDescriptors returns all known channel descriptors.
func SupportedChannelDescriptors() []ChannelDescriptor {
	descriptors := make([]ChannelDescriptor, 0, len(channelDescriptorOrder))
	for _, id := range channelDescriptorOrder {
		descriptors = append(descriptors, channelDescriptors[id])
	}
	return descriptors
}

// LookupChannelDescriptor resolves a channel descriptor by canonicalized ID.
func LookupChannelDescriptor(id string) (ChannelDescriptor, bool) {
	channelID, err := NormalizeChannelID(id)
	if err != nil {
		return ChannelDescriptor{}, false
	}
	desc, ok := channelDescriptors[channelID]
	return desc, ok
}

// NormalizeChannelID normalizes and validates a channel identifier.
func NormalizeChannelID(raw string) (ChannelID, error) {
	channelID := ChannelID(strings.ToLower(strings.TrimSpace(raw)))
	if channelID == "" {
		return "", fmt.Errorf("unsupported channel %q", raw)
	}
	if _, ok := channelDescriptors[channelID]; !ok {
		return "", fmt.Errorf("unsupported channel %q", raw)
	}
	return channelID, nil
}

func supportsGatewayCommandsForChannel(raw string) bool {
	desc, ok := LookupChannelDescriptor(raw)
	if !ok {
		return false
	}
	// Keep command parser behavior strict to canonical IDs.
	if strings.TrimSpace(raw) != string(desc.ID) {
		return false
	}
	return desc.Capabilities.SupportsGatewayCmd
}
