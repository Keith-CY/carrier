package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildProviderAuthPrompt_DeviceCodeWithoutEnvVar(t *testing.T) {
	p := &LLMProvider{ID: "dc-no-env", Name: "Device No Env", AuthMode: AuthModeOAuthDeviceCode}
	prompt := BuildProviderAuthPrompt(p)
	if !strings.Contains(prompt, "/onboard confirm") {
		t.Fatalf("expected confirm guidance, got %q", prompt)
	}
	if strings.Contains(prompt, "paste `") {
		t.Fatalf("unexpected paste token guidance for empty env var, got %q", prompt)
	}
}

func TestBuildProviderAuthPrompt_DefaultMode(t *testing.T) {
	p := &LLMProvider{ID: "mystery", Name: "Mystery", AuthMode: AuthMode("mystery")}
	prompt := BuildProviderAuthPrompt(p)
	if !strings.Contains(strings.ToLower(prompt), "not recognised") {
		t.Fatalf("expected not-recognised hint, got %q", prompt)
	}
}

func TestHandleProviderAuthInput_DeviceCodeWithoutEnvVarBranches(t *testing.T) {
	p := &LLMProvider{ID: "dc-no-env", Name: "Device No Env", AuthMode: AuthModeOAuthDeviceCode}

	okResult, err := HandleProviderAuthInput(p, "confirm")
	if err != nil {
		t.Fatalf("expected confirm to succeed, got %v", err)
	}
	if okResult == nil || !okResult.Done {
		t.Fatalf("expected done result, got %#v", okResult)
	}

	_, err = HandleProviderAuthInput(p, "not-yet")
	if err == nil || !strings.Contains(err.Error(), "/onboard confirm") {
		t.Fatalf("expected confirm instruction error, got %v", err)
	}
}

func TestHandleProviderAuthInput_OAuthPluginRejectsNonConfirm(t *testing.T) {
	p := &LLMProvider{ID: "plugin", Name: "Plugin", AuthMode: AuthModeOAuthPlugin}
	_, err := HandleProviderAuthInput(p, "later")
	if err == nil || !strings.Contains(err.Error(), "/onboard confirm") {
		t.Fatalf("expected confirm instruction error, got %v", err)
	}
}

func TestHandleProviderAuthInput_DefaultModeAutoDone(t *testing.T) {
	p := &LLMProvider{ID: "custom", Name: "Custom", AuthMode: AuthMode("unknown")}
	result, err := HandleProviderAuthInput(p, "whatever")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || !result.Done {
		t.Fatalf("expected done result, got %#v", result)
	}
}

func TestHandleProviderAuthInput_MissingEnvVarForPastedCredential(t *testing.T) {
	p := &LLMProvider{ID: "broken", Name: "Broken", AuthMode: AuthModeAPIKey, EnvVar: ""}
	_, err := HandleProviderAuthInput(p, "token")
	if err == nil || !strings.Contains(err.Error(), "does not expose an env var") {
		t.Fatalf("expected missing env var error, got %v", err)
	}
}

func TestHandleProviderAuthInput_ReuseReturnsLoadErrorWhenCredentialStoreMalformed(t *testing.T) {
	tmp := t.TempDir()
	storePath := filepath.Join(tmp, "credentials.json")
	if err := os.WriteFile(storePath, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write malformed credential store: %v", err)
	}
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", storePath)

	p := GetLLMProvider("openai-codex")
	if p == nil {
		t.Fatal("openai-codex provider not found")
	}
	_, err := HandleProviderAuthInput(p, "reuse")
	if err == nil || !strings.Contains(err.Error(), "failed to load saved credential") {
		t.Fatalf("expected load error, got %v", err)
	}
}

func TestHandleProviderAuthInput_SaveFailureStillReturnsDone(t *testing.T) {
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(blocker, "credentials.json"))

	p := GetLLMProvider("anthropic")
	if p == nil {
		t.Fatal("anthropic provider not found")
	}
	result, err := HandleProviderAuthInput(p, "sk-save-failure")
	if err != nil {
		t.Fatalf("expected no hard error on save failure, got %v", err)
	}
	if result == nil || !result.Done {
		t.Fatalf("expected done result, got %#v", result)
	}
	if !strings.Contains(result.Instructions, "failed to save") {
		t.Fatalf("expected save failure hint, got %q", result.Instructions)
	}
}

func TestProviderEnvVarsToSet_NilProviderAndNoEnvVar(t *testing.T) {
	if got := ProviderEnvVarsToSet(nil, "v", ""); got != nil {
		t.Fatalf("expected nil for nil provider, got %#v", got)
	}
	p := &LLMProvider{ID: "no-env", Name: "No Env", EnvVar: ""}
	if got := ProviderEnvVarsToSet(p, "value", ""); got != nil {
		t.Fatalf("expected nil for empty provider env var, got %#v", got)
	}
}
