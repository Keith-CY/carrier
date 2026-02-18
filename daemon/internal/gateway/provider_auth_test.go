package gateway

import (
	"strings"
	"testing"
)

func TestHandleProviderAuthInput_APIKey_Valid(t *testing.T) {
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
}

func TestHandleProviderAuthInput_APIKey_Empty(t *testing.T) {
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
	p := GetLLMProvider("mistral")
	_, err := HandleProviderAuthInput(p, "   ")
	if err == nil {
		t.Error("expected error for whitespace-only API key")
	}
}

func TestHandleProviderAuthInput_None_AutoComplete(t *testing.T) {
	p := GetLLMProvider("ollama")
	if p == nil {
		t.Fatal("ollama provider not found")
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

func TestHandleProviderAuthInput_OAuthDeviceCode_Confirm(t *testing.T) {
	p := GetLLMProvider("openai-codex")
	if p == nil {
		t.Fatal("openai-codex provider not found")
	}
	for _, input := range []string{"confirm", "done", "yes", "y"} {
		result, err := HandleProviderAuthInput(p, input)
		if err != nil {
			t.Errorf("input %q: unexpected error: %v", input, err)
			continue
		}
		if !result.Done {
			t.Errorf("input %q: result should be done", input)
		}
	}
}

func TestHandleProviderAuthInput_OAuthDeviceCode_Reject(t *testing.T) {
	p := GetLLMProvider("qwen-portal")
	_, err := HandleProviderAuthInput(p, "not-a-valid-confirm")
	if err == nil {
		t.Error("expected error for non-confirm input in OAuth device code flow")
	}
}

func TestHandleProviderAuthInput_OAuthPlugin_Confirm(t *testing.T) {
	p := GetLLMProvider("google-antigravity")
	result, err := HandleProviderAuthInput(p, "confirm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Done {
		t.Error("result should be done after confirm")
	}
}

func TestHandleProviderAuthInput_GcloudADC_Confirm(t *testing.T) {
	p := GetLLMProvider("google-vertex")
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
}

func TestBuildProviderAuthPrompt_OAuthDeviceCode(t *testing.T) {
	p := GetLLMProvider("openai-codex")
	prompt := BuildProviderAuthPrompt(p)
	if !strings.Contains(prompt, "openclaw models auth login") {
		t.Errorf("expected login command in prompt, got: %q", prompt)
	}
	if !strings.Contains(prompt, "openai-codex") {
		t.Errorf("expected provider ID in prompt, got: %q", prompt)
	}
}

func TestBuildProviderAuthPrompt_OAuthPlugin(t *testing.T) {
	p := GetLLMProvider("google-gemini-cli")
	prompt := BuildProviderAuthPrompt(p)
	if !strings.Contains(prompt, "openclaw models auth login") {
		t.Errorf("expected login command in prompt, got: %q", prompt)
	}
}

func TestBuildProviderAuthPrompt_GcloudADC(t *testing.T) {
	p := GetLLMProvider("google-vertex")
	prompt := BuildProviderAuthPrompt(p)
	if !strings.Contains(prompt, "gcloud") {
		t.Errorf("expected gcloud in prompt, got: %q", prompt)
	}
}

func TestBuildProviderAuthPrompt_None(t *testing.T) {
	p := GetLLMProvider("ollama")
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

func TestProviderEnvVarsToSet_None(t *testing.T) {
	p := GetLLMProvider("ollama")
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
