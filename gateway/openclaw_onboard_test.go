package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOpenclawChannel(t *testing.T) {
	if _, ok := parseManagedChannel("openclaw", "telegram"); !ok {
		t.Fatal("expected telegram channel to be supported")
	}
	if _, ok := parseManagedChannel("openclaw", "discord"); ok {
		t.Fatal("did not expect discord channel to be supported in managed openclaw flow")
	}
}

func TestPrepareOpenclawManagedOnboard_WritesConfigAndRecord(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	sess := &OnboardSession{
		SelectedAgent:    "openclaw",
		SelectedChannel:  "telegram",
		ChannelToken:     "telegram-token-open",
		SelectedProvider: "openai",
		EnvVars: map[string]string{
			"OPENAI_API_KEY": "sk-openclaw-123",
		},
	}

	result, err := prepareManagedOnboard("openclaw", sess, "telegram:418258935")
	if err != nil {
		t.Fatalf("prepareManagedOnboard: %v", err)
	}
	if result.WorkspacePath == "" || result.ConfigPath == "" || result.RecordPath == "" {
		t.Fatalf("expected non-empty output paths, got %+v", result)
	}
	if !strings.HasSuffix(result.ConfigPath, "openclaw.json") {
		t.Fatalf("expected openclaw config path suffix openclaw.json, got %q", result.ConfigPath)
	}

	cfgRaw, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
		t.Fatalf("parse config json: %v", err)
	}

	agents, ok := cfg["agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agents object, got %#v", cfg["agents"])
	}
	defaults, ok := agents["defaults"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agents.defaults object, got %#v", agents["defaults"])
	}
	if defaults["workspace"] != result.WorkspacePath {
		t.Fatalf("workspace mismatch: got %v want %s", defaults["workspace"], result.WorkspacePath)
	}
	channels, ok := cfg["channels"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected channels object, got %#v", cfg["channels"])
	}
	telegram, ok := channels["telegram"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected channels.telegram object, got %#v", channels["telegram"])
	}
	if telegram["enabled"] != true {
		t.Fatalf("expected channels.telegram.enabled=true, got %#v", telegram["enabled"])
	}
	token, _ := telegram["botToken"].(string)
	if got := strings.TrimSpace(token); got != "telegram-token-open" {
		t.Fatalf("expected channels.telegram.botToken to be persisted, got %q", got)
	}
	allowFrom, _ := telegram["allowFrom"].([]interface{})
	if len(allowFrom) != 1 || strings.TrimSpace(anyToString(allowFrom[0])) != "418258935" {
		t.Fatalf("expected channels.telegram.allowFrom=[418258935], got %#v", telegram["allowFrom"])
	}
	if _, ok := telegram["setup_pending"]; ok {
		t.Fatalf("did not expect setup_pending for non-pending channel config")
	}
	models, _ := cfg["models"].(map[string]interface{})
	modelProviders, _ := models["providers"].(map[string]interface{})
	openaiProvider, _ := modelProviders["openai"].(map[string]interface{})
	apiKeyRef, _ := openaiProvider["apiKey"].(map[string]interface{})
	if got := strings.TrimSpace(anyToString(apiKeyRef["provider"])); got != "carrier_file" {
		t.Fatalf("expected models.providers.openai.apiKey.provider=carrier_file, got %q", got)
	}

	secretsPath := filepath.Join(tmp, ".openclaw", "carrier-secrets.json")
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
	if got := strings.TrimSpace(anyToString(secretOpenAI["apiKey"])); got != "sk-openclaw-123" {
		t.Fatalf("expected openclaw carrier secret providers.openai.apiKey, got %q", got)
	}

	recordRaw, err := os.ReadFile(result.RecordPath)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	var record map[string]interface{}
	if err := json.Unmarshal(recordRaw, &record); err != nil {
		t.Fatalf("parse record json: %v", err)
	}
	if record["agent_id"] != "openclaw" {
		t.Fatalf("unexpected agent_id: %v", record["agent_id"])
	}
	if record["workspace_path"] != result.WorkspacePath {
		t.Fatalf("record workspace_path mismatch: got %v want %s", record["workspace_path"], result.WorkspacePath)
	}
	if strings.Contains(string(recordRaw), "sk-openclaw-123") || strings.Contains(string(recordRaw), "telegram-token-open") {
		t.Fatalf("managed record should not contain secret token values: %s", recordRaw)
	}
}

