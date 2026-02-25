package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

func seedManagedEnvConfiguredSession(store *OnboardStore, key, agentID, channel, channelToken, providerID string, env map[string]string) {
	store.start(key)
	store.update(key, func(s *OnboardSession) {
		s.Step = OnboardEnvConfigured
		s.SelectedAgent = agentID
		s.SelectedChannel = channel
		s.ChannelToken = channelToken
		s.SelectedProvider = providerID
		if s.EnvVars == nil {
			s.EnvVars = map[string]string{}
		}
		for k, v := range env {
			s.EnvVars[k] = v
		}
	})
}

func TestOnboardSelectChannel_ErrorBranches(t *testing.T) {
	store := NewOnboardStore()

	resp := onboardSelectChannel("req-missing", "telegram:missing", "telegram", store)
	if resp.Result != "error" || resp.ErrorCode != "E_USAGE" {
		t.Fatalf("expected missing-session usage error, got %+v", resp)
	}

	nonManagedKey := "telegram:non-managed"
	store.start(nonManagedKey)
	store.update(nonManagedKey, func(s *OnboardSession) {
		s.Step = OnboardChannelSelect
		s.SelectedAgent = "worker"
	})
	resp = onboardSelectChannel("req-non-managed", nonManagedKey, "telegram", store)
	if resp.Result != "error" || resp.ErrorCode != "E_USAGE" {
		t.Fatalf("expected non-managed usage error, got %+v", resp)
	}
	if got := store.get(nonManagedKey).Step; got != OnboardAgentSelected {
		t.Fatalf("expected step reset to agent_selected, got %q", got)
	}

	unsupportedKey := "telegram:unsupported"
	store.start(unsupportedKey)
	store.update(unsupportedKey, func(s *OnboardSession) {
		s.Step = OnboardChannelSelect
		s.SelectedAgent = "openclaw"
	})
	resp = onboardSelectChannel("req-unsupported", unsupportedKey, "discord", store)
	if resp.Result != "error" || resp.ErrorCode != "E_USAGE" {
		t.Fatalf("expected unsupported-channel usage error, got %+v", resp)
	}
}

func TestCredentialReuseHint_AndPromptBranches(t *testing.T) {
	if got := credentialReuseHint(nil); got != "" {
		t.Fatalf("expected empty hint for nil provider, got %q", got)
	}

	tmp := t.TempDir()
	t.Setenv("CARRIER_DISABLE_KEYCHAIN", "1")
	t.Setenv("CARRIER_CREDENTIAL_STORE", filepath.Join(tmp, "credentials.json"))
	if got := credentialReuseHint(GetLLMProvider("openai")); got != "" {
		t.Fatalf("expected empty hint without saved credential, got %q", got)
	}

	resp := onboardPromptEnvVars("req-nil", nil)
	if resp.Result != "error" || resp.ErrorCode != "E_USAGE" {
		t.Fatalf("expected usage error for nil session prompt, got %+v", resp)
	}
}

func TestOnboardHandleAuth_NoSessionAndUnknownProvider(t *testing.T) {
	store := NewOnboardStore()
	resp := onboardHandleAuth("req-missing", "telegram:missing", "token", store)
	if resp.Result != "error" || resp.ErrorCode != "E_USAGE" {
		t.Fatalf("expected missing-session error, got %+v", resp)
	}

	key := "telegram:unknown-provider"
	store.start(key)
	store.update(key, func(s *OnboardSession) {
		s.Step = OnboardProviderSelected
		s.SelectedAgent = "openclaw"
		s.SelectedProvider = "unknown-provider"
	})
	resp = onboardHandleAuth("req-unknown-provider", key, "anything", store)
	if resp.Result != "ok" {
		t.Fatalf("expected ok response for unknown provider edge case, got %+v", resp)
	}
	if got := store.get(key).Step; got != OnboardAuthConfigured {
		t.Fatalf("expected step auth_configured, got %q", got)
	}
}

func TestOnboardConfirm_ManagedPrepareFailure(t *testing.T) {
	store := NewOnboardStore()
	key := "telegram:managed-prepare-fail"
	seedManagedEnvConfiguredSession(store, key, "openclaw", "", "", "openai", map[string]string{
		"OPENAI_API_KEY": "sk-test",
	})

	resp := onboardConfirm(context.Background(), "req-managed-prepare-fail", key, "yes", nil, store, "telegram:1")
	if resp.Result != "error" || resp.ErrorCode != "E_ENV" {
		t.Fatalf("expected E_ENV on prepare failure, got %+v", resp)
	}
	if got := store.get(key).Step; got != OnboardEnvConfigured {
		t.Fatalf("expected rollback to env_configured, got %q", got)
	}
}

func TestOnboardConfirm_ManagedOpenClawSuccessIncludesSetupNotes(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/openclaw/install": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		},
		"POST /api/v1/agents/openclaw/start": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		},
		"GET /api/v1/agents/openclaw/status": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"statuses": []map[string]interface{}{{
					"id":           "openclaw",
					"name":         "OpenClaw",
					"installState": "installed",
					"runtimeState": "running",
					"health":       "healthy",
				}},
			})
		},
	})

	store := NewOnboardStore()
	key := "telegram:managed-openclaw-success"
	seedManagedEnvConfiguredSession(store, key, "openclaw", "telegram", "tg-token", "openai", map[string]string{
		"OPENAI_API_KEY": "sk-openclaw",
	})

	resp := onboardConfirm(context.Background(), "req-managed-openclaw-success", key, "yes", daemon, store, "telegram:123")
	if resp.Result != "ok" {
		t.Fatalf("expected managed openclaw success, got %+v", resp)
	}
	if got := store.get(key).Step; got != OnboardDone {
		t.Fatalf("expected step done, got %q", got)
	}
	for _, want := range []string{"OpenClaw workspace:", "OpenClaw config:", "Carrier record:"} {
		if !strings.Contains(resp.Message, want) {
			t.Fatalf("expected %q in response, got %q", want, resp.Message)
		}
	}
}

func TestOnboardConfirm_PicoClawPairHintFromLogs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	_, daemon, _, _, _ := setupTestEnv(t, map[string]http.HandlerFunc{
		"POST /api/v1/agents/picoclaw/install": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		},
		"POST /api/v1/agents/picoclaw/start": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		},
		"GET /api/v1/agents/picoclaw/status": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"statuses": []map[string]interface{}{{
					"id":           "picoclaw",
					"name":         "PicoClaw",
					"installState": "installed",
					"runtimeState": "running",
					"health":       "healthy",
				}},
			})
		},
		"GET /api/v1/agents/picoclaw/logs": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"lines": []string{"PAIR_CODE: pair-0123456789abcdef0123456789abcdef"},
			})
		},
	})

	store := NewOnboardStore()
	key := "telegram:managed-picoclaw-success"
	seedManagedEnvConfiguredSession(store, key, "picoclaw", "telegram", "tg-token", "openai", map[string]string{
		"OPENAI_API_KEY": "sk-pico",
	})

	resp := onboardConfirm(context.Background(), "req-managed-picoclaw-success", key, "yes", daemon, store, "telegram:123")
	if resp.Result != "ok" {
		t.Fatalf("expected managed picoclaw success, got %+v", resp)
	}
	if !strings.Contains(resp.Message, "PicoClaw pair code") {
		t.Fatalf("expected pair hint with code, got %q", resp.Message)
	}
}
