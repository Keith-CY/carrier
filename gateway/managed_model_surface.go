package gateway

import (
	"errors"
	"fmt"
	"strings"

	"carrier/shared/catalog"
)

var errManagedAgentInstanceNotFound = errors.New("managed agent instance not found")

type agentModelsSummary struct {
	AgentID                string                     `json:"agentId"`
	InstanceID             string                     `json:"instanceId,omitempty"`
	ConfigPath             string                     `json:"configPath,omitempty"`
	Synced                 bool                       `json:"synced,omitempty"`
	DriftState             string                     `json:"driftState,omitempty"`
	DriftReason            string                     `json:"driftReason,omitempty"`
	ModelSurface           *agentLauncherModelSurface `json:"modelSurface,omitempty"`
	DiscoveredModelSurface *agentLauncherModelSurface `json:"discoveredModelSurface,omitempty"`
}

type managedConfigJSON struct {
	DefaultModel     string                              `json:"default_model"`
	ModelList        []managedConfigJSONModel            `json:"model_list"`
	ProviderProfiles map[string]managedConfigJSONProfile `json:"provider_profiles"`
	Models           managedOpenClawModelsConfig         `json:"models"`
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
	BaseURL string                         `json:"baseUrl"`
	Auth    string                         `json:"auth"`
	Models  []managedOpenClawProviderModel `json:"models"`
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

func discoverManagedAgentModelsSummary(agentID string) (*agentModelsSummary, error) {
	instances, _, err := loadManagedInstances()
	if err != nil {
		return nil, err
	}
	idx := findManagedInstanceIndexByAgentID(instances, agentID)
	if idx < 0 {
		return nil, errManagedAgentInstanceNotFound
	}
	discoveredSurface, available, err := discoverManagedModelSurfaceFromConfig(instances[idx])
	if err != nil {
		return nil, err
	}
	driftState, driftReason := managedModelSurfaceDrift(instances[idx].ModelSurface, discoveredSurface, available)
	return buildAgentModelsSummaryWithDiscovery(instances[idx], false, discoveredSurface, driftState, driftReason), nil
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
			ProfileName:      strings.TrimSpace(profile.ProfileName),
			ModelAlias:       strings.TrimSpace(profile.ModelAlias),
			ModelID:          strings.TrimSpace(profile.ModelID),
			ProviderID:       strings.TrimSpace(profile.ProviderID),
			ProviderKey:      strings.TrimSpace(profile.ProviderKey),
			ProtocolFamily:   strings.TrimSpace(profile.ProtocolFamily),
			BaseURL:          strings.TrimSpace(profile.BaseURL),
			AuthMethod:       strings.TrimSpace(profile.AuthMethod),
			TimeoutMs:        profile.TimeoutMs,
			RetryBudget:      profile.RetryBudget,
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
	return buildAgentModelsSummaryWithDiscovery(inst, synced, nil, "", "")
}

func buildAgentModelsSummaryWithDiscovery(inst managedAgentInstance, synced bool, discoveredSurface *managedAgentModelSurface, driftState, driftReason string) *agentModelsSummary {
	return &agentModelsSummary{
		AgentID:                strings.TrimSpace(inst.AgentID),
		InstanceID:             strings.TrimSpace(inst.ID),
		ConfigPath:             strings.TrimSpace(inst.ConfigPath),
		Synced:                 synced,
		DriftState:             strings.TrimSpace(driftState),
		DriftReason:            strings.TrimSpace(driftReason),
		ModelSurface:           buildAgentLauncherModelSurface(inst.ModelSurface),
		DiscoveredModelSurface: buildAgentLauncherModelSurface(discoveredSurface),
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
