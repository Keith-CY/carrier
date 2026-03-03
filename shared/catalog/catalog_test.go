package catalog

import "testing"

func TestProviderCatalogBasics(t *testing.T) {
	providers := ListProviders()
	if len(providers) != 6 {
		t.Fatalf("provider count = %d, want 6", len(providers))
	}
	for _, id := range []string{"anthropic", "openai", "openai-codex", "openrouter", "ollama", "openai-compatible"} {
		p := GetProvider(id)
		if p == nil {
			t.Fatalf("expected provider %q", id)
		}
		if p.ID != id {
			t.Fatalf("provider id = %q, want %q", p.ID, id)
		}
	}
	// Unknown provider returns nil.
	if p := GetProvider("nonexistent-provider"); p != nil {
		t.Fatalf("expected nil for unknown provider, got %#v", p)
	}
	// Aliases should resolve to their canonical provider.
	for _, alias := range []string{"vllm", "openai-v1"} {
		p := GetProvider(alias)
		if p == nil {
			t.Fatalf("alias %q should resolve to openai-compatible", alias)
		}
		if p.ID != "openai-compatible" {
			t.Fatalf("GetProvider(%q).ID = %q, want openai-compatible", alias, p.ID)
		}
	}
	if !IsSupportedProvider("openai") {
		t.Fatal("openai should be supported")
	}
	if !IsSupportedProvider("openrouter") {
		t.Fatal("openrouter should be supported")
	}
	ids := SupportedProviderIDs()
	if len(ids) != 6 {
		t.Fatalf("SupportedProviderIDs len = %d, want 6", len(ids))
	}
	if ids[0] != "anthropic" || ids[1] != "openai" || ids[2] != "openai-codex" || ids[3] != "openrouter" || ids[4] != "ollama" || ids[5] != "openai-compatible" {
		t.Fatalf("unexpected SupportedProviderIDs order: %#v", ids)
	}
}

func TestProvidersByCategory(t *testing.T) {
	byCategory := ProvidersByCategory()
	if len(byCategory["builtin"]) != 2 {
		t.Fatalf("builtin len = %d, want 2", len(byCategory["builtin"]))
	}
	if len(byCategory["custom"]) != 1 {
		t.Fatalf("custom len = %d, want 1", len(byCategory["custom"]))
	}
	if len(byCategory["compatible"]) != 3 {
		t.Fatalf("compatible len = %d, want 3", len(byCategory["compatible"]))
	}
}

func TestChannelCatalogBasics(t *testing.T) {
	channels := ListChannels()
	if len(channels) != 3 {
		t.Fatalf("channel count = %d, want 3", len(channels))
	}
	for _, id := range []string{"telegram", "discord", "feishu"} {
		c := GetChannel(id)
		if c == nil {
			t.Fatalf("expected channel %q", id)
		}
	}
	if c := GetChannel("qq"); c != nil {
		t.Fatalf("expected nil for unsupported channel qq, got %#v", c)
	}
	if !IsSupportedChannel("telegram") {
		t.Fatal("telegram should be supported")
	}
	if IsSupportedChannel("line") {
		t.Fatal("line should not be supported")
	}
}

func TestMapToManagedProvider(t *testing.T) {
	cases := map[string]string{
		"openai-codex":      "openai",
		"openai-compatible": "openai",
		"openrouter":        "openrouter",
		"ollama":            "ollama",
		"openai":            "openai",
		"anthropic":         "anthropic",
		"  custom  ":        "custom",
	}
	for input, want := range cases {
		if got := MapToManagedProvider(input); got != want {
			t.Fatalf("MapToManagedProvider(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestProviderAliasesFor(t *testing.T) {
	aliases := ProviderAliasesFor("openai-compatible")
	if len(aliases) != 2 {
		t.Fatalf("ProviderAliasesFor(openai-compatible) len = %d, want 2", len(aliases))
	}
	seen := map[string]bool{}
	for _, a := range aliases {
		seen[a] = true
	}
	if !seen["vllm"] || !seen["openai-v1"] {
		t.Fatalf("expected vllm and openai-v1, got %v", aliases)
	}
	// Provider with no aliases returns nil.
	if got := ProviderAliasesFor("anthropic"); got != nil {
		t.Fatalf("ProviderAliasesFor(anthropic) = %v, want nil", got)
	}
}

func TestNormalizeProviderID(t *testing.T) {
	cases := map[string]string{
		"vllm":               "openai-compatible",
		"VLLM":               "openai-compatible",
		"openai-v1":          "openai-compatible",
		"  openai-v1  ":      "openai-compatible",
		"openai":             "openai",
		"anthropic":          "anthropic",
		"openai-compatible":  "openai-compatible",
		"unknown-provider":   "unknown-provider",
	}
	for input, want := range cases {
		if got := NormalizeProviderID(input); got != want {
			t.Fatalf("NormalizeProviderID(%q) = %q, want %q", input, got, want)
		}
	}
}
