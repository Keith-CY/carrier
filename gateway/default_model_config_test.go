package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeGatewayDefaultProviderConfig(t *testing.T, providerID, modelID, envVar string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.v2.json")
	t.Setenv("CARRIER_CONFIG", path)
	modelName := providerID + "-default"
	payload := map[string]interface{}{
		"config_version": 2,
		"default_model":  modelName,
		"model_list": []map[string]string{
			{
				"model_name":  modelName,
				"model":       modelID,
				"provider_id": providerID,
				"env_var":     envVar,
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal config payload: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write config payload: %v", err)
	}
	return path
}

func TestReadCarrierDefaultProviderIDMissingConfig(t *testing.T) {
	t.Setenv("CARRIER_CONFIG", filepath.Join(t.TempDir(), "missing.json"))
	if got := readCarrierDefaultProviderID(); got != "" {
		t.Fatalf("expected empty provider id, got %q", got)
	}
}

func TestReadCarrierDefaultProviderIDReturnsTrimmedProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.v2.json")
	t.Setenv("CARRIER_CONFIG", path)

	payload := map[string]interface{}{
		"config_version": 2,
		"default_model":  "my-default",
		"model_list": []map[string]string{
			{
				"model_name":  "my-default",
				"model":       "openai/gpt-5.1",
				"provider_id": " openai ",
				"env_var":     "OPENAI_API_KEY",
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal config payload: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write config payload: %v", err)
	}

	if got := readCarrierDefaultProviderID(); got != "openai" {
		t.Fatalf("provider id = %q, want %q", got, "openai")
	}
}
