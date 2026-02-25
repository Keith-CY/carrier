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
