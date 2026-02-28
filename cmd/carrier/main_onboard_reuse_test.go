package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptProviderAuthMinimalReusesSavedOAuthCredential(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")

	const savedToken = "codex-token-saved"
	if _, err := saveProviderCredential("openai-codex", savedToken); err != nil {
		t.Fatalf("saveProviderCredential(openai-codex): %v", err)
	}

	provider, ok := resolveChoice("openai-codex", providerOptions)
	if !ok {
		t.Fatal("openai-codex provider option is missing")
	}

	origFlow := runOpenAICodexDeviceCodeFlow
	t.Cleanup(func() {
		runOpenAICodexDeviceCodeFlow = origFlow
	})
	runOpenAICodexDeviceCodeFlow = func(out io.Writer) (string, error) {
		t.Fatal("device-code flow should not run when saved credential is reused")
		return "", nil
	}

	var out bytes.Buffer
	credEnv, provided, err := promptProviderAuthMinimal(
		bufio.NewReader(strings.NewReader("y\n")),
		&out,
		provider,
	)
	if err != nil {
		t.Fatalf("promptProviderAuthMinimal error: %v", err)
	}
	if !provided {
		t.Fatal("expected provider credential to be provided")
	}
	if got := strings.TrimSpace(credEnv["OPENAI_CODEX_TOKEN"]); got != savedToken {
		t.Fatalf("OPENAI_CODEX_TOKEN = %q, want %q", got, savedToken)
	}
	if !strings.Contains(out.String(), "Reuse saved "+provider.Name+" credential") {
		t.Fatalf("stdout missing reuse prompt: %q", out.String())
	}
}

func TestPromptProviderAuthMinimalReusesEnvCredential(t *testing.T) {
	t.Setenv("OPENAI_CODEX_TOKEN", "env-codex-token")
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(t.TempDir(), "credentials.json"))

	provider, ok := resolveChoice("openai-codex", providerOptions)
	if !ok {
		t.Fatal("openai-codex provider option is missing")
	}

	origFlow := runOpenAICodexDeviceCodeFlow
	t.Cleanup(func() {
		runOpenAICodexDeviceCodeFlow = origFlow
	})
	runOpenAICodexDeviceCodeFlow = func(out io.Writer) (string, error) {
		t.Fatal("device-code flow should not run when env credential is reused")
		return "", nil
	}

	var out bytes.Buffer
	credEnv, provided, err := promptProviderAuthMinimal(
		bufio.NewReader(strings.NewReader("y\n")),
		&out,
		provider,
	)
	if err != nil {
		t.Fatalf("promptProviderAuthMinimal error: %v", err)
	}
	if !provided {
		t.Fatal("expected provider credential to be provided")
	}
	if got := strings.TrimSpace(credEnv["OPENAI_CODEX_TOKEN"]); got != "env-codex-token" {
		t.Fatalf("OPENAI_CODEX_TOKEN = %q, want %q", got, "env-codex-token")
	}
	if !strings.Contains(out.String(), "environment variable OPENAI_CODEX_TOKEN") {
		t.Fatalf("stdout missing env reuse prompt: %q", out.String())
	}
}

func TestPromptChannelCredentialsMinimalReusesTelegramTokenFromConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.v2.json")
	t.Setenv("CARRIER_CONFIG", cfgPath)
	t.Setenv("CARRIER_TELEGRAM_BOT_TOKEN", "")

	raw := `{
  "config_version": 2,
  "channels": [
    {
      "id": "telegram",
      "bot_token": "tg-config-token",
      "enabled": true
    }
  ]
}`
	if err := os.WriteFile(cfgPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	channel, ok := resolveChoice("telegram", onboardChannelOptions)
	if !ok {
		t.Fatal("telegram channel option is missing")
	}

	var out bytes.Buffer
	env, cfg, err := promptChannelCredentialsMinimal(
		bufio.NewReader(strings.NewReader("y\n")),
		&out,
		channel,
	)
	if err != nil {
		t.Fatalf("promptChannelCredentialsMinimal error: %v", err)
	}

	if got := env["CARRIER_TELEGRAM_BOT_TOKEN"]; got != "tg-config-token" {
		t.Fatalf("CARRIER_TELEGRAM_BOT_TOKEN = %q, want %q", got, "tg-config-token")
	}
	if cfg.BotToken != "tg-config-token" {
		t.Fatalf("cfg.BotToken = %q, want %q", cfg.BotToken, "tg-config-token")
	}
	if cfg.TransportMode != "auto" {
		t.Fatalf("cfg.TransportMode = %q, want %q", cfg.TransportMode, "auto")
	}
	if !strings.Contains(out.String(), "Reuse existing Telegram token") {
		t.Fatalf("stdout missing channel reuse prompt: %q", out.String())
	}
}

func TestPromptChannelCredentialsMinimalAllowsDecliningTelegramTokenReuse(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.v2.json")
	t.Setenv("CARRIER_CONFIG", cfgPath)
	t.Setenv("CARRIER_TELEGRAM_BOT_TOKEN", "")

	raw := `{
  "config_version": 2,
  "channels": [
    {
      "id": "telegram",
      "bot_token": "tg-config-token",
      "enabled": true
    }
  ]
}`
	if err := os.WriteFile(cfgPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	channel, ok := resolveChoice("telegram", onboardChannelOptions)
	if !ok {
		t.Fatal("telegram channel option is missing")
	}

	var out bytes.Buffer
	env, cfg, err := promptChannelCredentialsMinimal(
		bufio.NewReader(strings.NewReader("n\ntg-new-token\n")),
		&out,
		channel,
	)
	if err != nil {
		t.Fatalf("promptChannelCredentialsMinimal error: %v", err)
	}
	if got := env["CARRIER_TELEGRAM_BOT_TOKEN"]; got != "tg-new-token" {
		t.Fatalf("CARRIER_TELEGRAM_BOT_TOKEN = %q, want %q", got, "tg-new-token")
	}
	if cfg.BotToken != "tg-new-token" {
		t.Fatalf("cfg.BotToken = %q, want %q", cfg.BotToken, "tg-new-token")
	}
}
