package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFindAvailablePort(t *testing.T) {
	port, err := findAvailablePort(9090, 9190)
	if err != nil {
		t.Fatalf("findAvailablePort error: %v", err)
	}
	if port < 9090 || port > 9190 {
		t.Fatalf("port = %d, want range [9090, 9190]", port)
	}
}

func TestAutoAllocatePortOnAdd(t *testing.T) {
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
	envVars := map[string]string{"OPENAI_API_KEY": "sk-unit-test"}

	result, err := prepareManagedAgentAddArtifacts(
		"openclaw",
		"openclaw-port",
		"telegram",
		"tg-token",
		provider,
		envVars,
		"",
	)
	if err != nil {
		t.Fatalf("prepareManagedAgentAddArtifacts error: %v", err)
	}
	if result.Port < 9090 || result.Port > 9190 {
		t.Fatalf("result.Port = %d, want range [9090, 9190]", result.Port)
	}

	raw, err := os.ReadFile(result.RecordPath)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("parse record: %v", err)
	}
	port := int(payload["port"].(float64))
	if port != result.Port {
		t.Fatalf("record port = %d, want %d", port, result.Port)
	}
}
