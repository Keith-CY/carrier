package gateway

import (
	"testing"
)

func TestSetupStore_ConfigureAndGet(t *testing.T) {
	s := NewSetupStore()

	if s.IsConfigured() {
		t.Error("should not be configured initially")
	}
	if s.GetRedacted() != nil {
		t.Error("GetRedacted should return nil when not configured")
	}

	cfg := s.Configure(ProviderTelegram, "bot-token-123", "webhook-secret-456")
	if cfg == nil {
		t.Fatal("Configure returned nil")
	}
	if cfg.Provider != ProviderTelegram {
		t.Errorf("provider: %q", cfg.Provider)
	}
	if cfg.Token != "bot-token-123" {
		t.Errorf("token: %q", cfg.Token)
	}
	if cfg.ConfiguredAt == "" {
		t.Error("configured_at should be set")
	}

	if !s.IsConfigured() {
		t.Error("should be configured after Configure")
	}

	redacted := s.GetRedacted()
	if redacted == nil {
		t.Fatal("GetRedacted returned nil after configure")
	}
	if redacted.Provider != ProviderTelegram {
		t.Errorf("redacted provider: %q", redacted.Provider)
	}
	if redacted.ConfiguredAt == "" {
		t.Error("redacted configured_at should be set")
	}
}

func TestSetupStore_ReconfigureReplaces(t *testing.T) {
	s := NewSetupStore()
	s.Configure(ProviderTelegram, "tok1", "sec1")
	s.Configure(ProviderDiscord, "tok2", "sec2")

	cfg := s.GetConfig()
	if cfg.Provider != ProviderDiscord {
		t.Errorf("expected discord, got %q", cfg.Provider)
	}
}

func TestIsValidProviderType(t *testing.T) {
	valid := []string{"telegram", "discord", "feishu", "dummy"}
	invalid := []string{"slack", "teams", "", "TELEGRAM"}

	for _, p := range valid {
		if !IsValidProviderType(p) {
			t.Errorf("%q should be valid", p)
		}
	}
	for _, p := range invalid {
		if IsValidProviderType(p) {
			t.Errorf("%q should not be valid", p)
		}
	}
}
