package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	sharedconfig "carrier/shared/config"
)

func persistManagedInstanceModelSurfaceConfig(inst managedAgentInstance) error {
	if inst.ModelSurface == nil || strings.TrimSpace(inst.ConfigPath) == "" {
		return nil
	}
	raw, err := os.ReadFile(strings.TrimSpace(inst.ConfigPath))
	if err != nil {
		return fmt.Errorf("read managed config: %w", err)
	}
	var updated []byte
	switch strings.ToLower(strings.TrimSpace(inst.Type)) {
	case "picoclaw":
		updated, err = rewriteManagedPicoclawConfigModelSurface(raw, inst.ModelSurface)
	case "openclaw":
		updated, err = rewriteManagedOpenClawConfigModelSurface(raw, inst.ModelSurface)
	case "zeroclaw":
		updated, err = rewriteManagedZeroClawConfigModelSurface(raw, inst.ModelSurface)
	default:
		return nil
	}
	if err != nil {
		return err
	}
	return os.WriteFile(strings.TrimSpace(inst.ConfigPath), updated, 0o600)
}

func rewriteManagedPicoclawConfigModelSurface(raw []byte, surface *managedAgentModelSurface) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse picoclaw config: %w", err)
	}
	profiles := managedModelProfilesFromSurface(surface)
	oldProviderProfiles := map[string]any{}
	if existing, ok := payload["provider_profiles"].(map[string]any); ok {
		oldProviderProfiles = existing
	}
	oldProviders := map[string]any{}
	if existing, ok := payload["providers"].(map[string]any); ok {
		oldProviders = existing
	}
	modelList := make([]any, 0, len(profiles))
	providerProfiles := map[string]any{}
	providers := map[string]any{}
	for _, profile := range profiles {
		modelItem := map[string]any{
			"model_name":      strings.TrimSpace(profile.ProfileName),
			"model":           strings.TrimSpace(profile.ModelID),
			"protocol_family": strings.TrimSpace(profile.ProtocolFamily),
		}
		if strings.TrimSpace(profile.ModelAlias) != "" {
			modelItem["model_alias"] = strings.TrimSpace(profile.ModelAlias)
		}
		if strings.TrimSpace(profile.BaseURL) != "" {
			modelItem["base_url"] = strings.TrimSpace(profile.BaseURL)
		}
		if strings.TrimSpace(profile.AuthMethod) != "" {
			modelItem["auth_method"] = strings.TrimSpace(profile.AuthMethod)
		}
		modelList = append(modelList, modelItem)

		entry := cloneStringAnyMap(oldProviderProfiles[strings.TrimSpace(profile.ProfileName)])
		entry["provider"] = strings.TrimSpace(profile.ProviderKey)
		entry["provider_id"] = strings.TrimSpace(profile.ProviderID)
		entry["protocol_family"] = strings.TrimSpace(profile.ProtocolFamily)
		entry["model"] = strings.TrimSpace(profile.ModelID)
		if strings.TrimSpace(profile.ModelAlias) != "" {
			entry["model_alias"] = strings.TrimSpace(profile.ModelAlias)
		}
		if strings.TrimSpace(profile.BaseURL) != "" {
			entry["base_url"] = strings.TrimSpace(profile.BaseURL)
		}
		if strings.TrimSpace(profile.AuthMethod) != "" {
			entry["auth_method"] = strings.TrimSpace(profile.AuthMethod)
		}
		providerProfiles[strings.TrimSpace(profile.ProfileName)] = entry

		if providerKey := strings.TrimSpace(profile.ProviderKey); providerKey != "" {
			providerEntry := cloneStringAnyMap(oldProviders[providerKey])
			if strings.TrimSpace(profile.AuthMethod) != "" {
				providerEntry["auth_method"] = strings.TrimSpace(profile.AuthMethod)
			}
			providers[providerKey] = providerEntry
		}
	}
	payload["model_list"] = modelList
	payload["provider_profiles"] = providerProfiles
	if len(providers) > 0 {
		payload["providers"] = providers
	}
	if defaults, ok := payload["agents"].(map[string]any); ok {
		if inner, ok := defaults["defaults"].(map[string]any); ok {
			if defaultProfile := defaultManagedModelProfile(surface); defaultProfile != nil {
				inner["provider"] = strings.TrimSpace(defaultProfile.ProviderKey)
				inner["model"] = deriveManagedModelName(strings.TrimSpace(defaultProfile.ModelID))
			}
		}
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal picoclaw config: %w", err)
	}
	return append(encoded, '\n'), nil
}

func rewriteManagedOpenClawConfigModelSurface(raw []byte, surface *managedAgentModelSurface) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse openclaw config: %w", err)
	}
	models := cloneStringAnyMap(payload["models"])
	oldProviders := map[string]any{}
	if existing, ok := models["providers"].(map[string]any); ok {
		oldProviders = existing
	}
	grouped := map[string][]managedModelProfile{}
	for _, profile := range managedModelProfilesFromSurface(surface) {
		key := firstNonEmpty(strings.TrimSpace(profile.ProviderKey), strings.TrimSpace(profile.ProviderID))
		grouped[key] = append(grouped[key], profile)
	}
	providers := map[string]any{}
	for providerKey, profiles := range grouped {
		entry := cloneStringAnyMap(oldProviders[providerKey])
		modelEntries := make([]any, 0, len(profiles))
		for _, profile := range profiles {
			modelEntries = append(modelEntries, map[string]any{
				"id":   strings.TrimSpace(profile.ModelID),
				"name": strings.TrimSpace(profile.ProfileName),
			})
			if strings.TrimSpace(profile.BaseURL) != "" {
				entry["baseUrl"] = strings.TrimSpace(profile.BaseURL)
			}
			if strings.TrimSpace(profile.AuthMethod) != "" {
				entry["auth"] = strings.TrimSpace(profile.AuthMethod)
			}
		}
		entry["models"] = modelEntries
		providers[providerKey] = entry
	}
	models["providers"] = providers
	payload["models"] = models
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal openclaw config: %w", err)
	}
	return append(encoded, '\n'), nil
}

