package gateway

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"carrier/shared/catalog"
	sharedconfig "carrier/shared/config"
	"carrier/shared/openclawcfg"
)

func resolveManagedConfigPath(home string, cfg managedAgentConfig, renderer managedRendererSelection) string {
	configPath := strings.TrimSpace(renderer.ConfigPath)
	if configPath == "" {
		configPath = filepath.Join(home, cfg.ConfigDir, cfg.ConfigFile)
	}
	if strings.HasPrefix(configPath, "~/") {
		return filepath.Join(home, strings.TrimPrefix(configPath, "~/"))
	}
	if filepath.IsAbs(configPath) {
		return filepath.Clean(configPath)
	}
	return filepath.Join(home, cfg.ConfigDir, configPath)
}

func renderManagedConfigBytes(
	renderer managedRendererSelection,
	cfg managedAgentConfig,
	channelID, channelToken string,
	channelSetupPending bool,
	allowFrom []string,
	provider *LLMProvider,
	providerKey, providerBaseURL, providerToken string,
	profiles []managedModelProfile,
	primaryProfile managedModelProfile,
	workspacePath string,
) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(renderer.ConfigFormat)) {
	case "toml":
		if strings.EqualFold(cfg.ID, "zeroclaw") {
			return renderZeroClawConfigTOML(channelID, channelToken, channelSetupPending, allowFrom, providerKey, providerToken, primaryProfile.ModelID, profiles), nil
		}
		return nil, fmt.Errorf("toml renderer is not supported for %s", cfg.ID)
	case "json":
		var payload map[string]interface{}
		if strings.EqualFold(cfg.ID, "openclaw") {
			payload = buildManagedOpenClawJSONConfigPayload(channelID, channelToken, channelSetupPending, allowFrom, provider, providerKey, providerBaseURL, providerToken, primaryProfile.ModelID, workspacePath)
		} else {
			payload = buildManagedPicoClawJSONConfigPayload(channelID, channelToken, channelSetupPending, allowFrom, provider, providerKey, providerToken, profiles, primaryProfile, workspacePath)
		}
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(raw, '\n'), nil
	default:
		return nil, fmt.Errorf("unsupported config format %q", renderer.ConfigFormat)
	}
}

func buildManagedPicoClawJSONConfigPayload(
	channelID, channelToken string,
	channelSetupPending bool,
	allowFrom []string,
	provider *LLMProvider,
	providerKey, providerToken string,
	profiles []managedModelProfile,
	primaryProfile managedModelProfile,
	workspacePath string,
) map[string]interface{} {
	modelList := make([]interface{}, 0, len(profiles))
	providerProfiles := map[string]interface{}{}
	providers := map[string]interface{}{}
	for _, profile := range profiles {
		modelItem := map[string]interface{}{
			"model_name":      profile.ProfileName,
			"model":           profile.ModelID,
			"protocol_family": profile.ProtocolFamily,
		}
		if strings.TrimSpace(profile.ModelAlias) != "" {
			modelItem["model_alias"] = profile.ModelAlias
		}
		if strings.TrimSpace(profile.BaseURL) != "" {
			modelItem["base_url"] = profile.BaseURL
		}
		if strings.TrimSpace(profile.AuthMethod) != "" {
			modelItem["auth_method"] = profile.AuthMethod
		}
		modelList = append(modelList, modelItem)

		profileEntry := map[string]interface{}{
			"provider":        profile.ProviderKey,
			"provider_id":     profile.ProviderID,
			"protocol_family": profile.ProtocolFamily,
			"model":           profile.ModelID,
			"credential_ref":  provider.ID,
		}
		if strings.TrimSpace(profile.ModelAlias) != "" {
			profileEntry["model_alias"] = profile.ModelAlias
		}
		if strings.TrimSpace(profile.BaseURL) != "" {
			profileEntry["base_url"] = profile.BaseURL
		}
		if strings.TrimSpace(profile.AuthMethod) != "" {
			profileEntry["auth_method"] = profile.AuthMethod
		}
		providerProfiles[profile.ProfileName] = profileEntry

		if _, ok := providers[profile.ProviderKey]; !ok {
			providerItem := map[string]interface{}{
				"credential_ref": provider.ID,
			}
			if strings.TrimSpace(profile.AuthMethod) != "" {
				providerItem["auth_method"] = profile.AuthMethod
			} else if providerToken != "" {
				providerItem["api_key"] = providerToken
			}
			providers[profile.ProviderKey] = providerItem
		}
	}

	channels := map[string]interface{}{}
	if strings.TrimSpace(channelID) != "" {
		channelConfig := map[string]interface{}{
			"enabled":    true,
			"allow_from": allowFrom,
		}
		if channelSetupPending {
			channelConfig["enabled"] = false
			channelConfig["setup_pending"] = true
		} else {
			channelConfig["token"] = channelToken
		}
		channels[channelID] = channelConfig
	}

	return map[string]interface{}{
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"workspace":             workspacePath,
				"provider":              providerKey,
				"model":                 deriveManagedModelName(primaryProfile.ModelID),
				"max_tokens":            8192,
				"temperature":           0.7,
				"max_tool_iterations":   20,
				"restrict_to_workspace": true,
			},
		},
		"model_list":        modelList,
		"provider_profiles": providerProfiles,
		"providers":         providers,
		"channels":          channels,
	}
}

