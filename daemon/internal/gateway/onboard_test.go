package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnboardStore_StartAndGet(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:123"

	if s.get(key) != nil {
		t.Error("expected nil before start")
	}

	sess := s.start(key)
	if sess == nil {
		t.Fatal("start returned nil")
	}
	if sess.Step != OnboardIdle {
		t.Errorf("expected idle, got %q", sess.Step)
	}
	if sess.EnvVars == nil {
		t.Error("EnvVars should be initialized")
	}
}

func TestOnboardStore_Update(t *testing.T) {
	s := NewOnboardStore()
	key := "discord:ch"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardAgentSelected
		sess.SelectedAgent = "openclaw"
	})
	sess := s.get(key)
	if sess.Step != OnboardAgentSelected {
		t.Errorf("expected agent_selected, got %q", sess.Step)
	}
	if sess.SelectedAgent != "openclaw" {
		t.Errorf("selectedAgent: %q", sess.SelectedAgent)
	}
}

func TestOnboardStore_Clear(t *testing.T) {
	s := NewOnboardStore()
	key := "feishu:chat"
	s.start(key)
	s.clear(key)
	if s.get(key) != nil {
		t.Error("expected nil after clear")
	}
}

func TestOnboardStore_HasActive(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:999"
	if s.hasActive(key) {
		t.Error("no session, should not be active")
	}
	s.start(key)
	if s.hasActive(key) {
		t.Error("idle step should not be active")
	}
	s.update(key, func(sess *OnboardSession) { sess.Step = OnboardAgentSelected })
	if !s.hasActive(key) {
		t.Error("agent_selected step should be active")
	}
	s.update(key, func(sess *OnboardSession) { sess.Step = OnboardDone })
	if s.hasActive(key) {
		t.Error("done step should not be active")
	}
}

func TestOnboardCancel_NoSession(t *testing.T) {
	s := NewOnboardStore()
	resp := onboardCancel("req-1", "telegram:1", s)
	if resp.Result != "ok" {
		t.Errorf("cancel with no session should be ok: %+v", resp)
	}
	if !strings.Contains(resp.Message, "No active") {
		t.Errorf("message: %q", resp.Message)
	}
}

func TestOnboardCancel_ActiveSession(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:1"
	s.start(key)
	s.update(key, func(sess *OnboardSession) { sess.Step = OnboardAgentSelected })

	resp := onboardCancel("req-1", key, s)
	if resp.Result != "ok" {
		t.Errorf("cancel should be ok: %+v", resp)
	}
	if !strings.Contains(resp.Message, "cancelled") {
		t.Errorf("message: %q", resp.Message)
	}
	if s.get(key) != nil {
		t.Error("session should be cleared after cancel")
	}
}

func TestOnboardEnvInput_Done(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:1"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardAgentSelected
		sess.SelectedAgent = "openclaw"
	})

	resp := onboardEnvInput("req-1", key, "done", s)
	if resp.Result != "ok" {
		t.Errorf("done should be ok: %+v", resp)
	}
	sess := s.get(key)
	if sess.Step != OnboardEnvConfigured {
		t.Errorf("expected env_configured, got %q", sess.Step)
	}
}

func TestOnboardEnvInput_SetVar(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:1"
	s.start(key)
	s.update(key, func(sess *OnboardSession) { sess.Step = OnboardAgentSelected })

	resp := onboardEnvInput("req-1", key, "FOO=bar", s)
	if resp.Result != "ok" {
		t.Errorf("env set should be ok: %+v", resp)
	}
	sess := s.get(key)
	if sess.EnvVars["FOO"] != "bar" {
		t.Errorf("FOO should be 'bar', got %q", sess.EnvVars["FOO"])
	}
}

func TestOnboardEnvInput_InvalidFormat(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:1"
	s.start(key)
	s.update(key, func(sess *OnboardSession) { sess.Step = OnboardAuthConfigured })

	resp := onboardEnvInput("req-1", key, "notakvpair", s)
	if resp.Result != "error" {
		t.Errorf("invalid env should be error: %+v", resp)
	}
}

// --- Provider selection step tests ---

func TestOnboardSelectProvider_ValidProvider(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:2"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardAgentSelected
		sess.SelectedAgent = "openclaw"
	})

	resp := onboardSelectProvider("req-1", key, "anthropic", s)
	if resp.Result != "ok" {
		t.Errorf("expected ok, got: %+v", resp)
	}
	sess := s.get(key)
	if sess.SelectedProvider != "anthropic" {
		t.Errorf("expected anthropic, got %q", sess.SelectedProvider)
	}
	if sess.Step != OnboardProviderSelected {
		t.Errorf("expected provider_selected, got %q", sess.Step)
	}
	if !strings.Contains(resp.Message, "API key") {
		t.Errorf("expected API key prompt, got: %q", resp.Message)
	}
}

