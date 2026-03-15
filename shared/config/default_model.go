package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"carrier/shared/catalog"
)

var userHomeDirFn = os.UserHomeDir

// CarrierDefaultModel describes the selected default model in carrier config.v2.
type CarrierDefaultModel struct {
	ProviderID     string
	ProtocolFamily string
	ModelID        string
	ModelName      string
	ModelAlias     string
	EnvVar         string
	BaseURL        string
}

type carrierModelEntry struct {
	ModelName      string `json:"model_name"`
	ModelAlias     string `json:"model_alias,omitempty"`
	Model          string `json:"model"`
	ProviderID     string `json:"provider_id"`
	ProtocolFamily string `json:"protocol_family,omitempty"`
	EnvVar         string `json:"env_var"`
	BaseURL        string `json:"base_url,omitempty"`
}

type carrierModelConfig struct {
	DefaultModel string              `json:"default_model"`
	ModelList    []carrierModelEntry `json:"model_list"`
}

// LoadCarrierDefaultModel resolves and loads carrier config.v2, then returns
// the selected default model entry.
func LoadCarrierDefaultModel() (*CarrierDefaultModel, error) {
	path, err := resolveCarrierConfigV2Path()
	if err != nil {
		return nil, err
	}
	return loadCarrierDefaultModelFromPath(path)
}

// LoadCarrierModelForProvider resolves and loads carrier config.v2, then returns
// the first model entry matching the requested provider ID.
func LoadCarrierModelForProvider(providerID string) (*CarrierDefaultModel, error) {
	path, err := resolveCarrierConfigV2Path()
	if err != nil {
		return nil, err
	}
	return loadCarrierModelForProviderFromPath(path, providerID)
}

// LoadCarrierModelProfilesForProvider resolves and loads carrier config.v2,
// then returns every model entry matching the requested provider ID.
func LoadCarrierModelProfilesForProvider(providerID string) ([]CarrierDefaultModel, error) {
	path, err := resolveCarrierConfigV2Path()
	if err != nil {
		return nil, err
	}
	return loadCarrierModelProfilesForProviderFromPath(path, providerID)
}

// LoadCarrierModelProfilesForProtocolFamily resolves and loads carrier
// config.v2, then returns every model entry matching the requested protocol
// family.
func LoadCarrierModelProfilesForProtocolFamily(protocolFamily string) ([]CarrierDefaultModel, error) {
	path, err := resolveCarrierConfigV2Path()
	if err != nil {
		return nil, err
	}
	return loadCarrierModelProfilesForProtocolFamilyFromPath(path, protocolFamily)
}

// LoadCarrierModelProfilesForAlias resolves and loads carrier config.v2, then
// returns every model entry matching the requested provider and logical model
// alias. Alias matching falls back to model_name for compatibility.
func LoadCarrierModelProfilesForAlias(providerID, alias string) ([]CarrierDefaultModel, error) {
	path, err := resolveCarrierConfigV2Path()
	if err != nil {
		return nil, err
	}
	return loadCarrierModelProfilesForAliasFromPath(path, providerID, alias)
}

func loadCarrierDefaultModelFromPath(path string) (*CarrierDefaultModel, error) {
	cfg, err := loadCarrierModelConfigFromPath(path)
	if err != nil {
		return nil, err
	}
	defaultName := strings.TrimSpace(cfg.DefaultModel)
	if defaultName != "" {
		for _, m := range cfg.ModelList {
			if strings.EqualFold(strings.TrimSpace(m.ModelName), defaultName) {
				return convertCarrierModel(m), nil
			}
		}
	}
	return convertCarrierModel(cfg.ModelList[0]), nil
}

func loadCarrierModelForProviderFromPath(path string, providerID string) (*CarrierDefaultModel, error) {
	profiles, err := loadCarrierModelProfilesForProviderFromPath(path, providerID)
	if err != nil {
		return nil, err
	}
	return &profiles[0], nil
}

func loadCarrierModelProfilesForProviderFromPath(path string, providerID string) ([]CarrierDefaultModel, error) {
	cfg, err := loadCarrierModelConfigFromPath(path)
	if err != nil {
		return nil, err
	}
	target := strings.ToLower(strings.TrimSpace(providerID))
	if target == "" {
		return nil, errors.New("provider_id is required")
	}
	profiles := make([]CarrierDefaultModel, 0, len(cfg.ModelList))
	for _, m := range cfg.ModelList {
		if strings.EqualFold(strings.TrimSpace(m.ProviderID), target) {
			profiles = append(profiles, *convertCarrierModel(m))
		}
	}
	if len(profiles) == 0 {
		return nil, errors.New("provider_id is not found in model_list")
	}
	return profiles, nil
}

