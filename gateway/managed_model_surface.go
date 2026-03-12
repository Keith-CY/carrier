package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"carrier/shared/catalog"
)

var errManagedAgentInstanceNotFound = errors.New("managed agent instance not found")

type agentModelsSummary struct {
	AgentID     string                     `json:"agentId"`
	InstanceID  string                     `json:"instanceId,omitempty"`
	ConfigPath  string                     `json:"configPath,omitempty"`
	Synced      bool                       `json:"synced,omitempty"`
	ModelSurface *agentLauncherModelSurface `json:"modelSurface,omitempty"`
}

type managedConfigJSON struct {
	DefaultModel     string                                 `json:"default_model"`
	ModelList        []managedConfigJSONModel               `json:"model_list"`
	ProviderProfiles map[string]managedConfigJSONProfile    `json:"provider_profiles"`
	Models           managedOpenClawModelsConfig            `json:"models"`
}

type managedConfigJSONModel struct {
	ModelName      string `json:"model_name"`
	ModelAlias     string `json:"model_alias"`
	ModelID        string `json:"model"`
	Provider       string `json:"provider"`
	ProviderID     string `json:"provider_id"`
	ProtocolFamily string `json:"protocol_family"`
	BaseURL        string `json:"base_url"`
	AuthMethod     string `json:"auth_method"`
}

type managedConfigJSONProfile struct {
	Provider       string `json:"provider"`
	ProviderID     string `json:"provider_id"`
	ModelAlias     string `json:"model_alias"`
	ModelID        string `json:"model"`
	ProtocolFamily string `json:"protocol_family"`
	BaseURL        string `json:"base_url"`
	AuthMethod     string `json:"auth_method"`
	CredentialRef  string `json:"credential_ref"`
}

type managedOpenClawModelsConfig struct {
	Providers map[string]managedOpenClawProviderConfig `json:"providers"`
}

type managedOpenClawProviderConfig struct {
	BaseURL string                          `json:"baseUrl"`
	Auth    string                          `json:"auth"`
	Models  []managedOpenClawProviderModel  `json:"models"`
}

type managedOpenClawProviderModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type managedZeroClawLocalConfig struct {
	DefaultProvider string
	DefaultModel    string
	Profiles        []managedZeroClawProviderProfile
}

type managedZeroClawProviderProfile struct {
	SectionName    string
	ModelAlias     string
	Model          string
	Provider       string
	ProviderID     string
	ProtocolFamily string
	BaseURL        string
	AuthMethod     string
}

func currentManagedAgentModelsSummary(agentID string) (*agentModelsSummary, error) {
	inst, ok := latestManagedInstanceForAgent(agentID)
	if !ok {
		return nil, errManagedAgentInstanceNotFound
	}
	return buildAgentModelsSummary(inst, false), nil
}

func syncManagedAgentModelsSummary(agentID string) (*agentModelsSummary, error) {
	instances, path, err := loadManagedInstances()
	if err != nil {
		return nil, err
	}
	idx := findManagedInstanceIndexByAgentID(instances, agentID)
	if idx < 0 {
		return nil, errManagedAgentInstanceNotFound
	}
	surface, updated, err := readManagedModelSurfaceFromConfig(instances[idx])
	if err != nil {
		return nil, err
	}
	if updated && surface != nil {
		instances[idx].ModelSurface = surface
		instances[idx].UpdatedAt = nowRFC3339Nano()
		if err := saveManagedInstances(path, instances); err != nil {
			return nil, err
		}
	}
	return buildAgentModelsSummary(instances[idx], updated), nil
}

func buildAgentModelsSummary(inst managedAgentInstance, synced bool) *agentModelsSummary {
	return &agentModelsSummary{
		AgentID:      strings.TrimSpace(inst.AgentID),
		InstanceID:   strings.TrimSpace(inst.ID),
		ConfigPath:   strings.TrimSpace(inst.ConfigPath),
		Synced:       synced,
		ModelSurface: buildAgentLauncherModelSurface(inst.ModelSurface),
	}
}

