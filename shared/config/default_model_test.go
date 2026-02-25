package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeCarrierDefaultModelFixture(t *testing.T, defaultModel string, modelList []map[string]string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.v2.json")
	t.Setenv("CARRIER_CONFIG", path)
	payload := map[string]interface{}{
		"config_version": 2,
		"default_model":  defaultModel,
		"model_list":     modelList,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestLoadCarrierDefaultModelUsesNamedDefault(t *testing.T) {
	writeCarrierDefaultModelFixture(t, "openai-default", []map[string]string{
		{
			"model_name":  "openai-codex-default",
			"model":       "openai-codex/gpt-5.3-codex",
			"provider_id": "openai-codex",
			"env_var":     "OPENAI_CODEX_TOKEN",
		},
		{
			"model_name":  "openai-default",
			"model":       "openai/gpt-5.2",
			"provider_id": "openai",
			"env_var":     "OPENAI_API_KEY",
		},
	})

	got, err := LoadCarrierDefaultModel()
	if err != nil {
		t.Fatalf("LoadCarrierDefaultModel error: %v", err)
	}
	if got.ProviderID != "openai" {
		t.Fatalf("ProviderID = %q, want %q", got.ProviderID, "openai")
	}
	if got.ModelID != "openai/gpt-5.2" {
		t.Fatalf("ModelID = %q, want %q", got.ModelID, "openai/gpt-5.2")
	}
	if got.EnvVar != "OPENAI_API_KEY" {
		t.Fatalf("EnvVar = %q, want %q", got.EnvVar, "OPENAI_API_KEY")
	}
}

func TestLoadCarrierDefaultModelFallsBackToFirstEntry(t *testing.T) {
	writeCarrierDefaultModelFixture(t, "missing-default", []map[string]string{
		{
			"model_name":  "openrouter-default",
			"model":       "openrouter/auto",
			"provider_id": "openrouter",
			"env_var":     "OPENROUTER_API_KEY",
		},
		{
			"model_name":  "openai-default",
			"model":       "openai/gpt-5.2",
			"provider_id": "openai",
			"env_var":     "OPENAI_API_KEY",
		},
	})

	got, err := LoadCarrierDefaultModel()
	if err != nil {
		t.Fatalf("LoadCarrierDefaultModel error: %v", err)
	}
	if got.ProviderID != "openrouter" {
		t.Fatalf("ProviderID = %q, want %q", got.ProviderID, "openrouter")
	}
	if got.ModelID != "openrouter/auto" {
		t.Fatalf("ModelID = %q, want %q", got.ModelID, "openrouter/auto")
	}
}

func TestLoadCarrierDefaultModelRejectsEmptyModelList(t *testing.T) {
	writeCarrierDefaultModelFixture(t, "", []map[string]string{})

	_, err := LoadCarrierDefaultModel()
	if err == nil {
		t.Fatal("expected error for empty model_list")
	}
}
