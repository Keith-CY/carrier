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
	sharedconfig "carrier/shared/config"
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
	CredentialEnv  string
}

type managedAgentModelProfileUpdate struct {
	ProfileName      string
	ModelAlias       string
	ModelID          string
	ProviderID       string
	BaseURL          string
	AuthMethod       string
	TimeoutMs        int
	RetryBudget      int
	FallbackStrategy string
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

func updateManagedAgentModelsDefaultSummary(agentID, profileName string) (*agentModelsSummary, error) {
	instances, path, err := loadManagedInstances()
	if err != nil {
		return nil, err
	}
	idx := findManagedInstanceIndexByAgentID(instances, agentID)
	if idx < 0 {
		return nil, errManagedAgentInstanceNotFound
	}
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		return nil, fmt.Errorf("profile name is required")
	}
	surface := instances[idx].ModelSurface
	if surface == nil || len(surface.Profiles) == 0 {
		return nil, fmt.Errorf("managed agent model surface is unavailable")
	}
	profiles := make([]managedModelProfile, 0, len(surface.Profiles))
	matched := false
	for _, profile := range surface.Profiles {
		profiles = append(profiles, managedModelProfile{
			ProfileName:    strings.TrimSpace(profile.ProfileName),
			ModelAlias:     strings.TrimSpace(profile.ModelAlias),
			ModelID:        strings.TrimSpace(profile.ModelID),
			ProviderID:     strings.TrimSpace(profile.ProviderID),
			ProviderKey:    strings.TrimSpace(profile.ProviderKey),
			ProtocolFamily: strings.TrimSpace(profile.ProtocolFamily),
			BaseURL:        strings.TrimSpace(profile.BaseURL),
			AuthMethod:     strings.TrimSpace(profile.AuthMethod),
			TimeoutMs:      profile.TimeoutMs,
			RetryBudget:    profile.RetryBudget,
			FallbackStrategy: strings.TrimSpace(profile.FallbackStrategy),
		})
		if strings.EqualFold(strings.TrimSpace(profile.ProfileName), profileName) {
			matched = true
		}
	}
	if !matched {
		return nil, fmt.Errorf("profile %q not found", profileName)
	}
	instances[idx].ModelSurface = buildManagedModelSurfaceWithDefault(profiles, profileName)
	instances[idx].UpdatedAt = nowRFC3339Nano()
	if err := saveManagedInstances(path, instances); err != nil {
		return nil, err
	}
	return buildAgentModelsSummary(instances[idx], false), nil
}

