package main

import (
	"encoding/json"
	"fmt"
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
			Provider:  "anthropic",
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

func TestLatestManagedInstanceProviderNormalizesAliasProvider(t *testing.T) {
	t.Setenv("CARRIER_INSTANCE_STORE", filepath.Join(t.TempDir(), "instances.json"))

	path, err := managedInstancesPath()
	if err != nil {
		t.Fatalf("managedInstancesPath: %v", err)
	}
	if err := saveManagedInstances(path, []managedAgentInstance{
		{
			ID:        "openclaw-legacy",
			Type:      "openclaw",
			AgentID:   "openclaw",
			Provider:  "claude-code",
			UpdatedAt: time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		},
	}); err != nil {
		t.Fatalf("saveManagedInstances: %v", err)
	}

	providerID := latestManagedInstanceProvider("openclaw")
	if providerID != "openai-codex" {
		t.Fatalf("provider id = %q, want %q", providerID, "openai-codex")
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

func TestPrepareManagedAgentAddArtifactsAllowsWebUIOnlyChannel(t *testing.T) {
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
		"openclaw-webui-only",
		"",
		"",
		provider,
		envVars,
		"",
	)
	if err != nil {
		t.Fatalf("prepareManagedAgentAddArtifacts: %v", err)
	}

	rawCfg, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfgPayload map[string]interface{}
	if err := json.Unmarshal(rawCfg, &cfgPayload); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	channels, _ := cfgPayload["channels"].(map[string]interface{})
	if len(channels) != 0 {
		t.Fatalf("channels = %#v, want empty map for WebUI-only", channels)
	}
}

func TestPrepareManagedAgentAddArtifactsAllowsPendingChannelToken(t *testing.T) {
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
		"picoclaw",
		"picoclaw-channel-pending",
		"telegram",
		"",
		provider,
		envVars,
		"",
	)
	if err != nil {
		t.Fatalf("prepareManagedAgentAddArtifacts: %v", err)
	}

	rawCfg, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfgPayload map[string]interface{}
	if err := json.Unmarshal(rawCfg, &cfgPayload); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	channels, _ := cfgPayload["channels"].(map[string]interface{})
	telegram, _ := channels["telegram"].(map[string]interface{})
	if telegram["enabled"] != false {
		t.Fatalf("channels.telegram.enabled = %#v, want false", telegram["enabled"])
	}
	if telegram["setup_pending"] != true {
		t.Fatalf("channels.telegram.setup_pending = %#v, want true", telegram["setup_pending"])
	}
	if _, ok := telegram["token"]; ok {
		t.Fatalf("channels.telegram.token should be omitted when setup is pending")
	}
}

func TestPrepareManagedAgentAddArtifactsWritesZeroClawTOMLConfig(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)

	provider := choiceOption{
		ID:           "openrouter",
		Name:         "OpenRouter",
		AuthMode:     authModeAPIKey,
		ProviderEnv:  "OPENROUTER_API_KEY",
		ExampleModel: "openrouter/arcee-ai/trinity-mini:free",
	}
	envVars := map[string]string{
		"OPENROUTER_API_KEY": "sk-or-test",
	}

	result, err := prepareManagedAgentAddArtifacts(
		"zeroclaw",
		"zeroclaw-unit",
		"",
		"",
		provider,
		envVars,
		"",
	)
	if err != nil {
		t.Fatalf("prepareManagedAgentAddArtifacts: %v", err)
	}
	if !strings.HasSuffix(result.ConfigPath, "config.toml") {
		t.Fatalf("expected zeroclaw config path suffix config.toml, got %q", result.ConfigPath)
	}

	rawCfg, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read zeroclaw config: %v", err)
	}
	cfgText := string(rawCfg)
	for _, want := range []string{
		`api_key = "sk-or-test"`,
		`default_provider = "openrouter"`,
		`default_model = "arcee-ai/trinity-mini:free"`,
		"[gateway]",
		fmt.Sprintf("port = %d", result.Port),
		`host = "127.0.0.1"`,
		"require_pairing = false",
	} {
		if !strings.Contains(cfgText, want) {
			t.Fatalf("zeroclaw config missing %q\nconfig:\n%s", want, cfgText)
		}
	}

	var recordPayload struct {
		ConfigPath string `json:"config_path"`
		Port       int    `json:"port"`
	}
	rawRecord, err := os.ReadFile(result.RecordPath)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	if err := json.Unmarshal(rawRecord, &recordPayload); err != nil {
		t.Fatalf("parse record: %v", err)
	}
	if !strings.HasSuffix(recordPayload.ConfigPath, "config.toml") {
		t.Fatalf("record config_path = %q, want config.toml suffix", recordPayload.ConfigPath)
	}
	if recordPayload.Port != result.Port {
		t.Fatalf("record port = %d, want %d", recordPayload.Port, result.Port)
	}
}

