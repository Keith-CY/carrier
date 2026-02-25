package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePicoclawChannel(t *testing.T) {
	if _, ok := parseManagedChannel("picoclaw", "telegram"); !ok {
		t.Fatal("expected telegram channel to be supported")
	}
	if _, ok := parseManagedChannel("picoclaw", "discord"); ok {
		t.Fatal("did not expect discord channel to be supported in managed picoclaw flow")
	}
}

func TestOnboardSelectProvider_ReuseCarrierDefault(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	writeGatewayDefaultProviderConfig(t, "openai-codex", "openai-codex/gpt-5.3-codex", "OPENAI_CODEX_TOKEN")

	if _, err := saveProviderCredential("openai-codex", "codex-token-1"); err != nil {
		t.Fatalf("saveProviderCredential: %v", err)
	}

	s := NewOnboardStore()
	key := "telegram:reuse"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardAgentSelected
		sess.SelectedAgent = "picoclaw"
	})

	resp := onboardSelectProvider("req-1", key, "reuse", s)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got: %+v", resp)
	}
	sess := s.get(key)
	if sess.Step != OnboardAuthConfigured {
		t.Fatalf("expected auth_configured, got %q", sess.Step)
	}
	if sess.SelectedProvider != "openai-codex" {
		t.Fatalf("expected selected provider openai-codex, got %q", sess.SelectedProvider)
	}
	if got := sess.EnvVars["OPENAI_CODEX_TOKEN"]; got != "codex-token-1" {
		t.Fatalf("expected OPENAI_CODEX_TOKEN to be reused, got %q", got)
	}
	if !strings.Contains(resp.Message, "Reused Carrier default provider") {
		t.Fatalf("expected reuse confirmation in message, got %q", resp.Message)
	}
}

func TestPreparePicoclawManagedOnboard_WritesConfigAndRecord(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")

	sess := &OnboardSession{
		SelectedAgent:    "picoclaw",
		SelectedChannel:  "telegram",
		ChannelToken:     "telegram-token-abc",
		SelectedProvider: "openai-codex",
		EnvVars: map[string]string{
			"OPENAI_CODEX_TOKEN": "codex-token-value",
		},
	}

	result, err := prepareManagedOnboard("picoclaw", sess, "telegram:418258935")
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

	modelList, ok := cfg["model_list"].([]interface{})
	if !ok || len(modelList) != 1 {
		t.Fatalf("expected 1 model in model_list, got %#v", cfg["model_list"])
	}
	model, ok := modelList[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected model entry map, got %#v", modelList[0])
	}
	if model["model"] != "openai/gpt-5.3-codex" {
		t.Fatalf("unexpected model id: %v", model["model"])
	}
	if model["auth_method"] != "oauth" {
		t.Fatalf("unexpected auth_method: %v", model["auth_method"])
	}
	if _, hasAPIKey := model["api_key"]; hasAPIKey {
		t.Fatalf("did not expect oauth model api_key to be persisted, got %#v", model["api_key"])
	}
	agents, ok := cfg["agents"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agents object, got %#v", cfg["agents"])
	}
	defaults, ok := agents["defaults"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected agents.defaults object, got %#v", agents["defaults"])
	}
	if defaults["provider"] != "openai" {
		t.Fatalf("unexpected default provider: %v", defaults["provider"])
	}
	if defaults["model"] != "gpt-5.3-codex" {
		t.Fatalf("unexpected default model: %v", defaults["model"])
	}

	providers, ok := cfg["providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected providers object, got %#v", cfg["providers"])
	}
	openaiProvider, ok := providers["openai"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected providers.openai object, got %#v", providers["openai"])
	}
	if openaiProvider["auth_method"] != "oauth" {
		t.Fatalf("unexpected providers.openai.auth_method: %v", openaiProvider["auth_method"])
	}

	authPath := filepath.Join(tmp, ".picoclaw", "auth.json")
	authRaw, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read auth store: %v", err)
	}
	var authStore map[string]interface{}
	if err := json.Unmarshal(authRaw, &authStore); err != nil {
		t.Fatalf("parse auth store json: %v", err)
	}
	creds, ok := authStore["credentials"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected credentials map, got %#v", authStore["credentials"])
	}
	openaiCred, ok := creds["openai"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected openai credential, got %#v", creds["openai"])
	}
	if openaiCred["access_token"] != "codex-token-value" {
		t.Fatalf("unexpected openai access_token: %v", openaiCred["access_token"])
	}
	if openaiCred["auth_method"] != "oauth" {
		t.Fatalf("unexpected openai auth_method: %v", openaiCred["auth_method"])
	}

	recordRaw, err := os.ReadFile(result.RecordPath)
	if err != nil {
		t.Fatalf("read record: %v", err)
	}
	var record map[string]interface{}
	if err := json.Unmarshal(recordRaw, &record); err != nil {
		t.Fatalf("parse record json: %v", err)
	}
	if record["workspace_path"] != result.WorkspacePath {
		t.Fatalf("record workspace_path mismatch: got %v want %s", record["workspace_path"], result.WorkspacePath)
	}
	recordText := string(recordRaw)
	if strings.Contains(recordText, "codex-token-value") || strings.Contains(recordText, "telegram-token-abc") {
		t.Fatalf("managed record should not contain secret token values: %s", recordText)
	}
}

func TestExtractPairCode(t *testing.T) {
	lines := []string{
		"booting...",
		"PAIR_CODE: pair-abcdef0123456789abcdef0123456789",
	}
	if got := extractPairCode(lines); got != "pair-abcdef0123456789abcdef0123456789" {
		t.Fatalf("extractPairCode mismatch: %q", got)
	}
}

func TestExtractPairedTelegramChatID(t *testing.T) {
	lines := []string{
		"booting...",
		"✅ paired telegram:418258935",
	}
	if got := extractPairedTelegramChatID(lines); got != "418258935" {
		t.Fatalf("extractPairedTelegramChatID mismatch: %q", got)
	}
}

func TestActorChatID(t *testing.T) {
	if got := actorChatID("telegram:418258935"); got != "418258935" {
		t.Fatalf("actorChatID numeric mismatch: %q", got)
	}
	if got := actorChatID("webui:add"); got != "" {
		t.Fatalf("actorChatID should ignore non-numeric chat id, got %q", got)
	}
}

func TestExtractOpenAIAccountID_FromClaims(t *testing.T) {
	tokenWithTopLevel := "aaaa.eyJjaGF0Z3B0X2FjY291bnRfaWQiOiJhY2N0LTEifQ.bbbb"
	if got := extractOpenAIAccountID(tokenWithTopLevel); got != "acct-1" {
		t.Fatalf("extractOpenAIAccountID top-level mismatch: %q", got)
	}

	tokenWithNamespaced := "aaaa.eyJodHRwczovL2FwaS5vcGVuYWkuY29tL2F1dGguY2hhdGdwdF9hY2NvdW50X2lkIjoiYWNjdC0yIn0.bbbb"
	if got := extractOpenAIAccountID(tokenWithNamespaced); got != "acct-2" {
		t.Fatalf("extractOpenAIAccountID namespaced mismatch: %q", got)
	}
}
