package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// CarrierDefaultModel describes the selected default model in carrier config.v2.
type CarrierDefaultModel struct {
	ProviderID string
	ModelID    string
	ModelName  string
	EnvVar     string
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

func loadCarrierDefaultModelFromPath(path string) (*CarrierDefaultModel, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg struct {
		DefaultModel string `json:"default_model"`
		ModelList    []struct {
			ModelName  string `json:"model_name"`
			Model      string `json:"model"`
			ProviderID string `json:"provider_id"`
			EnvVar     string `json:"env_var"`
		} `json:"model_list"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	if len(cfg.ModelList) == 0 {
		return nil, errors.New("empty model_list")
	}
	defaultName := strings.TrimSpace(cfg.DefaultModel)
	if defaultName != "" {
		for _, m := range cfg.ModelList {
			if strings.EqualFold(strings.TrimSpace(m.ModelName), defaultName) {
				return &CarrierDefaultModel{
					ProviderID: strings.TrimSpace(m.ProviderID),
					ModelID:    strings.TrimSpace(m.Model),
					ModelName:  strings.TrimSpace(m.ModelName),
					EnvVar:     strings.TrimSpace(m.EnvVar),
				}, nil
			}
		}
	}
	m := cfg.ModelList[0]
	return &CarrierDefaultModel{
		ProviderID: strings.TrimSpace(m.ProviderID),
		ModelID:    strings.TrimSpace(m.Model),
		ModelName:  strings.TrimSpace(m.ModelName),
		EnvVar:     strings.TrimSpace(m.EnvVar),
	}, nil
}

func resolveCarrierConfigV2Path() (string, error) {
	if path := strings.TrimSpace(os.Getenv("CARRIER_CONFIG")); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".carrier", "config.v2.json"), nil
}