func TestOnboardSelectProvider_LocalProvider_AutoAdvance(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:3"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardAgentSelected
		sess.SelectedAgent = "openclaw"
	})

	resp := onboardSelectProvider("req-1", key, "vllm", s)
	if resp.Result != "ok" {
		t.Errorf("expected ok, got: %+v", resp)
	}
	sess := s.get(key)
	if sess.Step != OnboardAuthConfigured {
		t.Errorf("vllm should auto-advance to auth_configured, got %q", sess.Step)
	}
	if !strings.Contains(strings.ToLower(resp.Message), "no auth") {
		t.Errorf("expected 'no auth' message, got: %q", resp.Message)
	}
}

func TestOnboardSelectProvider_Skip(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:4"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardAgentSelected
		sess.SelectedAgent = "openclaw"
	})

	resp := onboardSelectProvider("req-1", key, "skip", s)
	if resp.Result != "ok" {
		t.Errorf("expected ok, got: %+v", resp)
	}
	sess := s.get(key)
	if sess.Step != OnboardAuthConfigured {
		t.Errorf("skip should advance to auth_configured, got %q", sess.Step)
	}
}

func TestOnboardSelectProvider_Done(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:5"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardAgentSelected
		sess.SelectedAgent = "openclaw"
	})

	resp := onboardSelectProvider("req-1", key, "done", s)
	if resp.Result != "ok" {
		t.Errorf("expected ok, got: %+v", resp)
	}
	sess := s.get(key)
	if sess.Step != OnboardAuthConfigured {
		t.Errorf("done should advance to auth_configured, got %q", sess.Step)
	}
}

func TestOnboardSelectProvider_Invalid(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:6"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardAgentSelected
		sess.SelectedAgent = "openclaw"
	})

	resp := onboardSelectProvider("req-1", key, "nonexistent-provider-xyz", s)
	if resp.Result != "error" {
		t.Errorf("expected error for unknown provider, got: %+v", resp)
	}
	if resp.ErrorCode != "E_PROVIDER_NOT_FOUND" {
		t.Errorf("expected E_PROVIDER_NOT_FOUND, got %q", resp.ErrorCode)
	}
}

// --- Auth input step tests ---

func TestOnboardHandleAuth_APIKey_Valid(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:7"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardProviderSelected
		sess.SelectedAgent = "openclaw"
		sess.SelectedProvider = "anthropic"
	})

	resp := onboardHandleAuth("req-1", key, "sk-my-secret-key", s)
	if resp.Result != "ok" {
		t.Errorf("expected ok, got: %+v", resp)
	}
	sess := s.get(key)
	if sess.Step != OnboardAuthConfigured {
		t.Errorf("expected auth_configured, got %q", sess.Step)
	}
	if sess.EnvVars["ANTHROPIC_API_KEY"] != "sk-my-secret-key" {
		t.Errorf("expected ANTHROPIC_API_KEY set, got %v", sess.EnvVars)
	}
}

func TestOnboardHandleAuth_APIKey_Empty(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:8"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardProviderSelected
		sess.SelectedAgent = "openclaw"
		sess.SelectedProvider = "openai"
	})

	resp := onboardHandleAuth("req-1", key, "", s)
	if resp.Result != "error" {
		t.Errorf("expected error for empty API key, got: %+v", resp)
	}
}

func TestOnboardHandleAuth_Skip(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:9"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardProviderSelected
		sess.SelectedAgent = "openclaw"
		sess.SelectedProvider = "anthropic"
	})

	resp := onboardHandleAuth("req-1", key, "skip", s)
	if resp.Result != "ok" {
		t.Errorf("expected ok after skip, got: %+v", resp)
	}
	sess := s.get(key)
	if sess.Step != OnboardAuthConfigured {
		t.Errorf("skip should advance to auth_configured, got %q", sess.Step)
	}
}

func TestOnboardHandleAuth_OAuthDeviceCode_Token(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")

	s := NewOnboardStore()
	key := "telegram:10"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardProviderSelected
		sess.SelectedAgent = "openclaw"
		sess.SelectedProvider = "openai-codex"
	})

	resp := onboardHandleAuth("req-1", key, "codex-token-1", s)
	if resp.Result != "ok" {
		t.Errorf("expected ok after token input, got: %+v", resp)
	}
	sess := s.get(key)
	if sess.Step != OnboardAuthConfigured {
		t.Errorf("expected auth_configured, got %q", sess.Step)
	}
	if sess.EnvVars["OPENAI_CODEX_TOKEN"] != "codex-token-1" {
		t.Fatalf("expected OPENAI_CODEX_TOKEN to be set, got %v", sess.EnvVars)
	}
}

