package gateway

import (
	"testing"
)

func TestListLLMProviders_NotEmpty(t *testing.T) {
	providers := ListLLMProviders()
	if len(providers) == 0 {
		t.Fatal("expected non-empty provider list")
	}
}

func TestListLLMProviders_ReturnsCopy(t *testing.T) {
	p1 := ListLLMProviders()
	p2 := ListLLMProviders()
	if len(p1) != len(p2) {
		t.Fatal("expected same length on repeated calls")
	}
	// Mutating the returned slice should not affect subsequent calls
	p1[0].ID = "mutated"
	p3 := ListLLMProviders()
	if p3[0].ID == "mutated" {
		t.Error("ListLLMProviders should return an independent copy")
	}
}

func TestGetLLMProvider_KnownProviders(t *testing.T) {
	cases := []struct {
		id           string
		wantAuthMode AuthMode
		wantEnvVar   string
		wantCategory string
	}{
		{"anthropic", AuthModeAPIKey, "ANTHROPIC_API_KEY", "builtin"},
		{"openai", AuthModeAPIKey, "OPENAI_API_KEY", "builtin"},
		{"openai-codex", AuthModeOAuthDeviceCode, "OPENAI_CODEX_TOKEN", "custom"},
		{"openrouter", AuthModeAPIKey, "OPENROUTER_API_KEY", "compatible"},
		{"ollama", AuthModeNone, "", "compatible"},
		{"openai-compatible", AuthModeAPIKey, "OPENAI_COMPATIBLE_API_KEY", "compatible"},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			p := GetLLMProvider(tc.id)
			if p == nil {
				t.Fatalf("GetLLMProvider(%q) returned nil", tc.id)
			}
			if p.ID != tc.id {
				t.Errorf("ID: got %q, want %q", p.ID, tc.id)
			}
			if p.AuthMode != tc.wantAuthMode {
				t.Errorf("AuthMode: got %q, want %q", p.AuthMode, tc.wantAuthMode)
			}
			if p.EnvVar != tc.wantEnvVar {
				t.Errorf("EnvVar: got %q, want %q", p.EnvVar, tc.wantEnvVar)
			}
			if p.Category != tc.wantCategory {
				t.Errorf("Category: got %q, want %q", p.Category, tc.wantCategory)
			}
			if p.Name == "" {
				t.Errorf("Name should not be empty for %q", tc.id)
			}
		})
	}
}

func TestGetLLMProvider_Unknown(t *testing.T) {
	p := GetLLMProvider("nonexistent-provider")
	if p != nil {
		t.Errorf("expected nil for unknown provider, got %+v", p)
	}
}

func TestGetLLMProvider_AliasResolution(t *testing.T) {
	p := GetLLMProvider("vllm")
	if p == nil {
		t.Fatal("expected vllm alias to resolve to openai-compatible")
	}
	if p.ID != "openai-compatible" {
		t.Fatalf("GetLLMProvider(vllm).ID = %q, want openai-compatible", p.ID)
	}
}

func TestGetLLMProvider_ReturnsCopy(t *testing.T) {
	p1 := GetLLMProvider("anthropic")
	if p1 == nil {
		t.Fatal("expected non-nil")
	}
	original := p1.ID
	p1.ID = "tampered"
	p2 := GetLLMProvider("anthropic")
	if p2.ID != original {
		t.Error("GetLLMProvider should return an independent copy")
	}
}

func TestLLMProvidersByCategory(t *testing.T) {
	bycat := LLMProvidersByCategory()

	for _, cat := range []string{"builtin", "custom", "compatible", "generic"} {
		providers, ok := bycat[cat]
		if !ok {
			t.Errorf("category %q missing from result", cat)
			continue
		}
		for _, p := range providers {
			if p.Category != cat {
				t.Errorf("provider %q has category %q but is in bucket %q", p.ID, p.Category, cat)
			}
		}
	}

	// Check specific placements
	builtin := bycat["builtin"]
	found := false
	for _, p := range builtin {
		if p.ID == "anthropic" {
			found = true
			break
		}
	}
	if !found {
		t.Error("anthropic should be in builtin category")
	}

	compatible := bycat["compatible"]
	foundOpenAICompat := false
	for _, p := range compatible {
		if p.ID == "openai-compatible" {
			foundOpenAICompat = true
			break
		}
	}
	if !foundOpenAICompat {
		t.Error("openai-compatible should be in compatible category")
	}

	custom := bycat["custom"]
	foundCodex := false
	for _, p := range custom {
		if p.ID == "openai-codex" {
			foundCodex = true
		}
	}
	if !foundCodex {
		t.Error("openai-codex should be in custom category")
	}
}

func TestLLMProvidersByCategory_TotalCount(t *testing.T) {
	all := ListLLMProviders()
	bycat := LLMProvidersByCategory()
	total := len(bycat["builtin"]) + len(bycat["custom"]) + len(bycat["compatible"]) + len(bycat["generic"])
	if total != len(all) {
		t.Errorf("by-category total %d != catalog total %d", total, len(all))
	}
}

func TestAllProvidersHaveRequiredFields(t *testing.T) {
	for _, p := range ListLLMProviders() {
		if p.ID == "" {
			t.Errorf("provider has empty ID: %+v", p)
		}
		if p.Name == "" {
			t.Errorf("provider %q has empty Name", p.ID)
		}
		if p.AuthMode == "" {
			t.Errorf("provider %q has empty AuthMode", p.ID)
		}
		if p.Category == "" {
			t.Errorf("provider %q has empty Category", p.ID)
		}
	}
}

func TestAuthModeValues(t *testing.T) {
	modes := []AuthMode{
		AuthModeAPIKey,
		AuthModeOAuthDeviceCode,
		AuthModeOAuthPlugin,
		AuthModeGcloudADC,
		AuthModeNone,
	}
	expected := []string{"api_key", "oauth_device_code", "oauth_plugin", "gcloud_adc", "none"}
	for i, m := range modes {
		if string(m) != expected[i] {
			t.Errorf("AuthMode %d: got %q, want %q", i, m, expected[i])
		}
	}
}
