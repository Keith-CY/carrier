package catalog

import "testing"

func TestProviderCatalogBasics(t *testing.T) {
	providers := ListProviders()
	if len(providers) != 4 {
		t.Fatalf("provider count = %d, want 4", len(providers))
	}
	for _, id := range []string{"anthropic", "openai", "openai-codex", "openai-compatible"} {
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
	if IsSupportedProvider("openrouter") {
		t.Fatal("openrouter should not be supported")
	}
	ids := SupportedProviderIDs()
	if len(ids) != 4 {
		t.Fatalf("SupportedProviderIDs len = %d, want 4", len(ids))
	}
	if ids[0] != "anthropic" || ids[1] != "openai" || ids[2] != "openai-codex" || ids[3] != "openai-compatible" {
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
	if len(byCategory["generic"]) != 1 {
		t.Fatalf("generic len = %d, want 1", len(byCategory["generic"]))
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
