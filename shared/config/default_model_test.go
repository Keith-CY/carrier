package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
			"model_name":  "openai-compatible-default",
			"model":       "openai-compatible/auto",
			"provider_id": "openai-compatible",
			"env_var":     "OPENAI_COMPATIBLE_API_KEY",
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
	if got.ProviderID != "openai-compatible" {
		t.Fatalf("ProviderID = %q, want %q", got.ProviderID, "openai-compatible")
	}
	if got.ModelID != "openai-compatible/auto" {
		t.Fatalf("ModelID = %q, want %q", got.ModelID, "openai-compatible/auto")
	}
}

func TestLoadCarrierDefaultModelRejectsEmptyModelList(t *testing.T) {
	writeCarrierDefaultModelFixture(t, "", []map[string]string{})

	_, err := LoadCarrierDefaultModel()
	if err == nil {
		t.Fatal("expected error for empty model_list")
	}
}

func TestLoadCarrierModelForProvider(t *testing.T) {
	writeCarrierDefaultModelFixture(t, "openai-default", []map[string]string{
		{
			"model_name":  "openai-default",
			"model":       "openai/gpt-5.1-codex",
			"provider_id": "openai",
			"env_var":     "OPENAI_API_KEY",
		},
		{
			"model_name":  "openai-codex-default",
			"model":       "openai-codex/gpt-5.3-codex",
			"provider_id": "openai-codex",
			"env_var":     "OPENAI_CODEX_TOKEN",
		},
	})

	got, err := LoadCarrierModelForProvider("openai-codex")
	if err != nil {
		t.Fatalf("LoadCarrierModelForProvider error: %v", err)
	}
	if got.ProviderID != "openai-codex" {
		t.Fatalf("ProviderID = %q, want %q", got.ProviderID, "openai-codex")
	}
	if got.ModelID != "openai-codex/gpt-5.3-codex" {
		t.Fatalf("ModelID = %q, want %q", got.ModelID, "openai-codex/gpt-5.3-codex")
	}

	if _, err := LoadCarrierModelForProvider("anthropic"); err == nil {
		t.Fatal("expected missing provider error")
	}
}

func TestLoadCarrierModelForProviderRequiresProviderID(t *testing.T) {
	writeCarrierDefaultModelFixture(t, "openai-default", []map[string]string{
		{
			"model_name":  "openai-default",
			"model":       "openai/gpt-5.2",
			"provider_id": "openai",
			"env_var":     "OPENAI_API_KEY",
		},
	})

	_, err := LoadCarrierModelForProvider("   ")
	if err == nil {
		t.Fatal("expected provider_id required error")
	}
	if !strings.Contains(err.Error(), "provider_id is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadCarrierModelForProviderResolvePathError(t *testing.T) {
	t.Setenv("CARRIER_CONFIG", "")

	origUserHomeDirFn := userHomeDirFn
	t.Cleanup(func() {
		userHomeDirFn = origUserHomeDirFn
	})
	userHomeDirFn = func() (string, error) {
		return "", errors.New("home resolution failed")
	}

	_, err := LoadCarrierModelForProvider("openai")
	if err == nil {
		t.Fatal("expected resolve path error")
	}
	if !strings.Contains(err.Error(), "home resolution failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadCarrierModelForProviderFromPathLoadError(t *testing.T) {
	_, err := loadCarrierModelForProviderFromPath(filepath.Join(t.TempDir(), "missing-config.v2.json"), "openai")
	if err == nil {
		t.Fatal("expected load error for missing config path")
	}
}
