package gateway

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestParseZeroclawChannel(t *testing.T) {
	if _, ok := parseZeroclawChannel("telegram"); !ok {
		t.Fatal("expected telegram channel to be supported")
	}
	if _, ok := parseZeroclawChannel("discord"); ok {
		t.Fatal("did not expect discord channel to be supported in managed zeroclaw flow")
	}
}

func TestPrepareZeroclawManagedOnboard_WritesConfigAndRecord(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	sess := &OnboardSession{
		SelectedAgent:    "zeroclaw",
		SelectedChannel:  "telegram",
		ChannelToken:     "telegram-token-zero",
		SelectedProvider: "openai",
		EnvVars: map[string]string{
			"OPENAI_API_KEY": "sk-zeroclaw-123",
		},
	}

	result, err := prepareZeroclawManagedOnboard(sess, "telegram:418258935")
	if err != nil {
		t.Fatalf("prepareZeroclawManagedOnboard: %v", err)
	}
	if result.WorkspacePath == "" || result.ConfigPath == "" || result.RecordPath == "" {
		t.Fatalf("expected non-empty output paths, got %+v", result)
	}
	if got := strings.TrimSpace(sess.EnvVars["ZEROCLAW_API_KEY"]); got != "sk-zeroclaw-123" {
		t.Fatalf("expected ZEROCLAW_API_KEY populated from provider token, got %q", got)
	}

	cfgRaw, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
		t.Fatalf("parse config json: %v", err)
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
	if strings.Contains(string(recordRaw), "sk-zeroclaw-123") || strings.Contains(string(recordRaw), "telegram-token-zero") {
		t.Fatalf("managed record should not contain secret token values: %s", recordRaw)
	}
}
