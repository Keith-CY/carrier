package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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

	var cfgPayload struct {
		Providers map[string]struct {
			APIKey string `json:"api_key"`
		} `json:"providers"`
		Channels map[string]struct {
			AllowFrom []string `json:"allow_from"`
		} `json:"channels"`
	}
	rawCfg, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := json.Unmarshal(rawCfg, &cfgPayload); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if got := cfgPayload.Providers["openai"].APIKey; got != "sk-unit-test" {
		t.Fatalf("providers.openai.api_key = %q, want %q", got, "sk-unit-test")
	}
	allowFrom := cfgPayload.Channels["telegram"].AllowFrom
	if len(allowFrom) != 1 || allowFrom[0] != "88990011" {
		t.Fatalf("channels.telegram.allow_from = %v, want [%s]", allowFrom, "88990011")
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
