package configv2

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// unsetEnv removes env vars for the duration of the test, restoring them on cleanup.
// Unlike t.Setenv("", ""), this truly unsets the variable so os.LookupEnv returns exists=false.
func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		old, existed := os.LookupEnv(k)
		os.Unsetenv(k)
		if existed {
			t.Cleanup(func() { os.Setenv(k, old) })
		} else {
			t.Cleanup(func() { os.Unsetenv(k) })
		}
	}
}

// helper: create a temp dir and point CARRIER_CONFIG there, returning cleanup func.
func setupTempConfig(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	t.Setenv("CARRIER_CONFIG", filepath.Join(dir, "config.v2.json"))
	return dir
}

func writeConfig(t *testing.T, cfg *Config) string {
	t.Helper()
	path := os.Getenv("CARRIER_CONFIG")
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// ---------- Load ----------

func TestLoad_MissingFile_ReturnsDefault(t *testing.T) {
	setupTempConfig(t)
	cfg, path, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ConfigVersion != CurrentVersion {
		t.Errorf("expected config_version=%d, got %d", CurrentVersion, cfg.ConfigVersion)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}

func TestLoad_ValidFile(t *testing.T) {
	setupTempConfig(t)
	original := &Config{
		ConfigVersion: CurrentVersion,
		DefaultModel:  "gpt-4",
		Channels: []Channel{
			{ID: "telegram", BotToken: "tok123", Enabled: true},
		},
		ModelList: []Model{
			{ModelName: "gpt-4", Model: "gpt-4-turbo", ProviderID: "openai", EnvVar: "OPENAI_API_KEY"},
		},
	}
	writeConfig(t, original)

	cfg, _, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultModel != "gpt-4" {
		t.Errorf("expected default_model=gpt-4, got %s", cfg.DefaultModel)
	}
	if len(cfg.Channels) != 1 || cfg.Channels[0].BotToken != "tok123" {
		t.Errorf("channel mismatch: %+v", cfg.Channels)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	setupTempConfig(t)
	path := os.Getenv("CARRIER_CONFIG")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoad_WrongVersion(t *testing.T) {
	setupTempConfig(t)
	writeConfig(t, &Config{ConfigVersion: 999})

	_, _, err := Load()
	if err == nil {
		t.Fatal("expected error for wrong config_version")
	}
}

// ---------- Save ----------

func TestSave_RoundTrip(t *testing.T) {
	setupTempConfig(t)
	original := &Config{
		DefaultModel: "claude",
		Channels: []Channel{
			{ID: "discord", BotToken: "disc-tok", Enabled: true},
		},
		ModelList: []Model{
			{ModelName: "claude", Model: "claude-sonnet-4-20250514", ProviderID: "anthropic", EnvVar: "ANTHROPIC_API_KEY"},
		},
	}

	path, err := Save(original)
	if err != nil {
		t.Fatalf("save error: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}

	loaded, _, err := Load()
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if loaded.DefaultModel != "claude" {
		t.Errorf("round-trip default_model mismatch: %s", loaded.DefaultModel)
	}
	if loaded.ConfigVersion != CurrentVersion {
		t.Errorf("round-trip version mismatch: %d", loaded.ConfigVersion)
	}
	if len(loaded.Channels) != 1 || loaded.Channels[0].ID != "discord" {
		t.Errorf("round-trip channel mismatch: %+v", loaded.Channels)
	}
}

func TestSave_NilConfig(t *testing.T) {
	_, err := Save(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

// ---------- ApplyGatewayEnvironment ----------

func TestApplyGatewayEnvironment_Telegram(t *testing.T) {
	// Ensure env vars are truly unset so setEnvIfUnset can write them.
	unsetEnv(t,
		"CARRIER_TELEGRAM_BOT_TOKEN",
		"CARRIER_TELEGRAM_WEBHOOK_SECRET",
		"CARRIER_TELEGRAM_WEBHOOK_URL",
		"CARRIER_TELEGRAM_TRANSPORT_MODE",
	)

	cfg := &Config{
		Channels: []Channel{
			{
				ID:            "telegram",
				BotToken:      "tg-bot-token",
				WebhookSecret: "tg-secret",
				WebhookURL:    "https://example.com/hook",
				TransportMode: "webhook",
				Enabled:       true,
			},
		},
	}

	if err := ApplyGatewayEnvironment(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := map[string]string{
		"CARRIER_TELEGRAM_BOT_TOKEN":      "tg-bot-token",
		"CARRIER_TELEGRAM_WEBHOOK_SECRET": "tg-secret",
		"CARRIER_TELEGRAM_WEBHOOK_URL":    "https://example.com/hook",
		"CARRIER_TELEGRAM_TRANSPORT_MODE": "webhook",
	}
	for k, want := range checks {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s: expected %q, got %q", k, want, got)
		}
	}
}

func TestApplyGatewayEnvironment_Discord(t *testing.T) {
	unsetEnv(t, "CARRIER_DISCORD_BOT_TOKEN", "CARRIER_DISCORD_PUBLIC_KEY")

	cfg := &Config{
		Channels: []Channel{
			{ID: "discord", BotToken: "disc-tok", WebhookSecret: "disc-pk", Enabled: true},
		},
	}

	if err := ApplyGatewayEnvironment(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := os.Getenv("CARRIER_DISCORD_BOT_TOKEN"); got != "disc-tok" {
		t.Errorf("expected disc-tok, got %q", got)
	}
	if got := os.Getenv("CARRIER_DISCORD_PUBLIC_KEY"); got != "disc-pk" {
		t.Errorf("expected disc-pk, got %q", got)
	}
}

func TestApplyGatewayEnvironment_Feishu(t *testing.T) {
	unsetEnv(t, "CARRIER_FEISHU_APP_TOKEN", "CARRIER_FEISHU_VERIFICATION_TOKEN")

	cfg := &Config{
		Channels: []Channel{
			{ID: "feishu", BotToken: "feishu-tok", WebhookSecret: "feishu-vt", Enabled: true},
		},
	}

	if err := ApplyGatewayEnvironment(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := os.Getenv("CARRIER_FEISHU_APP_TOKEN"); got != "feishu-tok" {
		t.Errorf("expected feishu-tok, got %q", got)
	}
	if got := os.Getenv("CARRIER_FEISHU_VERIFICATION_TOKEN"); got != "feishu-vt" {
		t.Errorf("expected feishu-vt, got %q", got)
	}
}

func TestApplyGatewayEnvironment_DisabledChannelSkipped(t *testing.T) {
	unsetEnv(t, "CARRIER_TELEGRAM_BOT_TOKEN")

	cfg := &Config{
		Channels: []Channel{
			{ID: "telegram", BotToken: "should-not-set", Enabled: false},
		},
	}

	if err := ApplyGatewayEnvironment(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := os.Getenv("CARRIER_TELEGRAM_BOT_TOKEN"); got != "" {
		t.Errorf("disabled channel should not set env, got %q", got)
	}
}

func TestApplyGatewayEnvironment_EmptyFields(t *testing.T) {
	t.Setenv("CARRIER_TELEGRAM_BOT_TOKEN", "old-value")

	cfg := &Config{
		Channels: []Channel{
			{ID: "telegram", BotToken: "", Enabled: true},
		},
	}

	if err := ApplyGatewayEnvironment(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty bot_token should not overwrite existing env.
	if got := os.Getenv("CARRIER_TELEGRAM_BOT_TOKEN"); got != "old-value" {
		t.Errorf("empty field should not overwrite, got %q", got)
	}
}

func TestApplyGatewayEnvironment_Nil(t *testing.T) {
	if err := ApplyGatewayEnvironment(nil); err != nil {
		t.Fatalf("nil config should be no-op, got: %v", err)
	}
}

func TestApplyGatewayEnvironment_DefaultModel(t *testing.T) {
	unsetEnv(t,
		"CARRIER_DEFAULT_MODEL_NAME",
		"CARRIER_DEFAULT_MODEL_ID",
		"CARRIER_DEFAULT_PROVIDER_ID",
		"CARRIER_DEFAULT_PROVIDER_ENV",
	)

	cfg := &Config{
		DefaultModel: "my-model",
		ModelList: []Model{
			{ModelName: "my-model", Model: "gpt-4-turbo", ProviderID: "openai", EnvVar: "OPENAI_API_KEY"},
		},
	}

	if err := ApplyGatewayEnvironment(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checks := map[string]string{
		"CARRIER_DEFAULT_MODEL_NAME":  "my-model",
		"CARRIER_DEFAULT_MODEL_ID":    "gpt-4-turbo",
		"CARRIER_DEFAULT_PROVIDER_ID": "openai",
		"CARRIER_DEFAULT_PROVIDER_ENV": "OPENAI_API_KEY",
	}
	for k, want := range checks {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s: expected %q, got %q", k, want, got)
		}
	}
}

func TestApplyGatewayEnvironment_DefaultModelFallback(t *testing.T) {
	unsetEnv(t, "CARRIER_DEFAULT_MODEL_NAME")

	cfg := &Config{
		DefaultModel: "nonexistent",
		ModelList: []Model{
			{ModelName: "first", Model: "m1", ProviderID: "p1"},
		},
	}

	if err := ApplyGatewayEnvironment(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back to first model.
	if got := os.Getenv("CARRIER_DEFAULT_MODEL_NAME"); got != "first" {
		t.Errorf("expected fallback to first model, got %q", got)
	}
}

// ---------- pickDefaultModel ----------

func TestPickDefaultModel_Empty(t *testing.T) {
	if m := pickDefaultModel(nil); m != nil {
		t.Errorf("expected nil for nil config")
	}
	if m := pickDefaultModel(&Config{}); m != nil {
		t.Errorf("expected nil for empty model list")
	}
}

func TestPickDefaultModel_Match(t *testing.T) {
	cfg := &Config{
		DefaultModel: "second",
		ModelList: []Model{
			{ModelName: "first"},
			{ModelName: "second"},
		},
	}
	m := pickDefaultModel(cfg)
	if m == nil || m.ModelName != "second" {
		t.Errorf("expected second, got %+v", m)
	}
}

func TestPickDefaultModel_NoMatch_Fallback(t *testing.T) {
	cfg := &Config{
		DefaultModel: "missing",
		ModelList:    []Model{{ModelName: "only"}},
	}
	m := pickDefaultModel(cfg)
	if m == nil || m.ModelName != "only" {
		t.Errorf("expected fallback to first, got %+v", m)
	}
}

// ---------- DefaultPath ----------

func TestDefaultPath_EnvOverride(t *testing.T) {
	t.Setenv("CARRIER_CONFIG", "/custom/path.json")
	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if path != "/custom/path.json" {
		t.Errorf("expected /custom/path.json, got %s", path)
	}
}

// ---------- Credential file loading ----------

func TestLoadCredentialFromFile(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.json")
	t.Setenv("CARRIER_CREDENTIAL_STORE", credPath)

	creds := map[string]interface{}{
		"providers": map[string]string{
			"openai":    "sk-test-key",
			"anthropic": "ant-key",
		},
	}
	raw, _ := json.Marshal(creds)
	if err := os.WriteFile(credPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	val, ok := loadCredentialFromFile("openai")
	if !ok || val != "sk-test-key" {
		t.Errorf("expected sk-test-key, got %q (ok=%v)", val, ok)
	}

	val, ok = loadCredentialFromFile("anthropic")
	if !ok || val != "ant-key" {
		t.Errorf("expected ant-key, got %q (ok=%v)", val, ok)
	}

	_, ok = loadCredentialFromFile("missing")
	if ok {
		t.Error("expected not found for missing provider")
	}
}

func TestLoadCredentialFromFile_MissingFile(t *testing.T) {
	t.Setenv("CARRIER_CREDENTIAL_STORE", "/nonexistent/path.json")
	_, ok := loadCredentialFromFile("any")
	if ok {
		t.Error("expected not found for missing credential file")
	}
}

func TestApplyGatewayEnvironment_CredentialProjection(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials.json")
	t.Setenv("CARRIER_CREDENTIAL_STORE", credPath)
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	unsetEnv(t, "OPENAI_API_KEY")

	creds := map[string]interface{}{
		"providers": map[string]string{"openai": "sk-from-cred"},
	}
	raw, _ := json.Marshal(creds)
	os.WriteFile(credPath, raw, 0o600)

	cfg := &Config{
		ModelList: []Model{
			{ModelName: "gpt4", ProviderID: "openai", EnvVar: "OPENAI_API_KEY", CredentialRef: "openai"},
		},
	}

	if err := ApplyGatewayEnvironment(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := os.Getenv("OPENAI_API_KEY"); got != "sk-from-cred" {
		t.Errorf("expected sk-from-cred, got %q", got)
	}
}