func TestPrepareManagedAgentAddArtifactsUsesConfiguredModelForProvider(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)

	configPath := filepath.Join(t.TempDir(), "config.v2.json")
	if err := os.WriteFile(configPath, []byte(`{
  "default_model": "openrouter-live",
  "model_list": [
    {
      "model_name": "openrouter-live",
      "model": "openrouter/google/gemini-2.0-flash-001",
      "provider_id": "openrouter",
      "credential_ref": "openrouter"
    }
  ]
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CARRIER_CONFIG", configPath)

	provider := choiceOption{
		ID:           "openrouter",
		Name:         "OpenRouter",
		AuthMode:     authModeAPIKey,
		ProviderEnv:  "OPENROUTER_API_KEY",
		ExampleModel: "openrouter/arcee-ai/trinity-mini:free",
	}
	envVars := map[string]string{
		"OPENROUTER_API_KEY": "sk-or-test",
	}

	result, err := prepareManagedAgentAddArtifacts(
		"zeroclaw",
		"zeroclaw-unit",
		"",
		"",
		provider,
		envVars,
		"",
	)
	if err != nil {
		t.Fatalf("prepareManagedAgentAddArtifacts: %v", err)
	}

	rawCfg, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read zeroclaw config: %v", err)
	}
	cfgText := string(rawCfg)
	if !strings.Contains(cfgText, `default_model = "google/gemini-2.0-flash-001"`) {
		t.Fatalf("expected configured provider model in zeroclaw config, got:\n%s", cfgText)
	}
	if strings.Contains(cfgText, `default_model = "arcee-ai/trinity-mini:free"`) {
		t.Fatalf("expected configured provider model to override provider example model, got:\n%s", cfgText)
	}
}

func TestPrepareManagedAgentAddArtifactsPreservesOpenRouterVendorModel(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)

	configPath := filepath.Join(t.TempDir(), "config.v2.json")
	if err := os.WriteFile(configPath, []byte(`{
  "default_model": "openrouter-live",
  "model_list": [
    {
      "model_name": "openrouter-live",
      "model": "openai/gpt-4o-mini",
      "provider_id": "openrouter",
      "credential_ref": "openrouter"
    }
  ]
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CARRIER_CONFIG", configPath)

	provider := choiceOption{
		ID:           "openrouter",
		Name:         "OpenRouter",
		AuthMode:     authModeAPIKey,
		ProviderEnv:  "OPENROUTER_API_KEY",
		ExampleModel: "openrouter/arcee-ai/trinity-mini:free",
	}
	envVars := map[string]string{
		"OPENROUTER_API_KEY": "sk-or-test",
	}

	result, err := prepareManagedAgentAddArtifacts(
		"zeroclaw",
		"zeroclaw-unit",
		"",
		"",
		provider,
		envVars,
		"",
	)
	if err != nil {
		t.Fatalf("prepareManagedAgentAddArtifacts: %v", err)
	}

	rawCfg, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read zeroclaw config: %v", err)
	}
	cfgText := string(rawCfg)
	for _, want := range []string{
		`default_provider = "openrouter"`,
		`default_model = "openai/gpt-4o-mini"`,
		`provider = "openrouter"`,
		`model = "openai/gpt-4o-mini"`,
	} {
		if !strings.Contains(cfgText, want) {
			t.Fatalf("expected zeroclaw config to contain %q, got:\n%s", want, cfgText)
		}
	}
	if strings.Contains(cfgText, `default_provider = "openai"`) {
		t.Fatalf("expected zeroclaw config to preserve openrouter as provider, got:\n%s", cfgText)
	}
}

func TestPrepareManagedAgentAddArtifactsRendersProtocolProfiles(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	t.Setenv("HOME", home)

	configPath := filepath.Join(t.TempDir(), "config.v2.json")
	if err := os.WriteFile(configPath, []byte(`{
  "default_model": "openrouter-fast",
  "model_list": [
    {
      "model_name": "openrouter-fast",
      "model_alias": "flash",
      "model": "openrouter/google/gemini-2.0-flash-001",
      "provider_id": "openrouter",
      "env_var": "OPENROUTER_API_KEY",
      "base_url": "https://openrouter.ai/api/v1"
    },
    {
      "model_name": "openrouter-safe",
      "model_alias": "flash",
      "model": "openrouter/deepseek/deepseek-chat-v3-0324",
      "provider_id": "openrouter",
      "env_var": "OPENROUTER_API_KEY",
      "base_url": "https://openrouter.ai/api/v1"
    }
  ]
}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CARRIER_CONFIG", configPath)

	provider := choiceOption{
		ID:           "openrouter",
		Name:         "OpenRouter",
		AuthMode:     authModeAPIKey,
		ProviderEnv:  "OPENROUTER_API_KEY",
		ExampleModel: "openrouter/arcee-ai/trinity-mini:free",
	}
	envVars := map[string]string{
		"OPENROUTER_API_KEY": "sk-or-test",
	}

	result, err := prepareManagedAgentAddArtifacts(
		"zeroclaw",
		"zeroclaw-unit",
		"",
		"",
		provider,
		envVars,
		"",
	)
	if err != nil {
		t.Fatalf("prepareManagedAgentAddArtifacts: %v", err)
	}

	rawCfg, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read zeroclaw config: %v", err)
	}
	cfgText := string(rawCfg)
	if !strings.Contains(cfgText, `[provider_profiles.openrouter_fast]`) {
		t.Fatalf("expected zeroclaw config to contain managed profile section, got:\n%s", cfgText)
	}
	if !strings.Contains(cfgText, `model_alias = "flash"`) {
		t.Fatalf("expected zeroclaw config to contain model alias, got:\n%s", cfgText)
	}
	if !strings.Contains(cfgText, `[provider_profiles.openrouter_safe]`) {
		t.Fatalf("expected zeroclaw config to contain second managed profile section, got:\n%s", cfgText)
	}
}
