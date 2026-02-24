package gateway

import (
	"path/filepath"
	"strings"
	"testing"
)

func prepareCredentialStore(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "credentials.json")
	t.Setenv("CARRIER_CREDENTIAL_STORE", path)
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	return path
}

func TestHandleProviderAuthInput_APIKey_Valid(t *testing.T) {
	prepareCredentialStore(t)

	p := GetLLMProvider("anthropic")
	if p == nil {
		t.Fatal("anthropic provider not found")
	}
	result, err := HandleProviderAuthInput(p, "sk-test-key-12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Done {
		t.Error("result should be done")
	}
	if result.EnvVar != "ANTHROPIC_API_KEY" {
		t.Errorf("EnvVar: got %q, want ANTHROPIC_API_KEY", result.EnvVar)
	}
	if result.Value != "sk-test-key-12345" {
		t.Errorf("Value: got %q", result.Value)
	}
	if !strings.Contains(result.Instructions, "Credential saved") {
		t.Errorf("expected save message, got %q", result.Instructions)
	}
}

func TestHandleProviderAuthInput_APIKey_Empty(t *testing.T) {
	prepareCredentialStore(t)

	p := GetLLMProvider("openai")
	if p == nil {
		t.Fatal("openai provider not found")
	}
	_, err := HandleProviderAuthInput(p, "")
	if err == nil {
		t.Error("expected error for empty API key")
	}
}

func TestHandleProviderAuthInput_APIKey_Whitespace(t *testing.T) {
	prepareCredentialStore(t)

	p := GetLLMProvider("anthropic")
	_, err := HandleProviderAuthInput(p, "   ")
	if err == nil {
		t.Error("expected error for whitespace-only API key")
	}
}

func TestHandleProviderAuthInput_None_AutoComplete(t *testing.T) {
	p := GetLLMProvider("vllm")
	if p == nil {
		t.Fatal("vllm provider not found")
	}
	result, err := HandleProviderAuthInput(p, "anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Done {
		t.Error("none auth mode should auto-complete")
	}
	if result.EnvVar != "" {
		t.Errorf("none auth should not set env var, got %q", result.EnvVar)
	}
}

func TestHandleProviderAuthInput_OAuthDeviceCode_Token(t *testing.T) {
	prepareCredentialStore(t)

	p := GetLLMProvider("openai-codex")
	if p == nil {
		t.Fatal("openai-codex provider not found")
	}
	result, err := HandleProviderAuthInput(p, "codex-token-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Done {
		t.Error("result should be done")
	}
	if result.EnvVar != "OPENAI_CODEX_TOKEN" {
		t.Errorf("EnvVar: got %q, want OPENAI_CODEX_TOKEN", result.EnvVar)
	}
	if result.Value != "codex-token-abc" {
		t.Errorf("Value: got %q", result.Value)
	}
}

func TestHandleProviderAuthInput_OAuthDeviceCode_ConfirmRejected(t *testing.T) {
	prepareCredentialStore(t)

	p := GetLLMProvider("openai-codex")
	if p == nil {
		t.Fatal("openai-codex provider not found")
	}
	_, err := HandleProviderAuthInput(p, "confirm")
	if err == nil {
		t.Fatal("expected error for confirm input in OAuth device-code flow")
	}
	if !strings.Contains(err.Error(), "OPENAI_CODEX_TOKEN") {
		t.Fatalf("expected OPENAI_CODEX_TOKEN guidance, got %v", err)
	}
}

func TestHandleProviderAuthInput_OAuthDeviceCode_Reuse(t *testing.T) {
	prepareCredentialStore(t)

	p := GetLLMProvider("openai-codex")
	if p == nil {
		t.Fatal("openai-codex provider not found")
	}
	if _, err := HandleProviderAuthInput(p, "first-token"); err != nil {
		t.Fatalf("seed token failed: %v", err)
	}
	result, err := HandleProviderAuthInput(p, "reuse")
	if err != nil {
		t.Fatalf("reuse failed: %v", err)
	}
	if result.Value != "first-token" {
		t.Fatalf("expected reused token first-token, got %q", result.Value)
	}
	if !strings.Contains(result.Instructions, "Reused saved credential") {
		t.Fatalf("expected reuse message, got %q", result.Instructions)
	}
}

func TestHandleProviderAuthInput_OAuthDeviceCode_ReuseWithoutSavedValue(t *testing.T) {
	prepareCredentialStore(t)

	p := GetLLMProvider("openai-codex")
	if p == nil {
		t.Fatal("openai-codex provider not found")
	}
	_, err := HandleProviderAuthInput(p, "reuse")
	if err == nil {
		t.Fatal("expected error when no saved credential exists")
	}
	if !strings.Contains(err.Error(), "no saved credential") {
		t.Fatalf("expected no-saved-credential error, got %v", err)
	}
}

