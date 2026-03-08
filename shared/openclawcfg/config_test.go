package openclawcfg

import "testing"

func TestProviderSecretPointerEscapesPathSegments(t *testing.T) {
	got := ProviderSecretPointer("foo~/bar")
	want := "/providers/foo~0~1bar/apiKey"
	if got != want {
		t.Fatalf("ProviderSecretPointer()=%q, want %q", got, want)
	}
}

func TestProviderSecretPointerFallsBackToOpenAIWhenEmpty(t *testing.T) {
	got := ProviderSecretPointer("   ")
	want := "/providers/openai/apiKey"
	if got != want {
		t.Fatalf("ProviderSecretPointer()=%q, want %q", got, want)
	}
}

func TestChannelTokenFieldTelegramUsesBotToken(t *testing.T) {
	if got := ChannelTokenField("telegram"); got != "botToken" {
		t.Fatalf("ChannelTokenField(telegram)=%q, want botToken", got)
	}
	if got := ChannelTokenField("discord"); got != "token" {
		t.Fatalf("ChannelTokenField(discord)=%q, want token", got)
	}
}

func TestBuildProviderEntryIncludesFileSecretPointerWhenRequested(t *testing.T) {
	got := BuildProviderEntry("openai", "openai-main", "", "openai/gpt-5.2", true)
	if got["baseUrl"] != "https://api.openai.com/v1" {
		t.Fatalf("baseUrl=%#v, want https://api.openai.com/v1", got["baseUrl"])
	}
	models, ok := got["models"].([]interface{})
	if !ok || len(models) != 1 {
		t.Fatalf("models=%#v, want single model definition", got["models"])
	}
	model0, _ := models[0].(map[string]interface{})
	if model0["id"] != "gpt-5.2" || model0["name"] != "gpt-5.2" {
		t.Fatalf("model=%#v, want id/name gpt-5.2", model0)
	}
	apiKey, ok := got["apiKey"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected apiKey object, got %#v", got["apiKey"])
	}
	if apiKey["source"] != "file" {
		t.Fatalf("apiKey.source=%#v, want file", apiKey["source"])
	}
	if apiKey["provider"] != CarrierFileSecretProviderAlias {
		t.Fatalf("apiKey.provider=%#v, want %q", apiKey["provider"], CarrierFileSecretProviderAlias)
	}
	if apiKey["id"] != "/providers/openai-main/apiKey" {
		t.Fatalf("apiKey.id=%#v, want /providers/openai-main/apiKey", apiKey["id"])
	}
}

func TestBuildProviderEntryCodexSetsOAuth(t *testing.T) {
	got := BuildProviderEntry("openai-codex", "openai", "", "openai/gpt-5.3-codex", false)
	if got["auth"] != "oauth" {
		t.Fatalf("auth=%#v, want oauth", got["auth"])
	}
	if _, hasAPIKey := got["apiKey"]; hasAPIKey {
		t.Fatalf("unexpected apiKey in codex provider entry: %#v", got["apiKey"])
	}
}

func TestBuildProviderEntryUsesExplicitBaseURLAndDefaultModel(t *testing.T) {
	got := BuildProviderEntry("custom-provider", "custom-provider", "https://example.invalid/v1", "", false)
	if got["baseUrl"] != "https://example.invalid/v1" {
		t.Fatalf("baseUrl=%#v, want https://example.invalid/v1", got["baseUrl"])
	}
	if _, hasAPIKey := got["apiKey"]; hasAPIKey {
		t.Fatalf("unexpected apiKey in provider entry: %#v", got["apiKey"])
	}
	models, ok := got["models"].([]interface{})
	if !ok || len(models) != 1 {
		t.Fatalf("models=%#v, want single model definition", got["models"])
	}
	model0, _ := models[0].(map[string]interface{})
	if model0["id"] != "default" || model0["name"] != "default" {
		t.Fatalf("model=%#v, want id/name default", model0)
	}
}

func TestBuildProviderEntryFallsBackBaseURLFromProviderKey(t *testing.T) {
	got := BuildProviderEntry("custom-provider", "anthropic", "", "claude-3-5-haiku", false)
	if got["baseUrl"] != "https://api.anthropic.com/v1" {
		t.Fatalf("baseUrl=%#v, want https://api.anthropic.com/v1", got["baseUrl"])
	}
}

func TestBuildProviderEntryFallsBackBaseURLToOpenAI(t *testing.T) {
	got := BuildProviderEntry("custom-provider", "custom-provider", "", "model-x", false)
	if got["baseUrl"] != "https://api.openai.com/v1" {
		t.Fatalf("baseUrl=%#v, want https://api.openai.com/v1", got["baseUrl"])
	}
}