func buildManagedOpenClawJSONConfigPayload(
	channelID, channelToken string,
	channelSetupPending bool,
	allowFrom []string,
	provider *LLMProvider,
	providerKey, providerBaseURL, providerToken, modelID, workspacePath string,
) map[string]interface{} {
	return openclawcfg.BuildManagedConfigPayload(openclawcfg.ManagedPayloadParams{
		ChannelID:           channelID,
		ChannelToken:        channelToken,
		ChannelSetupPending: channelSetupPending,
		AllowFrom:           allowFrom,
		ProviderID:          provider.ID,
		ProviderKey:         providerKey,
		ProviderBaseURL:     providerBaseURL,
		IncludeAPIKeyRef:    strings.TrimSpace(providerToken) != "",
		ModelID:             modelID,
		WorkspacePath:       workspacePath,
	})
}

func renderZeroClawConfigTOML(
	channelID, channelToken string,
	channelSetupPending bool,
	allowFrom []string,
	providerKey, providerToken, modelID string,
	profiles []managedModelProfile,
) []byte {
	if strings.TrimSpace(providerKey) == "" {
		providerKey = "openai"
	}
	if strings.TrimSpace(modelID) == "" {
		modelID = "anthropic/claude-sonnet-4-6"
	}
	modelID = sharedconfig.NormalizeModelForProvider(providerKey, modelID)
	if strings.TrimSpace(providerToken) == "" {
		providerToken = ""
	}

	allowedUsers := "[]"
	if len(allowFrom) > 0 {
		quoted := make([]string, 0, len(allowFrom))
		for _, raw := range allowFrom {
			id := strings.TrimSpace(raw)
			if id == "" {
				continue
			}
			quoted = append(quoted, strconv.Quote(id))
		}
		if len(quoted) > 0 {
			allowedUsers = "[" + strings.Join(quoted, ", ") + "]"
		}
	}

	lines := []string{
		"# Generated by Carrier managed onboarding",
		"# Edit manually if you need advanced ZeroClaw settings.",
		"",
		fmt.Sprintf("api_key = %s", strconv.Quote(providerToken)),
		fmt.Sprintf("default_provider = %s", strconv.Quote(providerKey)),
		fmt.Sprintf("default_model = %s", strconv.Quote(modelID)),
		"default_temperature = 0.7",
		"",
		"[agent]",
		"max_tool_iterations = 20",
	}
	if len(profiles) > 0 {
		lines = append(lines, "", "# Managed provider profiles")
		for _, profile := range profiles {
			lines = append(lines,
				"",
				fmt.Sprintf("[provider_profiles.%s]", sanitizeManagedProfileSectionName(profile.ProfileName)),
				fmt.Sprintf("protocol_family = %s", strconv.Quote(profile.ProtocolFamily)),
				fmt.Sprintf("provider = %s", strconv.Quote(profile.ProviderKey)),
				fmt.Sprintf("provider_id = %s", strconv.Quote(profile.ProviderID)),
			)
			if strings.TrimSpace(profile.ModelAlias) != "" {
				lines = append(lines, fmt.Sprintf("model_alias = %s", strconv.Quote(profile.ModelAlias)))
			}
			lines = append(lines, fmt.Sprintf("model = %s", strconv.Quote(sharedconfig.NormalizeModelForProvider(profile.ProviderKey, profile.ModelID))))
			if strings.TrimSpace(profile.BaseURL) != "" {
				lines = append(lines, fmt.Sprintf("base_url = %s", strconv.Quote(profile.BaseURL)))
			}
			if strings.TrimSpace(profile.EnvVar) != "" {
				lines = append(lines, fmt.Sprintf("credential_env = %s", strconv.Quote(profile.EnvVar)))
			}
		}
	}
	if strings.TrimSpace(channelID) == "" {
		lines = append(lines,
			"",
			"# No chat channel configured (WebUI-only mode)",
		)
		return []byte(strings.Join(lines, "\n") + "\n")
	}

	lines = append(lines,
		"",
		fmt.Sprintf("[channels_config.%s]", channelID),
		fmt.Sprintf("bot_token = %s", strconv.Quote(channelToken)),
		fmt.Sprintf("allowed_users = %s", allowedUsers),
		"mention_only = false",
	)
	if channelSetupPending {
		lines = append(lines,
			"",
			"# channel setup is pending; configure channel token in Web UI before enabling channel start",
		)
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func resolveManagedModelProfiles(provider *LLMProvider) []managedModelProfile {
	if provider == nil {
		return []managedModelProfile{buildFallbackManagedModelProfile(nil)}
	}
	configuredProfiles, err := sharedconfig.LoadCarrierModelProfilesForProvider(provider.ID)
	if err == nil && len(configuredProfiles) > 0 {
		profiles := make([]managedModelProfile, 0, len(configuredProfiles))
		for _, configured := range configuredProfiles {
			profiles = append(profiles, buildManagedModelProfile(provider, configured))
		}
		return profiles
	}
	return []managedModelProfile{buildFallbackManagedModelProfile(provider)}
}

func buildManagedModelProfile(provider *LLMProvider, configured sharedconfig.CarrierDefaultModel) managedModelProfile {
	modelID := normalizeManagedModelID(configured.ProviderID, configured.ModelID)
	providerKey := deriveManagedProviderKey(configured.ProviderID, modelID)
	baseURL := strings.TrimSpace(configured.BaseURL)
	if baseURL == "" {
		baseURL = resolveManagedProviderBaseURL(provider, providerKey)
	}
	authMethod := ""
	if catalog.IsOpenAICodexProviderID(configured.ProviderID) {
		authMethod = "oauth"
	}
	return managedModelProfile{
		ProfileName:    firstNonEmptyProfile(strings.TrimSpace(configured.ModelName), deriveManagedModelName(modelID)),
		ModelAlias:     strings.TrimSpace(configured.ModelAlias),
		ModelID:        modelID,
		EnvVar:         strings.TrimSpace(configured.EnvVar),
		ProviderID:     strings.TrimSpace(configured.ProviderID),
		ProviderKey:    providerKey,
		ProtocolFamily: firstNonEmptyProfile(strings.TrimSpace(configured.ProtocolFamily), catalog.ProtocolFamilyForProvider(configured.ProviderID)),
		BaseURL:        baseURL,
		AuthMethod:     authMethod,
	}
}

func buildFallbackManagedModelProfile(provider *LLMProvider) managedModelProfile {
	providerID := ""
	modelID := ""
	envVar := ""
	if provider != nil {
		providerID = strings.TrimSpace(provider.ID)
		modelID = strings.TrimSpace(provider.ExampleModel)
		envVar = strings.TrimSpace(provider.EnvVar)
	}
	if modelID == "" {
		modelID = providerID + "/default"
	}
	modelID = normalizeManagedModelID(providerID, modelID)
	providerKey := deriveManagedProviderKey(providerID, modelID)
	authMethod := ""
	if catalog.IsOpenAICodexProviderID(providerID) {
		authMethod = "oauth"
	}
	return managedModelProfile{
		ProfileName:    deriveManagedModelName(modelID),
		ModelID:        modelID,
		EnvVar:         envVar,
		ProviderID:     providerID,
		ProviderKey:    providerKey,
		ProtocolFamily: catalog.ProtocolFamilyForProvider(providerID),
		BaseURL:        resolveManagedProviderBaseURL(provider, providerKey),
		AuthMethod:     authMethod,
	}
}

func normalizeManagedModelID(providerID, modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if catalog.IsOpenAICodexProviderID(providerID) {
		if _, name, ok := strings.Cut(modelID, "/"); ok && strings.TrimSpace(name) != "" {
			return "openai/" + strings.TrimSpace(name)
		}
		return "openai/gpt-5.3-codex"
	}
	return modelID
}

func deriveManagedProviderKey(providerID, modelID string) string {
	if providerKey := strings.TrimSpace(providerID); providerKey != "" {
		return mapCarrierProviderToManagedProvider(providerKey)
	}
	if vendor, _, ok := strings.Cut(strings.TrimSpace(modelID), "/"); ok && strings.TrimSpace(vendor) != "" {
		return mapCarrierProviderToManagedProvider(strings.TrimSpace(vendor))
	}
	return ""
}

func deriveManagedModelName(modelID string) string {
	modelName := strings.TrimSpace(modelID)
	if _, name, ok := strings.Cut(modelName, "/"); ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	if modelName == "" {
		return "default"
	}
	return modelName
}

func sanitizeManagedProfileSectionName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.Trim(strings.ToLower(b.String()), "_")
}

func firstNonEmptyProfile(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mapCarrierProviderToManagedProvider(providerID string) string {
	return catalog.MapToManagedProvider(providerID)
}

func resolveManagedProviderBaseURL(provider *LLMProvider, providerKey string) string {
	if provider == nil {
		return ""
	}
	if base := strings.TrimSpace(provider.DefaultBase); base != "" {
		return base
	}
	return catalog.ResolveProviderBaseURL(provider.ID, providerKey, "")
}