func TestOnboardHandleAuth_OAuthDeviceCode_Reuse(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")

	s := NewOnboardStore()
	key := "telegram:10-reuse"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardProviderSelected
		sess.SelectedAgent = "openclaw"
		sess.SelectedProvider = "openai-codex"
	})
	if resp := onboardHandleAuth("req-1", key, "codex-token-2", s); resp.Result != "ok" {
		t.Fatalf("seed token should succeed: %+v", resp)
	}

	key2 := "telegram:10-reuse-2"
	s.start(key2)
	s.update(key2, func(sess *OnboardSession) {
		sess.Step = OnboardProviderSelected
		sess.SelectedAgent = "openclaw"
		sess.SelectedProvider = "openai-codex"
	})
	resp := onboardHandleAuth("req-2", key2, "reuse", s)
	if resp.Result != "ok" {
		t.Fatalf("reuse should succeed: %+v", resp)
	}
	sess2 := s.get(key2)
	if sess2.EnvVars["OPENAI_CODEX_TOKEN"] != "codex-token-2" {
		t.Fatalf("expected reused OPENAI_CODEX_TOKEN, got %v", sess2.EnvVars)
	}
	if !strings.Contains(resp.Message, "Reused saved credential") {
		t.Fatalf("expected reuse message, got %q", resp.Message)
	}
}

func TestOnboardHandleAuth_OAuthDeviceCode_ConfirmRejected(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")

	s := NewOnboardStore()
	key := "telegram:10-confirm"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardProviderSelected
		sess.SelectedAgent = "openclaw"
		sess.SelectedProvider = "openai-codex"
	})

	resp := onboardHandleAuth("req-1", key, "confirm", s)
	if resp.Result != "error" {
		t.Fatalf("expected error for confirm-only input, got %+v", resp)
	}
	if resp.ErrorCode != "E_AUTH_INPUT" {
		t.Fatalf("expected E_AUTH_INPUT, got %q", resp.ErrorCode)
	}
	if !strings.Contains(resp.Message, "OPENAI_CODEX_TOKEN") {
		t.Fatalf("expected token guidance, got %q", resp.Message)
	}
}

func TestOnboardHandleAuth_OAuth_BadInput(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")

	s := NewOnboardStore()
	key := "telegram:11"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardProviderSelected
		sess.SelectedAgent = "openclaw"
		sess.SelectedProvider = "openai-codex"
	})

	resp := onboardHandleAuth("req-1", key, "", s)
	if resp.Result != "error" {
		t.Errorf("expected error for empty OAuth token input, got: %+v", resp)
	}
}

// --- Session new step EnvInput backward compat ---

func TestOnboardEnvInput_Done_WithProvider(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:12"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardAuthConfigured
		sess.SelectedAgent = "openclaw"
		sess.SelectedProvider = "anthropic"
		sess.EnvVars["ANTHROPIC_API_KEY"] = "my-key"
	})

	resp := onboardEnvInput("req-1", key, "done", s)
	if resp.Result != "ok" {
		t.Errorf("done should be ok: %+v", resp)
	}
	sess := s.get(key)
	if sess.Step != OnboardEnvConfigured {
		t.Errorf("expected env_configured, got %q", sess.Step)
	}
	if !strings.Contains(resp.Message, "anthropic") {
		t.Errorf("expected provider name in summary, got: %q", resp.Message)
	}
}

// --- State machine ordering tests ---

func TestOnboardStoreNewSteps(t *testing.T) {
	s := NewOnboardStore()
	key := "discord:ch"

	s.start(key)
	s.update(key, func(sess *OnboardSession) { sess.Step = OnboardProviderSelected })
	if !s.hasActive(key) {
		t.Error("provider_selected should be active")
	}

	s.update(key, func(sess *OnboardSession) { sess.Step = OnboardAuthConfigured })
	if !s.hasActive(key) {
		t.Error("auth_configured should be active")
	}
}

func TestApplyOnboardEnvVars(t *testing.T) {
	const key = "CARRIER_ONBOARD_ENV_TEST"
	t.Setenv(key, "old")

	if err := applyOnboardEnvVars(map[string]string{key: "new"}); err != nil {
		t.Fatalf("applyOnboardEnvVars returned error: %v", err)
	}
	if got := os.Getenv(key); got != "new" {
		t.Fatalf("env %s = %q, want %q", key, got, "new")
	}
}

func TestOnboardSelectChannel_Openclaw(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:openclaw"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardChannelSelect
		sess.SelectedAgent = "openclaw"
	})

	resp := onboardSelectChannel("req-openclaw", key, "telegram", s)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %+v", resp)
	}
	if !strings.Contains(resp.Message, "OpenClaw channel selected") {
		t.Fatalf("expected openclaw channel prompt, got %q", resp.Message)
	}
	if got := s.get(key).Step; got != OnboardChannelToken {
		t.Fatalf("expected channel token step, got %q", got)
	}
}

func TestOnboardSelectChannel_Zeroclaw(t *testing.T) {
	s := NewOnboardStore()
	key := "telegram:zeroclaw"
	s.start(key)
	s.update(key, func(sess *OnboardSession) {
		sess.Step = OnboardChannelSelect
		sess.SelectedAgent = "zeroclaw"
	})

	resp := onboardSelectChannel("req-zeroclaw", key, "telegram", s)
	if resp.Result != "ok" {
		t.Fatalf("expected ok, got %+v", resp)
	}
	if !strings.Contains(resp.Message, "ZeroClaw channel selected") {
		t.Fatalf("expected zeroclaw channel prompt, got %q", resp.Message)
	}
	if got := s.get(key).Step; got != OnboardChannelToken {
		t.Fatalf("expected channel token step, got %q", got)
	}
}
