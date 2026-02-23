package configv2

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaultConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.v2.json")
	t.Setenv("CARRIER_CONFIG", path)

	cfg, gotPath, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if gotPath != path {
		t.Fatalf("Load path = %q, want %q", gotPath, path)
	}
	if cfg == nil {
		t.Fatal("Load returned nil config")
	}
	if cfg.ConfigVersion != CurrentVersion {
		t.Fatalf("config version = %d, want %d", cfg.ConfigVersion, CurrentVersion)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.v2.json")
	t.Setenv("CARRIER_CONFIG", path)
	if err := os.WriteFile(path, []byte(`{"config_version":2,`), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}

	if _, _, err := Load(); err == nil {
		t.Fatal("expected parse error for invalid JSON")
	}
}

func TestLoadUnsupportedConfigVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.v2.json")
	t.Setenv("CARRIER_CONFIG", path)
	if err := os.WriteFile(path, []byte(`{"config_version":1}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, _, err := Load(); err == nil {
		t.Fatal("expected unsupported config_version error")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.v2.json")
	t.Setenv("CARRIER_CONFIG", path)

	original := &Config{
		Channels: []Channel{
			{
				ID:            "telegram",
				BotToken:      "tg-token",
				WebhookSecret: "tg-secret",
				WebhookURL:    "https://example.com/webhook/telegram",
				TransportMode: "auto",
				Enabled:       true,
			},
		},
		ModelList: []Model{
			{
				ModelName:     "openai-default",
				Model:         "openai/gpt-5.2",
				ProviderID:    "openai",
				AuthMode:      "api_key",
				EnvVar:        "OPENAI_API_KEY",
				CredentialRef: "openai",
			},
		},
		DefaultModel: "openai-default",
		BaseAgent: BaseAgentSpec{
			Enabled:           true,
			PublicMemoryID:    "carrier.base.public.v1",
			ActiveMemoryID:    "carrier.base.active.v1",
			SelfHealBackupDir: "base-agent-memory-backups",
		},
		ConfiguredAt: "2026-02-21T10:00:00Z",
	}

	if _, err := Save(original); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	loaded, _, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.ConfigVersion != CurrentVersion {
		t.Fatalf("ConfigVersion = %d, want %d", loaded.ConfigVersion, CurrentVersion)
	}
	if len(loaded.Channels) != 1 || loaded.Channels[0].BotToken != "tg-token" {
		t.Fatalf("unexpected channels after round-trip: %+v", loaded.Channels)
	}
	if len(loaded.ModelList) != 1 || loaded.ModelList[0].CredentialRef != "openai" {
		t.Fatalf("unexpected model_list after round-trip: %+v", loaded.ModelList)
	}
	if loaded.DefaultModel != "openai-default" {
		t.Fatalf("DefaultModel = %q, want %q", loaded.DefaultModel, "openai-default")
	}
}

func TestApplyGatewayEnvironmentCoversChannelsAndProviders(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "credentials.json")
	credRaw, err := json.Marshal(map[string]any{
		"providers": map[string]string{
			"openai":       "sk-openai",
			"openai-codex": "codex-token",
		},
	})
	if err != nil {
		t.Fatalf("marshal credentials: %v", err)
	}
	if err := os.WriteFile(storePath, credRaw, 0o600); err != nil {
		t.Fatalf("write credentials file: %v", err)
	}

	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", storePath)
	t.Setenv("CARRIER_DEFAULT_MODEL_NAME", "")
	t.Setenv("CARRIER_DEFAULT_MODEL_ID", "")
	t.Setenv("CARRIER_DEFAULT_PROVIDER_ID", "")
	t.Setenv("CARRIER_DEFAULT_PROVIDER_ENV", "")

	cfg := &Config{
		Channels: []Channel{
			{
				ID:            "telegram",
				BotToken:      "tg-token",
				WebhookSecret: "tg-secret",
				WebhookURL:    "https://example.com/webhook/telegram",
				TransportMode: "auto",
				Enabled:       true,
			},
		},
		ModelList: []Model{
			{
				ModelName:     "openai-default",
				Model:         "openai/gpt-5.2",
				ProviderID:    "openai",
				AuthMode:      "api_key",
				EnvVar:        "OPENAI_API_KEY",
				CredentialRef: "openai",
			},
			{
				ModelName:     "openai-codex-default",
				Model:         "openai-codex/gpt-5.3-codex",
				ProviderID:    "openai-codex",
				AuthMode:      "oauth_device_code",
				EnvVar:        "OPENAI_CODEX_TOKEN",
				CredentialRef: "openai-codex",
			},
		},
		DefaultModel: "openai-codex-default",
	}

	if err := ApplyGatewayEnvironment(cfg); err != nil {
		t.Fatalf("ApplyGatewayEnvironment returned error: %v", err)
	}

	assertEnv(t, "CARRIER_TELEGRAM_BOT_TOKEN", "tg-token")
	assertEnv(t, "CARRIER_TELEGRAM_WEBHOOK_SECRET", "tg-secret")
	assertEnv(t, "CARRIER_TELEGRAM_WEBHOOK_URL", "https://example.com/webhook/telegram")
	assertEnv(t, "CARRIER_TELEGRAM_TRANSPORT_MODE", "auto")
	assertEnv(t, "OPENAI_API_KEY", "sk-openai")
	assertEnv(t, "OPENAI_CODEX_TOKEN", "codex-token")
	assertEnv(t, "CARRIER_DEFAULT_MODEL_NAME", "")
	assertEnv(t, "CARRIER_DEFAULT_MODEL_ID", "")
	assertEnv(t, "CARRIER_DEFAULT_PROVIDER_ID", "")
	assertEnv(t, "CARRIER_DEFAULT_PROVIDER_ENV", "")
}

func TestApplyGatewayEnvironmentPreservesExistingEnv(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(storePath, []byte(`{"providers":{"openai":"sk-from-store"}}`), 0o600); err != nil {
		t.Fatalf("write credentials file: %v", err)
	}
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", storePath)
	t.Setenv("CARRIER_TELEGRAM_BOT_TOKEN", "existing-telegram-token")
	t.Setenv("OPENAI_API_KEY", "existing-openai-token")

	cfg := &Config{
		Channels: []Channel{
			{ID: "telegram", BotToken: "new-token", Enabled: true},
		},
		ModelList: []Model{
			{
				ModelName:     "openai-default",
				Model:         "openai/gpt-5.2",
				ProviderID:    "openai",
				EnvVar:        "OPENAI_API_KEY",
				CredentialRef: "openai",
			},
		},
		DefaultModel: "openai-default",
	}

	if err := ApplyGatewayEnvironment(cfg); err != nil {
		t.Fatalf("ApplyGatewayEnvironment returned error: %v", err)
	}
	assertEnv(t, "CARRIER_TELEGRAM_BOT_TOKEN", "existing-telegram-token")
	assertEnv(t, "OPENAI_API_KEY", "existing-openai-token")
}

func assertEnv(t *testing.T, key, want string) {
	t.Helper()
	got := os.Getenv(key)
	if got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}
