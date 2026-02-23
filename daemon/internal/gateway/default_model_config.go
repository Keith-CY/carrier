package gateway

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type carrierDefaultModelConfig struct {
	ProviderID string
	ModelID    string
	ModelName  string
	EnvVar     string
}

func readCarrierDefaultProviderID() string {
	cfg, err := readCarrierDefaultModelConfig()
	if err != nil || cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.ProviderID)
}

func readCarrierDefaultModelConfig() (*carrierDefaultModelConfig, error) {
	path, err := resolveCarrierConfigV2Path()
	if err != nil {
		return nil, err
	}
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
				return &carrierDefaultModelConfig{
					ProviderID: strings.TrimSpace(m.ProviderID),
					ModelID:    strings.TrimSpace(m.Model),
					ModelName:  strings.TrimSpace(m.ModelName),
					EnvVar:     strings.TrimSpace(m.EnvVar),
				}, nil
			}
		}
	}
	m := cfg.ModelList[0]
	return &carrierDefaultModelConfig{
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
