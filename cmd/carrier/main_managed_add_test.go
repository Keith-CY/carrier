package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLatestManagedPairedChatIDUsesLatestInstance(t *testing.T) {
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(t.TempDir(), "instances.json"))

	path, err := managedInstancesPath()
	if err != nil {
		t.Fatalf("managedInstancesPath: %v", err)
	}
	if err := saveManagedInstances(path, []managedAgentInstance{
		{
			ID:           "openclaw-old",
			Type:         "openclaw",
			AgentID:      "openclaw",
			Channel:      "telegram",
			PairedChatID: "1001",
			UpdatedAt:    time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
		{
			ID:           "openclaw-new",
			Type:         "openclaw",
			AgentID:      "openclaw",
			Channel:      "telegram",
			PairedChatID: "2002",
			UpdatedAt:    time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
		{
			ID:           "openclaw-other-channel",
			Type:         "openclaw",
			AgentID:      "openclaw",
			Channel:      "discord",
			PairedChatID: "9999",
			UpdatedAt:    time.Date(2026, 2, 23, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	pairedChatID, source := latestManagedPairedChatID("openclaw", "telegram")
	if pairedChatID != "2002" {
		t.Fatalf("paired chat id = %q, want %q", pairedChatID, "2002")
	}
	if source != "latest managed instance" {
		t.Fatalf("source = %q, want %q", source, "latest managed instance")
	}
}

func TestLatestManagedInstanceProviderUsesLatestInstance(t *testing.T) {
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(t.TempDir(), "instances.json"))

	path, err := managedInstancesPath()
	if err != nil {
		t.Fatalf("managedInstancesPath: %v", err)
	}
	if err := saveManagedInstances(path, []managedAgentInstance{
		{
			ID:        "openclaw-old",
			Type:      "openclaw",
			AgentID:   "openclaw",
			Provider:  "openrouter",
			UpdatedAt: time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
		{
			ID:        "openclaw-new",
			Type:      "openclaw",
			AgentID:   "openclaw",
			Provider:  "openai",
			UpdatedAt: time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
		{
			ID:        "openclaw-empty-provider",
			Type:      "openclaw",
			AgentID:   "openclaw",
			Provider:  "",
			UpdatedAt: time.Date(2026, 2, 23, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
		{
			ID:        "picoclaw-newer",
			Type:      "picoclaw",
			AgentID:   "picoclaw",
			Provider:  "openai-codex",
			UpdatedAt: time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	providerID := latestManagedInstanceProvider("openclaw")
	if providerID != "openai" {
		t.Fatalf("provider id = %q, want %q", providerID, "openai")
	}
}

func TestPrepareManagedAgentAddArtifactsWritesPairedChatID(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)

	provider := choiceOption{
		ID:           "openai",
		Name:         "OpenAI",
		AuthMode:     authModeAPIKey,
		ProviderEnv:  "OPENAI_API_KEY",
		ExampleModel: "openai/gpt-5.2",
	}
	envVars := map[string]string{
		"OPENAI_API_KEY": "sk-unit-test",
	}

	result, err := prepareManagedAgentAddArtifacts(
		"openclaw",
		"openclaw-unit",
		"telegram",
		"tg-test-token",
		provider,
		envVars,
		"88990011",
	)
	if err != nil {
		t.Fatalf("prepareManagedAgentAddArtifacts: %v", err)
	}
	if !strings.HasSuffix(result.ConfigPath, "openclaw.json") {
		t.Fatalf("expected openclaw config path suffix openclaw.json, got %q", result.ConfigPath)
	}

	rawCfg, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfgPayload map[string]interface{}
	if err := json.Unmarshal(rawCfg, &cfgPayload); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	models, _ := cfgPayload["models"].(map[string]interface{})
	providers, _ := models["providers"].(map[string]interface{})
	openaiProvider, _ := providers["openai"].(map[string]interface{})
	apiKeyRef, _ := openaiProvider["apiKey"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(apiKeyRef["provider"])); got != "carrier_file" {
		t.Fatalf("models.providers.openai.apiKey.provider = %q, want carrier_file", got)
	}
	channels, _ := cfgPayload["channels"].(map[string]interface{})
	telegram, _ := channels["telegram"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(telegram["botToken"])); got != "tg-test-token" {
		t.Fatalf("channels.telegram.botToken = %q, want %q", got, "tg-test-token")
	}
	rawAllowFrom, _ := telegram["allowFrom"].([]interface{})
	allowFrom := make([]string, 0, len(rawAllowFrom))
	for _, item := range rawAllowFrom {
		allowFrom = append(allowFrom, strings.TrimSpace(anyToString(item)))
	}
	if len(allowFrom) != 1 || allowFrom[0] != "88990011" {
		t.Fatalf("channels.telegram.allowFrom = %v, want [%s]", allowFrom, "88990011")
	}

	secretsPath := filepath.Join(home, ".openclaw", "carrier-secrets.json")
	secretsRaw, err := os.ReadFile(secretsPath)
	if err != nil {
		t.Fatalf("read openclaw carrier secrets: %v", err)
	}
	var secretsPayload map[string]interface{}
	if err := json.Unmarshal(secretsRaw, &secretsPayload); err != nil {
		t.Fatalf("parse openclaw carrier secrets: %v", err)
	}
	secretProviders, _ := secretsPayload["providers"].(map[string]interface{})
	secretOpenAI, _ := secretProviders["openai"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(secretOpenAI["apiKey"])); got != "sk-unit-test" {
		t.Fatalf("carrier secrets providers.openai.apiKey = %q, want %q", got, "sk-unit-test")
	}

	var recordPayload struct {
		PairedChatID string `json:"paired_chat_id"`
	}
	rawRecord, err := os.ReadFile(result.RecordPath)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	if err := json.Unmarshal(rawRecord, &recordPayload); err != nil {
		t.Fatalf("parse record: %v", err)
	}
	if recordPayload.PairedChatID != "88990011" {
		t.Fatalf("record paired_chat_id = %q, want %q", recordPayload.PairedChatID, "88990011")
	}
}
