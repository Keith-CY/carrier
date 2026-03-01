package openclawcfg

import "strings"

const (
	CarrierFileSecretProviderAlias = "carrier_file"
	CarrierFileSecretsPath         = "~/.openclaw/carrier-secrets.json"
	CarrierSecretFilePatchKey      = "_carrier_openclaw_secret_file"
)

type ManagedPayloadParams struct {
	ChannelID           string
	ChannelToken        string
	ChannelSetupPending bool
	AllowFrom           []string
	ProviderID          string
	ProviderKey         string
	IncludeAPIKeyRef    bool
	ModelID             string
	WorkspacePath       string
}

func BuildManagedConfigPayload(params ManagedPayloadParams) map[string]interface{} {
	providerItem := BuildProviderEntry(params.ProviderID, params.ProviderKey, params.IncludeAPIKeyRef)

	channels := map[string]interface{}{}
	if channelID := strings.TrimSpace(params.ChannelID); channelID != "" {
		channelConfig := map[string]interface{}{
			"enabled": true,
		}
		if params.ChannelSetupPending {
			channelConfig["enabled"] = false
			channelConfig["setup_pending"] = true
		} else {
			channelConfig[ChannelTokenField(channelID)] = params.ChannelToken
		}
		if len(params.AllowFrom) > 0 {
			channelConfig["allowFrom"] = params.AllowFrom
		}
		channels[channelID] = channelConfig
	}

	return map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"workspace":           strings.TrimSpace(params.WorkspacePath),
				"model":               map[string]interface{}{"primary": strings.TrimSpace(params.ModelID)},
				"maxTokens":           8192,
				"temperature":         0.7,
				"maxToolIterations":   20,
				"restrictToWorkspace": true,
			},
		},
		"models": map[string]interface{}{
			"providers": map[string]interface{}{
				strings.TrimSpace(params.ProviderKey): providerItem,
			},
		},
		"secrets": map[string]interface{}{
			"providers": map[string]interface{}{
				CarrierFileSecretProviderAlias: map[string]interface{}{
					"source": "file",
					"path":   CarrierFileSecretsPath,
					"mode":   "json",
				},
			},
			"defaults": map[string]interface{}{
				"file": CarrierFileSecretProviderAlias,
			},
		},
		"channels": channels,
	}
}

func BuildProviderEntry(providerID, providerKey string, includeAPIKeyRef bool) map[string]interface{} {
	providerItem := map[string]interface{}{}
	if includeAPIKeyRef {
		providerItem["apiKey"] = map[string]interface{}{
			"source":   "file",
			"provider": CarrierFileSecretProviderAlias,
			"id":       ProviderSecretPointer(providerKey),
		}
	}
	if strings.EqualFold(strings.TrimSpace(providerID), "openai-codex") {
		providerItem["auth"] = "oauth"
	}
	return providerItem
}

func ProviderSecretPointer(providerKey string) string {
	escaped := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(providerKey), "~", "~0"), "/", "~1")
	if escaped == "" {
		escaped = "openai"
	}
	return "/providers/" + escaped + "/apiKey"
}

func ChannelTokenField(channelID string) string {
	if strings.EqualFold(strings.TrimSpace(channelID), "telegram") {
		return "botToken"
	}
	return "token"
}
