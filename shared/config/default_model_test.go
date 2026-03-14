package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeModelForProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		want     string
	}{
		{
			name:     "openrouter strips provider prefix",
			provider: "openrouter",
			model:    "openrouter/arcee-ai/trinity-mini:free",
			want:     "arcee-ai/trinity-mini:free",
		},
		{
			name:     "openai strips provider prefix",
			provider: "openai",
			model:    "openai/gpt-5.2",
			want:     "gpt-5.2",
		},
		{
			name:     "plain model id remains unchanged",
			provider: "anthropic",
			model:    "claude-opus-4-6",
			want:     "claude-opus-4-6",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeModelForProvider(tc.provider, tc.model); got != tc.want {
				t.Fatalf("NormalizeModelForProvider(%q, %q) = %q, want %q", tc.provider, tc.model, got, tc.want)
			}
		})
	}
}

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

func TestLoadCarrierDefaultModelPreservesBaseURL(t *testing.T) {
	writeCarrierDefaultModelFixture(t, "openrouter-default", []map[string]string{
		{
			"model_name":  "openrouter-default",
			"model":       "openrouter/arcee-ai/trinity-mini:free",
			"provider_id": "openrouter",
			"env_var":     "OPENROUTER_API_KEY",
			"base_url":    "https://openrouter.ai/api/v1",
		},
	})

	got, err := LoadCarrierDefaultModel()
	if err != nil {
		t.Fatalf("LoadCarrierDefaultModel error: %v", err)
	}
	if got.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("BaseURL = %q, want %q", got.BaseURL, "https://openrouter.ai/api/v1")
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

func TestLoadCarrierModelProfilesResolveProtocolFamily(t *testing.T) {
	writeCarrierDefaultModelFixture(t, "openrouter-fast", []map[string]string{
		{
			"model_name":  "openrouter-fast",
			"model_alias": "flash",
			"model":       "openrouter/google/gemini-2.0-flash-001",
			"provider_id": "openrouter",
			"env_var":     "OPENROUTER_API_KEY",
			"base_url":    "https://openrouter.ai/api/v1",
		},
		{
			"model_name":  "ollama-dev",
			"model":       "ollama/llama3.2",
			"provider_id": "ollama",
		},
		{
			"model_name":  "codex-default",
			"model":       "openai-codex/gpt-5.3-codex",
			"provider_id": "openai-codex",
			"env_var":     "OPENAI_CODEX_TOKEN",
		},
	})

	openrouterProfiles, err := LoadCarrierModelProfilesForProvider("openrouter")
	if err != nil {
		t.Fatalf("LoadCarrierModelProfilesForProvider error: %v", err)
	}
	if len(openrouterProfiles) != 1 {
		t.Fatalf("expected 1 openrouter profile, got %d", len(openrouterProfiles))
	}
	if openrouterProfiles[0].ProtocolFamily != "openai-compatible" {
		t.Fatalf("ProtocolFamily = %q, want %q", openrouterProfiles[0].ProtocolFamily, "openai-compatible")
	}
	if openrouterProfiles[0].ModelAlias != "flash" {
		t.Fatalf("ModelAlias = %q, want %q", openrouterProfiles[0].ModelAlias, "flash")
	}

	oauthProfiles, err := LoadCarrierModelProfilesForProtocolFamily("oauth-openai")
	if err != nil {
		t.Fatalf("LoadCarrierModelProfilesForProtocolFamily error: %v", err)
	}
	if len(oauthProfiles) != 1 {
		t.Fatalf("expected 1 oauth-openai profile, got %d", len(oauthProfiles))
	}
	if oauthProfiles[0].ProviderID != "openai-codex" {
		t.Fatalf("ProviderID = %q, want %q", oauthProfiles[0].ProviderID, "openai-codex")
	}

	ollamaProfiles, err := LoadCarrierModelProfilesForProtocolFamily("ollama")
	if err != nil {
		t.Fatalf("LoadCarrierModelProfilesForProtocolFamily error: %v", err)
	}
	if len(ollamaProfiles) != 1 {
		t.Fatalf("expected 1 ollama profile, got %d", len(ollamaProfiles))
	}
	if ollamaProfiles[0].ProtocolFamily != "ollama" {
		t.Fatalf("ProtocolFamily = %q, want %q", ollamaProfiles[0].ProtocolFamily, "ollama")
	}
}