func readManagedModelSurfaceFromConfig(inst managedAgentInstance) (*managedAgentModelSurface, bool, error) {
	configPath := strings.TrimSpace(inst.ConfigPath)
	if configPath == "" {
		return inst.ModelSurface, false, nil
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return nil, false, fmt.Errorf("read managed config: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(inst.Type)) {
	case "zeroclaw":
		surface := parseManagedZeroClawModelSurface(raw)
		if surface == nil {
			return inst.ModelSurface, false, nil
		}
		return surface, true, nil
	case "picoclaw", "openclaw":
		surface := parseManagedJSONModelSurface(raw)
		if surface == nil {
			return inst.ModelSurface, false, nil
		}
		return surface, true, nil
	default:
		return inst.ModelSurface, false, nil
	}
}

func parseManagedJSONModelSurface(raw []byte) *managedAgentModelSurface {
	var cfg managedConfigJSON
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil
	}
	if len(cfg.ModelList) > 0 {
		profiles := make([]managedModelProfile, 0, len(cfg.ModelList))
		for _, item := range cfg.ModelList {
			profileName := strings.TrimSpace(item.ModelName)
			if profileName == "" {
				profileName = deriveManagedModelName(strings.TrimSpace(item.ModelID))
			}
			profileCfg := cfg.ProviderProfiles[profileName]
			providerID := firstNonEmpty(strings.TrimSpace(profileCfg.ProviderID), strings.TrimSpace(item.ProviderID), strings.TrimSpace(profileCfg.Provider), strings.TrimSpace(item.Provider))
			providerKey := firstNonEmpty(strings.TrimSpace(profileCfg.Provider), strings.TrimSpace(item.Provider), deriveManagedProviderKey(providerID, firstNonEmpty(strings.TrimSpace(profileCfg.ModelID), strings.TrimSpace(item.ModelID))))
			modelID := firstNonEmpty(strings.TrimSpace(profileCfg.ModelID), strings.TrimSpace(item.ModelID))
			profiles = append(profiles, managedModelProfile{
				ProfileName:    profileName,
				ModelAlias:     firstNonEmpty(strings.TrimSpace(profileCfg.ModelAlias), strings.TrimSpace(item.ModelAlias)),
				ModelID:        modelID,
				ProviderID:     providerID,
				ProviderKey:    providerKey,
				ProtocolFamily: firstNonEmpty(strings.TrimSpace(profileCfg.ProtocolFamily), strings.TrimSpace(item.ProtocolFamily), catalog.ProtocolFamilyForProvider(providerID)),
				BaseURL:        firstNonEmpty(strings.TrimSpace(profileCfg.BaseURL), strings.TrimSpace(item.BaseURL), catalog.ResolveProviderBaseURL(providerID, providerKey, "")),
				AuthMethod:     firstNonEmpty(strings.TrimSpace(profileCfg.AuthMethod), strings.TrimSpace(item.AuthMethod), authMethodFromCredentialRef(strings.TrimSpace(profileCfg.CredentialRef))),
			})
		}
		return buildManagedModelSurfaceWithDefault(profiles, strings.TrimSpace(cfg.DefaultModel))
	}
	if len(cfg.Models.Providers) == 0 {
		return nil
	}
	providerKeys := make([]string, 0, len(cfg.Models.Providers))
	for key := range cfg.Models.Providers {
		providerKeys = append(providerKeys, key)
	}
	sort.Strings(providerKeys)
	profiles := make([]managedModelProfile, 0, len(providerKeys))
	for _, providerKey := range providerKeys {
		providerCfg := cfg.Models.Providers[providerKey]
		for idx, model := range providerCfg.Models {
			modelID := strings.TrimSpace(model.ID)
			if modelID == "" {
				continue
			}
			profileName := strings.TrimSpace(providerKey)
			if len(providerCfg.Models) > 1 {
				profileName = fmt.Sprintf("%s-%d", strings.TrimSpace(providerKey), idx+1)
			}
			profiles = append(profiles, managedModelProfile{
				ProfileName:    profileName,
				ModelID:        modelID,
				ProviderID:     strings.TrimSpace(providerKey),
				ProviderKey:    strings.TrimSpace(providerKey),
				ProtocolFamily: catalog.ProtocolFamilyForProvider(strings.TrimSpace(providerKey)),
				BaseURL:        strings.TrimSpace(providerCfg.BaseURL),
				AuthMethod:     strings.TrimSpace(providerCfg.Auth),
			})
		}
	}
	return buildManagedModelSurfaceWithDefault(profiles, "")
}

