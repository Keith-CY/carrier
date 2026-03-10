package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var userHomeDirFn = os.UserHomeDir

// CarrierDefaultModel describes the selected default model in carrier config.v2.
type CarrierDefaultModel struct {
	ProviderID string
	ModelID    string
	ModelName  string
	EnvVar     string
	BaseURL    string
}

type carrierModelEntry struct {
	ModelName  string `json:"model_name"`
	Model      string `json:"model"`
	ProviderID string `json:"provider_id"`
	EnvVar     string `json:"env_var"`
	BaseURL    string `json:"base_url,omitempty"`
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
	cfg, err := loadCarrierModelConfigFromPath(path)
	if err != nil {
		return nil, err
	}
	target := strings.ToLower(strings.TrimSpace(providerID))
	if target == "" {
		return nil, errors.New("provider_id is required")
	}
	for _, m := range cfg.ModelList {
		if strings.EqualFold(strings.TrimSpace(m.ProviderID), target) {
			return convertCarrierModel(m), nil
		}
	}
	return nil, errors.New("provider_id is not found in model_list")
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
		ProviderID: strings.TrimSpace(m.ProviderID),
		ModelID:    strings.TrimSpace(m.Model),
		ModelName:  strings.TrimSpace(m.ModelName),
		EnvVar:     strings.TrimSpace(m.EnvVar),
		BaseURL:    strings.TrimSpace(m.BaseURL),
	}
}

// NormalizeModelForProvider strips Carrier's provider-prefixed model notation
// (for example "openrouter/foo/bar") into the upstream model id expected by
// provider runtimes such as ZeroClaw.
func NormalizeModelForProvider(providerID, modelID string) string {
	_ = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ""
	}
	if slash := strings.Index(modelID, "/"); slash > 0 && slash < len(modelID)-1 {
		return strings.TrimSpace(modelID[slash+1:])
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