func TestBuildManagedConfigPayloadHandlesPendingAndAllowFrom(t *testing.T) {
	params := ManagedPayloadParams{
		ChannelID:           "telegram",
		ChannelToken:        "token-123",
		ChannelSetupPending: true,
		AllowFrom:           []string{"10001", "10002"},
		ProviderID:          "openai-codex",
		ProviderKey:         "openai",
		IncludeAPIKeyRef:    true,
		ModelID:             "gpt-5",
		WorkspacePath:       " /tmp/work ",
	}

	payload := BuildManagedConfigPayload(params)

	agents := payload["agents"].(map[string]interface{})
	defaults := agents["defaults"].(map[string]interface{})
	if defaults["workspace"] != "/tmp/work" {
		t.Fatalf("workspace=%#v, want /tmp/work", defaults["workspace"])
	}
	model := defaults["model"].(map[string]interface{})
	if model["primary"] != "gpt-5" {
		t.Fatalf("model.primary=%#v, want gpt-5", model["primary"])
	}
	if _, exists := defaults["maxTokens"]; exists {
		t.Fatalf("did not expect deprecated agents.defaults.maxTokens")
	}
	if _, exists := defaults["maxToolIterations"]; exists {
		t.Fatalf("did not expect deprecated agents.defaults.maxToolIterations")
	}
	if _, exists := defaults["restrictToWorkspace"]; exists {
		t.Fatalf("did not expect deprecated agents.defaults.restrictToWorkspace")
	}
	if _, exists := defaults["temperature"]; exists {
		t.Fatalf("did not expect deprecated agents.defaults.temperature")
	}

	models := payload["models"].(map[string]interface{})
	providers := models["providers"].(map[string]interface{})
	provider := providers["openai"].(map[string]interface{})
	if provider["auth"] != "oauth" {
		t.Fatalf("provider.auth=%#v, want oauth", provider["auth"])
	}
	if provider["baseUrl"] != "https://api.openai.com/v1" {
		t.Fatalf("provider.baseUrl=%#v, want https://api.openai.com/v1", provider["baseUrl"])
	}
	providerModels, ok := provider["models"].([]interface{})
	if !ok || len(providerModels) != 1 {
		t.Fatalf("provider.models=%#v, want single model definition", provider["models"])
	}

	channels := payload["channels"].(map[string]interface{})
	channel := channels["telegram"].(map[string]interface{})
	if channel["enabled"] != false {
		t.Fatalf("channel.enabled=%#v, want false", channel["enabled"])
	}
	if channel["setup_pending"] != true {
		t.Fatalf("channel.setup_pending=%#v, want true", channel["setup_pending"])
	}
	if _, hasToken := channel["botToken"]; hasToken {
		t.Fatalf("did not expect botToken when setup is pending")
	}
	allowFrom := channel["allowFrom"].([]string)
	if len(allowFrom) != 2 || allowFrom[0] != "10001" || allowFrom[1] != "10002" {
		t.Fatalf("allowFrom=%#v, want [10001 10002]", allowFrom)
	}
}

func TestBuildManagedConfigPayloadWritesChannelTokenWhenReady(t *testing.T) {
	params := ManagedPayloadParams{
		ChannelID:           "discord",
		ChannelToken:        "discord-token",
		ChannelSetupPending: false,
		ProviderID:          "openai",
		ProviderKey:         "provider/key",
		IncludeAPIKeyRef:    true,
		ModelID:             "gpt-4.1",
		WorkspacePath:       ".",
	}

	payload := BuildManagedConfigPayload(params)
	channels := payload["channels"].(map[string]interface{})
	channel := channels["discord"].(map[string]interface{})
	if channel["token"] != "discord-token" {
		t.Fatalf("channel.token=%#v, want discord-token", channel["token"])
	}
	if _, hasPending := channel["setup_pending"]; hasPending {
		t.Fatalf("did not expect setup_pending when channel setup is ready")
	}

	models := payload["models"].(map[string]interface{})
	providers := models["providers"].(map[string]interface{})
	provider := providers["provider/key"].(map[string]interface{})
	apiKey := provider["apiKey"].(map[string]interface{})
	if apiKey["id"] != "/providers/provider~1key/apiKey" {
		t.Fatalf("apiKey.id=%#v, want /providers/provider~1key/apiKey", apiKey["id"])
	}
}

func TestBuildManagedConfigPayloadOmitsChannelsInWebUIOnlyMode(t *testing.T) {
	params := ManagedPayloadParams{
		ChannelID:           "",
		ChannelToken:        "",
		ChannelSetupPending: false,
		ProviderID:          "openai",
		ProviderKey:         "openai",
		IncludeAPIKeyRef:    true,
		ModelID:             "openai/gpt-5.2",
		WorkspacePath:       ".",
	}

	payload := BuildManagedConfigPayload(params)
	channels := payload["channels"].(map[string]interface{})
	if len(channels) != 0 {
		t.Fatalf("channels=%#v, want empty map in webui-only mode", channels)
	}
}