func parseManagedZeroClawModelSurface(raw []byte) *managedAgentModelSurface {
	cfg := parseManagedZeroClawLocalConfig(raw)
	if len(cfg.Profiles) == 0 {
		return nil
	}
	profiles := make([]managedModelProfile, 0, len(cfg.Profiles))
	for _, profile := range cfg.Profiles {
		providerID := firstNonEmpty(strings.TrimSpace(profile.ProviderID), strings.TrimSpace(profile.Provider))
		providerKey := firstNonEmpty(strings.TrimSpace(profile.Provider), deriveManagedProviderKey(providerID, strings.TrimSpace(profile.Model)))
		profiles = append(profiles, managedModelProfile{
			ProfileName:    strings.TrimSpace(profile.SectionName),
			ModelAlias:     strings.TrimSpace(profile.ModelAlias),
			ModelID:        strings.TrimSpace(profile.Model),
			ProviderID:     providerID,
			ProviderKey:    providerKey,
			ProtocolFamily: firstNonEmpty(strings.TrimSpace(profile.ProtocolFamily), catalog.ProtocolFamilyForProvider(providerID)),
			BaseURL:        firstNonEmpty(strings.TrimSpace(profile.BaseURL), catalog.ResolveProviderBaseURL(providerID, providerKey, "")),
			AuthMethod:     strings.TrimSpace(profile.AuthMethod),
		})
	}
	return buildManagedModelSurfaceWithDefault(profiles, strings.TrimSpace(cfg.DefaultProvider))
}

func buildManagedModelSurfaceWithDefault(profiles []managedModelProfile, defaultProfile string) *managedAgentModelSurface {
	if len(profiles) == 0 {
		return nil
	}
	ordered := append([]managedModelProfile(nil), profiles...)
	defaultProfile = strings.TrimSpace(defaultProfile)
	if defaultProfile != "" {
		matchIdx := -1
		for idx, profile := range ordered {
			if strings.EqualFold(strings.TrimSpace(profile.ProfileName), defaultProfile) {
				matchIdx = idx
				break
			}
		}
		if matchIdx >= 0 && matchIdx != 0 {
			defaultEntry := ordered[matchIdx]
			ordered = append([]managedModelProfile{defaultEntry}, append(ordered[:matchIdx], ordered[matchIdx+1:]...)...)
		}
	}
	surface := buildManagedModelSurface(ordered)
	if defaultProfile != "" {
		for _, profile := range ordered {
			if strings.EqualFold(strings.TrimSpace(profile.ProfileName), defaultProfile) {
				surface.DefaultProfile = strings.TrimSpace(profile.ProfileName)
				break
			}
		}
	}
	return &surface
}

func parseManagedZeroClawLocalConfig(raw []byte) managedZeroClawLocalConfig {
	cfg := managedZeroClawLocalConfig{}
	section := ""
	currentProfile := managedZeroClawProviderProfile{}
	flushProfile := func() {
		if strings.TrimSpace(currentProfile.SectionName) == "" {
			return
		}
		cfg.Profiles = append(cfg.Profiles, currentProfile)
		currentProfile = managedZeroClawProviderProfile{}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if idx := strings.Index(trimmed, "#"); idx >= 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			flushProfile()
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"))
			if strings.HasPrefix(section, "provider_profiles.") {
				currentProfile.SectionName = strings.TrimSpace(strings.TrimPrefix(section, "provider_profiles."))
			}
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		unquoted, _ := strconv.Unquote(value)
		switch {
		case section == "":
			switch key {
			case "default_provider":
				cfg.DefaultProvider = strings.TrimSpace(unquoted)
			case "default_model":
				cfg.DefaultModel = strings.TrimSpace(unquoted)
			}
		case strings.HasPrefix(section, "provider_profiles."):
			switch key {
			case "model_alias":
				currentProfile.ModelAlias = strings.TrimSpace(unquoted)
			case "model":
				currentProfile.Model = strings.TrimSpace(unquoted)
			case "provider":
				currentProfile.Provider = strings.TrimSpace(unquoted)
			case "provider_id":
				currentProfile.ProviderID = strings.TrimSpace(unquoted)
			case "protocol_family":
				currentProfile.ProtocolFamily = strings.TrimSpace(unquoted)
			case "base_url":
				currentProfile.BaseURL = strings.TrimSpace(unquoted)
			case "auth_method":
				currentProfile.AuthMethod = strings.TrimSpace(unquoted)
			}
		}
	}
	flushProfile()
	return cfg
}

func authMethodFromCredentialRef(ref string) string {
	if strings.TrimSpace(ref) == "" {
		return ""
	}
	return "api_key"
}

func nowRFC3339Nano() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
