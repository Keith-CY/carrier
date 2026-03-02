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
	if p := GetProvider("vllm"); p != nil {
		t.Fatalf("expected nil for removed alias vllm, got %#v", p)
	}
	if !IsSupportedProvider("openai") {
		t.Fatal("openai should be supported")
	}
	if !IsSupportedProvider("claude-code") {
		t.Fatal("claude-code should be supported via alias")
	}
	if !IsSupportedProvider("opencode") {
		t.Fatal("opencode should be supported via alias")
	}
	if !IsSupportedProvider("openrouter") {
		t.Fatal("openrouter should be supported")
	}
	if p := GetProvider("claude-code"); p == nil || p.ID != "openai-codex" {
		t.Fatalf("GetProvider(claude-code) = %#v, want openai-codex", p)
	}
	if p := GetProvider("opencode"); p == nil || p.ID != "openai-codex" {
		t.Fatalf("GetProvider(opencode) = %#v, want openai-codex", p)
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
		"claude-code":       "openai",
		"opencode":          "openai",
		"anthropic":         "anthropic",
		"  custom  ":        "custom",
	}
	for input, want := range cases {
		if got := MapToManagedProvider(input); got != want {
			t.Fatalf("MapToManagedProvider(%q) = %q, want %q", input, got, want)
		}
	}
}
