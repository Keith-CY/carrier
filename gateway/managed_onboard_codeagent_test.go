package gateway

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestPrepareManagedOnboard_CodexSkipsChannelAndConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	sess := &OnboardSession{
		SelectedAgent:    "codex",
		SelectedProvider: "openai",
		EnvVars: map[string]string{
			"OPENAI_API_KEY": "sk-codex-test",
		},
	}

	result, err := prepareManagedOnboard("codex", sess, "webui:add")
	if err != nil {
		t.Fatalf("prepareManagedOnboard: %v", err)
	}
	if result.ConfigPath != "" {
		t.Fatalf("expected codex config path to be empty, got %q", result.ConfigPath)
	}
	if result.WorkspacePath == "" || result.RecordPath == "" {
		t.Fatalf("expected workspace and record paths, got %+v", result)
	}

	recordRaw, err := os.ReadFile(result.RecordPath)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	var record map[string]interface{}
	if err := json.Unmarshal(recordRaw, &record); err != nil {
		t.Fatalf("parse record: %v", err)
	}
	if got := strings.TrimSpace(anyToString(record["agent_id"])); got != "codex" {
		t.Fatalf("record agent_id = %q, want codex", got)
	}
	if got := strings.TrimSpace(anyToString(record["config_path"])); got != "" {
		t.Fatalf("record config_path = %q, want empty", got)
	}
}
