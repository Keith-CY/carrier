package openclawcfg

import "strings"
import "carrier/shared/catalog"

const (
	CarrierFileSecretProviderAlias = "carrier_file"
	// CarrierFileSecretsPath is relative to OpenClaw's working directory (its workspace).
	// This is intentional - carrier-secrets.json is rsync'd to the instance's workspace directory,
	// and OpenClaw is started from there. Do not change to absolute path.
	CarrierFileSecretsPath    = "./carrier-secrets.json"
	CarrierSecretFilePatchKey = "_carrier_openclaw_secret_file"
)

type ManagedPayloadParams struct {
	ChannelID           string
	ChannelToken        string
	ChannelSetupPending bool
	AllowFrom           []string
	ProviderID          string
	ProviderKey         string
	ProviderBaseURL     string
	IncludeAPIKeyRef    bool
	ModelID             string
	WorkspacePath       string
}

func BuildManagedConfigPayload(params ManagedPayloadParams) map[string]interface{} {
	providerItem := BuildProviderEntry(
		params.ProviderID,
		params.ProviderKey,
		params.ProviderBaseURL,
		params.ModelID,
		params.IncludeAPIKeyRef,
	)

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
				"workspace": strings.TrimSpace(params.WorkspacePath),
				"model":     map[string]interface{}{"primary": strings.TrimSpace(params.ModelID)},
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

func BuildProviderEntry(providerID, providerKey, providerBaseURL, modelID string, includeAPIKeyRef bool) map[string]interface{} {
	baseURL := strings.TrimSpace(providerBaseURL)
	if baseURL == "" {
		baseURL = defaultProviderBaseURL(providerID, providerKey)
	}
	modelName := strings.TrimSpace(modelID)
	if _, name, ok := strings.Cut(modelName, "/"); ok && strings.TrimSpace(name) != "" {
		modelName = strings.TrimSpace(name)
	}
	if modelName == "" {
		modelName = "default"
	}

	providerItem := map[string]interface{}{
		"baseUrl": baseURL,
		"models": []interface{}{
			map[string]interface{}{
				"id":   modelName,
				"name": modelName,
			},
		},
	}
	if includeAPIKeyRef {
		providerItem["apiKey"] = map[string]interface{}{
			"source":   "file",
			"provider": CarrierFileSecretProviderAlias,
			"id":       ProviderSecretPointer(providerKey),
		}
	}
	if catalog.IsOpenAICodexProviderID(providerID) {
		providerItem["auth"] = "oauth"
	}
	return providerItem
}

func defaultProviderBaseURL(providerID, providerKey string) string {
	normalizedID := catalog.NormalizeProviderID(providerID)
	switch normalizedID {
	case "openai", "openai-codex", "openai-compatible":
		return "https://api.openai.com/v1"
	case "anthropic":
		return "https://api.anthropic.com/v1"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	case "ollama":
		return "http://localhost:11434/v1"
	}

	switch strings.ToLower(strings.TrimSpace(providerKey)) {
	case "openai":
		return "https://api.openai.com/v1"
	case "anthropic":
		return "https://api.anthropic.com/v1"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	case "ollama":
		return "http://localhost:11434/v1"
	default:
		return "https://api.openai.com/v1"
	}
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