func loadCarrierModelProfilesForProtocolFamilyFromPath(path string, protocolFamily string) ([]CarrierDefaultModel, error) {
	cfg, err := loadCarrierModelConfigFromPath(path)
	if err != nil {
		return nil, err
	}
	target := strings.ToLower(strings.TrimSpace(protocolFamily))
	if target == "" {
		return nil, errors.New("protocol_family is required")
	}
	profiles := make([]CarrierDefaultModel, 0, len(cfg.ModelList))
	for _, m := range cfg.ModelList {
		converted := convertCarrierModel(m)
		if strings.EqualFold(strings.TrimSpace(converted.ProtocolFamily), target) {
			profiles = append(profiles, *converted)
		}
	}
	if len(profiles) == 0 {
		return nil, errors.New("protocol_family is not found in model_list")
	}
	return profiles, nil
}

func loadCarrierModelProfilesForAliasFromPath(path, providerID, alias string) ([]CarrierDefaultModel, error) {
	cfg, err := loadCarrierModelConfigFromPath(path)
	if err != nil {
		return nil, err
	}
	targetProvider := strings.ToLower(strings.TrimSpace(providerID))
	if targetProvider == "" {
		return nil, errors.New("provider_id is required")
	}
	targetAlias := strings.ToLower(strings.TrimSpace(alias))
	if targetAlias == "" {
		return nil, errors.New("model_alias is required")
	}
	profiles := make([]CarrierDefaultModel, 0, len(cfg.ModelList))
	for _, m := range cfg.ModelList {
		if !strings.EqualFold(strings.TrimSpace(m.ProviderID), targetProvider) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(m.ModelAlias), targetAlias) || strings.EqualFold(strings.TrimSpace(m.ModelName), targetAlias) {
			profiles = append(profiles, *convertCarrierModel(m))
		}
	}
	if len(profiles) == 0 {
		return nil, errors.New("model_alias is not found in model_list")
	}
	return profiles, nil
}

func loadCarrierModelConfigFromPath(path string) (*carrierModelConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg carrierModelConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if len(cfg.ModelList) == 0 {
		return nil, errors.New("empty model_list")
	}
	return &cfg, nil
}

func convertCarrierModel(m carrierModelEntry) *CarrierDefaultModel {
	return &CarrierDefaultModel{
		ProviderID:     strings.TrimSpace(m.ProviderID),
		ProtocolFamily: resolveCarrierModelProtocolFamily(m.ProviderID, m.ProtocolFamily),
		ModelID:        strings.TrimSpace(m.Model),
		ModelName:      strings.TrimSpace(m.ModelName),
		ModelAlias:     strings.TrimSpace(m.ModelAlias),
		EnvVar:         strings.TrimSpace(m.EnvVar),
		BaseURL:        strings.TrimSpace(m.BaseURL),
	}
}

func resolveCarrierModelProtocolFamily(providerID, explicit string) string {
	if value := strings.TrimSpace(explicit); value != "" {
		return value
	}
	return strings.TrimSpace(catalog.ProtocolFamilyForProvider(providerID))
}

// NormalizeModelForProvider strips Carrier's provider-prefixed model notation
// (for example "openrouter/foo/bar") into the upstream model id expected by
// provider runtimes such as ZeroClaw.
func NormalizeModelForProvider(providerID, modelID string) string {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
	}
	if slash := strings.Index(modelID, "/"); slash > 0 && slash < len(modelID)-1 {
		prefix := strings.TrimSpace(modelID[:slash])
		if prefix != "" {
			managedProvider := strings.TrimSpace(catalog.MapToManagedProvider(providerID))
			if strings.EqualFold(prefix, providerID) || (managedProvider != "" && strings.EqualFold(prefix, managedProvider)) {
				return strings.TrimSpace(modelID[slash+1:])
			}
		}
	}
	return modelID
}

func resolveCarrierConfigV2Path() (string, error) {
	if path := strings.TrimSpace(os.Getenv("CARRIER_CONFIG")); path != "" {
		return path, nil
	}
	home, err := userHomeDirFn()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".carrier", "config.v2.json"), nil
}