func TestHandleProviderAuthInput_OAuthPlugin_Confirm(t *testing.T) {
	p := &LLMProvider{
		ID:       "plugin-provider",
		Name:     "Plugin Provider",
		AuthMode: AuthModeOAuthPlugin,
	}
	result, err := HandleProviderAuthInput(p, "confirm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Done {
		t.Error("result should be done after confirm")
	}
}

func TestHandleProviderAuthInput_GcloudADC_Confirm(t *testing.T) {
	p := &LLMProvider{
		ID:       "vertex-provider",
		Name:     "Vertex Provider",
		AuthMode: AuthModeGcloudADC,
	}
	result, err := HandleProviderAuthInput(p, "done")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Done {
		t.Error("result should be done after done")
	}
}

func TestBuildProviderAuthPrompt_APIKey(t *testing.T) {
	p := GetLLMProvider("anthropic")
	prompt := BuildProviderAuthPrompt(p)
	if !strings.Contains(prompt, "ANTHROPIC_API_KEY") {
		t.Errorf("expected env var in prompt, got: %q", prompt)
	}
	if !strings.Contains(prompt, "Anthropic") {
		t.Errorf("expected provider name in prompt, got: %q", prompt)
	}
	if !strings.Contains(prompt, "/onboard reuse") {
		t.Errorf("expected reuse hint in prompt, got: %q", prompt)
	}
}

func TestBuildProviderAuthPrompt_OAuthDeviceCode(t *testing.T) {
	p := GetLLMProvider("openai-codex")
	prompt := BuildProviderAuthPrompt(p)
	if !strings.Contains(prompt, "OPENAI_CODEX_TOKEN") {
		t.Errorf("expected token env var in prompt, got: %q", prompt)
	}
	if !strings.Contains(prompt, "/onboard reuse") {
		t.Errorf("expected reuse hint in prompt, got: %q", prompt)
	}
	if strings.Contains(prompt, "openclaw models auth login") {
		t.Errorf("legacy login command should not appear, got: %q", prompt)
	}
}

func TestBuildProviderAuthPrompt_OAuthPlugin(t *testing.T) {
	p := &LLMProvider{
		ID:       "plugin-provider",
		Name:     "Plugin Provider",
		AuthMode: AuthModeOAuthPlugin,
	}
	prompt := BuildProviderAuthPrompt(p)
	if !strings.Contains(prompt, "/onboard confirm") {
		t.Errorf("expected confirm guidance in prompt, got: %q", prompt)
	}
	if strings.Contains(prompt, "openclaw models auth login") {
		t.Errorf("legacy login command should not appear, got: %q", prompt)
	}
}

func TestBuildProviderAuthPrompt_GcloudADC(t *testing.T) {
	p := &LLMProvider{
		ID:       "vertex-provider",
		Name:     "Vertex Provider",
		AuthMode: AuthModeGcloudADC,
	}
	prompt := BuildProviderAuthPrompt(p)
	if !strings.Contains(prompt, "gcloud") {
		t.Errorf("expected gcloud in prompt, got: %q", prompt)
	}
}

func TestBuildProviderAuthPrompt_None(t *testing.T) {
	p := GetLLMProvider("vllm")
	prompt := BuildProviderAuthPrompt(p)
	if !strings.Contains(strings.ToLower(prompt), "no auth") {
		t.Errorf("expected 'no auth' in prompt, got: %q", prompt)
	}
}

func TestProviderEnvVarsToSet_APIKey(t *testing.T) {
	p := GetLLMProvider("anthropic")
	m := ProviderEnvVarsToSet(p, "my-key")
	if m["ANTHROPIC_API_KEY"] != "my-key" {
		t.Errorf("expected ANTHROPIC_API_KEY=my-key, got %v", m)
	}
}

func TestProviderEnvVarsToSet_OAuthDeviceCode(t *testing.T) {
	p := GetLLMProvider("openai-codex")
	m := ProviderEnvVarsToSet(p, "codex-token")
	if m["OPENAI_CODEX_TOKEN"] != "codex-token" {
		t.Errorf("expected OPENAI_CODEX_TOKEN=codex-token, got %v", m)
	}
	if m["OPENAI_API_KEY"] != "codex-token" {
		t.Errorf("expected OPENAI_API_KEY=codex-token alias, got %v", m)
	}
}

func TestProviderEnvVarsToSet_None(t *testing.T) {
	p := GetLLMProvider("vllm")
	m := ProviderEnvVarsToSet(p, "")
	if len(m) != 0 {
		t.Errorf("expected empty map for none auth, got %v", m)
	}
}

func TestProviderEnvVarsToSet_EmptyValue(t *testing.T) {
	p := GetLLMProvider("openai")
	m := ProviderEnvVarsToSet(p, "")
	if len(m) != 0 {
		t.Errorf("expected empty map when value is empty, got %v", m)
	}
}
