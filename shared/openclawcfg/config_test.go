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
	got := BuildProviderEntry("openai", "openai-main", true)
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
	got := BuildProviderEntry("openai-codex", "openai", false)
	if got["auth"] != "oauth" {
		t.Fatalf("auth=%#v, want oauth", got["auth"])
	}
	if _, hasAPIKey := got["apiKey"]; hasAPIKey {
		t.Fatalf("unexpected apiKey in codex provider entry: %#v", got["apiKey"])
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

	models := payload["models"].(map[string]interface{})
	providers := models["providers"].(map[string]interface{})
	provider := providers["openai"].(map[string]interface{})
	if provider["auth"] != "oauth" {
		t.Fatalf("provider.auth=%#v, want oauth", provider["auth"])
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

func TestBuildManagedConfigPayloadWebUIOnlyEmptyChannels(t *testing.T) {
	params := ManagedPayloadParams{
		ChannelID:        "",
		ProviderID:       "openai",
		ProviderKey:      "openai",
		IncludeAPIKeyRef: false,
		ModelID:          "gpt-4.1",
		WorkspacePath:    "/tmp/work",
	}

	payload := BuildManagedConfigPayload(params)
	channels := payload["channels"].(map[string]interface{})
	if len(channels) != 0 {
		t.Fatalf("expected empty channels map for webui-only mode, got %#v", channels)
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