func TestPrepareOpenclawManagedOnboard_AllowsPendingChannelSetupWithoutToken(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	sess := &OnboardSession{
		SelectedAgent:       "openclaw",
		SelectedChannel:     "telegram",
		ChannelSetupPending: true,
		SelectedProvider:    "openai",
		EnvVars: map[string]string{
			"OPENAI_API_KEY": "sk-openclaw-pending",
		},
	}

	result, err := prepareManagedOnboard("openclaw", sess, "telegram:418258935")
	if err != nil {
		t.Fatalf("prepareManagedOnboard: %v", err)
	}

	cfgRaw, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
		t.Fatalf("parse config json: %v", err)
	}
	channels, ok := cfg["channels"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected channels object, got %#v", cfg["channels"])
	}
	telegram, ok := channels["telegram"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected channels.telegram object, got %#v", channels["telegram"])
	}
	if telegram["enabled"] != false {
		t.Fatalf("expected channels.telegram.enabled=false when setup is pending, got %#v", telegram["enabled"])
	}
	if telegram["setup_pending"] != true {
		t.Fatalf("expected channels.telegram.setup_pending=true, got %#v", telegram["setup_pending"])
	}
	if _, ok := telegram["botToken"]; ok {
		t.Fatalf("did not expect channels.telegram.botToken when setup is pending")
	}

	recordRaw, err := os.ReadFile(result.RecordPath)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	var record map[string]interface{}
	if err := json.Unmarshal(recordRaw, &record); err != nil {
		t.Fatalf("parse record json: %v", err)
	}
	if record["channel_setup_pending"] != true {
		t.Fatalf("expected channel_setup_pending=true in record, got %#v", record["channel_setup_pending"])
	}
}

func TestPrepareOpenclawManagedOnboard_EmptyTokenAutoSetsPending(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	sess := &OnboardSession{
		SelectedAgent:    "openclaw",
		SelectedChannel:  "telegram",
		SelectedProvider: "openai",
		EnvVars: map[string]string{
			"OPENAI_API_KEY": "sk-openclaw-123",
		},
	}

	result, err := prepareManagedOnboard("openclaw", sess, "telegram:418258935")
	if err != nil {
		t.Fatalf("prepareManagedOnboard: %v", err)
	}

	cfgRaw, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
		t.Fatalf("parse config json: %v", err)
	}
	channels, _ := cfg["channels"].(map[string]interface{})
	telegram, _ := channels["telegram"].(map[string]interface{})
	if telegram["enabled"] != false || telegram["setup_pending"] != true {
		t.Fatalf("expected pending telegram channel, got %#v", telegram)
	}
}

func TestPrepareOpenclawManagedOnboard_RequiresOpenAIKey(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))

	sess := &OnboardSession{
		SelectedAgent:    "openclaw",
		SelectedChannel:  "telegram",
		ChannelToken:     "telegram-token-open",
		SelectedProvider: "openai-compatible",
		EnvVars:          map[string]string{},
	}

	if _, err := prepareManagedOnboard("openclaw", sess, "telegram:418258935"); err == nil {
		t.Fatal("expected error when OPENAI_API_KEY cannot be resolved")
	}
}

func TestPrepareOpenclawManagedOnboard_AllowsWebUIOnlyMode(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	sess := &OnboardSession{
		SelectedAgent:    "openclaw",
		SelectedChannel:  "",
		SelectedProvider: "openai",
		EnvVars: map[string]string{
			"OPENAI_API_KEY": "sk-openclaw-webui-only",
		},
	}

	result, err := prepareManagedOnboard("openclaw", sess, "webui:add")
	if err != nil {
		t.Fatalf("prepareManagedOnboard: %v", err)
	}

	cfgRaw, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
		t.Fatalf("parse config json: %v", err)
	}
	channels, ok := cfg["channels"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected channels object, got %#v", cfg["channels"])
	}
	if len(channels) != 0 {
		t.Fatalf("expected empty channels for webui-only mode, got %#v", channels)
	}
}
