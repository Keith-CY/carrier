package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"carrier/configv2"
)

func TestRunOnboardOverridesStaleDefaultModelEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_CONFIG", filepath.Join(tmp, "config.v2.json"))
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")

	// Simulate stale defaults from a previous onboarding.
	t.Setenv("CARRIER_DEFAULT_PROVIDER_ID", "openai-codex")
	t.Setenv("CARRIER_DEFAULT_MODEL_ID", "openai-codex/gpt-5.3-codex")
	t.Setenv("CARRIER_DEFAULT_MODEL_NAME", "openai-codex-default")
	t.Setenv("CARRIER_DEFAULT_PROVIDER_ENV", "OPENAI_CODEX_TOKEN")
	t.Setenv("OPENAI_CODEX_TOKEN", "")
	t.Setenv("OPENAI_API_KEY", "")

	origDaemonHealthProbe := daemonHealthProbe
	origGatewayHealthProbe := gatewayHealthProbe
	origDaemonPairCodeFetcher := daemonPairCodeFetcher
	t.Cleanup(func() {
		daemonHealthProbe = origDaemonHealthProbe
		gatewayHealthProbe = origGatewayHealthProbe
		daemonPairCodeFetcher = origDaemonPairCodeFetcher
	})

	daemonHealthProbe = func(string) bool { return true }
	gatewayHealthProbe = func(string) bool { return false }
	daemonPairCodeFetcher = func(string) (string, string, error) { return "", "", nil }

	startCalls := 0
	startGateway := func() error {
		startCalls++
		return nil
	}

	// Input order:
	// 1) Telegram token
	// 2) Provider override (switch from openai-codex to openai)
	// 3) OpenAI API key
	input := "tg-test-token\nopenai\nsk-test-openai\n"
	var out bytes.Buffer
	if err := runOnboard(strings.NewReader(input), &out, startGateway); err != nil {
		t.Fatalf("runOnboard error: %v", err)
	}
	if startCalls != 1 {
		t.Fatalf("start gateway called %d times, want 1", startCalls)
	}

	cfg, _, err := configv2.Load()
	if err != nil {
		t.Fatalf("configv2.Load: %v", err)
	}
	if cfg.DefaultModel != "openai-default" {
		t.Fatalf("cfg.DefaultModel = %q, want %q", cfg.DefaultModel, "openai-default")
	}
	if len(cfg.ModelList) != 1 {
		t.Fatalf("len(cfg.ModelList) = %d, want 1", len(cfg.ModelList))
	}
	defaultModel := cfg.ModelList[0]
	if defaultModel.ProviderID != "openai" {
		t.Fatalf("default model provider_id = %q, want %q", defaultModel.ProviderID, "openai")
	}
	if defaultModel.Model != "openai/gpt-5.2" {
		t.Fatalf("default model id = %q, want %q", defaultModel.Model, "openai/gpt-5.2")
	}
	if got := os.Getenv("OPENAI_API_KEY"); got != "sk-test-openai" {
		t.Fatalf("OPENAI_API_KEY = %q, want %q", got, "sk-test-openai")
	}
	if got := os.Getenv("CARRIER_DEFAULT_PROVIDER_ID"); got != "openai-codex" {
		t.Fatalf("CARRIER_DEFAULT_PROVIDER_ID should stay untouched, got %q", got)
	}
	if got := os.Getenv("CARRIER_DEFAULT_MODEL_ID"); got != "openai-codex/gpt-5.3-codex" {
		t.Fatalf("CARRIER_DEFAULT_MODEL_ID should stay untouched, got %q", got)
	}
	if got := os.Getenv("CARRIER_DEFAULT_MODEL_NAME"); got != "openai-codex-default" {
		t.Fatalf("CARRIER_DEFAULT_MODEL_NAME should stay untouched, got %q", got)
	}
	if got := os.Getenv("CARRIER_DEFAULT_PROVIDER_ENV"); got != "OPENAI_CODEX_TOKEN" {
		t.Fatalf("CARRIER_DEFAULT_PROVIDER_ENV should stay untouched, got %q", got)
	}
}
