package gateway

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestParseZeroclawChannel(t *testing.T) {
	if _, ok := parseManagedChannel("zeroclaw", "telegram"); !ok {
		t.Fatal("expected telegram channel to be supported")
	}
	if _, ok := parseManagedChannel("zeroclaw", "discord"); ok {
		t.Fatal("did not expect discord channel to be supported in managed zeroclaw flow")
	}
}

func TestPrepareZeroclawManagedOnboard_WritesTOMLConfigAndRecord(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_MANAGED_ZEROCLAW_VERSION", "0.1.7")

	sess := &OnboardSession{
		SelectedAgent:    "zeroclaw",
		SelectedChannel:  "telegram",
		ChannelToken:     "telegram-token-zero",
		SelectedProvider: "openai",
		EnvVars: map[string]string{
			"OPENAI_API_KEY": "sk-zeroclaw-123",
		},
	}

	result, err := prepareManagedOnboard("zeroclaw", sess, "telegram:418258935")
	if err != nil {
		t.Fatalf("prepareManagedOnboard: %v", err)
	}
	if result.WorkspacePath == "" || result.ConfigPath == "" || result.RecordPath == "" {
		t.Fatalf("expected non-empty output paths, got %+v", result)
	}
	if !strings.HasSuffix(result.ConfigPath, "config.toml") {
		t.Fatalf("expected zeroclaw config.toml path, got %q", result.ConfigPath)
	}
	if got := strings.TrimSpace(sess.EnvVars["ZEROCLAW_API_KEY"]); got != "sk-zeroclaw-123" {
		t.Fatalf("expected ZEROCLAW_API_KEY populated from provider token, got %q", got)
	}

	cfgRaw, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	cfgText := string(cfgRaw)
	for _, snippet := range []string{
		`api_key = "sk-zeroclaw-123"`,
		`default_provider = "openai"`,
		`default_model = "openai/`,
		`[channels_config.telegram]`,
		`bot_token = "telegram-token-zero"`,
		`allowed_users = ["418258935"]`,
	} {
		if !strings.Contains(cfgText, snippet) {
			t.Fatalf("expected config to contain %q, got:\n%s", snippet, cfgText)
		}
	}

	recordRaw, err := os.ReadFile(result.RecordPath)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	var record map[string]interface{}
	if err := json.Unmarshal(recordRaw, &record); err != nil {
		t.Fatalf("parse record json: %v", err)
	}
	if record["agent_id"] != "zeroclaw" {
		t.Fatalf("unexpected agent_id: %v", record["agent_id"])
	}
	if record["renderer_id"] != "zeroclaw.toml.v1" {
		t.Fatalf("unexpected renderer_id: %v", record["renderer_id"])
	}
	if record["config_format"] != "toml" {
		t.Fatalf("unexpected config_format: %v", record["config_format"])
	}
	if strings.Contains(string(recordRaw), "sk-zeroclaw-123") || strings.Contains(string(recordRaw), "telegram-token-zero") {
		t.Fatalf("managed record should not contain secret token values: %s", recordRaw)
	}
}

func TestPrepareZeroclawManagedOnboard_RejectsUnsupportedVersion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_MANAGED_ZEROCLAW_VERSION", "2.0.0")

	sess := &OnboardSession{
		SelectedAgent:    "zeroclaw",
		SelectedChannel:  "telegram",
		ChannelToken:     "telegram-token-zero",
		SelectedProvider: "openai",
		EnvVars: map[string]string{
			"OPENAI_API_KEY": "sk-zeroclaw-123",
		},
	}

	if _, err := prepareManagedOnboard("zeroclaw", sess, "telegram:418258935"); err == nil || !strings.Contains(err.Error(), "unsupported zeroclaw version") {
		t.Fatalf("expected unsupported version error, got %v", err)
	}
}

func TestPrepareZeroclawManagedOnboard_AllowsWebUIOnlyMode(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_MANAGED_ZEROCLAW_VERSION", "0.1.7")

	sess := &OnboardSession{
		SelectedAgent:    "zeroclaw",
		SelectedChannel:  "",
		SelectedProvider: "openai",
		EnvVars: map[string]string{
			"OPENAI_API_KEY": "sk-zeroclaw-webui",
		},
	}

	result, err := prepareManagedOnboard("zeroclaw", sess, "webui:add")
	if err != nil {
		t.Fatalf("prepareManagedOnboard: %v", err)
	}

	cfgRaw, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	cfgText := string(cfgRaw)
	if strings.Contains(cfgText, "[channels_config.") {
		t.Fatalf("expected no channels_config section in webui-only mode, got:\n%s", cfgText)
	}
	if !strings.Contains(cfgText, "No chat channel configured (WebUI-only mode)") {
		t.Fatalf("expected webui-only comment in config, got:\n%s", cfgText)
	}
}