func rewriteManagedZeroClawConfigModelSurface(raw []byte, surface *managedAgentModelSurface) ([]byte, error) {
	cfg := parseManagedZeroClawLocalConfig(raw)
	defaultProfile := defaultManagedModelProfile(surface)
	defaultProvider := strings.TrimSpace(cfg.DefaultProvider)
	defaultModel := strings.TrimSpace(cfg.DefaultModel)
	if defaultProfile != nil {
		defaultProvider = firstNonEmpty(strings.TrimSpace(defaultProfile.ProviderKey), strings.TrimSpace(defaultProfile.ProviderID), defaultProvider)
		defaultModel = sharedconfig.NormalizeModelForProvider(defaultProvider, strings.TrimSpace(defaultProfile.ModelID))
	}
	existingProfiles := map[string]managedZeroClawProviderProfile{}
	for _, profile := range cfg.Profiles {
		existingProfiles[strings.TrimSpace(profile.SectionName)] = profile
	}
	lines := strings.Split(string(raw), "\n")
	kept := make([]string, 0, len(lines))
	section := ""
	skipProfileSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"))
			skipProfileSection = strings.HasPrefix(section, "provider_profiles.")
			if skipProfileSection {
				continue
			}
		}
		if skipProfileSection {
			continue
		}
		if section == "" {
			key, _, ok := strings.Cut(trimmed, "=")
			if ok {
				switch strings.ToLower(strings.TrimSpace(key)) {
				case "default_provider", "default_model":
					continue
				}
			}
		}
		kept = append(kept, line)
	}
	insertAt := len(kept)
	for i, line := range kept {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			insertAt = i
			break
		}
	}
	profileLines := []string{
		fmt.Sprintf("default_provider = %s", strconv.Quote(strings.TrimSpace(defaultProvider))),
		fmt.Sprintf("default_model = %s", strconv.Quote(strings.TrimSpace(defaultModel))),
	}
	for _, profile := range managedModelProfilesFromSurface(surface) {
		legacy := existingProfiles[strings.TrimSpace(profile.ProfileName)]
		providerKey := firstNonEmpty(strings.TrimSpace(profile.ProviderKey), strings.TrimSpace(profile.ProviderID))
		modelID := sharedconfig.NormalizeModelForProvider(providerKey, strings.TrimSpace(profile.ModelID))
		profileLines = append(profileLines,
			"",
			fmt.Sprintf("[provider_profiles.%s]", sanitizeManagedProfileSectionName(profile.ProfileName)),
			fmt.Sprintf("protocol_family = %s", strconv.Quote(strings.TrimSpace(profile.ProtocolFamily))),
			fmt.Sprintf("provider = %s", strconv.Quote(strings.TrimSpace(providerKey))),
			fmt.Sprintf("provider_id = %s", strconv.Quote(strings.TrimSpace(profile.ProviderID))),
		)
		if strings.TrimSpace(profile.ModelAlias) != "" {
			profileLines = append(profileLines, fmt.Sprintf("model_alias = %s", strconv.Quote(strings.TrimSpace(profile.ModelAlias))))
		}
		profileLines = append(profileLines, fmt.Sprintf("model = %s", strconv.Quote(modelID)))
		if strings.TrimSpace(profile.BaseURL) != "" {
			profileLines = append(profileLines, fmt.Sprintf("base_url = %s", strconv.Quote(strings.TrimSpace(profile.BaseURL))))
		}
		if strings.TrimSpace(profile.AuthMethod) != "" {
			profileLines = append(profileLines, fmt.Sprintf("auth_method = %s", strconv.Quote(strings.TrimSpace(profile.AuthMethod))))
		}
		if strings.TrimSpace(legacy.CredentialEnv) != "" {
			profileLines = append(profileLines, fmt.Sprintf("credential_env = %s", strconv.Quote(strings.TrimSpace(legacy.CredentialEnv))))
		}
	}
	kept = append(kept[:insertAt], append(profileLines, kept[insertAt:]...)...)
	text := strings.Join(kept, "\n")
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return []byte(text), nil
}

func defaultManagedModelProfile(surface *managedAgentModelSurface) *managedModelProfile {
	if surface == nil || len(surface.Profiles) == 0 {
		return nil
	}
	profiles := managedModelProfilesFromSurface(surface)
	defaultProfile := strings.TrimSpace(surface.DefaultProfile)
	for i := range profiles {
		if defaultProfile != "" && strings.EqualFold(strings.TrimSpace(profiles[i].ProfileName), defaultProfile) {
			return &profiles[i]
		}
	}
	return &profiles[0]
}

func cloneStringAnyMap(value any) map[string]any {
	source, _ := value.(map[string]any)
	if source == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(source))
	for key, entry := range source {
		cloned[key] = entry
	}
	return cloned
}