func updateManagedAgentModelProfileSummary(agentID string, update managedAgentModelProfileUpdate) (*agentModelsSummary, error) {
	instances, path, err := loadManagedInstances()
	if err != nil {
		return nil, err
	}
	idx := findManagedInstanceIndexByAgentID(instances, agentID)
	if idx < 0 {
		return nil, errManagedAgentInstanceNotFound
	}
	update.ProfileName = strings.TrimSpace(update.ProfileName)
	if update.ProfileName == "" {
		return nil, fmt.Errorf("profile name is required")
	}
	surface := instances[idx].ModelSurface
	if surface == nil || len(surface.Profiles) == 0 {
		return nil, fmt.Errorf("managed agent model surface is unavailable")
	}
	currentProfiles := managedModelProfilesFromSurface(surface)
	matchIdx := -1
	for i, profile := range currentProfiles {
		if strings.EqualFold(strings.TrimSpace(profile.ProfileName), update.ProfileName) {
			matchIdx = i
			break
		}
	}
	if matchIdx < 0 {
		return nil, fmt.Errorf("profile %q not found", update.ProfileName)
	}
	profile := currentProfiles[matchIdx]
	if strings.TrimSpace(update.ModelAlias) != "" {
		profile.ModelAlias = strings.TrimSpace(update.ModelAlias)
	}
	if strings.TrimSpace(update.ModelID) != "" {
		profile.ModelID = strings.TrimSpace(update.ModelID)
	}
	if strings.TrimSpace(update.ProviderID) != "" {
		profile.ProviderID = strings.TrimSpace(update.ProviderID)
		profile.ProviderKey = strings.TrimSpace(update.ProviderID)
	}
	if strings.TrimSpace(update.BaseURL) != "" {
		profile.BaseURL = strings.TrimSpace(update.BaseURL)
	}
	if strings.TrimSpace(update.AuthMethod) != "" {
		profile.AuthMethod = strings.TrimSpace(update.AuthMethod)
	}
	if update.TimeoutMs > 0 {
		profile.TimeoutMs = update.TimeoutMs
	}
	if update.RetryBudget > 0 {
		profile.RetryBudget = update.RetryBudget
	}
	if strings.TrimSpace(update.FallbackStrategy) != "" {
		profile.FallbackStrategy = strings.TrimSpace(update.FallbackStrategy)
	}
	profile.ProviderKey = firstNonEmpty(strings.TrimSpace(profile.ProviderKey), strings.TrimSpace(profile.ProviderID), deriveManagedProviderKey(profile.ProviderID, profile.ModelID))
	profile.ProtocolFamily = firstNonEmpty(strings.TrimSpace(profile.ProtocolFamily), catalog.ProtocolFamilyForProvider(strings.TrimSpace(profile.ProviderID)))
	profile.BaseURL = firstNonEmpty(strings.TrimSpace(profile.BaseURL), catalog.ResolveProviderBaseURL(strings.TrimSpace(profile.ProviderID), strings.TrimSpace(profile.ProviderKey), ""))
	currentProfiles[matchIdx] = profile

	updatedSurface := buildManagedModelSurfaceWithDefault(currentProfiles, strings.TrimSpace(surface.DefaultProfile))
	instances[idx].ModelSurface = updatedSurface
	if err := persistManagedInstanceModelSurfaceConfig(instances[idx]); err != nil {
		return nil, err
	}
	instances[idx].UpdatedAt = nowRFC3339Nano()
	if err := saveManagedInstances(path, instances); err != nil {
		return nil, err
	}
	return buildAgentModelsSummary(instances[idx], false), nil
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

func managedModelProfilesFromSurface(surface *managedAgentModelSurface) []managedModelProfile {
	if surface == nil || len(surface.Profiles) == 0 {
		return nil
	}
	profiles := make([]managedModelProfile, 0, len(surface.Profiles))
	for _, profile := range surface.Profiles {
		profiles = append(profiles, managedModelProfile{
			ProfileName:      strings.TrimSpace(profile.ProfileName),
			ModelAlias:       strings.TrimSpace(profile.ModelAlias),
			ModelID:          strings.TrimSpace(profile.ModelID),
			ProviderID:       strings.TrimSpace(profile.ProviderID),
			ProviderKey:      firstNonEmpty(strings.TrimSpace(profile.ProviderKey), strings.TrimSpace(profile.ProviderID), deriveManagedProviderKey(strings.TrimSpace(profile.ProviderID), strings.TrimSpace(profile.ModelID))),
			ProtocolFamily:   firstNonEmpty(strings.TrimSpace(profile.ProtocolFamily), catalog.ProtocolFamilyForProvider(strings.TrimSpace(profile.ProviderID))),
			BaseURL:          firstNonEmpty(strings.TrimSpace(profile.BaseURL), catalog.ResolveProviderBaseURL(strings.TrimSpace(profile.ProviderID), strings.TrimSpace(profile.ProviderKey), "")),
			AuthMethod:       strings.TrimSpace(profile.AuthMethod),
			TimeoutMs:        profile.TimeoutMs,
			RetryBudget:      profile.RetryBudget,
			FallbackStrategy: strings.TrimSpace(profile.FallbackStrategy),
		})
	}
	return profiles
}

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
			case "credential_env":
				currentProfile.CredentialEnv = strings.TrimSpace(unquoted)
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
